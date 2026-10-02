package indexer

import (
	"fmt"

	"github.com/vedansh-5/graphcontext/pkg/resolver"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

type edgeKey struct {
	source, target string
	kind           store.EdgeKind
	line           int
}

func keyOf(e store.Edge) edgeKey {
	return edgeKey{e.SourceID, e.TargetID, e.Kind, e.Line}
}

// normalizeEdge applies the defaults the store applies on write, so a freshly
// resolved edge compares equal to its stored copy.
func normalizeEdge(e store.Edge) store.Edge {
	if e.CandidateCount == 0 {
		e.CandidateCount = 1
	}
	return e
}

// queueGraphDiff compares the resolved graph with what the store holds and
// queues only the difference. Editing one file usually changes a handful of
// rows, so this avoids rewriting the whole graph on every re-index.
func queueGraphDiff(s *store.Store, batch *store.Batch, res *resolver.ResolutionResult) error {
	oldNodes, err := s.AllNodes()
	if err != nil {
		return fmt.Errorf("read stored nodes: %w", err)
	}
	oldEdges, err := s.AllEdges()
	if err != nil {
		return fmt.Errorf("read stored edges: %w", err)
	}

	newNodes := make(map[string]store.Node, len(res.Nodes))
	for _, n := range res.Nodes {
		newNodes[n.ID] = n
	}
	newEdges := make(map[edgeKey]store.Edge, len(res.Edges))
	for _, e := range res.Edges {
		newEdges[keyOf(e)] = normalizeEdge(e)
	}

	storedNodes := make(map[string]store.Node, len(oldNodes))
	for _, n := range oldNodes {
		storedNodes[n.ID] = n
		if _, ok := newNodes[n.ID]; !ok {
			batch.DeleteNode(n.ID)
		}
	}
	storedEdges := make(map[edgeKey]store.Edge, len(oldEdges))
	for _, e := range oldEdges {
		k := keyOf(e)
		storedEdges[k] = e
		if _, ok := newEdges[k]; !ok {
			batch.DeleteEdge(e)
		}
	}

	// Iterate the resolver's slices, not the maps, to keep write order stable.
	for _, n := range res.Nodes {
		if old, ok := storedNodes[n.ID]; !ok || old != newNodes[n.ID] {
			batch.AddNode(newNodes[n.ID])
		}
	}
	for _, e := range res.Edges {
		k := keyOf(e)
		if old, ok := storedEdges[k]; !ok || old != newEdges[k] {
			batch.AddEdge(newEdges[k])
		}
	}
	return nil
}
