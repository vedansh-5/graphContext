package mcp_server

import (
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func registerTaskContextTool(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("get_task_context",
		mcp.WithDescription("Task Context Engine: Analyzes a coding task or feature description, identifies relevant symbols, and constructs a bounded, ranked context pack."),
		mcp.WithString("project_path", mcp.Required(), mcp.Description("Absolute path of the project to analyze")),
		mcp.WithString("task", mcp.Required(), mcp.Description("Natural language description of the task or bug to investigate")),
		mcp.WithNumber("limit", mcp.Description("Maximum context nodes to return (default 30)")),
	), withSession(handleGetTaskContext))
}

var commonStopWords = map[string]bool{
	"the": true, "a": true, "an": true, "in": true, "on": true, "at": true,
	"to": true, "for": true, "of": true, "with": true, "by": true, "from": true,
	"and": true, "or": true, "is": true, "are": true, "was": true, "were": true,
	"fix": true, "bug": true, "issue": true, "add": true, "update": true, "implement": true,
}

func handleGetTaskContext(sess *session, projectPath string, args map[string]any) (*mcp.CallToolResult, error) {
	task := argString(args, "task")
	if task == "" {
		return mcp.NewToolResultError("task is required"), nil
	}

	limit := argInt(args, "limit", 30)
	if limit <= 0 {
		limit = 30
	}

	tokens := store.SplitIdentifier(task)
	seedScores := make(map[string]int)

	for _, token := range tokens {
		lower := strings.ToLower(token)
		if len(lower) < 2 || commonStopWords[lower] {
			continue
		}

		hits, _ := sess.store.Search(lower, 10)
		for _, hit := range hits {
			seedScores[hit.NodeID] += 3
		}

		for _, node := range sess.graph.Nodes {
			if strings.EqualFold(node.Name, lower) {
				seedScores[node.ID] += 5
			} else if strings.Contains(strings.ToLower(node.Name), lower) {
				seedScores[node.ID] += 2
			}
		}
	}

	type scoredSeed struct {
		id    string
		score int
	}

	var scoredSeeds []scoredSeed
	for id, score := range seedScores {
		if _, ok := sess.graph.Nodes[id]; ok {
			scoredSeeds = append(scoredSeeds, scoredSeed{id: id, score: score})
		}
	}

	sort.Slice(scoredSeeds, func(i, j int) bool {
		if scoredSeeds[i].score != scoredSeeds[j].score {
			return scoredSeeds[i].score > scoredSeeds[j].score
		}
		return scoredSeeds[i].id < scoredSeeds[j].id
	})

	maxSeeds := 5
	if len(scoredSeeds) < maxSeeds {
		maxSeeds = len(scoredSeeds)
	}

	contextNodeMap := make(map[string]store.Node)
	seedNodeList := make([]store.Node, 0, maxSeeds)

	for i := 0; i < maxSeeds; i++ {
		seedID := scoredSeeds[i].id
		if n, ok := sess.graph.Nodes[seedID]; ok {
			seedNodeList = append(seedNodeList, n)
			contextNodeMap[seedID] = n
		}

		sub := sess.graph.Neighborhood(seedID, 1, limit)
		for _, n := range sub.Nodes {
			if len(contextNodeMap) >= limit {
				break
			}
			contextNodeMap[n.ID] = n
		}
	}

	var contextNodes []store.Node
	fileSet := make(map[string]bool)

	for _, n := range contextNodeMap {
		contextNodes = append(contextNodes, n)
		if n.FilePath != "" {
			fileSet[n.FilePath] = true
		}
	}

	sort.Slice(contextNodes, func(i, j int) bool {
		return contextNodes[i].ID < contextNodes[j].ID
	})

	var relevantFiles []string
	for f := range fileSet {
		relevantFiles = append(relevantFiles, f)
	}
	sort.Strings(relevantFiles)

	var relationships []store.Edge
	for _, n := range contextNodes {
		for _, e := range sess.graph.Out[n.ID] {
			if _, ok := contextNodeMap[e.TargetID]; ok {
				relationships = append(relationships, e)
			}
		}
	}

	sort.Slice(relationships, func(i, j int) bool {
		if relationships[i].SourceID != relationships[j].SourceID {
			return relationships[i].SourceID < relationships[j].SourceID
		}
		return relationships[i].TargetID < relationships[j].TargetID
	})

	answer := map[string]any{
		"task":              task,
		"seeds":             seedNodeList,
		"context_nodes":     contextNodes,
		"relationships":     relationships,
		"relevant_files":    relevantFiles,
	}

	stats := map[string]any{
		"nodes_found": len(contextNodes),
		"edges_found": len(relationships),
		"files_found": len(relevantFiles),
		"truncated":   len(contextNodeMap) >= limit,
	}

	return toolJSON(Envelope{
		Answer: answer,
		Stats:  stats,
	}), nil
}
