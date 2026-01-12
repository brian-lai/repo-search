# Architecture

This document describes the internal architecture of repo-search.

## Overview

repo-search is a Go-based MCP server that provides code search capabilities via stdio transport. It combines multiple search strategies:

1. **Keyword search** - Fast regex matching via ripgrep
2. **Symbol search** - Structured code navigation via ctags + SQLite
3. **Semantic search** - Natural language queries via embeddings
4. **Hybrid search** - Combined keyword + semantic with ranked results

## Directory Structure

```
repo-search/
├── cmd/
│   ├── repo-search/           # CLI entry point & MCP stdio server
│   ├── repo-search-index/     # Symbol indexer & embedding generator
│   ├── repo-search-daemon/    # Background indexing daemon
│   └── repo-search-eval/      # Evaluation framework for testing
├── internal/
│   ├── mcp/                   # MCP protocol implementation
│   │   ├── server.go          # JSON-RPC server over stdio
│   │   └── types.go           # MCP protocol types
│   ├── embedding/             # Embedding & vector search
│   │   ├── embedder.go        # Embedder interface
│   │   ├── provider.go        # Provider factory & config
│   │   ├── ollama.go          # Ollama HTTP client
│   │   ├── litellm.go         # LiteLLM/OpenAI-compatible client
│   │   ├── chunker.go         # Code chunking with symbol awareness
│   │   ├── store.go           # SQLite embedding storage
│   │   ├── math.go            # Vector math (cosine similarity)
│   │   └── search.go          # Semantic search implementation
│   ├── search/
│   │   ├── keyword/           # ripgrep integration
│   │   │   └── keyword.go     # Regex search via rg
│   │   ├── files/             # File operations
│   │   │   └── files.go       # Read with line slicing
│   │   ├── symbols/           # Symbol indexing
│   │   │   ├── ctags.go       # ctags parser
│   │   │   ├── index.go       # SQLite symbol index
│   │   │   └── schema.go      # Database schema
│   │   └── hybrid/            # Combined search
│   │       └── hybrid.go      # Keyword + semantic fusion
│   ├── tools/                 # MCP tool definitions
│   │   ├── tools.go           # Tool registration
│   │   ├── symbols.go         # find_symbol, list_defs_in_file
│   │   └── semantic.go        # search_semantic, hybrid_search
│   ├── daemon/                # Background daemon
│   │   ├── daemon.go          # Daemon process management
│   │   └── ipc.go             # Inter-process communication
│   └── registry/              # Project registry
│       └── registry.go        # Track indexed projects
├── evals/                     # Evaluation test cases and results
├── scripts/
│   └── repo-search-wrapper.sh # CLI wrapper for global install
├── templates/
│   └── mcp.json               # Template for new projects
└── docs/                      # Documentation
```

## Component Details

### MCP Server (`internal/mcp/`)

The MCP server implements JSON-RPC 2.0 over stdio:

```
stdin → JSON-RPC Parser → Method Router → Tool Handler → JSON-RPC Response → stdout
```

Key methods:
- `initialize` - Protocol handshake
- `tools/list` - Enumerate available tools
- `tools/call` - Execute a tool

### Keyword Search (`internal/search/keyword/`)

Wraps ripgrep for fast regex search:

```
Query → rg subprocess → Parse JSON output → Ranked results
```

Features:
- Respects `.gitignore`
- Configurable result limit
- Returns file path, line number, and snippet

### Symbol Index (`internal/search/symbols/`)

Two-stage indexing via ctags and SQLite:

```
Source files → ctags → JSON tags → SQLite index
```

Schema:
```sql
CREATE TABLE symbols (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    path TEXT NOT NULL,
    line INTEGER NOT NULL,
    scope TEXT,
    signature TEXT
);
```

Features:
- Fuzzy name matching
- Kind filtering (function, type, struct, etc.)
- Incremental updates via mtime tracking

### Embedding System (`internal/embedding/`)

#### Provider Abstraction

```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dimensions() int
    ModelName() string
}
```

Implementations:
- `OllamaEmbedder` - Local Ollama server
- `LiteLLMEmbedder` - OpenAI-compatible API

#### Code Chunking

The chunker splits code into embeddable chunks:

```
Source file → Parse symbols → Split at boundaries → Overlap chunks
```

Strategy:
- Chunk at function/type boundaries when possible
- Target ~500 tokens per chunk
- 50-token overlap between chunks
- Preserve context with file path prefix

#### Vector Storage

SQLite with blob storage for embeddings:

```sql
CREATE TABLE embeddings (
    id INTEGER PRIMARY KEY,
    path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    content_hash TEXT NOT NULL,
    embedding BLOB NOT NULL
);
```

