# repo-search

A local MCP server providing fast codebase search, file retrieval, symbol navigation, and semantic search for Claude Code.

## Features

- **`search_keyword`** - Fast regex search powered by ripgrep
- **`get_file`** - File reading with optional line-range slicing
- **`find_symbol`** - Symbol lookup (functions, types, etc.) via ctags + SQLite
- **`list_defs_in_file`** - List all definitions in a file
- **`search_semantic`** - Semantic code search via local embeddings (Ollama)
- **`hybrid_search`** - Combined keyword + semantic search

## Quick Start

```bash
# Clone and run interactive installer
git clone https://github.com/brian-lai/repo-search.git
cd repo-search
./install.sh
```

The installer will:
- ✓ Check for required dependencies (Go, ripgrep)
- ✓ Offer to install ctags automatically for symbol indexing
- ✓ Guide you through Ollama setup for semantic search (with prominent warnings if missing)
- ✓ Build and install globally to `~/.local/bin`
- ✓ Configure your shell PATH automatically

Then in any project:

```bash
cd /path/to/your/project
repo-search init      # Creates .mcp.json
repo-search index     # Index symbols
repo-search embed     # Optional: enable semantic search
claude                # Start Claude Code
```

See [Installation Guide](docs/installation.md) for detailed setup instructions.

## Requirements

| Dependency | Required | Purpose |
|------------|----------|---------|
| Go 1.21+ | Yes | Building from source |
| [ripgrep](https://github.com/BurntSushi/ripgrep) | Yes | Keyword search |
| [universal-ctags](https://github.com/universal-ctags/ctags) | No | Symbol indexing |
| [Ollama](https://ollama.ai) | No | Semantic search |

## CLI Commands

### Main Commands

```bash
repo-search init      # Initialize in current directory
repo-search index     # Index symbols
repo-search embed     # Generate embeddings
repo-search doctor    # Check dependencies
repo-search stats     # Show index statistics
repo-search migrate   # Discover existing indexes and register them
repo-search update    # Update to latest version
repo-search help      # Show all commands
```

### Daemon Commands

```bash
repo-search daemon start    # Start background indexing daemon
repo-search daemon stop     # Stop daemon
repo-search daemon status   # Show daemon status
repo-search daemon logs     # View daemon logs
```

### Registry Commands

```bash
repo-search registry list     # List registered projects
repo-search registry add      # Add current project to registry
repo-search registry remove   # Remove a project from registry
repo-search registry stats    # Show aggregate statistics
```

### Evaluation Commands

```bash
repo-search-eval run --repo <path>     # Run performance evaluation
repo-search-eval run --parallel 4      # Run with parallel execution (faster)
repo-search-eval report                # Show latest results
repo-search-eval list                  # List available test cases
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

Configure embedding provider via environment variables:

```bash
# Use Ollama (default)
export REPO_SEARCH_EMBEDDING_PROVIDER=ollama

# Or use LiteLLM/OpenAI
export REPO_SEARCH_EMBEDDING_PROVIDER=litellm
export REPO_SEARCH_LITELLM_API_KEY=sk-...
```

See [Installation Guide](docs/installation.md#configuration) for all options.

## Performance Evaluation

repo-search includes an evaluation tool to measure the performance improvement of MCP tools vs. standard CLI tools (grep, find, etc.) when working with Claude Code.

```bash
# Run evaluations on any repository
repo-search-eval run --repo /path/to/project

# Run with parallel execution for faster results
repo-search-eval run --repo /path/to/project --parallel 4

# View results
repo-search-eval report
```

Eval cases are stored in `.repo_search/evals/cases/` (auto-added to .gitignore) so you can version-control test cases while keeping results local.

See [Evaluation Guide](docs/evaluation.md) for detailed documentation on creating test cases, understanding metrics, and best practices.

## Documentation

- [Installation Guide](docs/installation.md) - Detailed setup and configuration
- [Evaluation Guide](docs/evaluation.md) - Performance testing and benchmarking
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
- [x] Background indexing daemon
- [x] Project registry
- [x] Evaluation framework
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
