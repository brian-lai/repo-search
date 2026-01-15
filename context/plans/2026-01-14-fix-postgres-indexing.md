# Fix PostgreSQL Indexing Bug

**Date:** 2026-01-14
**Objective:** Make `codetect index` and `codetect embed` respect database configuration so they can write to PostgreSQL instead of being hardcoded to SQLite

## Problem Statement

Currently, there's a critical disconnect in database usage:

- **`codetect index`** and **`codetect embed`** are hardcoded to always write to SQLite at `.codetect/symbols.db`
- **`codetect mcp`** (MCP server) correctly loads database config and can read from PostgreSQL
- **Result:** Even with `CODETECT_DB_TYPE=postgres` configured, all indexing goes to SQLite, and PostgreSQL remains empty

The bug is in `cmd/codetect-index/main.go` which ignores environment variables `CODETECT_DB_TYPE` and `CODETECT_DB_DSN`.

## Root Cause

**Hardcoded paths in cmd/codetect-index/main.go:**
- Line 80: `dbPath := filepath.Join(indexDir, "symbols.db")`
- Line 87: `idx, err := symbols.NewIndex(dbPath)` - Always creates SQLite index
- Line 186: `dbPath := filepath.Join(indexDir, "symbols.db")` (in runEmbed)
- Line 194: `idx, err := symbols.NewIndex(dbPath)` (in runEmbed)
- Line 405: `dbPath := filepath.Join(absPath, ".codetect", "symbols.db")` (in runStats)

**Missing:** Never calls `config.LoadDatabaseConfigFromEnv()` or uses the existing `symbols.NewIndexWithConfig()` constructor.

## Deeper Issue: Adapter Pattern Violation

**CRITICAL DISCOVERY:** The Symbol Index has a broken adapter pattern implementation that violates the architectural goal of isolating database-specific code.

### The Problem in `internal/search/symbols/index.go`

**Two constructors with inconsistent initialization:**

```go
// Line 26-37: Old constructor (SQLite only)
func NewIndex(dbPath string) (*Index, error) {
    sqlDB, err := OpenDB(dbPath)
    return &Index{
        sqlDB:   sqlDB,           // ✓ Set
        adapter: db.WrapSQL(sqlDB), // ✓ Set
        dialect: db.GetDialect(db.DatabaseSQLite),
        dbPath:  dbPath,
    }, nil
}

// Line 40-52: New constructor (multi-DB)
func NewIndexWithConfig(cfg db.Config) (*Index, error) {
    database, err := db.Open(cfg)
    return &Index{
        adapter: database,  // ✓ Set
        dialect: cfg.Dialect(),
        dbPath:  cfg.Path,
        // sqlDB is NIL! ❌
    }, nil
}
```

**All methods bypass the adapter and use raw sqlDB:**

The Index struct has both `sqlDB` and `adapter` fields, but **9 out of 11 methods** directly use `idx.sqlDB`, bypassing the abstraction layer:

1. **Line 117** - `FindSymbol`: `rows, err := idx.sqlDB.Query(query, args...)`
2. **Line 146** - `ListDefsInFile`: `rows, err := idx.sqlDB.Query(query, path)`
3. **Line 193** - `Update`: `tx, err := idx.sqlDB.Begin()`
4. **Line 253** - `FullReindex`: `tx, err := idx.sqlDB.Begin()`
5. **Line 256** - `FullReindex`: `_, err = idx.sqlDB.Exec("DELETE FROM files WHERE path = ?", path)`
6. **Line 272** - `Update`: `_, err = idx.sqlDB.Exec(query, path, mtimeNano)`
7. **Line 394** - `Stats`: `err := idx.sqlDB.QueryRow("SELECT COUNT(*) FROM symbols").Scan(&stats.TotalSymbols)`
8. **Line 397** - `Stats`: `err = idx.sqlDB.QueryRow("SELECT COUNT(*) FROM files").Scan(&stats.TotalFiles)`

Only 2 methods use the adapter:
- **Line 57-58** - `Close()`
- **Line 73** - `DB()` accessor

**Hardcoded SQLite placeholder syntax:**

All queries use `?` placeholders (SQLite syntax). PostgreSQL requires `$1, $2, $3` syntax. Example from FindSymbol:

```go
query = `SELECT name, kind, path, line, language, pattern, scope
         FROM symbols
         WHERE name LIKE ? AND kind = ?
         ORDER BY CASE WHEN name = ? THEN 0
                       WHEN name LIKE ? THEN 1
                       ELSE 2 END,
                  name
         LIMIT ?`
```

### Why This Breaks the Adapter Pattern

**Architecture Goal:** "Swapping out DB technologies shouldn't cause compatibility issues and any changes to DB would be isolated to one layer"

