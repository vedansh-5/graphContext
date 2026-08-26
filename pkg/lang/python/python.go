// Package python implements the Python language plugin.
//
// Python makes far less type information syntactically visible than Go. What is
// available: parameter and variable annotations, constructor calls, and the
// implicit type of `self`. Everything else is absent rather than guessed — the
// resolver narrows by scope instead, and the resolution-rate metric records how
// often that happens.
package python

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	tspy "github.com/smacker/go-tree-sitter/python"
	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

// Plugin is the Python language plugin.
type Plugin struct{}

func init() { lang.Register(Plugin{}) }

func (Plugin) Name() string         { return "python" }
func (Plugin) Extensions() []string { return []string{".py", ".pyi"} }

// ModulePath converts a file path to its dotted module name, so "auth/models.py"
// becomes "auth.models" and matches what an import statement writes.
func (Plugin) ModulePath(repoRoot, filePath string) string {
	rel, err := filepath.Rel(repoRoot, filePath)
	if err != nil {
		rel = filePath
	}
	rel = filepath.ToSlash(rel)
	rel = strings.TrimSuffix(strings.TrimSuffix(rel, ".pyi"), ".py")
	rel = strings.TrimSuffix(rel, "/__init__")
	return strings.ReplaceAll(rel, "/", ".")
}

// Parse extracts the intermediate form from one Python file.
func (p Plugin) Parse(path string, src []byte) (*lang.FileIR, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(tspy.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	defer tree.Close()

	ir := &lang.FileIR{Path: path, Language: "python", Types: lang.NewTypeFacts()}
	isTestFile := testFile(path)

	lang.Walk(tree.RootNode(), func(n *sitter.Node) {
		switch n.Type() {
		case "import_statement", "import_from_statement":
			p.imports(n, src, ir)
		case "class_definition":
			p.classDef(n, src, path, isTestFile, ir)
		case "function_definition":
			// Methods are handled by classDef; only module-level functions here.
			if lang.Enclosing(n, "class_definition") == nil {
				p.funcDef(n, src, path, "", isTestFile, ir)
			}
		}
	})

	p.refs(tree.RootNode(), src, ir)
	lang.SortIR(ir)
	return ir, nil
}

func (Plugin) imports(n *sitter.Node, src []byte, ir *lang.FileIR) {
	switch n.Type() {
	case "import_statement":
		// import os / import numpy as np
		for i := 0; i < int(n.NamedChildCount()); i++ {
			c := n.NamedChild(i)
			switch c.Type() {
			case "dotted_name":
				ir.Imports = append(ir.Imports, lang.ImportRef{
					Path: lang.Text(c, src), Line: lang.Line(n)})
			case "aliased_import":
				ir.Imports = append(ir.Imports, lang.ImportRef{
					Path:  lang.Text(c.ChildByFieldName("name"), src),
					Alias: lang.Text(c.ChildByFieldName("alias"), src),
					Line:  lang.Line(n)})
			}
		}
	case "import_from_statement":
		mod := n.ChildByFieldName("module_name")
		ref := lang.ImportRef{Path: lang.Text(mod, src), Line: lang.Line(n)}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			c := n.NamedChild(i)
			if c == mod {
				continue
			}
			switch c.Type() {
			case "wildcard_import":
				ref.Wildcard = true
			case "dotted_name":
				ref.Names = append(ref.Names, lang.Text(c, src))
			case "aliased_import":
				ref.Names = append(ref.Names, lang.Text(c.ChildByFieldName("name"), src))
			}
		}
		ir.Imports = append(ir.Imports, ref)
	}
}

func (p Plugin) classDef(n *sitter.Node, src []byte, path string, isTestFile bool, ir *lang.FileIR) {
	name := lang.Text(n.ChildByFieldName("name"), src)
	if name == "" {
		return
	}
	nd := lang.MakeNode(path, name, name, store.KindClass, "python", n)
	nd.Visibility = visibility(name)
	nd.Docstring = docstring(n.ChildByFieldName("body"), src)
	ir.Nodes = append(ir.Nodes, nd)

	// Base classes: recorded both as facts (for method resolution up the chain)
	// and as refs (which become `inherits` edges in pass 2).
	if bases := n.ChildByFieldName("superclasses"); bases != nil {
		for i := 0; i < int(bases.NamedChildCount()); i++ {
			b := lang.BaseTypeName(lang.Text(bases.NamedChild(i), src))
			if b == "" || b == "object" {
				continue
			}
			ir.Types.Bases[name] = lang.InsertSorted(ir.Types.Bases[name], b)
			ir.Refs = append(ir.Refs, lang.Ref{
				Kind: store.EdgeInherits, Name: b, FromID: nd.ID, Line: lang.Line(n)})
		}
	}

	for _, m := range methodsOf(n) {
		p.funcDef(m, src, path, name, isTestFile, ir)
	}
}

