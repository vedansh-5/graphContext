package analysis

import (
	"testing"

	"github.com/vedansh-5/graphcontext/pkg/store"
)

func buildTestGraph() *Graph {
	nodes := []store.Node{
		{ID: "app.go:main", Name: "main", Kind: store.KindFunction, FilePath: "app.go", IsEntrypoint: true, EntrypointKind: store.EntryMain},
		{ID: "svc.go:Service.Run", Name: "Run", QualifiedName: "Service.Run", Kind: store.KindMethod, FilePath: "svc.go"},
		{ID: "db.go:DB.Query", Name: "Query", QualifiedName: "DB.Query", Kind: store.KindMethod, FilePath: "db.go"},
		{ID: "db.go:DB.Log", Name: "Log", QualifiedName: "DB.Log", Kind: store.KindMethod, FilePath: "db.go"},
		{ID: "unused.go:UnusedFunc", Name: "UnusedFunc", Kind: store.KindFunction, FilePath: "unused.go"},
		{ID: "cycle_a.go:A", Name: "A", Kind: store.KindFunction, FilePath: "cycle_a.go"},
		{ID: "cycle_b.go:B", Name: "B", Kind: store.KindFunction, FilePath: "cycle_b.go"},
	}

	edges := []store.Edge{
		{SourceID: "app.go:main", TargetID: "svc.go:Service.Run", Kind: store.EdgeCalls, Line: 10, Confidence: store.ConfExact},
		{SourceID: "svc.go:Service.Run", TargetID: "db.go:DB.Query", Kind: store.EdgeCalls, Line: 20, Confidence: store.ConfExact},
		{SourceID: "svc.go:Service.Run", TargetID: "db.go:DB.Log", Kind: store.EdgeCalls, Line: 21, Confidence: store.ConfExact},
		{SourceID: "cycle_a.go:A", TargetID: "cycle_b.go:B", Kind: store.EdgeCalls, Line: 5, Confidence: store.ConfExact},
		{SourceID: "cycle_b.go:B", TargetID: "cycle_a.go:A", Kind: store.EdgeCalls, Line: 6, Confidence: store.ConfExact},
	}

	return NewGraph(nodes, edges)
}

func TestReverseReach(t *testing.T) {
	g := buildTestGraph()
	kinds := map[store.EdgeKind]bool{store.EdgeCalls: true}

	results, truncated := g.ReverseReach("db.go:DB.Query", kinds, 5, 10)
	if truncated {
		t.Errorf("expected truncated=false")
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 callers in reverse reach, got %d (%+v)", len(results), results)
	}
	if results[0].Node.ID != "svc.go:Service.Run" || results[0].Depth != 1 {
		t.Errorf("depth 1 caller should be Service.Run, got %+v", results[0])
	}
	if results[1].Node.ID != "app.go:main" || results[1].Depth != 2 {
		t.Errorf("depth 2 caller should be main, got %+v", results[1])
	}
}

func TestForwardReach(t *testing.T) {
	g := buildTestGraph()
	kinds := map[store.EdgeKind]bool{store.EdgeCalls: true}

	results, truncated := g.ForwardReach("app.go:main", kinds, 5, 10)
	if truncated {
		t.Errorf("expected truncated=false")
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 dependencies in forward reach, got %d (%+v)", len(results), results)
	}
}

func TestTrace(t *testing.T) {
	g := buildTestGraph()
	root, truncated := g.Trace("app.go:main", 5, 10)
	if truncated {
		t.Errorf("expected truncated=false")
	}
	if root == nil || root.ID != "app.go:main" {
		t.Fatalf("expected root main, got %+v", root)
	}
	if len(root.Calls) != 1 || root.Calls[0].ID != "svc.go:Service.Run" {
		t.Fatalf("expected call to Service.Run, got %+v", root.Calls)
	}
	if len(root.Calls[0].Calls) != 2 {
		t.Fatalf("expected 2 sub-calls from Service.Run, got %d", len(root.Calls[0].Calls))
	}
}

func TestSCCs(t *testing.T) {
	g := buildTestGraph()
	kinds := map[store.EdgeKind]bool{store.EdgeCalls: true}

	cycles := g.SCCs(kinds, func(n store.Node) string {
		return n.ID
	})

	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d (%+v)", len(cycles), cycles)
	}
	if len(cycles[0]) != 2 {
		t.Errorf("cycle length should be 2, got %d", len(cycles[0]))
	}
}

func TestDeadCandidates(t *testing.T) {
	g := buildTestGraph()
	rootCount, dead := g.DeadCandidates()
	if rootCount < 1 {
		t.Errorf("expected at least 1 root, got %d", rootCount)
	}
	var foundUnused bool
	for _, d := range dead {
		if d.ID == "unused.go:UnusedFunc" {
			foundUnused = true
		}
	}
	if !foundUnused {
		t.Errorf("expected UnusedFunc in dead code candidates, got %+v", dead)
	}
}

func TestPathsBetween(t *testing.T) {
	g := buildTestGraph()
	paths, truncated := g.PathsBetween("app.go:main", "db.go:DB.Query", 5, 10)
	if truncated {
		t.Errorf("expected truncated=false")
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d (%+v)", len(paths), paths)
	}
	expected := []string{"app.go:main", "svc.go:Service.Run", "db.go:DB.Query"}
	for i, id := range expected {
		if paths[0][i] != id {
			t.Errorf("path[%d] = %q, want %q", i, paths[0][i], id)
		}
	}
}

func TestNeighborhood(t *testing.T) {
	g := buildTestGraph()
	hood := g.Neighborhood("svc.go:Service.Run", 1, 10)
	if len(hood.Nodes) < 3 {
		t.Errorf("expected at least 3 nodes in neighborhood, got %d", len(hood.Nodes))
	}
}

func TestCondense(t *testing.T) {
	g := buildTestGraph()
	arch := g.Condense(func(n store.Node) string {
		return n.FilePath
	})
	if len(arch.Modules) < 3 {
		t.Errorf("expected at least 3 modules, got %d", len(arch.Modules))
	}
	if len(arch.Edges) == 0 {
		t.Errorf("expected condensed edges, got none")
	}
}
