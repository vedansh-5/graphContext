package resolver

import (
	"fmt"
	"sort"

	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

type ResolutionStats struct {
	TotalRefs      int
	Exact          int
	Ambiguous      int
	NameMatch      int
	Unknown        int
	ResolutionRate float64
}

type ResolutionResult struct {
	Nodes []store.Node
	Edges []store.Edge
	Stats ResolutionStats
}

func Resolve(repoRoot string, files []*lang.FileIR) (*ResolutionResult, error) {
	idx := buildIndex(repoRoot, files)

	var edges []store.Edge
	var extNodes []store.Node
	seenExt := make(map[string]bool)

	extNode := func(name string) string {
		id := "external:" + name
		if !seenExt[id] {
			seenExt[id] = true
			extNodes = append(extNodes, store.Node{
				ID:             id,
				Kind:           store.KindExternal,
				Name:           name,
				QualifiedName:  name,
				EntrypointKind: store.EntryNone,
				Visibility:     store.VisExported,
			})
		}
		return id
	}

	stats := ResolutionStats{}

	for _, f := range files {
		for _, ref := range f.Refs {
			stats.TotalRefs++
			resolved := false

			if ref.Kind == store.EdgeInherits || ref.Kind == store.EdgeImplements {
				if targetID, ok := idx.bindings[f.Path][ref.Name]; ok {
					edges = append(edges, store.Edge{
						SourceID:   ref.FromID,
						TargetID:   targetID,
						Kind:       ref.Kind,
						Line:       ref.Line,
						Confidence: store.ConfExact,
					})
					stats.Exact++
					continue
				}
				if node, ok := idx.inFile[f.Path][ref.Name]; ok {
					edges = append(edges, store.Edge{
						SourceID:   ref.FromID,
						TargetID:   node.ID,
						Kind:       ref.Kind,
						Line:       ref.Line,
						Confidence: store.ConfExact,
					})
					stats.Exact++
					continue
				}
				if nodes, ok := idx.byName[ref.Name]; ok && len(nodes) == 1 {
					edges = append(edges, store.Edge{
						SourceID:   ref.FromID,
						TargetID:   nodes[0].ID,
						Kind:       ref.Kind,
						Line:       ref.Line,
						Confidence: store.ConfExact,
					})
					stats.Exact++
					continue
				}
				targetID := extNode(ref.Name)
				edges = append(edges, store.Edge{
					SourceID:   ref.FromID,
					TargetID:   targetID,
					Kind:       ref.Kind,
					Line:       ref.Line,
					Confidence: store.ConfUnknown,
				})
				stats.Unknown++
				continue
			}

			if ref.Receiver == "" {
				if node, ok := idx.inFile[f.Path][ref.Name]; ok {
					edges = append(edges, store.Edge{
						SourceID:   ref.FromID,
						TargetID:   node.ID,
						Kind:       ref.Kind,
						Line:       ref.Line,
						Confidence: store.ConfExact,
					})
					stats.Exact++
					resolved = true
				} else if targetID, ok := idx.bindings[f.Path][ref.Name]; ok {
					edges = append(edges, store.Edge{
						SourceID:   ref.FromID,
						TargetID:   targetID,
						Kind:       ref.Kind,
						Line:       ref.Line,
						Confidence: store.ConfExact,
					})
					stats.Exact++
					resolved = true
				} else if nodes, ok := idx.byName[ref.Name]; ok {
					if len(nodes) == 1 {
						edges = append(edges, store.Edge{
							SourceID:   ref.FromID,
							TargetID:   nodes[0].ID,
							Kind:       ref.Kind,
							Line:       ref.Line,
							Confidence: store.ConfNameMatch,
						})
						stats.NameMatch++
						resolved = true
					} else {
						for _, n := range nodes {
							edges = append(edges, store.Edge{
								SourceID:       ref.FromID,
								TargetID:       n.ID,
								Kind:           ref.Kind,
								Line:           ref.Line,
								Confidence:     store.ConfAmbiguous,
								CandidateCount: len(nodes),
							})
						}
						stats.Ambiguous++
						resolved = true
					}
				}

				if !resolved {
					targetID := extNode(ref.Name)
					edges = append(edges, store.Edge{
						SourceID:   ref.FromID,
						TargetID:   targetID,
						Kind:       ref.Kind,
						Line:       ref.Line,
						Confidence: store.ConfUnknown,
					})
					stats.Unknown++
				}
				continue
			}

			recvType := idx.resolveReceiverType(ref.FromID, ref.Receiver)
			if recvType != "" {
				methods := idx.findMethodInHierarchy(recvType, ref.Name)
				if len(methods) == 1 {
					edges = append(edges, store.Edge{
						SourceID:   ref.FromID,
						TargetID:   methods[0].ID,
						Kind:       ref.Kind,
						Line:       ref.Line,
						Confidence: store.ConfExact,
					})
					stats.Exact++
					resolved = true
				} else if len(methods) > 1 {
					for _, m := range methods {
						edges = append(edges, store.Edge{
							SourceID:       ref.FromID,
							TargetID:       m.ID,
							Kind:           ref.Kind,
							Line:           ref.Line,
							Confidence:     store.ConfAmbiguous,
							CandidateCount: len(methods),
						})
					}
					stats.Ambiguous++
					resolved = true
				}
			}

			if !resolved {
				if targetFile, ok := idx.importAliases[f.Path][ref.Receiver]; ok {
					if node, ok := idx.inFile[targetFile][ref.Name]; ok {
						edges = append(edges, store.Edge{
							SourceID:   ref.FromID,
							TargetID:   node.ID,
							Kind:       ref.Kind,
							Line:       ref.Line,
							Confidence: store.ConfExact,
						})
						stats.Exact++
						resolved = true
					}
				}
			}

			if !resolved {
				var methodCandidates []store.Node
				if nodes, ok := idx.byName[ref.Name]; ok {
					for _, n := range nodes {
						if n.Kind == store.KindMethod {
							methodCandidates = append(methodCandidates, n)
						}
					}
				}
				if len(methodCandidates) == 1 {
					edges = append(edges, store.Edge{
						SourceID:   ref.FromID,
						TargetID:   methodCandidates[0].ID,
						Kind:       ref.Kind,
						Line:       ref.Line,
						Confidence: store.ConfNameMatch,
					})
					stats.NameMatch++
					resolved = true
				} else if len(methodCandidates) > 1 {
					for _, m := range methodCandidates {
						edges = append(edges, store.Edge{
							SourceID:       ref.FromID,
							TargetID:       m.ID,
							Kind:           ref.Kind,
							Line:           ref.Line,
							Confidence:     store.ConfAmbiguous,
							CandidateCount: len(methodCandidates),
						})
					}
					stats.Ambiguous++
					resolved = true
				}
			}

			if !resolved {
				targetName := fmt.Sprintf("%s.%s", ref.Receiver, ref.Name)
				targetID := extNode(targetName)
				edges = append(edges, store.Edge{
					SourceID:   ref.FromID,
					TargetID:   targetID,
					Kind:       ref.Kind,
					Line:       ref.Line,
					Confidence: store.ConfUnknown,
				})
				stats.Unknown++
			}
		}
	}

	for _, n := range idx.nodes {
		if n.Kind != store.KindInterface {
			continue
		}
		ifaceName := n.Name
		var ifaceMethods []string
		if methods, ok := idx.typeMethods[ifaceName]; ok {
			for mn := range methods {
				ifaceMethods = append(ifaceMethods, mn)
			}
		}
		for _, f := range files {
			if ms, ok := f.Types.Methods[ifaceName]; ok {
				for _, m := range ms {
					ifaceMethods = lang.InsertSorted(ifaceMethods, m)
				}
			}
		}
		if len(ifaceMethods) == 0 {
			continue
		}

		for _, candidate := range idx.nodes {
			if candidate.Kind != store.KindClass || candidate.ID == n.ID {
				continue
			}
			className := candidate.Name
			hasAll := true
			for _, reqMethod := range ifaceMethods {
				ms := idx.findMethodInHierarchy(className, reqMethod)
				if len(ms) == 0 {
					var inFacts bool
					for _, f := range files {
						if clsMs, ok := f.Types.Methods[className]; ok {
							for _, m := range clsMs {
								if m == reqMethod {
									inFacts = true
									break
								}
							}
						}
					}
					if !inFacts {
						hasAll = false
						break
					}
				}
			}
			if hasAll {
				edges = append(edges, store.Edge{
					SourceID:   candidate.ID,
					TargetID:   n.ID,
					Kind:       store.EdgeImplements,
					Line:       candidate.StartLine,
					Confidence: store.ConfExact,
				})
			}
		}
	}

	if stats.TotalRefs > 0 {
		stats.ResolutionRate = float64(stats.Exact+stats.NameMatch) / float64(stats.TotalRefs) * 100
	}

	allNodes := append(idx.nodes, extNodes...)
	sort.SliceStable(allNodes, func(i, j int) bool {
		if allNodes[i].FilePath != allNodes[j].FilePath {
			return allNodes[i].FilePath < allNodes[j].FilePath
		}
		if allNodes[i].StartByte != allNodes[j].StartByte {
			return allNodes[i].StartByte < allNodes[j].StartByte
		}
		return allNodes[i].ID < allNodes[j].ID
	})

	edgeMap := make(map[string]store.Edge)
	for _, e := range edges {
		k := fmt.Sprintf("%s|%s|%s|%d", e.SourceID, e.TargetID, e.Kind, e.Line)
		edgeMap[k] = e
	}
	var dedupEdges []store.Edge
	for _, e := range edgeMap {
		dedupEdges = append(dedupEdges, e)
	}
	sort.SliceStable(dedupEdges, func(i, j int) bool {
		if dedupEdges[i].SourceID != dedupEdges[j].SourceID {
			return dedupEdges[i].SourceID < dedupEdges[j].SourceID
		}
		if dedupEdges[i].TargetID != dedupEdges[j].TargetID {
			return dedupEdges[i].TargetID < dedupEdges[j].TargetID
		}
		if dedupEdges[i].Kind != dedupEdges[j].Kind {
			return dedupEdges[i].Kind < dedupEdges[j].Kind
		}
		return dedupEdges[i].Line < dedupEdges[j].Line
	})

	return &ResolutionResult{
		Nodes: allNodes,
		Edges: dedupEdges,
		Stats: stats,
	}, nil
}
