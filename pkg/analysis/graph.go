package analysis

import (
	"fmt"

	"github.com/vedansh-5/graphcontext/pkg/store"
)

type Graph struct {
	Nodes map[string]store.Node
	Out   map[string][]store.Edge
	In    map[string][]store.Edge
}

func NewGraph(nodes []store.Node, edges []store.Edge) *Graph {
	g := &Graph{
		Nodes: make(map[string]store.Node, len(nodes)),
		Out:   make(map[string][]store.Edge),
		In:    make(map[string][]store.Edge),
	}
	for _, n := range nodes {
		g.Nodes[n.ID] = n
	}
	for _, e := range edges {
		g.Out[e.SourceID] = append(g.Out[e.SourceID], e)
		g.In[e.TargetID] = append(g.In[e.TargetID], e)
	}
	return g
}

func Load(s *store.Store) (*Graph, error) {
	nodes, err := s.AllNodes()
	if err != nil {
		return nil, fmt.Errorf("load nodes: %w", err)
	}
	edges, err := s.AllEdges()
	if err != nil {
		return nil, fmt.Errorf("load edges: %w", err)
	}
	return NewGraph(nodes, edges), nil
}
