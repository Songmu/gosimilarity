package gosimilarity

import (
	"go/ast"
	"go/token"
)

type binding struct {
	kind    string
	name    string
	guard   *binding
	caseNum int
}

type scope struct {
	parent *scope
	names  map[string]*binding
}

type bindings struct {
	current *scope
	idents  map[*ast.Ident]*binding
}

func resolveBindings(file *ast.File) *bindings {
	b := &bindings{idents: make(map[*ast.Ident]*binding)}
	b.push()
	for _, decl := range file.Decls {
		if g, ok := decl.(*ast.GenDecl); ok {
			for _, spec := range g.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					b.declare(s.Name, "named-type")
				case *ast.ValueSpec:
					for _, id := range s.Names {
						b.declare(id, "global")
					}
				}
			}
		}
	}
	for _, decl := range file.Decls {
		b.walk(decl)
	}
	return b
}

func (b *bindings) push() {
	b.current = &scope{parent: b.current, names: make(map[string]*binding)}
}

func (b *bindings) pop() { b.current = b.current.parent }

func (b *bindings) declare(id *ast.Ident, kind string) {
	if id == nil || id.Name == "_" {
		return
	}
	obj := &binding{kind: kind, name: id.Name}
	b.current.names[id.Name] = obj
	b.idents[id] = obj
}

func (b *bindings) reference(id *ast.Ident) {
	for s := b.current; s != nil; s = s.parent {
		if obj := s.names[id.Name]; obj != nil {
			b.idents[id] = obj
			return
		}
	}
}

func (b *bindings) typeParams(fields *ast.FieldList) {
	if fields == nil {
		return
	}
	for _, f := range fields.List {
		for _, id := range f.Names {
			b.declare(id, "type-param")
		}
	}
	for _, f := range fields.List {
		b.walk(f.Type)
	}
}

func (b *bindings) signature(ft *ast.FuncType, recv *ast.FieldList) {
	b.typeParams(ft.TypeParams)
	for _, fields := range []*ast.FieldList{recv, ft.Params, ft.Results} {
		if fields != nil {
			for _, f := range fields.List {
				b.walk(f.Type)
			}
		}
	}
	for _, fields := range []*ast.FieldList{recv, ft.Params, ft.Results} {
		if fields != nil {
			for _, f := range fields.List {
				for _, id := range f.Names {
					b.declare(id, "variable")
				}
			}
		}
	}
}

func (b *bindings) block(body *ast.BlockStmt) {
	if body != nil {
		for _, stmt := range body.List {
			b.walk(stmt)
		}
	}
}

func (b *bindings) short(lhs []ast.Expr) {
	for _, expr := range lhs {
		id, ok := expr.(*ast.Ident)
		if !ok {
			b.walk(expr)
			continue
		}
		if obj := b.current.names[id.Name]; obj != nil {
			b.idents[id] = obj
		} else {
			b.declare(id, "variable")
		}
	}
}

