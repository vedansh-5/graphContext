package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/vedansh-5/graphcontext/pkg/crawler"
	"github.com/vedansh-5/graphcontext/pkg/lang"
	"github.com/vedansh-5/graphcontext/pkg/resolver"
	"github.com/vedansh-5/graphcontext/pkg/store"
)

// statUnchanged reports whether a file's stat proves it has not changed since
// it was recorded, so it need not be read.
//
// A matching size and mtime is only trusted when the file was last modified
// before the second it was indexed in. A file written in that same second
// could be edited again without its mtime moving, so it is re-hashed instead.
func statUnchanged(rec store.FileRecord, info os.FileInfo) bool {
	mtime := info.ModTime().UnixNano()
	return rec.ContentHash != "" &&
		rec.Size == info.Size() &&
		rec.ModTimeNs == mtime &&
		mtime < rec.IndexedAt.Unix()*int64(time.Second)
}

func hashOf(src []byte) string {
	sum := sha256.Sum256(src)
	return hex.EncodeToString(sum[:])
}

func EnsureFresh(repoRoot string, s *store.Store) (bool, error) {
	stored, err := s.FileRecords()
	if err != nil {
		return false, fmt.Errorf("read file records: %w", err)
	}

	filesChan := make(chan string, 100)
	crawler.Walk(repoRoot, filesChan)

	// fileData describes one source file on disk. src is nil when the file's
	// stat matched its record and it has not been read yet.
	type fileData struct {
		path    string
		rel     string
		hash    string
		src     []byte
		size    int64
		mtimeNs int64
		// restat marks a file whose content is unchanged but whose record
		// carries a stale stat and should be refreshed.
		restat bool
		dirty  bool
	}

	var (
		mu    sync.Mutex
		seen  = make(map[string]bool)
		files []fileData
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

				info, err := os.Stat(p)
				if err != nil {
					continue
				}
				fd := fileData{path: p, rel: rel, size: info.Size(), mtimeNs: info.ModTime().UnixNano()}

				rec, known := stored[rel]
				if known && statUnchanged(rec, info) {
					fd.hash = rec.ContentHash
				} else {
					src, err := os.ReadFile(p)
					if err != nil {
						continue
					}
					fd.src = src
					fd.hash = hashOf(src)
					fd.dirty = !known || rec.ContentHash != fd.hash
					fd.restat = !fd.dirty
				}

				mu.Lock()
				seen[rel] = true
				files = append(files, fd)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// A different plugin or resolver version means the same files now produce
	// a different graph, so the stored one is out of date even if no file changed.
	irVersion := fmt.Sprintf("%d.%d", lang.IRVersion, resolver.Version)
	storedVersion, err := s.Meta(irVersionKey)
	if err != nil {
		return false, fmt.Errorf("read ir version: %w", err)
	}
	staleIR := storedVersion != irVersion

	changed := staleIR && len(files) > 0
	for _, fd := range files {
		if fd.dirty {
			changed = true
			break
		}
	}

	var deleted []string
	for oldRel := range stored {
		if !seen[oldRel] {
			deleted = append(deleted, oldRel)
			changed = true
		}
	}

	batch := store.NewBatch()
	now := time.Now().UTC()
	recorded := 0
	for _, fd := range files {
		if fd.dirty || fd.restat {
			recorded++
			batch.RecordFile(store.FileRecord{
				Path:        fd.rel,
				ContentHash: fd.hash,
				IndexedAt:   now,
				Size:        fd.size,
				ModTimeNs:   fd.mtimeNs,
			})
		}
	}

	if !changed {
		// Nothing to re-index, but remember fresh stats so these files can be
		// skipped without hashing next time.
		if recorded > 0 {
			if err := s.Commit(batch); err != nil {
				return false, fmt.Errorf("commit file records: %w", err)
			}
		}
		return false, nil
	}

	// Unchanged files reuse their cached parse; only the rest are read and parsed.
	var cached map[string]store.FileIR
	if !staleIR {
		if cached, err = s.FileIRs(); err != nil {
			return false, fmt.Errorf("read cached parses: %w", err)
		}
	}

	var (
		parseWg sync.WaitGroup
		fileIRs = make([]*lang.FileIR, len(files))
		fresh   = make([][]byte, len(files))
		jobs    = make(chan int)
	)
	for w := 0; w < runtime.GOMAXPROCS(0); w++ {
		parseWg.Add(1)
		go func() {
			defer parseWg.Done()
			for idx := range jobs {
				fd := files[idx]
				if c, ok := cached[fd.rel]; ok && c.ContentHash == fd.hash {
					if ir, err := decodeIR(c.Data); err == nil {
						fileIRs[idx] = ir
						continue
					}
				}
				plugin, ok := lang.For(fd.rel)
				if !ok {
					continue
				}
				src := fd.src
				if src == nil {
					var err error
					if src, err = os.ReadFile(fd.path); err != nil {
						continue
					}
					// The file may have changed since it was stat'ed; only
					// cache the parse if it still matches the recorded hash.
					if hashOf(src) != fd.hash {
						fd.hash = ""
					}
				}
				ir, err := plugin.Parse(fd.rel, src)
				if err != nil {
					continue
				}
				fileIRs[idx] = ir
				if fd.hash != "" {
					if data, err := encodeIR(ir); err == nil {
						fresh[idx] = data
					}
				}
			}
		}()
	}
	for i := range files {
		jobs <- i
	}
	close(jobs)
	parseWg.Wait()

	var validIRs []*lang.FileIR
	for i, ir := range fileIRs {
		if ir == nil {
			continue
		}
		validIRs = append(validIRs, ir)
		if fresh[i] != nil {
			batch.PutIR(store.FileIR{Path: files[i].rel, ContentHash: files[i].hash, Data: fresh[i]})
		}
	}

	res, err := resolver.Resolve(repoRoot, validIRs)
	if err != nil {
		return false, fmt.Errorf("resolve: %w", err)
	}

	for _, d := range deleted {
		batch.RemoveFile(d)
	}
	if err := queueGraphDiff(s, batch, res); err != nil {
		return false, err
	}

	if err := s.Commit(batch); err != nil {
		return false, fmt.Errorf("commit batch: %w", err)
	}
	if staleIR {
		if err := s.SetMeta(irVersionKey, irVersion); err != nil {
			return false, fmt.Errorf("record ir version: %w", err)
		}
	}

	return true, nil
}
