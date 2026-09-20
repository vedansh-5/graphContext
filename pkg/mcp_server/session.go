package mcp_server

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/vedansh-5/graphcontext/pkg/analysis"
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
}

var (
	sessions   = make(map[string]*session)
	sessionsMu sync.Mutex
)

func getSession(projectPath string) (*session, error) {
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
		sessions[absPath] = sess
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

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

	return sess, nil
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

		sess, err := getSession(projectPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("session error: %v", err)), nil
		}

		return handler(sess, projectPath, args)
	}
}
