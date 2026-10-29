package gosimilarity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runJSON(t *testing.T, args ...string) (*report, int, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	err := Run(context.Background(), append([]string{"--format", "json"}, args...), &out, &stderr)
	code := 0
	if err != nil {
		var e interface{ ExitCode() int }
		if !errors.As(err, &e) {
			t.Fatal(err)
		}
		code = e.ExitCode()
	}
	var result report
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON %q: %v", out.String(), err)
	}
	return &result, code, stderr.String()
}

func TestRun(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "a.go", `package example
//line virtual.go:1000
func First(x int) int {
	return x + 1
}
type Node[T any] struct { Next *Node[T]; Value T }
type Alias = string
type Names []string
type Shape interface { Open() error }
`)
	writeFile(t, "b.go", `package other
func Second(y int) int {
	return y + 1
}
type Node[U any] struct { Next *Node[U]; Value U }
type Alias = string
type Names []string
type Shape interface { Open() error }
`)
	args := []string{"a.go", "-t", "1", "b.go", "-p", "--types", "--fail-on-duplicates"}
	t.Run("basic report", func(t *testing.T) {
		r, code, _ := runJSON(t, args...)
		if code != 1 || r.Status != "complete" || len(r.Pairs) != 5 || r.Summary.Functions != 2 || r.Summary.Types != 8 {
			t.Fatalf("unexpected result: code=%d summary=%+v", code, r.Summary)
		}
		if r.Declarations[0].Start.Line != 3 || r.Declarations[0].End.Line != 5 || r.Declarations[0].File != "a.go" {
			t.Fatal("locations are not physical source positions")
		}
		decls := make(map[string]bool)
		for _, d := range r.Declarations {
			decls[d.ID] = true
			if d.Source == "" || d.Nodes == 0 {
				t.Fatal("missing source or node count")
			}
		}
		for _, p := range r.Pairs {
			if !decls[p.A] || !decls[p.B] || p.Similarity != 1 {
				t.Fatal("unresolved pair reference or inexact match")
			}
		}
	})
	t.Run("category selection", func(t *testing.T) {
		for _, tt := range []struct {
			name             string
			args             []string
			functions, types int
		}{
			{"default", nil, 2, 0},
			{"types enabled", []string{"--types"}, 2, 8},
			{"types ignore function filters", []string{"--no-functions", "--min-tokens", "9999", "--filter-function", "absent"}, 0, 8},
			{"types disabled", []string{"--no-types"}, 2, 0},
			{"types false", []string{"--types=false"}, 2, 0},
		} {
			t.Run(tt.name, func(t *testing.T) {
				result, code, _ := runJSON(t, append([]string{".", "-t", "1"}, tt.args...)...)
				if code != 0 || len(result.Pairs) == 0 || result.Summary.Functions != tt.functions ||
					result.Summary.Types != tt.types || len(result.Declarations) != tt.functions+tt.types {
					t.Fatalf("%v: unexpected categories: code=%d summary=%+v", tt.args, code, result.Summary)
				}
			})
		}
	})
	t.Run("determinism", func(t *testing.T) {
		first, firstCode, firstStderr := runJSON(t, args...)
		again, code, stderr := runJSON(t, args...)
		if !reflect.DeepEqual(first, again) || firstCode != code || firstStderr != stderr {
			t.Fatal("output is not deterministic")
		}
	})
	t.Run("text rendering", func(t *testing.T) {
		var text, stderr bytes.Buffer
		if err := Run(context.Background(), []string{".", "-t", "1", "-p", "--types"}, &text, &stderr); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"Similarity: 100.00%", "[function]", "[type]", "a.go:3-5 First", "return x + 1"} {
			if !strings.Contains(text.String(), want) {
				t.Errorf("text missing %q", want)
			}
		}
	})
}

