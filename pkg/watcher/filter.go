package watcher

import (
	"path/filepath"
	"strings"

	"github.com/vedansh-5/graphcontext/pkg/lang"
)

var defaultIgnoredDirs = map[string]bool{
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

func shouldIgnoreDir(dirName string, customIgnored map[string]bool) bool {
	if customIgnored != nil && customIgnored[dirName] {
		return true
	}
	return defaultIgnoredDirs[dirName]
}

var defaultCodeExtensions = map[string]bool{
	".go":  true,
	".py":  true,
	".ts":  true,
	".tsx": true,
	".js":  true,
	".jsx": true,
}

func isAcceptedCodeFile(path string, customExtensions map[string]bool) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}
	if len(customExtensions) > 0 {
		return customExtensions[ext]
	}
	if defaultCodeExtensions[ext] {
		return true
	}
	_, ok := lang.For(path)
	return ok
}
