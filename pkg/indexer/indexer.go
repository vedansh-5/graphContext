package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vedansh-5/graphcontext/pkg/crawler"
	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/resolver"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

func EnsureFresh(repoRoot string, s *store.Store) (bool, error) {
	stored, err := s.FileHashes()
	if err != nil {
		return false, fmt.Errorf("read file hashes: %w", err)
	}

	filesChan := make(chan string, 100)
	crawler.Walk(repoRoot, filesChan)

	type fileData struct {
		path string
		rel  string
		hash string
		src  []byte
	}

	var (
		mu      sync.Mutex
		seen    = make(map[string]bool)
		changed = false
		toParse []fileData
	)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range filesChan {
				rel, err := filepath.Rel(repoRoot, p)
				if err != nil {
					continue
				}
				rel = filepath.ToSlash(rel)
				if _, ok := lang.For(rel); !ok {
					continue
				}

				src, err := os.ReadFile(p)
				if err != nil {
					continue
				}
				sum := sha256.Sum256(src)
				h := hex.EncodeToString(sum[:])

				mu.Lock()
				seen[rel] = true
				if stored[rel] != h {
					changed = true
				}
				toParse = append(toParse, fileData{path: p, rel: rel, hash: h, src: src})
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	var deleted []string
	for oldRel := range stored {
		if !seen[oldRel] {
			deleted = append(deleted, oldRel)
			changed = true
		}
	}

	if !changed {
		return false, nil
	}

	var (
		parseWg   sync.WaitGroup
		fileIRs   = make([]*lang.FileIR, len(toParse))
		parseErrs = make([]error, len(toParse))
	)

	for i := range toParse {
		parseWg.Add(1)
		go func(idx int) {
			defer parseWg.Done()
			fd := toParse[idx]
			plugin, ok := lang.For(fd.rel)
			if !ok {
				return
			}
			ir, err := plugin.Parse(fd.rel, fd.src)
			if err != nil {
				parseErrs[idx] = err
				return
			}
			fileIRs[idx] = ir
		}(i)
	}
	parseWg.Wait()

	var validIRs []*lang.FileIR
	for i, ir := range fileIRs {
		if parseErrs[i] == nil && ir != nil {
			validIRs = append(validIRs, ir)
		}
	}

	res, err := resolver.Resolve(repoRoot, validIRs)
	if err != nil {
		return false, fmt.Errorf("resolve: %w", err)
	}

	batch := store.NewBatch()
	now := time.Now().UTC()

	for _, fd := range toParse {
		batch.TouchFile(store.FileRecord{
			Path:        fd.rel,
			ContentHash: fd.hash,
			IndexedAt:   now,
		})
	}
	for _, d := range deleted {
		batch.RemoveFile(d)
	}

	for _, n := range res.Nodes {
		batch.AddNode(n)
	}
	for _, e := range res.Edges {
		batch.AddEdge(e)
	}

	if err := s.Commit(batch); err != nil {
		return false, fmt.Errorf("commit batch: %w", err)
	}

	return true, nil
}
