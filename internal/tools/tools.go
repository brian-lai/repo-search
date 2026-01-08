package tools

import (
	"encoding/json"
	"fmt"
	"os"

	"repo-search/internal/mcp"
	"repo-search/internal/search/files"
	"repo-search/internal/search/keyword"
)

// RegisterAll registers all available tools on the MCP server
func RegisterAll(server *mcp.Server) {
	registerSearchKeyword(server)
	registerGetFile(server)
	RegisterSymbolTools(server)
}

func registerSearchKeyword(server *mcp.Server) {
	tool := mcp.Tool{
		Name:        "search_keyword",
		Description: "Search for a keyword/pattern in the codebase using ripgrep. Returns matching files with line numbers and snippets.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"query": {
					Type:        "string",
					Description: "The search query (supports regex)",
				},
				"top_k": {
					Type:        "number",
					Description: "Maximum number of results to return (default: 20)",
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

		topK := 20
		if tk, ok := args["top_k"].(float64); ok {
			topK = int(tk)
		}

		// Get current working directory as root
		root, err := os.Getwd()
		if err != nil {
			root = "."
		}

		result, err := keyword.Search(query, root, topK)
		if err != nil {
			return nil, err
		}

		// Serialize results to JSON
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

func registerGetFile(server *mcp.Server) {
	tool := mcp.Tool{
		Name:        "get_file",
		Description: "Read the contents of a file, optionally specifying a line range.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"path": {
					Type:        "string",
					Description: "Path to the file (relative or absolute)",
				},
				"start_line": {
					Type:        "number",
					Description: "First line to read (1-indexed, inclusive). Omit to start from beginning.",
				},
				"end_line": {
					Type:        "number",
					Description: "Last line to read (1-indexed, inclusive). Omit to read to end.",
				},
			},
			Required: []string{"path"},
		},
	}

	handler := func(args map[string]any) (*mcp.ToolsCallResult, error) {
		path, ok := args["path"].(string)
		if !ok || path == "" {
			return nil, fmt.Errorf("path is required")
		}

		startLine := 0
		if sl, ok := args["start_line"].(float64); ok {
			startLine = int(sl)
		}

		endLine := 0
		if el, ok := args["end_line"].(float64); ok {
			endLine = int(el)
		}

		result, err := files.GetFile(path, startLine, endLine)
		if err != nil {
			return nil, err
		}

		// Serialize results to JSON
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
