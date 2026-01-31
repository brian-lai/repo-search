package embedding

import (
	"context"
	"testing"

	"codetect/internal/db"
)

// mockEmbedderV2 for testing v2 searcher
type mockEmbedderV2 struct {
	available  bool
	dims       int
	embeddings map[string][]float32
}

func (m *mockEmbedderV2) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		if emb, ok := m.embeddings[text]; ok {
			results[i] = emb
		} else {
			// Return a default embedding
			results[i] = make([]float32, m.dims)
			for j := range results[i] {
				results[i][j] = float32(j%10) / 10.0
			}
		}
	}
	return results, nil
}

func (m *mockEmbedderV2) Available() bool {
	return m.available
}

func (m *mockEmbedderV2) ProviderID() string {
	return "mock:test"
}

func (m *mockEmbedderV2) Dimensions() int {
	return m.dims
}

// setupV2SearcherTest creates test components for V2SemanticSearcher
func setupV2SearcherTest(t *testing.T) (*EmbeddingCache, *LocationStore, db.DB) {
	t.Helper()

	// Create in-memory SQLite database
	cfg := db.DefaultConfig(":memory:")
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}

	t.Cleanup(func() {
		database.Close()
	})

	cache, err := NewEmbeddingCache(database, cfg.Dialect(), 768, "test-model")
	if err != nil {
		t.Fatalf("creating cache: %v", err)
	}

	locations, err := NewLocationStore(database, cfg.Dialect())
	if err != nil {
		t.Fatalf("creating locations: %v", err)
	}

	return cache, locations, database
}

func TestV2SemanticSearcher_Available(t *testing.T) {
	tests := []struct {
		name      string
		embedder  Embedder
		available bool
	}{
		{
			name:      "nil embedder",
			embedder:  nil,
			available: false,
		},
		{
			name:      "unavailable embedder",
			embedder:  &mockEmbedderV2{available: false, dims: 768},
			available: false,
		},
		{
			name:      "available embedder",
			embedder:  &mockEmbedderV2{available: true, dims: 768},
			available: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			searcher := &V2SemanticSearcher{
				embedder: tc.embedder,
			}
			if got := searcher.Available(); got != tc.available {
				t.Errorf("Available() = %v, want %v", got, tc.available)
			}
		})
	}
}

func TestV2SemanticSearcher_Search_NotAvailable(t *testing.T) {
	searcher := &V2SemanticSearcher{
		embedder: &mockEmbedderV2{available: false, dims: 768},
	}

	response, err := searcher.Search(context.Background(), "test query", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response.Available {
		t.Error("expected Available=false")
	}
	if response.Error == "" {
		t.Error("expected an error message")
	}
}

func TestV2SemanticSearcher_Integration(t *testing.T) {
	cache, locations, _ := setupV2SearcherTest(t)

	// Create mock embedder
	embedder := &mockEmbedderV2{
		available:  true,
		dims:       768,
		embeddings: make(map[string][]float32),
	}

	repoRoot := "/test/repo"

	// Create test content
	testContent := "func hello() { println(\"Hello\") }"
	contentHash := hashContent(testContent)

	// Create an embedding
	testEmbedding := make([]float32, 768)
	for i := range testEmbedding {
		testEmbedding[i] = float32(i%10) / 10.0
	}

	// Store embedding in cache
	if err := cache.Put(contentHash, testEmbedding); err != nil {
		t.Fatalf("storing embedding: %v", err)
	}

	// Store location
	loc := ChunkLocation{
		RepoRoot:    repoRoot,
		Path:        "test.go",
		StartLine:   1,
		EndLine:     3,
		ContentHash: contentHash,
		NodeType:    "function",
		NodeName:    "hello",
		Language:    "go",
	}
	if err := locations.SaveLocation(loc); err != nil {
		t.Fatalf("saving location: %v", err)
	}

	// Create searcher (no vector index - will use brute force)
	searcher := NewV2SemanticSearcher(cache, locations, embedder, repoRoot, nil)

	// Search
	response, err := searcher.Search(context.Background(), "hello function", 10)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}

	if !response.Available {
		t.Errorf("expected Available=true, got error: %s", response.Error)
	}

	if len(response.Results) == 0 {
		t.Error("expected at least one result")
	}

	if len(response.Results) > 0 {
		r := response.Results[0]
		if r.Path != "test.go" {
			t.Errorf("Path = %q, want test.go", r.Path)
		}
		if r.StartLine != 1 {
			t.Errorf("StartLine = %d, want 1", r.StartLine)
		}
		if r.NodeType != "function" {
			t.Errorf("NodeType = %q, want function", r.NodeType)
		}
		if r.NodeName != "hello" {
			t.Errorf("NodeName = %q, want hello", r.NodeName)
		}
	}
}

