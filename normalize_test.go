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
	source := []byte("package example\n" + src)
	file, err := parser.ParseFile(fset, "example.go", source, parser.ParseComments|parser.SkipObjectResolution)
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
		n := &normalizer{trees: ts, bindings: b, ids: make(map[*binding]int), source: source, file: fset.File(file.Pos())}
		var normalized *tree
		var err error
		switch v := node.(type) {
		case *ast.FuncDecl:
			if !types {
				normalized, err = n.node(v.Body)
			}
		case *ast.TypeSpec:
			if types {
				normalized, err = n.node(v)
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

func assertNormalizedEquality(t *testing.T, aSource, bSource string, types, want bool) (*tree, *tree) {
	t.Helper()
	ts := newTrees()
	a := normalizeSource(t, ts, aSource, types)[0]
	b := normalizeSource(t, ts, bSource, types)[0]
	if got := equalTrees(a, b); got != want {
		t.Fatalf("equal = %v, want %v", got, want)
	}
	return a, b
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
		{"global variable names", "var value int; func f() { use(value) }", "var other int; func g() { use(other) }", false},
		{"global constant names", "const value=1; func f() { use(value) }", "const other=1; func g() { use(other) }", false},
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
		{"raw string carriage return", "func f() { use(`a\rb`) }", "func g() { use(`ab`) }", false},
		{"operators", "func f(x int) { use(x+1) }", "func g(y int) { use(y-1) }", false},
		{"assignment token", "func f() { x := 1; use(x) }", "func g() { x = 1; use(x) }", false},
		{"statement order", "func f() { first(); second() }", "func g() { second(); first() }", false},
		{"if scope", "func f(x int) { if x:=1; x>0 { use(x) }; use(x) }", "func g(a int) { if b:=1; b>0 { use(b) }; use(a) }", true},
		{"range scope", "func f(xs []int) { for x,v:=range xs { use(x,v) } }", "func g(ys []int) { for a,b:=range ys { use(a,b) } }", true},
		{"range assignment bindings", "func f(xs []int, k,v int) { for k,v=range xs { use(k,v) }; use(k,v) }", "func g(ys []int, i,x int) { for i,x=range ys { use(i,x) }; use(i,x) }", true},
		{"range assignment reference", "func f(xs []int, k,v int) { for k,v=range xs { use(k,v) }; use(k,v) }", "func g(ys []int, i,x int) { for i,x=range ys { use(i,x) }; use(i,i) }", false},
		{"range declaration shadows outer", "func f(xs []int, k int) { for k:=range xs { use(k) }; use(k) }", "func g(ys []int, outer int) { for inner:=range ys { use(inner) }; use(outer) }", true},
		{"range declaration reference", "func f(xs []int, k int) { for k:=range xs { use(k) }; use(k) }", "func g(ys []int, outer int) { for inner:=range ys { use(outer) }; use(outer) }", false},
		{"loop scope", "func f() { for x:=0;x<3;x++ { use(x) } }", "func g() { for y:=0;y<3;y++ { use(y) } }", true},
		{"switch scope", "func f(x int) { switch y:=x; y { case 1: z:=y; use(z); default: z:=y; use(z) } }", "func g(a int) { switch b:=a; b { case 1: c:=b; use(c); default: d:=b; use(d) } }", true},
		{"type switch cases", "func f(x any) { switch y:=x.(type) { case int: use(y); case string: use(y) } }", "func g(a any) { switch b:=a.(type) { case int: use(b); case string: use(b) } }", true},
		{"type switch binding source", "func f(x any, extra int) { switch v:=x.(type) { case int: use(v); case string: use(v) } }", "func g(x any, extra int) { switch v:=x.(type) { case int: use(extra); case string: use(v) } }", false},
		{"nested type switch source", "func f(x,y any) { switch a:=x.(type) { case int: switch b:=y.(type) { case int: use(a) } } }", "func g(x,y any) { switch a:=x.(type) { case int: switch b:=y.(type) { case int: use(b) } } }", false},
		{"type switch without guard declaration", "func f(x,y any) { use(x,y); switch x.(type) { case int: v:=x; use(v); case string: v:=x; use(v) } }", "func g(a,b any) { use(a,b); switch a.(type) { case int: c:=a; use(c); case string: d:=a; use(d) } }", true},
		{"type switch without guard reference", "func f(x,y any) { use(x,y); switch x.(type) { case int: v:=x; use(v); case string: v:=x; use(v) } }", "func g(a,b any) { use(a,b); switch a.(type) { case int: c:=a; use(c); case string: d:=b; use(d) } }", false},
		{"select scopes", "func f(c chan int) { select { case x:= <-c: use(x); case x:= <-c: use(x) } }", "func g(d chan int) { select { case a:= <-d: use(a); case b:= <-d: use(b) } }", true},
		{"slice keys", "func f() { const k=0; use([]int{k:1}) }", "func g() { const j=0; use([]int{j:1}) }", true},
		{"array keys", "func f() { const k=0; use([2]int{k:1}) }", "func g() { const j=0; use([2]int{j:1}) }", true},
		{"named literal parenthesized key", "func f() { const k=0; use(M{k:1}) }", "func g() { const k=0; use(M{(k):1}) }", true},
		{"variadic call", "func f(x []int) { use(x...) }", "func g(y []int) { use(y) }", false},
		{"child roles", "func f() { for ; ; tick() {} }", "func g() { for tick(); ; {} }", false},
		{"slice slots", "func f(x []int) { use(x[1:]) }", "func g(y []int) { use(y[:1]) }", false},
		{"empty optional body", "func f() { if ready() {} }", "func g() { if ready() {} else {} }", false},
		{"assignment lists", "func f(x,y,z int) { x,y=z }", "func g(x,y,z int) { x=y,z }", false},
		{"comments", "func f() { /* ignored */ work() }", "func g() { work() }", true},
		{"generic receiver", "type Box[T any] struct{}; func (b Box[T]) f(x T) { use(b,x) }", "type Box[U any] struct{}; func (c Box[U]) g(y U) { use(c,y) }", true},
		{"pointer generic receiver parameters", "type Pair[T,U any] struct{}; func (p *Pair[T,U]) f(x T,y U) { use(p,x,y,new(T),new(U),new(T)) }", "type Pair[V,W any] struct{}; func (q *Pair[V,W]) g(a V,b W) { use(q,a,b,new(V),new(W),new(V)) }", true},
		{"pointer generic receiver parameter reference", "type Pair[T,U any] struct{}; func (p *Pair[T,U]) f(x T,y U) { use(p,x,y,new(T),new(U),new(T)) }", "type Pair[V,W any] struct{}; func (q *Pair[V,W]) g(a V,b W) { use(q,a,b,new(V),new(W),new(W)) }", false},
		{"local type shadows outer variable", "func f(T func(int) int) { { type T int; use(T(1)) }; use(T(1)) }", "func g(x func(int) int) { { type T int; use(T(1)) }; use(x(1)) }", true},
		{"local type reference", "func f(T func(int) int) { { type T int; use(T(1)) }; use(T(1)) }", "func g(x func(int) int) { { type T int; use(x(1)) }; use(x(1)) }", false},
		{"label and variable namespaces", "func f(loop int) { loop: for { use(loop); break loop } }", "func g(x int) { loop: for { use(x); break loop } }", true},
		{"label spelling", "func f(loop int) { loop: for { use(loop); break loop } }", "func g(x int) { other: for { use(x); break other } }", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := assertNormalizedEquality(t, tt.a, tt.b, false, tt.equal)
			if d := mustDistance(t, a, b, .3); (d == 0) != tt.equal {
				t.Fatalf("distance = %g, equality = %v", d, tt.equal)
			}
		})
	}
}

