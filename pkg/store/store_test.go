package store

import (
	"fmt"
	"path/filepath"
	"reflect"
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
	if want := fmt.Sprint(SchemaVersion); v != want {
		t.Errorf("schema_version = %q, want %s", v, want)
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
	if v, _ := s2.Meta("schema_version"); v != fmt.Sprint(SchemaVersion) {
		t.Errorf("schema_version = %q, want %d", v, SchemaVersion)
	}
}

// Compile-time proof the shared types exist with the expected field names.
var _ = FileRecord{Path: "a", ContentHash: "b", IndexedAt: time.Now()}
var _ = Edge{SourceID: "a", TargetID: "b", Kind: EdgeCalls, Confidence: ConfExact}

func TestSplitIdentifier(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"checkRateLimit", []string{"check", "rate", "limit"}},
		{"check_rate_limit", []string{"check", "rate", "limit"}},
		{"CheckRateLimit", []string{"check", "rate", "limit"}},
		{"parseHTTPResponse", []string{"parse", "http", "response"}},
		{"AuthService.authenticate_user", []string{"auth", "service", "authenticate", "user"}},
		{"HTTPServer", []string{"http", "server"}},
		{"main", []string{"main"}},
		{"", nil},
	}
	for _, c := range cases {
		got := SplitIdentifier(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitIdentifier(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCommitAndRead(t *testing.T) {
	s := newTestStore(t)

	b := NewBatch()
	b.TouchFile(FileRecord{Path: "svc.py", ContentHash: "h1", IndexedAt: time.Now()})
	b.AddNode(fn("svc.py:login", "login", "svc.login", "svc.py"))
	b.AddNode(fn("svc.py:auth", "auth", "svc.auth", "svc.py"))
	b.AddEdge(Edge{
		SourceID: "svc.py:login", TargetID: "svc.py:auth",
		Kind: EdgeCalls, Line: 12, Confidence: ConfExact, CandidateCount: 1,
	})
	if err := s.Commit(b); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	n, err := s.Node("svc.py:login")
	if err != nil || n == nil {
		t.Fatalf("Node: %v (nil=%v)", err, n == nil)
	}
	if n.QualifiedName != "svc.login" || n.Kind != KindFunction {
		t.Errorf("roundtrip mismatch: %+v", n)
	}

	out, err := s.OutEdges("svc.py:login")
	if err != nil {
		t.Fatalf("OutEdges: %v", err)
	}
	if len(out) != 1 || out[0].TargetID != "svc.py:auth" || out[0].Confidence != ConfExact {
		t.Fatalf("OutEdges = %+v", out)
	}

	in, err := s.InEdges("svc.py:auth")
	if err != nil {
		t.Fatalf("InEdges: %v", err)
	}
	if len(in) != 1 || in[0].SourceID != "svc.py:login" {
		t.Fatalf("InEdges = %+v", in)
	}

	missing, err := s.Node("nope")
	if err != nil || missing != nil {
		t.Errorf("missing node should be (nil,nil), got (%v,%v)", missing, err)
	}
}

func TestTouchFileReplacesNodes(t *testing.T) {
	s := newTestStore(t)

	b := NewBatch()
	b.TouchFile(FileRecord{Path: "a.py", ContentHash: "h1", IndexedAt: time.Now()})
	b.AddNode(fn("a.py:old", "old", "a.old", "a.py"))
	if err := s.Commit(b); err != nil {
		t.Fatalf("first commit: %v", err)
	}

	// Re-index the same file with different contents.
	b2 := NewBatch()
	b2.TouchFile(FileRecord{Path: "a.py", ContentHash: "h2", IndexedAt: time.Now()})
	b2.AddNode(fn("a.py:new", "new", "a.new", "a.py"))
	if err := s.Commit(b2); err != nil {
		t.Fatalf("second commit: %v", err)
	}

	nodes, err := s.NodesInFile("a.py")
	if err != nil {
		t.Fatalf("NodesInFile: %v", err)
	}
	if len(nodes) != 1 || nodes[0].ID != "a.py:new" {
		t.Fatalf("re-index should replace, got %+v", nodes)
	}

	hashes, err := s.FileHashes()
	if err != nil {
		t.Fatalf("FileHashes: %v", err)
	}
	if hashes["a.py"] != "h2" {
		t.Errorf("hash = %q, want h2", hashes["a.py"])
	}
}

func TestRemoveFileCascadesEdges(t *testing.T) {
	s := newTestStore(t)

	b := NewBatch()
	b.TouchFile(FileRecord{Path: "a.py", ContentHash: "h", IndexedAt: time.Now()})
	b.TouchFile(FileRecord{Path: "b.py", ContentHash: "h", IndexedAt: time.Now()})
	b.AddNode(fn("a.py:caller", "caller", "a.caller", "a.py"))
	b.AddNode(fn("b.py:callee", "callee", "b.callee", "b.py"))
	b.AddEdge(Edge{SourceID: "a.py:caller", TargetID: "b.py:callee",
		Kind: EdgeCalls, Line: 1, Confidence: ConfExact})
	if err := s.Commit(b); err != nil {
		t.Fatalf("commit: %v", err)
	}

	del := NewBatch()
	del.RemoveFile("b.py")
	if err := s.Commit(del); err != nil {
		t.Fatalf("remove commit: %v", err)
	}

	edges, err := s.AllEdges()
	if err != nil {
		t.Fatalf("AllEdges: %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("edge should cascade away, got %+v", edges)
	}
	hashes, _ := s.FileHashes()
	if _, ok := hashes["b.py"]; ok {
		t.Error("file record should be gone")
	}
}

func TestConfidenceCounts(t *testing.T) {
	s := newTestStore(t)

	b := NewBatch()
	b.TouchFile(FileRecord{Path: "a.py", ContentHash: "h", IndexedAt: time.Now()})
	for _, id := range []string{"a", "b", "c"} {
		b.AddNode(fn("a.py:"+id, id, "a."+id, "a.py"))
	}
	b.AddEdge(Edge{SourceID: "a.py:a", TargetID: "a.py:b", Kind: EdgeCalls,
		Line: 1, Confidence: ConfExact})
	b.AddEdge(Edge{SourceID: "a.py:a", TargetID: "a.py:c", Kind: EdgeCalls,
		Line: 2, Confidence: ConfAmbiguous, CandidateCount: 2})
	if err := s.Commit(b); err != nil {
		t.Fatalf("commit: %v", err)
	}

	counts, err := s.ConfidenceCounts()
	if err != nil {
		t.Fatalf("ConfidenceCounts: %v", err)
	}
	if counts[ConfExact] != 1 || counts[ConfAmbiguous] != 1 {
		t.Errorf("counts = %+v", counts)
	}
}

func TestOrderingIsDeterministic(t *testing.T) {
	s := newTestStore(t)

	b := NewBatch()
	b.TouchFile(FileRecord{Path: "a.py", ContentHash: "h", IndexedAt: time.Now()})
	for _, id := range []string{"z", "m", "a"} {
		b.AddNode(fn("a.py:"+id, id, "a."+id, "a.py"))
	}
	if err := s.Commit(b); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var first []string
	for i := 0; i < 5; i++ {
		nodes, err := s.AllNodes()
		if err != nil {
			t.Fatalf("AllNodes: %v", err)
		}
		var ids []string
		for _, n := range nodes {
			ids = append(ids, n.ID)
		}
		if i == 0 {
			first = ids
			continue
		}
		if !reflect.DeepEqual(ids, first) {
			t.Fatalf("ordering not deterministic: %v vs %v", ids, first)
		}
	}
	want := []string{"a.py:a", "a.py:m", "a.py:z"}
	if !reflect.DeepEqual(first, want) {
		t.Errorf("AllNodes order = %v, want %v", first, want)
	}
}

func TestSearchMatchesSubwords(t *testing.T) {
	s := newTestStore(t)

	b := NewBatch()
	b.TouchFile(FileRecord{Path: "l.go", ContentHash: "h", IndexedAt: time.Now()})
	n := fn("l.go:checkRateLimit", "checkRateLimit", "limiter.checkRateLimit", "l.go")
	n.Docstring = "Rejects requests over quota."
	b.AddNode(n)
	b.AddNode(fn("l.go:unrelated", "unrelated", "limiter.unrelated", "l.go"))
	if err := s.Commit(b); err != nil {
		t.Fatalf("commit: %v", err)
	}

	for _, q := range []string{"rate limit", "checkRateLimit", "check_rate_limit", "rate"} {
		hits, err := s.Search(q, 10)
		if err != nil {
			t.Fatalf("Search(%q): %v", q, err)
		}
		if len(hits) == 0 || hits[0].NodeID != "l.go:checkRateLimit" {
			t.Errorf("Search(%q) = %+v, want checkRateLimit first", q, hits)
		}
	}

	// FTS operators in user input must not break the query.
	if _, err := s.Search(`bad" OR "x`, 10); err != nil {
		t.Errorf("hostile query should not error: %v", err)
	}
}

func TestSearchDropsPurgedNodes(t *testing.T) {
	s := newTestStore(t)

	b := NewBatch()
	b.TouchFile(FileRecord{Path: "a.go", ContentHash: "h", IndexedAt: time.Now()})
	b.AddNode(fn("a.go:findWidget", "findWidget", "a.findWidget", "a.go"))
	if err := s.Commit(b); err != nil {
		t.Fatalf("commit: %v", err)
	}

	del := NewBatch()
	del.RemoveFile("a.go")
	if err := s.Commit(del); err != nil {
		t.Fatalf("remove: %v", err)
	}

	hits, err := s.Search("find widget", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("purged node still searchable: %+v", hits)
	}
}

func TestSearchReindexReplacesTokens(t *testing.T) {
	s := newTestStore(t)

	// Index the same file twice, renaming its only function in between. An
	// external node sits in the same batch so node and FTS rowids diverge
	// unless the FTS row is keyed to its node.
	for _, name := range []string{"findWidget", "loadGadget"} {
		b := NewBatch()
		b.TouchFile(FileRecord{Path: "a.go", ContentHash: name, IndexedAt: time.Now()})
		b.AddNode(Node{ID: "external:fmt", Kind: KindExternal, Name: "fmt", QualifiedName: "fmt"})
		b.AddNode(fn("a.go:"+name, name, "a."+name, "a.go"))
		if err := s.Commit(b); err != nil {
			t.Fatalf("commit %s: %v", name, err)
		}
	}

	if hits, err := s.Search("find widget", 10); err != nil || len(hits) != 0 {
		t.Errorf("old name still searchable: hits=%+v err=%v", hits, err)
	}
	hits, err := s.Search("load gadget", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("want exactly 1 hit for the new name, got %+v", hits)
	}

	// Re-committing an unchanged node must not duplicate its FTS row.
	b := NewBatch()
	b.AddNode(fn("a.go:loadGadget", "loadGadget", "a.loadGadget", "a.go"))
	if err := s.Commit(b); err != nil {
		t.Fatalf("recommit: %v", err)
	}
	if hits, _ := s.Search("load gadget", 10); len(hits) != 1 {
		t.Errorf("want 1 hit after recommit, got %+v", hits)
	}
}

func TestBatchTargetedDeletes(t *testing.T) {
	s := newTestStore(t)

	b := NewBatch()
	b.TouchFile(FileRecord{Path: "a.go", ContentHash: "h1", IndexedAt: time.Now()})
	b.AddNode(fn("a.go:keep", "keep", "a.keep", "a.go"))
	b.AddNode(fn("a.go:drop", "drop", "a.drop", "a.go"))
	b.AddNode(fn("a.go:other", "other", "a.other", "a.go"))
	keepDrop := Edge{SourceID: "a.go:keep", TargetID: "a.go:drop", Kind: EdgeCalls, Line: 1, Confidence: ConfExact}
	keepOther := Edge{SourceID: "a.go:keep", TargetID: "a.go:other", Kind: EdgeCalls, Line: 2, Confidence: ConfExact}
	otherKeep := Edge{SourceID: "a.go:other", TargetID: "a.go:keep", Kind: EdgeCalls, Line: 3, Confidence: ConfExact}
	b.AddEdge(keepDrop)
	b.AddEdge(keepOther)
	b.AddEdge(otherKeep)
	if err := s.Commit(b); err != nil {
		t.Fatalf("commit: %v", err)
	}

	d := NewBatch()
	d.RecordFile(FileRecord{Path: "a.go", ContentHash: "h2", IndexedAt: time.Now()})
	d.DeleteNode("a.go:drop")
	d.DeleteEdge(keepOther)
	if err := s.Commit(d); err != nil {
		t.Fatalf("commit deletes: %v", err)
	}

	nodes, _ := s.AllNodes()
	if len(nodes) != 2 {
		t.Errorf("want 2 nodes left, got %+v", nodes)
	}
	// keep->drop went with its node, keep->other was deleted directly.
	edges, _ := s.AllEdges()
	if len(edges) != 1 || edges[0].SourceID != "a.go:other" {
		t.Errorf("want only other->keep left, got %+v", edges)
	}
	if hits, _ := s.Search("drop", 10); len(hits) != 0 {
		t.Errorf("deleted node still searchable: %+v", hits)
	}
	if h, _ := s.FileHashes(); h["a.go"] != "h2" {
		t.Errorf("file hash = %q, want h2", h["a.go"])
	}
}
