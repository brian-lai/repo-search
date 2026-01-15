package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"codetect/internal/config"
	"codetect/internal/db"
	"codetect/internal/mcp"
	"codetect/internal/search/symbols"
)

// RegisterSymbolTools registers the symbol-related MCP tools
func RegisterSymbolTools(server *mcp.Server) {
	registerFindSymbol(server)
	registerListDefsInFile(server)
}

func registerFindSymbol(server *mcp.Server) {
	tool := mcp.Tool{
		Name:        "find_symbol",
		Description: "Find symbol definitions (functions, types, variables, etc.) by name. Uses fuzzy matching.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"name": {
					Type:        "string",
					Description: "Symbol name to search for (supports partial matching)",
				},
				"kind": {
					Type:        "string",
					Description: "Filter by symbol kind: function, type, class, struct, interface, variable, constant",
				},
				"limit": {
					Type:        "number",
					Description: "Maximum number of results (default: 50)",
				},
			},
			Required: []string{"name"},
		},
	}

	handler := func(args map[string]any) (*mcp.ToolsCallResult, error) {
		name, ok := args["name"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("name is required")
		}

		kind := ""
		if k, ok := args["kind"].(string); ok {
			kind = k
		}

		limit := 50
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		// Get index path
		idx, err := openIndex()
		if err != nil {
			return &mcp.ToolsCallResult{
				Content: []mcp.Content{{
					Type: "text",
					Text: fmt.Sprintf(`{"available": false, "error": %q}`, err.Error()),
				}},
			}, nil
		}
		defer idx.Close()

		// Search for symbols
		syms, err := idx.FindSymbol(name, kind, limit)
		if err != nil {
			return nil, fmt.Errorf("searching symbols: %w", err)
		}

		result := symbols.FindSymbolResult{
			Symbols: syms,
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

func registerListDefsInFile(server *mcp.Server) {
	tool := mcp.Tool{
		Name:        "list_defs_in_file",
		Description: "List all symbol definitions in a specific file. Returns functions, types, variables, etc.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"path": {
					Type:        "string",
					Description: "File path to list definitions for",
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

		// Get index
		idx, err := openIndex()
		if err != nil {
			return &mcp.ToolsCallResult{
				Content: []mcp.Content{{
					Type: "text",
					Text: fmt.Sprintf(`{"available": false, "error": %q}`, err.Error()),
				}},
			}, nil
		}
		defer idx.Close()

		// Get symbols in file
		syms, err := idx.ListDefsInFile(path)
		if err != nil {
			return nil, fmt.Errorf("listing symbols: %w", err)
		}

		result := symbols.ListDefsResult{
			Path:    path,
			Symbols: syms,
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

// openIndex opens the symbol index for the current working directory.
// Uses database configuration from environment variables, supporting both
// SQLite (default) and PostgreSQL backends.
func openIndex() (*symbols.Index, error) {
	// Load database configuration from environment
	dbConfig := config.LoadDatabaseConfigFromEnv()

	// For SQLite, use path relative to current working directory
	if dbConfig.Type == db.DatabaseSQLite {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getting working directory: %w", err)
		}

		dbPath := filepath.Join(cwd, ".codetect", "symbols.db")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("no symbol index found - run 'make index' first")
		}
		dbConfig.Path = dbPath
	}

	// Convert to db.Config and open with config-aware constructor
	cfg := dbConfig.ToDBConfig()
	return symbols.NewIndexWithConfig(cfg)
}
