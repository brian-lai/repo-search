package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"codetect/internal/config"
	dbpkg "codetect/internal/db"
	"codetect/internal/embedding"
	"codetect/internal/fusion"
	"codetect/internal/indexer"
	"codetect/internal/mcp"
	"codetect/internal/rerank"
	"codetect/internal/search/files"
	"codetect/internal/search/keyword"
)

// RegisterV2SemanticTools registers the v2 semantic search MCP tools.
// These tools use the new retriever with RRF fusion and optional reranking.
func RegisterV2SemanticTools(server *mcp.Server) {
	registerHybridSearchV2(server)
}

func registerHybridSearchV2(server *mcp.Server) {
	tool := mcp.Tool{
		Name:        "hybrid_search_v2",
		Description: "v2 hybrid search combining keyword, semantic, and symbol search with RRF fusion. Uses AST-based chunking and content-addressed caching. Optionally applies cross-encoder reranking for higher precision.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"query": {
					Type:        "string",
					Description: "Search query (used for all search signals)",
				},
				"limit": {
					Type:        "number",
					Description: "Max results to return (default: 20)",
				},
				"rerank": {
					Type:        "boolean",
					Description: "Enable cross-encoder reranking for higher precision (default: false)",
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

		limit := 20
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		enableRerank := false
		if r, ok := args["rerank"].(bool); ok {
			enableRerank = r
		}

		// Get current working directory as repo root
		repoRoot, err := os.Getwd()
		if err != nil {
			repoRoot = "."
		}

		ctx := context.Background()
		start := time.Now()

		// Open v2 indexer for search
		idx, err := openV2Indexer(repoRoot)
		if err != nil {
			return &mcp.ToolsCallResult{
				Content: []mcp.Content{{
					Type: "text",
					Text: fmt.Sprintf(`{"available": false, "error": %q}`, err.Error()),
				}},
			}, nil
		}
		defer idx.Close()

		// Create native v2 semantic searcher
		v2Searcher, err := createV2SemanticSearcher(idx, repoRoot)
		semanticAvailable := err == nil && v2Searcher != nil && v2Searcher.Available()

		// Run keyword and semantic search in parallel
		var keywordResults, semanticResults []fusion.Result
		var keywordErr, semanticErr error
		var wg sync.WaitGroup

		// Keyword search
		wg.Add(1)
		go func() {
			defer wg.Done()
			keywordResults, keywordErr = searchKeywordV2(ctx, query, repoRoot, limit)
		}()

		// Semantic search using native v2 searcher
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v2Searcher == nil || !v2Searcher.Available() {
				return
			}
			semanticResults, semanticErr = searchSemanticV2(ctx, v2Searcher, query, repoRoot, limit)
		}()

		wg.Wait()

		// Log errors but continue (graceful degradation)
		if keywordErr != nil {
			// Non-fatal, just won't have keyword results
			keywordResults = nil
		}
		if semanticErr != nil {
			// Non-fatal, just won't have semantic results
			semanticResults = nil
		}

		// Fuse results with RRF
		weights := config.DefaultRetrieverConfig().Weights
		fusedResults := fusion.WeightedRRF(weights, keywordResults, semanticResults, nil)

		// Limit fused results
		if len(fusedResults) > limit*2 {
			fusedResults = fusedResults[:limit*2]
		}

		// Optionally apply reranking
		if enableRerank && len(fusedResults) > 0 {
			rerankCfg := config.DefaultRerankerConfig()
			rerankCfg.Enabled = true
			rerankCfg.TopK = limit

			reranker := rerank.NewReranker(rerankCfg)

			// Build contents map from snippets
			contents := make(map[string]string)
			for _, r := range fusedResults {
				if r.Snippet != "" {
					contents[r.ID] = r.Snippet
				}
			}

			rerankResult, err := reranker.Rerank(ctx, query, fusedResults, contents)
			if err == nil {
				fusedResults = rerankResult.Results
			}
		}

		// Apply final limit
		if len(fusedResults) > limit {
			fusedResults = fusedResults[:limit]
		}

		// Build response
		response := HybridSearchV2Result{
			Query:             query,
			Results:           fusedResults,
			KeywordCount:      len(keywordResults),
			SemanticCount:     len(semanticResults),
			SymbolCount:       0, // Symbol search not implemented for v2 yet
			SemanticAvailable: semanticAvailable,
			SymbolAvailable:   false,
			Reranked:          enableRerank,
			Duration:          time.Since(start).String(),
		}

		data, err := json.Marshal(response)
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

// HybridSearchV2Result is the response format for v2 hybrid search.
type HybridSearchV2Result struct {
	Query             string             `json:"query"`
	Results           []fusion.RRFResult `json:"results"`
	KeywordCount      int                `json:"keyword_count"`
	SemanticCount     int                `json:"semantic_count"`
	SymbolCount       int                `json:"symbol_count"`
	SemanticAvailable bool               `json:"semantic_available"`
	SymbolAvailable   bool               `json:"symbol_available"`
	Reranked          bool               `json:"reranked"`
	Duration          string             `json:"duration"`
}

// openV2Indexer opens a v2 indexer for the given repository.
func openV2Indexer(repoRoot string) (*indexer.Indexer, error) {
	// Load database configuration from environment
	dbConfig := config.LoadDatabaseConfigFromEnv()
	embConfig := embedding.LoadConfigFromEnv()

	// Build indexer config
	cfg := &indexer.Config{
		DBType:            string(dbConfig.Type),
		Dimensions:        dbConfig.VectorDimensions,
		EmbeddingProvider: string(embConfig.Provider),
		EmbeddingModel:    embConfig.Model,
		OllamaURL:         embConfig.OllamaURL,
		LiteLLMURL:        embConfig.LiteLLMURL,
		LiteLLMKey:        embConfig.LiteLLMKey,
		BatchSize:         32,
		MaxWorkers:        4,
	}

	// Set database path/DSN
	if dbConfig.Type == dbpkg.DatabasePostgres {
		cfg.DSN = dbConfig.DSN
	} else {
		cfg.DBPath = filepath.Join(repoRoot, ".codetect", "index.db")
	}

	// Check if v2 index exists
	if dbConfig.Type == dbpkg.DatabaseSQLite {
		if _, err := os.Stat(cfg.DBPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("no v2 index found - run 'codetect-index index --v2' first")
		}
	}

	return indexer.New(repoRoot, cfg)
}

// createV2SemanticSearcher creates a native v2 semantic searcher from indexer components.
func createV2SemanticSearcher(idx *indexer.Indexer, repoRoot string) (*embedding.V2SemanticSearcher, error) {
	// Create embedder from environment configuration
	embedder, err := embedding.NewEmbedderFromEnv()
	if err != nil {
		return nil, fmt.Errorf("creating embedder: %w", err)
	}

	// Check if embedder is available
	if !embedder.Available() {
		return nil, fmt.Errorf("embedder not available")
	}

	// Get the cache from the indexer
	cache := idx.Cache()
	if cache == nil {
		return nil, fmt.Errorf("embedding cache not available")
	}

	// Get locations store
	locations := idx.Locations()
	if locations == nil {
		return nil, fmt.Errorf("location store not available")
	}

	// Get vector index (may be nil, searcher will use brute-force fallback)
	vectorIndex := idx.VectorIndex()

	// Create native v2 semantic searcher
	return embedding.NewV2SemanticSearcher(cache, locations, embedder, repoRoot, vectorIndex), nil
}

// searchKeywordV2 performs keyword search and returns results in fusion format.
func searchKeywordV2(ctx context.Context, query, repoRoot string, limit int) ([]fusion.Result, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	results, err := keyword.Search(query, repoRoot, limit)
	if err != nil {
		return nil, err
	}

	fusionResults := make([]fusion.Result, 0, len(results.Results))
	for _, res := range results.Results {
		fusionResults = append(fusionResults, fusion.Result{
			ID:      fmt.Sprintf("%s:%d", res.Path, res.LineStart),
			Path:    res.Path,
			Line:    res.LineStart,
			EndLine: res.LineEnd,
			Score:   float64(res.Score),
			Source:  "keyword",
			Snippet: res.Snippet,
		})
	}
	return fusionResults, nil
}

// searchSemanticV2 performs semantic search using the native v2 searcher.
func searchSemanticV2(ctx context.Context, searcher *embedding.V2SemanticSearcher, query, repoRoot string, limit int) ([]fusion.Result, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Use SearchWithSnippets to include code snippets
	response, err := searcher.SearchWithSnippets(ctx, query, limit, func(path string, start, end int) string {
		result, err := files.GetFile(filepath.Join(repoRoot, path), start, end)
		if err != nil {
			return fmt.Sprintf("[Error reading %s: %v]", path, err)
		}
		snippet := result.Content
		if len(snippet) > 500 {
			snippet = snippet[:500] + "..."
		}
		return snippet
	})
	if err != nil {
		return nil, err
	}

	if !response.Available {
		return nil, nil
	}

	fusionResults := make([]fusion.Result, 0, len(response.Results))
	for _, res := range response.Results {
		fusionResults = append(fusionResults, fusion.Result{
			ID:      fmt.Sprintf("%s:%d:%d", res.Path, res.StartLine, res.EndLine),
			Path:    res.Path,
			Line:    res.StartLine,
			EndLine: res.EndLine,
			Score:   float64(res.Score),
			Source:  "semantic",
			Snippet: res.Snippet,
			Metadata: map[string]interface{}{
				"node_type": res.NodeType,
				"node_name": res.NodeName,
				"language":  res.Language,
			},
		})
	}
	return fusionResults, nil
}

