package db

import (
	"strings"
	"testing"
)

func TestGetDialect(t *testing.T) {
	tests := []struct {
		dbType DatabaseType
		name   string
	}{
		{DatabaseSQLite, "sqlite"},
		{DatabasePostgres, "postgres"},
		{DatabaseClickHouse, "clickhouse"},
		{"", "sqlite"}, // Default
	}

	for _, tt := range tests {
		t.Run(string(tt.dbType), func(t *testing.T) {
			d := GetDialect(tt.dbType)
			if d.Name() != tt.name {
				t.Errorf("GetDialect(%q).Name() = %q, want %q", tt.dbType, d.Name(), tt.name)
			}
		})
	}
}

func TestSQLiteDialect_Placeholders(t *testing.T) {
	d := &SQLiteDialect{}

	tests := []struct {
		n    int
		want string
	}{
		{0, ""},
		{1, "?"},
		{3, "?, ?, ?"},
		{5, "?, ?, ?, ?, ?"},
	}

	for _, tt := range tests {
		got := d.Placeholders(tt.n)
		if got != tt.want {
			t.Errorf("Placeholders(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestPostgresDialect_Placeholders(t *testing.T) {
	d := &PostgresDialect{}

	tests := []struct {
		n    int
		want string
	}{
		{0, ""},
		{1, "$1"},
		{3, "$1, $2, $3"},
		{5, "$1, $2, $3, $4, $5"},
	}

	for _, tt := range tests {
		got := d.Placeholders(tt.n)
		if got != tt.want {
			t.Errorf("Placeholders(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestSQLiteDialect_UpsertSQL(t *testing.T) {
	d := &SQLiteDialect{}

	sql := d.UpsertSQL("users", []string{"id", "name", "email"}, []string{"id"}, nil)

	// SQLite uses INSERT OR REPLACE
	if !strings.HasPrefix(sql, "INSERT OR REPLACE") {
		t.Errorf("SQLite UpsertSQL should use INSERT OR REPLACE, got: %s", sql)
	}
	if !strings.Contains(sql, "users") {
		t.Errorf("UpsertSQL should contain table name 'users', got: %s", sql)
	}
	if !strings.Contains(sql, "?, ?, ?") {
		t.Errorf("UpsertSQL should have 3 placeholders, got: %s", sql)
	}
}

func TestPostgresDialect_UpsertSQL(t *testing.T) {
	d := &PostgresDialect{}

	sql := d.UpsertSQL("users", []string{"id", "name", "email"}, []string{"id"}, []string{"name", "email"})

	// PostgreSQL uses ON CONFLICT
	if !strings.Contains(sql, "ON CONFLICT") {
		t.Errorf("Postgres UpsertSQL should use ON CONFLICT, got: %s", sql)
	}
	if !strings.Contains(sql, "DO UPDATE SET") {
		t.Errorf("Postgres UpsertSQL should have DO UPDATE SET, got: %s", sql)
	}
	if !strings.Contains(sql, "$1, $2, $3") {
		t.Errorf("Postgres UpsertSQL should have numbered placeholders, got: %s", sql)
	}
}

func TestClickHouseDialect_UpsertSQL(t *testing.T) {
	d := &ClickHouseDialect{}

	sql := d.UpsertSQL("users", []string{"id", "name", "email"}, []string{"id"}, nil)

	// ClickHouse uses plain INSERT (ReplacingMergeTree handles dedup)
	if !strings.HasPrefix(sql, "INSERT INTO") {
		t.Errorf("ClickHouse UpsertSQL should use INSERT INTO, got: %s", sql)
	}
	// Should NOT have ON CONFLICT
	if strings.Contains(sql, "ON CONFLICT") {
		t.Errorf("ClickHouse UpsertSQL should NOT use ON CONFLICT, got: %s", sql)
	}
}

func TestSQLiteDialect_CreateTableSQL(t *testing.T) {
	d := &SQLiteDialect{}

	columns := []ColumnDef{
		{Name: "id", Type: ColTypeAutoIncrement},
		{Name: "name", Type: ColTypeText, Nullable: false},
		{Name: "score", Type: ColTypeReal, Nullable: true},
	}

	sql := d.CreateTableSQL("test_table", columns)

	if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS test_table") {
		t.Errorf("CreateTableSQL should have CREATE TABLE IF NOT EXISTS, got: %s", sql)
	}
	if !strings.Contains(sql, "INTEGER PRIMARY KEY AUTOINCREMENT") {
		t.Errorf("CreateTableSQL should have AUTOINCREMENT for id, got: %s", sql)
	}
	if !strings.Contains(sql, "name TEXT NOT NULL") {
		t.Errorf("CreateTableSQL should have NOT NULL for name, got: %s", sql)
	}
}

func TestPostgresDialect_CreateTableSQL(t *testing.T) {
	d := &PostgresDialect{}

	columns := []ColumnDef{
		{Name: "id", Type: ColTypeAutoIncrement},
		{Name: "name", Type: ColTypeText, Nullable: false},
		{Name: "data", Type: ColTypeBlob, Nullable: true},
	}

	sql := d.CreateTableSQL("test_table", columns)

	if !strings.Contains(sql, "SERIAL PRIMARY KEY") {
		t.Errorf("Postgres CreateTableSQL should use SERIAL PRIMARY KEY, got: %s", sql)
	}
	if !strings.Contains(sql, "BYTEA") {
		t.Errorf("Postgres CreateTableSQL should use BYTEA for blob, got: %s", sql)
	}
}

func TestClickHouseDialect_CreateTableSQL(t *testing.T) {
	d := &ClickHouseDialect{}

	columns := []ColumnDef{
		{Name: "id", Type: ColTypeInteger, PrimaryKey: true},
		{Name: "name", Type: ColTypeText},
		{Name: "created_at", Type: ColTypeTimestamp},
	}

	sql := d.CreateTableSQL("test_table", columns)

	if !strings.Contains(sql, "ENGINE = ReplacingMergeTree()") {
		t.Errorf("ClickHouse CreateTableSQL should have ReplacingMergeTree, got: %s", sql)
	}
	if !strings.Contains(sql, "ORDER BY") {
		t.Errorf("ClickHouse CreateTableSQL should have ORDER BY, got: %s", sql)
	}
	if !strings.Contains(sql, "DateTime64(3)") {
		t.Errorf("ClickHouse CreateTableSQL should use DateTime64 for timestamp, got: %s", sql)
	}
}

func TestDialects_QuoteIdentifier(t *testing.T) {
	tests := []struct {
		dialect  Dialect
		name     string
		expected string
	}{
		{&SQLiteDialect{}, "table", `"table"`},
		{&SQLiteDialect{}, `my"table`, `"my""table"`},
		{&PostgresDialect{}, "table", `"table"`},
		{&PostgresDialect{}, `my"table`, `"my""table"`},
		{&ClickHouseDialect{}, "table", "`table`"},
		{&ClickHouseDialect{}, "my`table", "`my\\`table`"},
	}

	for _, tt := range tests {
		got := tt.dialect.QuoteIdentifier(tt.name)
		if got != tt.expected {
			t.Errorf("%s.QuoteIdentifier(%q) = %q, want %q",
				tt.dialect.Name(), tt.name, got, tt.expected)
		}
	}
}

func TestDialects_InitStatements(t *testing.T) {
	// SQLite should have PRAGMA statements
	sqlite := &SQLiteDialect{}
	stmts := sqlite.InitStatements()
	if len(stmts) == 0 {
		t.Error("SQLite should have init statements (PRAGMAs)")
	}
	foundWAL := false
	for _, s := range stmts {
		if strings.Contains(s, "WAL") {
			foundWAL = true
		}
	}
	if !foundWAL {
		t.Error("SQLite init statements should include WAL mode")
	}

	// Postgres should have pgvector extension init statement
	postgres := &PostgresDialect{}
	pgInit := postgres.InitStatements()
	if len(pgInit) != 1 || pgInit[0] != "CREATE EXTENSION IF NOT EXISTS vector" {
		t.Errorf("Postgres init statements should enable pgvector, got: %v", pgInit)
	}

	clickhouse := &ClickHouseDialect{}
	if len(clickhouse.InitStatements()) != 0 {
		t.Error("ClickHouse should have no init statements")
	}
}

func TestDialects_SupportsReturning(t *testing.T) {
	tests := []struct {
		dialect  Dialect
		expected bool
	}{
		{&SQLiteDialect{}, false},  // Conservative (SQLite 3.35+ does support it)
		{&PostgresDialect{}, true}, // PostgreSQL supports RETURNING
		{&ClickHouseDialect{}, false},
	}

	for _, tt := range tests {
		got := tt.dialect.SupportsReturning()
		if got != tt.expected {
			t.Errorf("%s.SupportsReturning() = %v, want %v",
				tt.dialect.Name(), got, tt.expected)
		}
	}
}

func TestColumnType_String(t *testing.T) {
	tests := []struct {
		ct   ColumnType
		want string
	}{
		{ColTypeInteger, "INTEGER"},
		{ColTypeText, "TEXT"},
		{ColTypeBlob, "BLOB"},
		{ColTypeTimestamp, "TIMESTAMP"},
		{ColTypeReal, "REAL"},
		{ColTypeBoolean, "BOOLEAN"},
		{ColTypeAutoIncrement, "AUTOINCREMENT"},
	}

	for _, tt := range tests {
		got := tt.ct.String()
		if got != tt.want {
			t.Errorf("ColumnType(%d).String() = %q, want %q", tt.ct, got, tt.want)
		}
	}
}
