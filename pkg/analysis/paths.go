package analysis

import (
	"sort"
)

func (g *Graph) PathsBetween(fromID, toID string, maxDepth, limit int) (paths [][]string, truncated bool) {
	var current []string
	visited := make(map[string]bool)

	var dfs func(curr string, depth int)
	dfs = func(curr string, depth int) {
		if len(paths) >= limit {
			truncated = true
			return
		}
		current = append(current, curr)
		visited[curr] = true

		if curr == toID {
			p := make([]string, len(current))
			copy(p, current)
			paths = append(paths, p)
		} else if depth < maxDepth {
			for _, e := range g.Out[curr] {
				if !visited[e.TargetID] {
					dfs(e.TargetID, depth+1)
				}
			}
		}

		visited[curr] = false
		current = current[:len(current)-1]
	}

	dfs(fromID, 0)

	sort.Slice(paths, func(i, j int) bool {
		if len(paths[i]) != len(paths[j]) {
			return len(paths[i]) < len(paths[j])
		}
		for k := 0; k < len(paths[i]); k++ {
			if paths[i][k] != paths[j][k] {
				return paths[i][k] < paths[j][k]
			}
		}
		return false
	})

	return paths, truncated
}

type DegreeInfo struct {
	InDegree  int `json:"in_degree"`
	OutDegree int `json:"out_degree"`
}

func (g *Graph) DegreeMetrics() map[string]DegreeInfo {
	res := make(map[string]DegreeInfo, len(g.Nodes))
	for id := range g.Nodes {
		res[id] = DegreeInfo{
			InDegree:  len(g.In[id]),
			OutDegree: len(g.Out[id]),
		}
	}
	return res
}
