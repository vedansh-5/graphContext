package mcp_server

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func argString(args map[string]any, key string) string {
	if val, ok := args[key]; ok {
		if s, ok := val.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func argInt(args map[string]any, key string, fallback int) int {
	if val, ok := args[key]; ok {
		switch v := val.(type) {
		case float64:
			return int(v)
		case int:
			return v
		case int64:
			return int(v)
		}
	}
	return fallback
}

func argBool(args map[string]any, key string) bool {
	if val, ok := args[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

func resolveSymbol(sess *session, symbol string) (store.Node, *mcp.CallToolResult) {
	if n, ok := sess.graph.Nodes[symbol]; ok {
		return n, nil
	}

	var exactMatches []store.Node
	for _, n := range sess.graph.Nodes {
		if n.Name == symbol || n.QualifiedName == symbol {
			exactMatches = append(exactMatches, n)
		}
	}

	if len(exactMatches) == 1 {
		return exactMatches[0], nil
	}

	if len(exactMatches) > 1 {
		sort.Slice(exactMatches, func(i, j int) bool { return exactMatches[i].ID < exactMatches[j].ID })
		candidates := make([]string, 0, len(exactMatches))
		for _, m := range exactMatches {
			candidates = append(candidates, fmt.Sprintf("%s (%s in %s:%d)", m.ID, m.Kind, m.FilePath, m.StartLine))
		}
		return store.Node{}, mcp.NewToolResultError(fmt.Sprintf(
			"ambiguous symbol %q matched multiple definitions. Please specify exact node id:\n- %s",
			symbol, strings.Join(candidates, "\n- "),
		))
	}

	return store.Node{}, mcp.NewToolResultError(fmt.Sprintf(
		"symbol %q not found. Use search_symbols to discover available identifiers.",
		symbol,
	))
}

func readSourceSpan(projectPath string, node store.Node) string {
	targetPath := node.FilePath
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(projectPath, targetPath)
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		return ""
	}

	start := node.StartByte
	end := node.EndByte
	if start < 0 || end > len(content) || start >= end {
		return ""
	}

	return string(content[start:end])
}

func groupNodeByScope(scope string, n store.Node) string {
	switch scope {
	case "function", "method":
		return n.ID
	case "file":
		return n.FilePath
	default:
		dir := filepath.Dir(n.FilePath)
		if dir == "." || dir == "" {
			return "root"
		}
		return filepath.ToSlash(dir)
	}
}
