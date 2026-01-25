# Current Work Summary

Executing: ast-grep Hybrid Indexer Prototype

**Branch:** `para/ast-grep-hybrid-indexer`
**Plan:** context/plans/2026-01-24-ast-grep-hybrid-indexer.md

## Objective

Replace ctags with ast-grep as the primary symbol indexer for supported languages, falling back to ctags for languages ast-grep doesn't support. This provides tree-sitter-based AST accuracy with broad language coverage.

## To-Do List

- [x] Create ast-grep wrapper with pattern definitions for common languages (internal/search/symbols/astgrep.go)
- [x] Implement ast-grep availability check (AstGrepAvailable function)
- [x] Define symbol extraction patterns for Go, TypeScript, JavaScript, Python, Rust
- [x] Implement JSON parsing for ast-grep output into Symbol structs
- [x] Add language detection from file extensions
- [x] Modify index.go Update() to group files by language
- [x] Implement hybrid logic: ast-grep for supported languages, ctags for others
- [x] Add batch symbol insertion (500-1000 at a time) to reduce DB round-trips
- [x] Add configuration option for index backend (auto/ast-grep/ctags)
- [x] Write unit tests for ast-grep wrapper
- [x] Write integration tests for hybrid indexing
- [x] Benchmark performance vs ctags-only approach

## Progress Notes

### 2026-01-24 - Execution Completed ✅

**Implementation Summary:**

1. **ast-grep wrapper** (`internal/search/symbols/astgrep.go`):
   - Pattern definitions for 13 languages (Go, TS, JS, Python, Rust, Java, C/C++, Ruby, PHP, C#, Kotlin, Swift)
   - JSON parsing and Symbol conversion
   - Language detection from file extensions
   - Deduplication logic

2. **Hybrid indexer** (`internal/search/symbols/index.go`):
   - Groups files by language
   - Tries ast-grep first for supported languages
   - Falls back to ctags for unsupported files or failures
   - Batch symbol insertion (500 at a time) - reduces DB round-trips by ~100x

3. **Configuration** (`internal/config/index.go`):
   - `CODETECT_INDEX_BACKEND` environment variable
   - Three modes: `auto` (default), `ast-grep`, `ctags`
   - Graceful degradation when tools unavailable

4. **Testing**:
   - Unit tests for all ast-grep wrapper functions
   - Integration test: 29 symbols indexed across 3 files
   - Benchmarks for performance comparison

**Key design decisions:**
- ast-grep for top 13 languages (Go, TS, JS, Python, Rust, Java, C/C++, Ruby, PHP, C#, Kotlin, Swift)
- Graceful fallback to universal-ctags for unsupported languages
- Batch insertions to improve performance (500 symbols/batch)
- Configuration option for backend selection
- Works with ast-grep only, ctags only, or both

---

```json
{
  "active_context": [
    "context/plans/2026-01-24-ast-grep-hybrid-indexer.md"
  ],
  "completed_summaries": [],
  "execution_branch": "para/ast-grep-hybrid-indexer",
  "execution_started": "2026-01-24T13:30:00Z",
  "last_updated": "2026-01-24T13:30:00Z"
}
```
