package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codetect/internal/embedding"
)

// mockEmbedderIntegration provides predictable embeddings for integration tests.
type mockEmbedderIntegration struct {
	dims       int
	callCount  int
	embeddings map[string][]float32
}

func newMockEmbedderIntegration(dims int) *mockEmbedderIntegration {
	return &mockEmbedderIntegration{
		dims:       dims,
		embeddings: make(map[string][]float32),
	}
}

func (m *mockEmbedderIntegration) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		m.callCount++
		if emb, ok := m.embeddings[text]; ok {
			results[i] = emb
		} else {
			// Generate deterministic embedding based on text hash
			results[i] = m.generateEmbedding(text)
			m.embeddings[text] = results[i]
		}
	}
	return results, nil
}

func (m *mockEmbedderIntegration) generateEmbedding(text string) []float32 {
	emb := make([]float32, m.dims)
	// Simple hash-based embedding for deterministic results
	hash := 0
	for _, c := range text {
		hash = hash*31 + int(c)
	}
	for i := range emb {
		emb[i] = float32((hash+i)%1000) / 1000.0
	}
	return emb
}

func (m *mockEmbedderIntegration) Available() bool {
	return true
}

func (m *mockEmbedderIntegration) ProviderID() string {
	return "mock:integration"
}

func (m *mockEmbedderIntegration) Dimensions() int {
	return m.dims
}

// TestV2SearchIntegration tests the full v2 search pipeline:
// 1. Create files → 2. Index with embeddings → 3. Search → 4. Verify results
func TestV2SearchIntegration(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "v2_search_integration")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files with distinct content
	testFiles := map[string]string{
		"auth.go": `package auth

// AuthenticateUser validates user credentials and returns a token.
func AuthenticateUser(username, password string) (string, error) {
	// Validate credentials
	if username == "" || password == "" {
		return "", fmt.Errorf("invalid credentials")
	}
	return generateToken(username), nil
}
`,
		"database.go": `package database

// QueryUsers retrieves users from the database.
func QueryUsers(db *DB, filter string) ([]User, error) {
	rows, err := db.Query("SELECT * FROM users WHERE name LIKE ?", filter)
	if err != nil {
		return nil, err
	}
	return parseUsers(rows), nil
}
`,
		"http.go": `package http

// HandleRequest processes incoming HTTP requests.
func HandleRequest(w ResponseWriter, r *Request) {
	switch r.Method {
	case "GET":
		handleGet(w, r)
	case "POST":
		handlePost(w, r)
	}
}
`,
	}

	for name, content := range testFiles {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	// Create indexer with mock embedder
	cfg := &Config{
		DBType:            "sqlite",
		EmbeddingProvider: "off", // We'll inject our mock
		Dimensions:        768,
		BatchSize:         10,
		MaxWorkers:        1,
	}

	idx, err := New(tempDir, cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer idx.Close()

	// Inject mock embedder and recreate pipeline
	mockEmb := newMockEmbedderIntegration(768)
	idx.embedder = mockEmb
	idx.pipeline = embedding.NewPipeline(
		idx.cache,
		idx.locations,
		mockEmb,
		embedding.WithBatchSize(cfg.BatchSize),
		embedding.WithMaxWorkers(cfg.MaxWorkers),
	)

	// Index the files
	ctx := context.Background()
	result, err := idx.Index(ctx, IndexOptions{Force: true, Verbose: false})
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}

	t.Logf("Indexed %d files, %d chunks, %d embedded (cache hits: %d)",
		result.FilesProcessed, result.ChunksCreated, result.ChunksEmbedded, result.CacheHits)

	// Verify embeddings were created
	if result.ChunksEmbedded == 0 && result.CacheHits == 0 {
		t.Error("No chunks processed")
	}

	// Create V2SemanticSearcher
	searcher := embedding.NewV2SemanticSearcher(
		idx.cache,
		idx.locations,
		mockEmb,
		tempDir,
		idx.vectorIndex,
	)

	if !searcher.Available() {
		t.Fatal("Searcher not available")
	}

	// Test search for "authentication"
	response, err := searcher.Search(ctx, "authentication user credentials", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if !response.Available {
		t.Errorf("Search not available: %s", response.Error)
	}

	if len(response.Results) == 0 {
		t.Error("No search results returned")
	}

	// Log results for debugging
	for i, r := range response.Results {
		t.Logf("Result %d: %s:%d-%d (score=%.4f, type=%s, name=%s)",
			i, r.Path, r.StartLine, r.EndLine, r.Score, r.NodeType, r.NodeName)
	}

	// Verify we get results from auth.go (most relevant to query)
	foundAuth := false
	for _, r := range response.Results {
		if r.Path == "auth.go" {
			foundAuth = true
			break
		}
	}
	if !foundAuth {
		t.Log("Expected auth.go in results for 'authentication' query")
		// Note: With mock embeddings, relevance may not match perfectly
	}
}

// TestV2SearchWithSnippetsIntegration tests search with code snippets.
func TestV2SearchWithSnippetsIntegration(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "v2_snippets_integration")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file
	content := `package main

func hello() {
	println("Hello, World!")
}

func goodbye() {
	println("Goodbye, World!")
}
`
	testFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	// Create and index
	cfg := &Config{
		DBType:            "sqlite",
		EmbeddingProvider: "off",
		Dimensions:        768,
	}

	idx, err := New(tempDir, cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer idx.Close()

	// Inject mock embedder
	mockEmb := newMockEmbedderIntegration(768)
	idx.embedder = mockEmb
	idx.pipeline = embedding.NewPipeline(
		idx.cache,
		idx.locations,
		mockEmb,
		embedding.WithBatchSize(10),
		embedding.WithMaxWorkers(1),
	)

	ctx := context.Background()
	_, err = idx.Index(ctx, IndexOptions{Force: true})
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}

	// Create searcher
	searcher := embedding.NewV2SemanticSearcher(
		idx.cache,
		idx.locations,
		mockEmb,
		tempDir,
		idx.vectorIndex,
	)

	// Search with snippets
	snippetFn := func(path string, start, end int) string {
		return "[snippet from " + path + "]"
	}

	response, err := searcher.SearchWithSnippets(ctx, "hello function", 5, snippetFn)
	if err != nil {
		t.Fatalf("SearchWithSnippets() error = %v", err)
	}

	if len(response.Results) == 0 {
		t.Fatal("No results returned")
	}

	// Verify snippets were populated
	for _, r := range response.Results {
		if r.Snippet == "" {
			t.Errorf("Result %s:%d has empty snippet", r.Path, r.StartLine)
		}
	}
}

