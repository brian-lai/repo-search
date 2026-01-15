package db

// Dialect abstracts SQL syntax differences between database engines.
// Each database (SQLite, PostgreSQL, ClickHouse) has its own implementation.
type Dialect interface {
	// Name returns the dialect name (e.g., "sqlite", "postgres", "clickhouse").
	Name() string

	// Placeholder returns the parameter placeholder for the given index (1-based).
	// SQLite/MySQL use "?", PostgreSQL uses "$1", "$2", etc.
	Placeholder(index int) string

	// Placeholders returns n placeholders joined by ", ".
	// Example: Placeholders(3) returns "?, ?, ?" for SQLite or "$1, $2, $3" for Postgres.
	Placeholders(n int) string

	// AutoIncrementPK returns the SQL type for an auto-incrementing primary key.
	// SQLite: "INTEGER PRIMARY KEY AUTOINCREMENT"
	// Postgres: "SERIAL PRIMARY KEY"
	// ClickHouse: depends on engine
	AutoIncrementPK() string

	// BlobType returns the SQL type for binary data.
	// SQLite: "BLOB", Postgres: "BYTEA", ClickHouse: "String"
	BlobType() string

	// TextType returns the SQL type for text data.
	// Most databases use "TEXT".
	TextType() string

	// IntegerType returns the SQL type for integers.
	// SQLite: "INTEGER", Postgres: "INTEGER" or "BIGINT"
	IntegerType() string

	// TimestampType returns the SQL type for timestamps.
	// SQLite stores as INTEGER (unix), Postgres uses TIMESTAMPTZ.
	TimestampType() string

	// UpsertSQL generates an upsert (insert or update) statement.
	// table: table name
	// columns: all columns to insert
	// conflictColumns: columns that define uniqueness (for ON CONFLICT)
	// updateColumns: columns to update on conflict (if nil, updates all non-conflict columns)
	//
	// SQLite: INSERT OR REPLACE INTO ...
	// Postgres: INSERT INTO ... ON CONFLICT (...) DO UPDATE SET ...
	UpsertSQL(table string, columns []string, conflictColumns []string, updateColumns []string) string

	// CreateTableSQL generates a CREATE TABLE IF NOT EXISTS statement.
	CreateTableSQL(table string, columns []ColumnDef) string

	// CreateIndexSQL generates a CREATE INDEX IF NOT EXISTS statement.
	CreateIndexSQL(table, indexName string, columns []string, unique bool) string

	// InitStatements returns database-specific initialization statements.
	// SQLite: ["PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON"]
	// Postgres: [] (configuration is connection-level)
	InitStatements() []string

	// SupportsReturning returns true if the database supports RETURNING clause.
	// Postgres: true, SQLite (3.35+): true, ClickHouse: false
	SupportsReturning() bool

	// QuoteIdentifier quotes a table or column name to handle reserved words.
	// SQLite/Postgres: "name" or [name], ClickHouse: `name`
	QuoteIdentifier(name string) string
}

// ColumnDef defines a column for table creation.
type ColumnDef struct {
	Name            string
	Type            ColumnType
	Nullable        bool
	PrimaryKey      bool
	Unique          bool
	Default         string // SQL expression for default value
	VectorDimension int    // For ColTypeVector: number of dimensions (e.g., 768)
}

// ColumnType represents abstract column types that map to database-specific types.
type ColumnType int

const (
	ColTypeInteger ColumnType = iota
	ColTypeText
	ColTypeBlob
	ColTypeTimestamp
	ColTypeReal
	ColTypeBoolean
	ColTypeAutoIncrement // Auto-incrementing primary key
	ColTypeVector        // Vector type for pgvector (PostgreSQL) or JSON (SQLite)
)

// String returns the string representation of the column type.
func (ct ColumnType) String() string {
	switch ct {
	case ColTypeInteger:
		return "INTEGER"
	case ColTypeText:
		return "TEXT"
	case ColTypeBlob:
		return "BLOB"
	case ColTypeTimestamp:
		return "TIMESTAMP"
	case ColTypeReal:
		return "REAL"
	case ColTypeBoolean:
		return "BOOLEAN"
	case ColTypeAutoIncrement:
		return "AUTOINCREMENT"
	case ColTypeVector:
		return "VECTOR"
	default:
		return "UNKNOWN"
	}
}

// GetDialect returns the appropriate dialect for the given database type.
func GetDialect(dbType DatabaseType) Dialect {
	switch dbType {
	case DatabasePostgres:
		return &PostgresDialect{}
	case DatabaseClickHouse:
		return &ClickHouseDialect{}
	default:
		return &SQLiteDialect{}
	}
}

// DatabaseType identifies the database engine.
type DatabaseType string

const (
	// DatabaseSQLite is the SQLite database engine.
	DatabaseSQLite DatabaseType = "sqlite"

	// DatabasePostgres is the PostgreSQL database engine.
	DatabasePostgres DatabaseType = "postgres"

	// DatabaseClickHouse is the ClickHouse database engine.
	DatabaseClickHouse DatabaseType = "clickhouse"
)
