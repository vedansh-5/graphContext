// Package generic is a table-driven language plugin.
//
// The Go, Python and TypeScript plugins are hand-written because they extract
// type facts that make call resolution exact. That is slow to do for every
// language. This package trades some of that depth for breadth: a language is
// described by a Spec naming the grammar's node types for declarations and
// calls, and one engine does the rest.
//
// What it extracts: types, functions and methods, call sites with their
// receiver text, and inheritance. What it does not: imports and variable
// types. So calls resolve by name within the language rather than by type,
// and their edges carry the lower confidence that goes with that.
package generic

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

// Call says how to read a call node.
type Call struct {
	// Name is the field holding the called name, when the grammar exposes it
	// directly. Receiver is then the field holding the receiver, if any.
	Name     string
	Receiver string
	// Callee is the field holding the whole callee expression, for grammars
	// where "a.b.c()" is one nested node. The last identifier in it is the
	// name and everything before it the receiver. If Name and Callee are both
	// empty, the call node's first named child is used as the callee.
	Callee string
}

// Scope describes a block that adds methods to a type declared elsewhere.
type Scope struct {
	// Type is the field naming the type the methods belong to.
	Type string
	// Trait is the field naming an interface the block implements, if any.
	Trait string
}

// Decl gives the context a Spec's hooks need to classify a declaration.
type Decl struct {
	Path string
	Name string
	// Type is the enclosing type's name, or "" for a free function.
	Type string
	Node *sitter.Node
	Src  []byte
}

// Spec describes one language to the engine.
type Spec struct {
	Name       string
	Extensions []string
	Grammar    func() *sitter.Language

	// Types maps a node type that declares a type to the kind of node created.
	Types map[string]store.NodeKind
	// BodyRequired lists type node types that only declare a type when they
	// have a body. In C, "struct point p;" uses the same node as the
	// definition of struct point.
	BodyRequired map[string]bool
	// Scopes maps a node type that holds methods for a type without declaring
	// it (a Rust impl block) to where that type is named.
	Scopes map[string]Scope
	// Functions maps a node type that declares a function or method to a fixed
	// name, or "" to read the name from the node. Constructors use a fixed
	// name so that "new Foo()" resolves to the type, not to its constructor.
	Functions map[string]string
	// Calls maps a call node type to how its callee is read.
	Calls map[string]Call
	// BaseFields and BaseNodes locate a type's supertypes: fields on the
	// declaration, and child node types of the declaration.
	BaseFields []string
	BaseNodes  []string

	// IsTest, IsMain and IsExported classify a declaration. All optional;
	// by default nothing is a test or entry point and everything is exported.
	IsTest     func(Decl) bool
	IsMain     func(Decl) bool
	IsExported func(Decl) bool
}

// Plugin adapts a Spec to lang.Language.
type Plugin struct{ spec Spec }

// New returns a plugin for the given spec.
func New(spec Spec) Plugin { return Plugin{spec: spec} }

func (p Plugin) Name() string         { return p.spec.Name }
func (p Plugin) Extensions() []string { return p.spec.Extensions }

// ModulePath is the file's path without its extension. The generic engine does
// not resolve imports, so this only has to be stable and unique.
func (p Plugin) ModulePath(repoRoot, filePath string) string {
	rel, err := filepath.Rel(repoRoot, filePath)
	if err != nil {
		rel = filePath
	}
	rel = filepath.ToSlash(rel)
	return strings.TrimSuffix(rel, filepath.Ext(rel))
}

// identTypes are the leaf node types that carry a name across grammars.
var identTypes = map[string]bool{
	"identifier":           true,
	"type_identifier":      true,
	"field_identifier":     true,
	"simple_identifier":    true,
	"property_identifier":  true,
	"namespace_identifier": true,
	"constant":             true,
	"name":                 true,
}

