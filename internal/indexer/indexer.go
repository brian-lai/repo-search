// Package indexer provides the v2 incremental indexing pipeline.
// It integrates Merkle tree change detection, AST-based chunking,
// content-addressed embedding cache, and HNSW vector indexing.
package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	ignore "github.com/sabhiram/go-gitignore"

	"codetect/internal/chunker"
	"codetect/internal/db"
	"codetect/internal/embedding"
	"codetect/internal/merkle"
)

// Indexer coordinates the v2 indexing pipeline with:
// - Merkle tree change detection for incremental updates
// - AST-based syntactic chunking
// - Content-addressed embedding cache
// - Optional HNSW vector indexing
type Indexer struct {
	repoPath string
	dataDir  string

	// Components
	merkleStore   *merkle.Store
	merkleBuilder *merkle.Builder
	astChunker    *chunker.ASTChunker
	cache         *embedding.EmbeddingCache
	locations     *embedding.LocationStore
	vectorIndex   embedding.VectorIndex
	embedder      embedding.Embedder
	pipeline      *embedding.Pipeline

	// Database
	database db.DB
	dialect  db.Dialect

	// Configuration
	config *Config
	logger *slog.Logger
}

// Config configures the indexer.
type Config struct {
	// Database settings
	DBType string // "sqlite" or "postgres"
	DBPath string // SQLite path (for sqlite type)
	DSN    string // PostgreSQL connection string

	// Embedding settings
	EmbeddingProvider string // "ollama", "litellm", or "off"
	EmbeddingModel    string // Model name
	Dimensions        int    // Vector dimensions
	OllamaURL         string // Ollama API URL
	LiteLLMURL        string // LiteLLM API URL
	LiteLLMKey        string // LiteLLM API key

	// Pipeline settings
	BatchSize  int // Batch size for embedding API calls
	MaxWorkers int // Max concurrent embedding workers

	// Ignore patterns (from .gitignore)
	IgnorePatterns []string
}

// DefaultConfig returns the default indexer configuration.
func DefaultConfig() *Config {
	return &Config{
		DBType:            "sqlite",
		EmbeddingProvider: "ollama",
		EmbeddingModel:    "nomic-embed-text",
		Dimensions:        768,
		OllamaURL:         "http://localhost:11434",
		LiteLLMURL:        "http://localhost:4000",
		BatchSize:         32,
		MaxWorkers:        4,
	}
}

// New creates a new v2 indexer.
func New(repoPath string, cfg *Config) (*Indexer, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	dataDir := filepath.Join(absPath, ".codetect")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}

	idx := &Indexer{
		repoPath: absPath,
		dataDir:  dataDir,
		config:   cfg,
		logger:   slog.Default(),
	}

	// Initialize database
	if err := idx.initDatabase(); err != nil {
		return nil, fmt.Errorf("initializing database: %w", err)
	}

	// Initialize components
	if err := idx.initComponents(); err != nil {
		idx.Close()
		return nil, fmt.Errorf("initializing components: %w", err)
	}

	return idx, nil
}

// initDatabase opens the database connection.
func (idx *Indexer) initDatabase() error {
	var dbCfg db.Config

	switch idx.config.DBType {
	case "postgres":
		dbCfg = db.Config{
			Type: db.DatabasePostgres,
			DSN:  idx.config.DSN,
		}
	default:
		dbPath := idx.config.DBPath
		if dbPath == "" {
			dbPath = filepath.Join(idx.dataDir, "index.db")
		}
		dbCfg = db.Config{
			Type: db.DatabaseSQLite,
			Path: dbPath,
		}
	}

	database, err := db.Open(dbCfg)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	idx.database = database
	idx.dialect = db.GetDialect(dbCfg.Type)
	return nil
}

