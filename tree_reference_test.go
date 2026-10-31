package gosimilarity

import (
	"fmt"
	"math"
	"testing"
)

// referenceDistance implements Zhang-Shasha ordered tree edit distance with
// unit insertion/deletion costs. This independent, unoptimized reference
// exercises production fast paths as well as the general forest recurrence.
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
		{"flat root-to-leaf mapping", ts.node("a", b), ts.node("b", a), 3, 2},
		{"repeated interned subtree", ts.node("r", shared, shared), ts.node("r", shared), .3, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, pair := range [][2]*tree{{tt.a, tt.b}, {tt.b, tt.a}} {
				if got := referenceDistance(pair[0], pair[1], tt.rename); math.Abs(got-tt.want) > 1e-10 {
					t.Fatalf("reference distance = %g, want %g", got, tt.want)
				}
				if got := distance(pair[0], pair[1], tt.rename); math.Abs(got-tt.want) > 1e-10 {
					t.Fatalf("production distance = %g, want %g", got, tt.want)
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
	if current != 1 || reference != 1 {
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
			if math.Abs(reference-current) > 1e-10 {
				t.Fatalf("reference TED %g differs from production distance %g", reference, current)
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
			var forest []*tree
			for _, value := range data {
				count := min(int(value>>2)%4, len(forest))
				start := len(forest) - count
				children := append([]*tree(nil), forest[start:]...)
				node := ts.node(string(rune('a'+value%4)), children...)
				forest = append(forest[:start], node)
			}
			return ts.node(string(rune('a'+len(data)%4)), forest...)
		}
		x, y := makeTree(a), makeTree(b)
		for _, rename := range []float64{.3, 1, 3} {
			got := referenceDistance(x, y, rename)
			forward, reverse := distance(x, y, rename), distance(y, x, rename)
			if got < math.Abs(float64(x.size-y.size))-1e-10 ||
				math.Abs(got-referenceDistance(y, x, rename)) > 1e-10 ||
				math.Abs(got-forward) > 1e-10 ||
				math.Abs(got-reverse) > 1e-10 ||
				(got == 0) != equalTrees(x, y) {
				t.Fatalf("TED differs at rename cost %g: reference=%g forward=%g reverse=%g",
					rename, got, forward, reverse)
			}
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
	makeBalanced := func(changes int) *tree {
		var build func(int, int) *tree
		build = func(depth, offset int) *tree {
			if depth == 0 {
				label := fmt.Sprintf("leaf%d", offset)
				if changes > 0 && offset == 32 || changes > 1 && offset == 16 {
					label = "changed"
				}
				return ts.node(label)
			}
			half := 1 << (depth - 1)
			return ts.node("branch", build(depth-1, offset), build(depth-1, offset+half))
		}
		return build(6, 0)
	}
	for _, tt := range []struct {
		name string
		a, b *tree
		want float64
	}{
		{"flat-32", makeFlat(32, false), makeFlat(32, true), .3},
		{"flat-128", makeFlat(128, false), makeFlat(128, true), .3},
		{"chain-128", makeChain(128, false), makeChain(128, true), .3},
		{"balanced-127", makeBalanced(0), makeBalanced(1), .3},
		{"balanced-127-two-renames", makeBalanced(0), makeBalanced(2), .6},
	} {
		for _, algorithm := range []struct {
			name string
			run  func(*tree, *tree, float64) float64
		}{{"current", distance}, {"reference", referenceDistance}} {
			b.Run(tt.name+"/"+algorithm.name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if got := algorithm.run(tt.a, tt.b, .3); math.Abs(got-tt.want) > 1e-10 {
						b.Fatalf("distance = %g, want %g", got, tt.want)
					}
				}
			})
		}
	}
}
