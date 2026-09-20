package watcher

import (
	"sort"
	"sync"
	"time"
)

type debouncer struct {
	mu       sync.Mutex
	duration time.Duration
	pending  map[string]FileChange
	timer    *time.Timer
	flushCh  chan EventBatch
	stopped  bool
}

func newDebouncer(duration time.Duration, flushCh chan EventBatch) *debouncer {
	if duration <= 0 {
		duration = 200 * time.Millisecond
	}
	return &debouncer{
		duration: duration,
		pending:  make(map[string]FileChange),
		flushCh:  flushCh,
	}
}

func (d *debouncer) record(change FileChange) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return
	}

	mergedOp, keep := coalesceOperations(d.pending[change.RelPath].Op, change.Op)
	if !keep {
		delete(d.pending, change.RelPath)
	} else {
		change.Op = mergedOp
		d.pending[change.RelPath] = change
	}

	if len(d.pending) == 0 {
		if d.timer != nil {
			d.timer.Stop()
			d.timer = nil
		}
		return
	}

	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.duration, d.flush)
}

func (d *debouncer) flush() {
	d.mu.Lock()
	if d.stopped || len(d.pending) == 0 {
		d.mu.Unlock()
		return
	}

	changes := make([]FileChange, 0, len(d.pending))
	for _, c := range d.pending {
		changes = append(changes, c)
	}

	sort.Slice(changes, func(i, j int) bool {
		return changes[i].RelPath < changes[j].RelPath
	})

	d.pending = make(map[string]FileChange)
	d.timer = nil
	d.mu.Unlock()

	d.flushCh <- EventBatch{
		Changes:   changes,
		CreatedAt: time.Now().UTC(),
	}
}

func (d *debouncer) stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopped = true
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
}

func coalesceOperations(existing, incoming ChangeOp) (ChangeOp, bool) {
	if existing == "" {
		return incoming, true
	}
	switch existing {
	case OpCreate:
		switch incoming {
		case OpDelete:
			return "", false
		default:
			return OpCreate, true
		}
	case OpModify:
		switch incoming {
		case OpDelete:
			return OpDelete, true
		default:
			return OpModify, true
		}
	case OpDelete:
		switch incoming {
		case OpCreate:
			return OpModify, true
		default:
			return OpDelete, true
		}
	}
	return incoming, true
}
