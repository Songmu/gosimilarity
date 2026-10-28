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
	seen := make(map[index]bool)
	var match func(int, int) bool
	match = func(i, j int) bool {
		key := index{i, j}
		if seen[key] {
			return memo[key]
		}
		seen[key] = true
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
		if root := gitRoot(name); root != "" {
			cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", "*.go")
			// A corrupt repository or missing Git must not silently change discovery.
			data, err := cmd.Output()
			if err != nil {
				result.diagnose("error", "git", displayPath(cwd, name), "", fmt.Sprintf("git ls-files failed: %v", err))
				continue
			}
			for _, entry := range strings.Split(string(data), "\x00") {
				if entry == "" {
					continue
				}
				full := filepath.Join(root, filepath.FromSlash(entry))
				rel, err := filepath.Rel(name, full)
				if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					continue
				}
				add(full, false)
			}
		} else {
			err := filepath.WalkDir(name, func(full string, entry fs.DirEntry, err error) error {
				if err != nil {
					result.diagnose("error", "discovery", displayPath(cwd, full), "", err.Error())
					return nil
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if entry.IsDir() {
					if entry.Name() == ".git" || entry.Name() == "vendor" || excluded(displayPath(cwd, full), opts.excludes) {
						return filepath.SkipDir
					}
				} else if filepath.Ext(full) == ".go" {
					add(full, false)
				}
				return nil
			})
			if err != nil {
				result.diagnose("error", "discovery", displayPath(cwd, name), "", err.Error())
			}
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
		if !direct {
			if automaticExclusion(name, opts.includeTests) {
				result.skip("automatic", shown, "automatically excluded path")
				continue
			}
			// Check every component, since a tracked path may traverse a symlink.
			link := false
			for p := name; ; p = filepath.Dir(p) {
				info, err := os.Lstat(p)
				if err != nil {
					if errors.Is(err, fs.ErrNotExist) {
						result.skip("missing", shown, "listed file is absent from the working tree")
					} else {
						result.diagnose("error", "read", shown, "", err.Error())
					}
					link = true
					break
				}
				if info.Mode()&os.ModeSymlink != 0 {
					result.skip("symlink", shown, "automatically discovered symlink")
					link = true
					break
				}
				if p == name && !info.Mode().IsRegular() {
					result.skip("non_regular", shown, "not a regular Go file")
					link = true
					break
				}
				if filepath.Dir(p) == p {
					break
				}
			}
			if link {
				continue
			}
		}
		inputs = append(inputs, inputFile{path: name, direct: direct})
	}
	return inputs
}
