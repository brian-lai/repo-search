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
- [ ] Add `SearchOptions` struct with `RepoRoots` filter
- [ ] Implement `SearchAcrossRepos()` method
- [ ] Update MCP tool to expose cross-repo search (optional)

### Phase 6: SQLite Compatibility
- [ ] Keep single table for SQLite (conditional in `tableName()`)
- [ ] Test SQLite path still works

### Phase 7: Migration Tool
- [ ] Add `codetect migrate-embeddings` command
- [ ] Implement migration from old `embeddings` table
- [ ] Add `--dry-run` flag for safety

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

---
```json
{
  "active_context": ["context/plans/2026-01-24-dimension-grouped-embeddings.md"],
  "completed_summaries": [
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
