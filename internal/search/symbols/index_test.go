package symbols

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer db.Close()

	// Verify schema was created
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count)
	if err != nil {
		t.Fatalf("Checking schema_version: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_version should have 1 row, got %d", count)
	}
}

func TestNewIndex(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "symbols.db")

	idx, err := NewIndex(dbPath)
	if err != nil {
		t.Fatalf("NewIndex() error = %v", err)
	}
	defer idx.Close()

	// Verify stats work
	symbolCount, fileCount, err := idx.Stats()
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if symbolCount != 0 {
		t.Errorf("expected 0 symbols, got %d", symbolCount)
	}
	if fileCount != 0 {
		t.Errorf("expected 0 files, got %d", fileCount)
	}
}

func TestFindSymbolEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "symbols.db")

	idx, err := NewIndex(dbPath)
	if err != nil {
		t.Fatalf("NewIndex() error = %v", err)
	}
	defer idx.Close()

	symbols, err := idx.FindSymbol("test", "", 10)
	if err != nil {
		t.Fatalf("FindSymbol() error = %v", err)
	}
	if len(symbols) != 0 {
		t.Errorf("expected 0 symbols, got %d", len(symbols))
	}
}

func TestListDefsInFileEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "symbols.db")

	idx, err := NewIndex(dbPath)
	if err != nil {
		t.Fatalf("NewIndex() error = %v", err)
	}
	defer idx.Close()

	symbols, err := idx.ListDefsInFile("nonexistent.go")
	if err != nil {
		t.Fatalf("ListDefsInFile() error = %v", err)
	}
	if len(symbols) != 0 {
		t.Errorf("expected 0 symbols, got %d", len(symbols))
	}
}

func TestIsCodeFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"main.go", true},
		{"server.ts", true},
		{"app.py", true},
		{"readme.md", false},
		{"config.json", false},
		{"image.png", false},
		{"Makefile", false},
		{"script.sh", true},
		{"app.java", true},
		{"lib.rs", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isCodeFile(tt.path)
			if result != tt.expected {
				t.Errorf("isCodeFile(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsIgnoredDir(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"node_modules", true},
		{"vendor", true},
		{"dist", true},
		{".git", true},
		{"src", false},
		{"internal", false},
		{"pkg", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isIgnoredDir(tt.name)
			if result != tt.expected {
				t.Errorf("isIgnoredDir(%q) = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

func TestClearSymbols(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "symbols.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer db.Close()

	// Insert a test symbol with repo_root (required NOT NULL column)
	_, err = db.Exec(`INSERT INTO symbols (repo_root, name, kind, path, line) VALUES (?, ?, ?, ?, ?)`,
		"/test/repo", "TestFunc", "function", "test.go", 10)
	if err != nil {
		t.Fatalf("Insert error = %v", err)
	}

	// Clear symbols for that file
	err = ClearSymbols(db, "test.go")
	if err != nil {
		t.Fatalf("ClearSymbols() error = %v", err)
	}

	// Verify cleared
	var count int
	db.QueryRow("SELECT COUNT(*) FROM symbols WHERE path = ?", "test.go").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 symbols after clear, got %d", count)
	}
}

func TestSchemaDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nested := filepath.Join(tmpDir, "a", "b", "c")
	dbPath := filepath.Join(nested, "symbols.db")

	// OpenDB should create nested directories
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB() error = %v", err)
	}
	defer db.Close()

	// Verify directory was created
	if _, err := os.Stat(nested); os.IsNotExist(err) {
		t.Errorf("nested directory should exist")
	}
}
