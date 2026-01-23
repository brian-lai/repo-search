package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	ignore "github.com/sabhiram/go-gitignore"

	"codetect/internal/config"
	"codetect/internal/db"
	"codetect/internal/embedding"
	"codetect/internal/logging"
	"codetect/internal/search/symbols"
)

var logger *slog.Logger

const version = "0.3.0"

func main() {
	logger = logging.Default("codetect-index")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "index":
		runIndex(os.Args[2:])

	case "embed":
		runEmbed(os.Args[2:])

	case "stats":
		runStats(os.Args[2:])

	case "version":
		fmt.Printf("codetect-index v%s\n", version)

	case "help", "-h", "--help":
		printUsage()

	default:
		logger.Error("unknown command", "command", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func runIndex(args []string) {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	force := fs.Bool("force", false, "Force full reindex")
	fs.Parse(args)

	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		logger.Error("invalid path", "error", err)
		os.Exit(1)
	}

	// Check if ctags is available
	if !symbols.CtagsAvailable() {
		logger.Warn("universal-ctags not found, symbol indexing will be skipped",
			"install", "brew install universal-ctags (macOS)")
		os.Exit(0)
	}

	// Load database configuration from environment
	dbConfig := config.LoadDatabaseConfigFromEnv()

	// For SQLite, ensure .codetect directory exists and set path relative to target
	if dbConfig.Type == db.DatabaseSQLite {
		indexDir := filepath.Join(absPath, ".codetect")
		if err := os.MkdirAll(indexDir, 0755); err != nil {
			logger.Error("creating index directory failed", "error", err)
			os.Exit(1)
		}
		// Override path for SQLite to be relative to indexed directory
		dbConfig.Path = filepath.Join(indexDir, "symbols.db")
	}

	// Convert to db.Config
	cfg := dbConfig.ToDBConfig()

	logger.Info("indexing", "path", absPath, "database", dbConfig.String())

	start := time.Now()

	// Open or create index using config-aware constructor with repoRoot for multi-repo isolation
	idx, err := symbols.NewIndexWithConfig(cfg, absPath)
	if err != nil {
		logger.Error("opening index failed", "error", err)
		os.Exit(1)
	}
	defer idx.Close()

	// Run indexing
	if *force {
		logger.Info("running full reindex")
		if err := idx.FullReindex(absPath); err != nil {
			logger.Error("indexing failed", "error", err)
			os.Exit(1)
		}
	} else {
		logger.Info("running incremental index")
		if err := idx.Update(absPath); err != nil {
			logger.Error("indexing failed", "error", err)
			os.Exit(1)
		}
	}

	// Print stats
	symbolCount, fileCount, err := idx.Stats()
	if err != nil {
		logger.Warn("could not get stats", "error", err)
	} else {
		elapsed := time.Since(start)
		logger.Info("indexing complete",
			"symbols", symbolCount,
			"files", fileCount,
			"duration", elapsed.Round(time.Millisecond))
	}
}

func runEmbed(args []string) {
	fs := flag.NewFlagSet("embed", flag.ExitOnError)
	force := fs.Bool("force", false, "Re-embed all chunks (ignore cache)")
	provider := fs.String("provider", "", "Embedding provider (ollama, litellm, off)")
	model := fs.String("model", "", "Embedding model (provider-specific default if empty)")
	parallel := fs.Int("parallel", 10, "Number of parallel embedding workers (default: 10)")
	fs.Parse(args)

	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		logger.Error("invalid path", "error", err)
		os.Exit(1)
	}

	// Load configuration from environment, with flag overrides
	cfg := embedding.LoadConfigFromEnv()
	if *provider != "" {
		switch *provider {
		case "ollama":
			cfg.Provider = embedding.ProviderOllama
		case "litellm":
			cfg.Provider = embedding.ProviderLiteLLM
		case "off":
			cfg.Provider = embedding.ProviderOff
		default:
			logger.Error("unknown provider", "provider", *provider)
			os.Exit(1)
		}
	}
	if *model != "" {
		cfg.Model = *model
	}

	// Check if embedding is disabled
	if cfg.Provider == embedding.ProviderOff {
		logger.Info("embedding disabled", "provider", "off")
		return
	}

	// Create embedder
	embedder, err := embedding.NewEmbedder(cfg)
	if err != nil {
		logger.Error("creating embedder failed", "error", err)
		os.Exit(1)
	}

	// Check availability
	if !embedder.Available() {
		logger.Error("provider not available", "provider", cfg.Provider)
		if cfg.Provider == embedding.ProviderOllama {
			logger.Info("install Ollama from https://ollama.ai, then run: ollama pull nomic-embed-text")
		} else if cfg.Provider == embedding.ProviderLiteLLM {
			logger.Info("check CODETECT_LITELLM_URL and CODETECT_LITELLM_API_KEY")
		}
		os.Exit(1)
	}

	logger.Info("using embedding provider", "provider", embedder.ProviderID())

	// Load database configuration from environment
	dbConfig := config.LoadDatabaseConfigFromEnv()

	// For SQLite, verify index exists and set path relative to target
	if dbConfig.Type == db.DatabaseSQLite {
		indexDir := filepath.Join(absPath, ".codetect")
		dbPath := filepath.Join(indexDir, "symbols.db")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			logger.Error("no symbol index found, run 'codetect-index index' first")
			os.Exit(1)
		}
		dbConfig.Path = dbPath
	}

	// Convert to db.Config
	dbCfg := dbConfig.ToDBConfig()

	logger.Debug("database config", "database", dbConfig.String())

	// Open index using config-aware constructor with repoRoot for multi-repo isolation
	idx, err := symbols.NewIndexWithConfig(dbCfg, absPath)
	if err != nil {
		logger.Error("opening index failed", "error", err)
		os.Exit(1)
	}
	defer idx.Close()

	// Create embedding store with dialect-aware constructor and repoRoot
	store, err := embedding.NewEmbeddingStoreWithOptions(
		idx.DBAdapter(),
		idx.Dialect(),
		dbConfig.VectorDimensions,
		absPath,
	)
	if err != nil {
		logger.Error("creating embedding store failed", "error", err)
		os.Exit(1)
	}
	searcher := embedding.NewSemanticSearcher(store, embedder)

	// Clear embeddings if force flag set
	if *force {
		logger.Info("clearing existing embeddings")
		if err := searcher.Store().DeleteAll(); err != nil {
			logger.Error("clearing embeddings failed", "error", err)
			os.Exit(1)
		}
	}

	// Load gitignore patterns
	gi := loadGitignore(absPath)

	// First pass: collect file info for preview
	logger.Info("scanning files to embed")
	var filesToEmbed []string
	var totalSize int64

	err = filepath.Walk(absPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(absPath, filePath)

		if info.IsDir() {
			name := info.Name()
			// Always skip these directories
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".codetect" {
				return filepath.SkipDir
			}
			// Check gitignore for directories
			if gi != nil && gi.MatchesPath(relPath+"/") {
				return filepath.SkipDir
			}
			return nil
		}

		// Check gitignore for files
		if gi != nil && gi.MatchesPath(relPath) {
			return nil
		}

		// Only count code files
		if isCodeFile(filePath) {
			filesToEmbed = append(filesToEmbed, filePath)
			totalSize += info.Size()
		}

		return nil
	})

	if err != nil {
		logger.Error("scanning directory failed", "error", err)
		os.Exit(1)
	}

	// Display preview
	if len(filesToEmbed) == 0 {
		logger.Info("no code files to embed")
		return
	}

	fmt.Fprintf(os.Stderr, "\n📊 Embedding Preview:\n")
	fmt.Fprintf(os.Stderr, "   Files to embed: %d\n", len(filesToEmbed))
	fmt.Fprintf(os.Stderr, "   Total size: %s\n", formatBytes(totalSize))
	fmt.Fprintf(os.Stderr, "   Provider: %s\n", cfg.Provider)
	if cfg.Model != "" {
		fmt.Fprintf(os.Stderr, "   Model: %s\n", cfg.Model)
	}
	fmt.Fprintf(os.Stderr, "\n")

	// Second pass: chunk files
	logger.Info("collecting code chunks")
	var allChunks []embedding.Chunk
	chunkerConfig := embedding.DefaultChunkerConfig()

	// Walk indexed files and create chunks
	for _, filePath := range filesToEmbed {
		relPath, _ := filepath.Rel(absPath, filePath)

		// Get symbols for this file (for smart chunking)
		syms, _ := idx.ListDefsInFile(relPath)

		chunks, err := embedding.ChunkFile(filePath, syms, chunkerConfig)
		if err != nil {
			continue // Skip files we can't chunk
		}

		// Fix paths to be relative
		for i := range chunks {
			chunks[i].Path = relPath
		}

		allChunks = append(allChunks, chunks...)
	}

	logger.Info("found chunks to embed", "chunks", len(allChunks))

	if len(allChunks) == 0 {
		logger.Info("no chunks to embed")
		return
	}

	// Embed chunks with progress
	start := time.Now()
	ctx := context.Background()

	// Progress output uses fmt.Fprintf for \r carriage return support
	progressFn := func(current, total int) {
		fmt.Fprintf(os.Stderr, "\rembedding chunk %d/%d...", current, total)
	}

	if err := searcher.IndexChunksParallel(ctx, allChunks, *parallel, progressFn); err != nil {
		fmt.Fprintln(os.Stderr) // newline after progress
		logger.Error("embedding failed", "error", err)
		os.Exit(1)
	}

	// Print stats
	count, fileCount, err := searcher.Store().Stats()
	fmt.Fprintln(os.Stderr) // newline after progress
	if err != nil {
		logger.Warn("could not get stats", "error", err)
	} else {
		elapsed := time.Since(start)
		logger.Info("embedding complete",
			"chunks", count,
			"files", fileCount,
			"duration", elapsed.Round(time.Millisecond))
	}
}

