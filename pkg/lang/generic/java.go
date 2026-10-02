package generic

import (
	"strings"

	"github.com/smacker/go-tree-sitter/java"
	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func init() { lang.Register(New(javaSpec)) }

var javaSpec = Spec{
	Name:       "java",
	Extensions: []string{".java"},
	Grammar:    java.GetLanguage,
	Types: map[string]store.NodeKind{
		"class_declaration":     store.KindClass,
		"enum_declaration":      store.KindClass,
		"record_declaration":    store.KindClass,
		"interface_declaration": store.KindInterface,
	},
	Functions: map[string]string{
		"method_declaration":      "",
		"constructor_declaration": "<init>",
	},
	Calls: map[string]Call{
		"method_invocation":          {Name: "name", Receiver: "object"},
		"object_creation_expression": {Name: "type"},
	},
	BaseFields: []string{"superclass", "interfaces"},
	BaseNodes:  []string{"extends_interfaces"},
	IsTest: func(d Decl) bool {
		mods := modifiers(d)
		return strings.Contains(mods, "@Test") || strings.Contains(mods, "@ParameterizedTest")
	},
	IsMain: func(d Decl) bool {
		return d.Name == "main" && strings.Contains(modifiers(d), "static")
	},
	IsExported: func(d Decl) bool {
		return strings.Contains(modifiers(d), "public")
	},
}
