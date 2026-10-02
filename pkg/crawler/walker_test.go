package crawler

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/vedansh-5/graphcontext/pkg/lang"
	_ "github.com/vedansh-5/graphcontext/pkg/lang/golang"
	_ "github.com/vedansh-5/graphcontext/pkg/lang/python"
	_ "github.com/vedansh-5/graphcontext/pkg/lang/typescript"
)

func TestWalkEmitsEveryRegisteredExtension(t *testing.T) {
	dir := t.TempDir()
	var want []string
	for _, ext := range lang.Extensions() {
		name := "file" + ext
		want = append(want, name)
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Not source, and source inside ignored directories.
	skipped := []string{"README.md", "venv/lib.py", "node_modules/pkg/index.js", "__pycache__/x.py"}
	for _, name := range skipped {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ch := make(chan string, 100)
	Walk(dir, ch)
	var got []string
	for p := range ch {
		rel, _ := filepath.Rel(dir, p)
		got = append(got, filepath.ToSlash(rel))
	}

	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
