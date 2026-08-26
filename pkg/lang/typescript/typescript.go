package typescript

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	tstsx "github.com/smacker/go-tree-sitter/typescript/tsx"
	tsts "github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

type Plugin struct{}

func init() { lang.Register(Plugin{}) }

func (Plugin) Name() string { return "typescript" }
func (Plugin) Extensions() []string {
	return []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"}
}

func (Plugin) ModulePath(repoRoot, filePath string) string {
	rel, err := filepath.Rel(repoRoot, filePath)
	if err != nil {
		rel = filePath
	}
	rel = filepath.ToSlash(rel)
	for _, ext := range []string{".d.ts", ".tsx", ".ts", ".jsx", ".js", ".mjs", ".cjs"} {
		if strings.HasSuffix(rel, ext) {
			rel = strings.TrimSuffix(rel, ext)
			break
		}
	}
	rel = strings.TrimSuffix(rel, "/index")
	return rel
}

func (p Plugin) Parse(path string, src []byte) (*lang.FileIR, error) {
	parser := sitter.NewParser()
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".tsx" || ext == ".jsx" {
		parser.SetLanguage(tstsx.GetLanguage())
	} else {
		parser.SetLanguage(tsts.GetLanguage())
	}

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	defer tree.Close()

	ir := &lang.FileIR{
		Path:     path,
		Language: "typescript",
		Types:    lang.NewTypeFacts(),
	}
	isTest := testFile(path)

	lang.Walk(tree.RootNode(), func(n *sitter.Node) {
		switch n.Type() {
		case "import_statement":
			p.importStmt(n, src, ir)
		case "export_statement":
			p.exportStmt(n, src, ir)
		case "class_declaration":
			p.classDecl(n, src, path, isTest, ir)
		case "interface_declaration":
			p.interfaceDecl(n, src, path, isTest, ir)
		case "type_alias_declaration":
			p.typeAliasDecl(n, src, path, isTest, ir)
		case "function_declaration", "generator_function_declaration":
			if lang.Enclosing(n, "class_declaration", "class_body") == nil {
				p.funcDecl(n, src, path, isTest, ir)
			}
		case "lexical_declaration", "variable_declaration":
			p.varDecl(n, src, path, isTest, ir)
		}
	})

	p.refs(tree.RootNode(), src, ir)

	lang.SortIR(ir)
	return ir, nil
}

func (Plugin) importStmt(n *sitter.Node, src []byte, ir *lang.FileIR) {
	sourceNode := n.ChildByFieldName("source")
	if sourceNode == nil {
		return
	}
	modPath := strings.Trim(lang.Text(sourceNode, src), "'\"`")
	line := lang.Line(n)

	clause := n.ChildByFieldName("import")
	if clause == nil {
		clause = lang.FirstChild(n, "import_clause")
	}

	if clause == nil {
		ir.Imports = append(ir.Imports, lang.ImportRef{
			Path: modPath,
			Line: line,
		})
		return
	}

	var names []string
	var alias string
	var wildcard bool

	for i := 0; i < int(clause.NamedChildCount()); i++ {
		c := clause.NamedChild(i)
		switch c.Type() {
		case "identifier":
			alias = lang.Text(c, src)
		case "namespace_import":
			wildcard = true
			if id := lang.FirstChild(c, "identifier"); id != nil {
				alias = lang.Text(id, src)
			}
		case "named_imports":
			for j := 0; j < int(c.NamedChildCount()); j++ {
				spec := c.NamedChild(j)
				if spec.Type() == "import_specifier" {
					nameNode := spec.ChildByFieldName("name")
					if nameNode == nil {
						nameNode = spec.NamedChild(0)
					}
					if nameNode != nil {
						names = append(names, lang.Text(nameNode, src))
					}
				}
			}
		}
	}

	ir.Imports = append(ir.Imports, lang.ImportRef{
		Path:     modPath,
		Alias:    alias,
		Names:    names,
		Wildcard: wildcard,
		Line:     line,
	})
}