// formatBytes converts bytes to human-readable format
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// isCodeFile returns true for files that should be embedded
func isCodeFile(path string) bool {
	ext := filepath.Ext(path)
	codeExts := map[string]bool{
		".go": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
		".py": true, ".rb": true, ".java": true, ".c": true, ".cpp": true,
		".h": true, ".hpp": true, ".rs": true, ".swift": true, ".kt": true,
		".scala": true, ".php": true, ".cs": true, ".sh": true, ".sql": true,
	}
	return codeExts[ext]
}

// loadGitignore loads gitignore patterns from local .gitignore and global ~/.gitignore
func loadGitignore(rootPath string) *ignore.GitIgnore {
	var patterns []string

	// Load global gitignore (~/.gitignore)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		globalGitignore := filepath.Join(homeDir, ".gitignore")
		if content, err := os.ReadFile(globalGitignore); err == nil {
			for _, line := range splitLines(string(content)) {
				if line != "" && !isComment(line) {
					patterns = append(patterns, line)
				}
			}
		}
	}

	// Load local .gitignore
	localGitignore := filepath.Join(rootPath, ".gitignore")
	if content, err := os.ReadFile(localGitignore); err == nil {
		for _, line := range splitLines(string(content)) {
			if line != "" && !isComment(line) {
				patterns = append(patterns, line)
			}
		}
	}

	if len(patterns) == 0 {
		return nil
	}

	gi := ignore.CompileIgnoreLines(patterns...)
	return gi
}

