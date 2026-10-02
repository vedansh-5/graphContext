package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/vedansh-5/graphcontext/pkg/store"
)

// writeSyntheticRepo creates n Go files, each with a few functions calling
// into the previous file, so the resolver has real cross-file work to do.
func writeSyntheticRepo(tb testing.TB, dir string, n int) {
	tb.Helper()
	for i := 0; i < n; i++ {
		src := fmt.Sprintf("package p\n\nfunc F%d() { F%d() }\n\nfunc G%d() { F%d() }\n", i, (i+n-1)%n, i, i)
		pkgDir := filepath.Join(dir, fmt.Sprintf("pkg%d", i%20))
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, fmt.Sprintf("f%d.go", i)), []byte(src), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
}

func benchStore(tb testing.TB) *store.Store {
	tb.Helper()
	s, err := store.Open(filepath.Join(tb.TempDir(), "bench.db"))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { s.Close() })
	return s
}

// BenchmarkEnsureFreshNoChange measures the cost of a freshness check when
// nothing on disk has changed, which is what every tool call pays.
func BenchmarkEnsureFreshNoChange(b *testing.B) {
	dir := b.TempDir()
	writeSyntheticRepo(b, dir, 2000)
	s := benchStore(b)
	if _, err := EnsureFresh(dir, s); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		changed, err := EnsureFresh(dir, s)
		if err != nil {
			b.Fatal(err)
		}
		if changed {
			b.Fatal("expected no change")
		}
	}
}

// BenchmarkEnsureFreshOneFileChanged measures re-indexing after a single edit.
func BenchmarkEnsureFreshOneFileChanged(b *testing.B) {
	dir := b.TempDir()
	writeSyntheticRepo(b, dir, 2000)
	s := benchStore(b)
	if _, err := EnsureFresh(dir, s); err != nil {
		b.Fatal(err)
	}
	target := filepath.Join(dir, "pkg0", "f0.go")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src := fmt.Sprintf("package p\n\nfunc F0() { F1999() }\n\nfunc G0() { F0() }\n\nfunc H%d() {}\n", i)
		if err := os.WriteFile(target, []byte(src), 0o644); err != nil {
			b.Fatal(err)
		}
		if _, err := EnsureFresh(dir, s); err != nil {
			b.Fatal(err)
		}
	}
}