// initComponents initializes all pipeline components.
func (idx *Indexer) initComponents() error {
	// Merkle tree components
	idx.merkleStore = merkle.NewStore(idx.dataDir)
	idx.merkleBuilder = merkle.NewBuilder()
	// Add any additional ignore patterns
	if len(idx.config.IgnorePatterns) > 0 {
		idx.merkleBuilder.IgnorePatterns = append(
			idx.merkleBuilder.IgnorePatterns,
			idx.config.IgnorePatterns...,
		)
	}

	// AST chunker
	idx.astChunker = chunker.NewASTChunker()

	// Embedding cache and locations
	var err error
	idx.cache, err = embedding.NewEmbeddingCache(
		idx.database,
		idx.dialect,
		idx.config.Dimensions,
		idx.config.EmbeddingModel,
	)
	if err != nil {
		return fmt.Errorf("creating embedding cache: %w", err)
	}

	idx.locations, err = embedding.NewLocationStore(idx.database, idx.dialect)
	if err != nil {
		return fmt.Errorf("creating location store: %w", err)
	}

	// Vector index (create brute force as fallback)
	// The NewBruteForceVectorIndex needs an EmbeddingStore, but we can skip it
	// for now since vector index is optional
	idx.vectorIndex = nil // Will be initialized when needed

	// Embedder (if enabled)
	if idx.config.EmbeddingProvider != "off" {
		idx.embedder, err = idx.createEmbedder()
		if err != nil {
			return fmt.Errorf("creating embedder: %w", err)
		}
	} else {
		idx.embedder = &embedding.NullEmbedder{}
	}

	// Create pipeline
	idx.pipeline = embedding.NewPipeline(
		idx.cache,
		idx.locations,
		idx.embedder,
		embedding.WithBatchSize(idx.config.BatchSize),
		embedding.WithMaxWorkers(idx.config.MaxWorkers),
	)

	return nil
}

// createEmbedder creates the appropriate embedder based on configuration.
func (idx *Indexer) createEmbedder() (embedding.Embedder, error) {
	cfg := embedding.ProviderConfig{
		Model:      idx.config.EmbeddingModel,
		OllamaURL:  idx.config.OllamaURL,
		LiteLLMURL: idx.config.LiteLLMURL,
		LiteLLMKey: idx.config.LiteLLMKey,
	}

	switch idx.config.EmbeddingProvider {
	case "ollama":
		cfg.Provider = embedding.ProviderOllama
	case "litellm":
		cfg.Provider = embedding.ProviderLiteLLM
	default:
		cfg.Provider = embedding.ProviderOff
	}

	return embedding.NewEmbedder(cfg)
}

// Close releases all resources.
func (idx *Indexer) Close() error {
	if idx.database != nil {
		return idx.database.Close()
	}
	return nil
}

// IndexOptions configures the index operation.
type IndexOptions struct {
	Force   bool // Force full reindex
	Verbose bool // Enable verbose logging
}

// IndexResult contains statistics from an index operation.
type IndexResult struct {
	FilesProcessed int           `json:"files_processed"`
	FilesDeleted   int           `json:"files_deleted"`
	ChunksCreated  int           `json:"chunks_created"`
	CacheHits      int           `json:"cache_hits"`
	ChunksEmbedded int           `json:"chunks_embedded"`
	Duration       time.Duration `json:"duration"`
	ChangeType     string        `json:"change_type"` // "full", "incremental", "none"
}

