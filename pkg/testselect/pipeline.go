package testselect

import (
	"github.com/vedansh-5/graphcontext/pkg/analysis"
	"github.com/vedansh-5/graphcontext/pkg/diff"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func SelectTestsFromDiff(repoRoot string, g *analysis.Graph, s *store.Store, gitArgs ...string) (SelectionResult, error) {
	diffs, err := diff.RunGitDiff(repoRoot, gitArgs...)
	if err != nil {
		return SelectionResult{}, err
	}

	changed, err := diff.MapDiffToSymbols(diffs, s)
	if err != nil {
		return SelectionResult{}, err
	}

	return SelectTests(changed, g, 10), nil
}
