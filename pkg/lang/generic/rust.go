package generic

import (
	"strings"

	"github.com/smacker/go-tree-sitter/rust"
	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func init() { lang.Register(New(rustSpec)) }

var rustSpec = Spec{
	Name:       "rust",
	Extensions: []string{".rs"},
	Grammar:    rust.GetLanguage,
	Types: map[string]store.NodeKind{
		"struct_item": store.KindClass,
		"enum_item":   store.KindClass,
		"union_item":  store.KindClass,
		"trait_item":  store.KindInterface,
	},
	Scopes: map[string]Scope{
		"impl_item": {Type: "type", Trait: "trait"},
	},
	Functions: map[string]string{
		"function_item":           "",
		"function_signature_item": "",
	},
	Calls: map[string]Call{
		"call_expression": {Callee: "function"},
	},
	BaseFields: []string{"bounds"},
	IsTest: func(d Decl) bool {
		// #[test], #[tokio::test], #[rstest] and friends.
		return strings.Contains(precedingText(d, "attribute"), "test")
	},
	IsMain: func(d Decl) bool { return d.Name == "main" && d.Type == "" },
	IsExported: func(d Decl) bool {
		return strings.HasPrefix(childText(d, "visibility_modifier"), "pub")
	},
}
