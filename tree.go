package gosimilarity

import (
	"hash/maphash"
	"math"
)

type tree struct {
	label    string
	children []*tree
	size     int
	id       int
}

// Interning verifies labels and children, so pointer equality is exact equality,
// not merely a matching fingerprint.
type trees struct {
	seed    maphash.Seed
	buckets map[uint64][]*tree
	nextID  int
}

func newTrees() *trees {
	return &trees{seed: maphash.MakeSeed(), buckets: make(map[uint64][]*tree)}
}

func (ts *trees) node(label string, children ...*tree) *tree {
	var h maphash.Hash
	h.SetSeed(ts.seed)
	h.WriteString(label)
	size := 1
	for _, c := range children {
		size += c.size
		id := uint64(c.id)
		var buf [8]byte
		for i := range buf {
			buf[i] = byte(id >> (8 * i))
		}
		h.Write(buf[:])
	}
	key := h.Sum64()
	for _, n := range ts.buckets[key] {
		if n.label != label || len(n.children) != len(children) {
			continue
		}
		equal := true
		for i, c := range children {
			if c != n.children[i] {
				equal = false
				break
			}
		}
		if equal {
			return n
		}
	}
	ts.nextID++
	n := &tree{label: label, children: children, size: size, id: ts.nextID}
	ts.buckets[key] = append(ts.buckets[key], n)
	return n
}

type treePair struct{ a, b *tree }

func distance(a, b *tree, rename float64) float64 {
	memo := make(map[treePair]float64)
	var visit func(*tree, *tree) float64
	visit = func(a, b *tree) float64 {
		if a == b {
			return 0
		}
		cost := 0.0
		if a.label != b.label {
			cost = rename
		}
		// With an empty child sequence, no alignment or memo table is needed.
		if len(a.children) == 0 || len(b.children) == 0 {
			return math.Min(float64(a.size+b.size), cost+float64(a.size+b.size-2))
		}
		key := treePair{a, b}
		if v, ok := memo[key]; ok {
			return v
		}
		// Only two rows are needed for ordered child-sequence alignment.
		prev := make([]float64, len(b.children)+1)
		next := make([]float64, len(prev))
		for j, c := range b.children {
			prev[j+1] = prev[j] + float64(c.size)
		}
		for _, x := range a.children {
			next[0] = prev[0] + float64(x.size)
			for j, y := range b.children {
				next[j+1] = math.Min(prev[j+1]+float64(x.size),
					math.Min(next[j]+float64(y.size), prev[j]+visit(x, y)))
			}
			prev, next = next, prev
		}
		v := math.Min(float64(a.size+b.size), cost+prev[len(b.children)])
		memo[key] = v
		return v
	}
	return visit(a, b)
}

func similarity(a, b *tree, d float64) float64 {
	if equalTrees(a, b) {
		return 1
	}
	return math.Min(math.Nextafter(1, 0), math.Max(0, 1-d/float64(max(a.size, b.size))))
}

func equalTrees(a, b *tree) bool {
	if a == b {
		return true
	}
	if a.label != b.label || len(a.children) != len(b.children) {
		return false
	}
	for i := range a.children {
		if !equalTrees(a.children[i], b.children[i]) {
			return false
		}
	}
	return true
}

func similarityUpperBound(a, b int) float64 {
	return math.Max(0, 1-math.Abs(float64(a)-float64(b))/float64(max(a, b)))
}