func (b *bindings) walk(node ast.Node) {
	if node == nil {
		return
	}
	switch n := node.(type) {
	case *ast.Ident:
		b.reference(n)
	case *ast.FuncDecl:
		b.push()
		if n.Recv != nil {
			// Receiver type arguments declare method-local type parameters.
			for _, f := range n.Recv.List {
				typ := unparen(f.Type)
				if p, ok := typ.(*ast.StarExpr); ok {
					typ = unparen(p.X)
				}
				var args []ast.Expr
				switch t := typ.(type) {
				case *ast.IndexExpr:
					args = []ast.Expr{t.Index}
				case *ast.IndexListExpr:
					args = t.Indices
				}
				for _, arg := range args {
					if id, ok := arg.(*ast.Ident); ok {
						b.declare(id, "type-param")
					}
				}
			}
		}
		b.signature(n.Type, n.Recv)
		b.block(n.Body)
		b.pop()
	case *ast.FuncLit:
		b.push()
		b.signature(n.Type, nil)
		b.block(n.Body)
		b.pop()
	case *ast.FuncType:
		b.push()
		b.signature(n, nil)
		b.pop()
	case *ast.BlockStmt:
		b.push()
		b.block(n)
		b.pop()
	case *ast.GenDecl:
		for _, spec := range n.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				if b.idents[s.Name] == nil {
					b.declare(s.Name, "named-type")
				}
				b.push()
				b.typeParams(s.TypeParams)
				b.walk(s.Type)
				b.pop()
			case *ast.ValueSpec:
				b.walk(s.Type)
				for _, v := range s.Values {
					b.walk(v)
				}
				for _, id := range s.Names {
					if b.idents[id] == nil {
						b.declare(id, "variable")
					}
				}
			}
		}
	case *ast.DeclStmt:
		b.walk(n.Decl)
	case *ast.AssignStmt:
		for _, rhs := range n.Rhs {
			b.walk(rhs)
		}
		if n.Tok == token.DEFINE {
			b.short(n.Lhs)
		} else {
			for _, lhs := range n.Lhs {
				b.walk(lhs)
			}
		}
	case *ast.RangeStmt:
		b.walk(n.X)
		b.push()
		if n.Tok == token.DEFINE {
			var lhs []ast.Expr
			if n.Key != nil {
				lhs = append(lhs, n.Key)
			}
			if n.Value != nil {
				lhs = append(lhs, n.Value)
			}
			b.short(lhs)
		} else {
			b.walk(n.Key)
			b.walk(n.Value)
		}
		b.walk(n.Body)
		b.pop()
	case *ast.IfStmt:
		b.push()
		b.walk(n.Init)
		b.walk(n.Cond)
		b.walk(n.Body)
		b.walk(n.Else)
		b.pop()
	case *ast.ForStmt:
		b.push()
		b.walk(n.Init)
		b.walk(n.Cond)
		b.walk(n.Post)
		b.walk(n.Body)
		b.pop()
	case *ast.SwitchStmt:
		b.push()
		b.walk(n.Init)
		b.walk(n.Tag)
		b.block(n.Body)
		b.pop()
	case *ast.TypeSwitchStmt:
		b.push()
		b.walk(n.Init)
		var guard *ast.Ident
		if a, ok := n.Assign.(*ast.AssignStmt); ok && a.Tok == token.DEFINE {
			if len(a.Lhs) == 1 {
				guard, _ = a.Lhs[0].(*ast.Ident)
			}
			for _, rhs := range a.Rhs {
				b.walk(rhs)
			}
		} else {
			b.walk(n.Assign)
		}
		guardBinding := &binding{kind: "type-switch-guard"}
		if guard != nil {
			b.idents[guard] = guardBinding
		}
		for caseNum, stmt := range n.Body.List {
			c := stmt.(*ast.CaseClause)
			for _, typ := range c.List {
				b.walk(typ)
			}
			b.push()
			// The guard's spelling is a declaration, but each case has its own binding.
			if guard != nil {
				b.current.names[guard.Name] = &binding{
					kind: "type-switch-case", guard: guardBinding, caseNum: caseNum,
				}
			}
			for _, s := range c.Body {
				b.walk(s)
			}
			b.pop()
		}
		b.pop()
	case *ast.CaseClause:
		for _, expr := range n.List {
			b.walk(expr)
		}
		b.push()
		for _, stmt := range n.Body {
			b.walk(stmt)
		}
		b.pop()
	case *ast.CommClause:
		b.push()
		b.walk(n.Comm)
		for _, stmt := range n.Body {
			b.walk(stmt)
		}
		b.pop()
	case *ast.SelectorExpr:
		b.walk(n.X)
	case *ast.StructType:
		for _, f := range n.Fields.List {
			b.walk(f.Type)
		}
	case *ast.InterfaceType:
		for _, f := range n.Methods.List {
			b.walk(f.Type)
		}
	case *ast.CompositeLit:
		b.walk(n.Type)
		expressionKeys := false
		switch unparen(n.Type).(type) {
		case *ast.MapType, *ast.ArrayType:
			expressionKeys = true
		}
		for _, e := range n.Elts {
			if kv, ok := e.(*ast.KeyValueExpr); ok {
				_, bareKey := unparen(kv.Key).(*ast.Ident)
				if !bareKey || expressionKeys {
					b.walk(kv.Key)
				}
				b.walk(kv.Value)
			} else {
				b.walk(e)
			}
		}
	case *ast.BranchStmt:
		// Labels are not variable references.
	case *ast.LabeledStmt:
		b.walk(n.Stmt)
	default:
		first := true
		ast.Inspect(node, func(child ast.Node) bool {
			if child == nil {
				return false
			}
			if first {
				first = false
				return true
			}
			b.walk(child)
			return false
		})
	}
}

func unparen(expr ast.Expr) ast.Expr {
	for {
		p, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = p.X
	}
}
