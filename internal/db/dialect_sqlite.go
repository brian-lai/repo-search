package db

import (
	"fmt"
	"strings"
)

// SQLiteDialect implements the Dialect interface for SQLite.
type SQLiteDialect struct{}

// Verify interface compliance at compile time.
var _ Dialect = (*SQLiteDialect)(nil)

func (d *SQLiteDialect) Name() string {
	return "sqlite"
}

func (d *SQLiteDialect) Placeholder(index int) string {
	return "?"
}

func (d *SQLiteDialect) Placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	placeholders := make([]string, n)
	for i := range placeholders {
		placeholders[i] = "?"
	}
	return strings.Join(placeholders, ", ")
}

func (d *SQLiteDialect) AutoIncrementPK() string {
	return "INTEGER PRIMARY KEY AUTOINCREMENT"
}

func (d *SQLiteDialect) BlobType() string {
	return "BLOB"
}

func (d *SQLiteDialect) TextType() string {
	return "TEXT"
}

func (d *SQLiteDialect) IntegerType() string {
	return "INTEGER"
}

func (d *SQLiteDialect) TimestampType() string {
	// SQLite stores timestamps as INTEGER (unix epoch)
	return "INTEGER"
}

func (d *SQLiteDialect) UpsertSQL(table string, columns []string, conflictColumns []string, updateColumns []string) string {
	// SQLite uses INSERT OR REPLACE for simple upserts
	// For more control, SQLite 3.24+ supports ON CONFLICT
	cols := strings.Join(columns, ", ")
	placeholders := d.Placeholders(len(columns))

	// Use INSERT OR REPLACE for simplicity (replaces entire row on conflict)
	return fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) VALUES (%s)", table, cols, placeholders)
}

func (d *SQLiteDialect) CreateTableSQL(table string, columns []ColumnDef) string {
	// Count primary key columns first to determine if we need a composite PK
	var primaryKeyCount int
	for _, col := range columns {
		if col.PrimaryKey && col.Type != ColTypeAutoIncrement {
			primaryKeyCount++
		}
	}

	var colDefs []string
	var primaryKeys []string
	useCompositePK := primaryKeyCount > 1 // Only use table-level PK constraint for composite keys

	for _, col := range columns {
		def := d.columnDefSQL(col, useCompositePK)
		colDefs = append(colDefs, def)

		// Collect composite primary key columns (only when we have multiple PKs)
		if useCompositePK && col.PrimaryKey && col.Type != ColTypeAutoIncrement {
			primaryKeys = append(primaryKeys, col.Name)
		}
	}

	sql := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n    %s",
		table, strings.Join(colDefs, ",\n    "))

	// Add composite primary key if needed (not for AUTOINCREMENT columns)
	if len(primaryKeys) > 1 {
		sql += fmt.Sprintf(",\n    PRIMARY KEY (%s)", strings.Join(primaryKeys, ", "))
	}

	sql += "\n)"
	return sql
}

func (d *SQLiteDialect) columnDefSQL(col ColumnDef, useCompositePK bool) string {
	var parts []string

	parts = append(parts, col.Name)

	// Handle auto-increment specially
	if col.Type == ColTypeAutoIncrement {
		parts = append(parts, d.AutoIncrementPK())
		return strings.Join(parts, " ")
	}

	// Map abstract type to SQLite type
	parts = append(parts, d.mapColumnType(col.Type))

	// Add inline PRIMARY KEY only if we're not using a composite PK constraint
	if col.PrimaryKey && !useCompositePK {
		parts = append(parts, "PRIMARY KEY")
	}

	if !col.Nullable && !col.PrimaryKey {
		parts = append(parts, "NOT NULL")
	}

	if col.Unique {
		parts = append(parts, "UNIQUE")
	}

	if col.Default != "" {
		parts = append(parts, "DEFAULT", col.Default)
	}

	return strings.Join(parts, " ")
}

func (d *SQLiteDialect) mapColumnType(ct ColumnType) string {
	switch ct {
	case ColTypeInteger:
		return "INTEGER"
	case ColTypeText:
		return "TEXT"
	case ColTypeBlob:
		return "BLOB"
	case ColTypeTimestamp:
		return "INTEGER" // Unix timestamp
	case ColTypeReal:
		return "REAL"
	case ColTypeBoolean:
		return "INTEGER" // SQLite uses 0/1 for booleans
	case ColTypeVector:
		return "TEXT" // Store vectors as JSON in SQLite
	default:
		return "TEXT"
	}
}

func (d *SQLiteDialect) CreateIndexSQL(table, indexName string, columns []string, unique bool) string {
	uniqueStr := ""
	if unique {
		uniqueStr = "UNIQUE "
	}
	return fmt.Sprintf("CREATE %sINDEX IF NOT EXISTS %s ON %s (%s)",
		uniqueStr, indexName, table, strings.Join(columns, ", "))
}

func (d *SQLiteDialect) InitStatements() []string {
	return []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	}
}

func (d *SQLiteDialect) SupportsReturning() bool {
	// SQLite 3.35+ supports RETURNING, but we'll be conservative
	return false
}

func (d *SQLiteDialect) QuoteIdentifier(name string) string {
	// SQLite accepts double quotes or square brackets
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
