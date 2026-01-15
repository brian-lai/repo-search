package db

import (
	"fmt"
	"strings"
)

// ClickHouseDialect implements the Dialect interface for ClickHouse.
// This is a stub implementation - actual ClickHouse support requires
// additional driver integration (e.g., clickhouse-go).
//
// Note: ClickHouse has significantly different semantics than traditional
// SQL databases (eventual consistency, no transactions in the traditional sense,
// different update/delete behavior). This dialect provides basic compatibility
// but full ClickHouse support may require architectural changes.
type ClickHouseDialect struct{}

// Verify interface compliance at compile time.
var _ Dialect = (*ClickHouseDialect)(nil)

func (d *ClickHouseDialect) Name() string {
	return "clickhouse"
}

func (d *ClickHouseDialect) Placeholder(index int) string {
	// ClickHouse uses ? or $1 depending on driver settings
	// Using ? for compatibility with clickhouse-go
	return "?"
}

func (d *ClickHouseDialect) Placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	placeholders := make([]string, n)
	for i := range placeholders {
		placeholders[i] = "?"
	}
	return strings.Join(placeholders, ", ")
}

func (d *ClickHouseDialect) AutoIncrementPK() string {
	// ClickHouse doesn't have auto-increment in the traditional sense
	// Typically uses UInt64 with generateUUIDv4() or similar
	return "UInt64"
}

func (d *ClickHouseDialect) BlobType() string {
	// ClickHouse uses String for binary data
	return "String"
}

func (d *ClickHouseDialect) TextType() string {
	return "String"
}

func (d *ClickHouseDialect) IntegerType() string {
	return "Int64"
}

func (d *ClickHouseDialect) TimestampType() string {
	return "DateTime64(3)" // Millisecond precision
}

func (d *ClickHouseDialect) UpsertSQL(table string, columns []string, conflictColumns []string, updateColumns []string) string {
	// ClickHouse doesn't support traditional upserts
	// Instead, use ReplacingMergeTree engine and just INSERT
	// Duplicates are handled at merge time based on ORDER BY columns
	cols := strings.Join(columns, ", ")
	placeholders := d.Placeholders(len(columns))

	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, cols, placeholders)
}

func (d *ClickHouseDialect) CreateTableSQL(table string, columns []ColumnDef) string {
	var colDefs []string
	var orderByColumns []string

	for _, col := range columns {
		def := d.columnDefSQL(col, false) // ClickHouse doesn't use inline PRIMARY KEY
		colDefs = append(colDefs, def)

		// Use primary key columns for ORDER BY (required in ClickHouse)
		if col.PrimaryKey {
			orderByColumns = append(orderByColumns, col.Name)
		}
	}

	sql := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n    %s\n)",
		table, strings.Join(colDefs, ",\n    "))

	// ClickHouse requires ENGINE specification
	// Using ReplacingMergeTree for upsert-like behavior
	sql += " ENGINE = ReplacingMergeTree()"

	if len(orderByColumns) > 0 {
		sql += fmt.Sprintf(" ORDER BY (%s)", strings.Join(orderByColumns, ", "))
	} else {
		// Default to tuple() if no primary key defined
		sql += " ORDER BY tuple()"
	}

	return sql
}

func (d *ClickHouseDialect) columnDefSQL(col ColumnDef, _ bool) string {
	// Note: ClickHouse doesn't use PRIMARY KEY constraints in column definitions
	// Primary keys are handled via ORDER BY clause
	var parts []string

	parts = append(parts, col.Name)
	parts = append(parts, d.mapColumnType(col.Type))

	// ClickHouse uses Nullable() wrapper for nullable columns
	// But for simplicity, we're not adding it by default

	if col.Default != "" {
		parts = append(parts, "DEFAULT", col.Default)
	}

	return strings.Join(parts, " ")
}

func (d *ClickHouseDialect) mapColumnType(ct ColumnType) string {
	switch ct {
	case ColTypeInteger, ColTypeAutoIncrement:
		return "Int64"
	case ColTypeText:
		return "String"
	case ColTypeBlob:
		return "String"
	case ColTypeTimestamp:
		return "DateTime64(3)"
	case ColTypeReal:
		return "Float64"
	case ColTypeBoolean:
		return "UInt8" // ClickHouse uses 0/1 for booleans
	default:
		return "String"
	}
}

func (d *ClickHouseDialect) CreateIndexSQL(table, indexName string, columns []string, unique bool) string {
	// ClickHouse has different indexing model (skipping indices, data skipping indices)
	// Traditional indices don't exist in the same way
	// This returns a data skipping index using minmax
	return fmt.Sprintf("ALTER TABLE %s ADD INDEX IF NOT EXISTS %s (%s) TYPE minmax GRANULARITY 3",
		table, indexName, strings.Join(columns, ", "))
}

func (d *ClickHouseDialect) InitStatements() []string {
	// ClickHouse configuration is typically server-level
	return []string{}
}

func (d *ClickHouseDialect) SupportsReturning() bool {
	return false
}

func (d *ClickHouseDialect) QuoteIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "\\`") + "`"
}
