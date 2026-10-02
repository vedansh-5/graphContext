package mcp_server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vedansh-5/graphcontext/pkg/store"
)

func TestViewNodeIsCompact(t *testing.T) {
	v := viewNode(store.Node{
		ID: "a.go:Run", Kind: store.KindFunction, Name: "Run", FilePath: "a.go",
		StartLine: 3, EndLine: 3, StartByte: 40, EndByte: 90,
		Docstring: "Run starts the server.\nIt blocks until done.",
	})
	if v.EndLine != 0 {
		t.Errorf("single-line node should omit end_line, got %d", v.EndLine)
	}
	if v.Doc != "Run starts the server." {
		t.Errorf("doc = %q, want the first line only", v.Doc)
	}
	if got := viewConfidence(store.ConfExact); got != "" {
		t.Errorf("exact confidence should be omitted, got %q", got)
	}
	if got := viewConfidence(store.ConfAmbiguous); got != "ambiguous" {
		t.Errorf("ambiguous confidence = %q", got)
	}
}

func TestSearchRanksExactNameFirst(t *testing.T) {
	dir := t.TempDir()
	src := `package app

func LoadConfigFromDisk() {}

func ReloadConfig() { Load() }

func Load() {}
`
	test := `package app

import "testing"

func TestLoad(t *testing.T) { Load() }
`
	for name, body := range map[string]string{"app.go": src, "app_test.go": test} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(closeAllSessions)
	sess, err := getSession(dir)
	if err != nil {
		t.Fatalf("getSession: %v", err)
	}

	res, err := handleSearchSymbols(sess, dir, map[string]any{"query": "load"})
	if err != nil || res.IsError {
		t.Fatalf("search failed: %v %v", err, res)
	}
	text := extractResultText(res)
	if strings.Contains(text, "\n") {
		t.Error("tool answers should be compact JSON, found a newline")
	}
	if strings.Contains(text, "StartByte") || strings.Contains(text, "start_byte") {
		t.Error("tool answers should not carry byte offsets")
	}

	env := parseResultEnvelope(t, text)
	matches := env.Answer.(map[string]any)["matches"].([]any)
	var ids []string
	for _, m := range matches {
		ids = append(ids, m.(map[string]any)["id"].(string))
	}
	if len(ids) != 4 {
		t.Fatalf("want 4 matches, got %v", ids)
	}
	if ids[0] != "app.go:Load" {
		t.Errorf("exact name should rank first, got %v", ids)
	}
	if ids[1] != "app.go:LoadConfigFromDisk" {
		t.Errorf("name prefix should rank second, got %v", ids)
	}
	if ids[3] != "app_test.go:TestLoad" && ids[2] != "app_test.go:TestLoad" {
		t.Errorf("test should not outrank code, got %v", ids)
	}
}
