package testselect

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vedansh-5/graphcontext/pkg/analysis"
	"github.com/vedansh-5/graphcontext/pkg/diff"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

type TestMatch struct {
	TestNode   store.Node       `json:"test_node"`
	TargetID   string           `json:"target_id"`
	Depth      int              `json:"depth"`
	Confidence store.Confidence `json:"confidence"`
}

type SelectionResult struct {
	ChangedSymbols         []diff.ChangedSymbol `json:"changed_symbols"`
	AffectedTests          []TestMatch          `json:"affected_tests"`
	TestFiles              []string             `json:"test_files"`
	TotalTestsInRepo       int                  `json:"total_tests_in_repo"`
	AffectedTestsCount     int                  `json:"affected_tests_count"`
	TotalTestFilesInRepo   int                  `json:"total_test_files_in_repo"`
	AffectedTestFilesCount int                  `json:"affected_test_files_count"`
	ReductionPercent       float64              `json:"reduction_percent"`
	Caveats                []string             `json:"caveats,omitempty"`
}

func SelectTests(changed []diff.ChangedSymbol, g *analysis.Graph, maxDepth int) SelectionResult {
	if maxDepth <= 0 {
		maxDepth = 10
	}

	allTestFiles := make(map[string]bool)
	allTestsCount := 0
	for _, n := range g.Nodes {
		if isTestNode(n) {
			allTestsCount++
			if n.FilePath != "" {
				allTestFiles[n.FilePath] = true
			}
		}
	}

	kinds := map[store.EdgeKind]bool{
		store.EdgeCalls:      true,
		store.EdgeInherits:   true,
		store.EdgeOverrides:  true,
		store.EdgeImplements: true,
	}

	bestMatches := make(map[string]TestMatch)
	lowConfidenceCount := 0

	for _, cs := range changed {
		if isTestNode(cs.Node) {
			bestMatches[cs.Node.ID] = TestMatch{
				TestNode:   cs.Node,
				TargetID:   cs.Node.ID,
				Depth:      0,
				Confidence: store.ConfExact,
			}
		}

		reached, _ := g.ReverseReach(cs.Node.ID, kinds, maxDepth, 2000)
		for _, r := range reached {
			if !isTestNode(r.Node) {
				continue
			}

			if r.Confidence != store.ConfExact {
				lowConfidenceCount++
			}

			existing, found := bestMatches[r.Node.ID]
			if !found || r.Depth < existing.Depth {
				bestMatches[r.Node.ID] = TestMatch{
					TestNode:   r.Node,
					TargetID:   cs.Node.ID,
					Depth:      r.Depth,
					Confidence: r.Confidence,
				}
			}
		}
	}

	matches := make([]TestMatch, 0)
	testFilesMap := make(map[string]bool)

	for _, m := range bestMatches {
		matches = append(matches, m)
		if m.TestNode.FilePath != "" {
			testFilesMap[m.TestNode.FilePath] = true
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Depth != matches[j].Depth {
			return matches[i].Depth < matches[j].Depth
		}
		if matches[i].Confidence != matches[j].Confidence {
			return matches[i].Confidence == store.ConfExact
		}
		return matches[i].TestNode.ID < matches[j].TestNode.ID
	})

	testFiles := make([]string, 0)
	for f := range testFilesMap {
		testFiles = append(testFiles, f)
	}
	sort.Strings(testFiles)

	reduction := 0.0
	if allTestsCount > 0 {
		saved := allTestsCount - len(matches)
		if saved > 0 {
			reduction = (float64(saved) / float64(allTestsCount)) * 100.0
		}
	}

	var caveats []string
	if lowConfidenceCount > 0 {
		caveats = append(caveats, fmt.Sprintf("%d test connections traversed via name_match or heuristic edges", lowConfidenceCount))
	}

	return SelectionResult{
		ChangedSymbols:         changed,
		AffectedTests:          matches,
		TestFiles:              testFiles,
		TotalTestsInRepo:       allTestsCount,
		AffectedTestsCount:     len(matches),
		TotalTestFilesInRepo:   len(allTestFiles),
		AffectedTestFilesCount: len(testFiles),
		ReductionPercent:       reduction,
		Caveats:                caveats,
	}
}

func isTestNode(n store.Node) bool {
	if n.IsTest {
		return true
	}
	if strings.HasPrefix(n.Name, "Test") || strings.HasPrefix(n.Name, "test_") {
		return true
	}

	lower := strings.ToLower(n.FilePath)
	if strings.HasSuffix(lower, "_test.go") || strings.HasSuffix(lower, ".test.ts") ||
		strings.HasSuffix(lower, ".spec.ts") || strings.HasPrefix(lower, "test_") ||
		strings.Contains(lower, "/tests/") || strings.Contains(lower, "/test/") {
		if n.Kind == store.KindFunction || n.Kind == store.KindMethod {
			return true
		}
	}
	return false
}
