package gosimilarity

import (
	"context"
	"fmt"
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

func distance(ctx context.Context, a, b *tree, rename float64) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if a == b {
		return 0, nil
	}
	// Deleting and reinserting a node costs two and preserves its children.
	rename = min(rename, 2)
	if a.size == b.size {
		// At equal size, a structural change needs at least two unit edits.
		limit := a.size
		// Only convert ratios bounded by the node count.
		if rename > 0 && 2/rename < float64(limit) {
			limit = int(2 / rename)
		}
		changes, err := labelChanges(ctx, a, b, limit)
		if err != nil {
			return 0, err
		}
		if changes >= 0 {
			cost := float64(changes) * rename
			if cost <= 2 {
				return cost, nil
			}
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
		return float64(b.size-1) + cost, nil
	}
	if flatTree(a) && flatTree(b) {
		cost := 0.0
		if a.label != b.label {
			cost = rename
		}
		d, err := labelDistance(ctx, a.children, b.children, rename)
		if err != nil {
			return 0, err
		}
		d += cost
		// If a root maps to the other tree's leaf, no other node can map:
		// ancestor relationships must be preserved in both directions.
		oneMatch := float64(a.size + b.size - 2)
		if d > oneMatch {
			for _, child := range a.children {
				if child.label == b.label {
					return oneMatch, nil
				}
			}
			for _, child := range b.children {
				if child.label == a.label {
					return oneMatch, nil
				}
			}
		}
		return d, nil
	}
	if x := chainNodes(a); x != nil {
		if y := chainNodes(b); y != nil {
			return labelDistance(ctx, x, y, rename)
		}
	}
	return orderedDistance(ctx, a, b, rename)
}

// labelChanges returns -1 for differing shapes or more than limit changes.
func labelChanges(ctx context.Context, a, b *tree, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return -1, err
	}
	if a == b {
		return 0, nil
	}
	if len(a.children) != len(b.children) {
		return -1, nil
	}
	changes := 0
	if a.label != b.label {
		changes++
	}
	if changes > limit {
		return -1, nil
	}
	for i := range a.children {
		n, err := labelChanges(ctx, a.children[i], b.children[i], limit-changes)
		if err != nil || n < 0 {
			return -1, err
		}
		changes += n
	}
	return changes, nil
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

func treeLabelCounts(ctx context.Context, root *tree) (map[string]int, error) {
	counts := make(map[string]int)
	var visit func(*tree) error
	visit = func(t *tree) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		counts[t.label]++
		for _, child := range t.children {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(root); err != nil {
		return nil, err
	}
	return counts, nil
}

// Ignoring tree order and ancestry gives an admissible label-count edit bound.
func labelSimilarityUpperBound(a, b *tree, labelsA, labelsB map[string]int, rename float64) float64 {
	common := 0
	for label, count := range labelsA {
		common += min(count, labelsB[label])
	}
	small, large := min(a.size, b.size), max(a.size, b.size)
	lower := float64(large-small) + min(rename, 2)*float64(small-common)
	// DP additions can round down; leave room for their accumulated rounding.
	margin := 8 * 0x1p-52 * (float64(a.size) + float64(b.size) + 1)
	return min(1, math.Max(0, 1-lower/float64(large))+margin)
}

// Chains and leaf sequences reduce exactly to string edit distance. Equal
// prefixes and suffixes can be removed before allocating the two DP rows.
func labelDistance(ctx context.Context, a, b []*tree, rename float64) (float64, error) {
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
		return float64(len(a)), nil
	}
	if uint64(len(a)) > distanceWorkBudget/uint64(len(b)) {
		return 0, fmt.Errorf("label edit distance for %d x %d nodes exceeds the %d-cell DP work budget",
			len(a), len(b), distanceWorkBudget)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	prev, next := make([]float64, len(b)+1), make([]float64, len(b)+1)
	for j := range prev {
		prev[j] = float64(j)
	}
	for i, x := range a {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
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
	return prev[len(b)], nil
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
