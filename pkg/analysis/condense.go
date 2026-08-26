package analysis

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vedansh-5/graphcontext/pkg/store"
)

type ModuleEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Weight int    `json:"weight"`
}

type ArchitectureMap struct {
	Modules []string     `json:"modules"`
	Edges   []ModuleEdge `json:"edges"`
	Mermaid string       `json:"mermaid"`
}

func (g *Graph) Condense(groupOf func(store.Node) string) *ArchitectureMap {
	weights := make(map[string]map[string]int)
	modulesSet := make(map[string]bool)

	for _, n := range g.Nodes {
		mod := groupOf(n)
		if mod != "" {
			modulesSet[mod] = true
		}
	}

	for _, edges := range g.Out {
		for _, e := range edges {
			s, okS := g.Nodes[e.SourceID]
			t, okT := g.Nodes[e.TargetID]
			if !okS || !okT {
				continue
			}
			from, to := groupOf(s), groupOf(t)
			if from == "" || to == "" || from == to {
				continue
			}
			if weights[from] == nil {
				weights[from] = make(map[string]int)
			}
			weights[from][to]++
		}
	}

	var modules []string
	for m := range modulesSet {
		modules = append(modules, m)
	}
	sort.Strings(modules)

	var modEdges []ModuleEdge
	for from, toMap := range weights {
		for to, weight := range toMap {
			modEdges = append(modEdges, ModuleEdge{
				From:   from,
				To:     to,
				Weight: weight,
			})
		}
	}

	sort.Slice(modEdges, func(i, j int) bool {
		if modEdges[i].Weight != modEdges[j].Weight {
			return modEdges[i].Weight > modEdges[j].Weight
		}
		if modEdges[i].From != modEdges[j].From {
			return modEdges[i].From < modEdges[j].From
		}
		return modEdges[i].To < modEdges[j].To
	})

	var b strings.Builder
	b.WriteString("graph TD\n")
	for _, e := range modEdges {
		fromClean := strings.ReplaceAll(strings.ReplaceAll(e.From, "/", "_"), ".", "_")
		toClean := strings.ReplaceAll(strings.ReplaceAll(e.To, "/", "_"), ".", "_")
		fmt.Fprintf(&b, "    %s[\"%s\"] -->|%d| %s[\"%s\"]\n", fromClean, e.From, e.Weight, toClean, e.To)
	}

	return &ArchitectureMap{
		Modules: modules,
		Edges:   modEdges,
		Mermaid: b.String(),
	}
}
