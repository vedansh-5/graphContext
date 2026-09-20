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

	seen := make(map[string]bool)
	var matches []store.Node

	hits, _ := sess.store.Search(query, limit*2)
	for _, hit := range hits {
		if node, ok := sess.graph.Nodes[hit.NodeID]; ok {
			if kindFilter != "" && node.Kind != kindFilter {
				continue
			}
			if !seen[node.ID] {
				seen[node.ID] = true
				matches = append(matches, node)
			}
		}
	}

	if len(matches) < limit {
		queryLower := strings.ToLower(query)
		for _, node := range sess.graph.Nodes {
			if seen[node.ID] {
				continue
			}
			if kindFilter != "" && node.Kind != kindFilter {
				continue
			}
			if strings.Contains(strings.ToLower(node.Name), queryLower) ||
				strings.Contains(strings.ToLower(node.QualifiedName), queryLower) {
				seen[node.ID] = true
				matches = append(matches, node)
				if len(matches) >= limit {
					break
				}
			}
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].ID < matches[j].ID
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}

	return toolJSON(Envelope{
		Answer: map[string]any{
			"matches": matches,
		},
		Stats: map[string]any{
			"count": len(matches),
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

	collectNeighbors := func(edges []store.Edge, useSource bool, kind store.EdgeKind) []store.Node {
		var list []store.Node
		for _, e := range edges {
			if e.Kind != kind {
				continue
			}
			id := e.TargetID
			if useSource {
				id = e.SourceID
			}
			if n, ok := sess.graph.Nodes[id]; ok {
				list = append(list, n)
			}
		}
		sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
		return list
	}

	answer := map[string]any{
		"node":       node,
		"callers":    collectNeighbors(sess.graph.In[node.ID], true, store.EdgeCalls),
		"callees":    collectNeighbors(sess.graph.Out[node.ID], false, store.EdgeCalls),
		"bases":      collectNeighbors(sess.graph.Out[node.ID], false, store.EdgeInherits),
		"subclasses": collectNeighbors(sess.graph.In[node.ID], true, store.EdgeInherits),
	}

	if radius > 1 {
		subgraph := sess.graph.Neighborhood(node.ID, radius, 100)
		answer["subgraph"] = subgraph
	}

	if includeSource {
		answer["source"] = readSourceSpan(projectPath, node)
	}

	return toolJSON(Envelope{Answer: answer}), nil
}
