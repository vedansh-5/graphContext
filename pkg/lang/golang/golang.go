// Package golang implements the Go language plugin.
//
// Go is the highest-fidelity of the three plugins because its grammar makes
// almost every type syntactically visible: method receivers, struct fields, and
// variable declarations all carry declared types. That means the resolver can
// resolve most method calls exactly, without any type inference.
package golang

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	tsgo "github.com/smacker/go-tree-sitter/golang"
	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

// Plugin is the Go language plugin.
type Plugin struct{}

func init() { lang.Register(Plugin{}) }

func (Plugin) Name() string         { return "go" }
func (Plugin) Extensions() []string { return []string{".go"} }

// ModulePath maps a file to the import path suffix other files use. Go imports
// address directories, not files, so the file name is dropped.
func (Plugin) ModulePath(repoRoot, filePath string) string {
	rel, err := filepath.Rel(repoRoot, filePath)
	if err != nil {
		rel = filePath
	}
	return filepath.ToSlash(filepath.Dir(rel))
}

// Parse extracts the intermediate form from one Go file.
func (p Plugin) Parse(path string, src []byte) (*lang.FileIR, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(tsgo.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	defer tree.Close()

	ir := &lang.FileIR{
		Path:     path,
		Language: "go",
		Types:    lang.NewTypeFacts(),
	}
	isTestFile := strings.HasSuffix(path, "_test.go")
	pkgName := packageName(tree.RootNode(), src)

	// Declarations first, so every ref can be attributed to an enclosing node.
	lang.Walk(tree.RootNode(), func(n *sitter.Node) {
		switch n.Type() {
		case "import_declaration":
			p.imports(n, src, ir)
		case "type_declaration":
			p.typeDecl(n, src, path, ir)
		case "function_declaration":
			p.funcDecl(n, src, path, pkgName, isTestFile, ir)
		case "method_declaration":
			p.methodDecl(n, src, path, isTestFile, ir)
		}
	})

	// References second: they need the declaration set to attribute FromID.
	p.refs(tree.RootNode(), src, path, ir)

	sortIR(ir)
	return ir, nil
}

func packageName(root *sitter.Node, src []byte) string {
	if c := lang.FirstChild(root, "package_clause"); c != nil {
		return lang.Text(lang.FirstChild(c, "package_identifier"), src)
	}
	return ""
}

func (Plugin) imports(n *sitter.Node, src []byte, ir *lang.FileIR) {
	lang.Walk(n, func(spec *sitter.Node) {
		if spec.Type() != "import_spec" {
			return
		}
		raw := lang.Text(spec.ChildByFieldName("path"), src)
		ref := lang.ImportRef{
			Path:  strings.Trim(raw, "`\""),
			Alias: lang.Text(spec.ChildByFieldName("name"), src),
			Line:  lang.Line(spec),
		}
		if ref.Alias == "." {
			ref.Wildcard, ref.Alias = true, ""
		}
		ir.Imports = append(ir.Imports, ref)
	})
}

// typeDecl records struct and interface declarations plus their field types.
func (Plugin) typeDecl(n *sitter.Node, src []byte, path string, ir *lang.FileIR) {
	for _, spec := range lang.Children(n, "type_spec") {
		name := lang.Text(spec.ChildByFieldName("name"), src)
		if name == "" {
			continue
		}
		body := spec.ChildByFieldName("type")
		kind := store.KindClass
		if body != nil && body.Type() == "interface_type" {
			kind = store.KindInterface
		}
		ir.Nodes = append(ir.Nodes, node(path, name, name, kind, "go", spec, src))

		if body == nil {
			continue
		}
		switch body.Type() {
		case "struct_type":
			// Struct field types are the backbone of Go call resolution:
			// they turn "r.db.Write()" into a lookup on a known type.
			lang.Walk(body, func(f *sitter.Node) {
				if f.Type() != "field_declaration" {
					return
				}
				typ := lang.BaseTypeName(lang.Text(f.ChildByFieldName("type"), src))
				for _, id := range lang.Children(f, "field_identifier") {
					if fn := lang.Text(id, src); fn != "" && typ != "" {
						ir.Types.Fields[name+"."+fn] = typ
					}
				}
			})
		case "interface_type":
			// An interface's method set is what types are matched against to
			// derive `implements` edges structurally in pass 2.
			var methods []string
			lang.Walk(body, func(m *sitter.Node) {
				switch m.Type() {
				case "method_elem", "method_spec":
					if mn := lang.Text(m.ChildByFieldName("name"), src); mn != "" {
						methods = append(methods, mn)
					}
				case "type_identifier": // embedded interface
					if e := lang.Text(m, src); e != "" && e != name {
						ir.Types.Bases[name] = append(ir.Types.Bases[name], e)
					}
				}
			})
			sort.Strings(methods)
			ir.Types.Methods[name] = methods
		}
	}
}

func (p Plugin) funcDecl(n *sitter.Node, src []byte, path, pkg string, isTestFile bool, ir *lang.FileIR) {
	name := lang.Text(n.ChildByFieldName("name"), src)
	if name == "" {
		return
	}
	nd := node(path, name, name, store.KindFunction, "go", n, src)
	nd.Signature = signature(n, src)
	nd.IsTest = isTestFile && isTestFunc(name)
	if pkg == "main" && name == "main" {
		nd.IsEntrypoint, nd.EntrypointKind = true, store.EntryMain
	}
	ir.Nodes = append(ir.Nodes, nd)
	p.paramTypes(n, nd.ID, src, ir)
	p.localTypes(n, nd.ID, src, ir)
}

// methodDecl records a method and — critically — the receiver's type, which is
// what makes every `r.field` reference inside the body resolvable.
func (p Plugin) methodDecl(n *sitter.Node, src []byte, path string, isTestFile bool, ir *lang.FileIR) {
	name := lang.Text(n.ChildByFieldName("name"), src)
	if name == "" {
		return
	}
	recvName, recvType := receiver(n, src)
	qname := name
	if recvType != "" {
		qname = recvType + "." + name
		ir.Types.Methods[recvType] = insertSorted(ir.Types.Methods[recvType], name)
	}

	nd := node(path, name, qname, store.KindMethod, "go", n, src)
	nd.Signature = signature(n, src)
	nd.IsTest = isTestFile && isTestFunc(name)
	ir.Nodes = append(ir.Nodes, nd)

	if recvName != "" && recvType != "" {
		ir.Types.Vars[lang.VarKey(nd.ID, recvName)] = recvType
	}
	p.paramTypes(n, nd.ID, src, ir)
	p.localTypes(n, nd.ID, src, ir)
}

// receiver returns the receiver's binding name and base type.
func receiver(n *sitter.Node, src []byte) (name, typ string) {
	r := n.ChildByFieldName("receiver")
	if r == nil {
		return "", ""
	}
	for _, d := range lang.Children(r, "parameter_declaration") {
		name = lang.Text(d.ChildByFieldName("name"), src)
		typ = lang.BaseTypeName(lang.Text(d.ChildByFieldName("type"), src))
	}
	return name, typ
}

func (Plugin) paramTypes(n *sitter.Node, ownerID string, src []byte, ir *lang.FileIR) {
	params := n.ChildByFieldName("parameters")
	if params == nil {
		return
	}
	for _, d := range lang.Children(params, "parameter_declaration") {
		typ := lang.BaseTypeName(lang.Text(d.ChildByFieldName("type"), src))
		if typ == "" {
			continue
		}
		for _, id := range lang.Children(d, "identifier") {
			if pn := lang.Text(id, src); pn != "" {
				ir.Types.Vars[lang.VarKey(ownerID, pn)] = typ
			}
		}
	}
}

// localTypes records local variable types from the two forms Go makes visible:
// an explicit `var x T`, and `x := T{}` / `x := NewT()` composite or constructor
// literals. Anything else stays unrecorded and degrades to scope narrowing.
func (Plugin) localTypes(n *sitter.Node, ownerID string, src []byte, ir *lang.FileIR) {
	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}
	lang.Walk(body, func(s *sitter.Node) {
		switch s.Type() {
		case "var_spec":
			typ := lang.BaseTypeName(lang.Text(s.ChildByFieldName("type"), src))
			if typ == "" {
				return
			}
			for _, id := range lang.Children(s, "identifier") {
				if vn := lang.Text(id, src); vn != "" {
					ir.Types.Vars[lang.VarKey(ownerID, vn)] = typ
				}
			}
		case "short_var_declaration":
			left, right := s.ChildByFieldName("left"), s.ChildByFieldName("right")
			if left == nil || right == nil {
				return
			}
			names := lang.Children(left, "identifier")
			vals := []*sitter.Node{}
			for i := 0; i < int(right.NamedChildCount()); i++ {
				vals = append(vals, right.NamedChild(i))
			}
			if len(names) != len(vals) {
				return // multi-return calls: type unknown without cross-file info
			}
			for i, id := range names {
				if typ := literalType(vals[i], src); typ != "" {
					ir.Types.Vars[lang.VarKey(ownerID, lang.Text(id, src))] = typ
				}
			}
		}
	})
}