// splitLines splits content into lines, trimming whitespace
func splitLines(content string) []string {
	var lines []string
	for _, line := range filepath.SplitList(content) {
		lines = append(lines, line)
	}
	// Actually split by newlines
	lines = nil
	start := 0
	for i, c := range content {
		if c == '\n' {
			line := content[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

// isComment returns true if line is a gitignore comment
func isComment(line string) bool {
	for _, c := range line {
		if c == ' ' || c == '\t' {
			continue
		}
		return c == '#'
	}
	return false
}

func runStats(args []string) {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		logger.Error("invalid path", "error", err)
		os.Exit(1)
	}

	// Load database configuration from environment
	dbConfig := config.LoadDatabaseConfigFromEnv()

	// For SQLite, verify index exists and set path relative to target
	if dbConfig.Type == db.DatabaseSQLite {
		dbPath := filepath.Join(absPath, ".codetect", "symbols.db")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			logger.Error("no index found, run 'index' first")
			os.Exit(1)
		}
		dbConfig.Path = dbPath
	}

	// Convert to db.Config
	dbCfg := dbConfig.ToDBConfig()

	// Open index using config-aware constructor with repoRoot for multi-repo isolation
	idx, err := symbols.NewIndexWithConfig(dbCfg, absPath)
	if err != nil {
		logger.Error("opening index failed", "error", err)
		os.Exit(1)
	}
	defer idx.Close()

	symbolCount, fileCount, err := idx.Stats()
	if err != nil {
		logger.Error("getting stats failed", "error", err)
		os.Exit(1)
	}

	fmt.Printf("Database: %s\n", dbConfig.String())
	fmt.Printf("Symbols: %d\n", symbolCount)
	fmt.Printf("Files: %d\n", fileCount)

	// Try to get embedding stats using dialect-aware constructor with repoRoot
	store, err := embedding.NewEmbeddingStoreWithOptions(
		idx.DBAdapter(),
		idx.Dialect(),
		dbConfig.VectorDimensions,
		absPath,
	)
	if err == nil {
		embCount, embFileCount, err := store.Stats()
		if err == nil && embCount > 0 {
			fmt.Printf("Embeddings: %d chunks from %d files\n", embCount, embFileCount)
		}
	}
}

func printUsage() {
	fmt.Println(`codetect-index - Codebase indexer for codetect MCP

Usage:
  codetect-index index [--force] [path]   Index symbols using ctags
  codetect-index embed [options] [path]   Generate embeddings
  codetect-index stats [path]             Show index statistics
  codetect-index version                  Print version
  codetect-index help                     Show this help

Index Options:
  --force    Force full reindex (default: incremental)

Embed Options:
  --force      Re-embed all chunks (ignore cache)
  --provider   Embedding provider (ollama, litellm, off)
  --model      Embedding model (provider-specific default if empty)

Database Environment Variables:
  CODETECT_DB_TYPE              Database type: sqlite (default), postgres
  CODETECT_DB_DSN               PostgreSQL connection string
  CODETECT_DB_PATH              SQLite database path override
  CODETECT_VECTOR_DIMENSIONS    Vector dimensions [default: 768]

Embedding Environment Variables:
  CODETECT_EMBEDDING_PROVIDER   Provider (ollama, litellm, off) [default: ollama]
  CODETECT_OLLAMA_URL           Ollama URL [default: http://localhost:11434]
  CODETECT_LITELLM_URL          LiteLLM URL [default: http://localhost:4000]
  CODETECT_LITELLM_API_KEY      LiteLLM API key
  CODETECT_EMBEDDING_MODEL      Model override

Logging Environment Variables:
  CODETECT_LOG_LEVEL            Log level (debug, info, warn, error) [default: info]
  CODETECT_LOG_FORMAT           Output format (text, json) [default: text]

Database:
  Default: SQLite stored in .codetect/symbols.db relative to indexed path.
  PostgreSQL: Set CODETECT_DB_TYPE=postgres and CODETECT_DB_DSN.

Requirements:
  - universal-ctags (for symbol extraction)
  - Ollama OR LiteLLM (optional, for semantic search)
  - PostgreSQL + pgvector (optional, for production deployments)

Install:
  macOS:   brew install universal-ctags
  Ubuntu:  apt install universal-ctags
  Ollama:  https://ollama.ai then 'ollama pull nomic-embed-text'`)
}
