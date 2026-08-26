package store

import "time"

// NodeKind enumerates every kind of entity the graph can hold.
type NodeKind string

const (
	KindFile      NodeKind = "file"
	KindModule    NodeKind = "module"
	KindClass     NodeKind = "class"
	KindInterface NodeKind = "interface"
	KindFunction  NodeKind = "function"
	KindMethod    NodeKind = "method"
	KindPackage   NodeKind = "package"
	KindExternal  NodeKind = "external"
)

// EdgeKind enumerates every relationship the graph can hold.
type EdgeKind string

const (
	EdgeContains   EdgeKind = "contains"
	EdgeImports    EdgeKind = "imports"
	EdgeCalls      EdgeKind = "calls"
	EdgeInherits   EdgeKind = "inherits"
	EdgeImplements EdgeKind = "implements"
	EdgeOverrides  EdgeKind = "overrides"
	EdgeReferences EdgeKind = "references"
	EdgeDecorates  EdgeKind = "decorates"
	EdgeDependsOn  EdgeKind = "depends_on"
)

// Confidence records how sure the resolver is about an edge's target.
type Confidence string

const (
	// ConfExact means the import map or type propagation found exactly one target.
	ConfExact Confidence = "exact"
	// ConfAmbiguous means scope narrowed the target to k candidates; all k are emitted.
	ConfAmbiguous Confidence = "ambiguous"
	// ConfNameMatch means only a bare global name match was possible.
	ConfNameMatch Confidence = "name_match"
	// ConfUnknown means the target could not be resolved at all.
	ConfUnknown Confidence = "unknown"
)

// EntrypointKind classifies why a node is an entry point.
type EntrypointKind string

const (
	EntryNone    EntrypointKind = ""
	EntryMain    EntrypointKind = "main"
	EntryRoute   EntrypointKind = "route"
	EntryCLI     EntrypointKind = "cli"
	EntryHandler EntrypointKind = "handler"
	EntryFixture EntrypointKind = "fixture"
)

// Visibility records whether a symbol is externally reachable.
type Visibility string

const (
	VisExported Visibility = "exported"
	VisPrivate  Visibility = "private"
)

// Node is one entity in the graph. ID is "<file_path>:<qualified_name>".
type Node struct {
	ID             string
	Kind           NodeKind
	Name           string
	QualifiedName  string
	FilePath       string
	StartLine      int
	EndLine        int
	StartByte      int
	EndByte        int
	Language       string
	Signature      string
	Docstring      string
	IsTest         bool
	IsEntrypoint   bool
	EntrypointKind EntrypointKind
	Visibility     Visibility
}

// Edge is one directed relationship between two nodes.
type Edge struct {
	SourceID       string
	TargetID       string
	Kind           EdgeKind
	Line           int
	Confidence     Confidence
	CandidateCount int
}

// FileRecord tracks a file's content hash so re-indexing can skip unchanged files.
type FileRecord struct {
	Path        string
	ContentHash string
	IndexedAt   time.Time
}
