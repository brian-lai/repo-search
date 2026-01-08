package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"repo-search/internal/embedding"
	"repo-search/internal/mcp"
	"repo-search/internal/search/files"
	"repo-search/internal/search/hybrid"
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

// openSemanticSearcher creates a semantic searcher using the index database
func openSemanticSearcher() (*embedding.SemanticSearcher, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	dbPath := filepath.Join(cwd, ".repo_search", "symbols.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("no index found - run 'make index' first")
	}

	// Open the database
	idx, err := openIndex()
	if err != nil {
		return nil, err
	}

	// Get db from index
	db := idx.DB()

	// Create Ollama client
	ollamaClient := embedding.NewOllamaClient()

	// Create semantic searcher
	return embedding.NewSemanticSearcher(db, ollamaClient)
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
