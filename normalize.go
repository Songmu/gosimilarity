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
	ids      map[*binding]int
	source   []byte
	file     *token.File
}

func (n *normalizer) ident(id *ast.Ident) string {
	obj := n.bindings.idents[id]
	if obj != nil && obj.kind == "type-switch-guard" {
		return "TypeSwitchGuard:" + strconv.Itoa(n.bindingID(obj))
	}
	if obj != nil && obj.kind == "type-switch-case" {
		return "TypeSwitchCase:" + strconv.Itoa(n.bindingID(obj.guard)) + ":" + strconv.Itoa(obj.caseNum)
	}
	if obj != nil && (obj.kind == "variable" || obj.kind == "type-param") {
		i := n.bindingID(obj)
		return "BoundIdent:" + obj.kind + ":" + strconv.Itoa(i)
	}
	return "Ident:" + id.Name
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

func (n *normalizer) node(node ast.Node) (*tree, error) {
	return n.nodeRole(node, "")
}

// Roles live on child labels and scalar attributes on parent labels. Field
// lists are flattened so containers and absent slots do not inflate Size.
func (n *normalizer) nodeRole(node ast.Node, role string) (*tree, error) {
	if node == nil || reflect.ValueOf(node).IsNil() {
		return nil, nil
	}
	switch v := node.(type) {
	case *ast.ParenExpr:
		return n.nodeRole(v.X, role)
	case *ast.Ident:
		return n.trees.node(role + "|" + n.ident(v)), nil
	case *ast.BasicLit:
		value, err := n.literalValue(v)
		if err != nil {
			return nil, err
		}
		return n.trees.node(role + "|BasicLit:" + v.Kind.String() + ":" + strconv.Quote(value)), nil
	case *ast.BadExpr, *ast.BadStmt, *ast.BadDecl:
		return nil, fmt.Errorf("invalid syntax node %T", node)
	}
	v := reflect.ValueOf(node).Elem()
	typ := v.Type()
	label := role + "|" + typ.Name()
	children := make([]*tree, 0, v.NumField())
	add := func(child ast.Node, role string) error {
		if fields, ok := child.(*ast.FieldList); ok {
			if fields != nil {
				for _, field := range fields.List {
					t, err := n.nodeRole(field, role+".List")
					if err != nil {
						return err
					}
					children = append(children, t)
				}
			}
			return nil
		}
		t, err := n.nodeRole(child, role)
		if err != nil {
			return err
		}
		if t != nil {
			children = append(children, t)
		}
		return nil
	}
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
				label += "|" + field.Name + "=" + strconv.FormatBool(value.Int() != 0)
			}
			continue
		}
		if field.Type == tokenType {
			label += "|" + field.Name + "=" + strconv.Quote(token.Token(value.Int()).String())
			continue
		}
		switch {
		case field.Type.Implements(nodeType):
			var child ast.Node
			if !value.IsNil() {
				child = value.Interface().(ast.Node)
			}
			if err := add(child, field.Name); err != nil {
				return nil, err
			}
		case value.Kind() == reflect.Slice && field.Type.Elem().Implements(nodeType):
			for j := 0; j < value.Len(); j++ {
				if err := add(value.Index(j).Interface().(ast.Node), field.Name); err != nil {
					return nil, err
				}
			}
		case value.Kind() == reflect.String:
			label += "|" + field.Name + "=" + strconv.Quote(value.String())
		case value.Kind() == reflect.Bool:
			label += "|" + field.Name + "=" + strconv.FormatBool(value.Bool())
		case typ.Name() == "ChanType" && field.Name == "Dir":
			label += "|" + field.Name + "=" + strconv.FormatInt(value.Int(), 10)
		default:
			return nil, fmt.Errorf("unsupported AST field %s.%s (%s)", typ.Name(), field.Name, field.Type)
		}
	}
	return n.trees.node(label, children...), nil
}

func (n *normalizer) literalValue(lit *ast.BasicLit) (string, error) {
	if lit.Kind != token.STRING || len(lit.Value) == 0 || lit.Value[0] != '`' || n.file == nil {
		return lit.Value, nil
	}
	// The scanner removes CR characters from raw strings, so Value
	// and End cannot recover their exact source spelling.
	start := n.file.Offset(lit.Pos())
	end := bytes.IndexByte(n.source[start+1:], '`')
	if end < 0 {
		return "", fmt.Errorf("unterminated raw string literal")
	}
	return string(n.source[start : start+end+2]), nil
}
