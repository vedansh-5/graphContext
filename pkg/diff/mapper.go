package diff

import (
	"path/filepath"
	"sort"

	"github.com/vedansh-5/graphcontext/pkg/store"
)

type ChangedSymbol struct {
	Node         store.Node `json:"node"`
	ChangeType   string     `json:"change_type"`
	MatchedLines []int      `json:"matched_lines"`
}

func MapDiffToSymbols(diffs []FileDiff, s *store.Store) ([]ChangedSymbol, error) {
	results := make([]ChangedSymbol, 0)
	seen := make(map[string]bool)

	for _, fd := range diffs {
		relPath := filepath.ToSlash(fd.Path())
		nodes, err := s.NodesInFile(relPath)
		if err != nil {
			return nil, err
		}

		if len(nodes) == 0 {
			allNodes, err := s.AllNodes()
			if err == nil {
				for _, n := range allNodes {
					if filepath.ToSlash(n.FilePath) == relPath {
						nodes = append(nodes, n)
					}
				}
			}
		}

		if fd.IsDeleted {
			for _, n := range nodes {
				if !seen[n.ID] {
					seen[n.ID] = true
					results = append(results, ChangedSymbol{
						Node:       n,
						ChangeType: "deleted",
					})
				}
			}
			continue
		}

		addedMap := make(map[int]bool, len(fd.AddedLines))
		for _, line := range fd.AddedLines {
			addedMap[line] = true
		}

		deletedMap := make(map[int]bool, len(fd.DeletedLines))
		for _, line := range fd.DeletedLines {
			deletedMap[line] = true
		}

		for _, n := range nodes {
			if seen[n.ID] {
				continue
			}

			var matched []int
			for line := n.StartLine; line <= n.EndLine; line++ {
				if addedMap[line] {
					matched = append(matched, line)
				}
			}

			if len(matched) > 0 {
				seen[n.ID] = true
				changeType := "modified"
				if fd.IsNew || len(matched) >= (n.EndLine-n.StartLine+1) {
					changeType = "added"
				}
				results = append(results, ChangedSymbol{
					Node:         n,
					ChangeType:   changeType,
					MatchedLines: matched,
				})
				continue
			}

			for line := n.StartLine; line <= n.EndLine; line++ {
				if deletedMap[line] {
					seen[n.ID] = true
					results = append(results, ChangedSymbol{
						Node:         n,
						ChangeType:   "modified",
						MatchedLines: []int{line},
					})
					break
				}
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Node.ID < results[j].Node.ID
	})

	return results, nil
}
