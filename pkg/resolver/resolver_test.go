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

func TestTypeHierarchyAndReceiverResolution(t *testing.T) {
	tf := lang.NewTypeFacts()
	tf.Vars[lang.VarKey("svc.go:Save", "r")] = "UserRepo"
	tf.Fields["UserRepo.db"] = "DB"
	tf.Bases["UserRepo"] = []string{"BaseRepo"}

	files := []*lang.FileIR{
		{
			Path:     "svc.go",
			Language: "go",
			Types:    tf,
			Nodes: []store.Node{
				{ID: "svc.go:DB.Write", Kind: store.KindMethod, Name: "Write", QualifiedName: "DB.Write", FilePath: "svc.go"},
				{ID: "svc.go:BaseRepo.Ping", Kind: store.KindMethod, Name: "Ping", QualifiedName: "BaseRepo.Ping", FilePath: "svc.go"},
			},
		},
	}
	idx := buildIndex("/repo", files)
	targetType := idx.resolveReceiverType("svc.go:Save", "r.db")
	if targetType != "DB" {
		t.Errorf("resolveReceiverType(r.db) = %q, want DB", targetType)
	}

	methods := idx.findMethodInHierarchy("UserRepo", "Ping")
	if len(methods) == 0 || methods[0].ID != "svc.go:BaseRepo.Ping" {
		t.Errorf("findMethodInHierarchy(UserRepo, Ping) = %+v, want BaseRepo.Ping", methods)
	}
}
