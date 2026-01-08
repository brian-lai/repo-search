# repo-search

A local MCP server providing fast codebase search, file retrieval, symbol navigation, and semantic search for Claude Code.

## Overview

`repo-search` is a Go-based MCP (Model Context Protocol) server that gives Claude Code fast, grounded access to your codebase via:

- **`search_keyword`**: Fast regex search powered by ripgrep
- **`get_file`**: File reading with optional line-range slicing
- **`find_symbol`**: Symbol lookup (functions, types, etc.) via ctags + SQLite
- **`list_defs_in_file`**: List all definitions in a file
- **`search_semantic`**: Semantic code search via local embeddings (Ollama)
- **`hybrid_search`**: Combined keyword + semantic search

## Requirements

### Required
- Go 1.21+
- [ripgrep](https://github.com/BurntSushi/ripgrep) (`rg`)

### Optional (for symbol indexing)
- [universal-ctags](https://github.com/universal-ctags/ctags)

### Optional (for semantic search)
- [Ollama](https://ollama.ai) with `nomic-embed-text` model

## Installation

```bash
# Clone the repo
git clone https://github.com/yourorg/repo-search.git
cd repo-search

# Check dependencies
make doctor

# Build
make build

# Index symbols (requires universal-ctags)
make index

# Generate embeddings (requires Ollama)
make embed
```

### Installing ctags

```bash
# macOS
brew install universal-ctags

# Ubuntu/Debian
apt install universal-ctags

# Fedora
dnf install ctags
```

### Installing Ollama

```bash
# macOS/Linux - install from website
# https://ollama.ai

# Pull the embedding model
ollama pull nomic-embed-text
```

## Usage

### With Claude Code

The `.mcp.json` file registers `repo-search` as an MCP server. When you enter this repository with Claude Code, the server is automatically available.

**Using the wrapper script:**

```bash
./bin/claude
```

This runs indexing and then launches Claude Code.

**Or use Claude Code directly** - the MCP server will be started automatically via `.mcp.json`.

### Manual Testing

Test the MCP server directly via stdin/stdout:

```bash
# Build and start the server
make build
./dist/repo-search

# Then send JSON-RPC requests (one per line):
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_keyword","arguments":{"query":"func main","top_k":5}}}
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"find_symbol","arguments":{"name":"Server","kind":"struct"}}}
```

## MCP Tools

### search_keyword

Search for patterns in the codebase using ripgrep.

**Input:**
```json
{
  "query": "string (regex pattern)",
  "top_k": "number (default: 20)"
}
```

**Output:**
```json
{
  "results": [
    {
      "path": "main.go",
      "line_start": 10,
      "line_end": 10,
      "snippet": "func main() {",
      "score": 100
    }
  ]
}
```

### get_file

Read file contents with optional line-range slicing.

**Input:**
```json
{
  "path": "string (file path)",
  "start_line": "number (1-indexed, optional)",
  "end_line": "number (1-indexed, optional)"
}
```

**Output:**
```json
{
  "path": "main.go",
  "content": "package main\n\nimport ..."
}
```

### find_symbol

Find symbol definitions by name. Supports fuzzy matching.

**Input:**
```json
{
  "name": "string (symbol name)",
  "kind": "string (optional: function, type, struct, interface, variable, constant)",
  "limit": "number (default: 50)"
}
```

**Output:**
```json
{
  "symbols": [
    {
      "name": "Server",
      "kind": "struct",
      "path": "internal/mcp/server.go",
      "line": 18,
      "scope": "package:mcp"
    }
  ]
}
```

### list_defs_in_file

List all symbol definitions in a specific file.

**Input:**
```json
{
  "path": "string (file path)"
}
```

**Output:**
```json
{
  "path": "internal/mcp/server.go",
  "symbols": [
    {"name": "Server", "kind": "struct", "line": 18},
    {"name": "NewServer", "kind": "function", "line": 27},
    {"name": "Run", "kind": "function", "line": 44}
  ]
}
```

### search_semantic

Search for code semantically similar to a query. Requires Ollama.

**Input:**
```json
{
  "query": "string (natural language query)",
  "limit": "number (default: 10)"
}
```

**Output:**
```json
{
  "available": true,
  "results": [
    {
      "path": "internal/embedding/math.go",
      "start_line": 9,
      "end_line": 28,
      "snippet": "func CosineSimilarity...",
      "score": 0.72
    }
  ]
}
```

### hybrid_search

Combined keyword + semantic search with weighted scoring.

**Input:**
```json
{
  "query": "string (search query)",
  "keyword_limit": "number (default: 20)",
  "semantic_limit": "number (default: 10)"
}
```

**Output:**
```json
{
  "results": [
    {
      "path": "file.go",
      "start_line": 1,
      "end_line": 10,
      "snippet": "...",
      "score": 0.8,
      "source": "both"
    }
  ],
  "keyword_count": 5,
  "semantic_count": 3,
  "semantic_enabled": true
}
```

## Makefile Targets

| Target           | Description                              |
|------------------|------------------------------------------|
| `make build`     | Build both binaries to `dist/`           |
| `make mcp`       | Build and run the MCP server             |
| `make index`     | Index symbols (requires ctags)           |
| `make embed`     | Generate embeddings (requires Ollama)    |
| `make index-all` | Run both index and embed                 |
| `make stats`     | Show index statistics                    |
| `make doctor`    | Check all dependencies                   |
| `make test`      | Run tests                                |
| `make clean`     | Remove build artifacts and index         |

## Architecture

```
repo-search/
├── cmd/
│   ├── repo-search/          # MCP stdio server
│   └── repo-search-index/    # Symbol indexer + embedding CLI
├── internal/
│   ├── mcp/                  # JSON-RPC over stdio transport
│   ├── embedding/            # Ollama embeddings + vector search
│   │   ├── ollama.go         # Ollama HTTP client
│   │   ├── chunker.go        # Code chunking with symbol boundaries
│   │   ├── store.go          # SQLite embedding storage
│   │   ├── math.go           # Vector math (cosine similarity)
│   │   └── search.go         # Semantic search
│   ├── search/
│   │   ├── keyword/          # ripgrep integration
│   │   ├── files/            # file read + slicing
│   │   ├── symbols/          # ctags + SQLite symbol index
│   │   └── hybrid/           # Combined keyword + semantic search
│   └── tools/                # MCP tool definitions
├── bin/
│   └── claude                # wrapper script
├── .mcp.json                 # MCP server registration
├── .repo_search/             # Index storage (gitignored)
│   └── symbols.db            # SQLite database (symbols + embeddings)
└── Makefile
```

## Roadmap

### Phase 1 (Complete)
- [x] MCP stdio server
- [x] `search_keyword` via ripgrep
- [x] `get_file` with line slicing
- [x] `.mcp.json` project registration
- [x] `bin/claude` wrapper

### Phase 2 (Complete)
- [x] Symbol indexing via universal-ctags
- [x] SQLite-backed symbol table
- [x] `find_symbol` MCP tool
- [x] `list_defs_in_file` MCP tool
- [x] Incremental indexing (mtime-based)
- [x] Graceful degradation when ctags not available

### Phase 3 (Complete)
- [x] Ollama embedding client (`nomic-embed-text` model)
- [x] Symbol-aware code chunking with overlap
- [x] SQLite embedding storage with content hashing
- [x] Cosine similarity vector search
- [x] `search_semantic` MCP tool
- [x] `hybrid_search` MCP tool
- [x] Incremental embedding (skip unchanged chunks)
- [x] Graceful degradation when Ollama unavailable

## License

MIT
