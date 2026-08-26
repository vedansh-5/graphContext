package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store owns the SQLite connection holding one repository's graph.
type Store struct {
	conn *sql.DB
	path string
}

// CachePathFor returns the on-disk database location for a repository root.
// The database never lives inside the analyzed repository.
func CachePathFor(repoRoot string) (string, error) {
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate user cache dir: %w", err)
	}
	sum := sha256.Sum256([]byte(abs))
	key := hex.EncodeToString(sum[:])[:16]
	return filepath.Join(base, "graphcontext", key, "graph.db"), nil
}

// Open opens (creating if needed) the database at dbPath and applies the schema.
//
// The version check runs BEFORE the schema is applied: applySchema writes the
// current version, so reading it afterwards would always report a match. On a
// mismatch the file is deleted and rebuilt, which is always safe because the
// graph is a derived artifact.
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	s, err := connect(dbPath)
	if err != nil {
		return nil, err
	}

	v, err := s.storedSchemaVersion()
	if err != nil {
		s.Close()
		return nil, err
	}
	if v != "" && v != fmt.Sprint(SchemaVersion) {
		s.Close()
		if err := removeDBFiles(dbPath); err != nil {
			return nil, err
		}
		if s, err = connect(dbPath); err != nil {
			return nil, err
		}
	}

	if err := s.applySchema(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// OpenForRepo opens the cache-directory database belonging to a repository root.
func OpenForRepo(repoRoot string) (*Store, error) {
	p, err := CachePathFor(repoRoot)
	if err != nil {
		return nil, err
	}
	return Open(p)
}

func connect(dbPath string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)", dbPath)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// modernc's driver serializes poorly across concurrent writers; a single
	// connection keeps writes ordered without a hand-rolled queue.
	conn.SetMaxOpenConns(1)
	return &Store{conn: conn, path: dbPath}, nil
}

// storedSchemaVersion reads meta.schema_version, returning "" when the database
// is brand new (the meta table does not exist yet).
func (s *Store) storedSchemaVersion() (string, error) {
	var name string
	err := s.conn.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='meta'`).Scan(&name)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("probe meta table: %w", err)
	}
	return s.Meta("schema_version")
}

// removeDBFiles deletes the database and its WAL sidecars.
func removeDBFiles(dbPath string) error {
	for _, p := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale db %s: %w", p, err)
		}
	}
	return nil
}

// Path returns the database file location.
func (s *Store) Path() string { return s.path }

// Close releases the connection.
func (s *Store) Close() error { return s.conn.Close() }

// Meta reads one metadata value. A missing key returns "" with no error.
func (s *Store) Meta(key string) (string, error) {
	var v string
	err := s.conn.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read meta %q: %w", key, err)
	}
	return v, nil
}

// SetMeta writes one metadata value.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.conn.Exec(
		`INSERT INTO meta(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("write meta %q: %w", key, err)
	}
	return nil
}
