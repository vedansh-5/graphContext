package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := "package app\n\nfunc Login() { check() }\n\nfunc check() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func run(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = Main(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRunTool(t *testing.T) {
	dir := testProject(t)

	code, out, errOut := run("run", "search_symbols", "-C", dir, "query=login", "limit=5")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	var env struct {
		Answer struct {
			Matches []map[string]any `json:"matches"`
		} `json:"answer"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if len(env.Answer.Matches) == 0 {
		t.Errorf("expected a match for login, got %s", out)
	}
}

func TestRunToolErrors(t *testing.T) {
	dir := testProject(t)
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"unknown tool", []string{"run", "nope", "-C", dir}, 2, "unknown tool"},
		{"unknown argument", []string{"run", "search_symbols", "-C", dir, "nope=1"}, 2, "no argument"},
		{"bad number", []string{"run", "search_symbols", "-C", dir, "query=x", "limit=abc"}, 2, "must be a number"},
		{"not key=value", []string{"run", "search_symbols", "query"}, 2, "expected key=value"},
		{"tool error", []string{"run", "search_symbols", "-C", dir}, 1, "query is required"},
		{"no tool", []string{"run"}, 2, "usage"},
		{"unknown command", []string{"frobnicate"}, 2, "unknown command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run(tt.args...)
			if code != tt.code {
				t.Errorf("exit %d, want %d", code, tt.code)
			}
			if out != "" && tt.code != 0 && !strings.Contains(out, "Usage") {
				t.Errorf("errors belong on stderr, got stdout: %s", out)
			}
			if !strings.Contains(errOut, tt.want) {
				t.Errorf("stderr %q does not mention %q", errOut, tt.want)
			}
		})
	}
}

func TestIndexAndTools(t *testing.T) {
	dir := testProject(t)

	code, out, errOut := run("index", dir)
	if code != 0 {
		t.Fatalf("index: exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "1 files") {
		t.Errorf("index summary missing file count: %s", out)
	}
	if _, out, _ = run("index", dir); !strings.Contains(out, "already up to date") {
		t.Errorf("second index should be a no-op: %s", out)
	}

	code, out, _ = run("tools")
	if code != 0 || !strings.Contains(out, "search_symbols") || !strings.Contains(out, "diff_impact") {
		t.Errorf("tools: exit %d, output: %s", code, out)
	}
	if strings.Contains(out, "project_path") {
		t.Errorf("tools output should hide project_path, it is set by -C: %s", out)
	}

	if code, _, errOut = run("index", filepath.Join(dir, "missing")); code != 1 || errOut == "" {
		t.Errorf("index of a missing dir: exit %d, stderr %q", code, errOut)
	}
}
