package gosimilarity

import (
	"context"
	"reflect"
	"testing"
)

func TestArrayAndSliceKinds(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "types.go", "package example\ntype First [2]int\ntype Second [2]int\ntype Slice []int\n")
	r, code, _ := runJSON(t, "types.go", "--no-functions", "-t", "0")
	var kinds []string
	for _, d := range r.Declarations {
		kinds = append(kinds, d.TypeKind)
	}
	if code != 0 || !reflect.DeepEqual(kinds, []string{"array", "array", "slice"}) {
		t.Fatalf("unexpected type classification: code=%d kinds=%v", code, kinds)
	}
	if r.Summary.TypeCandidates != 1 || len(r.Pairs) != 1 ||
		r.Pairs[0].A != r.Declarations[0].ID || r.Pairs[0].B != r.Declarations[1].ID {
		t.Fatalf("arrays and slices were compared together: %+v", r)
	}
}

func TestCompareExactPruning(t *testing.T) {
	ts := newTrees()
	same, different := ts.node("same"), ts.node("different")
	r := newReport()
	r.Declarations = []declaration{
		{ID: "d1", Kind: "function", tree: same, Nodes: same.size},
		{ID: "d2", Kind: "function", tree: same, Nodes: same.size},
		{ID: "d3", Kind: "function", tree: different, Nodes: different.size},
	}
	compare(context.Background(), options{threshold: 1, renameCost: .3}, r)
	if r.Summary.CandidatePairs != 3 || r.Summary.FunctionCandidates != 3 ||
		r.Summary.TypeCandidates != 0 || r.Summary.PrunedPairs != 2 || r.Summary.DistanceCalls != 0 {
		t.Fatalf("unexpected exact-match accounting: %+v", r.Summary)
	}
	if len(r.Pairs) != 1 || r.Pairs[0].ID != "p1-2" || r.Pairs[0].Similarity != 1 {
		t.Fatalf("unexpected exact matches: %+v", r.Pairs)
	}
}

func TestCompareRanking(t *testing.T) {
	ts := newTrees()
	tree := ts.node("same")
	r := newReport()
	r.Declarations = []declaration{
		{ID: "d1", Kind: "function", Start: position{Line: 1}, End: position{Line: 3}, tree: tree, Nodes: tree.size},
		{ID: "d2", Kind: "function", Start: position{Line: 4}, End: position{Line: 8}, tree: tree, Nodes: tree.size},
		{ID: "d3", Kind: "function", Start: position{Line: 9}, End: position{Line: 13}, tree: tree, Nodes: tree.size},
	}
	compare(context.Background(), options{threshold: 1, renameCost: .3}, r)
	var ids []string
	for _, p := range r.Pairs {
		ids = append(ids, p.ID)
	}
	if want := []string{"p2-3", "p1-2", "p1-3"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("ranked pairs = %v, want %v", ids, want)
	}
	if len(r.Groups) != 1 || !reflect.DeepEqual(r.Groups[0].Pairs, ids) {
		t.Fatalf("group did not retain ranked pair order: %+v", r.Groups)
	}
}

func TestCompareCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := newReport()
	r.Declarations = []declaration{{ID: "d1", Kind: "function"}, {ID: "d2", Kind: "function"}}
	compare(ctx, options{}, r)
	if r.Status != "incomplete" || r.Summary.CandidatePairs != 0 || len(r.Pairs) != 0 ||
		len(r.Diagnostics) != 1 || r.Diagnostics[0].Stage != "compare" {
		t.Fatalf("canceled comparison: %+v", r)
	}
}
