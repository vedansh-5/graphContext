package archlint

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vedansh-5/graphcontext/pkg/analysis"
	"github.com/vedansh-5/graphcontext/pkg/diff"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

var dependencyKinds = map[store.EdgeKind]bool{
	store.EdgeCalls:      true,
	store.EdgeImports:    true,
	store.EdgeInherits:   true,
	store.EdgeImplements: true,
	store.EdgeReferences: true,
}

func LintGraph(g *analysis.Graph, rules RuleSet) []Violation {
	var violations []Violation

	for _, edges := range g.Out {
		for _, e := range edges {
			if !dependencyKinds[e.Kind] {
				continue
			}
			violations = append(violations, evaluateEdge(e, g, rules)...)
		}
	}

	sortViolations(violations)
	return violations
}

func LintDiff(changed []diff.ChangedSymbol, g *analysis.Graph, rules RuleSet) []Violation {
	var violations []Violation
	seenEdges := make(map[string]bool)

	for _, cs := range changed {
		for _, e := range g.Out[cs.Node.ID] {
			if !dependencyKinds[e.Kind] {
				continue
			}
			edgeKey := fmt.Sprintf("%s->%s:%s:%d", e.SourceID, e.TargetID, e.Kind, e.Line)
			if seenEdges[edgeKey] {
				continue
			}
			seenEdges[edgeKey] = true
			violations = append(violations, evaluateEdge(e, g, rules)...)
		}
	}

	sortViolations(violations)
	return violations
}

func evaluateEdge(e store.Edge, g *analysis.Graph, rules RuleSet) []Violation {
	srcNode, okSrc := g.Nodes[e.SourceID]
	tgtNode, okTgt := g.Nodes[e.TargetID]
	if !okSrc || !okTgt {
		return nil
	}

	srcFile := filepath.ToSlash(srcNode.FilePath)
	tgtFile := filepath.ToSlash(tgtNode.FilePath)
	if srcFile == tgtFile {
		return nil
	}

	var results []Violation

	for _, fr := range rules.Forbidden {
		if matchPathPattern(fr.From, srcFile) && matchPathPattern(fr.To, tgtFile) {
			msg := fr.Message
			if msg == "" {
				msg = fmt.Sprintf("forbidden dependency: %s cannot depend on %s", fr.From, fr.To)
			}
			results = append(results, Violation{
				RuleType:   "forbidden",
				SourceID:   srcNode.ID,
				SourceFile: srcFile,
				SourceLine: e.Line,
				TargetID:   tgtNode.ID,
				TargetFile: tgtFile,
				EdgeKind:   e.Kind,
				Message:    msg,
			})
		}
	}

	if rules.Layers != nil && len(rules.Layers.Layers) > 0 {
		srcIdx := findLayerIndex(rules.Layers.Layers, srcFile)
		tgtIdx := findLayerIndex(rules.Layers.Layers, tgtFile)

		if srcIdx != -1 && tgtIdx != -1 && srcIdx > tgtIdx {
			msg := fmt.Sprintf("layer violation: innermost layer %q cannot depend on outer layer %q",
				rules.Layers.Layers[srcIdx], rules.Layers.Layers[tgtIdx])
			results = append(results, Violation{
				RuleType:   "layer_inversion",
				SourceID:   srcNode.ID,
				SourceFile: srcFile,
				SourceLine: e.Line,
				TargetID:   tgtNode.ID,
				TargetFile: tgtFile,
				EdgeKind:   e.Kind,
				Message:    msg,
			})
		}
	}

	return results
}

func findLayerIndex(layers []string, path string) int {
	for i, l := range layers {
		if matchPathPattern(l, path) {
			return i
		}
	}
	return -1
}

func matchPathPattern(pattern, path string) bool {
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)

	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(path, prefix+"/")
	}

	if strings.Contains(pattern, "*") {
		matched, err := filepath.Match(pattern, path)
		if err == nil && matched {
			return true
		}
	}

	return path == pattern || strings.HasPrefix(path, pattern+"/")
}

func sortViolations(vs []Violation) {
	sort.Slice(vs, func(i, j int) bool {
		if vs[i].SourceFile != vs[j].SourceFile {
			return vs[i].SourceFile < vs[j].SourceFile
		}
		if vs[i].SourceLine != vs[j].SourceLine {
			return vs[i].SourceLine < vs[j].SourceLine
		}
		if vs[i].SourceID != vs[j].SourceID {
			return vs[i].SourceID < vs[j].SourceID
		}
		return vs[i].TargetID < vs[j].TargetID
	})
}