**Current Reality:**
- ❌ Database-specific code (SQL with "?" placeholders) is in business logic methods
- ❌ Methods directly depend on raw `*sql.DB` instead of the `db.DB` interface
- ❌ Using `NewIndexWithConfig` with PostgreSQL will cause nil pointer panics
- ❌ Database changes are NOT isolated - they leak into every Symbol Index method

**Required Fix:**
1. Refactor all Symbol Index methods to use `idx.adapter` instead of `idx.sqlDB`
2. Convert all hardcoded SQL to use `idx.dialect.Placeholder(n)` for parameter binding
3. Either remove the `sqlDB` field entirely or deprecate direct usage
4. Ensure PostgreSQL and SQLite can be swapped without code changes

## Good News: Foundation Exists

The codebase already has the right patterns in other areas:
- ✅ `config.LoadDatabaseConfigFromEnv()` - loads DB config from environment
- ✅ `db.Open(cfg)` - routes to appropriate database backend
- ✅ `db.Dialect` interface - provides `Placeholder(n)` for database-agnostic queries
- ✅ `embedding.NewEmbeddingStoreWithOptions()` - properly uses adapter pattern
- ✅ Migration tool (`cmd/migrate-to-postgres/main.go`) - shows correct config loading

**The fix requires two parts:**
1. Make `codetect-index` use config loading (straightforward)
2. Refactor Symbol Index to actually use the adapter pattern (moderate complexity)

## Approach

### Phase 1: Refactor Symbol Index for Adapter Pattern Compliance

**PRIORITY:** Must fix the adapter pattern violation before PostgreSQL will work.

Modify `internal/search/symbols/index.go` to use the adapter interface throughout:

**1.1: Extract sqlDB from adapter (backward compatible approach)**

```go
// Add helper to get underlying *sql.DB when needed for transactions
func (idx *Index) getSQLDB() (*sql.DB, error) {
    if idx.sqlDB != nil {
        return idx.sqlDB, nil
    }
    // Extract from adapter for NewIndexWithConfig case
    return idx.adapter.Unwrap()
}
```

**1.2: Convert methods to use adapter + dialect-aware placeholders**

For each of the 9 methods using `idx.sqlDB`:

**FindSymbol (line 82-137):**
- Replace `idx.sqlDB.Query()` with `idx.adapter.Query()`
- Convert placeholders: `?` → `idx.dialect.Placeholder(n)`
- Example transformation:
  ```go
  // Before:
  query = `SELECT ... WHERE name LIKE ? AND kind = ? ... LIMIT ?`
  rows, err := idx.sqlDB.Query(query, args...)

  // After:
  query = fmt.Sprintf(`SELECT ... WHERE name LIKE %s AND kind = %s ... LIMIT %s`,
      idx.dialect.Placeholder(1),
      idx.dialect.Placeholder(2),
      idx.dialect.Placeholder(len(args)))
  rows, err := idx.adapter.Query(ctx, query, args...)
  ```

**ListDefsInFile (line 139-180):**
- Replace `idx.sqlDB.Query()` with `idx.adapter.Query()`
- Convert `?` to `idx.dialect.Placeholder(1)`

**Update (line 182-289):**
- Transactions need special handling: use `getSQLDB()` helper
- Convert placeholders in all queries
- Example:
  ```go
  sqlDB, err := idx.getSQLDB()
  if err != nil { return err }
  tx, err := sqlDB.Begin()
  ```

**FullReindex (line 291-386):**
- Use `getSQLDB()` for transaction
- Convert placeholders in DELETE query

**Stats (line 388-428):**
- Replace `idx.sqlDB.QueryRow()` with `idx.adapter.QueryRow()`
- Simple queries, no placeholders needed

**1.3: Update NewIndexWithConfig documentation**
- Add comment explaining adapter-first design
- Note that sqlDB is only for transaction support

**1.4: Consider deprecating NewIndex**
- Add deprecation comment to `NewIndex(dbPath)` constructor
- Encourage migration to `NewIndexWithConfig`
- Keep for backward compatibility (wrapped with adapter)

### Phase 2: Update codetect-index Commands

Modify `cmd/codetect-index/main.go` to load and use database configuration:

**For `runIndex()` function (lines 48-118):**
1. Load config: `dbConfig := config.LoadDatabaseConfigFromEnv()`
2. Convert to db.Config: `cfg := dbConfig.ToDBConfig()`
3. Use config-aware constructor: `idx, err := symbols.NewIndexWithConfig(cfg)`
4. Remove hardcoded path logic

**For `runEmbed()` function (lines 120-308):**
1. Load config (same as above)
2. Open database via config
3. Use dialect-aware embedding store constructor
4. Remove hardcoded path logic