// Parse extracts the intermediate form from one file.
func (p Plugin) Parse(path string, src []byte) (*lang.FileIR, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(p.spec.Grammar())
	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	defer tree.Close()

	ir := &lang.FileIR{Path: path, Language: p.spec.Name, Types: lang.NewTypeFacts()}
	seen := map[string]int{}

	// Declarations first, so every call can be attributed to an enclosing node.
	lang.Walk(tree.RootNode(), func(n *sitter.Node) {
		if kind, ok := p.spec.Types[n.Type()]; ok {
			p.typeDecl(n, kind, src, path, ir, seen)
		}
		if fixed, ok := p.spec.Functions[n.Type()]; ok {
			p.funcDecl(n, fixed, src, path, ir, seen)
		}
		if scope, ok := p.spec.Scopes[n.Type()]; ok && scope.Trait != "" {
			p.implDecl(n, scope, src, path, ir)
		}
	})
	dropOrphanRefs(ir)
	p.calls(tree.RootNode(), src, ir)

	lang.SortIR(ir)
	return ir, nil
}

func (p Plugin) typeDecl(n *sitter.Node, kind store.NodeKind, src []byte, path string, ir *lang.FileIR, seen map[string]int) {
	if p.spec.BodyRequired[n.Type()] && n.ChildByFieldName("body") == nil {
		return
	}
	name := typeName(declName(n, src))
	if name == "" {
		return
	}
	// A nested type is named on its own: type names are what calls and
	// inheritance refer to, and they rarely repeat the outer type.
	nd := lang.MakeNode(path, name, name, kind, p.spec.Name, n)
	nd.ID = uniqueID(nd.ID, seen)
	nd.Signature = signature(n, src)
	nd.Visibility = p.visibility(Decl{Path: path, Name: name, Node: n, Src: src})
	ir.Nodes = append(ir.Nodes, nd)

	for _, base := range p.bases(n, src) {
		if base == name {
			continue
		}
		ir.Types.Bases[name] = lang.InsertSorted(ir.Types.Bases[name], base)
		ir.Refs = append(ir.Refs, lang.Ref{
			Kind: store.EdgeInherits, Name: base, FromID: nd.ID, Line: lang.Line(n)})
	}
}

// dropOrphanRefs removes refs whose source is not a node in this file. An impl
// block for a type declared in another file has nothing here to hang an edge
// on; its trait is still recorded in the type facts.
func dropOrphanRefs(ir *lang.FileIR) {
	ids := make(map[string]bool, len(ir.Nodes))
	for _, n := range ir.Nodes {
		ids[n.ID] = true
	}
	kept := ir.Refs[:0]
	for _, r := range ir.Refs {
		if ids[r.FromID] {
			kept = append(kept, r)
		}
	}
	ir.Refs = kept
}

// implDecl records that a type implements a trait: "impl Trait for Type".
func (p Plugin) implDecl(n *sitter.Node, scope Scope, src []byte, path string, ir *lang.FileIR) {
	typ := typeName(lang.Text(n.ChildByFieldName(scope.Type), src))
	trait := typeName(lang.Text(n.ChildByFieldName(scope.Trait), src))
	if typ == "" || trait == "" {
		return
	}
	ir.Types.Bases[typ] = lang.InsertSorted(ir.Types.Bases[typ], trait)
	ir.Refs = append(ir.Refs, lang.Ref{
		Kind: store.EdgeImplements, Name: trait, FromID: path + ":" + typ, Line: lang.Line(n)})
}

func (p Plugin) funcDecl(n *sitter.Node, fixed string, src []byte, path string, ir *lang.FileIR, seen map[string]int) {
	name := fixed
	owner := p.enclosingType(n, src)
	if name == "" {
		name = declName(n, src)
		// C++ defines methods out of line as "Type::method".
		if i := strings.LastIndex(name, "::"); i >= 0 {
			if owner == "" {
				owner = typeName(name[:i])
			}
			name = name[i+2:]
		}
	}
	if name == "" {
		return
	}

	kind, qname := store.KindFunction, name
	if owner != "" {
		kind, qname = store.KindMethod, owner+"."+name
		ir.Types.Methods[owner] = lang.InsertSorted(ir.Types.Methods[owner], name)
	}

	d := Decl{Path: path, Name: name, Type: owner, Node: n, Src: src}
	nd := lang.MakeNode(path, name, qname, kind, p.spec.Name, n)
	// Overloads share a qualified name; keep each as its own node.
	nd.ID = uniqueID(nd.ID, seen)
	nd.Signature = signature(n, src)
	nd.Visibility = p.visibility(d)
	nd.IsTest = p.spec.IsTest != nil && p.spec.IsTest(d)
	if p.spec.IsMain != nil && p.spec.IsMain(d) {
		nd.IsEntrypoint, nd.EntrypointKind = true, store.EntryMain
	}
	ir.Nodes = append(ir.Nodes, nd)
}