func (Plugin) exportStmt(n *sitter.Node, src []byte, ir *lang.FileIR) {
	sourceNode := n.ChildByFieldName("source")
	if sourceNode == nil {
		return
	}
	modPath := strings.Trim(lang.Text(sourceNode, src), "'\"`")
	line := lang.Line(n)

	var names []string
	var wildcard bool

	for i := 0; i < int(n.NamedChildCount()); i++ {
		c := n.NamedChild(i)
		switch c.Type() {
		case "export_clause":
			for j := 0; j < int(c.NamedChildCount()); j++ {
				spec := c.NamedChild(j)
				if spec.Type() == "export_specifier" {
					nameNode := spec.ChildByFieldName("name")
					if nameNode == nil {
						nameNode = spec.NamedChild(0)
					}
					if nameNode != nil {
						names = append(names, lang.Text(nameNode, src))
					}
				}
			}
		}
	}

	if lang.FirstChild(n, "gl_star") != nil || strings.Contains(lang.Text(n, src), "*") {
		wildcard = true
	}

	ir.Imports = append(ir.Imports, lang.ImportRef{
		Path:     modPath,
		Names:    names,
		Wildcard: wildcard,
		Line:     line,
	})
}

func (p Plugin) classDecl(n *sitter.Node, src []byte, path string, isTest bool, ir *lang.FileIR) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	className := lang.Text(nameNode, src)
	node := lang.MakeNode(path, className, className, store.KindClass, "typescript", n)
	node.Docstring = jsdoc(n, src)
	node.Signature = signature(n, src)
	node.Visibility = visibility(n)
	node.IsTest = isTest
	ir.Nodes = append(ir.Nodes, node)

	if heritage := lang.FirstChild(n, "class_heritage"); heritage != nil {
		for i := 0; i < int(heritage.NamedChildCount()); i++ {
			c := heritage.NamedChild(i)
			switch c.Type() {
			case "extends_clause":
				if val := c.ChildByFieldName("value"); val != nil {
					base := lang.BaseTypeName(lang.Text(val, src))
					ir.Types.Bases[className] = lang.InsertSorted(ir.Types.Bases[className], base)
					ir.Refs = append(ir.Refs, lang.Ref{
						Kind:   store.EdgeInherits,
						Name:   base,
						FromID: node.ID,
						Line:   lang.Line(c),
					})
				} else if c.NamedChildCount() > 0 {
					base := lang.BaseTypeName(lang.Text(c.NamedChild(0), src))
					ir.Types.Bases[className] = lang.InsertSorted(ir.Types.Bases[className], base)
					ir.Refs = append(ir.Refs, lang.Ref{
						Kind:   store.EdgeInherits,
						Name:   base,
						FromID: node.ID,
						Line:   lang.Line(c),
					})
				}
			case "implements_clause":
				for j := 0; j < int(c.NamedChildCount()); j++ {
					implNode := c.NamedChild(j)
					base := lang.BaseTypeName(lang.Text(implNode, src))
					ir.Types.Bases[className] = lang.InsertSorted(ir.Types.Bases[className], base)
					ir.Refs = append(ir.Refs, lang.Ref{
						Kind:   store.EdgeImplements,
						Name:   base,
						FromID: node.ID,
						Line:   lang.Line(implNode),
					})
				}
			}
		}
	}

	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}

	for i := 0; i < int(body.NamedChildCount()); i++ {
		member := body.NamedChild(i)
		switch member.Type() {
		case "method_definition":
			p.methodDef(member, src, path, className, isTest, ir)
		case "public_field_definition", "field_definition", "property_definition":
			p.fieldDef(member, src, className, ir)
		}
	}
}