**For `runStats()` function (lines 393-436):**
1. Load config (same as above)
2. Open database via config
3. Remove hardcoded path logic

### Phase 3: Handle SQLite Backward Compatibility

When database type is SQLite and no explicit path is set:
- Default to `.codetect/symbols.db` for backward compatibility
- Use `dbConfig.Path` if explicitly configured
- Ensure existing SQLite-based workflows continue to work

### Phase 4: Update Symbol Tools (for consistency)

Modify `internal/tools/symbols.go` line 159-171 (`openIndex()`) to:
- Load database config like semantic tools do
- Use `symbols.NewIndexWithConfig()`
- Maintain consistency with other tools

### Phase 5: Testing

1. **Unit tests:**
   - Test with `CODETECT_DB_TYPE=sqlite` (default behavior)
   - Test with `CODETECT_DB_TYPE=postgres` (new functionality)
   - Test without env vars (backward compatibility)

2. **Integration tests:**
   - Index with PostgreSQL, verify data in postgres
   - Embed with PostgreSQL, verify vectors in postgres
   - Stats with PostgreSQL, verify correct counts
   - Ensure MCP server can read from PostgreSQL after indexing

3. **End-to-end test:**
   ```bash
   # Set up PostgreSQL
   export CODETECT_DB_TYPE=postgres
   export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5465/codetect?sslmode=disable"

   # Index and embed
   codetect index
   codetect embed

   # Verify PostgreSQL has data
   psql $CODETECT_DB_DSN -c "SELECT COUNT(*) FROM symbols"
   psql $CODETECT_DB_DSN -c "SELECT COUNT(*) FROM embeddings"

   # Verify MCP server can use it
   codetect mcp  # Should query PostgreSQL
   ```

## Files to Modify

### Critical Changes (Adapter Pattern Compliance):
1. **`internal/search/symbols/index.go`** (ALL methods need refactoring)
   - **Line 14-22**: Index struct (document adapter-first design)
   - **Line 26-37**: `NewIndex()` (consider deprecation comment)
   - **Line 40-52**: `NewIndexWithConfig()` (add documentation)
   - **New**: `getSQLDB()` helper method for transaction support
   - **Line 82-137**: `FindSymbol()` - Use adapter, convert placeholders
   - **Line 139-180**: `ListDefsInFile()` - Use adapter, convert placeholders
   - **Line 182-289**: `Update()` - Use getSQLDB for transaction, convert placeholders
   - **Line 291-386**: `FullReindex()` - Use getSQLDB for transaction, convert placeholders
   - **Line 388-428**: `Stats()` - Use adapter

   **Complexity:** Moderate - 9 methods need placeholder conversion, transaction handling

### Primary Changes (Command Integration):
2. **`cmd/codetect-index/main.go`** (lines 48-436)
   - `runIndex()` - Load config, use NewIndexWithConfig
   - `runEmbed()` - Load config, use config-aware store
   - `runStats()` - Load config, open correct database

   **Complexity:** Low - straightforward config loading

### Secondary Changes (Consistency):
3. **`internal/tools/symbols.go`** (lines 159-171)
   - `openIndex()` - Make database-aware like semantic tools

   **Complexity:** Low - copy pattern from semantic tools

### No Changes Needed:
- `internal/config/database.go` - Already works ✓
- `internal/db/open.go` - Already routes correctly ✓
- `internal/embedding/store.go` - Already dialect-aware ✓
- `internal/db/dialect.go` - Placeholder() method exists ✓

## Implementation Pattern (From migrate-to-postgres)

Reference implementation in `cmd/migrate-to-postgres/main.go` shows the correct pattern:

```go
// Load database configuration
dbConfig := config.LoadDatabaseConfigFromEnv()

// For symbols
cfg := dbConfig.ToDBConfig()
idx, err := symbols.NewIndexWithConfig(cfg)

// For embeddings
database, err := db.Open(cfg)
dialect := cfg.Dialect()
store, err := embedding.NewEmbeddingStoreWithOptions(
    database,
    dialect,
    dbConfig.VectorDimensions,
)
```

## Success Criteria

**Adapter Pattern Compliance:**
- [ ] All Symbol Index methods use `idx.adapter` interface (not raw `idx.sqlDB`)
- [ ] All queries use dialect-aware placeholders (no hardcoded `?`)
- [ ] `NewIndexWithConfig` works with both PostgreSQL and SQLite
- [ ] Transaction handling works via `getSQLDB()` helper

**PostgreSQL Functionality:**
- [ ] `codetect index` with `CODETECT_DB_TYPE=postgres` writes to PostgreSQL
- [ ] `codetect embed` with `CODETECT_DB_TYPE=postgres` writes to PostgreSQL
- [ ] `codetect stats` shows correct counts from PostgreSQL
- [ ] MCP server can query PostgreSQL data after indexing

