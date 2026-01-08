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
	store  *EmbeddingStore
	client *OllamaClient
	db     *sql.DB
}

// NewSemanticSearcher creates a new semantic searcher
func NewSemanticSearcher(db *sql.DB, client *OllamaClient) (*SemanticSearcher, error) {
	store, err := NewEmbeddingStore(db)
	if err != nil {
		return nil, fmt.Errorf("creating embedding store: %w", err)
	}

	return &SemanticSearcher{
		store:  store,
		client: client,
		db:     db,
	}, nil
}

// Available checks if semantic search is available
func (s *SemanticSearcher) Available() bool {
	if s.client == nil {
		return false
	}
	return s.client.Available()
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
			Error:     "Ollama not available",
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
	queryEmbedding, err := s.client.EmbedWithContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}

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

// Client returns the underlying Ollama client
func (s *SemanticSearcher) Client() *OllamaClient {
	return s.client
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
		return fmt.Errorf("ollama not available")
	}

	model := s.client.Model()

	// Filter out already indexed chunks
	var toEmbed []Chunk
	for _, chunk := range chunks {
		has, err := s.store.HasEmbedding(chunk, model)
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

	// Embed chunks one by one (Ollama doesn't support true batching)
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

		emb, err := s.client.EmbedWithContext(ctx, chunk.Content)
		if err != nil {
			return fmt.Errorf("embedding chunk %d (%s:%d): %w",
				i, chunk.Path, chunk.StartLine, err)
		}
		embeddings[i] = emb
	}

	// Save all embeddings
	if err := s.store.SaveBatch(toEmbed, embeddings, model); err != nil {
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
