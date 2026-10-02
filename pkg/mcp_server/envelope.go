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
	// Compact on purpose: indentation is whitespace the model pays for.
	b, err := json.Marshal(e)
	if err != nil {
		return mcp.NewToolResultError(err.Error())
	}
	return mcp.NewToolResultText(string(b))
}
