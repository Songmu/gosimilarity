package gosimilarity

import (
	"fmt"
	"io"
	"math"
	"slices"
)

type position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
	Offset int `json:"offset"`
}

type declaration struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	File      string   `json:"file"`
	Start     position `json:"start"`
	End       position `json:"end"`
	Nodes     int      `json:"nodes"`
	Receiver  string   `json:"receiver,omitempty"`
	Signature string   `json:"signature,omitempty"`
	TypeKind  string   `json:"type_kind,omitempty"`
	Alias     bool     `json:"alias,omitempty"`
	Source    string   `json:"source,omitempty"`
	tree      *tree
	body      string
}

type pair struct {
	ID         string  `json:"id"`
	A          string  `json:"a"`
	B          string  `json:"b"`
	Similarity float64 `json:"similarity"`
	Score      float64 `json:"score"`
	a, b       int
}

type group struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Members []string `json:"members"`
	Pairs   []string `json:"pairs"`
}

type diagnostic struct {
	Severity    string `json:"severity"`
	Stage       string `json:"stage"`
	File        string `json:"file,omitempty"`
	Declaration string `json:"declaration,omitempty"`
	Message     string `json:"message"`
}

type summary struct {
	Files              int            `json:"files"`
	ParsedFiles        int            `json:"parsed_files"`
	SkippedFiles       int            `json:"skipped_files"`
	SkippedReasons     map[string]int `json:"skipped_reasons"`
	FilteredFunctions  int            `json:"filtered_functions"`
	Functions          int            `json:"functions"`
	Types              int            `json:"types"`
	CandidatePairs     int            `json:"candidate_pairs"`
	FunctionCandidates int            `json:"function_candidates"`
	TypeCandidates     int            `json:"type_candidates"`
	PrunedPairs        int            `json:"pruned_pairs"`
	DistanceCalls      int            `json:"distance_calls"`
	MatchedPairs       int            `json:"matched_pairs"`
}

type report struct {
	SchemaVersion int           `json:"schema_version"`
	Status        string        `json:"status"`
	Summary       summary       `json:"summary"`
	Declarations  []declaration `json:"declarations"`
	Pairs         []pair        `json:"pairs"`
	Groups        []group       `json:"groups"`
	Diagnostics   []diagnostic  `json:"diagnostics"`
}

func newReport() *report {
	return &report{
		SchemaVersion: 1, Status: "complete",
		Summary:      summary{SkippedReasons: make(map[string]int)},
		Declarations: []declaration{}, Pairs: []pair{}, Groups: []group{}, Diagnostics: []diagnostic{},
	}
}

func (r *report) diagnose(severity, stage, file, decl, message string) {
	r.Diagnostics = append(r.Diagnostics, diagnostic{severity, stage, file, decl, message})
	if severity == "error" {
		r.Status = "incomplete"
	}
}

func (r *report) skip(reason, file, message string) {
	r.Summary.SkippedFiles++
	r.Summary.SkippedReasons[reason]++
	r.diagnose("info", "skip", file, "", message)
}

func (r *report) groupPairs() {
	parent := make([]int, len(r.Declarations))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	for _, p := range r.Pairs {
		a, b := find(p.a), find(p.b)
		if a != b {
			parent[b] = a
		}
	}
	components := make(map[int][]int)
	var roots []int
	for i, p := range r.Pairs {
		root := find(p.a)
		if _, ok := components[root]; !ok {
			roots = append(roots, root)
		}
		components[root] = append(components[root], i)
	}
	// Pairs are already ranked; first appearance ranks each component.
	for _, root := range roots {
		indices := components[root]
		members := make(map[int]bool)
		g := group{ID: fmt.Sprintf("g%d", len(r.Groups)+1)}
		for _, i := range indices {
			p := r.Pairs[i]
			members[p.a], members[p.b] = true, true
			g.Pairs = append(g.Pairs, p.ID)
		}
		if len(members) < 3 {
			continue
		}
		var sorted []int
		for i := range members {
			sorted = append(sorted, i)
		}
		slices.Sort(sorted)
		for _, i := range sorted {
			g.Members = append(g.Members, r.Declarations[i].ID)
		}
		g.Kind = r.Declarations[sorted[0]].Kind
		r.Groups = append(r.Groups, g)
	}
}

func percent(score float64) float64 {
	if score == 1 {
		return 100
	}
	return math.Min(99.99, score*100)
}

func renderText(w io.Writer, r *report) error {
	decls := make(map[string]declaration, len(r.Declarations))
	for _, d := range r.Declarations {
		decls[d.ID] = d
	}
	byPair := make(map[string]group)
	for _, g := range r.Groups {
		for _, id := range g.Pairs {
			byPair[id] = g
		}
	}
	printDecl := func(d declaration) error {
		name := d.Name
		if d.Receiver != "" {
			name = "(" + d.Receiver + ")." + name
		}
		_, err := fmt.Fprintf(w, "  %s:%d-%d %s\n", d.File, d.Start.Line, d.End.Line, name)
		return err
	}
	printPair := func(p pair) error {
		a, b := decls[p.A], decls[p.B]
		if _, err := fmt.Fprintf(w, "Similarity: %.2f%%, Score: %.1f points [%s]\n", percent(p.Similarity), p.Score, a.Kind); err != nil {
			return err
		}
		if err := printDecl(a); err != nil {
			return err
		}
		if err := printDecl(b); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  nodes: %d / %d\n", a.Nodes, b.Nodes); err != nil {
			return err
		}
		return nil
	}
	pairs := make(map[string]pair, len(r.Pairs))
	for _, p := range r.Pairs {
		pairs[p.ID] = p
	}
	printed := make(map[string]bool)
	for _, p := range r.Pairs {
		if g, ok := byPair[p.ID]; ok {
			if printed[g.ID] {
				continue
			}
			printed[g.ID] = true
			if _, err := fmt.Fprintf(w, "Group %s [%s]: %d declarations, %d matching pairs (not necessarily all-to-all)\n", g.ID, g.Kind, len(g.Members), len(g.Pairs)); err != nil {
				return err
			}
			for _, id := range g.Pairs {
				if err := printPair(pairs[id]); err != nil {
					return err
				}
			}
			for _, id := range g.Members {
				if d := decls[id]; d.Source != "" {
					if _, err := fmt.Fprintf(w, "\n%s\n", d.Source); err != nil {
						return err
					}
				}
			}
		} else {
			if err := printPair(p); err != nil {
				return err
			}
			for _, id := range []string{p.A, p.B} {
				if d := decls[id]; d.Source != "" {
					if _, err := fmt.Fprintf(w, "\n%s\n", d.Source); err != nil {
						return err
					}
				}
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "Status: %s; %d files parsed, %d functions, %d types, %d matching pairs, %d files skipped\n",
		r.Status, r.Summary.ParsedFiles, r.Summary.Functions, r.Summary.Types, len(r.Pairs), r.Summary.SkippedFiles)
	return err
}
