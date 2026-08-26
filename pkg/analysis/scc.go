package analysis

import (
	"sort"

	"github.com/vedansh-5/graphcontext/pkg/store"
)

func (g *Graph) SCCs(kinds map[store.EdgeKind]bool, groupOf func(store.Node) string) [][]string {
	adj := make(map[string]map[string]bool)
	for _, edges := range g.Out {
		for _, e := range edges {
			if !kinds[e.Kind] {
				continue
			}
			s, okS := g.Nodes[e.SourceID]
			t, okT := g.Nodes[e.TargetID]
			if !okS || !okT {
				continue
			}
			from, to := groupOf(s), groupOf(t)
			if from == "" || to == "" || from == to {
				continue
			}
			if adj[from] == nil {
				adj[from] = make(map[string]bool)
			}
			adj[from][to] = true
			if adj[to] == nil {
				adj[to] = make(map[string]bool)
			}
		}
	}

	index := make(map[string]int)
	low := make(map[string]int)
	onStack := make(map[string]bool)
	var stack []string
	counter := 0
	var comps [][]string

	var connect func(v string)
	connect = func(v string) {
		index[v] = counter
		low[v] = counter
		counter++
		stack = append(stack, v)
		onStack[v] = true

		for w := range adj[v] {
			if _, seen := index[w]; !seen {
				connect(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] && index[w] < low[v] {
				low[v] = index[w]
			}
		}

		if low[v] == index[v] {
			var comp []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp = append(comp, w)
				if w == v {
					break
				}
			}
			if len(comp) > 1 {
				sort.Strings(comp)
				comps = append(comps, comp)
			}
		}
	}

	var vertices []string
	for v := range adj {
		vertices = append(vertices, v)
	}
	sort.Strings(vertices)
	for _, v := range vertices {
		if _, seen := index[v]; !seen {
			connect(v)
		}
	}

	sort.Slice(comps, func(i, j int) bool {
		if len(comps[i]) != len(comps[j]) {
			return len(comps[i]) > len(comps[j])
		}
		return comps[i][0] < comps[j][0]
	})
	return comps
}
