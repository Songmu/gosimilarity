package gosimilarity

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"strconv"
)

type normalizer struct {
	trees    *trees
	bindings *bindings
	self     *binding
	ids      map[*binding]int
	source   []byte
	file     *token.File
}

func (n *normalizer) ident(id *ast.Ident) *tree {
	obj := n.bindings.idents[id]
	if obj != nil && obj.kind == "type-switch-guard" {
		return n.trees.node("TypeSwitchGuard:" + strconv.Itoa(n.bindingID(obj)))
	}
	if obj != nil && obj.kind == "type-switch-case" {
		return n.trees.node("TypeSwitchCase:" + strconv.Itoa(n.bindingID(obj.guard)) + ":" + strconv.Itoa(obj.caseNum))
	}
	if obj != nil && obj == n.self {
		return n.trees.node("SelfType")
	}
	if obj != nil && (obj.kind == "variable" || obj.kind == "type-param") {
		i := n.bindingID(obj)
		return n.trees.node("BoundIdent:" + obj.kind + ":" + strconv.Itoa(i))
	}
	return n.trees.node("Ident:" + id.Name)
}

func (n *normalizer) bindingID(obj *binding) int {
	i, ok := n.ids[obj]
	if !ok {
		i = len(n.ids)
		n.ids[obj] = i
	}
	return i
}

var (
	positionType = reflect.TypeFor[token.Pos]()
	tokenType    = reflect.TypeFor[token.Token]()
	nodeType     = reflect.TypeFor[ast.Node]()
)

// Field wrappers retain roles and empty slots. Every wrapper and syntax node
// counts toward Size; positions, comments and redundant parentheses do not.
func (n *normalizer) node(node ast.Node) (*tree, error) {
	if node == nil || reflect.ValueOf(node).IsNil() {
		return n.trees.node("Nil"), nil
	}
	switch v := node.(type) {
	case *ast.ParenExpr:
		return n.node(v.X)
	case *ast.Ident:
		return n.ident(v), nil
	case *ast.BasicLit:
		value := v.Value
		if v.Kind == token.STRING && len(value) > 0 && value[0] == '`' && n.file != nil {
			// The scanner removes CR characters from raw strings, so Value
			// and End cannot recover their exact source spelling.
			start := n.file.Offset(v.Pos())
			end := bytes.IndexByte(n.source[start+1:], '`')
			if end < 0 {
				return nil, fmt.Errorf("unterminated raw string literal")
			}
			value = string(n.source[start : start+end+2])
		}
		return n.trees.node("BasicLit",
			n.trees.node("Kind:"+v.Kind.String()),
			n.trees.node("Value", n.trees.node("Value:"+value))), nil
	case *ast.BadExpr, *ast.BadStmt, *ast.BadDecl:
		return nil, fmt.Errorf("invalid syntax node %T", node)
	}
	v := reflect.ValueOf(node).Elem()
	typ := v.Type()
	children := make([]*tree, 0, v.NumField())
	for i := 0; i < v.NumField(); i++ {
		field := typ.Field(i)
		switch field.Name {
		case "Doc", "Comment", "Obj", "Scope", "Unresolved":
			continue
		}
		value := v.Field(i)
		if field.Type == positionType {
			// Assign distinguishes a type alias; all other token positions
			// are locations, not syntax attributes.
			if field.Name == "Assign" || field.Name == "Ellipsis" {
				children = append(children, n.trees.node(field.Name+":"+strconv.FormatBool(value.Int() != 0)))
			}
			continue
		}
		if field.Type == tokenType {
			children = append(children, n.trees.node(field.Name+":"+token.Token(value.Int()).String()))
			continue
		}
		var slots []*tree
		switch {
		case field.Type.Implements(nodeType):
			var child ast.Node
			if !value.IsNil() {
				child = value.Interface().(ast.Node)
			}
			t, err := n.node(child)
			if err != nil {
				return nil, err
			}
			slots = append(slots, t)
		case value.Kind() == reflect.Slice && field.Type.Elem().Implements(nodeType):
			for j := 0; j < value.Len(); j++ {
				t, err := n.node(value.Index(j).Interface().(ast.Node))
				if err != nil {
					return nil, err
				}
				slots = append(slots, t)
			}
		case value.Kind() == reflect.String:
			slots = append(slots, n.trees.node("Value:"+value.String()))
		case value.Kind() == reflect.Bool:
			slots = append(slots, n.trees.node(strconv.FormatBool(value.Bool())))
		case typ.Name() == "ChanType" && field.Name == "Dir":
			slots = append(slots, n.trees.node(strconv.FormatInt(value.Int(), 10)))
		default:
			return nil, fmt.Errorf("unsupported AST field %s.%s (%s)", typ.Name(), field.Name, field.Type)
		}
		children = append(children, n.trees.node(field.Name, slots...))
	}
	return n.trees.node(typ.Name(), children...), nil
}