func (p Plugin) methodDef(n *sitter.Node, src []byte, path, className string, isTest bool, ir *lang.FileIR) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	methodName := lang.Text(nameNode, src)
	qname := className + "." + methodName
	mNode := lang.MakeNode(path, methodName, qname, store.KindMethod, "typescript", n)
	mNode.Docstring = jsdoc(n, src)
	mNode.Signature = signature(n, src)
	mNode.Visibility = memberVisibility(n, methodName, src)
	mNode.IsTest = isTest || strings.HasPrefix(strings.ToLower(methodName), "test")

	ir.Nodes = append(ir.Nodes, mNode)
	ir.Types.Methods[className] = lang.InsertSorted(ir.Types.Methods[className], methodName)

	ir.Types.Vars[lang.VarKey(mNode.ID, "this")] = className

	params := n.ChildByFieldName("parameters")
	paramTypes := p.extractParams(params, src, mNode.ID, ir)

	if methodName == "constructor" {
		body := n.ChildByFieldName("body")
		if body != nil {
			lang.Walk(body, func(expr *sitter.Node) {
				if expr.Type() != "assignment_expression" {
					return
				}
				left := expr.ChildByFieldName("left")
				right := expr.ChildByFieldName("right")
				if left == nil || right == nil {
					return
				}
				if left.Type() == "member_expression" {
					obj := left.ChildByFieldName("object")
					prop := left.ChildByFieldName("property")
					if obj != nil && prop != nil && lang.Text(obj, src) == "this" {
						fieldName := lang.Text(prop, src)
						if right.Type() == "new_expression" {
							if ctor := right.ChildByFieldName("constructor"); ctor != nil {
								ctorType := lang.BaseTypeName(lang.Text(ctor, src))
								ir.Types.Fields[className+"."+fieldName] = ctorType
							}
						} else if right.Type() == "identifier" {
							varName := lang.Text(right, src)
							if typ, ok := paramTypes[varName]; ok && typ != "" {
								ir.Types.Fields[className+"."+fieldName] = typ
							}
						}
					}
				}
			})
		}
	}
}

func (Plugin) fieldDef(n *sitter.Node, src []byte, className string, ir *lang.FileIR) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		nameNode = n.ChildByFieldName("property")
	}
	if nameNode == nil && n.NamedChildCount() > 0 {
		nameNode = n.NamedChild(0)
	}
	if nameNode == nil {
		return
	}
	fieldName := lang.Text(nameNode, src)

	var fieldType string
	if typeNode := n.ChildByFieldName("type"); typeNode != nil {
		fieldType = cleanType(lang.Text(typeNode, src))
	} else if typeAnnot := lang.FirstChild(n, "type_annotation"); typeAnnot != nil {
		fieldType = cleanType(lang.Text(typeAnnot, src))
	}

	if fieldType == "" {
		if val := n.ChildByFieldName("value"); val != nil && val.Type() == "new_expression" {
			if ctor := val.ChildByFieldName("constructor"); ctor != nil {
				fieldType = cleanType(lang.Text(ctor, src))
			}
		}
	}

	if fieldType != "" {
		ir.Types.Fields[className+"."+fieldName] = fieldType
	}
}

func (Plugin) interfaceDecl(n *sitter.Node, src []byte, path string, isTest bool, ir *lang.FileIR) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	ifaceName := lang.Text(nameNode, src)
	node := lang.MakeNode(path, ifaceName, ifaceName, store.KindInterface, "typescript", n)
	node.Docstring = jsdoc(n, src)
	node.Signature = signature(n, src)
	node.Visibility = visibility(n)
	node.IsTest = isTest
	ir.Nodes = append(ir.Nodes, node)

	body := n.ChildByFieldName("body")
	if body == nil {
		return
	}

	for i := 0; i < int(body.NamedChildCount()); i++ {
		member := body.NamedChild(i)
		switch member.Type() {
		case "method_signature", "property_signature":
			if mName := member.ChildByFieldName("name"); mName != nil {
				name := lang.Text(mName, src)
				ir.Types.Methods[ifaceName] = lang.InsertSorted(ir.Types.Methods[ifaceName], name)
			}
		}
	}
}

func (Plugin) typeAliasDecl(n *sitter.Node, src []byte, path string, isTest bool, ir *lang.FileIR) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	typeName := lang.Text(nameNode, src)
	node := lang.MakeNode(path, typeName, typeName, store.KindClass, "typescript", n)
	node.Docstring = jsdoc(n, src)
	node.Signature = signature(n, src)
	node.Visibility = visibility(n)
	node.IsTest = isTest
	ir.Nodes = append(ir.Nodes, node)
}

