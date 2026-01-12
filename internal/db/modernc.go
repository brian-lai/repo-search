package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // Register modernc SQLite driver
)

// ModerncDB wraps a *sql.DB using the modernc.org/sqlite driver.
// This is a pure Go implementation with no CGO dependencies.
// Note: Does not support loading native extensions like sqlite-vec.
type ModerncDB struct {
	db   *sql.DB
	path string
}

// Verify interface compliance at compile time.
var _ DB = (*ModerncDB)(nil)

// OpenModernc opens a SQLite database using the modernc.org/sqlite driver.
func OpenModernc(cfg Config) (*ModerncDB, error) {
	// Ensure parent directory exists
	if cfg.Path != ":memory:" {
		dir := filepath.Dir(cfg.Path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating db directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for better concurrent performance
	if cfg.EnableWAL && cfg.Path != ":memory:" {
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting WAL mode: %w", err)
		}
	}

	return &ModerncDB{db: db, path: cfg.Path}, nil
}

// Query executes a query that returns rows.
func (m *ModerncDB) Query(query string, args ...any) (Rows, error) {
	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRows{rows}, nil
}

// QueryContext executes a query with context.
func (m *ModerncDB) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRows{rows}, nil
}

// QueryRow executes a query expected to return at most one row.
func (m *ModerncDB) QueryRow(query string, args ...any) Row {
	return &sqlRow{m.db.QueryRow(query, args...)}
}

// QueryRowContext executes a query with context.
func (m *ModerncDB) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	return &sqlRow{m.db.QueryRowContext(ctx, query, args...)}
}

// Exec executes a query without returning rows.
func (m *ModerncDB) Exec(query string, args ...any) (Result, error) {
	return m.db.Exec(query, args...)
}

// ExecContext executes a query with context.
func (m *ModerncDB) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	return m.db.ExecContext(ctx, query, args...)
}

// Begin starts a transaction.
func (m *ModerncDB) Begin() (Tx, error) {
	tx, err := m.db.Begin()
	if err != nil {
		return nil, err
	}
	return &sqlTx{tx}, nil
}

// BeginTx starts a transaction with context and options.
func (m *ModerncDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) {
	tx, err := m.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &sqlTx{tx}, nil
}

// Close closes the database connection.
func (m *ModerncDB) Close() error {
	return m.db.Close()
}

// Ping verifies a connection to the database is still alive.
func (m *ModerncDB) Ping() error {
	return m.db.Ping()
}

// PingContext verifies with context.
func (m *ModerncDB) PingContext(ctx context.Context) error {
	return m.db.PingContext(ctx)
}

// Unwrap returns the underlying *sql.DB for compatibility with existing code.
// Use sparingly - prefer using the DB interface methods.
func (m *ModerncDB) Unwrap() *sql.DB {
	return m.db
}

// --- sql.Rows wrapper ---

type sqlRows struct {
	*sql.Rows
}

func (r *sqlRows) Next() bool {
	return r.Rows.Next()
}

func (r *sqlRows) Scan(dest ...any) error {
	return r.Rows.Scan(dest...)
}

func (r *sqlRows) Close() error {
	return r.Rows.Close()
}

func (r *sqlRows) Err() error {
	return r.Rows.Err()
}

func (r *sqlRows) Columns() ([]string, error) {
	return r.Rows.Columns()
}

// --- sql.Row wrapper ---

type sqlRow struct {
	*sql.Row
}

func (r *sqlRow) Scan(dest ...any) error {
	return r.Row.Scan(dest...)
}

func (r *sqlRow) Err() error {
	return r.Row.Err()
}

// --- sql.Tx wrapper ---

type sqlTx struct {
	*sql.Tx
}

func (t *sqlTx) Query(query string, args ...any) (Rows, error) {
	rows, err := t.Tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRows{rows}, nil
}

func (t *sqlTx) QueryRow(query string, args ...any) Row {
	return &sqlRow{t.Tx.QueryRow(query, args...)}
}

func (t *sqlTx) Exec(query string, args ...any) (Result, error) {
	return t.Tx.Exec(query, args...)
}

func (t *sqlTx) Prepare(query string) (Stmt, error) {
	stmt, err := t.Tx.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &sqlStmt{stmt}, nil
}

func (t *sqlTx) Commit() error {
	return t.Tx.Commit()
}

func (t *sqlTx) Rollback() error {
	return t.Tx.Rollback()
}

// --- sql.Stmt wrapper ---

type sqlStmt struct {
	*sql.Stmt
}

func (s *sqlStmt) Exec(args ...any) (Result, error) {
	return s.Stmt.Exec(args...)
}

func (s *sqlStmt) Query(args ...any) (Rows, error) {
	rows, err := s.Stmt.Query(args...)
	if err != nil {
		return nil, err
	}
	return &sqlRows{rows}, nil
}

func (s *sqlStmt) QueryRow(args ...any) Row {
	return &sqlRow{s.Stmt.QueryRow(args...)}
}

func (s *sqlStmt) Close() error {
	return s.Stmt.Close()
}
