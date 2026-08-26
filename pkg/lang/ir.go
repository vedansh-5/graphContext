// Package lang defines the language-neutral intermediate form produced by pass 1
// of indexing, and the plugin interface each language implements.
//
// Pass 1 is pure: a plugin sees one file's bytes and nothing else. It records
// what it saw, never what it concludes. Turning references into edges is the
// resolver's job (pass 2), because that requires knowledge of other files.
package lang

import "github.com/vedansh-5/graphcontext/pkg/store"

// FileIR is everything pass 1 extracts from a single file.
type FileIR struct {
	Path     string
	Language string
	// Nodes are declarations found in this file, in source order.
	Nodes []store.Node
	// Imports are import statements exactly as written.
	Imports []ImportRef
	// Refs are unresolved references: calls, inheritance, decorators.
	Refs []Ref
	// Types holds the syntactically visible type information the resolver uses
	// to turn a reference like "r.db.Save" into a concrete target.
	Types TypeFacts
}

// ImportRef is one import statement, unresolved.
//
// The Path is the module string exactly as written — "fmt", "./utils",
// "auth.services". Mapping that to a file is per-language and happens in pass 2.
type ImportRef struct {
	Path string
	// Alias is the local binding for the whole module ("import numpy as np" -> "np").
	Alias string
	// Names are individually imported symbols ("from x import a, b" -> [a b]).
	// Empty means the whole module was imported.
	Names []string
	// Wildcard marks "from x import *" or "export * from" re-exports.
	Wildcard bool
	Line     int
}

// Ref is a reference from one declaration to a name defined elsewhere.
//
// Receiver carries the expression text to the left of the final dot, verbatim:
// "r.db" in r.db.Save(), "self" in self.save(), "" for a bare save(). The
// resolver combines Receiver with TypeFacts to decide what Name actually means.
type Ref struct {
	Kind store.EdgeKind
	// Name is the referenced identifier, without any receiver.
	Name string
	// Receiver is the expression left of the final dot, or "" for a bare name.
	Receiver string
	// FromID is the node ID of the enclosing declaration.
	FromID string
	Line   int
}

// TypeFacts is the syntactically visible type information in one file.
//
// Nothing here is inferred. Every entry comes from an explicit annotation, a
// declared field, a constructor call, or a method receiver. Where a language
// does not make the type visible, the entry is simply absent and the resolver
// falls back to scope narrowing.
type TypeFacts struct {
	// Vars maps a local binding to its type, keyed by VarKey(enclosingNodeID, name).
	Vars map[string]string
	// Fields maps "<TypeName>.<fieldName>" to the field's type.
	Fields map[string]string
	// Methods maps a type name to its method names, sorted. Used to compute
	// interface satisfaction structurally, which is how Go interfaces work.
	Methods map[string][]string
	// Bases maps a type name to the types it extends or implements by name.
	Bases map[string][]string
}

// NewTypeFacts returns TypeFacts with every map initialised.
func NewTypeFacts() TypeFacts {
	return TypeFacts{
		Vars:    map[string]string{},
		Fields:  map[string]string{},
		Methods: map[string][]string{},
		Bases:   map[string][]string{},
	}
}

// VarKey builds the composite key for TypeFacts.Vars. Variable names are only
// meaningful inside their enclosing declaration, so the scope is part of the key.
func VarKey(enclosingNodeID, varName string) string {
	return enclosingNodeID + "\x00" + varName
}
