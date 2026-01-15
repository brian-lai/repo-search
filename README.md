# codetect

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
git clone https://github.com/brian-lai/codetect.git
cd codetect
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
codetect init      # Creates .mcp.json
codetect index     # Index symbols
codetect embed     # Optional: enable semantic search
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
codetect init      # Initialize in current directory
codetect index     # Index symbols
codetect embed     # Generate embeddings
codetect doctor    # Check dependencies
codetect stats     # Show index statistics
codetect migrate   # Discover existing indexes and register them
codetect update    # Update to latest version
codetect help      # Show all commands
```

### Daemon Commands

```bash
codetect daemon start    # Start background indexing daemon
codetect daemon stop     # Stop daemon
codetect daemon status   # Show daemon status
codetect daemon logs     # View daemon logs
```

### Registry Commands

```bash
codetect registry list     # List registered projects
codetect registry add      # Add current project to registry
codetect registry remove   # Remove a project from registry
codetect registry stats    # Show aggregate statistics
```

### Evaluation Commands

```bash
codetect-eval run --repo <path>     # Run performance evaluation
codetect-eval report                # Show latest results
codetect-eval list                  # List available test cases
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

### Embedding Provider

Configure embedding provider via environment variables:

```bash
# Use Ollama (default)
export CODETECT_EMBEDDING_PROVIDER=ollama

# Or use LiteLLM/OpenAI
export CODETECT_EMBEDDING_PROVIDER=litellm
export CODETECT_LITELLM_API_KEY=sk-...
```

### Database Backend

codetect supports two database backends for vector search:

| Backend | Best For | Performance | Setup |
|---------|----------|-------------|-------|
| **SQLite** (default) | Small-medium projects (< 10K files) | Fast for small datasets | Zero config |
| **PostgreSQL + pgvector** | Large projects (> 10K files) | 60x faster at scale | Docker or manual install |

**Quick Start with PostgreSQL:**

```bash
# Start PostgreSQL with Docker
docker-compose up -d

# Configure codetect
export CODETECT_DB_TYPE=postgres
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5432/codetect?sslmode=disable"

# Index and embed as usual
codetect index
codetect embed
```

**Performance Comparison:**

| Dataset Size | SQLite | PostgreSQL | Speedup |
|--------------|--------|------------|---------|
| 100 vectors  | 77 μs  | 603 μs     | 0.13x (slower) |
| 1,000 vectors | 1.19 ms | 745 μs   | 1.6x faster |
| 10,000 vectors | 58.1 ms | 963 μs  | **60x faster** |

For large codebases, PostgreSQL + pgvector provides massive performance improvements through HNSW indexing. See [PostgreSQL Setup Guide](docs/postgres-setup.md) for detailed installation and migration instructions.

See [Installation Guide](docs/installation.md#configuration) for all configuration options.

## Performance Evaluation

codetect includes an evaluation tool to measure the performance improvement of MCP tools vs. standard CLI tools (grep, find, etc.) when working with Claude Code.

```bash
# Run evaluations on any repository
codetect-eval run --repo /path/to/project

# View results
codetect-eval report
```

Eval cases are stored in `.codetect/evals/cases/` (auto-added to .gitignore) so you can version-control test cases while keeping results local.

See [Evaluation Guide](docs/evaluation.md) for detailed documentation on creating test cases, understanding metrics, and best practices.

## Documentation

- [Installation Guide](docs/installation.md) - Detailed setup and configuration
- [PostgreSQL Setup Guide](docs/postgres-setup.md) - PostgreSQL + pgvector for scalable vector search
- [Benchmarks](docs/benchmarks.md) - Vector search performance benchmarks and methodology
- [Evaluation Guide](docs/evaluation.md) - Performance testing and benchmarking
- [Architecture](docs/architecture.md) - Internal design and data flow
- [MCP Compatibility](docs/mcp-compatibility.md) - Supported tools and multi-tool roadmap

## Compatibility

codetect uses [MCP (Model Context Protocol)](https://modelcontextprotocol.io/), an open standard for LLM tool integration.

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
- [x] PostgreSQL + pgvector support for scalable vector search
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
