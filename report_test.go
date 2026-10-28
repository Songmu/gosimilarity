package gosimilarity

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestRunGroupedText(t *testing.T) {
	t.Chdir(t.TempDir())
	sources := []string{
		"func (b *Box) First(x int) int {\n\treturn x + 1\n}",
		"func (b Box) Middle(y int) int {\n\treturn y + 1\n}",
		"func (b *Box) Last(z int) int {\n\treturn z + 1\n}",
	}
	writeFile(t, "methods.go", "package example\ntype Box struct{}\n"+strings.Join(sources, "\n")+"\n")
	for _, printSource := range []bool{false, true} {
		t.Run(map[bool]string{false: "without source", true: "with source"}[printSource], func(t *testing.T) {
			args := []string{"methods.go", "--filter-function", "Middle", "-t", "1"}
			if printSource {
				args = append(args, "--print")
			}
			var out, stderr bytes.Buffer
			if err := Run(context.Background(), args, &out, &stderr); err != nil {
				t.Fatal(err)
			}
			want := "Group g1 [function]: 3 declarations, 2 matching pairs (not necessarily all-to-all)\n" +
				"Similarity: 100.00%, Score: 3.0 points [function]\n" +
				"  methods.go:3-5 (*Box).First\n" +
				"  methods.go:6-8 (Box).Middle\n" +
				"  nodes: 5 / 5\n" +
				"Similarity: 100.00%, Score: 3.0 points [function]\n" +
				"  methods.go:6-8 (Box).Middle\n" +
				"  methods.go:9-11 (*Box).Last\n" +
				"  nodes: 5 / 5\n"
			if printSource {
				for _, source := range sources {
					want += "\n" + source + "\n"
				}
			}
			want += "\nStatus: complete; 1 files parsed, 3 functions, 0 types, 2 matching pairs, 0 files skipped\n"
			if got := out.String(); got != want {
				t.Fatalf("text report:\n%s\nwant:\n%s", got, want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected diagnostics: %s", &stderr)
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestRunWriterFailures(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "example.go", "package example\ntype A int\ntype B int\n")
	for _, tt := range []struct {
		name        string
		args        []string
		out, stderr io.Writer
	}{
		{"text", nil, failingWriter{}, io.Discard},
		{"json", []string{"--format", "json"}, failingWriter{}, io.Discard},
		{"diagnostics", []string{"-m", "3", "--min-tokens", "0"}, io.Discard, failingWriter{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"example.go", "--types", "--fail-on-duplicates"}, tt.args...)
			err := Run(context.Background(), args, tt.out, tt.stderr)
			var exit interface{ ExitCode() int }
			if !errors.As(err, &exit) || exit.ExitCode() != 2 || !strings.Contains(err.Error(), "write failed") {
				t.Fatalf("expected write failure with exit 2, got %v", err)
			}
		})
	}
}
