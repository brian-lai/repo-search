package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	ignore "github.com/sabhiram/go-gitignore"

	"codetect/internal/embedding"
	"codetect/internal/search/symbols"
)

const version = "0.3.0"

func main() {
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
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
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
		fmt.Fprintf(os.Stderr, "error: invalid path: %v\n", err)
		os.Exit(1)
	}

	// Check if ctags is available
	if !symbols.CtagsAvailable() {
		fmt.Fprintln(os.Stderr, "[codetect-index] warning: universal-ctags not found")
		fmt.Fprintln(os.Stderr, "[codetect-index] symbol indexing will be skipped")
		fmt.Fprintln(os.Stderr, "[codetect-index] install with: brew install universal-ctags (macOS)")
		os.Exit(0)
	}

	// Create .codetect directory
	indexDir := filepath.Join(absPath, ".codetect")
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: creating index directory: %v\n", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(indexDir, "symbols.db")
	fmt.Fprintf(os.Stderr, "[codetect-index] indexing %s\n", absPath)
	fmt.Fprintf(os.Stderr, "[codetect-index] database: %s\n", dbPath)

	start := time.Now()

	// Open or create index
	idx, err := symbols.NewIndex(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: opening index: %v\n", err)
		os.Exit(1)
	}
	defer idx.Close()

	// Run indexing
	if *force {
		fmt.Fprintln(os.Stderr, "[codetect-index] running full reindex...")
		if err := idx.FullReindex(absPath); err != nil {
			fmt.Fprintf(os.Stderr, "error: indexing failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintln(os.Stderr, "[codetect-index] running incremental index...")
		if err := idx.Update(absPath); err != nil {
			fmt.Fprintf(os.Stderr, "error: indexing failed: %v\n", err)
			os.Exit(1)
		}
	}

	// Print stats
	symbolCount, fileCount, err := idx.Stats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not get stats: %v\n", err)
	} else {
		elapsed := time.Since(start)
		fmt.Fprintf(os.Stderr, "[codetect-index] indexed %d symbols from %d files in %v\n",
			symbolCount, fileCount, elapsed.Round(time.Millisecond))
	}
}