// Index performs incremental or full indexing.
func (idx *Indexer) Index(ctx context.Context, opts IndexOptions) (*IndexResult, error) {
	start := time.Now()
	result := &IndexResult{}

	// 1. Build current Merkle tree
	if opts.Verbose {
		idx.logger.Info("building merkle tree", "path", idx.repoPath)
	}

	newTree, err := idx.merkleBuilder.Build(idx.repoPath)
	if err != nil {
		return nil, fmt.Errorf("building merkle tree: %w", err)
	}

	// 2. Determine what changed
	var filesToProcess []string
	var filesToDelete []string

	if opts.Force {
		result.ChangeType = "full"
		filesToProcess = idx.collectAllFiles(newTree.Root)
		if opts.Verbose {
			idx.logger.Info("force mode", "files", len(filesToProcess))
		}
	} else {
		oldTree, _ := idx.merkleStore.Load()
		changes := merkle.Diff(oldTree, newTree)

		if changes.IsEmpty() {
			result.ChangeType = "none"
			result.Duration = time.Since(start)
			if opts.Verbose {
				idx.logger.Info("no changes detected")
			}
			return result, nil
		}

		result.ChangeType = "incremental"
		filesToProcess = append(changes.Added, changes.Modified...)
		filesToDelete = changes.Deleted

		if opts.Verbose {
			idx.logger.Info("detected changes",
				"added", len(changes.Added),
				"modified", len(changes.Modified),
				"deleted", len(changes.Deleted))
		}
	}

	// 3. Handle deletions
	for _, path := range filesToDelete {
		if err := idx.locations.DeleteByPath(idx.repoPath, path); err != nil {
			idx.logger.Warn("failed to delete locations", "path", path, "error", err)
		}
	}
	result.FilesDeleted = len(filesToDelete)

	// 4. Process files in batches
	batchSize := 100
	for i := 0; i < len(filesToProcess); i += batchSize {
		end := i + batchSize
		if end > len(filesToProcess) {
			end = len(filesToProcess)
		}
		batch := filesToProcess[i:end]

		batchResult, err := idx.processBatch(ctx, batch, opts.Verbose)
		if err != nil {
			idx.logger.Warn("batch processing error", "error", err)
			continue
		}

		result.FilesProcessed += len(batch)
		result.ChunksCreated += batchResult.ChunksCreated
		result.CacheHits += batchResult.CacheHits
		result.ChunksEmbedded += batchResult.ChunksEmbedded
	}

	// 5. Save Merkle tree
	if err := idx.merkleStore.Save(newTree); err != nil {
		return nil, fmt.Errorf("saving merkle tree: %w", err)
	}

	result.Duration = time.Since(start)
	return result, nil
}

// processBatch processes a batch of files.
func (idx *Indexer) processBatch(ctx context.Context, files []string, verbose bool) (*IndexResult, error) {
	result := &IndexResult{}

	// Chunk all files using AST chunker
	var allChunks []embedding.Chunk
	for _, relPath := range files {
		fullPath := filepath.Join(idx.repoPath, relPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			if verbose {
				idx.logger.Debug("skipping file", "path", relPath, "error", err)
			}
			continue
		}

		// Use AST chunker
		astChunks, err := idx.astChunker.ChunkFile(ctx, relPath, content)
		if err != nil {
			if verbose {
				idx.logger.Debug("chunk error", "path", relPath, "error", err)
			}
			continue
		}

		// Convert chunker.Chunk to embedding.Chunk
		for _, ac := range astChunks {
			allChunks = append(allChunks, embedding.Chunk{
				Path:      ac.Path,
				StartLine: ac.StartLine,
				EndLine:   ac.EndLine,
				Content:   ac.Content,
				Kind:      ac.NodeType, // Map NodeType to Kind
			})
		}
	}

	result.ChunksCreated = len(allChunks)

	if len(allChunks) == 0 {
		return result, nil
	}

	// Process through embedding pipeline
	embedResult, err := idx.pipeline.EmbedChunks(ctx, idx.repoPath, allChunks)
	if err != nil {
		return nil, fmt.Errorf("embedding chunks: %w", err)
	}

	result.CacheHits = embedResult.CacheHits
	result.ChunksEmbedded = embedResult.Embedded

	return result, nil
}

// collectAllFiles recursively collects all file paths from a Merkle tree node.
func (idx *Indexer) collectAllFiles(node *merkle.Node) []string {
	var files []string
	if node == nil {
		return files
	}

	if !node.IsDir {
		return []string{node.Path}
	}

	for _, child := range node.Children {
		files = append(files, idx.collectAllFiles(child)...)
	}
	return files
}

