package symbols

import (
	"os"
	"path/filepath"
	"testing"

	"codetect/internal/config"
)

func TestHybridIndexing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if neither ast-grep nor ctags available
	hasAstGrep := AstGrepAvailable()
	hasCtags := CtagsAvailable()

	if !hasAstGrep && !hasCtags {
		t.Skip("Neither ast-grep nor ctags available")
	}

	// Create temporary test directory
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create test files
	testFiles := map[string]string{
		"main.go": `package main

func main() {
	println("Hello")
}

type Server struct {
	Port int
}

func (s *Server) Start() error {
	return nil
}
`,
		"script.js": `function getUserData(id) {
	return fetch('/api/user/' + id);
}

class UserManager {
	constructor() {
		this.users = [];
	}
}
`,
		"util.py": `def calculate_sum(numbers):
	return sum(numbers)

class Calculator:
	def add(self, a, b):
		return a + b
`,
	}

	for filename, content := range testFiles {
		path := filepath.Join(tmpDir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Writing test file %s: %v", filename, err)
		}
	}

	// Create index
	idx, err := NewIndex(dbPath)
	if err != nil {
		t.Fatalf("Creating index: %v", err)
	}
	defer idx.Close()

	// Index the test directory
	if err := idx.Update(tmpDir); err != nil {
		t.Fatalf("Indexing: %v", err)
	}

	// Verify symbols were indexed
	symbolCount, fileCount, err := idx.Stats()
	if err != nil {
		t.Fatalf("Getting stats: %v", err)
	}

	if symbolCount == 0 {
		t.Error("No symbols indexed")
	}

	t.Logf("Indexed %d symbols across %d files", symbolCount, fileCount)

	// Search for specific symbols
	tests := []struct {
		name         string
		expectedName string
	}{
		{"main", "main"},
		{"Server", "Server"},
		{"getUserData", "getUserData"},
		{"UserManager", "UserManager"},
		{"calculate_sum", "calculate_sum"},
		{"Calculator", "Calculator"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// FindSymbol takes (name, kind, limit) - pass empty kind to match any
			results, err := idx.FindSymbol(tt.expectedName, "", 10)
			if err != nil {
				t.Fatalf("FindSymbol(%q): %v", tt.expectedName, err)
			}

			found := false
			for _, sym := range results {
				if sym.Name == tt.expectedName {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("Symbol %q not found in index", tt.expectedName)
			}
		})
	}
}

func TestBackendConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		wantBackend config.IndexBackend
	}{
		{"default", "", config.IndexBackendAuto},
		{"auto", "auto", config.IndexBackendAuto},
		{"hybrid", "hybrid", config.IndexBackendAuto},
		{"ast-grep", "ast-grep", config.IndexBackendAstGrep},
		{"astgrep", "astgrep", config.IndexBackendAstGrep},
		{"ctags", "ctags", config.IndexBackendCtags},
		{"universal-ctags", "universal-ctags", config.IndexBackendCtags},
		{"unknown", "invalid", config.IndexBackendAuto}, // Falls back to auto
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.envValue != "" {
				os.Setenv("CODETECT_INDEX_BACKEND", tt.envValue)
				defer os.Unsetenv("CODETECT_INDEX_BACKEND")
			}

			cfg := config.LoadIndexConfigFromEnv()

			if cfg.Backend != tt.wantBackend {
				t.Errorf("Backend = %q, want %q", cfg.Backend, tt.wantBackend)
			}
		})
	}
}

func TestIndexConfigMethods(t *testing.T) {
	tests := []struct {
		backend          config.IndexBackend
		wantUseAstGrep   bool
		wantUseCtags     bool
		wantRequireAstGrep bool
	}{
		{config.IndexBackendAuto, true, true, false},
		{config.IndexBackendAstGrep, true, false, true},
		{config.IndexBackendCtags, false, true, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.backend), func(t *testing.T) {
			cfg := config.IndexConfig{Backend: tt.backend}

			if got := cfg.UseAstGrep(); got != tt.wantUseAstGrep {
				t.Errorf("UseAstGrep() = %v, want %v", got, tt.wantUseAstGrep)
			}

			if got := cfg.UseCtags(); got != tt.wantUseCtags {
				t.Errorf("UseCtags() = %v, want %v", got, tt.wantUseCtags)
			}

			if got := cfg.RequireAstGrep(); got != tt.wantRequireAstGrep {
				t.Errorf("RequireAstGrep() = %v, want %v", got, tt.wantRequireAstGrep)
			}
		})
	}
}
