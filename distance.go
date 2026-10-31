package gosimilarity

import (
	"context"
	"fmt"
)

const distanceMemoryBudget = 64 << 20
const distanceWorkBudget = 100_000_000

type editNode struct {
	tree     *tree
	leftmost int
}

func postorderTree(root *tree) ([]editNode, []int) {
	nodes := make([]editNode, 1, root.size+1)
	var visit func(*tree)
	visit = func(t *tree) {
		leftmost := len(nodes)
		for _, child := range t.children {
			visit(child)
		}
		nodes = append(nodes, editNode{tree: t, leftmost: leftmost})
	}
	visit(root)
	last := make([]int, len(nodes))
	for i := 1; i < len(nodes); i++ {
		last[nodes[i].leftmost] = i
	}
	var keyroots []int
	for i := 1; i < len(nodes); i++ {
		if last[nodes[i].leftmost] == i {
			keyroots = append(keyroots, i)
		}
	}
	return nodes, keyroots
}

// orderedDistance is Zhang-Shasha ordered tree edit distance. Postorder
// occurrences, not interned node IDs, retain repeated subtree positions.
// Forest edits delete one node at a time, promoting its children.
func orderedDistance(ctx context.Context, a, b *tree, rename float64) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	const maxCells = distanceMemoryBudget / (2 * 8)
	// Check before indexing or multiplying, including on 32-bit platforms.
	if a.size >= maxCells || b.size >= maxCells || a.size+1 > maxCells/(b.size+1) {
		return 0, fmt.Errorf("ordered tree edit distance for %d x %d nodes exceeds the %d MiB DP memory budget",
			a.size, b.size, distanceMemoryBudget>>20)
	}
	x, rootsX := postorderTree(a)
	y, rootsY := postorderTree(b)
	forestLength := func(nodes []editNode, roots []int) uint64 {
		var total uint64
		for _, root := range roots {
			total += uint64(root - nodes[root].leftmost + 1)
		}
		return total
	}
	workX, workY := forestLength(x, rootsX), forestLength(y, rootsY)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	// The memory guard bounds each sum; divide to avoid overflowing their product.
	if workX > distanceWorkBudget/workY {
		return 0, fmt.Errorf("ordered tree edit distance for %d x %d nodes requires %d x %d cell updates, exceeding the %d-cell DP work budget",
			a.size, b.size, workX, workY, distanceWorkBudget)
	}
	cols := len(y)
	distances := make([]float64, len(x)*cols)
	forest := make([]float64, len(distances))
	for _, i := range rootsX {
		for _, j := range rootsY {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			left, right := x[i].leftmost, y[j].leftmost
			forest[(left-1)*cols+right-1] = 0
			for row := left; row <= i; row++ {
				forest[row*cols+right-1] = float64(row - left + 1)
			}
			for col := right; col <= j; col++ {
				forest[(left-1)*cols+col] = float64(col - right + 1)
			}
			for row := left; row <= i; row++ {
				if err := ctx.Err(); err != nil {
					return 0, err
				}
				for col := right; col <= j; col++ {
					pos := row*cols + col
					cost := min(forest[pos-cols]+1, forest[pos-1]+1)
					if x[row].leftmost == left && y[col].leftmost == right {
						matched := forest[pos-cols-1]
						if x[row].tree.label != y[col].tree.label {
							matched += rename
						}
						cost = min(cost, matched)
						distances[pos] = cost
					} else {
						matched := forest[(x[row].leftmost-1)*cols+y[col].leftmost-1] + distances[pos]
						cost = min(cost, matched)
					}
					forest[pos] = cost
				}
			}
		}
	}
	return distances[len(distances)-1], nil
}
