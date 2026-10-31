package gosimilarity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

func BenchmarkComparisonCandidates(b *testing.B) {
	ts := newTrees()
	small := ts.node("small")
	large := ts.node("large", small, small, small, small, small, small, small, small, small)
	var declarations []declaration
	for i := range 10000 {
		tr, file := small, "small.go"
		if i%2 == 0 {
			tr, file = large, "large.go"
		}
		declarations = append(declarations, declaration{
			ID: fmt.Sprintf("d%d", i+1), Kind: "function", File: file, tree: tr, Nodes: tr.size,
		})
	}
	for _, threshold := range []float64{.9, 1} {
		for _, exhaustive := range []bool{true, false} {
			b.Run(fmt.Sprintf("cross-file-%g-10000/exhaustive=%t", threshold, exhaustive), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					r := newReport()
					r.Declarations = slices.Clone(declarations)
					opts := options{threshold: threshold, crossFileOnly: true}
					if exhaustive {
						if err := r.compareExhaustive(context.Background(), opts); err != nil {
							b.Fatal(err)
						}
						r.finishComparison()
					} else {
						compare(context.Background(), opts, r)
					}
					if r.Summary.CandidatePairs != 25000000 || r.Summary.PrunedPairs != 25000000 ||
						r.Summary.DistanceCalls != 0 || len(r.Pairs) != 0 {
						b.Fatalf("unexpected benchmark comparison: %+v", r.Summary)
					}
					b.ReportMetric(float64(r.Summary.CandidatePairs), "candidates/op")
					b.ReportMetric(float64(r.Summary.DistanceCalls), "distances/op")
				}
			})
		}
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
		got, err := distance(a, c, .3)
		if err != nil {
			b.Fatal(err)
		}
		if got != .3 {
			b.Fatalf("distance = %g", got)
		}
	}
}
