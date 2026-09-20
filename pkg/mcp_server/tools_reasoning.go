package mcp_server

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func registerReasoningTools(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("impact_of_change",
		mcp.WithDescription("Calculates the blast radius of modifying or deleting a symbol: transitive callers, affected test suites, and dangling references."),
		mcp.WithString("project_path", mcp.Required(), mcp.Description("Absolute path of the project to analyze")),
		mcp.WithString("symbol", mcp.Required(), mcp.Description("Exact node ID or symbol name")),
		mcp.WithString("change_type", mcp.Description("Type of change: modify (default), delete, or change_signature")),
		mcp.WithNumber("max_depth", mcp.Description("Traversal depth (default 5)")),
		mcp.WithNumber("limit", mcp.Description("Maximum affected nodes to return (default 100)")),
	), withSession(handleImpactOfChange))

	s.AddTool(mcp.NewTool("trace",
		mcp.WithDescription("Traces execution flow from an entrypoint as a call-tree, or computes call paths between two symbols."),
		mcp.WithString("project_path", mcp.Required(), mcp.Description("Absolute path of the project to analyze")),
		mcp.WithString("from", mcp.Required(), mcp.Description("Starting symbol name or node ID")),
		mcp.WithString("to", mcp.Description("Optional destination symbol to find execution paths between from and to")),
		mcp.WithString("direction", mcp.Description("Traversal direction when to is omitted: forward (default) or backward")),
		mcp.WithNumber("max_depth", mcp.Description("Maximum depth for call tree or path search (default 6)")),
		mcp.WithNumber("limit", mcp.Description("Maximum nodes/paths to return (default 200)")),
	), withSession(handleTrace))
}

func handleImpactOfChange(sess *session, projectPath string, args map[string]any) (*mcp.CallToolResult, error) {
	symbol := argString(args, "symbol")
	if symbol == "" {
		return mcp.NewToolResultError("symbol is required"), nil
	}

	node, errRes := resolveSymbol(sess, symbol)
	if errRes != nil {
		return errRes, nil
	}

	changeType := argString(args, "change_type")
	if changeType == "" {
		changeType = "modify"
	}

	maxDepth := argInt(args, "max_depth", 5)
	limit := argInt(args, "limit", 100)

	kinds := map[store.EdgeKind]bool{
		store.EdgeCalls:    true,
		store.EdgeInherits: true,
	}

	results, truncated := sess.graph.ReverseReach(node.ID, kinds, maxDepth, limit)

	byFile := make(map[string][]map[string]any)
	var tests []string
	var entrypoints []string
	unresolvedCount := 0

	for _, r := range results {
		if r.Confidence != store.ConfExact {
			unresolvedCount++
		}

		byFile[r.Node.FilePath] = append(byFile[r.Node.FilePath], map[string]any{
			"id":         r.Node.ID,
			"name":       r.Node.Name,
			"kind":       r.Node.Kind,
			"line":       r.Node.StartLine,
			"depth":      r.Depth,
			"confidence": r.Confidence,
		})

		if r.Node.IsTest || strings.Contains(r.Node.FilePath, "test") || strings.HasPrefix(r.Node.Name, "Test") {
			tests = append(tests, r.Node.ID)
		}
		if r.Node.IsEntrypoint || r.Node.Name == "main" {
			entrypoints = append(entrypoints, r.Node.ID)
		}
	}

	answer := map[string]any{
		"target":           node.ID,
		"change_type":      changeType,
		"total_affected":   len(results),
		"affected_by_file": byFile,
		"affected_tests":   tests,
		"entrypoints":      entrypoints,
	}

	if changeType == "delete" {
		var dangling []map[string]any
		for _, e := range sess.graph.In[node.ID] {
			if e.Kind == store.EdgeContains {
				continue
			}
			dangling = append(dangling, map[string]any{
				"source_id":  e.SourceID,
				"kind":       e.Kind,
				"line":       e.Line,
				"confidence": e.Confidence,
			})
		}
		answer["dangling_references"] = dangling
	}

	var caveats []string
	if unresolvedCount > 0 {
		caveats = append(caveats, fmt.Sprintf("%d references matched via name or heuristic resolution", unresolvedCount))
	}

	stats := map[string]any{
		"nodes_evaluated": len(sess.graph.Nodes),
		"truncated":       truncated,
	}

	return toolJSON(Envelope{
		Answer:  answer,
		Caveats: caveats,
		Stats:   stats,
	}), nil
}

func handleTrace(sess *session, projectPath string, args map[string]any) (*mcp.CallToolResult, error) {
	fromSym := argString(args, "from")
	if fromSym == "" {
		return mcp.NewToolResultError("from is required"), nil
	}

	fromNode, errRes := resolveSymbol(sess, fromSym)
	if errRes != nil {
		return errRes, nil
	}

	toSym := argString(args, "to")
	maxDepth := argInt(args, "max_depth", 6)
	limit := argInt(args, "limit", 200)

	if toSym != "" {
		toNode, errRes := resolveSymbol(sess, toSym)
		if errRes != nil {
			return errRes, nil
		}

		rawPaths, truncated := sess.graph.PathsBetween(fromNode.ID, toNode.ID, maxDepth, limit)
		type pathHop struct {
			ID   string         `json:"id"`
			Name string         `json:"name"`
			Kind store.NodeKind `json:"kind"`
			File string         `json:"file"`
			Line int            `json:"line"`
		}

		var renderedPaths [][]pathHop
		for _, raw := range rawPaths {
			var hops []pathHop
			for _, id := range raw {
				hop := pathHop{ID: id}
				if n, ok := sess.graph.Nodes[id]; ok {
					hop.Name = n.Name
					hop.Kind = n.Kind
					hop.File = n.FilePath
					hop.Line = n.StartLine
				}
				hops = append(hops, hop)
			}
			renderedPaths = append(renderedPaths, hops)
		}

		var caveats []string
		if len(renderedPaths) == 0 {
			caveats = append(caveats, "no static path found; execution may occur via dynamic dispatch or reflection")
		}

		return toolJSON(Envelope{
			Answer: map[string]any{
				"from":  fromNode.ID,
				"to":    toNode.ID,
				"paths": renderedPaths,
			},
			Caveats: caveats,
			Stats: map[string]any{
				"paths_count": len(renderedPaths),
				"truncated":   truncated,
			},
		}), nil
	}

	direction := argString(args, "direction")
	if direction == "backward" {
		kinds := map[store.EdgeKind]bool{store.EdgeCalls: true}
		results, truncated := sess.graph.ReverseReach(fromNode.ID, kinds, maxDepth, limit)
		return toolJSON(Envelope{
			Answer: map[string]any{
				"entry":     fromNode.ID,
				"direction": "backward",
				"callers":   results,
			},
			Stats: map[string]any{
				"count":     len(results),
				"truncated": truncated,
			},
		}), nil
	}

	tree, truncated := sess.graph.Trace(fromNode.ID, maxDepth, limit)
	return toolJSON(Envelope{
		Answer: map[string]any{
			"entry":     fromNode.ID,
			"direction": "forward",
			"call_tree": tree,
		},
		Stats: map[string]any{
			"truncated": truncated,
		},
	}), nil
}
