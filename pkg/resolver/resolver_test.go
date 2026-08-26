package resolver

import (
	"reflect"
	"testing"

	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/lang/golang"
	"github.com/vedansh-5/graphcontext/pkg/lang/python"
	"github.com/vedansh-5/graphcontext/pkg/lang/typescript"
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

func TestResolveGoEndToEnd(t *testing.T) {
	dbSrc := `package db

type DB struct{}

func (d *DB) Execute() error {
	return nil
}
`
	svcSrc := `package service

import "db"

type AppService struct {
	db *db.DB
}

func (s *AppService) Run() error {
	return s.db.Execute()
}
`
	ir1, err := golang.Plugin{}.Parse("db/db.go", []byte(dbSrc))
	if err != nil {
		t.Fatalf("Parse db.go: %v", err)
	}
	ir2, err := golang.Plugin{}.Parse("service/svc.go", []byte(svcSrc))
	if err != nil {
		t.Fatalf("Parse svc.go: %v", err)
	}

	res, err := Resolve("", []*lang.FileIR{ir1, ir2})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	var foundExecute bool
	for _, e := range res.Edges {
		if e.SourceID == "service/svc.go:AppService.Run" && e.TargetID == "db/db.go:DB.Execute" {
			if e.Confidence == store.ConfExact {
				foundExecute = true
			}
		}
	}
	if !foundExecute {
		t.Errorf("Go end-to-end failed to resolve s.db.Execute(); edges: %+v", res.Edges)
	}
}

func TestResolvePythonEndToEnd(t *testing.T) {
	modelsSrc := `class User:
    pass

class Base:
    def ping(self):
        pass
`
	repoSrc := `from models import User, Base

class UserRepo(Base):
    def __init__(self, db: DB):
        self.db = db

    def save(self):
        self.ping()
        u = User()
        return self.db.write(u)
`
	ir1, err := python.Plugin{}.Parse("models.py", []byte(modelsSrc))
	if err != nil {
		t.Fatalf("Parse models.py: %v", err)
	}
	ir2, err := python.Plugin{}.Parse("repo.py", []byte(repoSrc))
	if err != nil {
		t.Fatalf("Parse repo.py: %v", err)
	}

	res, err := Resolve("", []*lang.FileIR{ir1, ir2})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	var foundPing, foundUser, foundInherits bool
	for _, e := range res.Edges {
		if e.SourceID == "repo.py:UserRepo.save" && e.TargetID == "models.py:Base.ping" && e.Confidence == store.ConfExact {
			foundPing = true
		}
		if e.SourceID == "repo.py:UserRepo.save" && e.TargetID == "models.py:User" {
			foundUser = true
		}
		if e.SourceID == "repo.py:UserRepo" && e.TargetID == "models.py:Base" && e.Kind == store.EdgeInherits {
			foundInherits = true
		}
	}
	if !foundPing {
		t.Errorf("Python end-to-end failed to resolve self.ping(); edges: %+v", res.Edges)
	}
	if !foundUser {
		t.Errorf("Python end-to-end failed to resolve User() constructor call; edges: %+v", res.Edges)
	}
	if !foundInherits {
		t.Errorf("Python end-to-end failed to resolve inherits edge; edges: %+v", res.Edges)
	}
}

func TestResolveTypeScriptEndToEnd(t *testing.T) {
	modelsSrc := `export interface Store {
  save(): void;
}

export class Base {
  ping(): void {}
}
`
	repoSrc := `import { Store, Base } from './models';

export class UserRepo extends Base implements Store {
  save(): void {
    this.ping();
  }
}
`
	ir1, err := typescript.Plugin{}.Parse("models.ts", []byte(modelsSrc))
	if err != nil {
		t.Fatalf("Parse models.ts: %v", err)
	}
	ir2, err := typescript.Plugin{}.Parse("repo.ts", []byte(repoSrc))
	if err != nil {
		t.Fatalf("Parse repo.ts: %v", err)
	}

	res, err := Resolve("", []*lang.FileIR{ir1, ir2})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	var foundPing, foundInherits, foundImplements bool
	for _, e := range res.Edges {
		if e.SourceID == "repo.ts:UserRepo.save" && e.TargetID == "models.ts:Base.ping" && e.Confidence == store.ConfExact {
			foundPing = true
		}
		if e.SourceID == "repo.ts:UserRepo" && e.TargetID == "models.ts:Base" && e.Kind == store.EdgeInherits {
			foundInherits = true
		}
		if e.SourceID == "repo.ts:UserRepo" && e.TargetID == "models.ts:Store" && e.Kind == store.EdgeImplements {
			foundImplements = true
		}
	}
	if !foundPing {
		t.Errorf("TS end-to-end failed to resolve this.ping(); edges: %+v", res.Edges)
	}
	if !foundInherits {
		t.Errorf("TS end-to-end failed to resolve inherits edge; edges: %+v", res.Edges)
	}
	if !foundImplements {
		t.Errorf("TS end-to-end failed to resolve implements edge; edges: %+v", res.Edges)
	}
}

func TestAmbiguityResolution(t *testing.T) {
	src1 := `package store
type SqlStore struct{}
func (s *SqlStore) Connect() {}
`
	src2 := `package store
type MongoStore struct{}
func (m *MongoStore) Connect() {}
`
	src3 := `package app
func Init(client any) {
	client.Connect()
}
`
	ir1, _ := golang.Plugin{}.Parse("sql.go", []byte(src1))
	ir2, _ := golang.Plugin{}.Parse("mongo.go", []byte(src2))
	ir3, _ := golang.Plugin{}.Parse("app.go", []byte(src3))

	res, err := Resolve("", []*lang.FileIR{ir1, ir2, ir3})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	var ambCount int
	for _, e := range res.Edges {
		if e.SourceID == "app.go:Init" && (e.TargetID == "sql.go:SqlStore.Connect" || e.TargetID == "mongo.go:MongoStore.Connect") {
			if e.Confidence == store.ConfAmbiguous && e.CandidateCount == 2 {
				ambCount++
			}
		}
	}
	if ambCount != 2 {
		t.Errorf("expected 2 ambiguous edges with CandidateCount=2, got %d; edges: %+v", ambCount, res.Edges)
	}
}

func TestResolveDeterminism(t *testing.T) {
	files := []*lang.FileIR{
		{
			Path:     "a.go",
			Language: "go",
			Nodes: []store.Node{
				{ID: "a.go:A", Kind: store.KindClass, Name: "A", QualifiedName: "A", FilePath: "a.go"},
			},
		},
		{
			Path:     "b.go",
			Language: "go",
			Nodes: []store.Node{
				{ID: "b.go:B", Kind: store.KindClass, Name: "B", QualifiedName: "B", FilePath: "b.go"},
			},
			Refs: []lang.Ref{
				{Kind: store.EdgeCalls, Name: "A", FromID: "b.go:B", Line: 5},
			},
		},
	}

	r1, _ := Resolve("", files)
	r2, _ := Resolve("", files)

	if !reflect.DeepEqual(r1, r2) {
		t.Errorf("two Resolve calls produced different results")
	}
}