func (p Plugin) visibility(d Decl) store.Visibility {
	if p.spec.IsExported != nil && !p.spec.IsExported(d) {
		return store.VisPrivate
	}
	return store.VisExported
}

// enclosingType returns the name of the nearest type a node is declared in.
func (p Plugin) enclosingType(n *sitter.Node, src []byte) string {
	for cur := n.Parent(); cur != nil; cur = cur.Parent() {
		if scope, ok := p.spec.Scopes[cur.Type()]; ok {
			if name := typeName(lang.Text(cur.ChildByFieldName(scope.Type), src)); name != "" {
				return name
			}
		}
		if _, ok := p.spec.Types[cur.Type()]; ok {
			if name := typeName(declName(cur, src)); name != "" {
				return name
			}
		}
		// A function nested in another function is not a method of the
		// outer function's type.
		if _, ok := p.spec.Functions[cur.Type()]; ok {
			return ""
		}
	}
	return ""
}

func (p Plugin) bases(n *sitter.Node, src []byte) []string {
	var holders []*sitter.Node
	for _, f := range p.spec.BaseFields {
		if c := n.ChildByFieldName(f); c != nil {
			holders = append(holders, c)
		}
	}
	for _, t := range p.spec.BaseNodes {
		holders = append(holders, lang.Children(n, t)...)
	}

	var out []string
	for _, h := range holders {
		lang.Walk(h, func(c *sitter.Node) {
			// Skip generic arguments: List<Foo> extends List, not Foo.
			for a := c.Parent(); a != nil && a != h; a = a.Parent() {
				if strings.Contains(a.Type(), "type_arguments") {
					return
				}
			}
			if c.NamedChildCount() == 0 && identTypes[c.Type()] {
				if name := lang.Text(c, src); name != "" {
					out = append(out, name)
				}
			}
		})
	}
	return out
}

func (p Plugin) calls(root *sitter.Node, src []byte, ir *lang.FileIR) {
	byRange := lang.DeclIndex(ir)
	lang.Walk(root, func(n *sitter.Node) {
		spec, ok := p.spec.Calls[n.Type()]
		if !ok {
			return
		}
		from := byRange(n)
		if from == "" {
			return // top-level or field initialiser: no enclosing function
		}

		var name, recv string
		if spec.Name != "" {
			name = lastIdent(n.ChildByFieldName(spec.Name), src)
			if spec.Receiver != "" {
				recv = cleanReceiver(lang.Text(n.ChildByFieldName(spec.Receiver), src))
			}
		} else {
			callee := n.NamedChild(0)
			if spec.Callee != "" {
				callee = n.ChildByFieldName(spec.Callee)
			}
			name, recv = splitCallee(callee, src)
		}
		if name == "" {
			return
		}
		ir.Refs = append(ir.Refs, lang.Ref{
			Kind: store.EdgeCalls, Name: name, Receiver: recv,
			FromID: from, Line: lang.Line(n),
		})
	})
}

// splitCallee splits a callee expression into the called name and the receiver
// text before it: "a.b.c" gives ("c", "a.b"), "Foo::new" gives ("new", "Foo").
func splitCallee(callee *sitter.Node, src []byte) (name, recv string) {
	if callee == nil {
		return "", ""
	}
	leaf := lastIdentNode(callee)
	if leaf == nil {
		return "", ""
	}
	name = lang.Text(leaf, src)
	recv = string(src[callee.StartByte():leaf.StartByte()])
	recv = strings.TrimSpace(recv)
	for _, sep := range []string{"?.", "::", "->", "."} {
		if strings.HasSuffix(recv, sep) {
			recv = strings.TrimSpace(strings.TrimSuffix(recv, sep))
			break
		}
	}
	return name, cleanReceiver(recv)
}

// opaqueReceiver stands in for a receiver that is an expression rather than a
// name, such as the result of another call. The resolver cannot look it up,
// but it must still know the call had a receiver.
const opaqueReceiver = "<expr>"

