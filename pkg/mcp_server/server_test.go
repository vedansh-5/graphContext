package mcp_server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	servicePy := `class AuthService:
    def login(self, username, password):
        self.validate(username)
        return True

    def validate(self, username):
        return len(username) > 0
`
	routesPy := `from auth.service import AuthService

def login_route():
    s = AuthService()
    return s.login("admin", "secret")
`
	testPy := `from auth.service import AuthService

def test_login():
    s = AuthService()
    assert s.login("test", "pass")
`
	unusedPy := `def orphan_function():
    return 42
`

	authDir := filepath.Join(dir, "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(authDir, "service.py"), []byte(servicePy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "routes.py"), []byte(routesPy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "test_service.py"), []byte(testPy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unused.py"), []byte(unusedPy), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func extractResultText(res *mcp.CallToolResult) string {
	if len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(mcp.TextContent); ok {
		return tc.Text
	}
	b, _ := json.Marshal(res.Content[0])
	return string(b)
}

func parseResultEnvelope(t *testing.T, resText string) Envelope {
	t.Helper()
	var env Envelope
	if err := json.Unmarshal([]byte(resText), &env); err != nil {
		t.Fatalf("failed to unmarshal result envelope: %v\nRaw: %s", err, resText)
	}
	return env
}

func TestMCPToolsEndToEnd(t *testing.T) {
	projDir := setupTestProject(t)
	sess, err := getSession(projDir)
	if err != nil {
		t.Fatalf("getSession failed: %v", err)
	}

	t.Run("search_symbols", func(t *testing.T) {
		res, err := handleSearchSymbols(sess, projDir, map[string]any{"query": "login"})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("search_symbols returned error: %v", res)
		}

		text := extractResultText(res)
		env := parseResultEnvelope(t, text)
		ans := env.Answer.(map[string]any)
		matches := ans["matches"].([]any)
		if len(matches) == 0 {
			t.Errorf("expected matches for 'login', got 0")
		}
	})

	t.Run("get_context", func(t *testing.T) {
		res, err := handleGetContext(sess, projDir, map[string]any{
			"symbol":         "login",
			"include_source": true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("get_context error: %v", res)
		}

		text := extractResultText(res)
		env := parseResultEnvelope(t, text)
		ans := env.Answer.(map[string]any)
		if ans["node"] == nil {
			t.Errorf("expected node in get_context answer")
		}
		if ans["source"] == "" {
			t.Errorf("expected source to be included")
		}
	})

	t.Run("get_task_context", func(t *testing.T) {
		res, err := handleGetTaskContext(sess, projDir, map[string]any{
			"task": "fix authentication login username validation",
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("get_task_context error: %v", res)
		}

		text := extractResultText(res)
		env := parseResultEnvelope(t, text)
		ans := env.Answer.(map[string]any)
		ctxNodes := ans["context_nodes"].([]any)
		if len(ctxNodes) == 0 {
			t.Errorf("expected context nodes for task, got 0")
		}
	})

	t.Run("impact_of_change", func(t *testing.T) {
		res, err := handleImpactOfChange(sess, projDir, map[string]any{
			"symbol":      "validate",
			"change_type": "delete",
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("impact_of_change error: %v", res)
		}

		text := extractResultText(res)
		env := parseResultEnvelope(t, text)
		ans := env.Answer.(map[string]any)
		if ans["target"] == nil {
			t.Errorf("expected target in impact answer")
		}
		if ans["dangling_references"] == nil {
			t.Errorf("expected dangling_references on delete change_type")
		}
	})

	t.Run("trace", func(t *testing.T) {
		res, err := handleTrace(sess, projDir, map[string]any{
			"from": "login_route",
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("trace error: %v", res)
		}

		text := extractResultText(res)
		env := parseResultEnvelope(t, text)
		ans := env.Answer.(map[string]any)
		if ans["call_tree"] == nil {
			t.Errorf("expected call_tree in trace answer")
		}

		pathRes, err := handleTrace(sess, projDir, map[string]any{
			"from": "login_route",
			"to":   "validate",
		})
		if err != nil {
			t.Fatal(err)
		}
		if pathRes.IsError {
			t.Fatalf("trace path error: %v", pathRes)
		}
	})

	t.Run("repo_overview", func(t *testing.T) {
		res, err := handleRepoOverview(sess, projDir, map[string]any{
			"analysis": "all",
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.IsError {
			t.Fatalf("repo_overview error: %v", res)
		}

		text := extractResultText(res)
		env := parseResultEnvelope(t, text)
		ans := env.Answer.(map[string]any)
		if ans["architecture"] == nil {
			t.Errorf("expected architecture in repo_overview")
		}
		if ans["dead_code"] == nil {
			t.Errorf("expected dead_code in repo_overview")
		}
	})
}
