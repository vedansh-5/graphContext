package lang

import (
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
)

// Line returns a node's 1-based starting line.
func Line(n *sitter.Node) int { return int(n.StartPoint().Row) + 1 }

// EndLine returns a node's 1-based ending line.
func EndLine(n *sitter.Node) int { return int(n.EndPoint().Row) + 1 }

// Text returns a node's source text.
func Text(n *sitter.Node, src []byte) string {
	if n == nil {
		return ""
	}
	return n.Content(src)
}

// Enclosing walks up from a node to the nearest ancestor of one of the given
// types. This generalises v1's getContainingFunction: scoping a tree-sitter
// query to "calls inside a function" is fragile, but walking parents is exact.
func Enclosing(n *sitter.Node, types ...string) *sitter.Node {
	want := make(map[string]bool, len(types))
	for _, t := range types {
		want[t] = true
	}
	for cur := n.Parent(); cur != nil; cur = cur.Parent() {
		if want[cur.Type()] {
			return cur
		}
	}
	return nil
}

// Children returns a node's named children of a given type.
func Children(n *sitter.Node, typ string) []*sitter.Node {
	if n == nil {
		return nil
	}
	var out []*sitter.Node
	for i := 0; i < int(n.NamedChildCount()); i++ {
		c := n.NamedChild(i)
		if c.Type() == typ {
			out = append(out, c)
		}
	}
	return out
}

// FirstChild returns a node's first named child of a given type, or nil.
func FirstChild(n *sitter.Node, typ string) *sitter.Node {
	c := Children(n, typ)
	if len(c) == 0 {
		return nil
	}
	return c[0]
}

// Walk visits every named node depth-first, in source order.
func Walk(n *sitter.Node, fn func(*sitter.Node)) {
	if n == nil {
		return
	}
	if n.IsNamed() {
		fn(n)
	}
	for i := 0; i < int(n.ChildCount()); i++ {
		Walk(n.Child(i), fn)
	}
}

// BaseTypeName strips pointer, slice, array, and package qualifiers from a type
// expression, leaving the bare type name. "*pkg.UserRepo" becomes "UserRepo".
//
// Dropping the package qualifier is deliberate: pass 1 records names, and pass 2
// resolves them against the import map. Keeping a half-qualified name here would
// force the resolver to re-parse it.
func BaseTypeName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "*&[]")
	if i := strings.LastIndex(s, "."); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.Index(s, "["); i >= 0 { // generics: List[T] / Foo[Bar]
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// ExportedByCase reports whether an identifier starts with an uppercase letter.
// Correct for Go; a reasonable convention signal elsewhere.
func ExportedByCase(name string) bool {
	for _, r := range name {
		return unicode.IsUpper(r)
	}
	return false
}

// ReceiverExpr returns the text left of the final dot in a selector expression,
// which is what the resolver needs to work out what a method call targets.
func ReceiverExpr(sel *sitter.Node, src []byte) string {
	if sel == nil {
		return ""
	}
	operand := sel.ChildByFieldName("operand")
	if operand == nil && sel.NamedChildCount() > 0 {
		operand = sel.NamedChild(0)
	}
	return Text(operand, src)
}