func cleanReceiver(recv string) string {
	// PHP writes variables as $name; "$this" is the resolver's "this".
	recv = strings.TrimPrefix(strings.TrimSpace(recv), "$")
	if strings.ContainsAny(recv, "({ \t\n") {
		return opaqueReceiver
	}
	// A path such as "super::Cart" names the type by its last segment.
	if i := strings.LastIndex(recv, "::"); i >= 0 {
		recv = recv[i+2:]
	}
	// The resolver walks field chains on dots.
	return strings.ReplaceAll(recv, "->", ".")
}

func lastIdent(n *sitter.Node, src []byte) string {
	return lang.Text(lastIdentNode(n), src)
}

// lastIdentNode returns the last identifier leaf under a node, skipping
// argument lists and generic arguments so "f::<T>(x)" still yields "f".
func lastIdentNode(n *sitter.Node) *sitter.Node {
	if n == nil {
		return nil
	}
	if n.NamedChildCount() == 0 {
		if identTypes[n.Type()] {
			return n
		}
		return nil
	}
	for i := int(n.NamedChildCount()) - 1; i >= 0; i-- {
		c := n.NamedChild(i)
		if strings.Contains(c.Type(), "argument") {
			continue
		}
		if leaf := lastIdentNode(c); leaf != nil {
			return leaf
		}
	}
	return nil
}

// declName finds a declaration's name: the "name" field where the grammar has
// one, otherwise the innermost "declarator" (C and C++), otherwise the first
// identifier child (grammars without field names).
func declName(n *sitter.Node, src []byte) string {
	if c := n.ChildByFieldName("name"); c != nil {
		return strings.TrimSpace(lang.Text(c, src))
	}
	if d := n.ChildByFieldName("declarator"); d != nil {
		for {
			next := d.ChildByFieldName("declarator")
			if next == nil {
				break
			}
			d = next
		}
		if d.NamedChildCount() == 0 || d.Type() == "qualified_identifier" {
			return strings.TrimSpace(lang.Text(d, src))
		}
		return ""
	}
	for i := 0; i < int(n.NamedChildCount()); i++ {
		if c := n.NamedChild(i); identTypes[c.Type()] {
			return lang.Text(c, src)
		}
	}
	return ""
}

// typeName reduces a type expression to its bare name: generics, pointers and
// qualifiers are dropped.
func typeName(s string) string {
	if i := strings.IndexAny(s, "<["); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "::"); i >= 0 {
		s = s[i+2:]
	}
	return lang.BaseTypeName(s)
}

// signature is the declaration up to its body, on one line.
func signature(n *sitter.Node, src []byte) string {
	end := n.EndByte()
	if body := n.ChildByFieldName("body"); body != nil {
		end = body.StartByte()
	}
	sig := strings.Join(strings.Fields(string(src[n.StartByte():end])), " ")
	const max = 200
	if len(sig) > max {
		sig = sig[:max] + "..."
	}
	return sig
}

// uniqueID makes a node ID unique within a file by suffixing repeats.
func uniqueID(id string, seen map[string]int) string {
	seen[id]++
	if n := seen[id]; n > 1 {
		return fmt.Sprintf("%s#%d", id, n)
	}
	return id
}

// modifiers returns the text of a declaration's modifiers, where annotations
// and keywords such as "public" and "static" live.
func modifiers(d Decl) string { return childText(d, "modifier") }

// childText joins the text of every direct child whose node type contains
// the given substring.
func childText(d Decl, typeSubstr string) string {
	var parts []string
	for i := 0; i < int(d.Node.ChildCount()); i++ {
		if c := d.Node.Child(i); strings.Contains(c.Type(), typeSubstr) {
			parts = append(parts, lang.Text(c, d.Src))
		}
	}
	return strings.Join(parts, " ")
}

// precedingText joins the text of the siblings directly before a declaration
// whose node type contains the given substring. Rust attributes such as
// #[test] are siblings of the function they annotate, not children.
func precedingText(d Decl, typeSubstr string) string {
	var parts []string
	for sib := d.Node.PrevNamedSibling(); sib != nil; sib = sib.PrevNamedSibling() {
		if !strings.Contains(sib.Type(), typeSubstr) {
			break
		}
		parts = append(parts, lang.Text(sib, d.Src))
	}
	return strings.Join(parts, " ")
}
