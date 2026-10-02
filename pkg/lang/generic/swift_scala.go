package generic

import (
	"strings"

	"github.com/smacker/go-tree-sitter/scala"
	"github.com/smacker/go-tree-sitter/swift"
	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func init() {
	lang.Register(New(swiftSpec))
	lang.Register(New(scalaSpec))
}

func notPrivate(d Decl) bool {
	mods := modifiers(d)
	return !strings.Contains(mods, "private") && !strings.Contains(mods, "fileprivate")
}

// swiftSpec covers Swift. The grammar uses class_declaration for classes,
// structs, enums and extensions alike, so an extension's methods join the
// type it extends.
var swiftSpec = Spec{
	Name:       "swift",
	Extensions: []string{".swift"},
	Grammar:    swift.GetLanguage,
	Types: map[string]store.NodeKind{
		"class_declaration":    store.KindClass,
		"protocol_declaration": store.KindInterface,
	},
	Functions: map[string]string{
		"function_declaration":          "",
		"protocol_function_declaration": "",
		"init_declaration":              "<init>",
	},
	Calls: map[string]Call{
		"call_expression": {},
	},
	BaseNodes:   []string{"inheritance_specifier"},
	IsExtension: func(d Decl) bool { return hasKeyword(d, "extension") },
	IsTest: func(d Decl) bool {
		// XCTest: a method named test* on a class.
		return d.Type != "" && strings.HasPrefix(d.Name, "test")
	},
	IsMain:     func(d Decl) bool { return d.Name == "main" },
	IsExported: notPrivate,
}

var scalaSpec = Spec{
	Name:       "scala",
	Extensions: []string{".scala", ".sc"},
	Grammar:    scala.GetLanguage,
	Types: map[string]store.NodeKind{
		"class_definition":  store.KindClass,
		"object_definition": store.KindClass,
		"enum_definition":   store.KindClass,
		"trait_definition":  store.KindInterface,
	},
	Functions: map[string]string{
		"function_definition":  "",
		"function_declaration": "",
	},
	Calls: map[string]Call{
		"call_expression":     {Callee: "function"},
		"instance_expression": {},
	},
	BaseFields: []string{"extend"},
	BaseNodes:  []string{"extends_clause"},
	IsMain:     func(d Decl) bool { return d.Name == "main" },
	IsExported: notPrivate,
}