// methodsOf returns the function definitions directly inside a class body,
// unwrapping decorators so @property and @staticmethod methods are not missed.
func methodsOf(class *sitter.Node) []*sitter.Node {
	body := class.ChildByFieldName("body")
	if body == nil {
		return nil
	}
	var out []*sitter.Node
	for i := 0; i < int(body.NamedChildCount()); i++ {
		c := body.NamedChild(i)
		if c.Type() == "decorated_definition" {
			if d := c.ChildByFieldName("definition"); d != nil {
				c = d
			}
		}
		if c.Type() == "function_definition" {
			out = append(out, c)
		}
	}
	return out
}

func (p Plugin) funcDef(n *sitter.Node, src []byte, path, class string, isTestFile bool, ir *lang.FileIR) {
	name := lang.Text(n.ChildByFieldName("name"), src)
	if name == "" {
		return
	}
	kind, qname := store.KindFunction, name
	if class != "" {
		kind, qname = store.KindMethod, class+"."+name
		ir.Types.Methods[class] = lang.InsertSorted(ir.Types.Methods[class], name)
	}

	nd := lang.MakeNode(path, name, qname, kind, "python", n)
	nd.Visibility = visibility(name)
	nd.Docstring = docstring(n.ChildByFieldName("body"), src)
	nd.Signature = signature(n, src)
	nd.IsTest = isTestFile && strings.HasPrefix(name, "test")
	if class == "" && name == "main" {
		nd.IsEntrypoint, nd.EntrypointKind = true, store.EntryMain
	}

	// Decorators mark routes, CLI commands and fixtures — the entry points that
	// give dead-code analysis a correct root set.
	for _, d := range decorators(n, src) {
		ir.Refs = append(ir.Refs, lang.Ref{
			Kind: store.EdgeDecorates, Name: d, FromID: nd.ID, Line: nd.StartLine})
		if k := entrypointKind(d); k != store.EntryNone {
			nd.IsEntrypoint, nd.EntrypointKind = true, k
		}
	}
	ir.Nodes = append(ir.Nodes, nd)

	// `self` has the enclosing class's type. This single fact is what makes the
	// large majority of Python method calls resolvable with no inference at all.
	if class != "" {
		ir.Types.Vars[lang.VarKey(nd.ID, "self")] = class
	}
	p.paramTypes(n, nd.ID, src, ir)
	p.localTypes(n, nd.ID, class, name, src, ir)
}

func (Plugin) paramTypes(n *sitter.Node, ownerID string, src []byte, ir *lang.FileIR) {
	params := n.ChildByFieldName("parameters")
	if params == nil {
		return
	}
	lang.Walk(params, func(c *sitter.Node) {
		if c.Type() != "typed_parameter" && c.Type() != "typed_default_parameter" {
			return
		}
		typ := lang.BaseTypeName(lang.Text(c.ChildByFieldName("type"), src))
		var pname string
		if id := lang.FirstChild(c, "identifier"); id != nil {
			pname = lang.Text(id, src)
		}
		if pname != "" && typ != "" {
			ir.Types.Vars[lang.VarKey(ownerID, pname)] = typ
		}
	})
}

// localTypes records the two forms Python makes visible: an annotated
// assignment, and a plain assignment whose right-hand side is a constructor
// call. Inside __init__, `self.x = ...` additionally becomes a field type.
func (p Plugin) localTypes(n *sitter.Node, ownerID, class, fnName string, src []byte, ir *lang.FileIR) {
	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}
	inInit := class != "" && fnName == "__init__"

	lang.Walk(body, func(a *sitter.Node) {
		if a.Type() != "assignment" {
			return
		}
		left, right := a.ChildByFieldName("left"), a.ChildByFieldName("right")
		if left == nil {
			return
		}
		typ := lang.BaseTypeName(lang.Text(a.ChildByFieldName("type"), src)) // x: T = ...
		if typ == "" {
			typ = constructedType(right, src) // x = T()
		}
		if typ == "" {
			// self.db = db  — inherit the annotated parameter's type.
			if inInit && left.Type() == "attribute" && right != nil && right.Type() == "identifier" {
				typ = ir.Types.Vars[lang.VarKey(ownerID, lang.Text(right, src))]
			}
			if typ == "" {
				return
			}
		}
		switch left.Type() {
		case "identifier":
			ir.Types.Vars[lang.VarKey(ownerID, lang.Text(left, src))] = typ
		case "attribute":
			if !inInit {
				return
			}
			obj := lang.Text(left.ChildByFieldName("object"), src)
			field := lang.Text(left.ChildByFieldName("attribute"), src)
			if obj == "self" && field != "" {
				ir.Types.Fields[class+"."+field] = typ
			}
		}
	})
}

