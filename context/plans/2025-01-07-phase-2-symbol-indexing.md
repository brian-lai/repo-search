# Phase 2 Plan: Symbol Indexing

## Overview

Add structural understanding of the codebase via universal-ctags, stored in SQLite for fast symbol lookup.

## Goals

- Engineers can query "where is X defined?" instantly
- Support Go, TypeScript, Python, Java, Rust out of the box
- Incremental indexing (only re-parse changed files)
- No impact on Phase 1 keyword search functionality

---

## Implementation Tasks

### 1. SQLite Schema Setup

**File:** `internal/search/symbols/schema.go`

```sql
CREATE TABLE IF NOT EXISTS symbols (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,           -- function, type, variable, constant, etc.
    path TEXT NOT NULL,
    line INTEGER NOT NULL,
    language TEXT,
    pattern TEXT,                 -- ctags search pattern
    scope TEXT,                   -- parent scope (class, module, etc.)
    signature TEXT,               -- function signature if available
    UNIQUE(name, path, line)
);

CREATE INDEX idx_symbols_name ON symbols(name);
CREATE INDEX idx_symbols_path ON symbols(path);
CREATE INDEX idx_symbols_kind ON symbols(kind);

CREATE TABLE IF NOT EXISTS files (
    path TEXT PRIMARY KEY,
    mtime INTEGER NOT NULL,
    size INTEGER NOT NULL,
    indexed_at INTEGER NOT NULL
);
```

---

### 2. Ctags Integration

**File:** `internal/search/symbols/ctags.go`

- Run `universal-ctags --output-format=json --fields=+nKS -R <path>`
- Parse JSON output into Symbol structs
- Handle ctags not being installed (graceful degradation)

**ctags JSON format example:**
```json
{
  "name": "Server",
  "path": "internal/mcp/server.go",
  "pattern": "/^type Server struct {$/",
  "kind": "struct",
  "line": 15,
  "language": "Go"
}
```

---

### 3. Indexer Update

**File:** `cmd/codetect-index/main.go`

Replace no-op with:
1. Check for `universal-ctags` availability
2. Scan files for mtime changes
3. Run ctags on changed files
4. Insert/update SQLite rows
5. Store index in `.codetect/symbols.db`

---

### 4. New MCP Tools

**File:** `internal/tools/symbols.go`

#### `find_symbol`

```ts
find_symbol(name: string, kind?: string, limit?: number)
  → { symbols: [{ name, kind, path, line, scope }] }
```

- Query SQLite with LIKE for fuzzy matching
- Support filtering by kind (function, type, etc.)

#### `list_defs_in_file`

```ts
list_defs_in_file(path: string)
  → { symbols: [{ name, kind, line }] }
```

- Return all definitions in a single file
- Useful for generating file outlines

---

### 5. Doctor Command Updates

Add check for `universal-ctags`:
```bash
command -v ctags >/dev/null && ctags --version | grep -q Universal
```

---

## File Changes Summary

| File | Change |
|------|--------|
| `internal/search/symbols/schema.go` | NEW: SQLite schema + migrations |
| `internal/search/symbols/ctags.go` | NEW: ctags JSON parsing |
| `internal/search/symbols/index.go` | UPDATE: Implement Index methods |
| `internal/tools/symbols.go` | NEW: find_symbol, list_defs_in_file tools |
| `cmd/codetect-index/main.go` | UPDATE: Real indexing logic |
| `Makefile` | UPDATE: doctor checks for ctags |
| `go.mod` | ADD: github.com/mattn/go-sqlite3 |

---

## Dependencies

- `github.com/mattn/go-sqlite3` (CGO) or `modernc.org/sqlite` (pure Go)
- `universal-ctags` binary (optional but recommended)

---

## Testing Plan

1. Unit tests for ctags JSON parsing
2. Integration test: index small repo, query symbols
3. Test incremental indexing (modify file, re-index)
4. Test graceful degradation when ctags is missing

---

## Success Criteria

- `find_symbol("Server")` returns Symbol struct in <100ms
- Indexing a 10k LOC repo completes in <5s
- Index size is reasonable (<10MB for typical repo)
- Phase 1 functionality unchanged

---

## Non-Goals (Phase 2)

- Cross-file reference tracking (find usages)
- Semantic analysis (type inference)
- Language server protocol integration