func TestTypeNormalization(t *testing.T) {
	tests := []struct {
		a, b  string
		equal bool
	}{
		{"type A struct { X int }", "type B struct { X int }", false},
		{"type Node[T any] struct { Next *Node[T]; Value T }", "type Item[U any] struct { Next *Item[U]; Value U }", false},
		{"type Node[T any] struct { Next *Node[T]; Value T }", "type Node[U any] struct { Next *Node[U]; Value U }", true},
		{"type A struct { X int }", "type A struct { Y int }", false},
		{"type A struct { X int }", "type A struct { X string }", false},
		{"type A struct { X int `json:\"x\"` }", "type A struct { X int `json:\"y\"` }", false},
		{"type A interface { Open() error }", "type A interface { Close() error }", false},
		{"type A External", "type A Other", false},
		{"type A = string", "type A string", false},
		{"type T[T any] struct { X T }", "type T[V any] struct { X V }", true},
		{"type T struct { T T }", "type U struct { U U }", false},
		{"type A chan<- int", "type A <-chan int", false},
		{"type A = func(x int) int", "type A = func(y int) int", true},
		{"type A func(int)", "type A func() int", false},
		{"type A[T any] struct { X T }", "type A[T comparable] struct { X T }", false},
	}
	for _, tt := range tests {
		t.Run(tt.a+"/"+tt.b, func(t *testing.T) {
			assertNormalizedEquality(t, tt.a, tt.b, true, tt.equal)
		})
	}
}

func TestUnsupportedNormalization(t *testing.T) {
	n := &normalizer{trees: newTrees(), bindings: &bindings{}, ids: make(map[*binding]int)}
	if _, err := n.node(&ast.BadExpr{}); err == nil {
		t.Fatal("invalid node normalized silently")
	}
}
