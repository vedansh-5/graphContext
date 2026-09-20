package mcp_server

import (
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/server"
)

func StartStdioServer() error {
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

	fmt.Fprintf(os.Stderr, "Starting graphContext MCP server on stdio...\n")
	return server.ServeStdio(s)
}
