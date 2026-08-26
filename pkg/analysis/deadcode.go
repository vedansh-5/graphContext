package analysis

import (
	"sort"
	"strings"

	"github.com/vedansh-5/graphcontext/pkg/store"
)

func (g *Graph) DeadCandidates() (rootCount int, dead []store.Node) {
	visited := make(map[string]bool)
	var frontier []string
	for id, n := range g.Nodes {
		if isRoot(n) {
			visited[id] = true
			frontier = append(frontier, id)
		}
	}
	rootCount = len(frontier)

	for len(frontier) > 0 {
		var next []string
		for _, id := range frontier {
			for _, e := range g.Out[id] {
				if e.Kind != store.EdgeCalls || visited[e.TargetID] {
					continue
				}
				visited[e.TargetID] = true
				next = append(next, e.TargetID)
			}
		}
		frontier = next
	}

	for id, n := range g.Nodes {
		if (n.Kind == store.KindFunction || n.Kind == store.KindMethod) && !visited[id] {
			dead = append(dead, n)
		}
	}

	sort.Slice(dead, func(i, j int) bool { return dead[i].ID < dead[j].ID })
	return rootCount, dead
}

func isRoot(n store.Node) bool {
	if n.Kind == store.KindFile {
		return true
	}
	if n.IsEntrypoint || n.IsTest || n.EntrypointKind != store.EntryNone {
		return true
	}
	if n.Name == "main" || strings.HasPrefix(n.Name, "Test") || strings.HasPrefix(n.Name, "test_") || strings.HasPrefix(n.Name, "__") {
		return true
	}
	return false
}
