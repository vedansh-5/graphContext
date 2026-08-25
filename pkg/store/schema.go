package store

import "fmt"

// SchemaVersion is bumped whenever the DDL below changes in a way that makes
// an existing database unreadable. Open() wipes and rebuilds on mismatch.
const SchemaVersion = 3

const schemaDDL = `
CREATE TABLE IF NOT EXISTS nodes (
	id              TEXT PRIMARY KEY,
	kind            TEXT NOT NULL,
	name            TEXT NOT NULL,
	qualified_name  TEXT NOT NULL,
	file_path       TEXT NOT NULL,
	start_line      INTEGER NOT NULL DEFAULT 0,
	end_line        INTEGER NOT NULL DEFAULT 0,
	start_byte      INTEGER NOT NULL DEFAULT 0,
	end_byte        INTEGER NOT NULL DEFAULT 0,
	language        TEXT NOT NULL DEFAULT '',
	signature       TEXT NOT NULL DEFAULT '',
	docstring       TEXT NOT NULL DEFAULT '',
	is_test         INTEGER NOT NULL DEFAULT 0,
	is_entrypoint   INTEGER NOT NULL DEFAULT 0,
	entrypoint_kind TEXT NOT NULL DEFAULT '',
	visibility      TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS edges (
	source_id       TEXT NOT NULL,
	target_id       TEXT NOT NULL,
	kind            TEXT NOT NULL,
	line            INTEGER NOT NULL DEFAULT 0,
	confidence      TEXT NOT NULL,
	candidate_count INTEGER NOT NULL DEFAULT 1,
	PRIMARY KEY (source_id, target_id, kind, line),
	FOREIGN KEY (source_id) REFERENCES nodes(id) ON DELETE CASCADE,
	FOREIGN KEY (target_id) REFERENCES nodes(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS files (
	path         TEXT PRIMARY KEY,
	content_hash TEXT NOT NULL,
	indexed_at   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_nodes_file  ON nodes(file_path);
CREATE INDEX IF NOT EXISTS idx_nodes_qname ON nodes(qualified_name);
CREATE INDEX IF NOT EXISTS idx_nodes_name  ON nodes(name);
CREATE INDEX IF NOT EXISTS idx_edges_src   ON edges(source_id);
CREATE INDEX IF NOT EXISTS idx_edges_tgt   ON edges(target_id);

CREATE VIRTUAL TABLE IF NOT EXISTS symbols_fts USING fts5(
	node_id UNINDEXED,
	tokens,
	docstring
);
`

// applySchema creates every table and index. Safe to run on an existing database.
func (s *Store) applySchema() error {
	if _, err := s.conn.Exec(schemaDDL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return s.SetMeta("schema_version", fmt.Sprint(SchemaVersion))
}
