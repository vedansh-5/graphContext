package mcp_server

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
)

type Envelope struct {
	Answer    any            `json:"answer"`
	Caveats   []string       `json:"caveats,omitempty"`
	Stats     map[string]any `json:"stats,omitempty"`
	GraphMeta map[string]any `json:"graph_meta,omitempty"`
}

func toolJSON(e Envelope) *mcp.CallToolResult {
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(err.Error())
	}
	return mcp.NewToolResultText(string(b))
}
