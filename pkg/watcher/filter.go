package watcher

import (
	"path/filepath"
	"strings"

	"github.com/vedansh-5/graphcontext/pkg/crawler"
)

func shouldIgnoreDir(dirName string, customIgnored map[string]bool) bool {
	if customIgnored != nil && customIgnored[dirName] {
		return true
	}
	return crawler.IgnoreDirs[dirName]
}

func isAcceptedCodeFile(path string, customExtensions map[string]bool) bool {
	if len(customExtensions) > 0 {
		return customExtensions[strings.ToLower(filepath.Ext(path))]
	}
	return crawler.IsSourceFile(path)
}