**Backward Compatibility:**
- [ ] SQLite still works without env vars (defaults to `.codetect/symbols.db`)
- [ ] Existing `NewIndex(dbPath)` constructor still works
- [ ] All existing tests pass (SQLite-based)

**Testing:**
- [ ] New unit tests validate adapter pattern usage
- [ ] New integration tests validate PostgreSQL indexing
- [ ] Placeholder conversion tested for both databases

## Risks & Mitigations

**Risk:** Breaking adapter pattern during refactoring
**Mitigation:**
- Add comprehensive unit tests for adapter usage
- Code review checklist: verify no direct `idx.sqlDB` usage in business logic
- Test both PostgreSQL and SQLite paths

**Risk:** Placeholder syntax differences causing query failures
**Mitigation:**
- Use `idx.dialect.Placeholder(n)` throughout
- Test queries against both SQLite (`?`) and PostgreSQL (`$1, $2, ...`)
- Add integration tests that run same queries on both databases

**Risk:** Transaction handling complexity
**Mitigation:**
- Create `getSQLDB()` helper to centralize transaction logic
- Document when transactions are needed vs. adapter methods
- Keep transaction scope minimal

**Risk:** Breaking existing SQLite workflows
**Mitigation:**
- Default to SQLite when no env vars set
- Extensive backward compatibility tests
- Keep `NewIndex(dbPath)` constructor working

**Risk:** Schema differences between SQLite and PostgreSQL
**Mitigation:**
- Use dialect-aware SQL via db.Dialect interface (already implemented)
- Schema migration already tested in Phase 7 of PostgreSQL support

**Risk:** Connection errors to PostgreSQL during indexing
**Mitigation:**
- Clear error messages, suggest checking `CODETECT_DB_DSN`
- Validate connection before starting indexing
- Document database setup in README

**Risk:** Users don't know about environment variables
**Mitigation:**
- Document configuration in README
- Add helpful error messages pointing to docs
- `codetect init` could optionally configure database

## Verification Steps

After implementation:

1. **Verify SQLite still works (backward compatibility):**
   ```bash
   unset CODETECT_DB_TYPE CODETECT_DB_DSN
   codetect index
   codetect embed
   # Should create .codetect/symbols.db
   ```

2. **Verify PostgreSQL indexing:**
   ```bash
   export CODETECT_DB_TYPE=postgres
   export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5465/codetect?sslmode=disable"
   codetect index
   codetect embed
   docker-compose exec postgres psql -U codetect -d codetect -c "\dt"
   # Should show: symbols, embeddings, files tables
   ```

3. **Verify MCP server uses PostgreSQL:**
   ```bash
   # With env vars still set
   echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"find_symbol","arguments":{"name":"main"}}}' | codetect mcp
   # Should query PostgreSQL database
   ```

## Estimated Effort

### Phase 1: Symbol Index Refactoring (Adapter Pattern)
- **Code changes:** ~3-4 hours
  - Add `getSQLDB()` helper: 15 minutes
  - Refactor 9 methods to use adapter: 2-3 hours
  - Convert placeholders to dialect-aware: 1 hour
  - Documentation and deprecation comments: 30 minutes
- **Testing:** ~1-2 hours
  - Unit tests for adapter usage
  - Placeholder conversion tests
  - Transaction handling tests

### Phase 2: Command Integration
- **Code changes:** ~1-2 hours
  - Update `runIndex()`, `runEmbed()`, `runStats()`: 1 hour
  - Backward compatibility handling: 30 minutes
  - Error handling and messages: 30 minutes
- **Testing:** ~1 hour
  - Config loading tests
  - Default behavior tests

### Phase 3: Integration Testing
- **Testing:** ~1-2 hours
  - End-to-end PostgreSQL indexing
  - End-to-end SQLite backward compatibility
  - MCP server integration tests

### Phase 4: Documentation
- **Documentation:** ~30-60 minutes
  - Update README with database configuration
  - Add examples for PostgreSQL setup
  - Document environment variables

### Total Estimate
- **Code changes:** ~4-6 hours
- **Testing:** ~3-5 hours
- **Documentation:** ~30-60 minutes
- **Total:** ~8-12 hours

**Complexity Assessment:**
- **Phase 1 (Symbol Index):** Moderate complexity - requires careful refactoring to maintain backward compatibility while fixing adapter pattern
- **Phase 2 (Commands):** Low complexity - straightforward config loading
- **Phase 3-4:** Low complexity - standard testing and documentation

This is a high-value fix that properly establishes the adapter pattern, enabling future database backends without code changes.