func (p Plugin) funcDecl(n *sitter.Node, src []byte, path string, isTest bool, ir *lang.FileIR) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	funcName := lang.Text(nameNode, src)
	node := lang.MakeNode(path, funcName, funcName, store.KindFunction, "typescript", n)
	node.Docstring = jsdoc(n, src)
	node.Signature = signature(n, src)
	node.Visibility = visibility(n)
	node.IsTest = isTest || strings.HasPrefix(strings.ToLower(funcName), "test")
	if funcName == "main" {
		node.IsEntrypoint = true
		node.EntrypointKind = store.EntryMain
	}
	ir.Nodes = append(ir.Nodes, node)

	params := n.ChildByFieldName("parameters")
	p.extractParams(params, src, node.ID, ir)
}

func (p Plugin) varDecl(n *sitter.Node, src []byte, path string, isTest bool, ir *lang.FileIR) {
	enclosingFuncID := ""
	if enc := lang.Enclosing(n, "function_declaration", "method_definition", "arrow_function", "function_expression"); enc != nil {
		if encName := enc.ChildByFieldName("name"); encName != nil {
			enclosingFuncID = path + ":" + lang.Text(encName, src)
		}
	}

	for i := 0; i < int(n.NamedChildCount()); i++ {
		c := n.NamedChild(i)
		if c.Type() != "variable_declarator" {
			continue
		}
		nameNode := c.ChildByFieldName("name")
		if nameNode == nil || nameNode.Type() != "identifier" {
			continue
		}
		varName := lang.Text(nameNode, src)
		valueNode := c.ChildByFieldName("value")

		if valueNode != nil && (valueNode.Type() == "arrow_function" || valueNode.Type() == "function_expression") {
			fNode := lang.MakeNode(path, varName, varName, store.KindFunction, "typescript", c)
			fNode.Docstring = jsdoc(n, src)
			fNode.Signature = signature(c, src)
			fNode.Visibility = visibility(n)
			fNode.IsTest = isTest || strings.HasPrefix(strings.ToLower(varName), "test")
			ir.Nodes = append(ir.Nodes, fNode)

			params := valueNode.ChildByFieldName("parameters")
			if params == nil {
				params = lang.FirstChild(valueNode, "formal_parameters")
			}
			p.extractParams(params, src, fNode.ID, ir)
			continue
		}

		var varType string
		if typeNode := c.ChildByFieldName("type"); typeNode != nil {
			varType = cleanType(lang.Text(typeNode, src))
		} else if typeAnnot := lang.FirstChild(c, "type_annotation"); typeAnnot != nil {
			varType = cleanType(lang.Text(typeAnnot, src))
		}

		if varType == "" && valueNode != nil && valueNode.Type() == "new_expression" {
			if ctor := valueNode.ChildByFieldName("constructor"); ctor != nil {
				varType = cleanType(lang.Text(ctor, src))
			}
		}

		if varType != "" {
			scopeID := enclosingFuncID
			if scopeID == "" {
				scopeID = path
			}
			ir.Types.Vars[lang.VarKey(scopeID, varName)] = varType
		}
	}
}

func (Plugin) extractParams(params *sitter.Node, src []byte, funcID string, ir *lang.FileIR) map[string]string {
	res := make(map[string]string)
	if params == nil {
		return res
	}
	for i := 0; i < int(params.NamedChildCount()); i++ {
		p := params.NamedChild(i)
		var pName, pType string
		switch p.Type() {
		case "required_parameter", "optional_parameter":
			if nameNode := p.ChildByFieldName("pattern"); nameNode != nil {
				pName = lang.Text(nameNode, src)
			} else if nameNode := p.ChildByFieldName("name"); nameNode != nil {
				pName = lang.Text(nameNode, src)
			} else if p.NamedChildCount() > 0 {
				pName = lang.Text(p.NamedChild(0), src)
			}
			if typeNode := p.ChildByFieldName("type"); typeNode != nil {
				pType = cleanType(lang.Text(typeNode, src))
			} else if typeAnnot := lang.FirstChild(p, "type_annotation"); typeAnnot != nil {
				pType = cleanType(lang.Text(typeAnnot, src))
			}
		case "identifier":
			pName = lang.Text(p, src)
		}
		if pName != "" && pType != "" {
			res[pName] = pType
			ir.Types.Vars[lang.VarKey(funcID, pName)] = pType
		}
	}
	return res
}

