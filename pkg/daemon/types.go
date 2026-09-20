package daemon

import (
	"time"

	"github.com/vedansh-5/graphcontext/pkg/store"
	"github.com/vedansh-5/graphcontext/pkg/watcher"
)

type Config struct {
	RepoRoot         string
	Store            *store.Store
	DebounceDuration time.Duration
	AutoWatch        bool
}

type SyncStatus struct {
	LastSync   time.Time `json:"last_sync"`
	SyncCount  int64     `json:"sync_count"`
	NodeCount  int       `json:"node_count"`
	EdgeCount  int       `json:"edge_count"`
	LastError  string    `json:"last_error,omitempty"`
	IsWatching bool      `json:"is_watching"`
}

type SyncEvent struct {
	Timestamp time.Time            `json:"timestamp"`
	Duration  time.Duration        `json:"duration"`
	Changed   bool                 `json:"changed"`
	Batch     *watcher.EventBatch  `json:"batch,omitempty"`
	NodeCount int                  `json:"node_count"`
	EdgeCount int                  `json:"edge_count"`
	Error     error                `json:"error,omitempty"`
}