func runEmbed(args []string) {
	fs := flag.NewFlagSet("embed", flag.ExitOnError)
	force := fs.Bool("force", false, "Re-embed all chunks (ignore cache)")
	provider := fs.String("provider", "", "Embedding provider (ollama, litellm, off)")
	model := fs.String("model", "", "Embedding model (provider-specific default if empty)")
	fs.Parse(args)

	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid path: %v\n", err)
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
			fmt.Fprintf(os.Stderr, "error: unknown provider: %s\n", *provider)
			os.Exit(1)
		}
	}
	if *model != "" {
		cfg.Model = *model
	}

	// Check if embedding is disabled
	if cfg.Provider == embedding.ProviderOff {
		fmt.Fprintln(os.Stderr, "[codetect-index] embedding disabled (provider=off)")
		return
	}

	// Create embedder
	embedder, err := embedding.NewEmbedder(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: creating embedder: %v\n", err)
		os.Exit(1)
	}

	// Check availability
	if !embedder.Available() {
		fmt.Fprintf(os.Stderr, "[codetect-index] error: %s not available\n", cfg.Provider)
		if cfg.Provider == embedding.ProviderOllama {
			fmt.Fprintln(os.Stderr, "[codetect-index] install Ollama from https://ollama.ai")
			fmt.Fprintln(os.Stderr, "[codetect-index] then run: ollama pull nomic-embed-text")
		} else if cfg.Provider == embedding.ProviderLiteLLM {
			fmt.Fprintln(os.Stderr, "[codetect-index] check CODETECT_LITELLM_URL and CODETECT_LITELLM_API_KEY")
		}
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "[codetect-index] using provider: %s\n", embedder.ProviderID())

	// Open database
	indexDir := filepath.Join(absPath, ".codetect")
	dbPath := filepath.Join(indexDir, "symbols.db")

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "[codetect-index] error: no symbol index found")
		fmt.Fprintln(os.Stderr, "[codetect-index] run 'codetect-index index' first")
		os.Exit(1)
	}

	idx, err := symbols.NewIndex(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: opening index: %v\n", err)
		os.Exit(1)
	}
	defer idx.Close()

	// Create embedding store and semantic searcher
	store, err := embedding.NewEmbeddingStoreFromSQL(idx.DB())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: creating embedding store: %v\n", err)
		os.Exit(1)
	}
	searcher := embedding.NewSemanticSearcher(store, embedder)

	// Clear embeddings if force flag set
	if *force {
		fmt.Fprintln(os.Stderr, "[codetect-index] clearing existing embeddings...")
		if err := searcher.Store().DeleteAll(); err != nil {
			fmt.Fprintf(os.Stderr, "error: clearing embeddings: %v\n", err)
			os.Exit(1)
		}
	}

	// Get indexed files and chunk them
	fmt.Fprintln(os.Stderr, "[codetect-index] collecting code chunks...")
	var allChunks []embedding.Chunk
	chunkerConfig := embedding.DefaultChunkerConfig()

	// Load gitignore patterns
	gi := loadGitignore(absPath)

	// Walk indexed files and create chunks
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

		// Only process code files
		if !isCodeFile(filePath) {
			return nil
		}

		// Get symbols for this file (for smart chunking)
		syms, _ := idx.ListDefsInFile(relPath)

		chunks, err := embedding.ChunkFile(filePath, syms, chunkerConfig)
		if err != nil {
			return nil // Skip files we can't chunk
		}

		// Fix paths to be relative
		for i := range chunks {
			chunks[i].Path = relPath
		}

		allChunks = append(allChunks, chunks...)
		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: walking directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "[codetect-index] found %d chunks to embed\n", len(allChunks))

	if len(allChunks) == 0 {
		fmt.Fprintln(os.Stderr, "[codetect-index] no chunks to embed")
		return
	}

	// Embed chunks with progress
	start := time.Now()
	ctx := context.Background()

	progressFn := func(current, total int) {
		fmt.Fprintf(os.Stderr, "\r[codetect-index] embedding chunk %d/%d...", current, total)
	}

	if err := searcher.IndexChunks(ctx, allChunks, progressFn); err != nil {
		fmt.Fprintf(os.Stderr, "\nerror: embedding failed: %v\n", err)
		os.Exit(1)
	}

	// Print stats
	count, fileCount, err := searcher.Store().Stats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nwarning: could not get stats: %v\n", err)
	} else {
		elapsed := time.Since(start)
		fmt.Fprintf(os.Stderr, "\n[codetect-index] embedded %d chunks from %d files in %v\n",
			count, fileCount, elapsed.Round(time.Millisecond))
	}
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
		fmt.Fprintf(os.Stderr, "error: invalid path: %v\n", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(absPath, ".codetect", "symbols.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "error: no index found (run 'index' first)")
		os.Exit(1)
	}

	idx, err := symbols.NewIndex(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: opening index: %v\n", err)
		os.Exit(1)
	}
	defer idx.Close()

	symbolCount, fileCount, err := idx.Stats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: getting stats: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Index: %s\n", dbPath)
	fmt.Printf("Symbols: %d\n", symbolCount)
	fmt.Printf("Files: %d\n", fileCount)

	// Try to get embedding stats
	store, err := embedding.NewEmbeddingStoreFromSQL(idx.DB())
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

Environment Variables:
  CODETECT_EMBEDDING_PROVIDER   Provider (ollama, litellm, off) [default: ollama]
  CODETECT_OLLAMA_URL           Ollama URL [default: http://localhost:11434]
  CODETECT_LITELLM_URL          LiteLLM URL [default: http://localhost:4000]
  CODETECT_LITELLM_API_KEY      LiteLLM API key
  CODETECT_EMBEDDING_MODEL      Model override
  CODETECT_EMBEDDING_DIMENSIONS Dimensions override

The index is stored in .codetect/symbols.db relative to the indexed path.

Requirements:
  - universal-ctags (for symbol extraction)
  - Ollama OR LiteLLM (optional, for semantic search)

Install:
  macOS:   brew install universal-ctags
  Ubuntu:  apt install universal-ctags
  Ollama:  https://ollama.ai then 'ollama pull nomic-embed-text'`)
}