func TestPartialFailureAndSizeOptions(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "ok.go", "package example\nfunc A() { work() }\nfunc B() { work() }\ntype AType int\ntype BType int\n")
	writeFile(t, "broken.go", "package example\nfunc broken(")
	r, code, _ := runJSON(t, ".", "--types", "--fail-on-duplicates")
	if code != 2 || r.Status != "incomplete" || len(r.Pairs) != 1 || r.Summary.FilteredFunctions != 2 {
		t.Fatalf("partial results lost: code=%d, %+v", code, r)
	}
	r, code, stderr := runJSON(t, "ok.go", "--types", "-m", "100", "--min-tokens", "0")
	if code != 0 || len(r.Pairs) != 2 || !strings.Contains(stderr, "overrides") {
		t.Fatal("min-tokens did not override min-lines with a warning")
	}
	r, code, _ = runJSON(t, "ok.go", "--no-functions", "--no-types")
	if code != 2 || r.Summary.Files != 0 {
		t.Fatal("no categories must fail before discovery")
	}
	r, code, _ = runJSON(t, "missing.go", "--types", "--fail-on-duplicates", "ok.go")
	if code != 2 || r.Status != "incomplete" || len(r.Pairs) != 1 {
		t.Fatal("missing explicit file did not override duplicate exit")
	}
}

func TestCompactTypeSimilarity(t *testing.T) {
	for _, tt := range []struct {
		name, source, threshold string
		nodes                   int
		similarity              float64
	}{
		{
			"unrelated fields",
			"type inputFile struct { path string; direct bool }\ntype exitError struct { code int; message string }",
			".87", 9, 1 - 1.5/9,
		},
		{
			"declared name",
			"type A struct { Value int }\ntype B struct { Value int }",
			"1", 6, .95,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			writeFile(t, "example.go", "package example\n"+tt.source)
			r, code, _ := runJSON(t, ".", "--types", "-t", "0")
			if code != 0 || len(r.Pairs) != 1 || len(r.Declarations) != 2 {
				t.Fatalf("unexpected report: code=%d, %+v", code, r)
			}
			if math.Abs(r.Pairs[0].Similarity-tt.similarity) > 1e-12 {
				t.Fatalf("similarity = %g, want %g", r.Pairs[0].Similarity, tt.similarity)
			}
			for _, d := range r.Declarations {
				if d.Nodes != tt.nodes {
					t.Fatalf("nodes = %d, want %d", d.Nodes, tt.nodes)
				}
			}
			r, code, _ = runJSON(t, ".", "--types", "-t", tt.threshold)
			if code != 0 || len(r.Pairs) != 0 {
				t.Fatal("type difference passed the threshold")
			}
		})
	}
}

func TestCompactFunctionSize(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "example.go", `package example
func First(x int) int {
	return x + 1
}
func Second(y int) int {
	return y + 2
}
`)
	r, code, _ := runJSON(t, ".", "--min-tokens", "5")
	if code != 0 || len(r.Pairs) != 1 || r.Summary.Functions != 2 ||
		math.Abs(r.Pairs[0].Similarity-.94) > 1e-12 {
		t.Fatalf("unexpected compact function score: code=%d, %+v", code, r)
	}
	r, code, _ = runJSON(t, ".", "--min-tokens", "6")
	if code != 0 || r.Summary.Functions != 0 || r.Summary.FilteredFunctions != 2 || len(r.Pairs) != 0 {
		t.Fatal("min-tokens counted containers or absent slots")
	}
}

func TestCLIOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--rename-cost", "0"}, {"-r", "-1"}, {"-r", "NaN"}, {"-r", "+Inf"},
		{"-t", "NaN"}, {"-t", "1.1"}, {"-t", "-.1"}, {"--min-tokens", "-1"},
		{"--min-lines", "-1"}, {"--format", "yaml"}, {"--unknown"},
		{"--exclude", "["}, {"--threshold"}, {"--no-size-penalty"},
		{"--types", "--no-types"}, {"--no-types", "--types"},
		{"--tags", "integration"}, {"--tags="}, {"--packages=false", "--tags", "integration"},
	} {
		var out, stderr bytes.Buffer
		err := Run(context.Background(), args, &out, &stderr)
		var e interface{ ExitCode() int }
		if !errors.As(err, &e) || e.ExitCode() != 2 {
			t.Errorf("%v: expected exit 2, got %v", args, err)
		}
	}
	for _, arg := range []string{"-h", "--help"} {
		var out, stderr bytes.Buffer
		if err := Run(context.Background(), []string{arg}, &out, &stderr); !errors.Is(err, flag.ErrHelp) {
			t.Fatal("help failed", err)
		}
		help := stderr.String()
		for _, want := range []string{"-threshold, -t value", "(default 0.87)", "-rename-cost, -r value", "(default 0.3)", "-version, -V"} {
			if strings.Count(help, want) != 1 {
				t.Errorf("%s help: want exactly one %q in:\n%s", arg, want, help)
			}
		}
	}
	{
		var out, stderr bytes.Buffer
		if err := Run(context.Background(), []string{"--threshold", "1e400"}, &out, &stderr); err == nil || !strings.Contains(stderr.String(), "value out of range") {
			t.Errorf("expected out of range error, got %v: %s", err, stderr.String())
		}
	}
	for _, arg := range []string{"-V", "--version"} {
		var out, stderr bytes.Buffer
		if err := Run(context.Background(), []string{arg}, &out, &stderr); err != nil || !strings.HasPrefix(out.String(), "gosimilarity v") {
			t.Fatal("version failed", err)
		}
	}
	t.Chdir(t.TempDir())
	writeFile(t, "-example.go", "package example\ntype A int\ntype B int")
	r, code, _ := runJSON(t, "--fail-on-duplicates", "--", "-example.go")
	if code != 0 || len(r.Declarations) != 0 || r.Summary.TypeCandidates != 0 || len(r.Pairs) != 0 {
		t.Fatal("types were compared without opting in")
	}
	r, code, _ = runJSON(t, "--types", "--fail-on-duplicates", "--", "-example.go")
	if code != 1 || len(r.Pairs) != 1 {
		t.Fatal("-- did not terminate option parsing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, stderr bytes.Buffer
	err := Run(ctx, nil, &out, &stderr)
	var e interface{ ExitCode() int }
	if !errors.As(err, &e) || e.ExitCode() != 2 {
		t.Fatal("cancellation did not mark inspection incomplete")
	}
}

func TestFiltersAndGrouping(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "example.go", `package example
func Target() {
	work("a")
}
func Middle() {
	work("b") // needle
}
func Last() {
	work("c")
}
type A int
type B int
`)
	r, code, _ := runJSON(t, ".", "--types", "--filter-function", "Target", "--filter-function-body", "needle")
	if code != 0 || len(r.Pairs) != 2 || len(r.Groups) != 0 {
		t.Fatal("pair-wide name/body filters or type filter independence failed")
	}
	r, code, _ = runJSON(t, ".", "--filter-function-body", "Target", "--no-types")
	if code != 0 || len(r.Pairs) != 0 {
		t.Fatal("body filter searched signature")
	}
	r, code, _ = runJSON(t, ".", "--filter-function", "Middle", "--no-types")
	if code != 0 || len(r.Pairs) != 2 || len(r.Groups) != 1 || len(r.Groups[0].Pairs) != 2 || len(r.Groups[0].Members) != 3 {
		t.Fatal("group did not retain exactly the filtered edges")
	}
	memberIDs := make(map[string]bool)
	pairIDs := make(map[string]bool)
	for _, d := range r.Declarations {
		memberIDs[d.ID] = true
	}
	for _, p := range r.Pairs {
		pairIDs[p.ID] = true
	}
	for _, id := range r.Groups[0].Members {
		if !memberIDs[id] {
			t.Fatal("unresolved member ID")
		}
	}
	for _, id := range r.Groups[0].Pairs {
		if !pairIDs[id] {
			t.Fatal("unresolved pair ID")
		}
	}
	r, _, _ = runJSON(t, ".", "--cross-file-only")
	if len(r.Pairs) != 0 {
		t.Fatal("cross-file filter retained same-file pairs")
	}
}

func git(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
}

