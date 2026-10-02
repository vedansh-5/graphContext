package mcp_server

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/vedansh-5/graphcontext/pkg/analysis"
	"github.com/vedansh-5/graphcontext/pkg/daemon"
	"github.com/vedansh-5/graphcontext/pkg/indexer"
	_ "github.com/vedansh-5/graphcontext/pkg/lang/golang"
	_ "github.com/vedansh-5/graphcontext/pkg/lang/python"
	_ "github.com/vedansh-5/graphcontext/pkg/lang/typescript"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

type session struct {
	store    *store.Store
	graph    *analysis.Graph
	repoRoot string
	mu       sync.RWMutex
	// live keeps the graph current from file-change events. It is nil when
	// the watcher could not start, and the session then re-checks the disk on
	// every call.
	live *daemon.Daemon
	// lastUsed and inUse are guarded by sessionsMu. A session is only evicted
	// when no tool call is running against it.
	lastUsed time.Time
	inUse    int
}

var (
	sessions   = make(map[string]*session)
	sessionsMu sync.Mutex
	// liveEnabled is guarded by sessionsMu. See SetLive.
	liveEnabled = true
)

// getSession returns the up-to-date session for a project, opening it on first
// use.
func getSession(projectPath string) (*session, error) {
	return openSession(projectPath, false)
}

// acquireSession is getSession for the duration of one tool call: the session
// cannot be evicted until release is called.
func acquireSession(projectPath string) (sess *session, release func(), err error) {
	sess, err = openSession(projectPath, true)
	if err != nil {
		return nil, nil, err
	}
	return sess, func() {
		sessionsMu.Lock()
		sess.inUse--
		sess.lastUsed = time.Now()
		sessionsMu.Unlock()
	}, nil
}

func openSession(projectPath string, hold bool) (*session, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, fmt.Errorf("resolve project path: %w", err)
	}

	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	sess, exists := sessions[absPath]
	if !exists {
		dbPath, err := store.CachePathFor(absPath)
		if err != nil {
			return nil, fmt.Errorf("locate cache path: %w", err)
		}

		st, err := store.Open(dbPath)
		if err != nil {
			return nil, fmt.Errorf("open graph store: %w", err)
		}

		sess = &session{
			store:    st,
			repoRoot: absPath,
		}
		if liveEnabled {
			sess.live = startLive(absPath, st)
		}
		sessions[absPath] = sess
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if sess.live != nil {
		g, err := sess.live.FreshGraph()
		if err != nil {
			return nil, fmt.Errorf("refresh live graph: %w", err)
		}
		sess.graph = g
		sess.touch(hold)
		return sess, nil
	}

	changed, err := indexer.EnsureFresh(absPath, sess.store)
	if err != nil {
		return nil, fmt.Errorf("ensure fresh index: %w", err)
	}

	if sess.graph == nil || changed {
		g, err := analysis.Load(sess.store)
		if err != nil {
			return nil, fmt.Errorf("load analysis graph: %w", err)
		}
		sess.graph = g
	}

	sess.touch(hold)
	return sess, nil
}

// touch records a use of the session. The caller holds sessionsMu.
func (sess *session) touch(hold bool) {
	sess.lastUsed = time.Now()
	if hold {
		sess.inUse++
	}
}

// close stops the session's daemon and closes its store.
func (sess *session) close() {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.live != nil {
		_ = sess.live.Stop()
	}
	_ = sess.store.Close()
}

// evictIdleSessions closes every session that has not been used for longer
// than ttl and has no tool call in flight. It returns how many were closed.
// An evicted project is simply reopened on its next tool call.
func evictIdleSessions(now time.Time, ttl time.Duration) int {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	evicted := 0
	for path, sess := range sessions {
		if sess.inUse > 0 || now.Sub(sess.lastUsed) <= ttl {
			continue
		}
		sess.close()
		delete(sessions, path)
		evicted++
	}
	return evicted
}

// closeAllSessions closes every session. Used on shutdown.
func closeAllSessions() {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	for path, sess := range sessions {
		sess.close()
		delete(sessions, path)
	}
}

// reapIdleSessions evicts idle sessions on a timer until stop is closed.
func reapIdleSessions(ttl time.Duration, stop <-chan struct{}) {
	interval := ttl / 4
	if interval > time.Minute {
		interval = time.Minute
	}
	if interval <= 0 {
		interval = ttl
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-ticker.C:
			if n := evictIdleSessions(now, ttl); n > 0 {
				log.Printf("closed %d idle project session(s)", n)
			}
		}
	}
}

// startLive starts a watching daemon over the session's store. It returns nil
// if that fails, which is not fatal: the session falls back to re-checking the
// disk on every call.
func startLive(repoRoot string, st *store.Store) *daemon.Daemon {
	d, err := daemon.New(daemon.Config{RepoRoot: repoRoot, Store: st})
	if err != nil {
		log.Printf("live graph disabled for %s: %v", repoRoot, err)
		return nil
	}
	if err := d.Start(context.Background()); err != nil {
		log.Printf("live graph disabled for %s: %v", repoRoot, err)
		_ = d.Stop()
		return nil
	}
	return d
}

type toolHandler func(sess *session, projectPath string, args map[string]any) (*mcp.CallToolResult, error)

func withSession(handler toolHandler) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]any)
		if !ok {
			return mcp.NewToolResultError("invalid arguments format"), nil
		}

		projectPath := argString(args, "project_path")
		if projectPath == "" {
			return mcp.NewToolResultError("project_path is required"), nil
		}

		sess, release, err := acquireSession(projectPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("session error: %v", err)), nil
		}
		defer release()

		return handler(sess, projectPath, args)
	}
}
