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

func distance(a, b *tree, rename float64) float64 {
	if a == b {
		return 0
	}
	// Deleting and reinserting a node costs two and preserves its children.
	rename = min(rename, 2)
	if a.size == b.size {
		// At equal size, a structural change needs at least two unit edits.
		switch labelChanges(a, b) {
		case 0:
			return 0
		case 1:
			return rename
		}
	}
	if len(a.children) == 0 || len(b.children) == 0 {
		if len(a.children) != 0 {
			a, b = b, a
		}
		cost := rename
		if containsLabel(b, a.label) {
			cost = 0
		}
		return float64(b.size-1) + cost
	}
	if flatTree(a) && flatTree(b) {
		cost := 0.0
		if a.label != b.label {
			cost = rename
		}
		d := cost + labelDistance(a.children, b.children, rename)
		// If a root maps to the other tree's leaf, no other node can map:
		// ancestor relationships must be preserved in both directions.
		oneMatch := float64(a.size + b.size - 2)
		if d > oneMatch {
			for _, child := range a.children {
				if child.label == b.label {
					return oneMatch
				}
			}
			for _, child := range b.children {
				if child.label == a.label {
					return oneMatch
				}
			}
		}
		return d
	}
	if x := chainNodes(a); x != nil {
		if y := chainNodes(b); y != nil {
			return labelDistance(x, y, rename)
		}
	}
	return orderedDistance(a, b, rename)
}

// labelChanges stops at two; differing shapes also return two.
func labelChanges(a, b *tree) int {
	if a == b {
		return 0
	}
	if len(a.children) != len(b.children) {
		return 2
	}
	changes := 0
	if a.label != b.label {
		changes++
	}
	for i := range a.children {
		changes += labelChanges(a.children[i], b.children[i])
		if changes > 1 {
			return 2
		}
	}
	return changes
}

func containsLabel(t *tree, label string) bool {
	if t.label == label {
		return true
	}
	for _, child := range t.children {
		if containsLabel(child, label) {
			return true
		}
	}
	return false
}

func flatTree(t *tree) bool {
	for _, child := range t.children {
		if len(child.children) != 0 {
			return false
		}
	}
	return true
}

func chainNodes(root *tree) []*tree {
	for t := root; len(t.children) != 0; t = t.children[0] {
		if len(t.children) != 1 {
			return nil
		}
	}
	nodes := make([]*tree, 0, root.size)
	for t := root; ; t = t.children[0] {
		nodes = append(nodes, t)
		if len(t.children) == 0 {
			return nodes
		}
	}
}

// Chains and leaf sequences reduce exactly to string edit distance. Equal
// prefixes and suffixes can be removed before allocating the two DP rows.
func labelDistance(a, b []*tree, rename float64) float64 {
	for len(a) > 0 && len(b) > 0 && a[0].label == b[0].label {
		a, b = a[1:], b[1:]
	}
	for len(a) > 0 && len(b) > 0 && a[len(a)-1].label == b[len(b)-1].label {
		a, b = a[:len(a)-1], b[:len(b)-1]
	}
	if len(a) < len(b) {
		a, b = b, a
	}
	if len(b) == 0 {
		return float64(len(a))
	}
	prev, next := make([]float64, len(b)+1), make([]float64, len(b)+1)
	for j := range prev {
		prev[j] = float64(j)
	}
	for i, x := range a {
		next[0] = float64(i + 1)
		for j, y := range b {
			cost := 0.0
			if x.label != y.label {
				cost = rename
			}
			next[j+1] = min(prev[j+1]+1, next[j]+1, prev[j]+cost)
		}
		prev, next = next, prev
	}
	return prev[len(b)]
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