// constructedType returns T for a call of the form T() or pkg.T(), when T looks
// like a class by naming convention. Bare function calls are left unresolved
// because their return type needs the callee's declaration.
func constructedType(v *sitter.Node, src []byte) string {
	if v == nil || v.Type() != "call" {
		return ""
	}
	name := lang.BaseTypeName(lang.Text(v.ChildByFieldName("function"), src))
	if name == "" || !lang.ExportedByCase(name) {
		return ""
	}
	return name
}

func (Plugin) refs(root *sitter.Node, src []byte, ir *lang.FileIR) {
	byRange := lang.DeclIndex(ir)
	lang.Walk(root, func(n *sitter.Node) {
		if n.Type() != "call" {
			return
		}
		fn := n.ChildByFieldName("function")
		if fn == nil {
			return
		}
		var name, recv string
		switch fn.Type() {
		case "identifier":
			name = lang.Text(fn, src)
		case "attribute":
			name = lang.Text(fn.ChildByFieldName("attribute"), src)
			recv = lang.Text(fn.ChildByFieldName("object"), src)
		default:
			return
		}
		from := byRange(n)
		if name == "" || from == "" {
			return
		}
		ir.Refs = append(ir.Refs, lang.Ref{
			Kind: store.EdgeCalls, Name: name, Receiver: recv,
			FromID: from, Line: lang.Line(n)})
	})
}

// decorators returns the decorator expressions attached to a definition.
func decorators(n *sitter.Node, src []byte) []string {
	parent := n.Parent()
	if parent == nil || parent.Type() != "decorated_definition" {
		return nil
	}
	var out []string
	for _, d := range lang.Children(parent, "decorator") {
		txt := strings.TrimPrefix(lang.Text(d, src), "@")
		if i := strings.Index(txt, "("); i >= 0 {
			txt = txt[:i]
		}
		if txt = strings.TrimSpace(txt); txt != "" {
			out = append(out, txt)
		}
	}
	return out
}

// entrypointKind classifies a decorator by the conventions the major frameworks
// share. Unknown decorators simply do not mark an entry point.
func entrypointKind(dec string) store.EntrypointKind {
	last := dec
	if i := strings.LastIndex(dec, "."); i >= 0 {
		last = dec[i+1:]
	}
	switch strings.ToLower(last) {
	case "route", "get", "post", "put", "delete", "patch", "websocket":
		return store.EntryRoute
	case "command", "group":
		return store.EntryCLI
	case "fixture":
		return store.EntryFixture
	case "task":
		return store.EntryHandler
	}
	return store.EntryNone
}

// docstring returns a block's leading string literal, which is where an agent
// gets a symbol's purpose without reading the file.
func docstring(body *sitter.Node, src []byte) string {
	if body == nil || body.NamedChildCount() == 0 {
		return ""
	}
	first := body.NamedChild(0)
	if first.Type() != "expression_statement" || first.NamedChildCount() == 0 {
		return ""
	}
	s := first.NamedChild(0)
	if s.Type() != "string" {
		return ""
	}
	var sb strings.Builder
	lang.Walk(s, func(c *sitter.Node) {
		if c.Type() == "string_content" {
			sb.WriteString(lang.Text(c, src))
		}
	})
	return strings.TrimSpace(sb.String())
}

func signature(n *sitter.Node, src []byte) string {
	end := n.EndByte()
	if body := n.ChildByFieldName("body"); body != nil {
		end = body.StartByte()
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(string(src[n.StartByte():end])), ":"))
}

// visibility follows Python's leading-underscore convention.
func visibility(name string) store.Visibility {
	if strings.HasPrefix(name, "_") {
		return store.VisPrivate
	}
	return store.VisExported
}

func testFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") ||
		strings.Contains(filepath.ToSlash(path), "/tests/")
}
