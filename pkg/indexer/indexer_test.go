package indexer

import (
	"os"
	"path/filepath"
	"testing"

	_ "github.com/vedansh-5/graphcontext/pkg/lang/golang"
	_ "github.com/vedansh-5/graphcontext/pkg/lang/python"
	_ "github.com/vedansh-5/graphcontext/pkg/lang/typescript"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func TestEnsureFreshIncremental(t *testing.T) {
	tmpDir := t.TempDir()

	f1 := filepath.Join(tmpDir, "db.go")
	if err := os.WriteFile(f1, []byte("package db\n\ntype DB struct{}\nfunc (d *DB) Write() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	f2 := filepath.Join(tmpDir, "svc.go")
	if err := os.WriteFile(f2, []byte("package svc\n\nimport \"db\"\n\ntype Svc struct { db *db.DB }\nfunc (s *Svc) Save() { s.db.Write() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	changed, err := EnsureFresh(tmpDir, s)
	if err != nil {
		t.Fatalf("EnsureFresh 1: %v", err)
	}
	if !changed {
		t.Errorf("EnsureFresh 1: expected changed=true on initial index")
	}

	nodes, err := s.AllNodes()
	if err != nil {
		t.Fatalf("AllNodes: %v", err)
	}
	if len(nodes) < 3 {
		t.Errorf("expected at least 3 nodes, got %d", len(nodes))
	}

	edges, err := s.AllEdges()
	if err != nil {
		t.Fatalf("AllEdges: %v", err)
	}
	if len(edges) == 0 {
		t.Errorf("expected resolved edges, got none")
	}

	changed, err = EnsureFresh(tmpDir, s)
	if err != nil {
		t.Fatalf("EnsureFresh 2: %v", err)
	}
	if changed {
		t.Errorf("EnsureFresh 2: expected changed=false on unchanged repo")
	}

	if err := os.WriteFile(f2, []byte("package svc\n\nimport \"db\"\n\ntype Svc struct { db *db.DB }\nfunc (s *Svc) Save() { s.db.Write() }\nfunc (s *Svc) Extra() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err = EnsureFresh(tmpDir, s)
	if err != nil {
		t.Fatalf("EnsureFresh 3: %v", err)
	}
	if !changed {
		t.Errorf("EnsureFresh 3: expected changed=true after file edit")
	}

	if err := os.Remove(f1); err != nil {
		t.Fatal(err)
	}

	changed, err = EnsureFresh(tmpDir, s)
	if err != nil {
		t.Fatalf("EnsureFresh 4: %v", err)
	}
	if !changed {
		t.Errorf("EnsureFresh 4: expected changed=true after file deletion")
	}
}

func TestEnsureFreshMultiLanguageProject(t *testing.T) {
	tmpDir := t.TempDir()

	pyFile := filepath.Join(tmpDir, "app.py")
	if err := os.WriteFile(pyFile, []byte("class App:\n    def run(self):\n        pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tsFile := filepath.Join(tmpDir, "client.ts")
	if err := os.WriteFile(tsFile, []byte("export class Client {\n  ping(): void {}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	goFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(goFile, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(t.TempDir(), "multi.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	changed, err := EnsureFresh(tmpDir, s)
	if err != nil {
		t.Fatalf("EnsureFresh: %v", err)
	}
	if !changed {
		t.Errorf("expected changed=true")
	}

	nodes, err := s.AllNodes()
	if err != nil {
		t.Fatalf("AllNodes: %v", err)
	}

	languages := make(map[string]bool)
	for _, n := range nodes {
		languages[n.Language] = true
	}

	if !languages["python"] || !languages["typescript"] || !languages["go"] {
		t.Errorf("expected nodes in python, typescript, and go; got languages: %+v", languages)
	}
}
