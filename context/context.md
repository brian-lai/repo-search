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
- [ ] Implement JSON parsing for ast-grep output into Symbol structs
- [ ] Add language detection from file extensions
- [ ] Modify index.go Update() to group files by language
- [ ] Implement hybrid logic: ast-grep for supported languages, ctags for others
- [ ] Add batch symbol insertion (500-1000 at a time) to reduce DB round-trips
- [ ] Add configuration option for index backend (auto/ast-grep/ctags)
- [ ] Write unit tests for ast-grep wrapper
- [ ] Write integration tests for hybrid indexing
- [ ] Benchmark performance vs ctags-only approach

## Progress Notes

### 2026-01-24 - Execution Started

Starting implementation of ast-grep hybrid indexer. Will create ast-grep wrapper first, then integrate with existing index logic.

**Key design decisions:**
- ast-grep for top 12+ languages (Go, TS, JS, Python, Rust, Java, C/C++, Ruby, PHP, C#, Kotlin, Swift)
- Graceful fallback to universal-ctags for unsupported languages
- Batch insertions to improve performance
- Configuration option for backend selection

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