// literalType extracts a type from a right-hand expression when it is visible:
// a composite literal T{...}, or &T{...}. Constructor calls like NewT() are left
// to pass 2, which can look up NewT's declared return type.
func literalType(v *sitter.Node, src []byte) string {
	if v == nil {
		return ""
	}
	switch v.Type() {
	case "composite_literal":
		return lang.BaseTypeName(lang.Text(v.ChildByFieldName("type"), src))
	case "unary_expression":
		return literalType(v.ChildByFieldName("operand"), src)
	}
	return ""
}

// refs records every call site, attributed to its enclosing declaration.
func (Plugin) refs(root *sitter.Node, src []byte, path string, ir *lang.FileIR) {
	byRange := declIndex(ir)
	lang.Walk(root, func(n *sitter.Node) {
		if n.Type() != "call_expression" {
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
		case "selector_expression":
			name = lang.Text(fn.ChildByFieldName("field"), src)
			recv = lang.ReceiverExpr(fn, src)
		default:
			return
		}
		if name == "" {
			return
		}
		from := byRange(n)
		if from == "" {
			return // package-level initialiser; no enclosing declaration
		}
		ir.Refs = append(ir.Refs, lang.Ref{
			Kind: store.EdgeCalls, Name: name, Receiver: recv,
			FromID: from, Line: lang.Line(n),
		})
	})
}