func declarationFiles(r *report) []string {
	var names []string
	for _, d := range r.Declarations {
		if !slices.Contains(names, d.File) {
			names = append(names, d.File)
		}
	}
	return names
}

func setupDiscovery(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, name := range []string{"a.go", "sub/b.go", "vendor/v.go", "a_test.go", "ignored.go"} {
		writeFile(t, name, "package example\ntype "+strings.ReplaceAll(filepath.Base(name), ".", "_")+" int\n")
	}
	writeFile(t, "generated.go", "// Code generated by example. DO NOT EDIT.\npackage example\ntype Generated int\n")
	writeFile(t, ".gitignore", "ignored.go\ntracked.go\n")
	return dir
}

func TestDiscovery(t *testing.T) {
	for _, inGit := range []bool{false, true} {
		t.Run(map[bool]string{false: "walk", true: "git"}[inGit], func(t *testing.T) {
			dir := setupDiscovery(t)
			if inGit {
				git(t, "init", "-q")
				writeFile(t, "tracked.go", "package example\ntype Old int")
				writeFile(t, "deleted.go", "package example\ntype Deleted int")
				git(t, "add", "-f", "tracked.go", "deleted.go")
				writeFile(t, "tracked.go", "package example\ntype WorkingTree int")
				if err := os.Remove("deleted.go"); err != nil {
					t.Fatal(err)
				}
			}
			r, code, _ := runJSON(t, ".", "--types")
			if code != 0 || r.Status != "complete" {
				t.Fatal("discovery incomplete")
			}
			want := []string{"a.go", "sub/b.go"}
			if inGit {
				want = append(want, "tracked.go")
			} else {
				want = append(want, "ignored.go")
				slices.Sort(want)
			}
			if got := declarationFiles(r); !reflect.DeepEqual(got, want) {
				t.Fatalf("files=%v, want %v", got, want)
			}
			if inGit {
				if r.Summary.SkippedReasons["missing"] != 1 || r.Declarations[len(r.Declarations)-1].Name != "WorkingTree" {
					t.Fatal("tracked deletion or current working tree not respected")
				}
			}
			withTests, _, _ := runJSON(t, ".", "--types", "--include-tests")
			if !slices.Contains(declarationFiles(withTests), "a_test.go") || slices.Contains(declarationFiles(withTests), "generated.go") {
				t.Fatal("include-tests changed unrelated exclusions")
			}
			one, _, _ := runJSON(t, ".", "generated.go", "ignored.go", "a_test.go", "vendor/v.go", "--types")
			two, _, _ := runJSON(t, "vendor/v.go", "a_test.go", "ignored.go", "generated.go", ".", "--types")
			if !reflect.DeepEqual(one, two) || one.Summary.ParsedFiles != len(want)+4-boolInt(!inGit) {
				t.Fatal("explicit inputs do not override automatic exclusions deterministically")
			}
			for _, args := range [][]string{
				{".", "generated.go", "--exclude", "generated.go", "--exclude", "sub/**"},
				{"--exclude", "sub/**", filepath.Join(dir, "generated.go"), ".", "--exclude", "generated.go"},
			} {
				excluded, code, _ := runJSON(t, append(args, "--types")...)
				if code != 0 || slices.Contains(declarationFiles(excluded), "generated.go") || slices.Contains(declarationFiles(excluded), "sub/b.go") {
					t.Fatal("explicit exclude precedence failed")
				}
			}
			missing, code, _ := runJSON(t, "absent.go", "--exclude", "absent.go")
			if code != 0 || missing.Status != "complete" {
				t.Fatal("excluded missing explicit file was read")
			}
		})
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func TestSymlinkIdentity(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "real/a.go", "package example\ntype A int")
	writeFile(t, "outside/b.go", "package example\ntype B int")
	for _, link := range [][2]string{{"real", "link"}, {"a.go", "real/link.go"}, {"../outside", "real/nested"}} {
		if err := os.Symlink(link[0], link[1]); err != nil {
			t.Skip("symlinks unavailable:", err)
		}
	}
	r, code, _ := runJSON(t, "real", "link", "real/a.go", "real/link.go", "--types")
	if code != 0 || r.Summary.ParsedFiles != 1 || len(r.Declarations) != 1 {
		t.Fatal("explicit symlinks did not deduplicate or nested link was followed")
	}
	r, code, _ = runJSON(t, "real/link.go", "--exclude", "real/a.go")
	if code != 0 || r.Summary.ParsedFiles != 0 {
		t.Fatal("exclusion did not use real path")
	}
}

func TestGitFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "example.go", "package example\ntype A int")
	t.Setenv("PATH", t.TempDir())
	_, code, _ := runJSON(t, ".")
	if code != 0 {
		t.Fatal("non-Git discovery required Git")
	}

	if err := os.Mkdir(".git", 0o755); err != nil {
		t.Fatal(err)
	}
	r, code, _ := runJSON(t, ".")
	if code != 2 || r.Status != "incomplete" || r.Summary.Files != 0 {
		t.Fatal("Git failure silently fell back to walking")
	}
}

