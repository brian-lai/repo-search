package db

import (
	"fmt"
	"strings"
)

// PostgresDialect implements the Dialect interface for PostgreSQL.
// This is a stub implementation - actual PostgreSQL support requires
// additional driver integration (e.g., lib/pq or pgx).
type PostgresDialect struct{}

// Verify interface compliance at compile time.
var _ Dialect = (*PostgresDialect)(nil)

func (d *PostgresDialect) Name() string {
	return "postgres"
}

func (d *PostgresDialect) Placeholder(index int) string {
	return fmt.Sprintf("$%d", index)
}

func (d *PostgresDialect) Placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	placeholders := make([]string, n)
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	return strings.Join(placeholders, ", ")
}

func (d *PostgresDialect) AutoIncrementPK() string {
	return "SERIAL PRIMARY KEY"
}

func (d *PostgresDialect) BlobType() string {
	return "BYTEA"
}

func (d *PostgresDialect) TextType() string {
	return "TEXT"
}

func (d *PostgresDialect) IntegerType() string {
	return "INTEGER"
}

func (d *PostgresDialect) TimestampType() string {
	return "TIMESTAMPTZ"
}

func (d *PostgresDialect) UpsertSQL(table string, columns []string, conflictColumns []string, updateColumns []string) string {
	cols := strings.Join(columns, ", ")
	placeholders := d.Placeholders(len(columns))

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, cols, placeholders)

	if len(conflictColumns) > 0 {
		sql += fmt.Sprintf(" ON CONFLICT (%s)", strings.Join(conflictColumns, ", "))

		if len(updateColumns) > 0 {
			var updates []string
			for _, col := range updateColumns {
				updates = append(updates, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
			}
			sql += fmt.Sprintf(" DO UPDATE SET %s", strings.Join(updates, ", "))
		} else {
			// Update all non-conflict columns
			var updates []string
			for _, col := range columns {
				isConflict := false
				for _, cc := range conflictColumns {
					if col == cc {
						isConflict = true
						break
					}
				}
				if !isConflict {
					updates = append(updates, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
				}
			}
			if len(updates) > 0 {
				sql += fmt.Sprintf(" DO UPDATE SET %s", strings.Join(updates, ", "))
			} else {
				sql += " DO NOTHING"
			}
		}
	}

	return sql
}

func (d *PostgresDialect) CreateTableSQL(table string, columns []ColumnDef) string {
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

func (d *PostgresDialect) columnDefSQL(col ColumnDef, useCompositePK bool) string {
	var parts []string

	parts = append(parts, col.Name)

	if col.Type == ColTypeAutoIncrement {
		parts = append(parts, d.AutoIncrementPK())
		return strings.Join(parts, " ")
	}

	// Handle vector type with dimensions
	if col.Type == ColTypeVector {
		if col.VectorDimension > 0 {
			parts = append(parts, fmt.Sprintf("vector(%d)", col.VectorDimension))
		} else {
			parts = append(parts, "vector") // Dynamic dimensions
		}
	} else {
		parts = append(parts, d.mapColumnType(col.Type))
	}

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

func (d *PostgresDialect) mapColumnType(ct ColumnType) string {
	switch ct {
	case ColTypeInteger:
		return "INTEGER"
	case ColTypeText:
		return "TEXT"
	case ColTypeBlob:
		return "BYTEA"
	case ColTypeTimestamp:
		return "TIMESTAMPTZ"
	case ColTypeReal:
		return "DOUBLE PRECISION"
	case ColTypeBoolean:
		return "BOOLEAN"
	default:
		return "TEXT"
	}
}

func (d *PostgresDialect) CreateIndexSQL(table, indexName string, columns []string, unique bool) string {
	uniqueStr := ""
	if unique {
		uniqueStr = "UNIQUE "
	}
	return fmt.Sprintf("CREATE %sINDEX IF NOT EXISTS %s ON %s (%s)",
		uniqueStr, indexName, table, strings.Join(columns, ", "))
}

func (d *PostgresDialect) InitStatements() []string {
	// Enable pgvector extension for vector similarity search
	return []string{
		"CREATE EXTENSION IF NOT EXISTS vector",
	}
}

func (d *PostgresDialect) SupportsReturning() bool {
	return true
}

func (d *PostgresDialect) QuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
