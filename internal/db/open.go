package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// Open opens a database connection using the configuration.
// Supports SQLite (default), PostgreSQL, and ClickHouse.
func Open(cfg Config) (DB, error) {
	// Route based on database type
	switch cfg.Type {
	case DatabasePostgres:
		return openPostgres(cfg)

	case DatabaseClickHouse:
		return openClickHouse(cfg)

	case DatabaseSQLite, "": // Default to SQLite
		return openSQLite(cfg)

	default:
		return nil, fmt.Errorf("unknown database type: %s", cfg.Type)
	}
}

// openSQLite opens a SQLite database using the specified driver.
func openSQLite(cfg Config) (DB, error) {
	switch cfg.Driver {
	case DriverModernc, "": // Default to modernc
		return OpenModernc(cfg)

	case DriverNcruces:
		// TODO: Implement ncruces driver with sqlite-vec support
		// This will enable native vector search via vec0 virtual tables.
		// See: https://github.com/ncruces/go-sqlite3
		// See: https://github.com/asg017/sqlite-vec-go-bindings
		return nil, fmt.Errorf("ncruces driver not yet implemented (requires sqlite-vec integration)")

	case DriverMattn:
		// TODO: Implement mattn driver with CGO
		// This requires CGO and a C compiler but provides full extension support.
		// See: https://github.com/mattn/go-sqlite3
		return nil, fmt.Errorf("mattn driver not yet implemented (requires CGO)")

	default:
		return nil, fmt.Errorf("unknown SQLite driver: %s", cfg.Driver)
	}
}

// openPostgres opens a PostgreSQL database connection.
// Requires DSN in config.
func openPostgres(cfg Config) (DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("postgres requires DSN in config")
	}

	// Open connection using lib/pq driver
	sqlDB, err := sql.Open("postgres", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	// Configure connection pool settings
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)
	}

	// Test the connection
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	return WrapSQL(sqlDB), nil
}

// openClickHouse opens a ClickHouse database connection.
// Requires DSN in config.
func openClickHouse(cfg Config) (DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("clickhouse requires DSN in config")
	}

	// TODO: Implement ClickHouse driver
	// Example:
	//   import _ "github.com/ClickHouse/clickhouse-go/v2"
	//   db, err := sql.Open("clickhouse", cfg.DSN)
	//   return WrapSQL(db), nil
	return nil, fmt.Errorf("clickhouse driver not yet implemented (requires clickhouse-go)")
}

// MustOpen opens a database connection and panics on error.
// Useful for testing and simple scripts.
func MustOpen(cfg Config) DB {
	db, err := Open(cfg)
	if err != nil {
		panic(fmt.Sprintf("failed to open database: %v", err))
	}
	return db
}

// OpenExtended opens a database with vector search support if available.
// Falls back to standard DB if the driver doesn't support extensions.
func OpenExtended(cfg Config) (DB, bool, error) {
	db, err := Open(cfg)
	if err != nil {
		return nil, false, err
	}

	// Check if the DB supports vector operations
	if ext, ok := db.(ExtendedDB); ok && ext.VectorSearchAvailable() {
		return db, true, nil
	}

	return db, false, nil
}
