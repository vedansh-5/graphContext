package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// fn builds a minimal function node for tests.
func fn(id, name, qname, file string) Node {
	return Node{
		ID: id, Kind: KindFunction, Name: name, QualifiedName: qname,
		FilePath: file, Language: "python", Visibility: VisExported,
	}
}

func TestOpenAppliesSchema(t *testing.T) {
	s := newTestStore(t)
	v, err := s.Meta("schema_version")
	if err != nil {
		t.Fatalf("Meta: %v", err)
	}
	if v != "3" {
		t.Errorf("schema_version = %q, want 3", v)
	}
}

func TestMetaRoundTrip(t *testing.T) {
	s := newTestStore(t)

	missing, err := s.Meta("nope")
	if err != nil {
		t.Fatalf("Meta on missing key should not error: %v", err)
	}
	if missing != "" {
		t.Errorf("missing key = %q, want empty", missing)
	}

	if err := s.SetMeta("indexed_commit", "abc123"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	if err := s.SetMeta("indexed_commit", "def456"); err != nil {
		t.Fatalf("SetMeta overwrite: %v", err)
	}
	got, _ := s.Meta("indexed_commit")
	if got != "def456" {
		t.Errorf("indexed_commit = %q, want def456", got)
	}
}

func TestCachePathIsOutsideRepo(t *testing.T) {
	repo := t.TempDir()
	p, err := CachePathFor(repo)
	if err != nil {
		t.Fatalf("CachePathFor: %v", err)
	}
	if filepath.Dir(p) == repo {
		t.Errorf("db must not live inside the repo: %s", p)
	}
	p2, _ := CachePathFor(repo)
	if p != p2 {
		t.Errorf("CachePathFor must be stable: %s vs %s", p, p2)
	}
}

func TestSchemaVersionMismatchRebuilds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "graph.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.SetMeta("marker", "survives?"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	// Simulate a database written by an older engine.
	if err := s.SetMeta("schema_version", "1"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	if m, _ := s2.Meta("marker"); m != "" {
		t.Errorf("stale-schema db should be rebuilt empty, marker = %q", m)
	}
	if v, _ := s2.Meta("schema_version"); v != "3" {
		t.Errorf("schema_version = %q, want 3", v)
	}
}

// Compile-time proof the shared types exist with the expected field names.
var _ = FileRecord{Path: "a", ContentHash: "b", IndexedAt: time.Now()}
var _ = Edge{SourceID: "a", TargetID: "b", Kind: EdgeCalls, Confidence: ConfExact}
