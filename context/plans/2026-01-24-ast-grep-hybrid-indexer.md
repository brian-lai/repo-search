# Plan: ast-grep Hybrid Indexer Prototype

## Objective

Replace ctags with ast-grep as the primary symbol indexer for supported languages, falling back to ctags for languages ast-grep doesn't support. This provides tree-sitter-based AST accuracy with broad language coverage.

## Background

Current bottlenecks identified in `codetect index`:
1. ctags runs recursively on entire repo even for incremental updates
2. Sequential symbol insertion (one INSERT per symbol)
3. No parallelization in symbol indexing phase
4. ctags misses some symbols and lacks AST accuracy

**Why ast-grep?**
- Tree-sitter based (AST-accurate symbol extraction)
- Single binary, no external dependencies beyond the tool itself
- JSON output format (similar to ctags, easy integration)
- Supports 20+ languages including: Go, TypeScript, JavaScript, Python, Rust, Java, C, C++, Ruby, etc.
- Simpler than raw tree-sitter (pattern-based queries)

## Approach

### Phase 1: ast-grep Integration (This Plan)

#### Step 1: Create ast-grep wrapper
**File:** `internal/search/symbols/astgrep.go`

- Implement `RunAstGrep(root string, files []string) ([]Symbol, error)`
- Define patterns for common symbol types per language:
  - Functions/methods
  - Classes/structs/interfaces
  - Type definitions
  - Constants/variables
- Parse JSON output into `Symbol` structs
- Handle language detection from file extension

#### Step 2: Implement hybrid indexer logic
**File:** `internal/search/symbols/index.go`

Modify `Update()` to:
1. Group files by language
2. For each language group:
   - If ast-grep supports it → use ast-grep
   - Else → use ctags
3. Merge results into unified symbol list
4. Batch insert symbols (fix perf issue)

#### Step 3: Add ast-grep availability check
**File:** `internal/search/symbols/astgrep.go`

- `AstGrepAvailable() bool` - check if `ast-grep` or `sg` binary exists
- Graceful degradation: if ast-grep unavailable, use ctags for all

#### Step 4: Performance improvements
**Files:** `internal/search/symbols/index.go`, `internal/search/symbols/astgrep.go`

- Batch symbol insertions (500-1000 at a time)
- Run ast-grep per-file or per-batch (not full recursive)
- Parallelize file processing where safe

#### Step 5: Add configuration option
**File:** `internal/config/config.go`

```yaml
index:
  backend: auto  # auto | ast-grep | ctags
```

- `auto` (default): ast-grep for supported languages, ctags fallback
- `ast-grep`: ast-grep only (error if unsupported language)
- `ctags`: ctags only (current behavior)

### Phase 2: SCIP Integration (Future Plan)

Deferred to a separate plan. Will add SCIP indexer support for projects that generate SCIP indexes (provides precise cross-file references).

## Language Support Matrix

| Language | ast-grep | ctags | Preferred |
|----------|----------|-------|-----------|
| Go | ✅ | ✅ | ast-grep |
| TypeScript | ✅ | ✅ | ast-grep |
| JavaScript | ✅ | ✅ | ast-grep |
| Python | ✅ | ✅ | ast-grep |
| Rust | ✅ | ✅ | ast-grep |
| Java | ✅ | ✅ | ast-grep |
| C/C++ | ✅ | ✅ | ast-grep |
| Ruby | ✅ | ✅ | ast-grep |
| PHP | ✅ | ✅ | ast-grep |
| C# | ✅ | ✅ | ast-grep |
| Kotlin | ✅ | ✅ | ast-grep |
| Swift | ✅ | ✅ | ast-grep |
| Lua | ✅ | ✅ | ast-grep |
| Elixir | ❌ | ✅ | ctags |
| Perl | ❌ | ✅ | ctags |
| Shell/Bash | ❌ | ✅ | ctags |
| SQL | ❌ | ✅ | ctags |
| Others | ❌ | ✅ | ctags |

## ast-grep Pattern Examples

### Go
```yaml
# Functions
sg --pattern 'func $NAME($$$) $$$' --json --lang go

# Methods
sg --pattern 'func ($RECV) $NAME($$$) $$$' --json --lang go

# Structs
sg --pattern 'type $NAME struct { $$$ }' --json --lang go

# Interfaces
sg --pattern 'type $NAME interface { $$$ }' --json --lang go
```

### TypeScript/JavaScript
```yaml
# Functions
sg --pattern 'function $NAME($$$) { $$$ }' --json --lang typescript

# Arrow functions (named)
sg --pattern 'const $NAME = ($$$) => $$$' --json --lang typescript

# Classes
sg --pattern 'class $NAME { $$$ }' --json --lang typescript

# Interfaces
sg --pattern 'interface $NAME { $$$ }' --json --lang typescript
```

### Python
```yaml
# Functions
sg --pattern 'def $NAME($$$): $$$' --json --lang python

# Classes
sg --pattern 'class $NAME: $$$' --json --lang python
sg --pattern 'class $NAME($$$): $$$' --json --lang python
```

## Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/search/symbols/astgrep.go` | Create | ast-grep wrapper, patterns, JSON parsing |
| `internal/search/symbols/index.go` | Modify | Hybrid logic, batch inserts |
| `internal/search/symbols/ctags.go` | Modify | Add per-file mode (not just recursive) |
| `internal/config/config.go` | Modify | Add `index.backend` config option |
| `cmd/codetect-index/main.go` | Modify | Pass config to indexer |

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| ast-grep patterns miss symbols | Medium | Medium | Compare output with ctags, refine patterns |
| ast-grep not installed | Medium | Low | Graceful fallback to ctags |
| Performance regression | Low | High | Benchmark before/after, batch operations |
| Pattern syntax changes | Low | Medium | Pin ast-grep version in docs |

## Success Criteria

1. **Functionality:**
   - [ ] ast-grep extracts symbols for Go, TypeScript, Python, Rust (top languages)
   - [ ] Falls back to ctags for unsupported languages
   - [ ] All existing tests pass
   - [ ] Symbol count comparable to ctags-only (±5%)

2. **Performance:**
   - [ ] Incremental index on unchanged repo: <1s (currently slow)
   - [ ] Full index on medium repo (1000 files): comparable or faster than ctags
   - [ ] Batch inserts reduce DB round-trips by 10x+

3. **Reliability:**
   - [ ] Graceful degradation when ast-grep unavailable
   - [ ] No crashes on malformed files
   - [ ] Clear error messages for configuration issues

## Testing Plan

1. **Unit tests:** `internal/search/symbols/astgrep_test.go`
   - Pattern matching for each language
   - JSON parsing
   - Symbol normalization

2. **Integration tests:**
   - Index a multi-language repo with hybrid backend
   - Compare symbol counts: ast-grep vs ctags
   - Verify fallback behavior

3. **Benchmarks:**
   - Before/after timing on codetect repo
   - Before/after timing on larger test repo

## Review Checklist

- [ ] ast-grep patterns cover main symbol types per language
- [ ] Fallback logic works when ast-grep unavailable
- [ ] Batch inserts implemented correctly
- [ ] Config option documented
- [ ] Error handling is robust
- [ ] No regressions in existing functionality

## Dependencies

- **External:** ast-grep CLI (`brew install ast-grep` or `cargo install ast-grep`)
- **Internal:** None (builds on existing symbol infrastructure)

## Estimated Scope

- **New code:** ~400-500 lines (astgrep.go)
- **Modified code:** ~100-150 lines (index.go, ctags.go, config.go)
- **Tests:** ~200-300 lines
