package gosimilarity

import (
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func normalizeSource(t *testing.T, ts *trees, src string, types bool) []*tree {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "example.go", "package example\n"+src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var before strings.Builder
	if err := format.Node(&before, fset, file); err != nil {
		t.Fatal(err)
	}
	b := resolveBindings(file)
	var result []*tree
	ast.Inspect(file, func(node ast.Node) bool {
		n := &normalizer{trees: ts, bindings: b, ids: make(map[*binding]int)}
		var normalized *tree
		var err error
		switch v := node.(type) {
		case *ast.FuncDecl:
			if !types {
				normalized, err = n.node(v.Body)
			}
		case *ast.TypeSpec:
			if types {
				n.self = b.idents[v.Name]
				p, e := n.node(v.TypeParams)
				if e != nil {
					t.Fatal(e)
				}
				rhs, e := n.node(v.Type)
				err = e
				if e == nil {
					if !v.Assign.IsValid() {
						normalized = ts.node("definition", p, rhs)
					} else {
						normalized = ts.node("alias", p, rhs)
					}
				}
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		if normalized != nil {
			result = append(result, normalized)
		}
		return true
	})
	var after strings.Builder
	if err := format.Node(&after, fset, file); err != nil || after.String() != before.String() {
		t.Fatal("normalization modified the AST", err)
	}
	return result
}

func TestNormalization(t *testing.T) {
	tests := []struct {
		name, a, b string
		equal      bool
	}{
		{"bindings", "func f(unused, x int) int { y := x + x; return y }", "func g(a int) int { b := (a + a); return b }", true},
		{"callback", "func f(fn func()) { fn() }", "func g(cb func()) { cb() }", true},
		{"distinct references", "func f(x,y int) int { return x+x }", "func g(a,b int) int { return a+b }", false},
		{"identifier namespace", "func f(x int) int { return x }", "func g() int { return p0 }", false},
		{"shadowing", "func f(x int) { { x := 1; use(x) }; use(x) }", "func g(a int) { { b := 1; use(b) }; use(a) }", true},
		{"shadow reference", "func f(x int) { { x := 1; use(x) } }", "func g(a int) { { b := 1; use(a) } }", false},
		{"short rhs", "func f(x int) { { x := x; use(x) } }", "func g(a int) { { b := a; use(b) } }", true},
		{"short existing", "func f(x int) { x,y := x,1; use(x,y) }", "func g(a int) { a,b := a,1; use(a,b) }", true},
		{"closure", "func f(x int) { use(func(y int) { use(x,y) }) }", "func g(a int) { use(func(b int) { use(a,b) }) }", true},
		{"closure captures", "func f(x int) { use(func(y int) { use(x) }) }", "func g(a int) { use(func(b int) { use(b) }) }", false},
		{"selectors", "func f(x T) { use(x.Field) }", "func g(y T) { use(y.Other) }", false},
		{"unknown key", "func f(x int) { use(T{x:x}) }", "func g(y int) { use(T{y:y}) }", false},
		{"known map key", "func f(x int) { use(map[int]int{x:x}) }", "func g(y int) { use(map[int]int{y:y}) }", true},
		{"literal spelling", "func f() { use(42) }", "func g() { use(4_2) }", false},
		{"string spelling", "func f() { use(\"a\") }", "func g() { use(`a`) }", false},
		{"operators", "func f(x int) { use(x+1) }", "func g(y int) { use(y-1) }", false},
		{"assignment token", "func f() { x := 1; use(x) }", "func g() { x = 1; use(x) }", false},
		{"statement order", "func f() { first(); second() }", "func g() { second(); first() }", false},
		{"if scope", "func f(x int) { if x:=1; x>0 { use(x) }; use(x) }", "func g(a int) { if b:=1; b>0 { use(b) }; use(a) }", true},
		{"range scope", "func f(xs []int) { for x,v:=range xs { use(x,v) } }", "func g(ys []int) { for a,b:=range ys { use(a,b) } }", true},
		{"loop scope", "func f() { for x:=0;x<3;x++ { use(x) } }", "func g() { for y:=0;y<3;y++ { use(y) } }", true},
		{"switch scope", "func f(x int) { switch y:=x; y { case 1: z:=y; use(z); default: z:=y; use(z) } }", "func g(a int) { switch b:=a; b { case 1: c:=b; use(c); default: d:=b; use(d) } }", true},
		{"type switch cases", "func f(x any) { switch y:=x.(type) { case int: use(y); case string: use(y) } }", "func g(a any) { switch b:=a.(type) { case int: use(b); case string: use(b) } }", true},
		{"select scopes", "func f(c chan int) { select { case x:= <-c: use(x); case x:= <-c: use(x) } }", "func g(d chan int) { select { case a:= <-d: use(a); case b:= <-d: use(b) } }", true},
		{"variadic call", "func f(x []int) { use(x...) }", "func g(y []int) { use(y) }", false},
		{"comments", "func f() { /* ignored */ work() }", "func g() { work() }", true},
		{"generic receiver", "type Box[T any] struct{}; func (b Box[T]) f(x T) { use(b,x) }", "type Box[U any] struct{}; func (c Box[U]) g(y U) { use(c,y) }", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTrees()
			a := normalizeSource(t, ts, tt.a, false)[0]
			b := normalizeSource(t, ts, tt.b, false)[0]
			if got := equalTrees(a, b); got != tt.equal {
				t.Fatalf("equal = %v, want %v", got, tt.equal)
			}
		})
	}
}

func TestTypeNormalization(t *testing.T) {
	tests := []struct {
		a, b  string
		equal bool
	}{
		{"type A struct { X int }", "type B struct { X int }", true},
		{"type Node[T any] struct { Next *Node[T]; Value T }", "type Item[U any] struct { Next *Item[U]; Value U }", true},
		{"type A struct { X int }", "type B struct { Y int }", false},
		{"type A interface { Open() error }", "type B interface { Close() error }", false},
		{"type A External", "type B Other", false},
		{"type A = string", "type B string", false},
		{"type T[T any] struct { X T }", "type U[V any] struct { X V }", true},
		{"type T struct { T T }", "type U struct { U U }", false},
		{"type A chan<- int", "type B <-chan int", false},
		{"type A = func(x int) int", "type B = func(y int) int", true},
	}
	for _, tt := range tests {
		t.Run(tt.a+"/"+tt.b, func(t *testing.T) {
			ts := newTrees()
			a := normalizeSource(t, ts, tt.a, true)[0]
			b := normalizeSource(t, ts, tt.b, true)[0]
			if equalTrees(a, b) != tt.equal {
				t.Fatalf("unexpected equality: want %v", tt.equal)
			}
		})
	}
}

func TestUnsupportedNormalization(t *testing.T) {
	n := &normalizer{trees: newTrees(), bindings: &bindings{}, ids: make(map[*binding]int)}
	if _, err := n.node(&ast.BadExpr{}); err == nil {
		t.Fatal("invalid node normalized silently")
	}
}
