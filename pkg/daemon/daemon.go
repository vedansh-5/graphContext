package daemon

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/vedansh-5/graphcontext/pkg/analysis"
	"github.com/vedansh-5/graphcontext/pkg/indexer"
	_ "github.com/vedansh-5/graphcontext/pkg/lang/golang"
	_ "github.com/vedansh-5/graphcontext/pkg/lang/python"
	_ "github.com/vedansh-5/graphcontext/pkg/lang/typescript"
	"github.com/vedansh-5/graphcontext/pkg/store"
	"github.com/vedansh-5/graphcontext/pkg/watcher"
)

type Daemon struct {
	repoRoot    string
	store       *store.Store
	ownsStore   bool
	watcher     *watcher.Watcher
	graph       *analysis.Graph
	mu          sync.RWMutex
	syncMu      sync.Mutex
	status      SyncStatus
	listeners   map[chan SyncEvent]struct{}
	listenersMu sync.Mutex
	done        chan struct{}
	closed      sync.Once
}

func New(cfg Config) (*Daemon, error) {
	if cfg.RepoRoot == "" {
		return nil, fmt.Errorf("daemon: repo root cannot be empty")
	}

	absRoot, err := filepath.Abs(cfg.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("daemon: resolve repo root: %w", err)
	}
	if realRoot, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = realRoot
	}

	st := cfg.Store
	ownsStore := false
	if st == nil {
		dbPath, err := store.CachePathFor(absRoot)
		if err != nil {
			return nil, fmt.Errorf("daemon: cache path: %w", err)
		}
		openedStore, err := store.Open(dbPath)
		if err != nil {
			return nil, fmt.Errorf("daemon: open store: %w", err)
		}
		st = openedStore
		ownsStore = true
	}

	debounceDur := cfg.DebounceDuration
	if debounceDur <= 0 {
		debounceDur = 200 * time.Millisecond
	}

	w, err := watcher.New(watcher.Config{
		RepoRoot:         absRoot,
		DebounceDuration: debounceDur,
	})
	if err != nil {
		if ownsStore {
			_ = st.Close()
		}
		return nil, fmt.Errorf("daemon: create watcher: %w", err)
	}

	d := &Daemon{
		repoRoot:  absRoot,
		store:     st,
		ownsStore: ownsStore,
		watcher:   w,
		listeners: make(map[chan SyncEvent]struct{}),
		done:      make(chan struct{}),
	}

	return d, nil
}

func (d *Daemon) Start(ctx context.Context) error {
	if _, err := d.SyncNow(); err != nil {
		return fmt.Errorf("daemon: initial sync: %w", err)
	}

	batchCh, errCh := d.watcher.Start(ctx)
	d.mu.Lock()
	d.status.IsWatching = true
	d.mu.Unlock()

	go d.eventLoop(ctx, batchCh, errCh)
	return nil
}

func (d *Daemon) Stop() error {
	var stopErr error
	d.closed.Do(func() {
		close(d.done)
		if d.watcher != nil {
			_ = d.watcher.Close()
		}
		d.mu.Lock()
		d.status.IsWatching = false
		d.mu.Unlock()

		if d.ownsStore && d.store != nil {
			stopErr = d.store.Close()
		}

		d.listenersMu.Lock()
		for ch := range d.listeners {
			close(ch)
		}
		d.listeners = make(map[chan SyncEvent]struct{})
		d.listenersMu.Unlock()
	})
	return stopErr
}

func (d *Daemon) Graph() *analysis.Graph {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.graph
}

func (d *Daemon) WithGraph(fn func(g *analysis.Graph) error) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.graph == nil {
		return fmt.Errorf("daemon: graph not initialized")
	}
	return fn(d.graph)
}

func (d *Daemon) Status() SyncStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

func (d *Daemon) Store() *store.Store {
	return d.store
}

func (d *Daemon) RepoRoot() string {
	return d.repoRoot
}

func (d *Daemon) SyncNow() (bool, error) {
	return d.syncInternal(nil)
}

func (d *Daemon) SubscribeSync() <-chan SyncEvent {
	d.listenersMu.Lock()
	defer d.listenersMu.Unlock()
	ch := make(chan SyncEvent, 32)
	d.listeners[ch] = struct{}{}
	return ch
}

func (d *Daemon) UnsubscribeSync(ch <-chan SyncEvent) {
	d.listenersMu.Lock()
	defer d.listenersMu.Unlock()
	for listener := range d.listeners {
		if listener == ch {
			delete(d.listeners, listener)
			close(listener)
			break
		}
	}
}

func (d *Daemon) eventLoop(ctx context.Context, batchCh <-chan watcher.EventBatch, errCh <-chan error) {
	for {
		select {
		case <-ctx.Done():
			_ = d.Stop()
			return
		case <-d.done:
			return
		case err, ok := <-errCh:
			if !ok {
				return
			}
			d.mu.Lock()
			d.status.LastError = err.Error()
			d.mu.Unlock()
		case batch, ok := <-batchCh:
			if !ok {
				return
			}
			_, _ = d.syncInternal(&batch)
		}
	}
}

func (d *Daemon) syncInternal(batch *watcher.EventBatch) (bool, error) {
	d.syncMu.Lock()
	defer d.syncMu.Unlock()

	start := time.Now()
	changed, err := indexer.EnsureFresh(d.repoRoot, d.store)
	if err != nil {
		d.mu.Lock()
		d.status.LastError = err.Error()
		d.mu.Unlock()

		d.notifySync(SyncEvent{
			Timestamp: time.Now().UTC(),
			Duration:  time.Since(start),
			Changed:   false,
			Batch:     batch,
			Error:     err,
		})
		return false, err
	}

	d.mu.RLock()
	needLoad := d.graph == nil || changed
	d.mu.RUnlock()

	var newGraph *analysis.Graph
	if needLoad {
		g, loadErr := analysis.Load(d.store)
		if loadErr != nil {
			d.mu.Lock()
			d.status.LastError = loadErr.Error()
			d.mu.Unlock()

			d.notifySync(SyncEvent{
				Timestamp: time.Now().UTC(),
				Duration:  time.Since(start),
				Changed:   false,
				Batch:     batch,
				Error:     loadErr,
			})
			return false, loadErr
		}
		newGraph = g
	}

	d.mu.Lock()
	if newGraph != nil {
		d.graph = newGraph
	}
	d.status.LastSync = time.Now().UTC()
	d.status.SyncCount++
	d.status.NodeCount = d.graph.NodeCount()
	d.status.EdgeCount = d.graph.EdgeCount()
	d.status.LastError = ""
	nodeCount := d.status.NodeCount
	edgeCount := d.status.EdgeCount
	d.mu.Unlock()

	d.notifySync(SyncEvent{
		Timestamp: time.Now().UTC(),
		Duration:  time.Since(start),
		Changed:   changed,
		Batch:     batch,
		NodeCount: nodeCount,
		EdgeCount: edgeCount,
	})

	return changed, nil
}

func (d *Daemon) notifySync(evt SyncEvent) {
	d.listenersMu.Lock()
	defer d.listenersMu.Unlock()
	for ch := range d.listeners {
		select {
		case ch <- evt:
		default:
		}
	}
}
