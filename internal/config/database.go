package config

import (
	"fmt"
	"os"
	"strings"

	"codetect/internal/db"
)

// DatabaseConfig holds database configuration for the MCP server.
type DatabaseConfig struct {
	// Type is the database type (sqlite, postgres)
	Type db.DatabaseType

	// Path is the SQLite database file path (for SQLite)
	Path string

	// DSN is the connection string (for PostgreSQL)
	DSN string

	// VectorDimensions is the embedding vector size
	VectorDimensions int
}

// LoadDatabaseConfigFromEnv loads database configuration from environment variables.
// Supports the following variables:
//   - CODETECT_DB_TYPE: Database type ("sqlite" or "postgres")
//   - CODETECT_DB_DSN: Connection string for PostgreSQL
//   - CODETECT_DB_PATH: Database file path for SQLite
//   - CODETECT_VECTOR_DIMENSIONS: Vector dimensions (default: 768)
//
// If no environment variables are set, defaults to SQLite with standard path.
func LoadDatabaseConfigFromEnv() DatabaseConfig {
	cfg := DatabaseConfig{
		Type:             db.DatabaseSQLite, // Default to SQLite
		VectorDimensions: 768,                // Default for nomic-embed-text
	}

	// Check for explicit database type
	if dbType := os.Getenv("CODETECT_DB_TYPE"); dbType != "" {
		switch strings.ToLower(dbType) {
		case "postgres", "postgresql":
			cfg.Type = db.DatabasePostgres
		case "sqlite", "sqlite3":
			cfg.Type = db.DatabaseSQLite
		default:
			fmt.Fprintf(os.Stderr, "Warning: Unknown database type %q, using SQLite\n", dbType)
			cfg.Type = db.DatabaseSQLite
		}
	}

	// Load DSN (for PostgreSQL)
	if dsn := os.Getenv("CODETECT_DB_DSN"); dsn != "" {
		cfg.DSN = dsn

		// Auto-detect database type from DSN if not explicitly set
		if cfg.Type == db.DatabaseSQLite && strings.HasPrefix(dsn, "postgres://") {
			cfg.Type = db.DatabasePostgres
		}
	}

	// Load path (for SQLite)
	if path := os.Getenv("CODETECT_DB_PATH"); path != "" {
		cfg.Path = path
	}

	// Load vector dimensions
	if dims := os.Getenv("CODETECT_VECTOR_DIMENSIONS"); dims != "" {
		var d int
		if _, err := fmt.Sscanf(dims, "%d", &d); err == nil && d > 0 {
			cfg.VectorDimensions = d
		}
	}

	return cfg
}

// ToDBConfig converts DatabaseConfig to db.Config for opening a database.
func (c DatabaseConfig) ToDBConfig() db.Config {
	switch c.Type {
	case db.DatabasePostgres:
		if c.DSN == "" {
			// Provide a helpful error via the config
			return db.Config{
				Type: db.DatabaseSQLite,
				Path: "", // Will cause error on open
			}
		}
		cfg := db.PostgresConfig(c.DSN)
		cfg.VectorDimensions = c.VectorDimensions
		return cfg

	default: // SQLite
		path := c.Path
		if path == "" {
			// Default path if not specified
			path = ".codetect/symbols.db"
		}
		return db.DefaultConfig(path)
	}
}

// String returns a human-readable description of the database configuration.
func (c DatabaseConfig) String() string {
	switch c.Type {
	case db.DatabasePostgres:
		// Mask password in DSN for display
		dsn := c.DSN
		if strings.Contains(dsn, "@") {
			parts := strings.Split(dsn, "@")
			if len(parts) == 2 {
				userPart := strings.Split(parts[0], ":")
				if len(userPart) >= 2 {
					dsn = userPart[0] + ":***@" + parts[1]
				}
			}
		}
		return fmt.Sprintf("PostgreSQL (%s)", dsn)
	default:
		return fmt.Sprintf("SQLite (%s)", c.Path)
	}
}
