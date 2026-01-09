package embedding

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// SemanticResult represents a search result from semantic search
type SemanticResult struct {
	Path      string  `json:"path"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Snippet   string  `json:"snippet"`
	Score     float32 `json:"score"`
}

// SemanticSearchResult is the full result of a semantic search
type SemanticSearchResult struct {
	Available bool             `json:"available"`
	Results   []SemanticResult `json:"results"`
	Error     string           `json:"error,omitempty"`
}

// SemanticSearcher performs semantic search over embedded code
type SemanticSearcher struct {
	store    *EmbeddingStore
	embedder Embedder
	db       *sql.DB
}

// NewSemanticSearcher creates a new semantic searcher
func NewSemanticSearcher(db *sql.DB, embedder Embedder) (*SemanticSearcher, error) {
	store, err := NewEmbeddingStore(db)
	if err != nil {
		return nil, fmt.Errorf("creating embedding store: %w", err)
	}

	return &SemanticSearcher{
		store:    store,
		embedder: embedder,
		db:       db,
	}, nil
}

// Available checks if semantic search is available
func (s *SemanticSearcher) Available() bool {
	if s.embedder == nil {
		return false
	}
	return s.embedder.Available()
}

// Search performs a semantic search for the given query
func (s *SemanticSearcher) Search(query string, limit int) (*SemanticSearchResult, error) {
	return s.SearchWithContext(context.Background(), query, limit)
}

// SearchWithContext performs a semantic search with a custom context
func (s *SemanticSearcher) SearchWithContext(ctx context.Context, query string, limit int) (*SemanticSearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// Check availability
	if !s.Available() {
		return &SemanticSearchResult{
			Available: false,
			Results:   []SemanticResult{},
			Error:     "Embedding provider not available",
		}, nil
	}

	// Get all embeddings
	records, err := s.store.GetAll()
	if err != nil {
		return nil, fmt.Errorf("getting embeddings: %w", err)
	}

	if len(records) == 0 {
		return &SemanticSearchResult{
			Available: true,
			Results:   []SemanticResult{},
			Error:     "No embeddings indexed. Run 'make embed' first.",
		}, nil
	}

	// Embed the query
	queryEmbeddings, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}
	if len(queryEmbeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned for query")
	}
	queryEmbedding := queryEmbeddings[0]

	// Build vector list for search
	vectors := make([][]float32, len(records))
	for i, r := range records {
		vectors[i] = r.Embedding
	}

	// Find top-k most similar
	topK := TopKByCosineSimilarity(queryEmbedding, vectors, limit)

	// Build results
	results := make([]SemanticResult, 0, len(topK))
	for _, item := range topK {
		if item.Score <= 0 {
			continue // Skip zero/negative similarity
		}

		record := records[item.Index]
		snippet := getSnippet(s.db, record.Path, record.StartLine, record.EndLine)

		results = append(results, SemanticResult{
			Path:      record.Path,
			StartLine: record.StartLine,
			EndLine:   record.EndLine,
			Snippet:   snippet,
			Score:     item.Score,
		})
	}

	return &SemanticSearchResult{
		Available: true,
		Results:   results,
	}, nil
}

// Store returns the underlying embedding store
func (s *SemanticSearcher) Store() *EmbeddingStore {
	return s.store
}

// Embedder returns the underlying embedding provider
func (s *SemanticSearcher) Embedder() Embedder {
	return s.embedder
}

// ProviderID returns the provider ID for the current embedder
func (s *SemanticSearcher) ProviderID() string {
	if s.embedder == nil {
		return "off"
	}
	return s.embedder.ProviderID()
}

// getSnippet retrieves a code snippet from file (truncated for display)
func getSnippet(_ *sql.DB, path string, startLine, endLine int) string {
	// Placeholder - in real usage with SearchWithSnippets, a custom snippetFn is provided
	return fmt.Sprintf("[%s:%d-%d] (%d lines)",
		path, startLine, endLine, endLine-startLine+1)
}

// IndexChunks embeds and stores chunks
func (s *SemanticSearcher) IndexChunks(ctx context.Context, chunks []Chunk, progressFn func(current, total int)) error {
	if len(chunks) == 0 {
		return nil
	}

	if !s.Available() {
		return fmt.Errorf("embedding provider not available")
	}

	providerID := s.embedder.ProviderID()

	// Filter out already indexed chunks
	var toEmbed []Chunk
	for _, chunk := range chunks {
		has, err := s.store.HasEmbedding(chunk, providerID)
		if err != nil {
			return fmt.Errorf("checking embedding: %w", err)
		}
		if !has {
			toEmbed = append(toEmbed, chunk)
		}
	}

	if len(toEmbed) == 0 {
		return nil // All chunks already indexed
	}

	// Embed chunks with progress tracking
	// Process one at a time for progress reporting
	embeddings := make([][]float32, len(toEmbed))
	for i, chunk := range toEmbed {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if progressFn != nil {
			progressFn(i+1, len(toEmbed))
		}

		embs, err := s.embedder.Embed(ctx, []string{chunk.Content})
		if err != nil {
			return fmt.Errorf("embedding chunk %d (%s:%d): %w",
				i, chunk.Path, chunk.StartLine, err)
		}
		if len(embs) == 0 {
			return fmt.Errorf("no embedding returned for chunk %d", i)
		}
		embeddings[i] = embs[0]
	}

	// Save all embeddings with provider ID
	if err := s.store.SaveBatch(toEmbed, embeddings, providerID); err != nil {
		return fmt.Errorf("saving embeddings: %w", err)
	}

	return nil
}

// SearchWithSnippets performs semantic search and includes actual code snippets
func (s *SemanticSearcher) SearchWithSnippets(ctx context.Context, query string, limit int, snippetFn func(path string, start, end int) string) (*SemanticSearchResult, error) {
	result, err := s.SearchWithContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	if snippetFn != nil && result.Available && len(result.Results) > 0 {
		for i := range result.Results {
			r := &result.Results[i]
			r.Snippet = snippetFn(r.Path, r.StartLine, r.EndLine)
			// Truncate long snippets
			if len(r.Snippet) > 500 {
				r.Snippet = r.Snippet[:500] + "..."
			}
		}
	}

	return result, nil
}

// TruncateSnippet truncates a snippet to maxLen characters
func TruncateSnippet(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	// Try to truncate at a newline
	lines := strings.Split(s, "\n")
	var result strings.Builder
	for _, line := range lines {
		if result.Len()+len(line)+1 > maxLen {
			break
		}
		if result.Len() > 0 {
			result.WriteByte('\n')
		}
		result.WriteString(line)
	}

	if result.Len() == 0 {
		return s[:maxLen] + "..."
	}

	return result.String() + "\n..."
}
