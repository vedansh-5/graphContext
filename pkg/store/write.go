package store

import (
	"database/sql"
	"fmt"
)

// Batch accumulates one indexing pass worth of changes, written in a single
// transaction. Batching is not an optimization detail: the measured v1 pipeline
// spent ~75us per unbatched write and that, not parsing, was the bottleneck.
type Batch struct {
	nodes []Node
	edges []Edge
	// touched files are re-indexed: their existing nodes are purged first.
	touched []FileRecord
	// removed files are gone from disk: purge nodes and forget the file record.
	removed []string
}

// NewBatch returns an empty batch.
func NewBatch() *Batch { return &Batch{} }

// AddNode queues a node insert.
func (b *Batch) AddNode(n Node) { b.nodes = append(b.nodes, n) }

// AddEdge queues an edge insert.
func (b *Batch) AddEdge(e Edge) { b.edges = append(b.edges, e) }

// TouchFile marks a file as (re-)indexed in this batch. Every node previously
// owned by this path is deleted before the batch's new nodes are inserted, so a
// re-index replaces rather than accumulates.
func (b *Batch) TouchFile(f FileRecord) { b.touched = append(b.touched, f) }

// RemoveFile marks a file as deleted from disk.
func (b *Batch) RemoveFile(path string) { b.removed = append(b.removed, path) }

// Len reports how many nodes and edges are queued.
func (b *Batch) Len() (nodes, edges int) { return len(b.nodes), len(b.edges) }

// Commit writes the whole batch in one transaction.
//
// Order matters: purges run first, then every node, then every edge. Edges carry
// foreign keys onto nodes, so all nodes in the batch must exist before any edge
// references them.
func (s *Store) Commit(b *Batch) error {
	tx, err := s.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, f := range b.touched {
		if err := purgeFile(tx, f.Path); err != nil {
			return err
		}
	}
	for _, p := range b.removed {
		if err := purgeFile(tx, p); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM files WHERE path = ?`, p); err != nil {
			return fmt.Errorf("delete file record %s: %w", p, err)
		}
	}

	if err := insertNodes(tx, b.nodes); err != nil {
		return err
	}
	if err := insertEdges(tx, b.edges); err != nil {
		return err
	}

	fileStmt, err := tx.Prepare(
		`INSERT INTO files(path, content_hash, indexed_at) VALUES(?, ?, ?)
		 ON CONFLICT(path) DO UPDATE SET
		   content_hash = excluded.content_hash,
		   indexed_at   = excluded.indexed_at`)
	if err != nil {
		return fmt.Errorf("prepare file insert: %w", err)
	}
	defer fileStmt.Close()
	for _, f := range b.touched {
		if _, err := fileStmt.Exec(f.Path, f.ContentHash, f.IndexedAt.Unix()); err != nil {
			return fmt.Errorf("insert file %s: %w", f.Path, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// purgeFile removes every node owned by a path, along with its FTS rows.
// Edges vanish through ON DELETE CASCADE.
func purgeFile(tx *sql.Tx, path string) error {
	if _, err := tx.Exec(
		`DELETE FROM symbols_fts WHERE node_id IN
		   (SELECT id FROM nodes WHERE file_path = ?)`, path); err != nil {
		return fmt.Errorf("purge fts for %s: %w", path, err)
	}
	if _, err := tx.Exec(`DELETE FROM nodes WHERE file_path = ?`, path); err != nil {
		return fmt.Errorf("purge nodes for %s: %w", path, err)
	}
	return nil
}

func insertNodes(tx *sql.Tx, nodes []Node) error {
	if len(nodes) == 0 {
		return nil
	}
	stmt, err := tx.Prepare(`
		INSERT INTO nodes(id, kind, name, qualified_name, file_path,
			start_line, end_line, start_byte, end_byte, language,
			signature, docstring, is_test, is_entrypoint, entrypoint_kind, visibility)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			kind=excluded.kind, name=excluded.name,
			qualified_name=excluded.qualified_name, file_path=excluded.file_path,
			start_line=excluded.start_line, end_line=excluded.end_line,
			start_byte=excluded.start_byte, end_byte=excluded.end_byte,
			language=excluded.language, signature=excluded.signature,
			docstring=excluded.docstring, is_test=excluded.is_test,
			is_entrypoint=excluded.is_entrypoint,
			entrypoint_kind=excluded.entrypoint_kind, visibility=excluded.visibility`)
	if err != nil {
		return fmt.Errorf("prepare node insert: %w", err)
	}
	defer stmt.Close()

	ftsDel, err := tx.Prepare(`DELETE FROM symbols_fts WHERE node_id = ?`)
	if err != nil {
		return fmt.Errorf("prepare fts delete: %w", err)
	}
	defer ftsDel.Close()

	ftsIns, err := tx.Prepare(
		`INSERT INTO symbols_fts(node_id, tokens, docstring) VALUES(?,?,?)`)
	if err != nil {
		return fmt.Errorf("prepare fts insert: %w", err)
	}
	defer ftsIns.Close()

	for _, n := range nodes {
		if _, err := stmt.Exec(n.ID, string(n.Kind), n.Name, n.QualifiedName,
			n.FilePath, n.StartLine, n.EndLine, n.StartByte, n.EndByte,
			n.Language, n.Signature, n.Docstring,
			boolToInt(n.IsTest), boolToInt(n.IsEntrypoint),
			string(n.EntrypointKind), string(n.Visibility)); err != nil {
			return fmt.Errorf("insert node %s: %w", n.ID, err)
		}
		// External nodes are synthetic and not worth searching.
		if n.Kind == KindExternal {
			continue
		}
		if _, err := ftsDel.Exec(n.ID); err != nil {
			return fmt.Errorf("clear fts for %s: %w", n.ID, err)
		}
		if _, err := ftsIns.Exec(n.ID, ftsTokens(n), n.Docstring); err != nil {
			return fmt.Errorf("index fts for %s: %w", n.ID, err)
		}
	}
	return nil
}

func insertEdges(tx *sql.Tx, edges []Edge) error {
	if len(edges) == 0 {
		return nil
	}
	stmt, err := tx.Prepare(`
		INSERT INTO edges(source_id, target_id, kind, line, confidence, candidate_count)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(source_id, target_id, kind, line) DO UPDATE SET
			confidence=excluded.confidence,
			candidate_count=excluded.candidate_count`)
	if err != nil {
		return fmt.Errorf("prepare edge insert: %w", err)
	}
	defer stmt.Close()

	for _, e := range edges {
		cc := e.CandidateCount
		if cc == 0 {
			cc = 1
		}
		if _, err := stmt.Exec(e.SourceID, e.TargetID, string(e.Kind),
			e.Line, string(e.Confidence), cc); err != nil {
			return fmt.Errorf("insert edge %s->%s: %w", e.SourceID, e.TargetID, err)
		}
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
