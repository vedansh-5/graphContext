package lang

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Language is one language plugin. Implementations must be safe for concurrent
// use: pass 1 runs files in parallel.
type Language interface {
	// Name is the value written to Node.Language ("go", "python", "typescript").
	Name() string
	// Extensions are the lowercase file extensions this plugin claims, with dots.
	Extensions() []string
	// Parse extracts the intermediate form from one file's bytes. It must not
	// touch the filesystem or depend on any other file.
	Parse(path string, src []byte) (*FileIR, error)
	// ModulePath converts a file path into the module identifier other files
	// would import it by. Pure path arithmetic; no filesystem access.
	ModulePath(repoRoot, filePath string) string
}

var (
	mu       sync.RWMutex
	registry = map[string]Language{} // extension -> plugin
)

// Register adds a plugin for each of its extensions. Registering the same
// extension twice panics, because that is a build-time wiring mistake rather
// than a runtime condition worth handling.
func Register(l Language) {
	mu.Lock()
	defer mu.Unlock()
	for _, ext := range l.Extensions() {
		ext = strings.ToLower(ext)
		if prev, ok := registry[ext]; ok {
			panic(fmt.Sprintf("lang: extension %q already registered by %q", ext, prev.Name()))
		}
		registry[ext] = l
	}
}

// For returns the plugin handling a file path, if any.
func For(path string) (Language, bool) {
	mu.RLock()
	defer mu.RUnlock()
	l, ok := registry[strings.ToLower(filepath.Ext(path))]
	return l, ok
}

// Extensions lists every registered extension, sorted. The crawler uses this to
// decide which files are worth reading at all.
func Extensions() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for ext := range registry {
		out = append(out, ext)
	}
	sort.Strings(out)
	return out
}

// reset clears the registry. Test-only.
func reset() {
	mu.Lock()
	defer mu.Unlock()
	registry = map[string]Language{}
}