func TestV2SemanticSearcher_SearchWithSnippets(t *testing.T) {
	cache, locations, _ := setupV2SearcherTest(t)

	embedder := &mockEmbedderV2{
		available:  true,
		dims:       768,
		embeddings: make(map[string][]float32),
	}

	repoRoot := "/test/repo"

	// Create test content
	testContent := "func test() {}"
	contentHash := hashContent(testContent)

	testEmbedding := make([]float32, 768)
	for i := range testEmbedding {
		testEmbedding[i] = float32(i%10) / 10.0
	}

	if err := cache.Put(contentHash, testEmbedding); err != nil {
		t.Fatalf("storing embedding: %v", err)
	}

	loc := ChunkLocation{
		RepoRoot:    repoRoot,
		Path:        "test.go",
		StartLine:   1,
		EndLine:     1,
		ContentHash: contentHash,
	}
	if err := locations.SaveLocation(loc); err != nil {
		t.Fatalf("saving location: %v", err)
	}

	searcher := NewV2SemanticSearcher(cache, locations, embedder, repoRoot, nil)

	// Search with snippet function
	snippetFn := func(path string, start, end int) string {
		return "snippet content"
	}

	response, err := searcher.SearchWithSnippets(context.Background(), "test", 10, snippetFn)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}

	if len(response.Results) == 0 {
		t.Fatal("expected at least one result")
	}

	if response.Results[0].Snippet != "snippet content" {
		t.Errorf("Snippet = %q, want 'snippet content'", response.Results[0].Snippet)
	}
}

func TestV2SemanticSearcher_FilterByRepo(t *testing.T) {
	cache, locations, _ := setupV2SearcherTest(t)

	embedder := &mockEmbedderV2{
		available:  true,
		dims:       768,
		embeddings: make(map[string][]float32),
	}

	repoRoot := "/test/repo1"
	otherRepo := "/test/repo2"

	// Create content that appears in both repos
	testContent := "func shared() {}"
	contentHash := hashContent(testContent)

	testEmbedding := make([]float32, 768)
	for i := range testEmbedding {
		testEmbedding[i] = float32(i%10) / 10.0
	}

	if err := cache.Put(contentHash, testEmbedding); err != nil {
		t.Fatalf("storing embedding: %v", err)
	}

	// Location in target repo
	loc1 := ChunkLocation{
		RepoRoot:    repoRoot,
		Path:        "file1.go",
		StartLine:   1,
		EndLine:     1,
		ContentHash: contentHash,
	}
	if err := locations.SaveLocation(loc1); err != nil {
		t.Fatalf("saving location: %v", err)
	}

	// Location in other repo (should be filtered out)
	loc2 := ChunkLocation{
		RepoRoot:    otherRepo,
		Path:        "file2.go",
		StartLine:   1,
		EndLine:     1,
		ContentHash: contentHash,
	}
	if err := locations.SaveLocation(loc2); err != nil {
		t.Fatalf("saving location: %v", err)
	}

	// Search only in repoRoot
	searcher := NewV2SemanticSearcher(cache, locations, embedder, repoRoot, nil)
	response, err := searcher.Search(context.Background(), "shared", 10)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}

	// Should only find the location in repoRoot
	if len(response.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(response.Results))
	}

	if len(response.Results) > 0 && response.Results[0].Path != "file1.go" {
		t.Errorf("expected file1.go, got %s", response.Results[0].Path)
	}
}

func TestV2CosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
		delta    float32
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
			delta:    0.001,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
			delta:    0.001,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: -1.0,
			delta:    0.001,
		},
		{
			name:     "empty vectors",
			a:        []float32{},
			b:        []float32{},
			expected: 0.0,
			delta:    0.001,
		},
		{
			name:     "different lengths",
			a:        []float32{1, 0},
			b:        []float32{1, 0, 0},
			expected: 0.0,
			delta:    0.001,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cosineSimilarity(tc.a, tc.b)
			if diff := got - tc.expected; diff < -tc.delta || diff > tc.delta {
				t.Errorf("cosineSimilarity() = %v, want %v (delta %v)", got, tc.expected, tc.delta)
			}
		})
	}
}
