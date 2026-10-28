package gosimilarity

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

type inputFile struct {
	path   string
	direct bool
}

func canonicalPath(name string) (string, error) {
	// EvalSymlinks must see .. before Abs or Clean can collapse it.
	resolved, err := filepath.EvalSymlinks(name)
	if err == nil {
		abs, err := filepath.Abs(resolved)
		if err != nil {
			return "", err
		}
		// A relative result may still inherit symlinks from the cwd.
		return filepath.EvalSymlinks(abs)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	// Resolve existing parents even for missing explicitly named files.
	for len(name) > 0 && os.IsPathSeparator(name[len(name)-1]) {
		name = name[:len(name)-1]
	}
	parent, base := filepath.Split(name)
	if info, e := os.Lstat(name); e == nil && info.Mode()&os.ModeSymlink != 0 {
		target, e := os.Readlink(name)
		if e != nil {
			return "", e
		}
		if !filepath.IsAbs(target) {
			// Joining here would prematurely clean symlink/.. in the target.
			target = parent + target
		}
		return canonicalPath(target)
	}
	if base == "" || base == "." || base == ".." {
		// In particular, do not collapse missing/.. into an existing parent.
		return "", err
	}
	if parent == "" {
		parent = "."
	}
	p, err := canonicalPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(p, base), nil
}

func displayPath(cwd, name string) string {
	rel, err := filepath.Rel(cwd, name)
	if err != nil {
		return filepath.ToSlash(name)
	}
	return filepath.ToSlash(rel)
}

func matchGlob(pattern, name string) bool {
	p, s := strings.Split(pattern, "/"), strings.Split(name, "/")
	type index struct{ p, s int }
	memo := make(map[index]bool)
	var match func(int, int) bool
	match = func(i, j int) bool {
		key := index{i, j}
		if result, ok := memo[key]; ok {
			return result
		}
		result := false
		switch {
		case i == len(p):
			result = j == len(s)
		case p[i] == "**":
			result = match(i+1, j) || (j < len(s) && match(i, j+1))
		case j < len(s):
			ok, _ := path.Match(p[i], s[j])
			result = ok && match(i+1, j+1)
		}
		memo[key] = result
		return result
	}
	return match(0, 0)
}

func excluded(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchGlob(pattern, name) {
			return true
		}
		// A matched directory excludes its descendants too.
		for p := path.Dir(name); p != "." && p != "/"; p = path.Dir(p) {
			if matchGlob(pattern, p) {
				return true
			}
		}
		if name != ".." && !strings.HasPrefix(name, "../") && !filepath.IsAbs(filepath.FromSlash(name)) && matchGlob(pattern, ".") {
			return true
		}
	}
	return false
}

func automaticExclusion(name string, tests bool) bool {
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		if part == ".git" || part == "vendor" {
			return true
		}
	}
	return !tests && strings.HasSuffix(name, "_test.go")
}

func gitRoot(dir string) string {
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func discover(ctx context.Context, paths []string, cwd string, opts options, result *report) []inputFile {
	files := make(map[string]bool)
	add := func(name string, direct bool) {
		if previous, ok := files[name]; !ok || direct {
			files[name] = previous || direct
		}
	}
	for _, input := range paths {
		if ctx.Err() != nil {
			result.diagnose("error", "discovery", "", "", ctx.Err().Error())
			break
		}
		name, err := canonicalPath(input)
		if err != nil {
			result.diagnose("error", "discovery", input, "", err.Error())
			continue
		}
		if excluded(displayPath(cwd, name), opts.excludes) {
			result.skip("excluded", displayPath(cwd, name), "matched --exclude")
			continue
		}
		info, err := os.Stat(name)
		if err != nil {
			result.diagnose("error", "discovery", displayPath(cwd, name), "", err.Error())
			continue
		}
		if !info.IsDir() {
			if !info.Mode().IsRegular() {
				result.diagnose("error", "discovery", displayPath(cwd, name), "", "input is not a regular file")
				continue
			}
			if filepath.Ext(name) != ".go" {
				result.diagnose("error", "discovery", displayPath(cwd, name), "", "input is not a Go file")
				continue
			}
			add(name, true)
			continue
		}
		var candidates []string
		if root := gitRoot(name); root != "" {
			candidates, err = collectGitFiles(ctx, root, name)
			if err != nil {
				// A corrupt repository or missing Git must not silently change discovery.
				result.diagnose("error", "git", displayPath(cwd, name), "", fmt.Sprintf("git ls-files failed: %v", err))
				continue
			}
		} else {
			candidates, err = collectWalkFiles(ctx, name, cwd, opts.excludes, result)
			if err != nil {
				result.diagnose("error", "discovery", displayPath(cwd, name), "", err.Error())
			}
		}
		for _, full := range candidates {
			add(full, false)
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	var inputs []inputFile
	for _, name := range names {
		direct := files[name]
		shown := displayPath(cwd, name)
		if excluded(shown, opts.excludes) {
			result.skip("excluded", shown, "matched --exclude")
			continue
		}
		if !direct && !validDiscoveredFile(name, shown, opts.includeTests, result) {
			continue
		}
		inputs = append(inputs, inputFile{path: name, direct: direct})
	}
	return inputs
}

func collectGitFiles(ctx context.Context, root, dir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", "*.go")
	data, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range strings.Split(string(data), "\x00") {
		if entry == "" {
			continue
		}
		full := filepath.Join(root, filepath.FromSlash(entry))
		rel, err := filepath.Rel(dir, full)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		files = append(files, full)
	}
	return files, nil
}

func collectWalkFiles(ctx context.Context, dir, cwd string, excludes []string, result *report) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(full string, entry fs.DirEntry, err error) error {
		if err != nil {
			result.diagnose("error", "discovery", displayPath(cwd, full), "", err.Error())
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "vendor" || excluded(displayPath(cwd, full), excludes) {
				return filepath.SkipDir
			}
		} else if filepath.Ext(full) == ".go" {
			files = append(files, full)
		}
		return nil
	})
	return files, err
}

func validDiscoveredFile(name, shown string, includeTests bool, result *report) bool {
	if automaticExclusion(name, includeTests) {
		result.skip("automatic", shown, "automatically excluded path")
		return false
	}
	// Check every component, since a tracked path may traverse a symlink.
	for p := name; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				result.skip("missing", shown, "listed file is absent from the working tree")
			} else {
				result.diagnose("error", "read", shown, "", err.Error())
			}
			return false
		}
		if info.Mode()&os.ModeSymlink != 0 {
			result.skip("symlink", shown, "automatically discovered symlink")
			return false
		}
		if p == name && !info.Mode().IsRegular() {
			result.skip("non_regular", shown, "not a regular Go file")
			return false
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return true
}
