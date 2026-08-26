package lang

import (
	"sort"
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/vedansh-5/graphcontext/pkg/store"
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

// MakeNode builds a store.Node from a tree-sitter declaration node, filling in
// the position fields every plugin computes identically. Visibility, test and
// entrypoint flags are language-specific and set by the caller.
func MakeNode(path, name, qname string, kind store.NodeKind, language string, n *sitter.Node) store.Node {
	return store.Node{
		ID:             path + ":" + qname,
		Kind:           kind,
		Name:           name,
		QualifiedName:  qname,
		FilePath:       path,
		Language:       language,
		StartLine:      Line(n),
		EndLine:        EndLine(n),
		StartByte:      int(n.StartByte()),
		EndByte:        int(n.EndByte()),
		EntrypointKind: store.EntryNone,
	}
}

// SortIR enforces deterministic ordering on an IR. Tree traversal is already in
// source order; sorting makes the guarantee explicit and survives refactors.
func SortIR(ir *FileIR) {
	sort.SliceStable(ir.Nodes, func(i, j int) bool {
		if ir.Nodes[i].StartByte != ir.Nodes[j].StartByte {
			return ir.Nodes[i].StartByte < ir.Nodes[j].StartByte
		}
		return ir.Nodes[i].ID < ir.Nodes[j].ID
	})
	sort.SliceStable(ir.Refs, func(i, j int) bool {
		a, b := ir.Refs[i], ir.Refs[j]
		switch {
		case a.Line != b.Line:
			return a.Line < b.Line
		case a.FromID != b.FromID:
			return a.FromID < b.FromID
		case a.Kind != b.Kind:
			return a.Kind < b.Kind
		case a.Receiver != b.Receiver:
			return a.Receiver < b.Receiver
		default:
			return a.Name < b.Name
		}
	})
	sort.SliceStable(ir.Imports, func(i, j int) bool {
		if ir.Imports[i].Line != ir.Imports[j].Line {
			return ir.Imports[i].Line < ir.Imports[j].Line
		}
		return ir.Imports[i].Path < ir.Imports[j].Path
	})
	for k, v := range ir.Types.Methods {
		sort.Strings(v)
		ir.Types.Methods[k] = v
	}
	for k, v := range ir.Types.Bases {
		sort.Strings(v)
		ir.Types.Bases[k] = v
	}
}

// DeclIndex returns a lookup from a tree-sitter node to the ID of the innermost
// enclosing function or method declaration, using byte ranges already recorded
// on the IR. Refs are attributed this way rather than by scoping the query,
// because nested and decorated definitions make query scoping unreliable.
func DeclIndex(ir *FileIR) func(*sitter.Node) string {
	type span struct {
		start, end int
		id         string
	}
	var spans []span
	for _, n := range ir.Nodes {
		if n.Kind == store.KindFunction || n.Kind == store.KindMethod {
			spans = append(spans, span{n.StartByte, n.EndByte, n.ID})
		}
	}
	return func(n *sitter.Node) string {
		pos := int(n.StartByte())
		best, bestLen := "", 1<<62
		for _, s := range spans {
			if pos >= s.start && pos < s.end && s.end-s.start < bestLen {
				best, bestLen = s.id, s.end-s.start
			}
		}
		return best
	}
}

// InsertSorted adds v to xs if absent, keeping xs sorted.
func InsertSorted(xs []string, v string) []string {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	xs = append(xs, v)
	sort.Strings(xs)
	return xs
}
