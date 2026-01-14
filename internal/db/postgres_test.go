package db

import (
	"context"
	"os"
	"testing"
)

// TestPostgresIntegration tests PostgreSQL driver integration.
// Requires POSTGRES_TEST_DSN environment variable with a valid PostgreSQL connection.
// Example: POSTGRES_TEST_DSN="postgres://user:password@localhost/test_db?sslmode=disable"
//
// Run with: POSTGRES_TEST_DSN="..." go test -v ./internal/db -run TestPostgres
func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("Skipping PostgreSQL integration test: POSTGRES_TEST_DSN not set")
	}

	t.Run("opens connection with DSN", func(t *testing.T) {
		cfg := PostgresConfig(dsn)

		db, err := openPostgres(cfg)
		if err != nil {
			t.Fatalf("openPostgres() error = %v", err)
		}
		defer db.Close()

		// Verify connection works
		if err := db.Ping(); err != nil {
			t.Errorf("Ping() error = %v", err)
		}
	})

	t.Run("basic CRUD operations", func(t *testing.T) {
		cfg := PostgresConfig(dsn)
		db, err := openPostgres(cfg)
		if err != nil {
			t.Fatalf("openPostgres() error = %v", err)
		}
		defer db.Close()

		// Create table with PostgreSQL syntax
		_, err = db.Exec(`
			CREATE TABLE IF NOT EXISTS postgres_test (
				id SERIAL PRIMARY KEY,
				name TEXT NOT NULL,
				value INTEGER
			)
		`)
		if err != nil {
			t.Fatalf("CREATE TABLE error = %v", err)
		}
		defer db.Exec("DROP TABLE IF EXISTS postgres_test")

		// Insert with PostgreSQL placeholders ($1, $2)
		result, err := db.Exec("INSERT INTO postgres_test (name, value) VALUES ($1, $2)", "test", 42)
		if err != nil {
			t.Fatalf("INSERT error = %v", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			t.Fatalf("RowsAffected() error = %v", err)
		}
		if rowsAffected != 1 {
			t.Errorf("got rowsAffected = %d, want 1", rowsAffected)
		}

		// Query with PostgreSQL placeholders
		var name string
		var value int
		err = db.QueryRow("SELECT name, value FROM postgres_test WHERE name = $1", "test").Scan(&name, &value)
		if err != nil {
			t.Fatalf("QueryRow() error = %v", err)
		}
		if name != "test" || value != 42 {
			t.Errorf("got name=%q value=%d, want name=%q value=%d", name, value, "test", 42)
		}

		// Update
		_, err = db.Exec("UPDATE postgres_test SET value = $1 WHERE name = $2", 100, "test")
		if err != nil {
			t.Fatalf("UPDATE error = %v", err)
		}

		err = db.QueryRow("SELECT value FROM postgres_test WHERE name = $1", "test").Scan(&value)
		if err != nil {
			t.Fatalf("QueryRow() after update error = %v", err)
		}
		if value != 100 {
			t.Errorf("after update, got value = %d, want 100", value)
		}

		// Delete
		_, err = db.Exec("DELETE FROM postgres_test WHERE name = $1", "test")
		if err != nil {
			t.Fatalf("DELETE error = %v", err)
		}

		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM postgres_test").Scan(&count)
		if err != nil {
			t.Fatalf("COUNT query error = %v", err)
		}
		if count != 0 {
			t.Errorf("after delete, got count = %d, want 0", count)
		}
	})

	t.Run("transactions work correctly", func(t *testing.T) {
		cfg := PostgresConfig(dsn)
		db, err := openPostgres(cfg)
		if err != nil {
			t.Fatalf("openPostgres() error = %v", err)
		}
		defer db.Close()

		_, err = db.Exec(`
			CREATE TABLE IF NOT EXISTS tx_test (
				id SERIAL PRIMARY KEY,
				value TEXT
			)
		`)
		if err != nil {
			t.Fatalf("CREATE TABLE error = %v", err)
		}
		defer db.Exec("DROP TABLE IF EXISTS tx_test")

		// Test commit
		tx, err := db.Begin()
		if err != nil {
			t.Fatalf("Begin() error = %v", err)
		}

		_, err = tx.Exec("INSERT INTO tx_test (value) VALUES ($1)", "committed")
		if err != nil {
			t.Fatalf("tx.Exec() error = %v", err)
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit() error = %v", err)
		}

		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM tx_test WHERE value = $1", "committed").Scan(&count)
		if err != nil {
			t.Fatalf("SELECT COUNT error = %v", err)
		}
		if count != 1 {
			t.Errorf("after commit, got count = %d, want 1", count)
		}

		// Test rollback
		tx, err = db.Begin()
		if err != nil {
			t.Fatalf("Begin() error = %v", err)
		}

		_, err = tx.Exec("INSERT INTO tx_test (value) VALUES ($1)", "rolled_back")
		if err != nil {
			t.Fatalf("tx.Exec() error = %v", err)
		}

		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback() error = %v", err)
		}

		err = db.QueryRow("SELECT COUNT(*) FROM tx_test WHERE value = $1", "rolled_back").Scan(&count)
		if err != nil {
			t.Fatalf("SELECT COUNT error = %v", err)
		}
		if count != 0 {
			t.Errorf("after rollback, got count = %d, want 0", count)
		}
	})

	t.Run("connection pool settings are applied", func(t *testing.T) {
		cfg := Config{
			Type:            DatabasePostgres,
			DSN:             dsn,
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: 60,
		}

		db, err := openPostgres(cfg)
		if err != nil {
			t.Fatalf("openPostgres() error = %v", err)
		}
		defer db.Close()

		// Verify we can make multiple concurrent queries
		ctx := context.Background()
		done := make(chan bool)

		for i := 0; i < 20; i++ {
			go func() {
				var result int
				err := db.QueryRowContext(ctx, "SELECT $1::int", 42).Scan(&result)
				if err != nil {
					t.Errorf("concurrent query error = %v", err)
				}
				done <- true
			}()
		}

		// Wait for all queries to complete
		for i := 0; i < 20; i++ {
			<-done
		}
	})

	t.Run("context cancellation works", func(t *testing.T) {
		cfg := PostgresConfig(dsn)
		db, err := openPostgres(cfg)
		if err != nil {
			t.Fatalf("openPostgres() error = %v", err)
		}
		defer db.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err = db.QueryContext(ctx, "SELECT 1")
		if err == nil {
			t.Error("expected error for cancelled context")
		}
	})
}

