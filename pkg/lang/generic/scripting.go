package generic

import (
	"strings"

	"github.com/smacker/go-tree-sitter/kotlin"
	"github.com/smacker/go-tree-sitter/php"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func init() {
	lang.Register(New(rubySpec))
	lang.Register(New(phpSpec))
	lang.Register(New(kotlinSpec))
}

// rubySpec covers Ruby. A bare "foo" with no arguments or parentheses parses
// as a plain identifier, so such calls are not recorded.
var rubySpec = Spec{
	Name:       "ruby",
	Extensions: []string{".rb", ".rake"},
	Grammar:    ruby.GetLanguage,
	Types: map[string]store.NodeKind{
		"class":  store.KindClass,
		"module": store.KindClass,
	},
	Functions: map[string]string{
		"method":           "",
		"singleton_method": "",
	},
	Calls: map[string]Call{
		"call": {Name: "method", Receiver: "receiver"},
	},
	BaseFields: []string{"superclass"},
	IsTest: func(d Decl) bool {
		return strings.HasPrefix(d.Name, "test_")
	},
	IsExported: func(d Decl) bool { return !strings.HasPrefix(d.Name, "_") },
}

var phpSpec = Spec{
	Name:       "php",
	Extensions: []string{".php"},
	Grammar:    php.GetLanguage,
	Types: map[string]store.NodeKind{
		"class_declaration":     store.KindClass,
		"trait_declaration":     store.KindClass,
		"enum_declaration":      store.KindClass,
		"interface_declaration": store.KindInterface,
	},
	Functions: map[string]string{
		"function_definition": "",
		"method_declaration":  "",
	},
	Calls: map[string]Call{
		"function_call_expression":   {Callee: "function"},
		"member_call_expression":     {Name: "name", Receiver: "object"},
		"scoped_call_expression":     {Name: "name", Receiver: "scope"},
		"object_creation_expression": {},
	},
	BaseNodes: []string{"base_clause", "class_interface_clause"},
	IsTest: func(d Decl) bool {
		// PHPUnit: a method named test* on a class named *Test.
		return strings.HasPrefix(d.Name, "test") && strings.HasSuffix(d.Type, "Test")
	},
	IsExported: func(d Decl) bool {
		mods := childText(d, "visibility_modifier")
		return !strings.Contains(mods, "private") && !strings.Contains(mods, "protected")
	},
}

var kotlinSpec = Spec{
	Name:       "kotlin",
	Extensions: []string{".kt", ".kts"},
	Grammar:    kotlin.GetLanguage,
	Types: map[string]store.NodeKind{
		"class_declaration":  store.KindClass,
		"object_declaration": store.KindClass,
	},
	Functions: map[string]string{"function_declaration": ""},
	Calls: map[string]Call{
		"call_expression": {},
	},
	BaseNodes: []string{"delegation_specifier"},
	IsTest: func(d Decl) bool {
		return strings.Contains(modifiers(d), "@Test")
	},
	IsMain: func(d Decl) bool { return d.Name == "main" && d.Type == "" },
	IsExported: func(d Decl) bool {
		mods := modifiers(d)
		return !strings.Contains(mods, "private") && !strings.Contains(mods, "internal")
	},
}
