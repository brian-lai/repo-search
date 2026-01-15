package embedding

import (
	"bufio"
	"os"
	"strings"

	"codetect/internal/search/symbols"
)

const (
	DefaultMaxChunkLines = 30
	DefaultChunkOverlap  = 15
	MinChunkLines        = 5
)

// Chunk represents a code chunk for embedding
type Chunk struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content"`
	Kind      string `json:"kind"` // "function", "class", "type", "block", "fixed"
}

// ChunkerConfig configures the chunking behavior
type ChunkerConfig struct {
	MaxChunkLines int
	ChunkOverlap  int
}

// DefaultChunkerConfig returns the default chunker configuration
func DefaultChunkerConfig() ChunkerConfig {
	return ChunkerConfig{
		MaxChunkLines: DefaultMaxChunkLines,
		ChunkOverlap:  DefaultChunkOverlap,
	}
}

// ChunkFile chunks a file using symbol boundaries if available
func ChunkFile(path string, syms []symbols.Symbol, config ChunkerConfig) ([]Chunk, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return nil, nil
	}

	// If we have symbols, use them for chunking
	if len(syms) > 0 {
		return chunkBySymbols(path, lines, syms, config)
	}

	// Fall back to fixed-size chunking
	return chunkByLines(path, lines, config), nil
}

// chunkBySymbols creates chunks based on symbol boundaries
func chunkBySymbols(path string, lines []string, syms []symbols.Symbol, config ChunkerConfig) ([]Chunk, error) {
	var chunks []Chunk
	covered := make(map[int]bool) // Track which lines are covered

	// Sort symbols by line number and filter to functions/types
	relevantSyms := filterRelevantSymbols(syms)

	for i, sym := range relevantSyms {
		startLine := sym.Line

		// Determine end line - either next symbol's start or file end
		var endLine int
		if i+1 < len(relevantSyms) {
			endLine = relevantSyms[i+1].Line - 1
		} else {
			endLine = len(lines)
		}

		// Clamp to file bounds
		if startLine < 1 {
			startLine = 1
		}
		if endLine > len(lines) {
			endLine = len(lines)
		}
		if startLine > endLine {
			continue
		}

		// Skip very small chunks
		if endLine-startLine+1 < MinChunkLines {
			continue
		}

		// If chunk is too large, split it
		if endLine-startLine+1 > config.MaxChunkLines {
			subChunks := splitLargeChunk(path, lines, startLine, endLine, sym.Kind, config)
			chunks = append(chunks, subChunks...)
		} else {
			chunk := createChunk(path, lines, startLine, endLine, sym.Kind)
			chunks = append(chunks, chunk)
		}

		// Mark lines as covered
		for l := startLine; l <= endLine; l++ {
			covered[l] = true
		}
	}

	// Create chunks for uncovered regions
	uncoveredChunks := chunkUncoveredRegions(path, lines, covered, config)
	chunks = append(chunks, uncoveredChunks...)

	return chunks, nil
}

// filterRelevantSymbols returns symbols that are good chunk boundaries
func filterRelevantSymbols(syms []symbols.Symbol) []symbols.Symbol {
	var relevant []symbols.Symbol
	for _, sym := range syms {
		switch sym.Kind {
		case "function", "struct", "class", "type", "interface", "method":
			relevant = append(relevant, sym)
		}
	}
	return relevant
}

// splitLargeChunk splits a chunk that exceeds MaxChunkLines
func splitLargeChunk(path string, lines []string, startLine, endLine int, kind string, config ChunkerConfig) []Chunk {
	var chunks []Chunk
	current := startLine

	for current <= endLine {
		chunkEnd := current + config.MaxChunkLines - 1
		if chunkEnd > endLine {
			chunkEnd = endLine
		}

		chunk := createChunk(path, lines, current, chunkEnd, kind)
		chunks = append(chunks, chunk)

		// Move to next chunk with overlap
		current = chunkEnd - config.ChunkOverlap + 1
		if current <= chunks[len(chunks)-1].StartLine {
			current = chunkEnd + 1
		}
	}

	return chunks
}

// chunkUncoveredRegions creates chunks for lines not covered by symbols
func chunkUncoveredRegions(path string, lines []string, covered map[int]bool, config ChunkerConfig) []Chunk {
	var chunks []Chunk
	regionStart := -1

	for i := 1; i <= len(lines); i++ {
		if !covered[i] {
			if regionStart == -1 {
				regionStart = i
			}
		} else {
			if regionStart != -1 {
				// End of uncovered region
				regionChunks := chunkByLines(path, lines[regionStart-1:i-1], config)
				// Adjust line numbers
				for j := range regionChunks {
					regionChunks[j].StartLine += regionStart - 1
					regionChunks[j].EndLine += regionStart - 1
				}
				chunks = append(chunks, regionChunks...)
				regionStart = -1
			}
		}
	}

	// Handle trailing uncovered region
	if regionStart != -1 {
		regionChunks := chunkByLines(path, lines[regionStart-1:], config)
		for j := range regionChunks {
			regionChunks[j].StartLine += regionStart - 1
			regionChunks[j].EndLine += regionStart - 1
		}
		chunks = append(chunks, regionChunks...)
	}

	return chunks
}

// chunkByLines creates fixed-size chunks with overlap
func chunkByLines(path string, lines []string, config ChunkerConfig) []Chunk {
	if len(lines) == 0 {
		return nil
	}

	var chunks []Chunk
	current := 0
	prevCurrent := -1 // Track previous position to avoid infinite loops

	for current < len(lines) {
		// Prevent infinite loop
		if current == prevCurrent {
			break
		}
		prevCurrent = current

		end := current + config.MaxChunkLines
		if end > len(lines) {
			end = len(lines)
		}

		// Skip if too small
		if end-current >= MinChunkLines {
			chunk := Chunk{
				Path:      path,
				StartLine: current + 1, // 1-indexed
				EndLine:   end,
				Content:   strings.Join(lines[current:end], "\n"),
				Kind:      "fixed",
			}
			chunks = append(chunks, chunk)
		}

		// If we've reached the end, stop
		if end >= len(lines) {
			break
		}

		// Move to next chunk with overlap
		nextCurrent := end - config.ChunkOverlap
		if nextCurrent <= current {
			// Ensure we always make progress
			nextCurrent = current + 1
		}
		current = nextCurrent
	}

	// If we have no chunks but have content, create one chunk
	if len(chunks) == 0 && len(lines) > 0 {
		chunks = append(chunks, Chunk{
			Path:      path,
			StartLine: 1,
			EndLine:   len(lines),
			Content:   strings.Join(lines, "\n"),
			Kind:      "fixed",
		})
	}

	return chunks
}

// createChunk creates a chunk from line range
func createChunk(path string, lines []string, startLine, endLine int, kind string) Chunk {
	// Convert to 0-indexed for slicing
	startIdx := startLine - 1
	endIdx := endLine

	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx > len(lines) {
		endIdx = len(lines)
	}

	return Chunk{
		Path:      path,
		StartLine: startLine,
		EndLine:   endLine,
		Content:   strings.Join(lines[startIdx:endIdx], "\n"),
		Kind:      kind,
	}
}

// ChunkFileSimple chunks a file without symbol information
func ChunkFileSimple(path string, config ChunkerConfig) ([]Chunk, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	chunks := chunkByLines(path, lines, config)
	// Set the path correctly
	for i := range chunks {
		chunks[i].Path = path
	}

	return chunks, nil
}
