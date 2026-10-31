package gosimilarity

import (
	"math"
	"testing"
)

func TestDistance(t *testing.T) {
	ts := newTrees()
	x, y := ts.node("x"), ts.node("y")
	root := ts.node("root", x)
	insert := ts.node("root", x, y)
	rename := ts.node("root", y)
	big := ts.node("root", insert, insert, insert)
	for _, tt := range []struct {
		a, b *tree
		cost float64
		want float64
	}{
		{root, root, .3, 0},
		{root, insert, .3, 1},
		{root, rename, .3, .3},
		{root, rename, .8, .8},
		{x, y, 10, 2},
		{x, big, .3, float64(big.size - 1)},
	} {
		for _, order := range [][2]*tree{{tt.a, tt.b}, {tt.b, tt.a}} {
			got := distance(order[0], order[1], tt.cost)
			if math.Abs(got-tt.want) > 1e-10 {
				t.Errorf("distance = %g, want %g", got, tt.want)
			}
			if got < math.Abs(float64(tt.a.size-tt.b.size)) {
				t.Error("distance below size lower bound")
			}
		}
	}
	if ts.node("root", x) != root {
		t.Fatal("equal interned trees differ")
	}
	other := newTrees()
	copy := other.node("root", other.node("x"))
	if !equalTrees(root, copy) || similarity(root, copy, distance(root, copy, .3)) != 1 {
		t.Fatal("interned and ordinary equality paths disagree")
	}
}

func TestSimilarityBoundaries(t *testing.T) {
	ts := newTrees()
	a, b := ts.node("a"), ts.node("b")
	if got := similarity(a, b, math.SmallestNonzeroFloat64); got >= 1 {
		t.Fatal("rounded nonzero distance accepted as exact")
	}
	if got := similarityUpperBound(9, 10); got != .9 {
		t.Fatalf("unsafe pruning at threshold .9: %g", got)
	}
	if got := percent(.99997); got != 99.99 {
		t.Fatalf("non-exact display = %g", got)
	}
	if percent(1) != 100 || similarity(a, a, 0) != 1 {
		t.Fatal("exact equality not preserved")
	}
}

func FuzzDistance(f *testing.F) {
	f.Add([]byte{1, 2, 3}, []byte{2, 3})
	f.Fuzz(func(t *testing.T, a, b []byte) {
		if len(a) > 20 || len(b) > 20 {
			t.Skip()
		}
		ts := newTrees()
		makeTree := func(data []byte) *tree {
			var children []*tree
			for _, v := range data {
				children = append(children, ts.node(string([]byte{v})))
			}
			return ts.node("root", children...)
		}
		x, y := makeTree(a), makeTree(b)
		d := distance(x, y, .3)
		if math.Abs(d-distance(y, x, .3)) > 1e-10 ||
			d < math.Abs(float64(x.size-y.size)) ||
			(d == 0) != equalTrees(x, y) {
			t.Fatal("distance invariant violated")
		}
	})
}