func TestPruningBoundaryAndRanking(t *testing.T) {
	ts := newTrees()
	leaf := ts.node("x")
	children := make([]*tree, 8)
	for i := range children {
		children[i] = leaf
	}
	a := ts.node("root", children...)
	b := ts.node("root", append(children, leaf)...)
	makeReport := func() *report {
		r := newReport()
		r.Declarations = []declaration{
			{ID: "d1", Kind: "function", Name: "A", Start: position{Line: 1}, End: position{Line: 3}, tree: a, Nodes: a.size},
			{ID: "d2", Kind: "function", Name: "B", Start: position{Line: 4}, End: position{Line: 8}, tree: b, Nodes: b.size},
		}
		return r
	}
	r := makeReport()
	compare(context.Background(), options{threshold: .9, renameCost: .3}, r)
	if len(r.Pairs) != 1 || r.Pairs[0].Similarity != .9 || r.Pairs[0].Score != 3.6 {
		t.Fatal("safe size-boundary match was pruned or ranked incorrectly")
	}
	r = makeReport()
	compare(context.Background(), options{threshold: 0, renameCost: .3}, r)
	if len(r.Pairs) != 1 || r.Pairs[0].Similarity != .9 {
		t.Fatal("pruned and unpruned comparison differ")
	}
}

func TestReadFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "good.go", "package example\ntype A int\ntype B int")
	writeFile(t, "unreadable.go", "package example\ntype C int")
	if err := os.Chmod("unreadable.go", 0); err != nil {
		t.Skip("permissions unavailable:", err)
	}
	t.Cleanup(func() { _ = os.Chmod("unreadable.go", 0o600) })
	if file, err := os.Open("unreadable.go"); err == nil {
		file.Close()
		t.Skip("environment can read files without permission")
	}
	r, code, _ := runJSON(t, "good.go", "unreadable.go", "--types", "--fail-on-duplicates")
	if code != 2 || r.Status != "incomplete" || len(r.Pairs) != 1 || r.Summary.ParsedFiles != 1 {
		t.Fatal("read failure lost partial results or error priority")
	}
}

func TestEmptyReceiver(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "example.go", "package example\nfunc () F() { work() }\ntype A int\ntype B int")
	r, code, _ := runJSON(t, ".", "--types")
	if code != 2 || r.Status != "incomplete" || len(r.Pairs) != 1 || r.Summary.Types != 2 || r.Summary.Functions != 0 {
		t.Fatal("malformed receiver did not preserve other results")
	}
}

