package symbols

import (
	"testing"
)

func TestAstGrepAvailable(t *testing.T) {
	// This test depends on local environment
	// Just verify it doesn't panic
	available := AstGrepAvailable()
	t.Logf("ast-grep available: %v", available)
}

func TestLanguageFromExtension(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"main.go", "go"},
		{"index.ts", "typescript"},
		{"component.tsx", "typescript"},
		{"script.js", "javascript"},
		{"app.jsx", "javascript"},
		{"module.mjs", "javascript"},
		{"main.py", "python"},
		{"lib.rs", "rust"},
		{"App.java", "java"},
		{"main.c", "c"},
		{"header.h", "c"},
		{"main.cpp", "cpp"},
		{"header.hpp", "cpp"},
		{"script.rb", "ruby"},
		{"index.php", "php"},
		{"Program.cs", "csharp"},
		{"Main.kt", "kotlin"},
		{"App.swift", "swift"},
		{"unknown.txt", ""},
		{"noext", ""},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := LanguageFromExtension(tt.filename)
			if got != tt.want {
				t.Errorf("LanguageFromExtension(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestGetLanguagePatterns(t *testing.T) {
	tests := []struct {
		language     string
		shouldExist  bool
		minPatterns  int
	}{
		{"go", true, 4},
		{"typescript", true, 5},
		{"javascript", true, 3},
		{"python", true, 2},
		{"rust", true, 4},
		{"java", true, 3},
		{"c", true, 2},
		{"cpp", true, 2},
		{"ruby", true, 3},
		{"php", true, 3},
		{"csharp", true, 3},
		{"kotlin", true, 2},
		{"swift", true, 3},
		{"unsupported", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.language, func(t *testing.T) {
			patterns := GetLanguagePatterns(tt.language)
			if tt.shouldExist {
				if patterns == nil {
					t.Errorf("GetLanguagePatterns(%q) = nil, expected patterns", tt.language)
					return
				}
				if len(patterns.Patterns) < tt.minPatterns {
					t.Errorf("GetLanguagePatterns(%q) has %d patterns, expected at least %d",
						tt.language, len(patterns.Patterns), tt.minPatterns)
				}
				if patterns.Language != tt.language {
					t.Errorf("GetLanguagePatterns(%q).Language = %q, want %q",
						tt.language, patterns.Language, tt.language)
				}
			} else {
				if patterns != nil {
					t.Errorf("GetLanguagePatterns(%q) = %v, expected nil", tt.language, patterns)
				}
			}
		})
	}
}

func TestSupportedLanguages(t *testing.T) {
	langs := SupportedLanguages()
	if len(langs) == 0 {
		t.Error("SupportedLanguages() returned empty list")
	}

	// Verify all supported languages have patterns
	for _, lang := range langs {
		t.Run(lang, func(t *testing.T) {
			patterns := GetLanguagePatterns(lang)
			if patterns == nil {
				t.Errorf("Language %q in SupportedLanguages() but has no patterns", lang)
			}
		})
	}
}

func TestExtractNameFromText(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"func MyFunction(", "MyFunction"},
		{"function getUserData()", "getUserData"},
		{"def calculate_total():", "calculate_total"},
		{"class MyClass {", "MyClass"},
		{"interface IUser {", "IUser"},
		{"type UserID", "UserID"},
		{"struct Point {", "Point"},
		{"enum Color {", "Color"},
		{"trait Displayable {", "Displayable"},
		{"const myConst =", "myConst"},
		{"  func   SpacedName  (", "SpacedName"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := extractNameFromText(tt.text)
			if got != tt.want {
				t.Errorf("extractNameFromText(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestDeduplicateSymbols(t *testing.T) {
	symbols := []Symbol{
		{Name: "foo", Path: "a.go", Line: 10},
		{Name: "foo", Path: "a.go", Line: 10}, // Duplicate
		{Name: "bar", Path: "a.go", Line: 20},
		{Name: "foo", Path: "b.go", Line: 10}, // Different file
		{Name: "foo", Path: "a.go", Line: 15}, // Different line
	}

	unique := deduplicateSymbols(symbols)

	if len(unique) != 4 {
		t.Errorf("deduplicateSymbols() returned %d symbols, want 4", len(unique))
	}

	// Verify the duplicate was removed
	seen := make(map[string]int)
	for _, sym := range unique {
		key := sym.Path + ":" + sym.Name
		seen[key]++
	}

	if seen["a.go:foo"] != 2 { // Lines 10 and 15
		t.Errorf("Expected 2 'foo' symbols in a.go (different lines), got %d", seen["a.go:foo"])
	}
}

func TestAstGrepEntryToSymbol(t *testing.T) {
	entry := AstGrepEntry{
		Text: "func MyFunction(x int) int {\n    return x * 2\n}",
		Range: AstGrepRange{
			Start: AstGrepPosition{Line: 10, Column: 0},
			End:   AstGrepPosition{Line: 12, Column: 1},
		},
		File: "/path/to/project/main.go",
		Meta: map[string]string{
			"NAME": "MyFunction",
		},
	}

	symbol := astGrepEntryToSymbol(entry, "function", "/path/to/project")

	if symbol.Name != "MyFunction" {
		t.Errorf("Symbol.Name = %q, want %q", symbol.Name, "MyFunction")
	}

	if symbol.Kind != "function" {
		t.Errorf("Symbol.Kind = %q, want %q", symbol.Kind, "function")
	}

	if symbol.Line != 10 {
		t.Errorf("Symbol.Line = %d, want %d", symbol.Line, 10)
	}

	if symbol.Path != "main.go" {
		t.Errorf("Symbol.Path = %q, want %q (relative to root)", symbol.Path, "main.go")
	}
}

func TestAstGrepEntryToSymbolWithoutMetaName(t *testing.T) {
	entry := AstGrepEntry{
		Text:  "function getUserData() {",
		Range: AstGrepRange{Start: AstGrepPosition{Line: 5}},
		File:  "script.js",
		Meta:  map[string]string{}, // No NAME in meta
	}

	symbol := astGrepEntryToSymbol(entry, "function", ".")

	// Should extract name from text
	if symbol.Name != "getUserData" {
		t.Errorf("Symbol.Name = %q, want %q (extracted from text)", symbol.Name, "getUserData")
	}
}
