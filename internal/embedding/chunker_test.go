package embedding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codetect/internal/search/symbols"
)

func TestChunkByLines(t *testing.T) {
	config := ChunkerConfig{
		MaxChunkLines: 10,
		ChunkOverlap:  2,
	}

	t.Run("creates chunks for small file", func(t *testing.T) {
		lines := make([]string, 8)
		for i := range lines {
			lines[i] = "line content"
		}

		chunks := chunkByLines("test.go", lines, config)

		if len(chunks) != 1 {
			t.Errorf("expected 1 chunk for small file, got %d", len(chunks))
		}

		if chunks[0].StartLine != 1 {
			t.Errorf("expected StartLine=1, got %d", chunks[0].StartLine)
		}
	})

	t.Run("creates overlapping chunks for large file", func(t *testing.T) {
		lines := make([]string, 30)
		for i := range lines {
			lines[i] = "line content"
		}

		chunks := chunkByLines("test.go", lines, config)

		if len(chunks) < 2 {
			t.Errorf("expected multiple chunks for large file, got %d", len(chunks))
		}

		// Verify chunks don't exceed MaxChunkLines
		for _, chunk := range chunks {
			lineCount := chunk.EndLine - chunk.StartLine + 1
			if lineCount > config.MaxChunkLines {
				t.Errorf("chunk exceeds MaxChunkLines: %d lines", lineCount)
			}
		}
	})

	t.Run("skips files below MinChunkLines", func(t *testing.T) {
		lines := []string{"a", "b", "c"} // 3 lines, below MinChunkLines

		chunks := chunkByLines("test.go", lines, config)

		// Should still create a chunk since we have content
		if len(chunks) != 1 {
			t.Errorf("expected 1 chunk even for small file, got %d", len(chunks))
		}
	})

	t.Run("handles empty input", func(t *testing.T) {
		chunks := chunkByLines("test.go", []string{}, config)

		if len(chunks) != 0 {
			t.Errorf("expected 0 chunks for empty file, got %d", len(chunks))
		}
	})
}

func TestChunkFile(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	content := `package main

func Hello() {
	println("Hello")
}

func World() {
	println("World")
}

type Foo struct {
	Name string
}

func (f *Foo) Bar() {
	println(f.Name)
}
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	config := DefaultChunkerConfig()

	t.Run("chunks file without symbols", func(t *testing.T) {
		chunks, err := ChunkFile(testFile, nil, config)
		if err != nil {
			t.Fatalf("ChunkFile error: %v", err)
		}

		if len(chunks) == 0 {
			t.Error("expected at least one chunk")
		}

		// All chunks should have "fixed" kind when no symbols
		for _, chunk := range chunks {
			if chunk.Kind != "fixed" {
				t.Errorf("expected kind='fixed', got %q", chunk.Kind)
			}
		}
	})

	t.Run("chunks file with symbols", func(t *testing.T) {
		syms := []symbols.Symbol{
			{Name: "Hello", Kind: "function", Line: 3},
			{Name: "World", Kind: "function", Line: 7},
			{Name: "Foo", Kind: "struct", Line: 11},
			{Name: "Bar", Kind: "method", Line: 15},
		}

		chunks, err := ChunkFile(testFile, syms, config)
		if err != nil {
			t.Fatalf("ChunkFile error: %v", err)
		}

		if len(chunks) == 0 {
			t.Error("expected at least one chunk")
		}
	})

	t.Run("handles nonexistent file", func(t *testing.T) {
		_, err := ChunkFile("/nonexistent/file.go", nil, config)
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestFilterRelevantSymbols(t *testing.T) {
	syms := []symbols.Symbol{
		{Name: "main", Kind: "function"},
		{Name: "Foo", Kind: "struct"},
		{Name: "x", Kind: "variable"},
		{Name: "Bar", Kind: "type"},
		{Name: "helper", Kind: "function"},
		{Name: "IFace", Kind: "interface"},
		{Name: "Method", Kind: "method"},
		{Name: "const1", Kind: "constant"},
	}

	relevant := filterRelevantSymbols(syms)

	// Should include: function, struct, type, interface, method
	// Should exclude: variable, constant
	expectedCount := 6
	if len(relevant) != expectedCount {
		t.Errorf("expected %d relevant symbols, got %d", expectedCount, len(relevant))
	}

	// Verify no variables or constants
	for _, sym := range relevant {
		if sym.Kind == "variable" || sym.Kind == "constant" {
			t.Errorf("unexpected kind in relevant symbols: %s", sym.Kind)
		}
	}
}

func TestCreateChunk(t *testing.T) {
	lines := []string{
		"package main",
		"",
		"func Hello() {",
		"    println(\"Hello\")",
		"}",
	}

	chunk := createChunk("test.go", lines, 3, 5, "function")

	if chunk.Path != "test.go" {
		t.Errorf("expected Path='test.go', got %q", chunk.Path)
	}
	if chunk.StartLine != 3 {
		t.Errorf("expected StartLine=3, got %d", chunk.StartLine)
	}
	if chunk.EndLine != 5 {
		t.Errorf("expected EndLine=5, got %d", chunk.EndLine)
	}
	if chunk.Kind != "function" {
		t.Errorf("expected Kind='function', got %q", chunk.Kind)
	}
	if !strings.Contains(chunk.Content, "func Hello") {
		t.Errorf("chunk content should contain function definition")
	}
}

func TestDefaultChunkerConfig(t *testing.T) {
	config := DefaultChunkerConfig()

	if config.MaxChunkLines != DefaultMaxChunkLines {
		t.Errorf("expected MaxChunkLines=%d, got %d", DefaultMaxChunkLines, config.MaxChunkLines)
	}
	if config.ChunkOverlap != DefaultChunkOverlap {
		t.Errorf("expected ChunkOverlap=%d, got %d", DefaultChunkOverlap, config.ChunkOverlap)
	}
}

func TestTruncateSnippet(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string unchanged",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length unchanged",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "truncates at newline",
			input:  "line1\nline2\nline3",
			maxLen: 12,
			want:   "line1\nline2\n...",
		},
		{
			name:   "hard truncate when no newline fits",
			input:  "verylonglinewithnonewlines",
			maxLen: 10,
			want:   "verylonglinewithnonewlines"[:10] + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateSnippet(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("TruncateSnippet() = %q, want %q", got, tt.want)
			}
		})
	}
}
