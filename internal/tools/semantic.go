package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"codetect/internal/config"
	"codetect/internal/db"
	"codetect/internal/embedding"
	"codetect/internal/mcp"
	"codetect/internal/search/files"
	"codetect/internal/search/hybrid"
)

// RegisterSemanticTools registers the semantic search MCP tools
func RegisterSemanticTools(server *mcp.Server) {
	registerSearchSemantic(server)
	registerHybridSearch(server)
}

func registerSearchSemantic(server *mcp.Server) {
	tool := mcp.Tool{
		Name:        "search_semantic",
		Description: "Search for code semantically similar to the query. Uses embeddings to find conceptually related code, not just keyword matches. Requires Ollama with nomic-embed-text model.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"query": {
					Type:        "string",
					Description: "Natural language query describing what you're looking for",
				},
				"limit": {
					Type:        "number",
					Description: "Maximum number of results (default: 10)",
				},
			},
			Required: []string{"query"},
		},
	}

	handler := func(args map[string]any) (*mcp.ToolsCallResult, error) {
		query, ok := args["query"].(string)
		if !ok || query == "" {
			return nil, fmt.Errorf("query is required")
		}

		limit := 10
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		// Open semantic searcher
		searcher, err := openSemanticSearcher()
		if err != nil {
			return &mcp.ToolsCallResult{
				Content: []mcp.Content{{
					Type: "text",
					Text: fmt.Sprintf(`{"available": false, "error": %q}`, err.Error()),
				}},
			}, nil
		}

		// Check availability
		if !searcher.Available() {
			return &mcp.ToolsCallResult{
				Content: []mcp.Content{{
					Type: "text",
					Text: `{"available": false, "error": "Ollama not available. Install Ollama and run: ollama pull nomic-embed-text"}`,
				}},
			}, nil
		}

		// Perform search with snippets
		result, err := searcher.SearchWithSnippets(context.Background(), query, limit, getSnippetFn())
		if err != nil {
			return nil, fmt.Errorf("semantic search: %w", err)
		}

		data, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}

		return &mcp.ToolsCallResult{
			Content: []mcp.Content{{
				Type: "text",
				Text: string(data),
			}},
		}, nil
	}

	server.RegisterTool(tool, handler)
}

func registerHybridSearch(server *mcp.Server) {
	tool := mcp.Tool{
		Name:        "hybrid_search",
		Description: "Search combining keyword (ripgrep) and semantic (embedding) search. Returns results from both approaches, ranked by combined score. Semantic search requires Ollama.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"query": {
					Type:        "string",
					Description: "Search query (used for both keyword and semantic search)",
				},
				"keyword_limit": {
					Type:        "number",
					Description: "Max keyword results (default: 20)",
				},
				"semantic_limit": {
					Type:        "number",
					Description: "Max semantic results (default: 10)",
				},
			},
			Required: []string{"query"},
		},
	}

	handler := func(args map[string]any) (*mcp.ToolsCallResult, error) {
		query, ok := args["query"].(string)
		if !ok || query == "" {
			return nil, fmt.Errorf("query is required")
		}

		config := hybrid.DefaultConfig()
		if kl, ok := args["keyword_limit"].(float64); ok {
			config.KeywordLimit = int(kl)
		}
		if sl, ok := args["semantic_limit"].(float64); ok {
			config.SemanticLimit = int(sl)
		}
		config.SnippetFn = getSnippetFn()

		// Try to open semantic searcher (optional)
		var semanticSearcher *embedding.SemanticSearcher
		if s, err := openSemanticSearcher(); err == nil && s.Available() {
			semanticSearcher = s
		}

		// Create hybrid searcher
		hybridSearcher := hybrid.NewSearcher(semanticSearcher)

		// Get working directory
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}

		// Perform search
		result, err := hybridSearcher.Search(context.Background(), query, cwd, config)
		if err != nil {
			return nil, fmt.Errorf("hybrid search: %w", err)
		}

		data, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}

		return &mcp.ToolsCallResult{
			Content: []mcp.Content{{
				Type: "text",
				Text: string(data),
			}},
		}, nil
	}

	server.RegisterTool(tool, handler)
}