func TestPostgresConfig(t *testing.T) {
	t.Run("creates config with sensible defaults", func(t *testing.T) {
		dsn := "postgres://user:pass@localhost/db"
		cfg := PostgresConfig(dsn)

		if cfg.Type != DatabasePostgres {
			t.Errorf("got Type = %v, want %v", cfg.Type, DatabasePostgres)
		}
		if cfg.DSN != dsn {
			t.Errorf("got DSN = %v, want %v", cfg.DSN, dsn)
		}
		if cfg.MaxOpenConns != 25 {
			t.Errorf("got MaxOpenConns = %d, want 25", cfg.MaxOpenConns)
		}
		if cfg.MaxIdleConns != 5 {
			t.Errorf("got MaxIdleConns = %d, want 5", cfg.MaxIdleConns)
		}
		if cfg.ConnMaxLifetime != 300 {
			t.Errorf("got ConnMaxLifetime = %d, want 300", cfg.ConnMaxLifetime)
		}
	})
}

func TestPostgresDialect(t *testing.T) {
	dialect := &PostgresDialect{}

	t.Run("uses correct placeholder syntax", func(t *testing.T) {
		tests := []struct {
			index int
			want  string
		}{
			{1, "$1"},
			{2, "$2"},
			{10, "$10"},
		}

		for _, tt := range tests {
			got := dialect.Placeholder(tt.index)
			if got != tt.want {
				t.Errorf("Placeholder(%d) = %q, want %q", tt.index, got, tt.want)
			}
		}
	})

	t.Run("generates valid CREATE TABLE SQL", func(t *testing.T) {
		sql := dialect.CreateTableSQL("test_table", []ColumnDef{
			{Name: "id", Type: ColTypeAutoIncrement, PrimaryKey: true},
			{Name: "name", Type: ColTypeText, Nullable: false},
			{Name: "created_at", Type: ColTypeInteger},
		})

		if sql == "" {
			t.Error("CreateTableSQL returned empty string")
		}
		// Just verify it contains key parts
		if !contains(sql, "CREATE TABLE") || !contains(sql, "test_table") {
			t.Errorf("CreateTableSQL returned invalid SQL: %s", sql)
		}
	})

	t.Run("generates valid UPSERT SQL", func(t *testing.T) {
		sql := dialect.UpsertSQL(
			"embeddings",
			[]string{"path", "content", "embedding"},
			[]string{"path"},
			[]string{"content", "embedding"},
		)

		if sql == "" {
			t.Error("UpsertSQL returned empty string")
		}
		// Verify it uses PostgreSQL ON CONFLICT syntax
		if !contains(sql, "ON CONFLICT") {
			t.Errorf("UpsertSQL should use ON CONFLICT, got: %s", sql)
		}
	})
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
