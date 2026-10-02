package crawler

import (
	"io/fs"
	"path/filepath"

	"github.com/vedansh-5/graphcontext/pkg/lang"
)

// IgnoreDirs are directory names skipped everywhere: by the crawler when
// indexing and by the watcher when listening for changes. Both must agree, or
// a file could be indexed but never refreshed.
var IgnoreDirs = map[string]bool{
	".git":         true,
	".github":      true,
	".idea":        true,
	".vscode":      true,
	".cache":       true,
	".venv":        true,
	"venv":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"coverage":     true,
	"__pycache__":  true,
}

// IsSourceFile reports whether a registered language plugin claims the file.
func IsSourceFile(path string) bool {
	_, ok := lang.For(path)
	return ok
}

// Walk finds all source files in the rootDir and sends them to the filesChannel.
// It runs concurrently in a goroutine and closes the channel when done.
func Walk(rootDir string, filesChan chan<- string) {
	// We span a goroutine so the Walk function returns immediately.
	// this allows the caller to start consuming from the channel rightaway.
	go func() {
		// ensure the channel is closed when the walk finishes, signaling workers to stop.
		defer close(filesChan)
		filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// if we hit permission issues or missing files, ignore and continue
				return nil
			}
			if d.IsDir() {
				// if its a directory -> we want to ignore, tell WalkDir to skip it entirely.
				if IgnoreDirs[d.Name()] {
					return fs.SkipDir
				}
				return nil
			}
			// Only files a language plugin can parse are worth emitting.
			if IsSourceFile(path) {
				// send the discovered file path into our pipeline
				// this blocks if the channel is full, creating natural backpressure.
				filesChan <- path
			}
			return nil
		})
	}()
}
