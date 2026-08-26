package resolver

import (
	"testing"

	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func TestBuildIndexAndImports(t *testing.T) {
	files := []*lang.FileIR{
		{
			Path:     "auth/models.go",
			Language: "go",
			Nodes: []store.Node{
				{ID: "auth/models.go:User", Kind: store.KindClass, Name: "User", QualifiedName: "User", FilePath: "auth/models.go"},
			},
		},
		{
			Path:     "auth/service.go",
			Language: "go",
			Imports: []lang.ImportRef{
				{Path: "auth", Line: 3},
			},
			Nodes: []store.Node{
				{ID: "auth/service.go:Login", Kind: store.KindFunction, Name: "Login", QualifiedName: "Login", FilePath: "auth/service.go"},
			},
		},
	}
	idx := buildIndex("/repo", files)
	if idx == nil {
		t.Fatal("expected index, got nil")
	}
	if len(idx.byName["User"]) == 0 {
		t.Errorf("expected User in byName index")
	}
}