// Stats returns statistics about the index.
func (idx *Indexer) Stats() (*IndexStats, error) {
	stats := &IndexStats{}

	// Location stats
	locStats, err := idx.locations.Stats(idx.repoPath)
	if err != nil {
		return nil, fmt.Errorf("getting location stats: %w", err)
	}
	stats.TotalChunks = locStats.TotalLocations
	stats.UniqueHashes = locStats.UniqueHashes
	stats.FileCount = locStats.FileCount
	stats.ByNodeType = locStats.ByNodeType
	stats.ByLanguage = locStats.ByLanguage

	// Cache stats
	cacheStats, err := idx.cache.Stats()
	if err != nil {
		return nil, fmt.Errorf("getting cache stats: %w", err)
	}
	stats.CachedEmbeddings = cacheStats.TotalEntries

	// Vector index stats
	if idx.vectorIndex != nil {
		count, err := idx.vectorIndex.Count(context.Background())
		if err == nil {
			stats.IndexedVectors = count
		}
		stats.VectorIndexNative = idx.vectorIndex.IsNative()
	}

	return stats, nil
}

// IndexStats contains statistics about the index.
type IndexStats struct {
	TotalChunks       int            `json:"total_chunks"`
	UniqueHashes      int            `json:"unique_hashes"`
	FileCount         int            `json:"file_count"`
	CachedEmbeddings  int            `json:"cached_embeddings"`
	IndexedVectors    int            `json:"indexed_vectors"`
	VectorIndexNative bool           `json:"vector_index_native"`
	ByNodeType        map[string]int `json:"by_node_type"`
	ByLanguage        map[string]int `json:"by_language"`
}

// LoadGitignore loads .gitignore patterns for the repository.
func LoadGitignore(repoPath string) []string {
	var patterns []string

	// Load global gitignore
	if homeDir, err := os.UserHomeDir(); err == nil {
		globalPath := filepath.Join(homeDir, ".gitignore")
		if content, err := os.ReadFile(globalPath); err == nil {
			patterns = append(patterns, parseGitignore(string(content))...)
		}
	}

	// Load local .gitignore
	localPath := filepath.Join(repoPath, ".gitignore")
	if content, err := os.ReadFile(localPath); err == nil {
		patterns = append(patterns, parseGitignore(string(content))...)
	}

	return patterns
}

// parseGitignore extracts patterns from gitignore content.
func parseGitignore(content string) []string {
	var patterns []string
	start := 0
	for i := 0; i <= len(content); i++ {
		if i == len(content) || content[i] == '\n' {
			line := content[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			// Skip empty lines and comments
			trimmed := line
			for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t') {
				trimmed = trimmed[1:]
			}
			if len(trimmed) > 0 && trimmed[0] != '#' {
				patterns = append(patterns, trimmed)
			}
			start = i + 1
		}
	}
	return patterns
}

// CompileGitignore compiles patterns into a matcher.
func CompileGitignore(patterns []string) *ignore.GitIgnore {
	if len(patterns) == 0 {
		return nil
	}
	return ignore.CompileIgnoreLines(patterns...)
}

// RepoPath returns the repository path.
func (idx *Indexer) RepoPath() string {
	return idx.repoPath
}

// Pipeline returns the embedding pipeline for external use.
func (idx *Indexer) Pipeline() *embedding.Pipeline {
	return idx.pipeline
}

// Locations returns the location store for external use.
func (idx *Indexer) Locations() *embedding.LocationStore {
	return idx.locations
}

// Cache returns the embedding cache for external use.
func (idx *Indexer) Cache() *embedding.EmbeddingCache {
	return idx.cache
}

// VectorIndex returns the vector index for external use.
// May be nil if no vector index is available.
func (idx *Indexer) VectorIndex() embedding.VectorIndex {
	return idx.vectorIndex
}
