package daemon

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vedansh-5/graphcontext/pkg/analysis"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func createTestRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "daemon_test_repo_*")
	if err != nil {
		t.Fatalf("failed to create temp repo: %v", err)
	}

	mainContent := `package main

func Entrypoint() {
	Helper()
}

func Helper() {
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}
	return dir
}

func TestDaemonLifecycleAndLiveSync(t *testing.T) {
	repoDir := createTestRepo(t)
	defer os.RemoveAll(repoDir)

	dbFile := filepath.Join(repoDir, "test.db")
	st, err := store.Open(dbFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	d, err := New(Config{
		RepoRoot:         repoDir,
		Store:            st,
		DebounceDuration: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New daemon failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start daemon failed: %v", err)
	}

	syncCh := d.SubscribeSync()
	defer d.UnsubscribeSync(syncCh)

	status := d.Status()
	if !status.IsWatching {
		t.Errorf("expected IsWatching to be true")
	}
	if status.NodeCount == 0 {
		t.Errorf("expected NodeCount > 0, got %d", status.NodeCount)
	}

	var foundEntrypoint bool
	err = d.WithGraph(func(g *analysis.Graph) error {
		for _, n := range g.Nodes {
			if n.Name == "Entrypoint" {
				foundEntrypoint = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithGraph failed: %v", err)
	}
	if !foundEntrypoint {
		t.Fatal("expected Entrypoint node in initial graph")
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = d.WithGraph(func(g *analysis.Graph) error {
				_ = g.NodeCount()
				_ = g.EdgeCount()
				return nil
			})
		}()
	}
	wg.Wait()

	newFile := filepath.Join(repoDir, "extra.go")
	extraContent := `package main

func ExtraCalculation() {
}
`
	if err := os.WriteFile(newFile, []byte(extraContent), 0644); err != nil {
		t.Fatalf("failed to write extra.go: %v", err)
	}

	select {
	case evt, ok := <-syncCh:
		if !ok {
			t.Fatal("sync channel closed unexpectedly")
		}
		if !evt.Changed {
			t.Errorf("expected Changed to be true after file addition")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for sync event after file creation")
	}

	var foundExtra bool
	_ = d.WithGraph(func(g *analysis.Graph) error {
		for _, n := range g.Nodes {
			if n.Name == "ExtraCalculation" {
				foundExtra = true
			}
		}
		return nil
	})
	if !foundExtra {
		t.Fatal("expected ExtraCalculation in hot graph after live update")
	}

	if err := os.Remove(newFile); err != nil {
		t.Fatalf("failed to remove extra.go: %v", err)
	}

	select {
	case evt, ok := <-syncCh:
		if !ok {
			t.Fatal("sync channel closed unexpectedly")
		}
		if !evt.Changed {
			t.Errorf("expected Changed to be true after file deletion")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for sync event after file deletion")
	}

	foundExtra = false
	_ = d.WithGraph(func(g *analysis.Graph) error {
		for _, n := range g.Nodes {
			if n.Name == "ExtraCalculation" {
				foundExtra = true
			}
		}
		return nil
	})
	if foundExtra {
		t.Fatal("expected ExtraCalculation to be removed from hot graph after file deletion")
	}

	if err := d.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if d.Status().IsWatching {
		t.Errorf("expected IsWatching to be false after Stop")
	}
}

func TestDaemonManualSyncNow(t *testing.T) {
	repoDir := createTestRepo(t)
	defer os.RemoveAll(repoDir)

	dbFile := filepath.Join(repoDir, "test_manual.db")
	st, err := store.Open(dbFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	d, err := New(Config{
		RepoRoot:         repoDir,
		Store:            st,
		DebounceDuration: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New daemon failed: %v", err)
	}
	defer d.Stop()

	changed, err := d.SyncNow()
	if err != nil {
		t.Fatalf("SyncNow failed: %v", err)
	}
	if !changed {
		t.Errorf("expected initial SyncNow to report changed=true")
	}

	changedAgain, err := d.SyncNow()
	if err != nil {
		t.Fatalf("second SyncNow failed: %v", err)
	}
	if changedAgain {
		t.Errorf("expected second SyncNow without file changes to report changed=false")
	}
}

// FreshGraph must reflect an edit before the debounced background sync runs,
// and must not re-check the disk when nothing has changed.
func TestFreshGraphReadsYourWrites(t *testing.T) {
	repoDir := createTestRepo(t)
	defer os.RemoveAll(repoDir)

	// Keep the database outside the repo so its writes are not seen as changes.
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	// A debounce this long means the background sync never fires in the test.
	d, err := New(Config{RepoRoot: repoDir, Store: st, DebounceDuration: time.Hour})
	if err != nil {
		t.Fatalf("New daemon failed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer d.Stop()

	g, err := d.FreshGraph()
	if err != nil {
		t.Fatalf("FreshGraph: %v", err)
	}
	before := g.NodeCount()

	syncs := d.Status().SyncCount
	for i := 0; i < 5; i++ {
		if _, err := d.FreshGraph(); err != nil {
			t.Fatalf("FreshGraph: %v", err)
		}
	}
	if got := d.Status().SyncCount; got != syncs {
		t.Errorf("FreshGraph synced %d times with nothing changed, want 0", got-syncs)
	}

	extra := "package main\n\nfunc Extra() {\n\tHelper()\n}\n"
	if err := os.WriteFile(filepath.Join(repoDir, "extra.go"), []byte(extra), 0644); err != nil {
		t.Fatalf("failed to write extra.go: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		g, err := d.FreshGraph()
		if err != nil {
			t.Fatalf("FreshGraph: %v", err)
		}
		if g.NodeCount() > before {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("new file never appeared: still %d nodes", g.NodeCount())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
