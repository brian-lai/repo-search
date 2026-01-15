# Phase 1 – Local Keyword Search MCP (codetect)

This document captures **Phase 1** of the Cursor-like setup for Claude Code: a **local MCP server** written in Go that provides fast keyword search and file retrieval, plus a one-command launcher.

Phase 1 intentionally avoids symbols and embeddings. The goal is to immediately stop Claude from repeatedly scanning files and instead give it **fast, grounded retrieval**.

---

## Phase 1 Goals

* Local-only (no cloud dependencies)
* Fast startup
* Minimal moving parts
* One-command workflow (`./bin/claude`)
* MCP tools available to Claude Code:

  * `search_keyword`
  * `get_file`

---

## Architecture (Phase 1)

```text
Claude Code
   │
   │ (MCP stdio)
   ▼
codetect (Go binary)
   │
   ├─ search_keyword → ripgrep (rg)
   └─ get_file       → direct file read
```

Claude **never scans the repo itself**. All retrieval flows through MCP tools.

---

## Repo Layout (Phase 1)

```text
codetect/
  cmd/
    codetect/              # MCP server (stdio)
    codetect-index/        # indexer CLI (no-op in Phase 1)
  internal/
    mcp/                      # MCP transport (JSON over stdio)
    search/
      keyword/                # ripgrep integration
      files/                  # file read + line slicing
      symbols/                # stub (Phase 2)
  bin/
    claude                    # wrapper: index + exec claude
  .mcp.json                   # project-scoped MCP config
  Makefile
  README.md
```

---

## MCP Configuration

### `.mcp.json`

Registers the MCP server **at project scope**, so entering the repo automatically enables it.

```json
{
  "mcpServers": {
    "codetect": {
      "command": "make",
      "args": ["mcp"],
      "env": {}
    }
  }
}
```

---

## One-Command Startup

### `bin/claude`

```bash
#!/usr/bin/env bash
set -euo pipefail

# Phase 1: indexing is a fast no-op
make -s index >/dev/null 2>&1 || true

exec claude "$@"
```

Usage:

```bash
./bin/claude
```

This mirrors Cursor’s "open repo → ready" behavior.

---

## Makefile (Phase 1)

```makefile
BINARY=dist/codetect
INDEXER=dist/codetect-index

.PHONY: build mcp index doctor clean

build:
	mkdir -p dist
	go build -o $(BINARY) ./cmd/codetect
	go build -o $(INDEXER) ./cmd/codetect-index

mcp: build
	./$(BINARY)

index: build
	./$(INDEXER) index .

doctor:
	@command -v rg >/dev/null || (echo "missing: ripgrep (rg)"; exit 1)
	@echo "ok"

clean:
	rm -rf dist .codetect
```

---

## MCP Tools (Phase 1)

### `search_keyword`

**Purpose:** Fast keyword search using ripgrep.

```ts
search_keyword(query: string, top_k?: number)
  → { results: [{ path, line_start, line_end, snippet, score }] }
```

Implementation notes:

* Calls `rg --line-number --no-heading`
* Caps results to `top_k`
* Treats `rg` exit code 1 as "no results" (not an error)

---

### `get_file`

**Purpose:** Read file contents (optionally line-ranged).

```ts
get_file(path: string, start_line?: number, end_line?: number)
  → { path, content }
```

Used by Claude to pull surrounding context once a file is identified.

---

## MCP Server Responsibilities (Phase 1)

* Read line-delimited JSON from stdin
* Support:

  * `tools/list`
  * `tools/call`
* Dispatch to Go services
* Return structured JSON responses

No networking, no persistence, no watchers yet.

---

## Indexer CLI (Phase 1)

### `codetect-index`

Phase 1 behavior:

* Exists for UX consistency
* Does nothing (fast no-op)

This allows later phases to add real indexing without changing the workflow.

---

## What Phase 1 Already Gets You

* Claude stops re-reading files
* Near-instant keyword lookup
* Deterministic context injection
* Debuggable, inspectable behavior
* Identical architectural model to Cursor

---

## Phase 2 Preview (Not Implemented Yet)

* Symbol indexing via `universal-ctags`
* SQLite-backed symbol table
* `find_symbol` MCP tool

---

## Phase 3 Preview (Optional)

* Ollama embeddings
* Local vector storage
* `search_semantic` MCP tool

---

## Design Principle

> Phase 1 should feel *boring* — and immediately useful.

Everything fancy comes later.

