package config

import (
	"os"
	"testing"

	"codetect/internal/db"
)

func TestLoadDatabaseConfigFromEnv(t *testing.T) {
	// Save and restore original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"CODETECT_DB_TYPE",
		"CODETECT_DB_DSN",
		"CODETECT_DB_PATH",
		"CODETECT_VECTOR_DIMENSIONS",
	}
	for _, key := range envVars {
		originalEnv[key] = os.Getenv(key)
		os.Unsetenv(key)
	}
	defer func() {
		for key, val := range originalEnv {
			if val != "" {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	t.Run("Default Configuration", func(t *testing.T) {
		cfg := LoadDatabaseConfigFromEnv()

		if cfg.Type != db.DatabaseSQLite {
			t.Errorf("Expected default type SQLite, got %v", cfg.Type)
		}

		if cfg.VectorDimensions != 768 {
			t.Errorf("Expected default dimensions 768, got %d", cfg.VectorDimensions)
		}
	})

	t.Run("PostgreSQL Explicit Type", func(t *testing.T) {
		os.Setenv("CODETECT_DB_TYPE", "postgres")
		os.Setenv("CODETECT_DB_DSN", "postgresql://user:pass@localhost/test")

		cfg := LoadDatabaseConfigFromEnv()

		if cfg.Type != db.DatabasePostgres {
			t.Errorf("Expected type PostgreSQL, got %v", cfg.Type)
		}

		if cfg.DSN != "postgresql://user:pass@localhost/test" {
			t.Errorf("Expected DSN to be set, got %s", cfg.DSN)
		}

		os.Unsetenv("CODETECT_DB_TYPE")
		os.Unsetenv("CODETECT_DB_DSN")
	})

	t.Run("PostgreSQL Auto-Detect from DSN", func(t *testing.T) {
		os.Setenv("CODETECT_DB_DSN", "postgres://user:pass@localhost/test")

		cfg := LoadDatabaseConfigFromEnv()

		if cfg.Type != db.DatabasePostgres {
			t.Errorf("Expected type PostgreSQL (auto-detected), got %v", cfg.Type)
		}

		os.Unsetenv("CODETECT_DB_DSN")
	})

	t.Run("SQLite with Path", func(t *testing.T) {
		os.Setenv("CODETECT_DB_TYPE", "sqlite")
		os.Setenv("CODETECT_DB_PATH", "/custom/path/db.sqlite")

		cfg := LoadDatabaseConfigFromEnv()

		if cfg.Type != db.DatabaseSQLite {
			t.Errorf("Expected type SQLite, got %v", cfg.Type)
		}

		if cfg.Path != "/custom/path/db.sqlite" {
			t.Errorf("Expected custom path, got %s", cfg.Path)
		}

		os.Unsetenv("CODETECT_DB_TYPE")
		os.Unsetenv("CODETECT_DB_PATH")
	})

	t.Run("Custom Vector Dimensions", func(t *testing.T) {
		os.Setenv("CODETECT_VECTOR_DIMENSIONS", "1536")

		cfg := LoadDatabaseConfigFromEnv()

		if cfg.VectorDimensions != 1536 {
			t.Errorf("Expected dimensions 1536, got %d", cfg.VectorDimensions)
		}

		os.Unsetenv("CODETECT_VECTOR_DIMENSIONS")
	})

	t.Run("Invalid Database Type Falls Back to SQLite", func(t *testing.T) {
		os.Setenv("CODETECT_DB_TYPE", "invalid")

		cfg := LoadDatabaseConfigFromEnv()

		if cfg.Type != db.DatabaseSQLite {
			t.Errorf("Expected fallback to SQLite, got %v", cfg.Type)
		}

		os.Unsetenv("CODETECT_DB_TYPE")
	})
}

func TestToDBConfig(t *testing.T) {
	t.Run("PostgreSQL Configuration", func(t *testing.T) {
		cfg := DatabaseConfig{
			Type:             db.DatabasePostgres,
			DSN:              "postgresql://user:pass@localhost/test",
			VectorDimensions: 768,
		}

		dbCfg := cfg.ToDBConfig()

		if dbCfg.Type != db.DatabasePostgres {
			t.Errorf("Expected PostgreSQL type, got %v", dbCfg.Type)
		}

		if dbCfg.DSN != "postgresql://user:pass@localhost/test" {
			t.Errorf("Expected DSN to be set, got %s", dbCfg.DSN)
		}

		if dbCfg.VectorDimensions != 768 {
			t.Errorf("Expected dimensions 768, got %d", dbCfg.VectorDimensions)
		}
	})

	t.Run("SQLite Configuration", func(t *testing.T) {
		cfg := DatabaseConfig{
			Type: db.DatabaseSQLite,
			Path: "/custom/path.db",
		}

		dbCfg := cfg.ToDBConfig()

		if dbCfg.Type != db.DatabaseSQLite {
			t.Errorf("Expected SQLite type, got %v", dbCfg.Type)
		}

		if dbCfg.Path != "/custom/path.db" {
			t.Errorf("Expected custom path, got %s", dbCfg.Path)
		}
	})

	t.Run("SQLite Default Path", func(t *testing.T) {
		cfg := DatabaseConfig{
			Type: db.DatabaseSQLite,
			Path: "",
		}

		dbCfg := cfg.ToDBConfig()

		if dbCfg.Path != ".codetect/symbols.db" {
			t.Errorf("Expected default path, got %s", dbCfg.Path)
		}
	})
}

func TestString(t *testing.T) {
	t.Run("PostgreSQL String", func(t *testing.T) {
		cfg := DatabaseConfig{
			Type: db.DatabasePostgres,
			DSN:  "postgresql://user:password@localhost:5432/test",
		}

		str := cfg.String()

		// Should mask password
		if !contains(str, "PostgreSQL") {
			t.Errorf("Expected 'PostgreSQL' in string, got %s", str)
		}

		if contains(str, "password") {
			t.Errorf("Password should be masked in string: %s", str)
		}

		if !contains(str, "***") {
			t.Errorf("Expected '***' for masked password, got %s", str)
		}
	})

	t.Run("SQLite String", func(t *testing.T) {
		cfg := DatabaseConfig{
			Type: db.DatabaseSQLite,
			Path: "/custom/path.db",
		}

		str := cfg.String()

		if !contains(str, "SQLite") {
			t.Errorf("Expected 'SQLite' in string, got %s", str)
		}

		if !contains(str, "/custom/path.db") {
			t.Errorf("Expected path in string, got %s", str)
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}
