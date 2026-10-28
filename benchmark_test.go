package gosimilarity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkRepository(b *testing.B) {
	dir := b.TempDir()
	var src strings.Builder
	src.WriteString("package example\n")
	for i := range 10000 {
		fmt.Fprintf(&src, "func F%05d(x int) int {\n return x + %d\n}\n", i, i)
	}
	for i := range 100 {
		fmt.Fprintf(&src, "type T%d struct { Field%d int }\n", i, i)
	}
	name := filepath.Join(dir, "example.go")
	if err := os.WriteFile(name, []byte(src.String()), 0o600); err != nil {
		b.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		opts options
	}{
		{"exact-10000", options{threshold: 1, renameCost: .3, minLines: 3}},
		{"filtered-10000", options{threshold: .87, renameCost: .3, minLines: 3, filterFunction: "F00000"}},
	} {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				r := newReport()
				analyze(context.Background(), []string{name}, dir, tt.opts, r)
				if r.Status != "complete" || r.Summary.Functions != 10000 || r.Summary.Types != 100 {
					b.Fatal("incomplete benchmark")
				}
				b.ReportMetric(float64(r.Summary.CandidatePairs), "candidates/op")
				b.ReportMetric(float64(r.Summary.DistanceCalls), "distances/op")
				b.ReportMetric(float64(len(r.Pairs)), "matches/op")
			}
		})
	}
}

func BenchmarkLargeTree(b *testing.B) {
	ts := newTrees()
	children := make([]*tree, 10000)
	for i := range children {
		children[i] = ts.node(fmt.Sprintf("leaf%d", i))
	}
	a := ts.node("root", children...)
	children = append([]*tree{}, children...)
	children[500] = ts.node("changed")
	c := ts.node("root", children...)
	b.ReportAllocs()
	for b.Loop() {
		if got := distance(a, c, .3); got != .3 {
			b.Fatalf("distance = %g", got)
		}
	}
}