Features:
- Content hashing for incremental updates
- Skip unchanged chunks on re-embed
- Efficient blob storage for vectors

#### Similarity Search

```
Query → Embed query → Cosine similarity vs all chunks → Top-K results
```

Currently brute-force search (sufficient for <100K chunks). Future: consider HNSW or IVF indexing for larger codebases.

### Hybrid Search (`internal/search/hybrid/`)

Combines keyword and semantic results:

```
Query → [Keyword search, Semantic search] → Merge & dedupe → Weighted ranking
```

Ranking formula:
- Keyword matches: score based on match quality
- Semantic matches: cosine similarity (0-1)
- Combined: weighted average with deduplication

## Data Flow

### Indexing Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ Source Code │ ──▶ │   ctags     │ ──▶ │   SQLite    │
│   Files     │     │   Parser    │     │   Symbols   │
└─────────────┘     └─────────────┘     └─────────────┘
       │
       ▼
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Chunker   │ ──▶ │  Embedder   │ ──▶ │   SQLite    │
│             │     │  (Ollama)   │     │  Embeddings │
└─────────────┘     └─────────────┘     └─────────────┘
```

### Query Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ MCP Request │ ──▶ │   Router    │ ──▶ │ Tool Handler│
└─────────────┘     └─────────────┘     └─────────────┘
                                               │
                    ┌──────────────────────────┼──────────────────────────┐
                    ▼                          ▼                          ▼
             ┌─────────────┐           ┌─────────────┐           ┌─────────────┐
             │   ripgrep   │           │   SQLite    │           │  Embedding  │
             │   Search    │           │   Symbols   │           │   Search    │
             └─────────────┘           └─────────────┘           └─────────────┘
                    │                          │                          │
                    └──────────────────────────┼──────────────────────────┘
                                               ▼
                                        ┌─────────────┐
                                        │ MCP Response│
                                        └─────────────┘
```

## Storage

All indexes are stored in `.repo_search/` at the project root:

```
.repo_search/
└── symbols.db        # SQLite database containing:
    ├── symbols       # ctags-derived symbol table
    ├── embeddings    # Vector embeddings for chunks
    └── metadata      # Index timestamps, config
```

This directory should be added to `.gitignore`.

## Graceful Degradation

repo-search is designed to work with partial dependencies:

| Dependency | If Missing |
|------------|------------|
| ripgrep | `search_keyword` fails (required) |
| ctags | `find_symbol`, `list_defs_in_file` unavailable |
| Ollama/LiteLLM | `search_semantic`, `hybrid_search` return `available: false` |

The MCP server always starts; tools report availability in their responses.

## Background Daemon (`internal/daemon/`)

The daemon provides automatic re-indexing when files change:

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  fsnotify   │ ──▶ │   Daemon    │ ──▶ │  Re-index   │
│   Watcher   │     │   Process   │     │   Changed   │
└─────────────┘     └─────────────┘     └─────────────┘
```

Features:
- File system watching via fsnotify
- Debounced re-indexing to avoid excessive updates
- IPC for daemon control (start/stop/status)
- Respects `.gitignore` patterns
- PID file and Unix socket for process management

Commands:
- `repo-search-daemon start` - Start the daemon
- `repo-search-daemon stop` - Stop the daemon
- `repo-search-daemon status` - Show daemon status

## Project Registry (`internal/registry/`)

Central tracking of all indexed projects:

```
~/.repo_search/
└── registry.json    # Global project registry
    ├── projects     # Registered project paths
    ├── settings     # Auto-watch configuration
    └── stats        # Index statistics per project
```

Features:
- JSON-based storage for portability
- Per-project index statistics (symbol count, embedding count, DB size)
- Watch enabled/disabled flags
- Last indexed timestamp tracking
- Global settings for auto-watch and debounce

## Evaluation Framework (`cmd/repo-search-eval/`, `evals/`)

Testing framework for comparing MCP vs non-MCP performance:

```
Test Cases → Runner → [MCP Search, Direct Search] → Validator → Report
```

Features:
- JSONL-based test case format
- Categories: search, navigate, understand
- Per-repo test case storage in `.repo_search/evals/cases/`
- Automated validation of results
- Performance comparison reports

Commands:
- `repo-search-eval run` - Run evaluation tests
- `repo-search-eval report` - Display saved reports
- `repo-search-eval list` - List available test cases

## Future Architecture (Planned)

### HTTP API Mode

REST interface for non-MCP tools:

```
HTTP Request → Router → Same tool handlers → JSON Response
```
