package gosimilarity

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
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

func TestNestedBlockSimilarity(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "blocks.go", `package example
func first(x int) { prepare(); consume(x); finish() }
func second(y int) { prepare(); { consume(y) }; finish() }
`)
	r, code, diagnostics := runJSON(t, "blocks.go", "--min-lines", "0")
	if code != 0 || len(r.Pairs) != 1 {
		t.Fatalf("nested block was not matched: code=%d pairs=%v diagnostics=%s", code, r.Pairs, diagnostics)
	}
	if got, want := r.Pairs[0].Similarity, 1-1.0/12; math.Abs(got-want) > 1e-10 {
		t.Fatalf("nested block similarity = %g, want %g", got, want)
	}
}

func TestComparisonBudgets(t *testing.T) {
	for _, tt := range []struct {
		name, message string
		calls         int
		nested        bool
	}{
		{"memory", "64 MiB DP memory budget", 700, false},
		{"work", "100000000-cell DP work budget", 400, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			var src strings.Builder
			src.WriteString("package example\n")
			for _, name := range []string{"largeA", "largeB"} {
				fmt.Fprintf(&src, "func %s() {\n", name)
				for i := range tt.calls {
					if tt.nested {
						src.WriteString("{\n")
					}
					call := "work"
					if name == "largeB" && i < 2 {
						call = "other"
					}
					fmt.Fprintf(&src, "%s()\n", call)
				}
				if tt.nested {
					src.WriteString(strings.Repeat("}\n", tt.calls))
				}
				src.WriteString("}\n")
			}
			src.WriteString("func smallA() { work() }\nfunc smallB() { work() }\n")
			writeFile(t, "large.go", src.String())
			r, code, diagnostics := runJSON(t, "large.go", "--min-lines", "0")
			if code != 2 || r.Status != "incomplete" || len(r.Diagnostics) != 1 {
				t.Fatalf("budget failure not reported: code=%d status=%s diagnostics=%s", code, r.Status, diagnostics)
			}
			d := r.Diagnostics[0]
			if d.Stage != "compare" || d.File != "large.go" || d.Declaration != "largeA" ||
				!strings.Contains(d.Message, "largeB") || !strings.Contains(d.Message, tt.message) {
				t.Fatalf("budget diagnostic lacks comparison context: %+v", d)
			}
			if len(r.Pairs) != 1 || r.Pairs[0].A != "d3" || r.Pairs[0].B != "d4" ||
				r.Pairs[0].Similarity != 1 {
				t.Fatalf("budget failure lost unrelated matches or reported an approximate match: %+v", r.Pairs)
			}
			writeFile(t, "large.go", strings.Replace(src.String(), "other()", "work()", 1))
			r, code, diagnostics = runJSON(t, "large.go", "--min-lines", "0")
			if code != 0 || r.Status != "complete" || len(r.Pairs) != 2 {
				t.Fatalf("budget rejected an exact single-label shortcut: code=%d status=%s pairs=%v diagnostics=%s",
					code, r.Status, r.Pairs, diagnostics)
			}
		})
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

func TestCompareLabelPruning(t *testing.T) {
	for _, calls := range []int{2, 700} {
		t.Run(fmt.Sprint(calls), func(t *testing.T) {
			t.Chdir(t.TempDir())
			src := "package example\nfunc calls() {\n" + strings.Repeat("work()\n", calls) +
				"}\nfunc increments() {\n" + strings.Repeat("x++\n", calls*3/2) + "}\n"
			writeFile(t, "different.go", src)
			r, code, diagnostics := runJSON(t, "different.go", "--min-lines", "0")
			if code != 0 || r.Status != "complete" || len(r.Declarations) != 2 ||
				r.Declarations[0].Nodes != r.Declarations[1].Nodes || len(r.Pairs) != 0 ||
				r.Summary.CandidatePairs != 1 || r.Summary.PrunedPairs != 1 || r.Summary.DistanceCalls != 0 {
				t.Fatalf("provably dissimilar pair entered DP: code=%d report=%+v diagnostics=%s", code, r, diagnostics)
			}
		})
	}
}

func TestCompareLabelBoundRounding(t *testing.T) {
	ts := newTrees()
	a, b := make([]*tree, 500), make([]*tree, 500)
	for i := range a {
		a[i], b[i] = ts.node("x"), ts.node("y")
	}
	x, y := ts.node("root", a...), ts.node("root", b...)
	threshold := similarity(x, y, mustDistance(t, x, y, .3))
	r := newReport()
	r.Declarations = []declaration{
		{ID: "d1", Kind: "function", tree: x, Nodes: x.size},
		{ID: "d2", Kind: "function", tree: y, Nodes: y.size},
	}
	compare(context.Background(), options{threshold: threshold, renameCost: .3}, r)
	if r.Status != "complete" || len(r.Pairs) != 1 || r.Pairs[0].Similarity != threshold {
		t.Fatalf("label bound pruned a threshold-boundary match: %+v", r)
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
	for _, opts := range []options{{}, {threshold: 1}, {filterFunction: "F"}} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r := newReport()
		r.Declarations = []declaration{{ID: "d1", Kind: "function"}, {ID: "d2", Kind: "function"}}
		compare(ctx, opts, r)
		if r.Status != "incomplete" || r.Summary.CandidatePairs != 0 || len(r.Pairs) != 0 ||
			len(r.Diagnostics) != 1 || r.Diagnostics[0].Stage != "compare" {
			t.Fatalf("canceled comparison (%+v): %+v", opts, r)
		}
	}
}

// Keep eligibility, pruning and accounting exhaustive so this also detects
// candidates lost by an index, not just differences in the reported matches.
func exhaustiveComparison(opts options, declarations []declaration) *report {
	r := newReport()
	r.Declarations = slices.Clone(declarations)
	for i, a := range declarations {
		for j := i + 1; j < len(declarations); j++ {
			b := declarations[j]
			if a.Kind != b.Kind || (a.Kind == "type" && a.TypeKind != b.TypeKind) ||
				(opts.crossFileOnly && a.File == b.File) {
				continue
			}
			if a.Kind == "function" &&
				((!strings.Contains(a.Name, opts.filterFunction) && !strings.Contains(b.Name, opts.filterFunction)) ||
					(!strings.Contains(a.body, opts.filterBody) && !strings.Contains(b.body, opts.filterBody))) {
				continue
			}
			r.Summary.CandidatePairs++
			if a.Kind == "function" {
				r.Summary.FunctionCandidates++
			} else {
				r.Summary.TypeCandidates++
			}
			if similarityUpperBound(a.Nodes, b.Nodes) < opts.threshold ||
				(opts.threshold == 1 && a.tree != b.tree) {
				r.Summary.PrunedPairs++
				continue
			}
			r.comparePair(context.Background(), i, j, &opts)
		}
	}
	r.finishComparison()
	return r
}

func TestEligiblePairCount(t *testing.T) {
	const maxDeclarations = 50001
	r := newReport()
	r.Declarations = make([]declaration, maxDeclarations)
	indices := make([]int, maxDeclarations)
	for i := range indices {
		indices[i] = i
		r.Declarations[i].File = []string{"a.go", "b.go"}[i%2]
	}
	for _, tt := range []struct {
		n, all, crossFile int
	}{
		{0, 0, 0},
		{1, 0, 0},
		{2, 1, 1},
		// The final counts fit int32, but n*(n-1) does not.
		{50000, 1249975000, 625000000},
		{50001, 1250025000, 625025000},
	} {
		t.Run(fmt.Sprint(tt.n), func(t *testing.T) {
			if got := r.eligiblePairCount(indices[:tt.n], false); got != tt.all {
				t.Fatalf("all pairs = %d, want %d", got, tt.all)
			}
			if got := r.eligiblePairCount(indices[:tt.n], true); got != tt.crossFile {
				t.Fatalf("cross-file pairs = %d, want %d", got, tt.crossFile)
			}
		})
	}
}

func TestCompareCandidateEquivalence(t *testing.T) {
	ts := newTrees()
	leaf := ts.node("leaf")
	treeOfSize := func(size int, label string) *tree {
		children := make([]*tree, size-1)
		for i := range children {
			children[i] = leaf
		}
		return ts.node(label, children...)
	}
	var declarations []declaration
	// Deliberately interleave kinds, sizes, files and equal trees. Size 9/10
	// straddles .9; size 1/10 exposes rounding in the size-ratio formula.
	for i := range 36 {
		size := []int{10, 1, 9, 2, 20, 10}[i%6]
		tr := treeOfSize(size, fmt.Sprintf("root%d", i%3))
		d := declaration{
			ID: fmt.Sprintf("d%d", i+1), Kind: "function",
			Name:  []string{"Target", "Other", "TargetOther"}[i%3],
			body:  []string{"plain", "needle", "plain"}[i%3],
			File:  []string{"c.go", "a.go", "b.go", "a.go", "c.go"}[i%5],
			Start: position{Line: 1}, End: position{Line: i%7 + 2},
			tree: tr, Nodes: tr.size,
		}
		if i%4 == 0 {
			d.Kind = "type"
			d.TypeKind = []string{"struct", "slice", "array"}[(i/4)%3]
		}
		declarations = append(declarations, d)
	}
	check := func(t *testing.T, opts options) {
		t.Helper()
		want := exhaustiveComparison(opts, declarations)
		got := newReport()
		got.Declarations = slices.Clone(declarations)
		compare(context.Background(), opts, got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("comparison differs from exhaustive loop:\ngot summary: %+v\nwant summary: %+v\ngot pairs: %+v\nwant pairs: %+v",
				got.Summary, want.Summary, got.Pairs, want.Pairs)
		}
	}
	for _, threshold := range []float64{
		0, similarityUpperBound(1, 10), math.Nextafter(.1, 0), .1, .87, math.Nextafter(.9, 0),
		.9, math.Nextafter(.9, 1), math.Nextafter(1, 0), 1,
	} {
		for _, crossFile := range []bool{false, true} {
			t.Run(fmt.Sprintf("%g/cross=%t", threshold, crossFile), func(t *testing.T) {
				check(t, options{threshold: threshold, crossFileOnly: crossFile, renameCost: .3})
			})
		}
	}
	for _, tt := range []struct {
		name string
		opts options
	}{
		{"name-filter", options{threshold: .87, renameCost: .3, filterFunction: "Target"}},
		{"body-filter-cross-file", options{threshold: .9, renameCost: .3, crossFileOnly: true, filterBody: "needle"}},
		{"both-filters-exact", options{threshold: 1, renameCost: .3, filterFunction: "Target", filterBody: "needle"}},
		{"missing-name-cross-file", options{threshold: .87, renameCost: .3, crossFileOnly: true, filterFunction: "missing", filterBody: "needle"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			check(t, tt.opts)
		})
	}
}

type cancelComparisonContext struct {
	context.Context
	cancel func()
	checks int
}

func (c *cancelComparisonContext) Err() error {
	c.checks--
	if c.checks == 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestCompareCanceledDuringDistance(t *testing.T) {
	ts := newTrees()
	x, y := ts.node("x"), ts.node("y")
	flat := func(leaf *tree) *tree {
		children := make([]*tree, 32)
		for i := range children {
			children[i] = leaf
		}
		return ts.node("root", children...)
	}
	chain := func(leaf *tree) *tree {
		root := leaf
		for range 32 {
			root = ts.node(leaf.label, root)
		}
		return root
	}
	general := func(leaf *tree) *tree {
		branch := ts.node("branch", leaf, leaf)
		return ts.node("root", branch, branch)
	}
	for _, tt := range []struct {
		name string
		a, b *tree
		opts options
	}{
		{"flat", flat(x), flat(y), options{}},
		{"chain", chain(x), chain(y), options{}},
		{"general-filtered", general(x), general(y), options{filterFunction: "F"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			counted := &cancelComparisonContext{Context: ctx, cancel: cancel, checks: 12}
			r := newReport()
			r.Declarations = []declaration{
				{ID: "d1", Kind: "function", Name: "F1", tree: tt.a, Nodes: tt.a.size},
				{ID: "d2", Kind: "function", Name: "F2", tree: tt.b, Nodes: tt.b.size},
			}
			compare(counted, tt.opts, r)
			if r.Status != "incomplete" || len(r.Pairs) != 0 || len(r.Diagnostics) != 1 ||
				r.Diagnostics[0].Message != context.Canceled.Error() || r.Summary.DistanceCalls != 1 {
				t.Fatalf("distance cancellation not propagated once: %+v", r)
			}
		})
	}
}

func TestCompareCanceledDuringEnumeration(t *testing.T) {
	const n = 120
	for _, threshold := range []float64{.9, 1} {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		checks := n + 3
		if threshold == 1 {
			checks += n // Exact matching also builds the interned-tree groups.
		}
		counted := &cancelComparisonContext{Context: ctx, cancel: cancel, checks: checks}
		ts := newTrees()
		tr := ts.node("same")
		r := newReport()
		for i := range n {
			r.Declarations = append(r.Declarations, declaration{
				ID: fmt.Sprintf("d%d", i+1), Kind: "function", tree: tr, Nodes: tr.size,
			})
		}
		compare(counted, options{threshold: threshold}, r)
		if r.Status != "incomplete" || len(r.Diagnostics) != 1 || r.Diagnostics[0].Stage != "compare" ||
			len(r.Pairs) == 0 || len(r.Pairs) >= n*(n-1)/2 {
			t.Fatalf("comparison did not stop during enumeration (threshold %g): %+v", threshold, r.Summary)
		}
		if r.Summary.CandidatePairs != len(r.Pairs) || r.Summary.PrunedPairs != 0 || r.Summary.DistanceCalls != 0 {
			t.Fatalf("cancellation changed counter meanings: %+v", r.Summary)
		}
	}
}

func TestCompareFiltersAcrossSides(t *testing.T) {
	ts := newTrees()
	tr := ts.node("same")
	declarations := []declaration{
		{ID: "d1", Kind: "function", Name: "Target", body: "plain", File: "a.go", tree: tr, Nodes: tr.size},
		{ID: "d2", Kind: "function", Name: "Other", body: "needle", File: "b.go", tree: tr, Nodes: tr.size},
		{ID: "d3", Kind: "function", Name: "Other", body: "plain", File: "c.go", tree: tr, Nodes: tr.size},
	}
	for _, threshold := range []float64{0, .9, 1} {
		r := newReport()
		r.Declarations = slices.Clone(declarations)
		compare(context.Background(), options{
			threshold: threshold, crossFileOnly: true, filterFunction: "Target", filterBody: "needle",
		}, r)
		if r.Summary.CandidatePairs != 1 || len(r.Pairs) != 1 || r.Pairs[0].ID != "p1-2" {
			t.Fatalf("filters must be allowed on opposite sides: %+v", r)
		}
	}
}
