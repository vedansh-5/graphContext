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

func TestResolveMultiFile(t *testing.T) {
	tf1 := lang.NewTypeFacts()
	tf1.Vars[lang.VarKey("svc.go:UserRepo.Save", "r")] = "UserRepo"
	tf1.Fields["UserRepo.db"] = "DB"
	tf1.Methods["UserRepo"] = []string{"Save"}
	tf1.Methods["Store"] = []string{"Save"}

	files := []*lang.FileIR{
		{
			Path:     "db.go",
			Language: "go",
			Nodes: []store.Node{
				{ID: "db.go:DB", Kind: store.KindClass, Name: "DB", QualifiedName: "DB", FilePath: "db.go", Visibility: store.VisExported},
				{ID: "db.go:DB.Write", Kind: store.KindMethod, Name: "Write", QualifiedName: "DB.Write", FilePath: "db.go", Visibility: store.VisExported},
			},
		},
		{
			Path:     "svc.go",
			Language: "go",
			Imports: []lang.ImportRef{
				{Path: "db", Line: 3},
			},
			Types: tf1,
			Nodes: []store.Node{
				{ID: "svc.go:UserRepo", Kind: store.KindClass, Name: "UserRepo", QualifiedName: "UserRepo", FilePath: "svc.go", Visibility: store.VisExported},
				{ID: "svc.go:UserRepo.Save", Kind: store.KindMethod, Name: "Save", QualifiedName: "UserRepo.Save", FilePath: "svc.go", Visibility: store.VisExported},
				{ID: "svc.go:Store", Kind: store.KindInterface, Name: "Store", QualifiedName: "Store", FilePath: "svc.go", Visibility: store.VisExported},
			},
			Refs: []lang.Ref{
				{Kind: store.EdgeCalls, Name: "Write", Receiver: "r.db", FromID: "svc.go:UserRepo.Save", Line: 10},
				{Kind: store.EdgeCalls, Name: "Println", Receiver: "fmt", FromID: "svc.go:UserRepo.Save", Line: 11},
			},
		},
	}

	res, err := Resolve("/repo", files)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	var foundCall, foundExternal, foundImplements bool
	for _, e := range res.Edges {
		if e.SourceID == "svc.go:UserRepo.Save" && e.TargetID == "db.go:DB.Write" {
			if e.Confidence == store.ConfExact {
				foundCall = true
			}
		}
		if e.SourceID == "svc.go:UserRepo.Save" && e.TargetID == "external:fmt.Println" {
			if e.Confidence == store.ConfUnknown {
				foundExternal = true
			}
		}
		if e.SourceID == "svc.go:UserRepo" && e.TargetID == "svc.go:Store" && e.Kind == store.EdgeImplements {
			if e.Confidence == store.ConfExact {
				foundImplements = true
			}
		}
	}

	if !foundCall {
		t.Errorf("did not resolve r.db.Write to db.go:DB.Write with exact confidence; edges: %+v", res.Edges)
	}
	if !foundExternal {
		t.Errorf("did not create external edge for fmt.Println; edges: %+v", res.Edges)
	}
	if !foundImplements {
		t.Errorf("did not derive structural interface implementation UserRepo -> Store; edges: %+v", res.Edges)
	}
	if res.Stats.TotalRefs != 2 {
		t.Errorf("Stats.TotalRefs = %d, want 2", res.Stats.TotalRefs)
	}
	if res.Stats.Exact != 1 {
		t.Errorf("Stats.Exact = %d, want 1", res.Stats.Exact)
	}
}
