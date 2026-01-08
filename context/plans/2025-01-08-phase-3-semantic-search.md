# Phase 3 Plan: Semantic Search (Optional, Pluggable)

## Overview

Add conceptual retrieval when keywords and symbols aren't enough. Powered by local Ollama embeddings, with graceful degradation when Ollama is not available.

## Goals

- Answer conceptual queries like "where do we handle authentication errors?"
- Support large or poorly-named codebases where keyword search fails
- **Local-only** — no cloud embedding services
- **Optional** — feature auto-enables when Ollama is detected
- Zero impact on Phase 1/2 functionality when disabled

---

## Design Principles

1. **Graceful Degradation**: If Ollama isn't available, semantic search returns "unavailable" without errors
2. **Lazy Indexing**: Embeddings generated on first semantic query or explicit `make embed`
3. **Chunking Strategy**: Function-level chunks preferred over arbitrary line splits
4. **Storage**: SQLite with vector stored as JSON array (simple, portable)
5. **No External Dependencies**: Pure Go implementation for vector math

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     Claude Code                          │
└─────────────────────┬───────────────────────────────────┘
                      │ MCP
┌─────────────────────▼───────────────────────────────────┐
│                   repo-search                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────┐  │
│  │  keyword    │  │   symbol    │  │    semantic     │  │
│  │  (ripgrep)  │  │  (ctags+db) │  │ (ollama+vector) │  │
│  └─────────────┘  └─────────────┘  └────────┬────────┘  │
└─────────────────────────────────────────────┼───────────┘
                                              │ HTTP
                                    ┌─────────▼─────────┐
                                    │   Ollama (local)  │
                                    │  (nomic-embed-*) │
                                    └───────────────────┘
```

---

## Implementation Tasks

### 1. Ollama Client

**File:** `internal/embedding/ollama.go`

```go
type OllamaClient struct {
    baseURL string
    model   string
}

func (c *OllamaClient) Available() bool
func (c *OllamaClient) Embed(text string) ([]float32, error)
func (c *OllamaClient) EmbedBatch(texts []string) ([][]float32, error)
```

- Default URL: `http://localhost:11434`
- Default model: `nomic-embed-text` (768 dimensions, good for code)
- Timeout: 30s per request
- Batch size: 32 texts per request

---

### 2. Code Chunker

**File:** `internal/embedding/chunker.go`

Chunk strategy (in priority order):
1. **Function-level** — Use ctags symbol boundaries if available
2. **Semantic blocks** — Split on blank lines + indentation patterns
3. **Fixed-size fallback** — 50-line chunks with 10-line overlap

```go
type Chunk struct {
    Path      string
    StartLine int
    EndLine   int
    Content   string
    Kind      string // "function", "class", "block", "fixed"
}

func ChunkFile(path string, symbols []Symbol) ([]Chunk, error)
```

---

### 3. Vector Storage (SQLite)

**File:** `internal/embedding/store.go`

Schema extension to `.repo_search/symbols.db`:

```sql
CREATE TABLE IF NOT EXISTS embeddings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    content_hash TEXT NOT NULL,       -- SHA256 of chunk content
    embedding TEXT NOT NULL,          -- JSON array of float32
    model TEXT NOT NULL,              -- e.g., "nomic-embed-text"
    created_at INTEGER NOT NULL,
    UNIQUE(path, start_line, end_line, model)
);

CREATE INDEX idx_embeddings_path ON embeddings(path);
CREATE INDEX idx_embeddings_hash ON embeddings(content_hash);
```

Vector stored as JSON: `[0.123, -0.456, ...]` (simple, debuggable)

---

### 4. Vector Search

**File:** `internal/embedding/search.go`

```go
type SemanticResult struct {
    Path      string  `json:"path"`
    StartLine int     `json:"start_line"`
    EndLine   int     `json:"end_line"`
    Snippet   string  `json:"snippet"`
    Score     float32 `json:"score"`     // cosine similarity
}

func SearchSemantic(query string, limit int) ([]SemanticResult, error)
```

Algorithm:
1. Embed query via Ollama
2. Load all embeddings from SQLite
3. Compute cosine similarity for each
4. Return top-k results

Performance note: For MVP, brute-force is acceptable up to ~10k chunks. Future optimization: use FAISS bindings or SQLite FTS5 with vectors.

---

### 5. Hybrid Search

**File:** `internal/search/hybrid/hybrid.go`

```go
type HybridResult struct {
    Path      string  `json:"path"`
    LineStart int     `json:"line_start"`
    LineEnd   int     `json:"line_end"`
    Snippet   string  `json:"snippet"`
    Score     float32 `json:"score"`
    Sources   []string `json:"sources"` // ["keyword", "symbol", "semantic"]
}

func HybridSearch(query string, limit int) ([]HybridResult, error)
```

Ranking strategy:
1. Run all available search backends in parallel
2. Normalize scores to [0, 1] per backend
3. Combine with weights: keyword=0.4, symbol=0.3, semantic=0.3
4. Deduplicate by file+line range
5. Re-rank by combined score

---

### 6. New MCP Tools

**File:** `internal/tools/semantic.go`

