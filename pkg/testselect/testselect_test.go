package testselect

import (
	"path/filepath"
	"testing"

	"github.com/vedansh-5/graphcontext/pkg/analysis"
	"github.com/vedansh-5/graphcontext/pkg/diff"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func setupTestGraph(t *testing.T) (*analysis.Graph, *store.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}

	nodes := []store.Node{
		{
			ID:        "pkg/auth/service.go:Validate",
			Name:      "Validate",
			FilePath:  "pkg/auth/service.go",
			StartLine: 5,
			EndLine:   8,
			Kind:      store.KindFunction,
		},
		{
			ID:        "pkg/auth/service.go:Login",
			Name:      "Login",
			FilePath:  "pkg/auth/service.go",
			StartLine: 10,
			EndLine:   15,
			Kind:      store.KindFunction,
		},
		{
			ID:        "pkg/auth/service_test.go:TestLogin",
			Name:      "TestLogin",
			FilePath:  "pkg/auth/service_test.go",
			StartLine: 5,
			EndLine:   10,
			Kind:      store.KindFunction,
			IsTest:    true,
		},
		{
			ID:        "pkg/billing/charge.go:Charge",
			Name:      "Charge",
			FilePath:  "pkg/billing/charge.go",
			StartLine: 5,
			EndLine:   10,
			Kind:      store.KindFunction,
		},
		{
			ID:        "pkg/billing/charge_test.go:TestCharge",
			Name:      "TestCharge",
			FilePath:  "pkg/billing/charge_test.go",
			StartLine: 5,
			EndLine:   10,
			Kind:      store.KindFunction,
			IsTest:    true,
		},
	}

	edges := []store.Edge{
		{
			SourceID:   "pkg/auth/service.go:Login",
			TargetID:   "pkg/auth/service.go:Validate",
			Kind:       store.EdgeCalls,
			Line:       12,
			Confidence: store.ConfExact,
		},
		{
			SourceID:   "pkg/auth/service_test.go:TestLogin",
			TargetID:   "pkg/auth/service.go:Login",
			Kind:       store.EdgeCalls,
			Line:       7,
			Confidence: store.ConfExact,
		},
		{
			SourceID:   "pkg/billing/charge_test.go:TestCharge",
			TargetID:   "pkg/billing/charge.go:Charge",
			Kind:       store.EdgeCalls,
			Line:       7,
			Confidence: store.ConfExact,
		},
	}

	b := store.NewBatch()
	for _, n := range nodes {
		b.AddNode(n)
	}
	for _, e := range edges {
		b.AddEdge(e)
	}

	if err := s.Commit(b); err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	g := analysis.NewGraph(nodes, edges)
	return g, s
}

func TestDirectAndTransitiveTestSelection(t *testing.T) {
	g, s := setupTestGraph(t)
	defer s.Close()

	changedValidate := []diff.ChangedSymbol{
		{
			Node:       g.Nodes["pkg/auth/service.go:Validate"],
			ChangeType: "modified",
		},
	}

	res := SelectTests(changedValidate, g, 5)

	if res.AffectedTestsCount != 1 {
		t.Fatalf("expected 1 affected test, got %d", res.AffectedTestsCount)
	}

	match := res.AffectedTests[0]
	if match.TestNode.ID != "pkg/auth/service_test.go:TestLogin" {
		t.Errorf("expected TestLogin to be selected, got %s", match.TestNode.ID)
	}
	if match.Depth != 2 {
		t.Errorf("expected depth 2 from Validate to TestLogin, got %d", match.Depth)
	}
	if len(res.TestFiles) != 1 || res.TestFiles[0] != "pkg/auth/service_test.go" {
		t.Errorf("expected test file pkg/auth/service_test.go, got %v", res.TestFiles)
	}
	if res.TotalTestsInRepo != 2 {
		t.Errorf("expected 2 total tests in repo, got %d", res.TotalTestsInRepo)
	}
	if res.ReductionPercent != 50.0 {
		t.Errorf("expected 50%% reduction, got %f", res.ReductionPercent)
	}
}

func TestDirectCallerTestSelection(t *testing.T) {
	g, s := setupTestGraph(t)
	defer s.Close()

	changedLogin := []diff.ChangedSymbol{
		{
			Node:       g.Nodes["pkg/auth/service.go:Login"],
			ChangeType: "modified",
		},
	}

	res := SelectTests(changedLogin, g, 5)

	if res.AffectedTestsCount != 1 {
		t.Fatalf("expected 1 affected test, got %d", res.AffectedTestsCount)
	}
	match := res.AffectedTests[0]
	if match.Depth != 1 {
		t.Errorf("expected depth 1 for direct test caller, got %d", match.Depth)
	}
}

func TestChangedTestDirectly(t *testing.T) {
	g, s := setupTestGraph(t)
	defer s.Close()

	changedTest := []diff.ChangedSymbol{
		{
			Node:       g.Nodes["pkg/auth/service_test.go:TestLogin"],
			ChangeType: "modified",
		},
	}

	res := SelectTests(changedTest, g, 5)

	if res.AffectedTestsCount != 1 {
		t.Fatalf("expected 1 affected test, got %d", res.AffectedTestsCount)
	}
	match := res.AffectedTests[0]
	if match.Depth != 0 {
		t.Errorf("expected depth 0 when test itself is modified, got %d", match.Depth)
	}
}
