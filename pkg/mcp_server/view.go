package mcp_server

import (
	"sort"
	"strings"

	"github.com/vedansh-5/graphcontext/pkg/store"
)

// Tool answers are read by a model, so every byte costs context. These views
// carry what an agent acts on (where a symbol is and what it looks like) and
// leave out what it can derive or never uses: byte offsets, the name already
// present in the ID, default values.

// nodeView is the compact form of a node in a tool answer.
type nodeView struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	File       string `json:"file,omitempty"`
	Line       int    `json:"line,omitempty"`
	EndLine    int    `json:"end_line,omitempty"`
	Signature  string `json:"signature,omitempty"`
	Doc        string `json:"doc,omitempty"`
	Test       bool   `json:"test,omitempty"`
	Entrypoint string `json:"entrypoint,omitempty"`
}

// maxDocLen caps the docstring summary carried in a nodeView.
const maxDocLen = 120

func viewNode(n store.Node) nodeView {
	v := nodeView{
		ID:         n.ID,
		Kind:       string(n.Kind),
		File:       n.FilePath,
		Line:       n.StartLine,
		Signature:  n.Signature,
		Doc:        docSummary(n.Docstring),
		Test:       n.IsTest,
		Entrypoint: string(n.EntrypointKind),
	}
	if n.EndLine > n.StartLine {
		v.EndLine = n.EndLine
	}
	return v
}

func viewNodes(nodes []store.Node) []nodeView {
	out := make([]nodeView, len(nodes))
	for i, n := range nodes {
		out[i] = viewNode(n)
	}
	return out
}

// docSummary is the first line of a docstring, capped.
func docSummary(doc string) string {
	doc = strings.TrimSpace(doc)
	if i := strings.IndexByte(doc, '\n'); i >= 0 {
		doc = strings.TrimSpace(doc[:i])
	}
	if len(doc) > maxDocLen {
		doc = doc[:maxDocLen] + "..."
	}
	return doc
}

// refView is a neighbour of a symbol: just enough to name it and judge how
// much to trust the link. Callers and callees are listed this way because an
// agent that wants more about one of them asks for it by ID.
type refView struct {
	ID         string `json:"id"`
	Line       int    `json:"line,omitempty"`
	Confidence string `json:"confidence,omitempty"`
}

// edgeView is the compact form of an edge in a tool answer.
type edgeView struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Kind       string `json:"kind"`
	Line       int    `json:"line,omitempty"`
	Confidence string `json:"confidence,omitempty"`
}

// viewConfidence leaves out "exact", the common case, so only the edges an
// agent should be careful with are marked.
func viewConfidence(c store.Confidence) string {
	if c == store.ConfExact {
		return ""
	}
	return string(c)
}

func viewEdges(edges []store.Edge) []edgeView {
	out := make([]edgeView, len(edges))
	for i, e := range edges {
		out[i] = edgeView{
			From: e.SourceID, To: e.TargetID, Kind: string(e.Kind),
			Line: e.Line, Confidence: viewConfidence(e.Confidence),
		}
	}
	return out
}

// neighbours lists the nodes linked to a symbol by edges of one kind, sorted
// by ID. incoming selects the edge's source instead of its target.
func neighbours(edges []store.Edge, incoming bool, kind store.EdgeKind) []refView {
	out := []refView{}
	seen := map[string]bool{}
	for _, e := range edges {
		if e.Kind != kind {
			continue
		}
		id := e.TargetID
		if incoming {
			id = e.SourceID
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, refView{ID: id, Line: e.Line, Confidence: viewConfidence(e.Confidence)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
