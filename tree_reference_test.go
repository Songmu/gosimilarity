package gosimilarity

import (
	"fmt"
	"math"
	"testing"
)

// referenceDistance implements Zhang-Shasha ordered tree edit distance with
// unit insertion/deletion costs. Unlike distance, deleting a node promotes its
// children. This test-only reference deliberately favors clarity over speed.
func referenceDistance(a, b *tree, rename float64) float64 {
	type orderedTree struct {
		nodes    []*tree
		leftmost []int
		keyroots []int
	}
	order := func(root *tree) orderedTree {
		out := orderedTree{nodes: []*tree{nil}, leftmost: []int{0}}
		var visit func(*tree) int
		visit = func(node *tree) int {
			left := len(out.nodes)
			for _, child := range node.children {
				visit(child)
			}
			out.nodes = append(out.nodes, node)
			out.leftmost = append(out.leftmost, left)
			return len(out.nodes) - 1
		}
		visit(root)
		last := make([]int, len(out.nodes))
		for i := 1; i < len(out.nodes); i++ {
			last[out.leftmost[i]] = i
		}
		for i := 1; i < len(out.nodes); i++ {
			if last[out.leftmost[i]] == i {
				out.keyroots = append(out.keyroots, i)
			}
		}
		return out
	}
	x, y := order(a), order(b)
	rows, cols := len(x.nodes), len(y.nodes)
	distances := make([]float64, rows*cols)
	forest := make([]float64, rows*cols)
	for _, i := range x.keyroots {
		for _, j := range y.keyroots {
			left, right := x.leftmost[i], y.leftmost[j]
			at := func(row, col int) int { return row*cols + col }
			forest[at(left-1, right-1)] = 0
			for row := left; row <= i; row++ {
				forest[at(row, right-1)] = float64(row - left + 1)
			}
			for col := right; col <= j; col++ {
				forest[at(left-1, col)] = float64(col - right + 1)
			}
			for row := left; row <= i; row++ {
				for col := right; col <= j; col++ {
					deleted := forest[at(row-1, col)] + 1
					inserted := forest[at(row, col-1)] + 1
					var matched float64
					if x.leftmost[row] == left && y.leftmost[col] == right {
						matched = forest[at(row-1, col-1)]
						if x.nodes[row].label != y.nodes[col].label {
							matched += rename
						}
						distances[at(row, col)] = min(deleted, inserted, matched)
					} else {
						matched = forest[at(x.leftmost[row]-1, y.leftmost[col]-1)] +
							distances[at(row, col)]
					}
					forest[at(row, col)] = min(deleted, inserted, matched)
				}
			}
		}
	}
	return distances[len(distances)-1]
}

func TestReferenceDistance(t *testing.T) {
	ts := newTrees()
	a, b := ts.node("a"), ts.node("b")
	shared := ts.node("w", a)
	for _, tt := range []struct {
		name   string
		a, b   *tree
		rename float64
		want   float64
	}{
		{"identity", ts.node("r", a, b), ts.node("r", a, b), .3, 0},
		{"rename", a, b, .3, .3},
		{"replace expensive label", a, b, 3, 2},
		{"insert internal node", ts.node("r", a), ts.node("r", shared), .3, 1},
		{"delete root", ts.node("r", a), a, .3, 1},
		{"promote siblings", ts.node("r", ts.node("w", a, b)), ts.node("r", a, b), .3, 1},
		{"ordered siblings", ts.node("r", a, b), ts.node("r", b, a), 1, 2},
		{"repeated interned subtree", ts.node("r", shared, shared), ts.node("r", shared), .3, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, pair := range [][2]*tree{{tt.a, tt.b}, {tt.b, tt.a}} {
				if got := referenceDistance(pair[0], pair[1], tt.rename); math.Abs(got-tt.want) > 1e-10 {
					t.Fatalf("reference distance = %g, want %g", got, tt.want)
				}
			}
		})
	}
}