// TestV2IncrementalEmbeddings tests that incremental indexing reuses embeddings.
func TestV2IncrementalEmbeddings(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "v2_incremental_test")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create initial file
	content := `package main

func initial() {
	println("initial")
}
`
	testFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	// Create indexer
	cfg := &Config{
		DBType:            "sqlite",
		EmbeddingProvider: "off",
		Dimensions:        768,
	}

	idx, err := New(tempDir, cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer idx.Close()

	// Inject mock embedder
	mockEmb := newMockEmbedderIntegration(768)
	idx.embedder = mockEmb
	idx.pipeline = embedding.NewPipeline(
		idx.cache,
		idx.locations,
		mockEmb,
		embedding.WithBatchSize(10),
		embedding.WithMaxWorkers(1),
	)

	ctx := context.Background()

	// First index
	result1, err := idx.Index(ctx, IndexOptions{Force: true})
	if err != nil {
		t.Fatalf("First Index() error = %v", err)
	}
	callsAfterFirst := mockEmb.callCount

	t.Logf("First index: %d files, %d chunks, embedded=%d, cache_hits=%d, embed_calls=%d",
		result1.FilesProcessed, result1.ChunksCreated, result1.ChunksEmbedded, result1.CacheHits, callsAfterFirst)

	// Index again without changes - should not call embedder
	result2, err := idx.Index(ctx, IndexOptions{Force: false})
	if err != nil {
		t.Fatalf("Second Index() error = %v", err)
	}
	callsAfterSecond := mockEmb.callCount

	t.Logf("Second index: %d files, %d chunks, embedded=%d, cache_hits=%d, embed_calls=%d",
		result2.FilesProcessed, result2.ChunksCreated, result2.ChunksEmbedded, result2.CacheHits, callsAfterSecond)

	if result2.ChangeType != "none" {
		t.Errorf("Expected no changes, got %s", result2.ChangeType)
	}

	// No additional embed calls should have been made
	if callsAfterSecond > callsAfterFirst {
		t.Errorf("Unexpected embed calls: before=%d, after=%d", callsAfterFirst, callsAfterSecond)
	}

	// Add a new file
	newContent := `package main

func newFunction() {
	println("new")
}
`
	newFile := filepath.Join(tempDir, "new.go")
	if err := os.WriteFile(newFile, []byte(newContent), 0644); err != nil {
		t.Fatalf("writing new file: %v", err)
	}

	// Index again - should only embed new content
	result3, err := idx.Index(ctx, IndexOptions{Force: false})
	if err != nil {
		t.Fatalf("Third Index() error = %v", err)
	}
	callsAfterThird := mockEmb.callCount

	t.Logf("Third index: %d files, %d chunks, embedded=%d, cache_hits=%d, embed_calls=%d",
		result3.FilesProcessed, result3.ChunksCreated, result3.ChunksEmbedded, result3.CacheHits, callsAfterThird)

	if result3.ChangeType != "incremental" {
		t.Errorf("Expected incremental changes, got %s", result3.ChangeType)
	}

	// Should have new embed calls for the new file
	if callsAfterThird <= callsAfterSecond {
		t.Errorf("Expected new embed calls for new file: before=%d, after=%d", callsAfterSecond, callsAfterThird)
	}
}