func TestSymlinkParentTraversal(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, "work/a.go", "package example\ntype Wrong int")
	writeFile(t, "other/a.go", "package example\ntype Correct int")
	if err := os.Mkdir("other/sub", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../other/sub", "work/link"); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	// Do not use filepath.Join: it would clean .. before the filesystem lookup.
	for _, name := range []string{"work/link/../a.go", filepath.ToSlash(dir) + "/work/link/../a.go"} {
		r, code, _ := runJSON(t, name, "--types")
		if code != 0 || len(r.Declarations) != 1 || r.Declarations[0].Name != "Correct" {
			t.Fatal("symlink traversal inspected the wrong file")
		}
		_, code, _ = runJSON(t, strings.Replace(name, "a.go", "missing.go", 1), "--exclude", "other/missing.go")
		if code != 0 {
			t.Fatal("missing symlink target used the wrong exclusion path")
		}
	}
	_, code, _ := runJSON(t, "work/missing/../a.go")
	if code != 2 {
		t.Fatal("missing intermediate component inspected an existing file")
	}
}

func TestExcludeCwd(t *testing.T) {
	root := t.TempDir()
	if excluded(filepath.ToSlash(root), []string{"."}) {
		t.Fatal("absolute fallback path was treated as inside cwd")
	}
	t.Chdir(root)
	writeFile(t, "inside/a.go", "package example\ntype A int")
	writeFile(t, "outside.go", "package example\ntype B int")
	t.Chdir(filepath.Join(root, "inside"))
	for _, paths := range [][]string{{"a.go"}, {"."}} {
		r, code, _ := runJSON(t, append(paths, "--exclude", ".")...)
		if code != 0 || r.Summary.Files != 0 {
			t.Fatal("excluding cwd did not exclude its descendants")
		}
	}
	r, code, _ := runJSON(t, "../outside.go", "--types", "--exclude", ".")
	if code != 0 || len(r.Declarations) != 1 {
		t.Fatal("cwd was treated as an ancestor of an outside file")
	}
}

func TestDanglingSymlinkExclusion(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, name := range []string{"real/sub", "real/missing"} {
		if err := os.MkdirAll(name, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, link := range [][2]string{
		{"real/sub", "link"},
		{"link/../missing.go", "alias.go"},
		{filepath.Join(dir, "real", "missing.go"), "absolute.go"},
		{"alias.go", "chain.go"},
		{"real/missing", "directory"},
	} {
		if err := os.Symlink(link[0], link[1]); err != nil {
			t.Skip("symlinks unavailable:", err)
		}
	}
	if err := os.Remove("real/missing"); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ input, pattern string }{
		{"alias.go", "real/missing.go"},
		{"absolute.go", "real/missing.go"},
		{"chain.go", "real/missing.go"},
		{"directory/child.go", "real/missing/**"},
	} {
		r, code, stderr := runJSON(t, tt.input, "--exclude", tt.pattern)
		if code != 0 || r.Summary.Files != 0 {
			t.Fatalf("%s: excluded dangling target caused a discovery error: %s", tt.input, stderr)
		}
	}
	_, code, _ := runJSON(t, "alias.go")
	if code != 2 {
		t.Fatal("unexcluded dangling symlink did not report a missing file")
	}
}

func TestOutputStreams(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "good.go", "package example\ntype A int\ntype B int")
	writeFile(t, "ignored_test.go", "package example\ntype Ignored int")
	for _, format := range []string{"text", "json"} {
		var out, stderr bytes.Buffer
		err := Run(context.Background(), []string{".", "--types", "--format", format, "-m", "3", "--min-tokens", "0"}, &out, &stderr)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(stderr.String(), "level=WARN ") || strings.Count(stderr.String(), "\n") != 1 {
			t.Fatalf("%s: expected only a warning on stderr, got %q", format, stderr.String())
		}
		if format == "text" {
			if !strings.Contains(out.String(), "Similarity:") || strings.Contains(out.String(), "level=") {
				t.Fatal("text report and logs were not separated")
			}
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(out.Bytes(), &fields); err != nil {
			t.Fatal("logs contaminated JSON stdout:", err)
		}
		if _, ok := fields["schema_version"]; ok {
			t.Fatal("unexpected schema_version in JSON report")
		}
		var r report
		if err := json.Unmarshal(out.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		if r.Summary.SkippedReasons["automatic"] != 1 || len(r.Diagnostics) != 2 {
			t.Fatal("quiet skip diagnostics were lost from the structured report")
		}
	}
}
