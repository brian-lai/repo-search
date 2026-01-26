package embedding

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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
}

// NewSemanticSearcher creates a new semantic searcher from an EmbeddingStore.
func NewSemanticSearcher(store *EmbeddingStore, embedder Embedder) *SemanticSearcher {
	return &SemanticSearcher{
		store:    store,
		embedder: embedder,
	}
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
		snippet := getSnippet(record.Path, record.StartLine, record.EndLine)

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

// getSnippet retrieves a code snippet placeholder (truncated for display)
func getSnippet(path string, startLine, endLine int) string {
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
	var successfulChunks []Chunk
	var successfulEmbeddings [][]float32
	var skippedCount int

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
			// Log and skip chunks that fail to embed
			fmt.Fprintf(os.Stderr, "\n[codetect-index] failed to embed %s:%d-%d: %v\n", chunk.Path, chunk.StartLine, chunk.EndLine, err)
			skippedCount++
			continue
		}
		if len(embs) == 0 || len(embs[0]) == 0 {
			// Log and skip chunks that return empty embeddings
			fmt.Fprintf(os.Stderr, "\n[codetect-index] empty embedding for %s:%d-%d\n", chunk.Path, chunk.StartLine, chunk.EndLine)
			skippedCount++
			continue
		}
		successfulChunks = append(successfulChunks, chunk)
		successfulEmbeddings = append(successfulEmbeddings, embs[0])
	}

	if skippedCount > 0 {
		fmt.Fprintf(os.Stderr, "\n[codetect-index] skipped %d chunks that failed to embed\n", skippedCount)
	}

	// Save all successful embeddings with provider ID
	if len(successfulChunks) > 0 {
		if err := s.store.SaveBatch(successfulChunks, successfulEmbeddings, providerID); err != nil {
			return fmt.Errorf("saving embeddings: %w", err)
		}
	}

	return nil
}

// IndexChunksParallel embeds and stores chunks with configurable parallelism.
func (s *SemanticSearcher) IndexChunksParallel(ctx context.Context, chunks []Chunk, parallelism int, progressFn func(current, total int)) error {
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

	// Limit parallelism to number of chunks
	if parallelism <= 0 {
		parallelism = len(toEmbed)
	}
	if parallelism > len(toEmbed) {
		parallelism = len(toEmbed)
	}

	// Sequential execution for parallelism=1
	if parallelism == 1 {
		return s.IndexChunks(ctx, chunks, progressFn)
	}

	// Parallel execution with worker pool
	type job struct {
		index int
		chunk Chunk
	}

	type result struct {
		chunk     Chunk
		embedding []float32
		err       error
	}

	jobs := make(chan job, len(toEmbed))
	results := make(chan result, len(toEmbed))

	// Atomic counter for progress
	var completed atomic.Int32
	total := int32(len(toEmbed))

	// Spawn workers
	var wg sync.WaitGroup
	for i := 0; i < parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				select {
				case <-ctx.Done():
					results <- result{err: ctx.Err()}
					return
				default:
				}

				// Embed chunk
				embs, err := s.embedder.Embed(ctx, []string{j.chunk.Content})
				if err != nil {
					results <- result{chunk: j.chunk, err: err}
				} else if len(embs) == 0 || len(embs[0]) == 0 {
					results <- result{chunk: j.chunk, err: fmt.Errorf("empty embedding")}
				} else {
					results <- result{chunk: j.chunk, embedding: embs[0], err: nil}
				}

				// Update progress
				current := completed.Add(1)
				if progressFn != nil {
					progressFn(int(current), int(total))
				}
			}
		}()
	}

	// Send jobs
	for i, chunk := range toEmbed {
		jobs <- job{index: i, chunk: chunk}
	}
	close(jobs)

	// Wait for workers and close results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var successfulChunks []Chunk
	var successfulEmbeddings [][]float32
	var skippedCount int

	for res := range results {
		if res.err != nil {
			if res.err == ctx.Err() {
				return res.err
			}
			// Log the error with chunk details
			fmt.Fprintf(os.Stderr, "\n[codetect-index] failed to embed %s:%d-%d: %v\n", res.chunk.Path, res.chunk.StartLine, res.chunk.EndLine, res.err)
			skippedCount++
			continue
		}
		successfulChunks = append(successfulChunks, res.chunk)
		successfulEmbeddings = append(successfulEmbeddings, res.embedding)
	}

	if skippedCount > 0 {
		fmt.Fprintf(os.Stderr, "\n[codetect-index] skipped %d chunks that failed to embed\n", skippedCount)
	}

	// Save all successful embeddings with provider ID
	if len(successfulChunks) > 0 {
		if err := s.store.SaveBatch(successfulChunks, successfulEmbeddings, providerID); err != nil {
			return fmt.Errorf("saving embeddings: %w", err)
		}
	}

	return nil
}

// CrossRepoSearchResult extends SemanticResult with repo information
type CrossRepoSearchResult struct {
	SemanticResult
	RepoRoot string `json:"repo_root"`
}

// CrossRepoSearchResponse is the response from cross-repo search
type CrossRepoSearchResponse struct {
	Available bool                   `json:"available"`
	Results   []CrossRepoSearchResult `json:"results"`
	Error     string                 `json:"error,omitempty"`
}

// SearchAcrossRepos performs semantic search across all repositories in the same dimension group.
// If repoRoots is empty, searches all repos. If specified, filters to those repos only.
// This is useful for org-wide code search.
func (s *SemanticSearcher) SearchAcrossRepos(ctx context.Context, query string, limit int, repoRoots []string) (*CrossRepoSearchResponse, error) {
	if limit <= 0 {
		limit = 10
	}

	// Check availability
	if !s.Available() {
		return &CrossRepoSearchResponse{
			Available: false,
			Results:   []CrossRepoSearchResult{},
			Error:     "Embedding provider not available",
		}, nil
	}

	// Get all embeddings across repos (from the dimension-specific table)
	records, err := s.store.GetAllAcrossRepos(repoRoots)
	if err != nil {
		return nil, fmt.Errorf("getting embeddings: %w", err)
	}

	if len(records) == 0 {
		return &CrossRepoSearchResponse{
			Available: true,
			Results:   []CrossRepoSearchResult{},
			Error:     "No embeddings indexed in this dimension group",
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
	results := make([]CrossRepoSearchResult, 0, len(topK))
	for _, item := range topK {
		if item.Score <= 0 {
			continue // Skip zero/negative similarity
		}

		record := records[item.Index]
		snippet := getSnippet(record.Path, record.StartLine, record.EndLine)

		results = append(results, CrossRepoSearchResult{
			SemanticResult: SemanticResult{
				Path:      record.Path,
				StartLine: record.StartLine,
				EndLine:   record.EndLine,
				Snippet:   snippet,
				Score:     item.Score,
			},
			RepoRoot: record.RepoRoot,
		})
	}

	return &CrossRepoSearchResponse{
		Available: true,
		Results:   results,
	}, nil
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