func cleanType(txt string) string {
	txt = strings.TrimSpace(txt)
	txt = strings.TrimPrefix(txt, ":")
	return lang.BaseTypeName(txt)
}

func (Plugin) refs(root *sitter.Node, src []byte, ir *lang.FileIR) {
	byRange := lang.DeclIndex(ir)

	lang.Walk(root, func(n *sitter.Node) {
		var name, recv string
		switch n.Type() {
		case "call_expression":
			fn := n.ChildByFieldName("function")
			if fn == nil {
				return
			}
			switch fn.Type() {
			case "identifier":
				name = lang.Text(fn, src)
			case "member_expression":
				prop := fn.ChildByFieldName("property")
				obj := fn.ChildByFieldName("object")
				if prop != nil {
					name = lang.Text(prop, src)
				}
				if obj != nil {
					recv = lang.Text(obj, src)
				}
			}
		case "new_expression":
			ctor := n.ChildByFieldName("constructor")
			if ctor != nil {
				name = lang.BaseTypeName(lang.Text(ctor, src))
			}
		default:
			return
		}

		from := byRange(n)
		if name == "" || from == "" {
			return
		}
		ir.Refs = append(ir.Refs, lang.Ref{
			Kind:     store.EdgeCalls,
			Name:     name,
			Receiver: recv,
			FromID:   from,
			Line:     lang.Line(n),
		})
	})
}

func jsdoc(n *sitter.Node, src []byte) string {
	prev := n.PrevSibling()
	if prev == nil && n.Parent() != nil && n.Parent().Type() == "export_statement" {
		prev = n.Parent().PrevSibling()
	}
	if prev == nil || prev.Type() != "comment" {
		return ""
	}
	txt := lang.Text(prev, src)
	if !strings.HasPrefix(txt, "/**") {
		return ""
	}
	txt = strings.TrimPrefix(txt, "/**")
	txt = strings.TrimSuffix(txt, "*/")
	lines := strings.Split(txt, "\n")
	var cleaned []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		l = strings.TrimPrefix(l, "*")
		l = strings.TrimSpace(l)
		if l != "" {
			cleaned = append(cleaned, l)
		}
	}
	return strings.Join(cleaned, " ")
}

func signature(n *sitter.Node, src []byte) string {
	end := n.EndByte()
	if body := n.ChildByFieldName("body"); body != nil {
		end = body.StartByte()
	}
	return strings.TrimSpace(string(src[n.StartByte():end]))
}

func visibility(n *sitter.Node) store.Visibility {
	parent := n.Parent()
	if parent != nil && parent.Type() == "export_statement" {
		return store.VisExported
	}
	return store.VisPrivate
}

func memberVisibility(n *sitter.Node, name string, src []byte) store.Visibility {
	if strings.HasPrefix(name, "#") || strings.HasPrefix(name, "_") {
		return store.VisPrivate
	}
	for i := 0; i < int(n.NamedChildCount()); i++ {
		c := n.NamedChild(i)
		if c.Type() == "accessibility_modifier" {
			if lang.Text(c, src) == "private" {
				return store.VisPrivate
			}
		}
	}
	return store.VisExported
}

func testFile(path string) bool {
	base := filepath.Base(path)
	return strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.") ||
		strings.HasPrefix(base, "test_") ||
		strings.Contains(filepath.ToSlash(path), "/__tests__/") ||
		strings.Contains(filepath.ToSlash(path), "/tests/")
}
