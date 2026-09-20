package archlint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vedansh-5/graphcontext/pkg/analysis"
	"github.com/vedansh-5/graphcontext/pkg/diff"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func setupTestGraph(t *testing.T) *analysis.Graph {
	t.Helper()

	nodes := []store.Node{
		{
			ID:       "pkg/domain/user.go:User",
			Name:     "User",
			FilePath: "pkg/domain/user.go",
			Kind:     store.KindClass,
		},
		{
			ID:       "pkg/domain/service.go:Register",
			Name:     "Register",
			FilePath: "pkg/domain/service.go",
			Kind:     store.KindFunction,
		},
		{
			ID:       "pkg/infra/db.go:Save",
			Name:     "Save",
			FilePath: "pkg/infra/db.go",
			Kind:     store.KindFunction,
		},
		{
			ID:       "pkg/ui/controller.go:Handle",
			Name:     "Handle",
			FilePath: "pkg/ui/controller.go",
			Kind:     store.KindFunction,
		},
	}

	edges := []store.Edge{
		{
			SourceID: "pkg/domain/service.go:Register",
			TargetID: "pkg/infra/db.go:Save",
			Kind:     store.EdgeCalls,
			Line:     15,
		},
		{
			SourceID: "pkg/ui/controller.go:Handle",
			TargetID: "pkg/domain/service.go:Register",
			Kind:     store.EdgeCalls,
			Line:     22,
		},
	}

	return analysis.NewGraph(nodes, edges)
}

func TestForbiddenRule(t *testing.T) {
	g := setupTestGraph(t)

	rules := RuleSet{
		Forbidden: []ForbiddenRule{
			{
				From:    "pkg/domain/**",
				To:      "pkg/infra/**",
				Message: "Domain layer must not depend on Infrastructure",
			},
		},
	}

	violations := LintGraph(g, rules)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}

	v := violations[0]
	if v.RuleType != "forbidden" {
		t.Errorf("expected rule_type forbidden, got %s", v.RuleType)
	}
	if v.SourceID != "pkg/domain/service.go:Register" {
		t.Errorf("expected source Register, got %s", v.SourceID)
	}
	if v.TargetID != "pkg/infra/db.go:Save" {
		t.Errorf("expected target Save, got %s", v.TargetID)
	}
	if v.Message != "Domain layer must not depend on Infrastructure" {
		t.Errorf("unexpected violation message: %s", v.Message)
	}
}

func TestLayerHierarchyRule(t *testing.T) {
	g := setupTestGraph(t)

	// UI (0) -> Application -> Domain (2)
	rules := RuleSet{
		Layers: &LayerRule{
			Layers: []string{
				"pkg/ui/**",
				"pkg/infra/**",
				"pkg/domain/**",
			},
		},
	}

	violations := LintGraph(g, rules)

	// Register (pkg/domain, index 2) calls Save (pkg/infra, index 1) -> violation!
	// Handle (pkg/ui, index 0) calls Register (pkg/domain, index 2) -> allowed!
	if len(violations) != 1 {
		t.Fatalf("expected 1 layer violation, got %d", len(violations))
	}

	v := violations[0]
	if v.RuleType != "layer_inversion" {
		t.Errorf("expected layer_inversion, got %s", v.RuleType)
	}
	if v.SourceID != "pkg/domain/service.go:Register" {
		t.Errorf("expected Register to be flagged, got %s", v.SourceID)
	}
}

func TestLintDiff(t *testing.T) {
	g := setupTestGraph(t)

	rules := RuleSet{
		Forbidden: []ForbiddenRule{
			{
				From: "pkg/domain/**",
				To:   "pkg/infra/**",
			},
		},
	}

	// Diff only modified ui/controller.go (which is allowed)
	changedUI := []diff.ChangedSymbol{
		{
			Node:       g.Nodes["pkg/ui/controller.go:Handle"],
			ChangeType: "modified",
		},
	}
	vUI := LintDiff(changedUI, g, rules)
	if len(vUI) != 0 {
		t.Errorf("expected 0 violations for clean diff, got %d", len(vUI))
	}

	// Diff modified domain/service.go (which introduces/contains forbidden call)
	changedDomain := []diff.ChangedSymbol{
		{
			Node:       g.Nodes["pkg/domain/service.go:Register"],
			ChangeType: "modified",
		},
	}
	vDomain := LintDiff(changedDomain, g, rules)
	if len(vDomain) != 1 {
		t.Fatalf("expected 1 violation for domain diff, got %d", len(vDomain))
	}
}

func TestLoadRulesFromFile(t *testing.T) {
	jsonContent := `{
		"forbidden": [
			{ "from": "pkg/domain/**", "to": "pkg/infra/**", "message": "no infra in domain" }
		],
		"layers": {
			"layers": ["pkg/ui/**", "pkg/domain/**"]
		}
	}`

	tmpFile := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(tmpFile, []byte(jsonContent), 0o644); err != nil {
		t.Fatal(err)
	}

	rules, err := LoadRulesFromFile(tmpFile)
	if err != nil {
		t.Fatalf("LoadRulesFromFile failed: %v", err)
	}

	if len(rules.Forbidden) != 1 || rules.Forbidden[0].Message != "no infra in domain" {
		t.Errorf("expected forbidden rule loaded, got %v", rules.Forbidden)
	}
	if rules.Layers == nil || len(rules.Layers.Layers) != 2 {
		t.Errorf("expected 2 layers loaded, got %v", rules.Layers)
	}
}
