package symbols

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schemaVersion = 1

const schema = `
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS symbols (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    path TEXT NOT NULL,
    line INTEGER NOT NULL,
    language TEXT,
    pattern TEXT,
    scope TEXT,
    signature TEXT,
    UNIQUE(name, path, line)
);

CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);
CREATE INDEX IF NOT EXISTS idx_symbols_path ON symbols(path);
CREATE INDEX IF NOT EXISTS idx_symbols_kind ON symbols(kind);

CREATE TABLE IF NOT EXISTS files (
    path TEXT PRIMARY KEY,
    mtime INTEGER NOT NULL,
    size INTEGER NOT NULL,
    indexed_at INTEGER NOT NULL
);
`

// OpenDB opens or creates the symbol database at the given path
func OpenDB(dbPath string) (*sql.DB, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for better concurrent performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}

	// Initialize schema if needed
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}

	return db, nil
}

func initSchema(db *sql.DB) error {
	// Check current schema version
	var version int
	err := db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if err == sql.ErrNoRows {
		// Fresh database, create schema
		if _, err := db.Exec(schema); err != nil {
			return fmt.Errorf("creating schema: %w", err)
		}
		if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion); err != nil {
			return fmt.Errorf("setting schema version: %w", err)
		}
		return nil
	}
	if err != nil {
		// Table doesn't exist, create schema
		if _, err := db.Exec(schema); err != nil {
			return fmt.Errorf("creating schema: %w", err)
		}
		if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion); err != nil {
			return fmt.Errorf("setting schema version: %w", err)
		}
		return nil
	}

	// Version exists, check for migrations
	if version < schemaVersion {
		// Future: add migration logic here
		if _, err := db.Exec("UPDATE schema_version SET version = ?", schemaVersion); err != nil {
			return fmt.Errorf("updating schema version: %w", err)
		}
	}

	return nil
}

// ClearSymbols removes all symbols for a given file path
func ClearSymbols(db *sql.DB, path string) error {
	_, err := db.Exec("DELETE FROM symbols WHERE path = ?", path)
	return err
}

// ClearAllSymbols removes all symbols from the database
func ClearAllSymbols(db *sql.DB) error {
	_, err := db.Exec("DELETE FROM symbols")
	return err
}
