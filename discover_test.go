package gosimilarity

import (
	"os"
	"reflect"
	"testing"
)

func TestDiscoveryExplicitNonGo(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "notes.txt", "not Go source")
	writeFile(t, "good.go", "package example\ntype Good int")

	r, code, _ := runJSON(t, "notes.txt", "good.go", "--types")
	want := []diagnostic{{
		Severity: "error", Stage: "discovery", File: "notes.txt",
		Message: "input is not a Go file",
	}}
	if code != 2 || r.Status != "incomplete" || !reflect.DeepEqual(r.Diagnostics, want) ||
		!reflect.DeepEqual(declarationFiles(r), []string{"good.go"}) {
		t.Fatalf("non-Go input did not preserve partial results: code=%d report=%+v", code, r)
	}

	r, code, _ = runJSON(t, "notes.txt", "--exclude", "notes.txt")
	if code != 0 || r.Status != "complete" || r.Summary.Files != 0 || r.Summary.SkippedReasons["excluded"] != 1 {
		t.Fatalf("explicit exclusion did not precede extension validation: code=%d report=%+v", code, r)
	}
}

func TestDiscoveryGitSubdirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	git(t, "init", "-q")
	for _, name := range []string{"outside.go", "src-extra/sibling.go", "src/tracked.go", "src/nested/untracked.go"} {
		writeFile(t, name, "package example\ntype Example int")
	}
	git(t, "add", "outside.go", "src-extra/sibling.go", "src/tracked.go")

	r, code, _ := runJSON(t, "src", "--types")
	want := []string{"src/nested/untracked.go", "src/tracked.go"}
	if code != 0 || r.Status != "complete" || !reflect.DeepEqual(declarationFiles(r), want) ||
		r.Summary.ParsedFiles != 2 || len(r.Diagnostics) != 0 {
		t.Fatalf("Git collection escaped the requested directory: code=%d report=%+v", code, r)
	}
}

func TestDiscoveryExcludedParent(t *testing.T) {
	for _, inGit := range []bool{false, true} {
		t.Run(map[bool]string{false: "walk", true: "git"}[inGit], func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			if inGit {
				git(t, "init", "-q")
			}
			writeFile(t, "keep.go", "package example\ntype Keep int")
			writeFile(t, "excluded/nested/child.go", "package example\ntype Excluded int")

			r, code, _ := runJSON(t, ".", "excluded/nested/child.go", "excluded/missing.go", "--exclude", "excluded", "--types")
			wantSkipped := 2
			if inGit {
				wantSkipped++ // Git lists the child; WalkDir prunes its parent.
			}
			if code != 0 || r.Status != "complete" || !reflect.DeepEqual(declarationFiles(r), []string{"keep.go"}) ||
				r.Summary.SkippedReasons["excluded"] != wantSkipped || len(r.Diagnostics) != wantSkipped {
				t.Fatalf("excluded parent did not suppress discovered and explicit children: code=%d report=%+v", code, r)
			}
		})
	}
}