// openSemanticSearcher creates a semantic searcher using the configured database.
// It supports both SQLite and PostgreSQL based on environment configuration.
// Falls back to SQLite if PostgreSQL is unavailable.
func openSemanticSearcher() (*embedding.SemanticSearcher, error) {
	// Load database configuration from environment
	dbConfig := config.LoadDatabaseConfigFromEnv()

	// Try to open with configured database type
	store, err := openEmbeddingStore(dbConfig)
	if err != nil {
		// If PostgreSQL fails, try falling back to SQLite
		if dbConfig.Type == db.DatabasePostgres {
			fmt.Fprintf(os.Stderr, "Warning: PostgreSQL unavailable (%v), falling back to SQLite\n", err)

			// Fallback to SQLite
			dbConfig.Type = db.DatabaseSQLite
			cwd, _ := os.Getwd()
			dbConfig.Path = filepath.Join(cwd, ".codetect", "symbols.db")

			store, err = openEmbeddingStore(dbConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to open database (tried PostgreSQL and SQLite): %w", err)
			}
		} else {
			return nil, err
		}
	}

	// Create embedder from environment configuration
	embedder, err := embedding.NewEmbedderFromEnv()
	if err != nil {
		return nil, fmt.Errorf("creating embedder: %w", err)
	}

	// Create semantic searcher
	return embedding.NewSemanticSearcher(store, embedder), nil
}

// openEmbeddingStore opens an embedding store with the given configuration.
func openEmbeddingStore(dbConfig config.DatabaseConfig) (*embedding.EmbeddingStore, error) {
	// Get current working directory as repo root for multi-repo isolation
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	switch dbConfig.Type {
	case db.DatabasePostgres:
		// Open PostgreSQL database
		if dbConfig.DSN == "" {
			return nil, fmt.Errorf("PostgreSQL DSN not configured - set CODETECT_DB_DSN")
		}

		cfg := dbConfig.ToDBConfig()
		database, err := db.Open(cfg)
		if err != nil {
			return nil, fmt.Errorf("opening PostgreSQL: %w", err)
		}

		// Create embedding store with PostgreSQL dialect and repoRoot
		dialect := db.GetDialect(db.DatabasePostgres)
		store, err := embedding.NewEmbeddingStoreWithOptions(database, dialect, dbConfig.VectorDimensions, cwd)
		if err != nil {
			database.Close()
			return nil, fmt.Errorf("creating PostgreSQL embedding store: %w", err)
		}

		return store, nil

	default: // SQLite
		// Determine database path
		dbPath := dbConfig.Path
		if dbPath == "" {
			dbPath = filepath.Join(cwd, ".codetect", "symbols.db")
		}

		// For SQLite, check if database exists
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("no index found at %s - run 'make index' first", dbPath)
		}

		// Open the database using the existing index function
		idx, err := openIndex()
		if err != nil {
			return nil, fmt.Errorf("opening SQLite index: %w", err)
		}

		// Create embedding store from index database with repoRoot
		store, err := embedding.NewEmbeddingStoreFromSQL(idx.DB(), cwd)
		if err != nil {
			return nil, fmt.Errorf("creating SQLite embedding store: %w", err)
		}

		return store, nil
	}
}

// getSnippetFn returns a function that reads code snippets from files
func getSnippetFn() func(path string, start, end int) string {
	return func(path string, start, end int) string {
		result, err := files.GetFile(path, start, end)
		if err != nil {
			return fmt.Sprintf("[Error reading %s: %v]", path, err)
		}

		snippet := result.Content

		// Truncate if too long
		if len(snippet) > 500 {
			snippet = snippet[:500] + "..."
		}

		return snippet
	}
}
