package mcp_server

import (
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func registerRepoOverviewTool(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("repo_overview",
		mcp.WithDescription("Repository-level architectural analysis: architecture map (Mermaid), circular dependencies, dead code candidates, and module coupling metrics."),
		mcp.WithString("project_path", mcp.Required(), mcp.Description("Absolute path of the project to analyze")),
		mcp.WithString("analysis", mcp.Description("Analysis type: all (default), architecture, cycles, dead_code, coupling")),
		mcp.WithString("level", mcp.Description("Granularity level: module (default) or file")),
		mcp.WithNumber("top", mcp.Description("Max items to return for ranked metrics (default 20)")),
	), withSession(handleRepoOverview))
}

type moduleCoupling struct {
	Name        string  `json:"name"`
	AfferentCa  int     `json:"afferent_ca"`
	EfferentCe  int     `json:"efferent_ce"`
	Instability float64 `json:"instability"`
}

func handleRepoOverview(sess *session, projectPath string, args map[string]any) (*mcp.CallToolResult, error) {
	analysisType := argString(args, "analysis")
	if analysisType == "" {
		analysisType = "all"
	}

	level := argString(args, "level")
	if level == "" {
		level = "module"
	}

	top := argInt(args, "top", 20)
	groupFn := func(n store.Node) string {
		return groupNodeByScope(level, n)
	}

	answer := make(map[string]any)
	var caveats []string

	if analysisType == "all" || analysisType == "architecture" {
		answer["architecture"] = sess.graph.Condense(groupFn)
	}

	if analysisType == "all" || analysisType == "cycles" {
		kinds := map[store.EdgeKind]bool{
			store.EdgeCalls:   true,
			store.EdgeImports: true,
		}
		rawSCCs := sess.graph.SCCs(kinds, groupFn)
		var multiNodeCycles [][]string
		for _, scc := range rawSCCs {
			if len(scc) > 1 {
				multiNodeCycles = append(multiNodeCycles, scc)
			}
		}
		answer["cycles"] = multiNodeCycles
	}

	if analysisType == "all" || analysisType == "dead_code" {
		rootCount, dead := sess.graph.DeadCandidates()
		if len(dead) > top && top > 0 {
			dead = dead[:top]
		}
		answer["dead_code"] = map[string]any{
			"root_count": rootCount,
			"candidates": dead,
		}
		caveats = append(caveats, "dead code candidates are statically unreachable; dynamic dispatch, reflection, or framework hooks may invoke them")
	}

	if analysisType == "all" || analysisType == "coupling" {
		arch := sess.graph.Condense(groupFn)
		inDegree := make(map[string]int)
		outDegree := make(map[string]int)

		for _, edge := range arch.Edges {
			outDegree[edge.From] += edge.Weight
			inDegree[edge.To] += edge.Weight
		}

		var metrics []moduleCoupling
		for _, mod := range arch.Modules {
			ca := inDegree[mod]
			ce := outDegree[mod]
			instability := 0.0
			if ca+ce > 0 {
				instability = float64(ce) / float64(ca+ce)
			}
			metrics = append(metrics, moduleCoupling{
				Name:        mod,
				AfferentCa:  ca,
				EfferentCe:  ce,
				Instability: instability,
			})
		}

		sort.Slice(metrics, func(i, j int) bool {
			if metrics[i].EfferentCe+metrics[i].AfferentCa != metrics[j].EfferentCe+metrics[j].AfferentCa {
				return (metrics[i].EfferentCe + metrics[i].AfferentCa) > (metrics[j].EfferentCe + metrics[j].AfferentCa)
			}
			return metrics[i].Name < metrics[j].Name
		})

		if len(metrics) > top && top > 0 {
			metrics = metrics[:top]
		}
		answer["coupling"] = metrics
	}

	return toolJSON(Envelope{
		Answer:  answer,
		Caveats: caveats,
		Stats: map[string]any{
			"total_nodes": len(sess.graph.Nodes),
		},
	}), nil
}
