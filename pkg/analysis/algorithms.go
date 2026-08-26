package analysis

import (
	"sort"

	"github.com/vedansh-5/graphcontext/pkg/store"
)

type Reached struct {
	Node       store.Node       `json:"node"`
	Depth      int              `json:"depth"`
	Confidence store.Confidence `json:"confidence"`
}

func (g *Graph) ReverseReach(seedID string, kinds map[store.EdgeKind]bool, maxDepth, limit int) (results []Reached, truncated bool) {
	visited := map[string]bool{seedID: true}
	frontier := []string{seedID}

	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		var next []string
		for _, id := range frontier {
			for _, e := range g.In[id] {
				if !kinds[e.Kind] || visited[e.SourceID] {
					continue
				}
				visited[e.SourceID] = true
				if len(results) >= limit {
					return results, true
				}
				if n, ok := g.Nodes[e.SourceID]; ok {
					results = append(results, Reached{Node: n, Depth: depth, Confidence: e.Confidence})
				}
				next = append(next, e.SourceID)
			}
		}
		frontier = next
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Depth != results[j].Depth {
			return results[i].Depth < results[j].Depth
		}
		return results[i].Node.ID < results[j].Node.ID
	})
	return results, false
}

func (g *Graph) ForwardReach(seedID string, kinds map[store.EdgeKind]bool, maxDepth, limit int) (results []Reached, truncated bool) {
	visited := map[string]bool{seedID: true}
	frontier := []string{seedID}

	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		var next []string
		for _, id := range frontier {
			for _, e := range g.Out[id] {
				if !kinds[e.Kind] || visited[e.TargetID] {
					continue
				}
				visited[e.TargetID] = true
				if len(results) >= limit {
					return results, true
				}
				if n, ok := g.Nodes[e.TargetID]; ok {
					results = append(results, Reached{Node: n, Depth: depth, Confidence: e.Confidence})
				}
				next = append(next, e.TargetID)
			}
		}
		frontier = next
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Depth != results[j].Depth {
			return results[i].Depth < results[j].Depth
		}
		return results[i].Node.ID < results[j].Node.ID
	})
	return results, false
}

type TraceNode struct {
	ID       string         `json:"id"`
	Name     string         `json:"name,omitempty"`
	Kind     store.NodeKind `json:"kind,omitempty"`
	File     string         `json:"file,omitempty"`
	CallLine int            `json:"call_line,omitempty"`
	Cycle    bool           `json:"cycle,omitempty"`
	Calls    []*TraceNode   `json:"calls,omitempty"`
}

func (g *Graph) Trace(entryID string, maxDepth, limit int) (root *TraceNode, truncated bool) {
	budget := limit

	var walk func(id string, depth, callLine int, path map[string]bool) *TraceNode
	walk = func(id string, depth, callLine int, path map[string]bool) *TraceNode {
		t := &TraceNode{ID: id, CallLine: callLine}
		if n, ok := g.Nodes[id]; ok {
			t.Name = n.Name
			t.Kind = n.Kind
			t.File = n.FilePath
		}
		if path[id] {
			t.Cycle = true
			return t
		}
		if depth >= maxDepth {
			return t
		}

		path[id] = true
		out := append([]store.Edge(nil), g.Out[id]...)
		sort.Slice(out, func(i, j int) bool {
			if out[i].Line != out[j].Line {
				return out[i].Line < out[j].Line
			}
			return out[i].TargetID < out[j].TargetID
		})

		for _, e := range out {
			if e.Kind != store.EdgeCalls {
				continue
			}
			if budget <= 0 {
				truncated = true
				break
			}
			budget--
			t.Calls = append(t.Calls, walk(e.TargetID, depth+1, e.Line, path))
		}
		delete(path, id)

		return t
	}

	return walk(entryID, 0, 0, map[string]bool{}), truncated
}

type NeighborhoodResult struct {
	Nodes     []store.Node `json:"nodes"`
	Edges     []store.Edge `json:"edges"`
	Truncated bool         `json:"truncated"`
}

func (g *Graph) Neighborhood(seedID string, radius int, limit int) NeighborhoodResult {
	visited := map[string]bool{seedID: true}
	frontier := []string{seedID}
	var nodeIDs []string
	if _, ok := g.Nodes[seedID]; ok {
		nodeIDs = append(nodeIDs, seedID)
	}

	for r := 0; r < radius && len(frontier) > 0; r++ {
		var next []string
		for _, id := range frontier {
			for _, e := range g.Out[id] {
				if !visited[e.TargetID] {
					visited[e.TargetID] = true
					nodeIDs = append(nodeIDs, e.TargetID)
					next = append(next, e.TargetID)
				}
			}
			for _, e := range g.In[id] {
				if !visited[e.SourceID] {
					visited[e.SourceID] = true
					nodeIDs = append(nodeIDs, e.SourceID)
					next = append(next, e.SourceID)
				}
			}
		}
		frontier = next
	}

	var resNodes []store.Node
	truncated := false
	for _, id := range nodeIDs {
		if len(resNodes) >= limit {
			truncated = true
			break
		}
		if n, ok := g.Nodes[id]; ok {
			resNodes = append(resNodes, n)
		}
	}

	subgraphNodeSet := make(map[string]bool, len(resNodes))
	for _, n := range resNodes {
		subgraphNodeSet[n.ID] = true
	}

	var resEdges []store.Edge
	for _, n := range resNodes {
		for _, e := range g.Out[n.ID] {
			if subgraphNodeSet[e.TargetID] {
				resEdges = append(resEdges, e)
			}
		}
	}

	sort.Slice(resNodes, func(i, j int) bool { return resNodes[i].ID < resNodes[j].ID })
	sort.Slice(resEdges, func(i, j int) bool {
		if resEdges[i].SourceID != resEdges[j].SourceID {
			return resEdges[i].SourceID < resEdges[j].SourceID
		}
		if resEdges[i].TargetID != resEdges[j].TargetID {
			return resEdges[i].TargetID < resEdges[j].TargetID
		}
		return resEdges[i].Line < resEdges[j].Line
	})

	return NeighborhoodResult{
		Nodes:     resNodes,
		Edges:     resEdges,
		Truncated: truncated,
	}
}
