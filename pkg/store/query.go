package store

import (
	"database/sql"
	"fmt"
)

const nodeCols = `id, kind, name, qualified_name, file_path,
	start_line, end_line, start_byte, end_byte, language,
	signature, docstring, is_test, is_entrypoint, entrypoint_kind, visibility`

const edgeCols = `source_id, target_id, kind, line, confidence, candidate_count`

// Node fetches one node by ID. A missing node returns (nil, nil).
func (s *Store) Node(id string) (*Node, error) {
	row := s.conn.QueryRow(`SELECT `+nodeCols+` FROM nodes WHERE id = ?`, id)
	n, err := scanNode(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read node %s: %w", id, err)
	}
	return n, nil
}

// NodesInFile returns every node owned by a file, ordered for determinism.
func (s *Store) NodesInFile(path string) ([]Node, error) {
	return s.queryNodes(
		`SELECT `+nodeCols+` FROM nodes WHERE file_path = ? ORDER BY start_byte, id`, path)
}

// AllNodes returns every node, ordered by ID. Used to load the in-memory graph.
func (s *Store) AllNodes() ([]Node, error) {
	return s.queryNodes(`SELECT ` + nodeCols + ` FROM nodes ORDER BY id`)
}

// AllEdges returns every edge, ordered deterministically.
func (s *Store) AllEdges() ([]Edge, error) {
	return s.queryEdges(`SELECT ` + edgeCols + ` FROM edges
		ORDER BY source_id, target_id, kind, line`)
}

// OutEdges returns edges leaving a node.
func (s *Store) OutEdges(id string) ([]Edge, error) {
	return s.queryEdges(`SELECT `+edgeCols+` FROM edges WHERE source_id = ?
		ORDER BY target_id, kind, line`, id)
}

// InEdges returns edges arriving at a node — the reverse direction that powers
// caller and impact queries.
func (s *Store) InEdges(id string) ([]Edge, error) {
	return s.queryEdges(`SELECT `+edgeCols+` FROM edges WHERE target_id = ?
		ORDER BY source_id, kind, line`, id)
}

// FileHashes returns path -> content_hash for every indexed file. The indexer
// diffs this against the working tree to decide what to re-parse.
func (s *Store) FileHashes() (map[string]string, error) {
	rows, err := s.conn.Query(`SELECT path, content_hash FROM files ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("read file hashes: %w", err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var p, h string
		if err := rows.Scan(&p, &h); err != nil {
			return nil, fmt.Errorf("scan file hash: %w", err)
		}
		out[p] = h
	}
	return out, rows.Err()
}

// ConfidenceCounts returns how many edges carry each confidence level. This is
// the raw material for the resolution-rate metric.
func (s *Store) ConfidenceCounts() (map[Confidence]int, error) {
	rows, err := s.conn.Query(
		`SELECT confidence, COUNT(*) FROM edges GROUP BY confidence ORDER BY confidence`)
	if err != nil {
		return nil, fmt.Errorf("count confidence: %w", err)
	}
	defer rows.Close()

	out := map[Confidence]int{}
	for rows.Next() {
		var c string
		var n int
		if err := rows.Scan(&c, &n); err != nil {
			return nil, fmt.Errorf("scan confidence count: %w", err)
		}
		out[Confidence(c)] = n
	}
	return out, rows.Err()
}

func (s *Store) queryNodes(q string, args ...any) ([]Node, error) {
	rows, err := s.conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query nodes: %w", err)
	}
	defer rows.Close()

	var out []Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

func (s *Store) queryEdges(q string, args ...any) ([]Edge, error) {
	rows, err := s.conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()

	var out []Edge
	for rows.Next() {
		var e Edge
		var kind, conf string
		if err := rows.Scan(&e.SourceID, &e.TargetID, &kind, &e.Line,
			&conf, &e.CandidateCount); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		e.Kind = EdgeKind(kind)
		e.Confidence = Confidence(conf)
		out = append(out, e)
	}
	return out, rows.Err()
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

func scanNode(sc scanner) (*Node, error) {
	var n Node
	var kind, entry, vis string
	var isTest, isEntry int
	if err := sc.Scan(&n.ID, &kind, &n.Name, &n.QualifiedName, &n.FilePath,
		&n.StartLine, &n.EndLine, &n.StartByte, &n.EndByte, &n.Language,
		&n.Signature, &n.Docstring, &isTest, &isEntry, &entry, &vis); err != nil {
		return nil, err
	}
	n.Kind = NodeKind(kind)
	n.IsTest = isTest != 0
	n.IsEntrypoint = isEntry != 0
	n.EntrypointKind = EntrypointKind(entry)
	n.Visibility = Visibility(vis)
	return &n, nil
}
