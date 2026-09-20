package watcher

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
}

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
