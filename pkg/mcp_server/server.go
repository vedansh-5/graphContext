package mcp_server

import (
	"fmt"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

func newServer() *server.MCPServer {
	s := server.NewMCPServer(
		"graphContext",
		"2.0.0",
		server.WithToolCapabilities(true),
	)

	registerOrientationTools(s)
	registerTaskContextTool(s)
	registerReasoningTools(s)
	registerRepoOverviewTool(s)
	registerDiffImpactTool(s)
	return s
}

func StartStdioServer() error {
	s := newServer()

	stop := make(chan struct{})
	go reapIdleSessions(idleTTL(), stop)

	fmt.Fprintf(os.Stderr, "Starting graphContext MCP server on stdio...\n")
	// ServeStdio returns on SIGINT, SIGTERM or when stdin closes. Stop the
	// watchers and close the stores before the process exits.
	err := server.ServeStdio(s)
	close(stop)
	closeAllSessions()
	return err
}

// defaultIdleTTL is how long a project may go unused before its watcher,
// in-memory graph and database handle are released.
const defaultIdleTTL = 30 * time.Minute

// idleTTL reads GRAPHCONTEXT_IDLE_TTL (a Go duration such as "10m"), falling
// back to defaultIdleTTL when it is unset or invalid.
func idleTTL() time.Duration {
	if v := os.Getenv("GRAPHCONTEXT_IDLE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
		fmt.Fprintf(os.Stderr, "ignoring invalid GRAPHCONTEXT_IDLE_TTL %q\n", v)
	}
	return defaultIdleTTL
}
