package mcp_server

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

// This file lets the tools be called in-process, without an MCP client. The
// command line uses it so a shell or CI job gets the same answers an agent does.

// ToolInfo describes one tool for help output.
type ToolInfo struct {
	Name        string
	Description string
	Args        []ArgInfo
}

// ArgInfo describes one tool argument.
type ArgInfo struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

// Tools lists every registered tool, sorted by name.
func Tools() []ToolInfo {
	var out []ToolInfo
	for name, st := range newServer().ListTools() {
		info := ToolInfo{Name: name, Description: st.Tool.Description}
		required := map[string]bool{}
		for _, r := range st.Tool.InputSchema.Required {
			required[r] = true
		}
		for argName, raw := range st.Tool.InputSchema.Properties {
			arg := ArgInfo{Name: argName, Required: required[argName]}
			if prop, ok := raw.(map[string]any); ok {
				arg.Type, _ = prop["type"].(string)
				arg.Description, _ = prop["description"].(string)
			}
			info.Args = append(info.Args, arg)
		}
		sort.Slice(info.Args, func(i, j int) bool { return info.Args[i].Name < info.Args[j].Name })
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// CallTool runs one tool with string arguments, converting each to the type
// the tool declares. It returns the tool's text output; toolErr is true when
// the tool itself reported an error, in which case text is the message.
func CallTool(ctx context.Context, name string, args map[string]string) (text string, toolErr bool, err error) {
	st := newServer().GetTool(name)
	if st == nil {
		return "", false, fmt.Errorf("unknown tool %q", name)
	}

	typed := make(map[string]any, len(args))
	for key, value := range args {
		raw, known := st.Tool.InputSchema.Properties[key]
		if !known {
			return "", false, fmt.Errorf("tool %s has no argument %q", name, key)
		}
		prop, _ := raw.(map[string]any)
		argType, _ := prop["type"].(string)
		switch argType {
		case "number", "integer":
			n, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return "", false, fmt.Errorf("argument %s must be a number, got %q", key, value)
			}
			typed[key] = n
		case "boolean":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return "", false, fmt.Errorf("argument %s must be true or false, got %q", key, value)
			}
			typed[key] = b
		default:
			typed[key] = value
		}
	}

	var req mcp.CallToolRequest
	req.Params.Name = name
	req.Params.Arguments = typed
	res, err := st.Handler(ctx, req)
	if err != nil {
		return "", false, err
	}
	return resultText(res), res.IsError, nil
}

func resultText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// SetLive controls whether new sessions start a file watcher. A long-running
// server wants one; a one-shot command does not, since it exits before any
// file could change.
func SetLive(enabled bool) {
	sessionsMu.Lock()
	liveEnabled = enabled
	sessionsMu.Unlock()
}

// Shutdown closes every open project session.
func Shutdown() { closeAllSessions() }
