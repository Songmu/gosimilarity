package gosimilarity

import (
	"context"
	"slices"
	"sort"
	"strings"
)

// Filters deliberately retain the exhaustive path: name and body predicates can
// be satisfied by different sides of a pair.
func (r *report) compareCandidates(ctx context.Context, opts options) error {
	type key struct{ kind, typeKind string }
	var buckets [][]int
	bucketIDs := make(map[key]int)
	for i, d := range r.Declarations {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		k := key{kind: d.Kind}
		if d.Kind == "type" {
			k.typeKind = d.TypeKind
		}
		id, ok := bucketIDs[k]
		if !ok {
			id = len(buckets)
			bucketIDs[k] = id
			buckets = append(buckets, nil)
		}
		buckets[id] = append(buckets[id], i)
	}
	for _, indices := range buckets {
		var err error
		if opts.threshold == 1 {
			err = r.compareExactCandidates(ctx, opts, indices)
		} else {
			err = r.compareSizeCandidates(ctx, opts, indices)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *report) addCandidates(kind string, n int) {
	r.Summary.CandidatePairs += n
	if kind == "function" {
		r.Summary.FunctionCandidates += n
	} else {
		r.Summary.TypeCandidates += n
	}
}

func (r *report) eligiblePairCount(indices []int, crossFileOnly bool) int {
	n := len(indices)
	count := n * (n - 1) / 2
	if crossFileOnly {
		files := make(map[string]int)
		for _, i := range indices {
			file := r.Declarations[i].File
			count -= files[file]
			files[file]++
		}
	}
	return count
}

func (r *report) compareExactCandidates(ctx context.Context, opts options, indices []int) error {
	var groups [][]int
	groupIDs := make(map[*tree]int)
	for _, i := range indices {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		t := r.Declarations[i].tree
		id, ok := groupIDs[t]
		if !ok {
			id = len(groups)
			groupIDs[t] = id
			groups = append(groups, nil)
		}
		groups[id] = append(groups[id], i)
	}
	pruned := r.eligiblePairCount(indices, opts.crossFileOnly)
	for _, group := range groups {
		pruned -= r.eligiblePairCount(group, opts.crossFileOnly)
	}
	kind := r.Declarations[indices[0]].Kind
	r.addCandidates(kind, pruned)
	r.Summary.PrunedPairs += pruned
	for _, group := range groups {
		if opts.crossFileOnly {
			slices.SortFunc(group, func(i, j int) int {
				return strings.Compare(r.Declarations[i].File, r.Declarations[j].File)
			})
		}
		fileEnd := 0
		for p, i := range group {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			start := p + 1
			if opts.crossFileOnly {
				if p >= fileEnd {
					fileEnd = p + 1
					for fileEnd < len(group) && r.Declarations[group[fileEnd]].File == r.Declarations[i].File {
						fileEnd++
					}
				}
				start = fileEnd
			}
			for q := start; q < len(group); q++ {
				if q%1024 == 0 && ctx.Err() != nil {
					return ctx.Err()
				}
				a, b := i, group[q]
				if a > b {
					a, b = b, a
				}
				r.addCandidates(kind, 1)
				r.comparePair(a, b, &opts)
			}
		}
	}
	return nil
}

func (r *report) compareSizeCandidates(ctx context.Context, opts options, indices []int) error {
	slices.SortFunc(indices, func(i, j int) int {
		return r.Declarations[i].Nodes - r.Declarations[j].Nodes
	})
	var files map[string][]int
	if opts.crossFileOnly {
		files = make(map[string][]int)
		for p, i := range indices {
			file := r.Declarations[i].File
			files[file] = append(files[file], p)
		}
	}
	for p, i := range indices {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		a := &r.Declarations[i]
		// Search using the original floating-point predicate, not an algebraic
		// size ratio: equivalent formulas can round differently at the threshold.
		end := p + 1 + sort.Search(len(indices)-p-1, func(q int) bool {
			return similarityUpperBound(a.Nodes, r.Declarations[indices[p+1+q]].Nodes) < opts.threshold
		})
		pruned := len(indices) - end
		viable := end - p - 1
		if opts.crossFileOnly {
			positions := files[a.File]
			fileEnd := sort.SearchInts(positions, end)
			pruned -= len(positions) - fileEnd
			viable -= fileEnd - sort.SearchInts(positions, p+1)
		}
		r.addCandidates(a.Kind, pruned)
		r.Summary.PrunedPairs += pruned
		if viable == 0 {
			continue
		}
		for q := p + 1; q < end; q++ {
			if q%1024 == 0 && ctx.Err() != nil {
				return ctx.Err()
			}
			j := indices[q]
			if opts.crossFileOnly && a.File == r.Declarations[j].File {
				continue
			}
			left, right := i, j
			if left > right {
				left, right = right, left
			}
			r.addCandidates(a.Kind, 1)
			r.comparePair(left, right, &opts)
		}
	}
	return nil
}
