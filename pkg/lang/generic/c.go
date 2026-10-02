package generic

import (
	"strings"

	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func init() {
	lang.Register(New(cSpec))
	lang.Register(New(cppSpec))
}

// notStatic treats a function as exported unless it is declared static, which
// in C and C++ limits it to its own file.
func notStatic(d Decl) bool {
	return !strings.Contains(childText(d, "storage_class_specifier"), "static")
}

// cSpec covers C. Headers (.h) are parsed as C; in a C++ project that loses
// the classes declared in them, but the functions are still found.
var cSpec = Spec{
	Name:       "c",
	Extensions: []string{".c", ".h"},
	Grammar:    c.GetLanguage,
	Types: map[string]store.NodeKind{
		"struct_specifier": store.KindClass,
		"union_specifier":  store.KindClass,
		"enum_specifier":   store.KindClass,
	},
	BodyRequired: map[string]bool{
		"struct_specifier": true,
		"union_specifier":  true,
		"enum_specifier":   true,
	},
	Functions: map[string]string{"function_definition": ""},
	Calls: map[string]Call{
		"call_expression": {Callee: "function"},
	},
	IsMain:     func(d Decl) bool { return d.Name == "main" },
	IsExported: notStatic,
}

var cppSpec = Spec{
	Name:       "cpp",
	Extensions: []string{".cc", ".cpp", ".cxx", ".hh", ".hpp", ".hxx"},
	Grammar:    cpp.GetLanguage,
	Types: map[string]store.NodeKind{
		"class_specifier":  store.KindClass,
		"struct_specifier": store.KindClass,
		"union_specifier":  store.KindClass,
		"enum_specifier":   store.KindClass,
	},
	BodyRequired: map[string]bool{
		"class_specifier":  true,
		"struct_specifier": true,
		"union_specifier":  true,
		"enum_specifier":   true,
	},
	Functions: map[string]string{"function_definition": ""},
	Calls: map[string]Call{
		"call_expression": {Callee: "function"},
		"new_expression":  {Name: "type"},
	},
	BaseNodes:  []string{"base_class_clause"},
	IsMain:     func(d Decl) bool { return d.Name == "main" && d.Type == "" },
	IsExported: notStatic,
}
