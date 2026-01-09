package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"repo-search/internal/embedding"
	"repo-search/internal/search/symbols"
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
		fmt.Printf("repo-search-index v%s\n", version)

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
		fmt.Fprintln(os.Stderr, "[repo-search-index] warning: universal-ctags not found")
		fmt.Fprintln(os.Stderr, "[repo-search-index] symbol indexing will be skipped")
		fmt.Fprintln(os.Stderr, "[repo-search-index] install with: brew install universal-ctags (macOS)")
		os.Exit(0)
	}

	// Create .repo_search directory
	indexDir := filepath.Join(absPath, ".repo_search")
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: creating index directory: %v\n", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(indexDir, "symbols.db")
	fmt.Fprintf(os.Stderr, "[repo-search-index] indexing %s\n", absPath)
	fmt.Fprintf(os.Stderr, "[repo-search-index] database: %s\n", dbPath)

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
		fmt.Fprintln(os.Stderr, "[repo-search-index] running full reindex...")
		if err := idx.FullReindex(absPath); err != nil {
			fmt.Fprintf(os.Stderr, "error: indexing failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintln(os.Stderr, "[repo-search-index] running incremental index...")
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
		fmt.Fprintf(os.Stderr, "[repo-search-index] indexed %d symbols from %d files in %v\n",
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
		fmt.Fprintln(os.Stderr, "[repo-search-index] embedding disabled (provider=off)")
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
		fmt.Fprintf(os.Stderr, "[repo-search-index] error: %s not available\n", cfg.Provider)
		if cfg.Provider == embedding.ProviderOllama {
			fmt.Fprintln(os.Stderr, "[repo-search-index] install Ollama from https://ollama.ai")
			fmt.Fprintln(os.Stderr, "[repo-search-index] then run: ollama pull nomic-embed-text")
		} else if cfg.Provider == embedding.ProviderLiteLLM {
			fmt.Fprintln(os.Stderr, "[repo-search-index] check REPO_SEARCH_LITELLM_URL and REPO_SEARCH_LITELLM_API_KEY")
		}
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "[repo-search-index] using provider: %s\n", embedder.ProviderID())

	// Open database
	indexDir := filepath.Join(absPath, ".repo_search")
	dbPath := filepath.Join(indexDir, "symbols.db")

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "[repo-search-index] error: no symbol index found")
		fmt.Fprintln(os.Stderr, "[repo-search-index] run 'repo-search-index index' first")
		os.Exit(1)
	}

	idx, err := symbols.NewIndex(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: opening index: %v\n", err)
		os.Exit(1)
	}
	defer idx.Close()

	// Create semantic searcher
	searcher, err := embedding.NewSemanticSearcher(idx.DB(), embedder)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: creating semantic searcher: %v\n", err)
		os.Exit(1)
	}

	// Clear embeddings if force flag set
	if *force {
		fmt.Fprintln(os.Stderr, "[repo-search-index] clearing existing embeddings...")
		if err := searcher.Store().DeleteAll(); err != nil {
			fmt.Fprintf(os.Stderr, "error: clearing embeddings: %v\n", err)
			os.Exit(1)
		}
	}

	// Get indexed files and chunk them
	fmt.Fprintln(os.Stderr, "[repo-search-index] collecting code chunks...")
	var allChunks []embedding.Chunk
	chunkerConfig := embedding.DefaultChunkerConfig()

	// Walk indexed files and create chunks
	err = filepath.Walk(absPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".repo_search" {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process code files
		if !isCodeFile(filePath) {
			return nil
		}

		relPath, _ := filepath.Rel(absPath, filePath)

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

	fmt.Fprintf(os.Stderr, "[repo-search-index] found %d chunks to embed\n", len(allChunks))

	if len(allChunks) == 0 {
		fmt.Fprintln(os.Stderr, "[repo-search-index] no chunks to embed")
		return
	}

	// Embed chunks with progress
	start := time.Now()
	ctx := context.Background()

	progressFn := func(current, total int) {
		fmt.Fprintf(os.Stderr, "\r[repo-search-index] embedding chunk %d/%d...", current, total)
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
		fmt.Fprintf(os.Stderr, "\n[repo-search-index] embedded %d chunks from %d files in %v\n",
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

	dbPath := filepath.Join(absPath, ".repo_search", "symbols.db")
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
	store, err := embedding.NewEmbeddingStore(idx.DB())
	if err == nil {
		embCount, embFileCount, err := store.Stats()
		if err == nil && embCount > 0 {
			fmt.Printf("Embeddings: %d chunks from %d files\n", embCount, embFileCount)
		}
	}
}

func printUsage() {
	fmt.Println(`repo-search-index - Codebase indexer for repo-search MCP

Usage:
  repo-search-index index [--force] [path]   Index symbols using ctags
  repo-search-index embed [options] [path]   Generate embeddings
  repo-search-index stats [path]             Show index statistics
  repo-search-index version                  Print version
  repo-search-index help                     Show this help

Index Options:
  --force    Force full reindex (default: incremental)

Embed Options:
  --force      Re-embed all chunks (ignore cache)
  --provider   Embedding provider (ollama, litellm, off)
  --model      Embedding model (provider-specific default if empty)

Environment Variables:
  REPO_SEARCH_EMBEDDING_PROVIDER   Provider (ollama, litellm, off) [default: ollama]
  REPO_SEARCH_OLLAMA_URL           Ollama URL [default: http://localhost:11434]
  REPO_SEARCH_LITELLM_URL          LiteLLM URL [default: http://localhost:4000]
  REPO_SEARCH_LITELLM_API_KEY      LiteLLM API key
  REPO_SEARCH_EMBEDDING_MODEL      Model override
  REPO_SEARCH_EMBEDDING_DIMENSIONS Dimensions override

The index is stored in .repo_search/symbols.db relative to the indexed path.

Requirements:
  - universal-ctags (for symbol extraction)
  - Ollama OR LiteLLM (optional, for semantic search)

Install:
  macOS:   brew install universal-ctags
  Ubuntu:  apt install universal-ctags
  Ollama:  https://ollama.ai then 'ollama pull nomic-embed-text'`)
}