#### `search_semantic`

```ts
search_semantic(query: string, top_k?: number)
  → {
      available: boolean,
      results: [{ path, start_line, end_line, snippet, score }]
    }
```

- Returns `available: false` if Ollama not detected
- Triggers lazy embedding if index is stale

#### `hybrid_search`

```ts
hybrid_search(query: string, top_k?: number)
  → {
      results: [{ path, line_start, line_end, snippet, score, sources }]
    }
```

- Always works (falls back to keyword-only if others unavailable)
- `sources` array indicates which backends contributed

---

### 7. Indexer Updates

**File:** `cmd/repo-search-index/main.go`

New subcommand:
```bash
repo-search-index embed [--force] [path]
```

- `--force`: Re-embed all chunks even if unchanged
- Only runs if Ollama is available
- Respects `.gitignore` and default ignores
- Progress output to stderr

---

### 8. Makefile Updates

```makefile
embed: build
	@./$(INDEXER) embed .

doctor:
	# ... existing checks ...
	@command -v curl >/dev/null && curl -s localhost:11434/api/tags >/dev/null 2>&1 \
		&& echo "✓ ollama: running" \
		|| echo "○ ollama: not detected (semantic search disabled)"
```

---

## Configuration

**File:** `.repo_search/config.json` (optional)

```json
{
  "semantic": {
    "enabled": true,
    "ollama_url": "http://localhost:11434",
    "model": "nomic-embed-text",
    "chunk_max_lines": 50,
    "chunk_overlap_lines": 10
  },
  "hybrid": {
    "keyword_weight": 0.4,
    "symbol_weight": 0.3,
    "semantic_weight": 0.3
  }
}
```

Defaults are sensible; config file is optional.

---

## File Changes Summary

| File | Change |
|------|--------|
| `internal/embedding/ollama.go` | NEW: Ollama HTTP client |
| `internal/embedding/chunker.go` | NEW: Code chunking logic |
| `internal/embedding/store.go` | NEW: SQLite vector storage |
| `internal/embedding/search.go` | NEW: Semantic search |
| `internal/embedding/math.go` | NEW: Cosine similarity |
| `internal/search/hybrid/hybrid.go` | NEW: Combined search |
| `internal/tools/semantic.go` | NEW: MCP tools |
| `cmd/repo-search-index/main.go` | UPDATE: Add embed subcommand |
| `Makefile` | UPDATE: Add embed target, update doctor |
| `go.mod` | No new deps (pure Go HTTP + math) |

---

## Dependencies

- **Ollama** (optional, external): Must be installed and running with an embedding model
- No new Go dependencies (HTTP client + JSON + basic math all in stdlib)

---

## Embedding Model Choice

Recommended: `nomic-embed-text`
- 768 dimensions
- Good code understanding
- Fast inference
- Open source

Alternative: `mxbai-embed-large`
- 1024 dimensions
- Higher quality
- Slower

User can configure via `.repo_search/config.json`.

---

## Testing Plan

1. **Unit tests for chunker** — various file types, edge cases
2. **Unit tests for vector math** — cosine similarity correctness
3. **Integration test with mock Ollama** — HTTP response parsing
4. **Integration test with real Ollama** — full embed + search cycle
5. **Hybrid search test** — verify deduplication and ranking
6. **Graceful degradation test** — behavior when Ollama unavailable

---

## Success Criteria

- `search_semantic("authentication error handling")` returns relevant code
- `hybrid_search` returns better results than keyword alone
- Embedding 1000 chunks completes in <60s
- Vector search on 10k chunks completes in <500ms
- Zero errors when Ollama is not installed (graceful degradation)
- Phase 1/2 functionality completely unaffected

---

## Performance Considerations

### Embedding Generation
- Batch requests to Ollama (32 texts per request)
- Skip unchanged chunks (content hash comparison)
- Progress indicator for large repos

### Vector Search
- Brute-force cosine similarity is O(n) but fast for <10k vectors
- Future: Consider approximate nearest neighbor (FAISS) for larger repos
- Cache query embeddings for repeated searches

### Storage
- JSON vectors are ~4x larger than binary but debuggable
- 10k chunks × 768 dims × 4 bytes × 4 (JSON overhead) ≈ 120MB
- Acceptable for typical repos; compression possible if needed

---

## Rollout Strategy

1. **Phase 3a**: Implement core embedding + semantic search
2. **Phase 3b**: Add hybrid search combining all backends
3. **Phase 3c**: Performance optimization if needed

Each sub-phase is independently shippable.

---

## Non-Goals (Phase 3)

- Cloud embedding services (OpenAI, Cohere, etc.)
- Multi-repo semantic search
- Real-time embedding updates (file watcher)
- GPU acceleration
- Custom fine-tuned models

---

## Review Checklist

- [ ] Ollama client handles timeouts gracefully
- [ ] Chunker respects function boundaries when possible
- [ ] Vector storage doesn't bloat SQLite excessively
- [ ] Hybrid search deduplication is correct
- [ ] Doctor command accurately reports Ollama status
- [ ] All existing tests still pass
- [ ] README updated with Phase 3 features