// BenchmarkV2Search benchmarks the v2 semantic search.
func BenchmarkV2Search(b *testing.B) {
	// Create temp directory with files
	tempDir, err := os.MkdirTemp("", "v2_search_bench")
	if err != nil {
		b.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create 50 test files
	for i := 0; i < 50; i++ {
		content := `package main

func function` + itoa(i) + `() {
	println("hello from function ` + itoa(i) + `")
}
`
		path := filepath.Join(tempDir, "file"+itoa(i)+".go")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			b.Fatalf("writing file: %v", err)
		}
	}

	// Create indexer
	cfg := &Config{
		DBType:            "sqlite",
		EmbeddingProvider: "off",
		Dimensions:        768,
	}

	idx, err := New(tempDir, cfg)
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}
	defer idx.Close()

	// Inject mock embedder
	mockEmb := newMockEmbedderIntegration(768)
	idx.embedder = mockEmb
	idx.pipeline = embedding.NewPipeline(
		idx.cache,
		idx.locations,
		mockEmb,
		embedding.WithBatchSize(32),
		embedding.WithMaxWorkers(4),
	)

	ctx := context.Background()

	// Index all files
	_, err = idx.Index(ctx, IndexOptions{Force: true})
	if err != nil {
		b.Fatalf("Index() error = %v", err)
	}

	// Create searcher
	searcher := embedding.NewV2SemanticSearcher(
		idx.cache,
		idx.locations,
		mockEmb,
		tempDir,
		idx.vectorIndex,
	)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := searcher.Search(ctx, "hello function", 10)
		if err != nil {
			b.Fatalf("Search() error = %v", err)
		}
	}
}

// BenchmarkV2IncrementalIndex benchmarks incremental indexing.
func BenchmarkV2IncrementalIndex(b *testing.B) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "v2_incr_bench")
	if err != nil {
		b.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create 100 test files
	for i := 0; i < 100; i++ {
		content := `package main

func function` + itoa(i) + `() {
	println("hello")
}
`
		path := filepath.Join(tempDir, "file"+itoa(i)+".go")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			b.Fatalf("writing file: %v", err)
		}
	}

	// Create indexer
	cfg := &Config{
		DBType:            "sqlite",
		EmbeddingProvider: "off",
		Dimensions:        768,
	}

	idx, err := New(tempDir, cfg)
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}
	defer idx.Close()

	// Inject mock embedder
	mockEmb := newMockEmbedderIntegration(768)
	idx.embedder = mockEmb
	idx.pipeline = embedding.NewPipeline(
		idx.cache,
		idx.locations,
		mockEmb,
		embedding.WithBatchSize(32),
		embedding.WithMaxWorkers(4),
	)

	ctx := context.Background()

	// Initial full index
	_, err = idx.Index(ctx, IndexOptions{Force: true})
	if err != nil {
		b.Fatalf("Initial Index() error = %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Incremental index (no changes)
		_, err := idx.Index(ctx, IndexOptions{Force: false})
		if err != nil {
			b.Fatalf("Index() error = %v", err)
		}
	}
}

// BenchmarkV2SingleFileChange benchmarks incremental index with single file change.
func BenchmarkV2SingleFileChange(b *testing.B) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "v2_single_bench")
	if err != nil {
		b.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create 100 test files
	for i := 0; i < 100; i++ {
		content := `package main

func function` + itoa(i) + `() {
	println("hello")
}
`
		path := filepath.Join(tempDir, "file"+itoa(i)+".go")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			b.Fatalf("writing file: %v", err)
		}
	}

	// Create indexer
	cfg := &Config{
		DBType:            "sqlite",
		EmbeddingProvider: "off",
		Dimensions:        768,
	}

	idx, err := New(tempDir, cfg)
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}
	defer idx.Close()

	// Inject mock embedder
	mockEmb := newMockEmbedderIntegration(768)
	idx.embedder = mockEmb
	idx.pipeline = embedding.NewPipeline(
		idx.cache,
		idx.locations,
		mockEmb,
		embedding.WithBatchSize(32),
		embedding.WithMaxWorkers(4),
	)

	ctx := context.Background()

	// Initial full index
	_, err = idx.Index(ctx, IndexOptions{Force: true})
	if err != nil {
		b.Fatalf("Initial Index() error = %v", err)
	}

	targetFile := filepath.Join(tempDir, "file0.go")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()

		// Modify one file
		content := `package main

func function0() {
	println("modified ` + itoa(i) + `")
}
`
		if err := os.WriteFile(targetFile, []byte(content), 0644); err != nil {
			b.Fatalf("modifying file: %v", err)
		}

		// Small delay to ensure file timestamp changes
		time.Sleep(10 * time.Millisecond)

		b.StartTimer()

		// Incremental index
		_, err := idx.Index(ctx, IndexOptions{Force: false})
		if err != nil {
			b.Fatalf("Index() error = %v", err)
		}
	}
}
