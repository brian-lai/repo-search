package symbols

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"codetect/internal/db"

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

// initSchemaWithAdapter initializes the database schema using the adapter and dialect.
// This supports multiple database backends (SQLite, PostgreSQL) by using dialect-aware DDL.
func initSchemaWithAdapter(adapter db.DB, dialect db.Dialect) error {
	// Run dialect-specific initialization statements (e.g., WAL mode for SQLite, pgvector extension for Postgres)
	for _, stmt := range dialect.InitStatements() {
		if _, err := adapter.Exec(stmt); err != nil {
			return fmt.Errorf("init statement %q: %w", stmt, err)
		}
	}

	// Create schema_version table
	schemaVersionColumns := []db.ColumnDef{
		{Name: "version", Type: db.ColTypeInteger, Nullable: false},
	}
	if _, err := adapter.Exec(dialect.CreateTableSQL("schema_version", schemaVersionColumns)); err != nil {
		return fmt.Errorf("creating schema_version table: %w", err)
	}

	// Check current schema version
	var version int
	err := adapter.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	needsSchema := err != nil // Either no rows or table was just created

	if needsSchema {
		// Create symbols table
		symbolColumns := []db.ColumnDef{
			{Name: "id", Type: db.ColTypeAutoIncrement},
			{Name: "name", Type: db.ColTypeText, Nullable: false},
			{Name: "kind", Type: db.ColTypeText, Nullable: false},
			{Name: "path", Type: db.ColTypeText, Nullable: false},
			{Name: "line", Type: db.ColTypeInteger, Nullable: false},
			{Name: "language", Type: db.ColTypeText, Nullable: true},
			{Name: "pattern", Type: db.ColTypeText, Nullable: true},
			{Name: "scope", Type: db.ColTypeText, Nullable: true},
			{Name: "signature", Type: db.ColTypeText, Nullable: true},
		}
		if _, err := adapter.Exec(dialect.CreateTableSQL("symbols", symbolColumns)); err != nil {
			return fmt.Errorf("creating symbols table: %w", err)
		}

		// Create unique constraint on symbols - use a unique index
		uniqueIdxSQL := dialect.CreateIndexSQL("symbols", "idx_symbols_unique", []string{"name", "path", "line"}, true)
		if _, err := adapter.Exec(uniqueIdxSQL); err != nil {
			// Ignore error if index already exists (some databases don't support IF NOT EXISTS for unique constraints)
			// The unique index is created by CreateIndexSQL with unique=true
		}

		// Create indexes on symbols table
		if _, err := adapter.Exec(dialect.CreateIndexSQL("symbols", "idx_symbols_name", []string{"name"}, false)); err != nil {
			return fmt.Errorf("creating name index: %w", err)
		}
		if _, err := adapter.Exec(dialect.CreateIndexSQL("symbols", "idx_symbols_path", []string{"path"}, false)); err != nil {
			return fmt.Errorf("creating path index: %w", err)
		}
		if _, err := adapter.Exec(dialect.CreateIndexSQL("symbols", "idx_symbols_kind", []string{"kind"}, false)); err != nil {
			return fmt.Errorf("creating kind index: %w", err)
		}

		// Create files table
		fileColumns := []db.ColumnDef{
			{Name: "path", Type: db.ColTypeText, Nullable: false, PrimaryKey: true},
			{Name: "mtime", Type: db.ColTypeInteger, Nullable: false},
			{Name: "size", Type: db.ColTypeInteger, Nullable: false},
			{Name: "indexed_at", Type: db.ColTypeInteger, Nullable: false},
		}
		if _, err := adapter.Exec(dialect.CreateTableSQL("files", fileColumns)); err != nil {
			return fmt.Errorf("creating files table: %w", err)
		}

		// Insert schema version
		insertVersionSQL := fmt.Sprintf("INSERT INTO schema_version (version) VALUES (%s)", dialect.Placeholder(1))
		if _, err := adapter.Exec(insertVersionSQL, schemaVersion); err != nil {
			return fmt.Errorf("setting schema version: %w", err)
		}
	} else if version < schemaVersion {
		// Future: add migration logic here
		updateVersionSQL := fmt.Sprintf("UPDATE schema_version SET version = %s", dialect.Placeholder(1))
		if _, err := adapter.Exec(updateVersionSQL, schemaVersion); err != nil {
			return fmt.Errorf("updating schema version: %w", err)
		}
	}

	return nil
}
