package watcher

import (
	"time"
)

type ChangeOp string

const (
	OpCreate ChangeOp = "create"
	OpModify ChangeOp = "modify"
	OpDelete ChangeOp = "delete"
)

type FileChange struct {
	Path    string   `json:"path"`
	RelPath string   `json:"rel_path"`
	Op      ChangeOp `json:"op"`
}

type EventBatch struct {
	Changes   []FileChange `json:"changes"`
	CreatedAt time.Time    `json:"created_at"`
}

type Config struct {
	RepoRoot           string
	DebounceDuration   time.Duration
	IgnoredDirs        map[string]bool
	AcceptedExtensions map[string]bool
}
