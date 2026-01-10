# repo-search

A local MCP server providing fast codebase search, file retrieval, symbol navigation, and semantic search for Claude Code.

## Features

- **`search_keyword`** - Fast regex search powered by ripgrep
- **`get_file`** - File reading with optional line-range slicing
- **`find_symbol`** - Symbol lookup (functions, types, etc.) via ctags + SQLite
- **`list_defs_in_file`** - List all definitions in a file
- **`search_semantic`** - Semantic code search via local embeddings (Ollama)
- **`hybrid_search`** - Combined keyword + semantic search

## Installation

```bash
# Clone and build
git clone https://github.com/brian-lai/repo-search.git
cd repo-search
make install          # Installs to ~/.local/bin
```

Make sure `~/.local/bin` is in your PATH:
```bash
export PATH="$HOME/.local/bin:$PATH"
```

## Quick Start

In any project:

```bash
cd /path/to/your/project
repo-search init      # Interactive setup (creates .mcp.json and .env.repo-search)
repo-search index     # Index symbols
repo-search embed     # Generate embeddings
claude                # Start Claude Code
```

The `repo-search init` command will prompt you to configure an embedding provider (Ollama, LiteLLM, LMStudio, or none) for that project.

See [Installation Guide](docs/installation.md) for detailed setup instructions.

## Requirements

| Dependency | Required | Purpose |
|------------|----------|---------|
| Go 1.21+ | Yes | Building from source |
| [ripgrep](https://github.com/BurntSushi/ripgrep) | Yes | Keyword search |
| [universal-ctags](https://github.com/universal-ctags/ctags) | No | Symbol indexing |
| [Ollama](https://ollama.ai) | No | Semantic search |

## CLI Commands

```bash
repo-search init      # Initialize in current directory
repo-search index     # Index symbols
repo-search embed     # Generate embeddings
repo-search doctor    # Check dependencies
repo-search stats     # Show index statistics
repo-search update    # Update to latest version
repo-search help      # Show all commands
```

## MCP Tools

### search_keyword

Search for patterns using ripgrep:

```json
{"query": "func main", "top_k": 5}
```

### get_file

Read file contents with optional line range:

```json
{"path": "main.go", "start_line": 10, "end_line": 20}
```

### find_symbol

Find symbol definitions by name:

```json
{"name": "Server", "kind": "struct", "limit": 50}
```

### list_defs_in_file

List all symbols in a file:

```json
{"path": "internal/mcp/server.go"}
```

### search_semantic

Search using natural language (requires Ollama):

```json
{"query": "error handling logic", "limit": 10}
```

### hybrid_search

Combined keyword + semantic search:

```json
{"query": "authentication", "keyword_limit": 20, "semantic_limit": 10}
```

## Configuration

Each project has its own `.env.repo-search` configuration file created by `repo-search init`. This file configures the embedding provider for that project.

Example configurations:

```bash
# Ollama (default)
export REPO_SEARCH_EMBEDDING_PROVIDER="ollama"
export REPO_SEARCH_OLLAMA_URL="http://localhost:11434"
export REPO_SEARCH_EMBEDDING_MODEL="nomic-embed-text"

# LMStudio
export REPO_SEARCH_EMBEDDING_PROVIDER="lmstudio"
export REPO_SEARCH_LMSTUDIO_URL="http://localhost:1234"
export REPO_SEARCH_EMBEDDING_MODEL="nomic-embed-code-GGUF"

# LiteLLM (OpenAI, Azure, etc.)
export REPO_SEARCH_EMBEDDING_PROVIDER="litellm"
export REPO_SEARCH_LITELLM_URL="http://localhost:4000"
export REPO_SEARCH_LITELLM_API_KEY="sk-..."
export REPO_SEARCH_EMBEDDING_MODEL="text-embedding-3-small"

# Disabled
export REPO_SEARCH_EMBEDDING_PROVIDER="off"
```

You can also create a global config at `~/.config/repo-search/config.env` which will be used as a fallback for projects without a local config.

See [Installation Guide](docs/installation.md#configuration) for all options.

## Documentation

- [Installation Guide](docs/installation.md) - Detailed setup and configuration
- [Architecture](docs/architecture.md) - Internal design and data flow
- [MCP Compatibility](docs/mcp-compatibility.md) - Supported tools and multi-tool roadmap

## Compatibility

repo-search uses [MCP (Model Context Protocol)](https://modelcontextprotocol.io/), an open standard for LLM tool integration.

| Tool | Support |
|------|---------|
| Claude Code | Fully supported |
| Cursor | Should work |
| Cline / Continue | Should work |
| Zed | Should work |

See [MCP Compatibility](docs/mcp-compatibility.md) for details and roadmap for non-MCP tools.

## Roadmap

- [x] MCP stdio server
- [x] Keyword search via ripgrep
- [x] Symbol indexing via ctags
- [x] Semantic search via Ollama
- [x] Hybrid search
- [x] Global installation
- [ ] Background indexing daemon
- [ ] Project registry
- [ ] HTTP API for non-MCP tools
- [ ] CLI query mode

## Development

```bash
# Build
make build

# Run tests
make test

# Index this repo
make index && make embed

# Check dependencies
make doctor
```

## License

MIT
