package mcp_server

import (
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func registerOrientationTools(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("search_symbols",
		mcp.WithDescription("Find functions, classes, methods, or files matching a query. Entrypoint to obtain exact node IDs."),
		mcp.WithString("project_path", mcp.Required(), mcp.Description("Absolute path of the project to analyze")),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search terms or partial symbol name")),
		mcp.WithString("kind", mcp.Description("Optional node kind filter: function, method, class, interface, file")),
		mcp.WithNumber("limit", mcp.Description("Maximum matches to return (default 20)")),
	), withSession(handleSearchSymbols))

	s.AddTool(mcp.NewTool("get_context",
		mcp.WithDescription("Context pack for a symbol: definition site, callers, callees, inheritance hierarchy, and source snippet."),
		mcp.WithString("project_path", mcp.Required(), mcp.Description("Absolute path of the project to analyze")),
		mcp.WithString("symbol", mcp.Required(), mcp.Description("Exact node ID or symbol name")),
		mcp.WithNumber("radius", mcp.Description("Neighborhood expansion radius (default 1)")),
		mcp.WithBoolean("include_source", mcp.Description("Whether to include the raw source code of the symbol")),
	), withSession(handleGetContext))
}

func handleSearchSymbols(sess *session, projectPath string, args map[string]any) (*mcp.CallToolResult, error) {
	query := argString(args, "query")
	if query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	kindFilter := store.NodeKind(argString(args, "kind"))
	limit := argInt(args, "limit", 20)

	// rank orders matches: an exact name first, then a name that starts with
	// the query, then full-text hits in relevance order, then substrings.
	// Lower is better.
	queryLower := strings.ToLower(query)
	type match struct {
		node store.Node
		rank int
	}
	seen := make(map[string]bool)
	var matches []match
	add := func(node store.Node, rank int) {
		if seen[node.ID] || node.Kind == store.KindExternal {
			return
		}
		if kindFilter != "" && node.Kind != kindFilter {
			return
		}
		name := strings.ToLower(node.Name)
		switch {
		case name == queryLower || strings.ToLower(node.QualifiedName) == queryLower:
			rank = 0
		case strings.HasPrefix(name, queryLower):
			rank = 1
		}
		seen[node.ID] = true
		matches = append(matches, match{node, rank})
	}

	hits, _ := sess.store.Search(query, limit*2)
	for i, hit := range hits {
		if node, ok := sess.graph.Nodes[hit.NodeID]; ok {
			add(node, 2+i)
		}
	}
	substringRank := 2 + len(hits)
	for _, node := range sess.graph.Nodes {
		if strings.Contains(strings.ToLower(node.Name), queryLower) ||
			strings.Contains(strings.ToLower(node.QualifiedName), queryLower) {
			add(node, substringRank)
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		// Code before the tests that exercise it.
		if a.node.IsTest != b.node.IsTest {
			return !a.node.IsTest
		}
		return a.node.ID < b.node.ID
	})

	total := len(matches)
	if len(matches) > limit {
		matches = matches[:limit]
	}
	views := make([]nodeView, len(matches))
	for i, m := range matches {
		views[i] = viewNode(m.node)
	}

	return toolJSON(Envelope{
		Answer: map[string]any{
			"matches": views,
		},
		Stats: map[string]any{
			"count":     len(views),
			"truncated": total > len(views),
		},
	}), nil
}

func handleGetContext(sess *session, projectPath string, args map[string]any) (*mcp.CallToolResult, error) {
	symbol := argString(args, "symbol")
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}

	node, errRes := resolveSymbol(sess, symbol)
	if errRes != nil {
		return errRes, nil
	}

	radius := argInt(args, "radius", 1)
	includeSource := argBool(args, "include_source")

	in, out := sess.graph.In[node.ID], sess.graph.Out[node.ID]
	answer := map[string]any{
		"node":       viewNode(node),
		"callers":    neighbours(in, true, store.EdgeCalls),
		"callees":    neighbours(out, false, store.EdgeCalls),
		"bases":      neighbours(out, false, store.EdgeInherits),
		"subclasses": neighbours(in, true, store.EdgeInherits),
	}

	if radius > 1 {
		subgraph := sess.graph.Neighborhood(node.ID, radius, 100)
		answer["subgraph"] = map[string]any{
			"nodes":     viewNodes(subgraph.Nodes),
			"edges":     viewEdges(subgraph.Edges),
			"truncated": subgraph.Truncated,
		}
	}

	if includeSource {
		answer["source"] = readSourceSpan(projectPath, node)
	}

	return toolJSON(Envelope{Answer: answer}), nil
}
