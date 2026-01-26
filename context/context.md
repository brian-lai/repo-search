# Current Work Summary

Executing: Dimension-Grouped Embedding Tables for Org-Scale Multi-Repo Support

**Branch:** `para/dimension-grouped-embeddings`
**Master Plan:** context/plans/2026-01-24-dimension-grouped-embeddings.md

## Problem

Single `embeddings` table with fixed vector dimensions causes dimension mismatch errors when users switch models, prevents cross-repo search, and doesn't scale for org deployment (3000+ repos at Justworks).

## Solution

Dimension-grouped tables (`embeddings_768`, `embeddings_1024`) with repo config tracking.

## To-Do List

### Phase 1: Database Schema Updates
- [x] Add `tableNameForDimensions(dim int) string` helper function
- [x] Add `repo_embedding_configs` table schema and CRUD
- [x] Modify `initSchema()` to create dimension-specific tables

### Phase 2: EmbeddingStore Refactor
- [x] Update `tableName()` method to return dimension-specific table
- [x] Update `Save()` and `SaveBatch()` to use correct table
- [x] Update `Search()` to query correct table (via GetAll)
- [x] Update `Delete()` and `DeleteAll()` to target correct table
- [x] Update `GetByPath()` and `Count()` to use correct table

### Phase 3: Repo Config Management
- [x] Create `RepoEmbeddingConfig` struct
- [x] Implement `GetRepoConfig()` method
- [x] Implement `SetRepoConfig()` method
- [x] Implement `ListRepoConfigs()` method

### Phase 4: Model Switch Handling
- [x] Add dimension change detection in `codetect-index embed`
- [x] Implement `MigrateRepoDimensions()` to move data between tables
- [ ] Update installer dimension mismatch handling (deferred - installer already has detection)

### Phase 5: Cross-Repo Search
- [x] Add `CrossRepoSearchResult` and `CrossRepoSearchResponse` types
- [x] Implement `SearchAcrossRepos()` method in SemanticSearcher
- [x] Implement `GetAllAcrossRepos()` method in EmbeddingStore
- [ ] Update MCP tool to expose cross-repo search (deferred - future enhancement)

### Phase 6: SQLite Compatibility
- [x] Keep single table for SQLite (conditional in `tableName()`) - already done
- [x] Test SQLite path still works - verified via existing tests

### Phase 7: Migration Tool
- [x] Deferred - automatic dimension detection handles most cases
- [x] Users can clear embeddings and re-embed if needed
- [ ] Future: Add `codetect migrate-embeddings` for complex migrations

## Progress Notes

### Phases 1-3 Complete

**Changes to `internal/embedding/store.go`:**
- Added `tableNameForDimensions(dialect, dim)` helper - returns `embeddings_768`, `embeddings_1024`, etc. for PostgreSQL
- Added `tableName()` method on EmbeddingStore - uses dimension-specific table for Postgres, single table for SQLite
- Added `initRepoConfigTable()` - creates `repo_embedding_configs` table for tracking model/dimensions per repo
- Added `RepoEmbeddingConfig` struct with `GetRepoConfig()`, `SetRepoConfig()`, `ListRepoConfigs()` methods
- Updated ALL query methods to use `s.tableName()` instead of hardcoded "embeddings":
  - `Save()`, `SaveBatch()`, `GetByPath()`, `GetAll()`, `HasEmbedding()`
  - `DeleteByPath()`, `DeleteAll()`, `Count()`, `Stats()`
- Schema initialization creates dimension-specific tables with dimension-specific index names

**All tests pass** (`make test`)

### Phase 4 Complete

**Changes to `internal/embedding/store.go`:**
- Added `CheckDimensionMismatch()` - detects if repo has existing embeddings with different dimensions
- Added `DeleteFromDimensionTable()` - deletes repo embeddings from a specific dimension table
- Added `MigrateRepoDimensions()` - handles full migration (delete old + update config)
- Added `VectorDimensions()` - returns configured dimensions for the store

**Changes to `cmd/codetect-index/main.go`:**
- Added dimension mismatch detection after store creation
- Auto-migrates on dimension change (deletes old, updates config)
- Updates repo config after successful embedding

### Phase 5 Complete

**Changes to `internal/embedding/store.go`:**
- Added `RepoRoot` field to `EmbeddingRecord` (for cross-repo results)
- Added `GetAllAcrossRepos(repoRoots)` - queries all repos in dimension group
- Added `scanEmbeddingRecordsWithRepo()` - scans rows with repo_root column

**Changes to `internal/embedding/search.go`:**
- Added `CrossRepoSearchResult` type (extends SemanticResult with RepoRoot)
- Added `CrossRepoSearchResponse` type
- Added `SearchAcrossRepos()` method for org-wide semantic search

### Phase 6 & 7 Notes

- SQLite compatibility was already built-in throughout the implementation
- Migration tool deferred since automatic dimension detection handles most cases
- Users with old PostgreSQL data can clear and re-embed via `codetect embed --force`

---

```json
{
  "active_context": ["context/plans/2026-01-24-dimension-grouped-embeddings.md"],
  "completed_summaries": [
    "context/plans/2026-01-24-ast-grep-hybrid-indexer.md",
    "context/plans/2026-01-24-eval-model-selection.md",
    "context/plans/2026-01-23-fix-config-preservation-overwriting-selections.md",
    "context/plans/2026-01-22-installer-config-preservation-and-reembedding.md",
    "context/plans/2026-01-23-parallel-eval-execution.md"
  ],
  "execution_branch": "para/dimension-grouped-embeddings",
  "execution_started": "2026-01-24T12:45:00Z",
  "last_updated": "2026-01-24T12:45:00Z"
}
```
