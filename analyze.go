package gosimilarity

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"math"
	"os"
	"reflect"
	"slices"
	"strings"
)

func analyze(ctx context.Context, paths []string, cwd string, opts options, r *report) {
	ts := newTrees()
	for _, input := range discover(ctx, paths, cwd, opts, r) {
		if ctx.Err() != nil {
			r.diagnose("error", "parse", "", "", ctx.Err().Error())
			break
		}
		r.Summary.Files++
		name := displayPath(cwd, input.path)
		src, err := os.ReadFile(input.path)
		if err != nil {
			r.diagnose("error", "read", name, "", err.Error())
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, src, parser.ParseComments|parser.SkipObjectResolution|parser.AllErrors)
		if !input.direct && file != nil && ast.IsGenerated(file) {
			r.skip("generated", name, "generated Go source")
			continue
		}
		if err != nil {
			r.diagnose("error", "parse", name, "", err.Error())
			continue
		}
		r.Summary.ParsedFiles++
		b := resolveBindings(file)
		loc := func(pos token.Pos) position {
			p := fset.PositionFor(pos, false)
			return position{Line: p.Line, Column: p.Column, Offset: p.Offset}
		}
		text := func(node ast.Node) string {
			return string(src[loc(node.Pos()).Offset:loc(node.End()).Offset])
		}
		add := func(node ast.Node, d declaration, n *normalizer) {
			d.File, d.Start, d.End = name, loc(node.Pos()), loc(node.End()-1)
			d.End.Column++
			d.End.Offset++
			var normalized *tree
			var err error
			switch v := node.(type) {
			case *ast.FuncDecl:
				if v.Recv != nil && len(v.Recv.List) == 0 {
					err = fmt.Errorf("method receiver list is empty")
					break
				}
				normalized, err = n.node(v.Body)
				d.body = text(v.Body)
				d.Signature = strings.TrimSpace(string(src[loc(v.Pos()).Offset:loc(v.Body.Pos()).Offset]))
				if v.Recv != nil {
					var out strings.Builder
					if e := format.Node(&out, fset, v.Recv.List[0].Type); e == nil {
						d.Receiver = out.String()
					}
				}
			case *ast.TypeSpec:
				normalized, err = n.node(v)
			}
			if err != nil {
				r.diagnose("error", "normalize", name, d.Name, err.Error())
				return
			}
			d.tree, d.Nodes = normalized, normalized.size
			if d.Kind == "function" {
				if (opts.minTokensSet && d.Nodes < opts.minTokens) ||
					(!opts.minTokensSet && d.End.Line-d.Start.Line+1 < opts.minLines) {
					r.Summary.FilteredFunctions++
					return
				}
				r.Summary.Functions++
			} else {
				r.Summary.Types++
			}
			if opts.print {
				d.Source = text(node)
			}
			d.ID = fmt.Sprintf("d%d", len(r.Declarations)+1)
			r.Declarations = append(r.Declarations, d)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			n := &normalizer{trees: ts, bindings: b, ids: make(map[*binding]int), source: src, file: fset.File(file.Pos())}
			switch v := node.(type) {
			case *ast.FuncDecl:
				if !opts.noFunctions && v.Body != nil {
					add(v, declaration{Kind: "function", Name: v.Name.Name}, n)
				}
			case *ast.TypeSpec:
				if !opts.noTypes {
					add(v, declaration{Kind: "type", Name: v.Name.Name, TypeKind: typeKind(v.Type), Alias: v.Assign.IsValid()}, n)
				}
			}
			return true
		})
	}
	compare(ctx, opts, r)
}

func typeKind(expr ast.Expr) string {
	expr = unparen(expr)
	if array, ok := expr.(*ast.ArrayType); ok {
		if array.Len == nil {
			return "slice"
		}
		return "array"
	}
	return strings.TrimSuffix(reflect.TypeOf(expr).Elem().Name(), "Type")
}

func compare(ctx context.Context, opts options, r *report) {
	var err error
	if opts.filterFunction != "" || opts.filterBody != "" {
		err = r.compareExhaustive(ctx, opts)
	} else {
		err = r.compareCandidates(ctx, opts)
	}
	if err != nil {
		r.diagnose("error", "compare", "", "", err.Error())
	}
	r.finishComparison()
}

func (r *report) compareExhaustive(ctx context.Context, opts options) error {
	for i := range r.Declarations {
		a := &r.Declarations[i]
		if ctx.Err() != nil {
			return ctx.Err()
		}
		for j := i + 1; j < len(r.Declarations); j++ {
			if j%1024 == 0 && ctx.Err() != nil {
				return ctx.Err()
			}
			b := &r.Declarations[j]
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
				(a.tree != b.tree && opts.threshold == 1) {
				r.Summary.PrunedPairs++
				continue
			}
			if err := r.comparePair(ctx, i, j, &opts); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *report) comparePair(ctx context.Context, i, j int, opts *options) error {
	a, b := &r.Declarations[i], &r.Declarations[j]
	d := 0.0
	if a.tree != b.tree {
		if opts.threshold > 0 && opts.renameCost > 0 {
			for _, decl := range []*declaration{a, b} {
				if decl.labels == nil {
					var err error
					decl.labels, err = treeLabelCounts(ctx, decl.tree)
					if err != nil {
						return err
					}
				}
			}
			upper := labelSimilarityUpperBound(a.tree, b.tree, a.labels, b.labels, opts.renameCost)
			if err := ctx.Err(); err != nil {
				return err
			}
			if upper < opts.threshold {
				r.Summary.PrunedPairs++
				return nil
			}
		}
		r.Summary.DistanceCalls++
		var err error
		d, err = distance(ctx, a.tree, b.tree, opts.renameCost)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			r.diagnose("error", "compare", a.File, a.Name,
				fmt.Sprintf("comparison with %s (%s): %v", b.Name, b.File, err))
			return nil
		}
	}
	sim := similarity(a.tree, b.tree, d)
	if sim < opts.threshold {
		return nil
	}
	score := sim * float64(a.End.Line-a.Start.Line+b.End.Line-b.Start.Line+2) / 2
	r.Pairs = append(r.Pairs, pair{
		ID: fmt.Sprintf("p%d-%d", i+1, j+1), A: a.ID, B: b.ID,
		Similarity: sim, Score: score, a: i, b: j,
	})
	return nil
}

func (r *report) finishComparison() {
	slices.SortFunc(r.Pairs, func(a, b pair) int {
		if a.Score != b.Score {
			if a.Score > b.Score {
				return -1
			}
			return 1
		}
		if a.a != b.a {
			return a.a - b.a
		}
		return a.b - b.b
	})
	r.Summary.MatchedPairs = len(r.Pairs)
	r.groupPairs()
	// Release comparison-only data before serialization.
	for i := range r.Declarations {
		r.Declarations[i].tree = nil
		r.Declarations[i].body = ""
		r.Declarations[i].labels = nil
	}
	slices.SortFunc(r.Diagnostics, func(a, b diagnostic) int {
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		if c := strings.Compare(a.Declaration, b.Declaration); c != 0 {
			return c
		}
		if c := strings.Compare(a.Stage, b.Stage); c != 0 {
			return c
		}
		return strings.Compare(a.Message, b.Message)
	})
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
