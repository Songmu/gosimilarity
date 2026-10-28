//go:build unix

package gosimilarity

import (
	"syscall"
	"testing"
)

func TestNonRegularGoInputs(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "good.go", "package example\ntype A int\ntype B int")
	if err := syscall.Mkfifo("pipe.go", 0o600); err != nil {
		t.Fatal(err)
	}
	r, code, _ := runJSON(t, "pipe.go")
	if code != 2 || r.Status != "incomplete" || r.Summary.ParsedFiles != 0 {
		t.Fatal("explicit non-regular input did not fail before reading")
	}
	r, code, _ = runJSON(t, ".", "--types")
	if code != 0 || r.Status != "complete" || len(r.Pairs) != 1 || r.Summary.SkippedReasons["non_regular"] != 1 {
		t.Fatal("directory discovery did not skip non-regular input")
	}
}
