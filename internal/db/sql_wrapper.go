package db

import (
	"context"
	"database/sql"
)

// WrapSQL wraps a *sql.DB to implement the db.DB interface.
// This allows existing code using *sql.DB to work with the adapter interface.
func WrapSQL(sqlDB *sql.DB) DB {
	return &SQLWrapper{sqlDB}
}

// SQLWrapper wraps *sql.DB to implement db.DB interface.
type SQLWrapper struct {
	*sql.DB
}

// Verify interface compliance at compile time.
var _ DB = (*SQLWrapper)(nil)

func (w *SQLWrapper) Query(query string, args ...any) (Rows, error) {
	rows, err := w.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRowsWrap{rows}, nil
}

func (w *SQLWrapper) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	rows, err := w.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRowsWrap{rows}, nil
}

func (w *SQLWrapper) QueryRow(query string, args ...any) Row {
	return &sqlRowWrap{w.DB.QueryRow(query, args...)}
}

func (w *SQLWrapper) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	return &sqlRowWrap{w.DB.QueryRowContext(ctx, query, args...)}
}

func (w *SQLWrapper) Exec(query string, args ...any) (Result, error) {
	return w.DB.Exec(query, args...)
}

func (w *SQLWrapper) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	return w.DB.ExecContext(ctx, query, args...)
}

func (w *SQLWrapper) Begin() (Tx, error) {
	tx, err := w.DB.Begin()
	if err != nil {
		return nil, err
	}
	return &sqlTxWrap{tx}, nil
}

func (w *SQLWrapper) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) {
	tx, err := w.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &sqlTxWrap{tx}, nil
}

func (w *SQLWrapper) Ping() error {
	return w.DB.Ping()
}

func (w *SQLWrapper) PingContext(ctx context.Context) error {
	return w.DB.PingContext(ctx)
}

// Unwrap returns the underlying *sql.DB.
func (w *SQLWrapper) Unwrap() *sql.DB {
	return w.DB
}

// --- sql.Rows wrapper ---

type sqlRowsWrap struct {
	*sql.Rows
}

func (r *sqlRowsWrap) Columns() ([]string, error) {
	return r.Rows.Columns()
}

// --- sql.Row wrapper ---

type sqlRowWrap struct {
	*sql.Row
}

func (r *sqlRowWrap) Err() error {
	return r.Row.Err()
}

// --- sql.Tx wrapper ---

type sqlTxWrap struct {
	*sql.Tx
}

func (t *sqlTxWrap) Query(query string, args ...any) (Rows, error) {
	rows, err := t.Tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRowsWrap{rows}, nil
}

func (t *sqlTxWrap) QueryRow(query string, args ...any) Row {
	return &sqlRowWrap{t.Tx.QueryRow(query, args...)}
}

func (t *sqlTxWrap) Exec(query string, args ...any) (Result, error) {
	return t.Tx.Exec(query, args...)
}

func (t *sqlTxWrap) Prepare(query string) (Stmt, error) {
	stmt, err := t.Tx.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &sqlStmtWrap{stmt}, nil
}

// --- sql.Stmt wrapper ---

type sqlStmtWrap struct {
	*sql.Stmt
}

func (s *sqlStmtWrap) Query(args ...any) (Rows, error) {
	rows, err := s.Stmt.Query(args...)
	if err != nil {
		return nil, err
	}
	return &sqlRowsWrap{rows}, nil
}

func (s *sqlStmtWrap) QueryRow(args ...any) Row {
	return &sqlRowWrap{s.Stmt.QueryRow(args...)}
}

func (s *sqlStmtWrap) Exec(args ...any) (Result, error) {
	return s.Stmt.Exec(args...)
}
