package mcp_server

import (
	"path/filepath"
	"testing"
	"time"
)

func sessionOpen(t *testing.T, projDir string) bool {
	t.Helper()
	abs, err := filepath.Abs(projDir)
	if err != nil {
		t.Fatal(err)
	}
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	_, ok := sessions[abs]
	return ok
}

func TestIdleSessionEviction(t *testing.T) {
	projDir := setupTestProject(t)
	t.Cleanup(closeAllSessions)
	const ttl = 30 * time.Minute

	sess, release, err := acquireSession(projDir)
	if err != nil {
		t.Fatalf("acquireSession: %v", err)
	}
	nodes := sess.graph.NodeCount()
	if nodes == 0 {
		t.Fatal("expected an indexed graph")
	}

	// Idle long enough, but a tool call is still running against it.
	later := time.Now().Add(2 * ttl)
	evictIdleSessions(later, ttl)
	if !sessionOpen(t, projDir) {
		t.Fatal("session evicted while in use")
	}

	release()

	// Released but used recently.
	evictIdleSessions(time.Now(), ttl)
	if !sessionOpen(t, projDir) {
		t.Fatal("recently used session evicted")
	}

	evictIdleSessions(time.Now().Add(2*ttl), ttl)
	if sessionOpen(t, projDir) {
		t.Fatal("idle session not evicted")
	}

	// The next call reopens the project from its cache.
	sess, err = getSession(projDir)
	if err != nil {
		t.Fatalf("getSession after eviction: %v", err)
	}
	if got := sess.graph.NodeCount(); got != nodes {
		t.Errorf("reopened graph has %d nodes, want %d", got, nodes)
	}
}

func TestCloseAllSessions(t *testing.T) {
	projDir := setupTestProject(t)
	if _, err := getSession(projDir); err != nil {
		t.Fatalf("getSession: %v", err)
	}
	closeAllSessions()
	if sessionOpen(t, projDir) {
		t.Fatal("session still open after closeAllSessions")
	}
}

func TestIdleTTL(t *testing.T) {
	t.Setenv("GRAPHCONTEXT_IDLE_TTL", "")
	if got := idleTTL(); got != defaultIdleTTL {
		t.Errorf("unset: got %v, want %v", got, defaultIdleTTL)
	}
	t.Setenv("GRAPHCONTEXT_IDLE_TTL", "90s")
	if got := idleTTL(); got != 90*time.Second {
		t.Errorf("90s: got %v", got)
	}
	t.Setenv("GRAPHCONTEXT_IDLE_TTL", "soon")
	if got := idleTTL(); got != defaultIdleTTL {
		t.Errorf("invalid: got %v, want %v", got, defaultIdleTTL)
	}
}
