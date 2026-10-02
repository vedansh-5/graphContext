package generic

import (
	"regexp"
	"strings"

	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func init() { lang.Register(New(csharpSpec)) }

// csharpTestAttr matches the test attributes of NUnit, xUnit and MSTest.
var csharpTestAttr = regexp.MustCompile(`\[\s*(Test|TestCase|Fact|Theory|TestMethod)\b`)

var csharpSpec = Spec{
	Name:       "csharp",
	Extensions: []string{".cs"},
	Grammar:    csharp.GetLanguage,
	Types: map[string]store.NodeKind{
		"class_declaration":     store.KindClass,
		"struct_declaration":    store.KindClass,
		"record_declaration":    store.KindClass,
		"enum_declaration":      store.KindClass,
		"interface_declaration": store.KindInterface,
	},
	Functions: map[string]string{
		"method_declaration":       "",
		"local_function_statement": "",
		"constructor_declaration":  "<init>",
	},
	Calls: map[string]Call{
		"invocation_expression":      {Callee: "function"},
		"object_creation_expression": {Name: "type"},
	},
	BaseFields: []string{"bases"},
	BaseNodes:  []string{"base_list"},
	IsTest: func(d Decl) bool {
		return csharpTestAttr.MatchString(childText(d, "attribute_list"))
	},
	IsMain: func(d Decl) bool {
		return d.Name == "Main" && strings.Contains(modifiers(d), "static")
	},
	IsExported: func(d Decl) bool {
		return strings.Contains(modifiers(d), "public")
	},
}
