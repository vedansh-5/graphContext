package watcher

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	cfg         Config
	fsWatcher   *fsnotify.Watcher
	debouncer   *debouncer
	batches     chan EventBatch
	errors      chan error
	done        chan struct{}
	watchedDirs map[string]bool
	closed      sync.Once
	mu          sync.RWMutex
	// seq counts filesystem events seen outside ignored directories.
	seq atomic.Uint64
	// degraded is set once the watcher may have missed a change.
	degraded atomic.Bool
}

// Seq returns a counter that moves every time something changes on disk under
// the repo, outside ignored directories. A reader that remembers the value it
// last synced at can tell, without touching the disk, whether it is stale.
func (w *Watcher) Seq() uint64 { return w.seq.Load() }

// Healthy reports whether Seq can be trusted. It turns false for good if a
// directory could not be watched or the OS reported an error such as a dropped
// event, because a change may then have gone unseen.
func (w *Watcher) Healthy() bool { return !w.degraded.Load() }

func New(cfg Config) (*Watcher, error) {
	if cfg.RepoRoot == "" {
		return nil, fmt.Errorf("watcher: repo root cannot be empty")
	}

	absRoot, err := filepath.Abs(cfg.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("watcher: resolve repo root: %w", err)
	}
	if realRoot, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = realRoot
	}
	cfg.RepoRoot = absRoot

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("watcher: initialize fsnotify: %w", err)
	}

	batchCh := make(chan EventBatch, 64)
	errCh := make(chan error, 64)

	w := &Watcher{
		cfg:         cfg,
		fsWatcher:   fsw,
		debouncer:   newDebouncer(cfg.DebounceDuration, batchCh),
		batches:     batchCh,
		errors:      errCh,
		done:        make(chan struct{}),
		watchedDirs: make(map[string]bool),
	}

	if err := w.watchTree(cfg.RepoRoot); err != nil {
		_ = fsw.Close()
		return nil, err
	}

	return w, nil
}

func (w *Watcher) Start(ctx context.Context) (<-chan EventBatch, <-chan error) {
	go w.eventLoop(ctx)
	return w.batches, w.errors
}

func (w *Watcher) Close() error {
	var closeErr error
	w.closed.Do(func() {
		close(w.done)
		w.debouncer.stop()
		closeErr = w.fsWatcher.Close()
	})
	return closeErr
}

func (w *Watcher) watchTree(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if shouldIgnoreDir(d.Name(), w.cfg.IgnoredDirs) {
				return fs.SkipDir
			}
			w.mu.Lock()
			if !w.watchedDirs[path] {
				if addErr := w.fsWatcher.Add(path); addErr == nil {
					w.watchedDirs[path] = true
				} else {
					w.degraded.Store(true)
				}
			}
			w.mu.Unlock()
		}
		return nil
	})
}

func (w *Watcher) eventLoop(ctx context.Context) {
	defer func() {
		close(w.batches)
		close(w.errors)
	}()

	for {
		select {
		case <-ctx.Done():
			_ = w.Close()
			return
		case <-w.done:
			return
		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			w.degraded.Store(true)
			select {
			case w.errors <- err:
			case <-w.done:
				return
			case <-ctx.Done():
				return
			}
		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			w.processFSEvent(event)
		}
	}
}

func (w *Watcher) processFSEvent(event fsnotify.Event) {
	if event.Name == "" {
		return
	}

	// Count every event outside ignored directories, not just accepted code
	// files: a created or removed directory can carry source files that never
	// produce an event of their own. Over-counting only costs a cheap re-check.
	if !w.inIgnoredDir(event.Name) {
		w.seq.Add(1)
	}

	stat, statErr := os.Stat(event.Name)
	isDir := statErr == nil && stat.IsDir()

	if isDir {
		if event.Has(fsnotify.Create) {
			if !shouldIgnoreDir(filepath.Base(event.Name), w.cfg.IgnoredDirs) {
				_ = w.watchTree(event.Name)
			}
		}
		return
	}

	relPath, err := filepath.Rel(w.cfg.RepoRoot, event.Name)
	if err != nil || strings.HasPrefix(relPath, "..") || relPath == "." {
		return
	}
	relPath = filepath.ToSlash(relPath)

	for _, seg := range strings.Split(relPath, "/") {
		if shouldIgnoreDir(seg, w.cfg.IgnoredDirs) {
			return
		}
	}

	if !isAcceptedCodeFile(event.Name, w.cfg.AcceptedExtensions) {
		return
	}

	op := determineOp(event, statErr == nil)
	if op == "" {
		return
	}

	w.debouncer.record(FileChange{
		Path:    event.Name,
		RelPath: relPath,
		Op:      op,
	})
}

func (w *Watcher) inIgnoredDir(path string) bool {
	relPath, err := filepath.Rel(w.cfg.RepoRoot, path)
	if err != nil {
		return false
	}
	for _, seg := range strings.Split(filepath.ToSlash(relPath), "/") {
		if shouldIgnoreDir(seg, w.cfg.IgnoredDirs) {
			return true
		}
	}
	return false
}

func determineOp(event fsnotify.Event, exists bool) ChangeOp {
	if event.Has(fsnotify.Remove) {
		return OpDelete
	}
	if event.Has(fsnotify.Rename) {
		if exists {
			return OpModify
		}
		return OpDelete
	}
	if event.Has(fsnotify.Create) {
		return OpCreate
	}
	if event.Has(fsnotify.Write) || event.Has(fsnotify.Chmod) {
		return OpModify
	}
	return ""
}