// declIndex returns a lookup from a node position to the enclosing declaration
// ID, using the byte ranges already recorded on the IR's nodes.
func declIndex(ir *lang.FileIR) func(*sitter.Node) string {
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

func node(path, name, qname string, kind store.NodeKind, language string, n *sitter.Node, src []byte) store.Node {
	vis := store.VisPrivate
	if lang.ExportedByCase(name) {
		vis = store.VisExported
	}
	return store.Node{
		ID: path + ":" + qname, Kind: kind, Name: name, QualifiedName: qname,
		FilePath: path, Language: language,
		StartLine: lang.Line(n), EndLine: lang.EndLine(n),
		StartByte: int(n.StartByte()), EndByte: int(n.EndByte()),
		Visibility: vis, EntrypointKind: store.EntryNone,
	}
}

// signature is the declaration's first line, which is enough for an agent to
// see the shape without reading the file.
func signature(n *sitter.Node, src []byte) string {
	body := n.ChildByFieldName("body")
	end := n.EndByte()
	if body != nil {
		end = body.StartByte()
	}
	return strings.TrimSpace(string(src[n.StartByte():end]))
}

func isTestFunc(name string) bool {
	for _, p := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func insertSorted(xs []string, v string) []string {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	xs = append(xs, v)
	sort.Strings(xs)
	return xs
}

// sortIR enforces deterministic ordering. Tree traversal is already in source
// order, but sorting makes the guarantee explicit and survives refactors.
func sortIR(ir *lang.FileIR) {
	sort.SliceStable(ir.Nodes, func(i, j int) bool {
		if ir.Nodes[i].StartByte != ir.Nodes[j].StartByte {
			return ir.Nodes[i].StartByte < ir.Nodes[j].StartByte
		}
		return ir.Nodes[i].ID < ir.Nodes[j].ID
	})
	sort.SliceStable(ir.Refs, func(i, j int) bool {
		a, b := ir.Refs[i], ir.Refs[j]
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.FromID != b.FromID {
			return a.FromID < b.FromID
		}
		if a.Receiver != b.Receiver {
			return a.Receiver < b.Receiver
		}
		return a.Name < b.Name
	})
	sort.SliceStable(ir.Imports, func(i, j int) bool {
		if ir.Imports[i].Line != ir.Imports[j].Line {
			return ir.Imports[i].Line < ir.Imports[j].Line
		}
		return ir.Imports[i].Path < ir.Imports[j].Path
	})
}
