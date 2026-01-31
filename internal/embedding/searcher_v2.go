// Package embedding provides the V2SemanticSearcher which uses
// the v2 content-addressed cache and location store for semantic search.
package embedding

import (
	"context"
	"fmt"
	"sort"

	"codetect/internal/config"
	"codetect/internal/db"
)

// V2SemanticSearcher provides semantic search using the v2 architecture.
// It combines the embedding cache (content-addressed), location store
// (file/line mappings), and vector index (HNSW/brute-force) for efficient
// semantic code search.
type V2SemanticSearcher struct {
	cache       *EmbeddingCache
	locations   *LocationStore
	vectorIndex VectorIndex
	embedder    Embedder
	repoRoot    string
}

// V2SearchResult represents a single search result from v2 semantic search.
type V2SearchResult struct {
	Path        string  `json:"path"`
	StartLine   int     `json:"start_line"`
	EndLine     int     `json:"end_line"`
	Score       float32 `json:"score"`
	ContentHash string  `json:"content_hash"`
	NodeType    string  `json:"node_type,omitempty"`
	NodeName    string  `json:"node_name,omitempty"`
	Language    string  `json:"language,omitempty"`
	Snippet     string  `json:"snippet,omitempty"`
}

// V2SearchResponse is the response from a v2 semantic search.
type V2SearchResponse struct {
	Query     string           `json:"query"`
	Results   []V2SearchResult `json:"results"`
	Total     int              `json:"total"`
	Available bool             `json:"available"`
	Error     string           `json:"error,omitempty"`
}

// NewV2SemanticSearcher creates a new v2 semantic searcher.
// If vectorIndex is nil, a brute-force index will be created from the cache.
func NewV2SemanticSearcher(
	cache *EmbeddingCache,
	locations *LocationStore,
	embedder Embedder,
	repoRoot string,
	vectorIndex VectorIndex,
) *V2SemanticSearcher {
	return &V2SemanticSearcher{
		cache:       cache,
		locations:   locations,
		vectorIndex: vectorIndex,
		embedder:    embedder,
		repoRoot:    repoRoot,
	}
}

// Available returns true if the searcher is ready for queries.
func (s *V2SemanticSearcher) Available() bool {
	if s.embedder == nil {
		return false
	}
	return s.embedder.Available()
}

// Search performs semantic search and returns matching locations.
// Flow: embed query → vector search → lookup locations → return results
func (s *V2SemanticSearcher) Search(ctx context.Context, query string, limit int) (*V2SearchResponse, error) {
	response := &V2SearchResponse{
		Query:     query,
		Results:   []V2SearchResult{},
		Available: s.Available(),
	}

	if !s.Available() {
		response.Error = "embedder not available"
		return response, nil
	}

	// Step 1: Embed the query (Embed takes []string, returns [][]float32)
	queryEmbeddings, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		response.Error = fmt.Sprintf("embedding query: %v", err)
		return response, nil
	}

	if len(queryEmbeddings) == 0 || len(queryEmbeddings[0]) == 0 {
		response.Error = "empty embedding returned"
		return response, nil
	}

	queryEmbedding := queryEmbeddings[0]

	// Step 2: Vector search for nearest neighbors
	var vectorResults []VectorResult

	if s.vectorIndex != nil {
		// Use provided vector index (HNSW or configured)
		vectorResults, err = s.vectorIndex.SearchWithFilter(ctx, queryEmbedding, limit*2, []string{s.repoRoot})
		if err != nil {
			response.Error = fmt.Sprintf("vector search: %v", err)
			return response, nil
		}
	} else {
		// Fall back to brute-force search against cache
		vectorResults, err = s.bruteForceSearch(ctx, queryEmbedding, limit*2)
		if err != nil {
			response.Error = fmt.Sprintf("brute-force search: %v", err)
			return response, nil
		}
	}

	// Step 3: Lookup locations for each content hash
	seenLocations := make(map[string]bool) // Dedupe by path:line
	for _, vr := range vectorResults {
		locs, err := s.locations.GetByHash(vr.ContentHash)
		if err != nil {
			continue
		}

		for _, loc := range locs {
			// Filter to this repo
			if loc.RepoRoot != s.repoRoot {
				continue
			}

			key := fmt.Sprintf("%s:%d:%d", loc.Path, loc.StartLine, loc.EndLine)
			if seenLocations[key] {
				continue
			}
			seenLocations[key] = true

			response.Results = append(response.Results, V2SearchResult{
				Path:        loc.Path,
				StartLine:   loc.StartLine,
				EndLine:     loc.EndLine,
				Score:       vr.Score,
				ContentHash: vr.ContentHash,
				NodeType:    loc.NodeType,
				NodeName:    loc.NodeName,
				Language:    loc.Language,
			})
		}
	}

	// Sort by score descending
	sort.Slice(response.Results, func(i, j int) bool {
		return response.Results[i].Score > response.Results[j].Score
	})

	// Apply limit
	if len(response.Results) > limit {
		response.Results = response.Results[:limit]
	}

	response.Total = len(response.Results)
	return response, nil
}

