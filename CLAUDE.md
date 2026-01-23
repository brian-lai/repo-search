# codetect

> **Workflow Methodology:** Follow `~/.claude/CLAUDE.md`

## About

codetect is a local MCP server providing fast codebase search, file retrieval, symbol navigation, and semantic search for Claude Code. It combines keyword search (ripgrep), symbol indexing (ctags), and semantic search (Ollama embeddings) to enable natural language code exploration.

## Tech Stack

- **Go 1.25+** - Primary language
- **SQLite** - Default embedded database (modernc.org/sqlite)
- **PostgreSQL + pgvector** - Optional high-performance vector backend
- **ripgrep** - Fast keyword search
- **universal-ctags** - Symbol indexing
- **Ollama** - Local embeddings for semantic search
- **MCP (Model Context Protocol)** - LLM tool integration

## Structure

```
cmd/
├── codetect/           # Main CLI entry point
├── codetect-daemon/    # Background indexing daemon
├── codetect-eval/      # Performance evaluation tool
├── codetect-index/     # Standalone indexer
└── migrate-to-postgres/# Database migration utility

internal/
├── config/            # Configuration management
├── daemon/            # Background daemon logic
├── db/                # Database adapters (SQLite, PostgreSQL)
├── embedding/         # Embedding generation (Ollama, LiteLLM)
├── mcp/               # MCP server implementation
├── registry/          # Project registry
├── search/            # Search logic (keyword, semantic, hybrid)
└── tools/             # Utility functions

context/
├── plans/             # Pre-work planning documents
├── summaries/         # Post-work reports
├── archives/          # Historical context snapshots
├── data/              # Input files, payloads, datasets
└── servers/           # MCP tool wrappers
```

## Key Files

- `cmd/codetect/main.go`: CLI entry point with command routing
- `internal/mcp/server.go`: MCP protocol server implementation
- `internal/db/sqlite.go`: SQLite adapter for symbols and embeddings
- `internal/db/postgres.go`: PostgreSQL adapter with pgvector support
- `internal/embedding/provider.go`: Embedding provider abstraction
- `internal/search/semantic.go`: Semantic search implementation
- `Makefile`: Build, test, and development commands
- `install.sh`: Interactive installation script
- `docker-compose.yml`: PostgreSQL + pgvector setup

## Conventions

- Follow standard Go conventions (`gofmt`, `golint`)
- Use table-driven tests for core logic
- Prefix unexported types/functions with lowercase
- Database migrations in `sql/` directory
- One feature per PR, squash merge to main
- Feature branches: `para/<feature-name>`
- Commit messages: Conventional commits format
- Document exported functions and types
- Benchmark performance-critical paths

## Getting Started

```bash
# Clone and install
git clone https://github.com/brian-lai/codetect.git
cd codetect
./install.sh

# Or build manually
make build
make install

# Initialize in a project
cd /path/to/your/project
codetect init      # Creates .mcp.json
codetect index     # Index symbols
codetect embed     # Optional: enable semantic search

# Start Claude Code
claude
```

## Development Commands

```bash
# Build and test
make build                    # Build binary
make test                     # Run all tests
make benchmark                # Run performance benchmarks

# Database setup
make setup-postgres           # Install pgvector extension
make test-postgres-setup      # Test database setup
make test-postgres            # PostgreSQL tests only

# Project management
make index                    # Index this repo
make embed                    # Generate embeddings
make doctor                   # Check dependencies
make clean                    # Clean build artifacts
```

## Configuration

**Environment Variables:**
- `CODETECT_EMBEDDING_PROVIDER` - `ollama` (default) or `litellm`
- `CODETECT_LITELLM_API_KEY` - API key for LiteLLM/OpenAI
- `CODETECT_DB_TYPE` - `sqlite` (default) or `postgres`
- `CODETECT_DB_DSN` - PostgreSQL connection string

**Project Config (`.codetect.yaml`):**
```yaml
db:
  type: postgres
  dsn: postgres://user:pass@localhost/codetect

embedding:
  provider: ollama
  model: nomic-embed-text
```

## MCP Tools

- `search_keyword` - Fast regex search via ripgrep
- `get_file` - File reading with line-range slicing
- `find_symbol` - Symbol lookup (functions, types, etc.)
- `list_defs_in_file` - List all definitions in a file
- `search_semantic` - Semantic search via local embeddings
- `hybrid_search` - Combined keyword + semantic search
