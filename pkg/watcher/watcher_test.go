package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDebounceCoalesceOperations(t *testing.T) {
	tests := []struct {
		name     string
		existing ChangeOp
		incoming ChangeOp
		wantOp   ChangeOp
		wantKeep bool
	}{
		{"empty + create", "", OpCreate, OpCreate, true},
		{"empty + modify", "", OpModify, OpModify, true},
		{"create + modify", OpCreate, OpModify, OpCreate, true},
		{"create + delete", OpCreate, OpDelete, "", false},
		{"modify + modify", OpModify, OpModify, OpModify, true},
		{"modify + delete", OpModify, OpDelete, OpDelete, true},
		{"delete + create", OpDelete, OpCreate, OpModify, true},
		{"delete + delete", OpDelete, OpDelete, OpDelete, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			op, keep := coalesceOperations(tc.existing, tc.incoming)
			if op != tc.wantOp || keep != tc.wantKeep {
				t.Fatalf("coalesce(%q, %q) = (%q, %v), want (%q, %v)",
					tc.existing, tc.incoming, op, keep, tc.wantOp, tc.wantKeep)
			}
		})
	}
}

func TestDebouncerFlush(t *testing.T) {
	flushCh := make(chan EventBatch, 10)
	d := newDebouncer(50*time.Millisecond, flushCh)
	defer d.stop()

	d.record(FileChange{RelPath: "b.go", Op: OpModify})
	d.record(FileChange{RelPath: "a.go", Op: OpCreate})
	d.record(FileChange{RelPath: "b.go", Op: OpModify})

	select {
	case batch := <-flushCh:
		if len(batch.Changes) != 2 {
			t.Fatalf("expected 2 changes in batch, got %d", len(batch.Changes))
		}
		if batch.Changes[0].RelPath != "a.go" || batch.Changes[0].Op != OpCreate {
			t.Errorf("expected a.go create, got %+v", batch.Changes[0])
		}
		if batch.Changes[1].RelPath != "b.go" || batch.Changes[1].Op != OpModify {
			t.Errorf("expected b.go modify, got %+v", batch.Changes[1])
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for debouncer flush")
	}
}

func TestFilterLogic(t *testing.T) {
	if !shouldIgnoreDir(".git", nil) {
		t.Errorf("expected .git to be ignored")
	}
	if !shouldIgnoreDir("node_modules", nil) {
		t.Errorf("expected node_modules to be ignored")
	}
	if shouldIgnoreDir("pkg", nil) {
		t.Errorf("expected pkg not to be ignored")
	}

	custom := map[string]bool{"custom_ignore": true}
	if !shouldIgnoreDir("custom_ignore", custom) {
		t.Errorf("expected custom_ignore to be ignored")
	}

	if !isAcceptedCodeFile("foo.go", nil) {
		t.Errorf("expected foo.go to be accepted")
	}
	if !isAcceptedCodeFile("bar.py", nil) {
		t.Errorf("expected bar.py to be accepted")
	}
	if !isAcceptedCodeFile("baz.ts", nil) {
		t.Errorf("expected baz.ts to be accepted")
	}
	if isAcceptedCodeFile("readme.md", nil) {
		t.Errorf("expected readme.md to be rejected")
	}
	if isAcceptedCodeFile("image.png", nil) {
		t.Errorf("expected image.png to be rejected")
	}
}

func TestWatcherLiveLifecycle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "watcher_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	w, err := New(Config{
		RepoRoot:         tmpDir,
		DebounceDuration: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New watcher failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	batchCh, _ := w.Start(ctx)

	filePath := filepath.Join(tmpDir, "service.go")
	if err := os.WriteFile(filePath, []byte("package test\nfunc A() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	select {
	case batch, ok := <-batchCh:
		if !ok {
			t.Fatal("batch channel closed prematurely")
		}
		if len(batch.Changes) == 0 {
			t.Fatal("expected at least 1 change in batch")
		}
		found := false
		for _, c := range batch.Changes {
			if c.RelPath == "service.go" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected service.go in batch, got %+v", batch.Changes)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for file creation event")
	}

	if err := os.WriteFile(filePath, []byte("package test\nfunc A() { println(1) }\n"), 0644); err != nil {
		t.Fatalf("failed to modify test file: %v", err)
	}

	select {
	case batch, ok := <-batchCh:
		if !ok {
			t.Fatal("batch channel closed prematurely")
		}
		found := false
		for _, c := range batch.Changes {
			if c.RelPath == "service.go" && c.Op == OpModify {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected service.go modify in batch, got %+v", batch.Changes)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for file modification event")
	}

	if err := os.Remove(filePath); err != nil {
		t.Fatalf("failed to remove test file: %v", err)
	}

	select {
	case batch, ok := <-batchCh:
		if !ok {
			t.Fatal("batch channel closed prematurely")
		}
		found := false
		for _, c := range batch.Changes {
			if c.RelPath == "service.go" && c.Op == OpDelete {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected service.go delete in batch, got %+v", batch.Changes)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for file deletion event")
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestWatcherIgnoresExcluded(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "watcher_test_ignore_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, ".git")
	_ = os.MkdirAll(gitDir, 0755)

	w, err := New(Config{
		RepoRoot:         tmpDir,
		DebounceDuration: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New watcher failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	batchCh, _ := w.Start(ctx)

	_ = os.WriteFile(filepath.Join(gitDir, "ignored.go"), []byte("package git\n"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "notes.txt"), []byte("hello"), 0644)

	select {
	case batch := <-batchCh:
		t.Fatalf("unexpected batch received for ignored paths: %+v", batch)
	case <-time.After(300 * time.Millisecond):
	}

	_ = w.Close()
}

func TestWatcherNewSubdirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "watcher_test_subdir_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	w, err := New(Config{
		RepoRoot:         tmpDir,
		DebounceDuration: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New watcher failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	batchCh, _ := w.Start(ctx)

	newSubdir := filepath.Join(tmpDir, "pkg", "sub")
	if err := os.MkdirAll(newSubdir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	// Give the watcher a moment to register the new directory
	time.Sleep(100 * time.Millisecond)

	newFilePath := filepath.Join(newSubdir, "worker.go")
	if err := os.WriteFile(newFilePath, []byte("package sub\n"), 0644); err != nil {
		t.Fatalf("failed to write file in subdir: %v", err)
	}

	select {
	case batch, ok := <-batchCh:
		if !ok {
			t.Fatal("batch channel closed prematurely")
		}
		found := false
		for _, c := range batch.Changes {
			if c.RelPath == "pkg/sub/worker.go" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected pkg/sub/worker.go in batch, got %+v", batch.Changes)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event in dynamically added subdirectory")
	}

	_ = w.Close()
}