// SearchWithSnippets performs search and adds code snippets to results.
func (s *V2SemanticSearcher) SearchWithSnippets(
	ctx context.Context,
	query string,
	limit int,
	snippetFn func(path string, start, end int) string,
) (*V2SearchResponse, error) {
	response, err := s.Search(ctx, query, limit)
	if err != nil {
		return response, err
	}

	if snippetFn == nil {
		return response, nil
	}

	// Add snippets to results
	for i := range response.Results {
		r := &response.Results[i]
		r.Snippet = snippetFn(r.Path, r.StartLine, r.EndLine)
	}

	return response, nil
}

// bruteForceSearch performs O(n) search against all embeddings in the cache.
// Used when no vector index is available.
func (s *V2SemanticSearcher) bruteForceSearch(ctx context.Context, query []float32, limit int) ([]VectorResult, error) {
	// Get all content hashes for this repo
	hashes, err := s.locations.GetHashesForRepo(s.repoRoot)
	if err != nil {
		return nil, fmt.Errorf("getting repo hashes: %w", err)
	}

	if len(hashes) == 0 {
		return nil, nil
	}

	// Batch fetch embeddings
	entries, err := s.cache.GetBatch(hashes)
	if err != nil {
		return nil, fmt.Errorf("fetching embeddings: %w", err)
	}

	// Compute similarities
	type scored struct {
		hash  string
		score float32
	}
	results := make([]scored, 0, len(entries))

	for hash, entry := range entries {
		if entry == nil || len(entry.Embedding) == 0 {
			continue
		}
		sim := cosineSimilarity(query, entry.Embedding)
		results = append(results, scored{hash: hash, score: sim})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Take top k
	if len(results) > limit {
		results = results[:limit]
	}

	// Convert to VectorResult
	vectorResults := make([]VectorResult, len(results))
	for i, r := range results {
		vectorResults[i] = VectorResult{
			ContentHash: r.hash,
			Score:       r.score,
			Distance:    1 - r.score, // Approximate distance from cosine similarity
		}
	}

	return vectorResults, nil
}

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a []float32, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (sqrt32(normA) * sqrt32(normB))
}

// sqrt32 computes the square root of a float32.
func sqrt32(x float32) float32 {
	if x <= 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// NewV2SemanticSearcherFromDB creates a V2SemanticSearcher from database components.
// This is a convenience constructor for use in MCP tools.
func NewV2SemanticSearcherFromDB(
	database db.DB,
	dialect db.Dialect,
	dimensions int,
	model string,
	embedder Embedder,
	repoRoot string,
) (*V2SemanticSearcher, error) {
	// Create cache
	cache, err := NewEmbeddingCache(database, dialect, dimensions, model)
	if err != nil {
		return nil, fmt.Errorf("creating cache: %w", err)
	}

	// Create location store
	locations, err := NewLocationStore(database, dialect)
	if err != nil {
		return nil, fmt.Errorf("creating locations: %w", err)
	}

	// Try to create a vector index (will use brute-force if HNSW not available)
	var vectorIndex VectorIndex
	if dialect.Name() == "postgres" {
		cfg := config.DefaultHNSWConfig()
		vectorIndex, _ = NewPostgresVectorIndex(database, dimensions, cfg)
	} else {
		// SQLite: try sqlite-vec, fall back to brute-force
		vectorIndex, _ = NewSQLiteVectorIndex(database, dimensions)
	}

	return NewV2SemanticSearcher(cache, locations, embedder, repoRoot, vectorIndex), nil
}
