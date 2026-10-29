package gosimilarity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

func discoverPackages(ctx context.Context, patterns []string, cwd string, opts options, result *report) []inputFile {
	args := []string{"list", "-e", "-find", "-json"}
	if opts.tagsSet {
		args = append(args, "-tags="+opts.tags)
	}
	args = append(args, "--")
	args = append(args, patterns...)
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = cwd
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		message := fmt.Sprintf("go list failed: %v", err)
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			message += ": " + detail
		}
		result.diagnose("error", "packages", "", "", message)
	} else if detail := strings.TrimSpace(stderr.String()); detail != "" {
		result.diagnose("warning", "packages", "", "", detail)
	}

	files := make(map[string]bool)
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var pkg struct {
			Dir          string
			ImportPath   string
			GoFiles      []string
			CgoFiles     []string
			TestGoFiles  []string
			XTestGoFiles []string
			Error        *struct{ Err string }
		}
		if err := dec.Decode(&pkg); err == io.EOF {
			break
		} else if err != nil {
			result.diagnose("error", "packages", "", "", fmt.Sprintf("invalid go list output: %v", err))
			break
		}
		if pkg.ImportPath == "command-line-arguments" {
			result.diagnose("error", "packages", "", "", "--packages accepts package patterns, not Go file paths")
			continue
		}
		var dir string
		if pkg.Dir != "" {
			dir, err = canonicalPath(pkg.Dir)
			if err != nil {
				result.diagnose("error", "packages", pkg.Dir, "", err.Error())
				continue
			}
			if shown := displayPath(cwd, dir); excluded(shown, opts.excludes) {
				result.skip("excluded", shown, "matched --exclude")
				continue
			}
		}
		if pkg.Error != nil {
			result.diagnose("error", "packages", "", "", pkg.ImportPath+": "+pkg.Error.Err)
		}
		if dir == "" {
			continue
		}
		names := append(pkg.GoFiles, pkg.CgoFiles...)
		if opts.includeTests {
			names = append(names, pkg.TestGoFiles...)
			names = append(names, pkg.XTestGoFiles...)
		}
		for _, name := range names {
			files[filepath.Join(dir, name)] = true
		}
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	var inputs []inputFile
	for _, name := range names {
		shown := displayPath(cwd, name)
		if excluded(shown, opts.excludes) {
			result.skip("excluded", shown, "matched --exclude")
			continue
		}
		if validDiscoveredFile(name, shown, opts.includeTests, result) {
			inputs = append(inputs, inputFile{path: name})
		}
	}
	return inputs
}