func TestDistanceComparison(t *testing.T) {
	ts := newTrees()
	leaf := ts.node("a")
	a, b := ts.node("r", leaf), ts.node("r", ts.node("w", leaf))
	current, reference := distance(a, b, .3), referenceDistance(a, b, .3)
	if math.Abs(current-1.3) > 1e-10 || reference != 1 {
		t.Fatalf("internal insertion: current = %g, reference = %g", current, reference)
	}
	t.Logf("internal insertion: current distance=%.2f similarity=%.4f; reference distance=%.2f similarity=%.4f",
		current, similarity(a, b, current), reference, similarity(a, b, reference))

	for _, tt := range []struct {
		name, source string
	}{
		{"guard", `func f(x int) { consume(x) }
func g(y int) { if ready() { consume(y) } }`},
		{"nested block", `func f(x int) { prepare(); consume(x); finish() }
func g(y int) { prepare(); { consume(y) }; finish() }`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			nodes := normalizeSource(t, newTrees(), tt.source, false)
			a, b := nodes[0], nodes[1]
			current, reference := distance(a, b, .3), referenceDistance(a, b, .3)
			if reference > current+1e-10 {
				t.Fatalf("standard TED %g exceeds restricted distance %g", reference, current)
			}
			t.Logf("nodes=%d/%d current distance=%.2f similarity=%.4f; reference distance=%.2f similarity=%.4f",
				a.size, b.size, current, similarity(a, b, current), reference, similarity(a, b, reference))
		})
	}
}

func FuzzReferenceDistance(f *testing.F) {
	f.Add([]byte{0, 1, 2}, []byte{1, 2, 0})
	f.Add([]byte{2, 0, 1, 0}, []byte{0, 1})
	f.Fuzz(func(t *testing.T, a, b []byte) {
		if len(a) > 12 || len(b) > 12 {
			t.Skip()
		}
		ts := newTrees()
		makeTree := func(data []byte) *tree {
			root := ts.node("a")
			for _, value := range data {
				label := string(rune('a' + value%3))
				switch value % 3 {
				case 0:
					root = ts.node(label, root)
				case 1:
					root = ts.node(label, root, ts.node("b"))
				case 2:
					root = ts.node(label, ts.node("b"), root)
				}
			}
			return root
		}
		x, y := makeTree(a), makeTree(b)
		got := referenceDistance(x, y, .3)
		if got < math.Abs(float64(x.size-y.size))-1e-10 ||
			math.Abs(got-referenceDistance(y, x, .3)) > 1e-10 ||
			got > distance(x, y, .3)+1e-10 ||
			(got == 0) != equalTrees(x, y) {
			t.Fatalf("reference distance invariant violated: %g", got)
		}
	})
}

func BenchmarkDistanceComparison(b *testing.B) {
	ts := newTrees()
	makeFlat := func(size int, changed bool) *tree {
		children := make([]*tree, size)
		for i := range children {
			label := fmt.Sprintf("leaf%d", i)
			if changed && i == size/2 {
				label = "changed"
			}
			children[i] = ts.node(label)
		}
		return ts.node("root", children...)
	}
	makeChain := func(size int, changed bool) *tree {
		root := ts.node("leaf")
		for i := range size {
			label := fmt.Sprintf("node%d", i)
			if changed && i == size/2 {
				label = "changed"
			}
			root = ts.node(label, root)
		}
		return root
	}
	for _, tt := range []struct {
		name string
		a, b *tree
	}{
		{"flat-32", makeFlat(32, false), makeFlat(32, true)},
		{"flat-128", makeFlat(128, false), makeFlat(128, true)},
		{"chain-128", makeChain(128, false), makeChain(128, true)},
	} {
		for _, algorithm := range []struct {
			name string
			run  func(*tree, *tree, float64) float64
		}{{"current", distance}, {"reference", referenceDistance}} {
			b.Run(tt.name+"/"+algorithm.name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if got := algorithm.run(tt.a, tt.b, .3); math.Abs(got-.3) > 1e-10 {
						b.Fatalf("distance = %g, want .3", got)
					}
				}
			})
		}
	}
}
