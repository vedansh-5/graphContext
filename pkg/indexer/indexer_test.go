package indexer

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	_ "github.com/vedansh-5/graphcontext/pkg/lang/golang"
	_ "github.com/vedansh-5/graphcontext/pkg/lang/python"
	_ "github.com/vedansh-5/graphcontext/pkg/lang/typescript"
	"github.com/vedansh-5/graphcontext/pkg/resolver"
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

// An incrementally updated store must end up identical to one indexed from
// scratch, since re-indexing writes only the rows that differ.
func TestEnsureFreshDeltaMatchesFullIndex(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	open := func(name string) *store.Store {
		t.Helper()
		s, err := store.Open(filepath.Join(t.TempDir(), name))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}

	write("db.go", "package app\n\ntype DB struct{}\n\nfunc (d *DB) Write() {}\n\nfunc (d *DB) Read() {}\n")
	write("svc.go", "package app\n\ntype Svc struct{ db *DB }\n\nfunc (s *Svc) Save() { s.db.Write() }\n")
	write("old.go", "package app\n\nfunc Legacy() { helper() }\n\nfunc helper() {}\n")

	incremental := open("inc.db")
	if _, err := EnsureFresh(dir, incremental); err != nil {
		t.Fatalf("initial index: %v", err)
	}

	// Edit one file, delete one, add one.
	write("svc.go", "package app\n\ntype Svc struct{ db *DB }\n\n\nfunc (s *Svc) Load() { s.db.Read() }\n")
	if err := os.Remove(filepath.Join(dir, "old.go")); err != nil {
		t.Fatal(err)
	}
	write("api.go", "package app\n\nfunc Handle(s *Svc) { s.Load() }\n")

	if changed, err := EnsureFresh(dir, incremental); err != nil || !changed {
		t.Fatalf("incremental index: changed=%v err=%v", changed, err)
	}

	full := open("full.db")
	if _, err := EnsureFresh(dir, full); err != nil {
		t.Fatalf("full index: %v", err)
	}

	gotNodes, _ := incremental.AllNodes()
	wantNodes, _ := full.AllNodes()
	if !reflect.DeepEqual(gotNodes, wantNodes) {
		t.Errorf("nodes differ:\n got  %+v\n want %+v", gotNodes, wantNodes)
	}
	gotEdges, _ := incremental.AllEdges()
	wantEdges, _ := full.AllEdges()
	if !reflect.DeepEqual(gotEdges, wantEdges) {
		t.Errorf("edges differ:\n got  %+v\n want %+v", gotEdges, wantEdges)
	}
	gotFiles, _ := incremental.FileHashes()
	wantFiles, _ := full.FileHashes()
	if !reflect.DeepEqual(gotFiles, wantFiles) {
		t.Errorf("file records differ:\n got  %+v\n want %+v", gotFiles, wantFiles)
	}

	if hits, _ := incremental.Search("legacy", 10); len(hits) != 0 {
		t.Errorf("deleted symbol still searchable: %+v", hits)
	}
	if hits, _ := incremental.Search("save", 10); len(hits) != 0 {
		t.Errorf("renamed-away symbol still searchable: %+v", hits)
	}
	if hits, _ := incremental.Search("load", 10); len(hits) != 1 {
		t.Errorf("want 1 hit for new symbol, got %+v", hits)
	}
}

// Editing one file must queue a small diff, not the whole graph.
func TestQueueGraphDiffIsMinimal(t *testing.T) {
	dir := t.TempDir()
	writeSyntheticRepo(t, dir, 50)
	s := benchStore(t)
	if _, err := EnsureFresh(dir, s); err != nil {
		t.Fatal(err)
	}

	nodes, _ := s.AllNodes()
	edges, _ := s.AllEdges()
	batch := store.NewBatch()
	if err := queueGraphDiff(s, batch, &resolver.ResolutionResult{Nodes: nodes, Edges: edges}); err != nil {
		t.Fatal(err)
	}
	if n, e := batch.Len(); n != 0 || e != 0 {
		t.Errorf("unchanged graph queued %d nodes and %d edges, want 0", n, e)
	}
}

func TestEnsureFreshStatSkip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	past := time.Now().Add(-time.Hour)
	write := func(src string, mtime time.Time) {
		t.Helper()
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	names := func(s *store.Store) map[string]bool {
		t.Helper()
		nodes, err := s.AllNodes()
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, n := range nodes {
			out[n.Name] = true
		}
		return out
	}

	s := benchStore(t)
	write("package a\n\nfunc One() {}\n", past)
	if changed, err := EnsureFresh(dir, s); err != nil || !changed {
		t.Fatalf("initial index: changed=%v err=%v", changed, err)
	}

	// Same size, same mtime: the stat matches, so the file is not read. This
	// is the trade-off that makes the check cheap, and the same one git makes.
	write("package a\n\nfunc Two() {}\n", past)
	if changed, err := EnsureFresh(dir, s); err != nil || changed {
		t.Fatalf("stat-identical file: changed=%v err=%v, want unchanged", changed, err)
	}

	// Same size, new mtime: must be re-hashed and picked up.
	write("package a\n\nfunc Two() {}\n", past.Add(time.Minute))
	if changed, err := EnsureFresh(dir, s); err != nil || !changed {
		t.Fatalf("same-size edit: changed=%v err=%v, want changed", changed, err)
	}
	if got := names(s); !got["Two"] || got["One"] {
		t.Errorf("after same-size edit, nodes = %v", got)
	}

	// Same content, new mtime: not a change, and the record is refreshed.
	newer := past.Add(2 * time.Minute)
	write("package a\n\nfunc Two() {}\n", newer)
	if changed, err := EnsureFresh(dir, s); err != nil || changed {
		t.Fatalf("touched file: changed=%v err=%v, want unchanged", changed, err)
	}
	recs, err := s.FileRecords()
	if err != nil {
		t.Fatal(err)
	}
	if recs["a.go"].ModTimeNs != newer.UnixNano() {
		t.Errorf("record mtime not refreshed: got %d, want %d", recs["a.go"].ModTimeNs, newer.UnixNano())
	}
}

// A file written in the same second it was indexed cannot be trusted by stat
// alone, because another edit in that second may leave its mtime unchanged.
func TestStatUnchangedDistrustsRecentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.go")
	if err := os.WriteFile(path, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	rec := store.FileRecord{ContentHash: "h", Size: info.Size(), ModTimeNs: info.ModTime().UnixNano()}

	rec.IndexedAt = info.ModTime()
	if statUnchanged(rec, info) {
		t.Error("file indexed in the second it was written must be re-hashed")
	}
	rec.IndexedAt = info.ModTime().Add(2 * time.Second)
	if !statUnchanged(rec, info) {
		t.Error("file indexed well after its last write should be trusted")
	}
}
