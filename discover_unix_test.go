//go:build unix

package gosimilarity

import (
	"context"
	"errors"
	"strings"
	"syscall"
	"testing"
	"time"
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

func TestPackageDiscoveryCancellation(t *testing.T) {
	setupPackageModule(t)
	writeFile(t, "good.go", "package fixture\ntype Good int")
	if err := syscall.Mkfifo("pipe.go", 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r := newReport()
	inputs := discoverPackages(ctx, []string{"."}, ".", options{packages: true}, r)
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) || r.Status != "incomplete" || len(inputs) != 0 {
		t.Fatalf("blocked go list was not canceled: context=%v report=%+v inputs=%v", ctx.Err(), r, inputs)
	}
	if len(r.Diagnostics) != 1 || !strings.Contains(r.Diagnostics[0].Message, context.DeadlineExceeded.Error()) {
		t.Fatalf("cancellation reason missing: diagnostics=%+v", r.Diagnostics)
	}
}
