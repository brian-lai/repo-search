# Plan: Dimension-Grouped Embedding Tables

**Date:** 2026-01-24
**Branch:** `para/dimension-grouped-embeddings`
**Objective:** Refactor embedding storage to use dimension-grouped tables for org-scale multi-repo support with model flexibility

---

## Problem Statement

Currently, codetect uses a single `embeddings` table with a fixed vector dimension baked into the PostgreSQL schema. This causes:

1. **Dimension mismatch errors** - When users switch models (e.g., nomic-embed-text 768d → bge-m3 1024d), inserts fail with `pq: expected 768 dimensions, not 1024`
2. **No cross-repo search** - While `repo_root` column exists for isolation, there's no way to search across repositories
3. **Scaling issues** - A single table with mixed repos becomes unwieldy at org scale (3000+ repos)

## Solution: Dimension-Grouped Tables

Create separate tables per vector dimension size:
- `embeddings_768` - For nomic-embed-text and other 768-dim models
- `embeddings_1024` - For bge-m3, snowflake-arctic-embed, jina-embeddings-v3, etc.

This enables:
- **Model flexibility** - Users can switch models; data moves to appropriate table
- **Cross-repo search** - Search across all repos using the same dimension group
- **Clean scaling** - Limited table count regardless of repo count
- **No dimension conflicts** - Each table has correct vector dimensions

---

## Schema Design

### New Table: `repo_embedding_configs`

Tracks which model/dimensions each repository uses:

```sql
CREATE TABLE repo_embedding_configs (
    repo_root TEXT PRIMARY KEY,
    model TEXT NOT NULL,
    dimensions INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

### Dimension-Specific Tables

```sql
-- 768-dimension embeddings (nomic-embed-text, etc.)
CREATE TABLE embeddings_768 (
    id BIGSERIAL PRIMARY KEY,
    repo_root TEXT NOT NULL,
    path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    content_hash TEXT NOT NULL,
    embedding vector(768) NOT NULL,
    model TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    UNIQUE(repo_root, path, start_line, end_line, model)
);

CREATE INDEX idx_embeddings_768_repo ON embeddings_768(repo_root);
CREATE INDEX idx_embeddings_768_path ON embeddings_768(repo_root, path);

-- 1024-dimension embeddings (bge-m3, snowflake, jina, etc.)
CREATE TABLE embeddings_1024 (
    id BIGSERIAL PRIMARY KEY,
    repo_root TEXT NOT NULL,
    path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    content_hash TEXT NOT NULL,
    embedding vector(1024) NOT NULL,
    model TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    UNIQUE(repo_root, path, start_line, end_line, model)
);

CREATE INDEX idx_embeddings_1024_repo ON embeddings_1024(repo_root);
CREATE INDEX idx_embeddings_1024_path ON embeddings_1024(repo_root, path);
```

### Supported Dimension Groups

Based on current model offerings:
- **768**: nomic-embed-text
- **1024**: bge-m3, snowflake-arctic-embed, jina-embeddings-v3

Future-proofing: Add tables dynamically when new dimension sizes are needed (e.g., 384, 1536, 3072 for OpenAI models).

---

## Implementation Phases

### Phase 1: Database Schema Updates

**Files to modify:**
- `internal/db/dialect_postgres.go` - Add helper for dimension-specific table names
- `internal/embedding/store.go` - Core refactor for dimension-grouped tables

**Tasks:**
1. Add `tableNameForDimensions(dim int) string` helper function
2. Add `repo_embedding_configs` table schema
3. Modify `initSchema()` to create dimension-specific tables
4. Add migration logic for existing `embeddings` table

**Table naming convention:**
```go
func tableNameForDimensions(dim int) string {
    return fmt.Sprintf("embeddings_%d", dim)
}
```

### Phase 2: EmbeddingStore Refactor

**Files to modify:**
- `internal/embedding/store.go` - All CRUD operations

**Current behavior:**
```go
// All operations target single "embeddings" table
s.dialect.CreateTableSQL("embeddings", columns)
```

**New behavior:**
```go
// Operations target dimension-specific table
tableName := tableNameForDimensions(s.vectorDim)
s.dialect.CreateTableSQL(tableName, columns)
```

**Methods to update:**
1. `initSchema()` - Create correct dimension table
2. `Save()` - Insert into correct table
3. `SaveBatch()` - Batch insert into correct table
4. `Search()` - Query correct table
5. `Delete()` - Delete from correct table
6. `DeleteAll()` - Delete repo's entries from correct table
7. `GetByPath()` - Query correct table
8. `Count()` - Count in correct table

### Phase 3: Repo Config Management

**Files to modify:**
- `internal/embedding/store.go` - Add config tracking
- `internal/embedding/repo_config.go` (new) - Repo config management

**New functions:**
```go
// GetRepoConfig returns the embedding config for a repository
func (s *EmbeddingStore) GetRepoConfig(repoRoot string) (*RepoEmbeddingConfig, error)

// SetRepoConfig updates the embedding config for a repository
func (s *EmbeddingStore) SetRepoConfig(repoRoot, model string, dimensions int) error

// ListRepoConfigs returns all repo configurations (for admin tools)
func (s *EmbeddingStore) ListRepoConfigs() ([]RepoEmbeddingConfig, error)
```

**RepoEmbeddingConfig struct:**
```go
type RepoEmbeddingConfig struct {
    RepoRoot   string    `json:"repo_root"`
    Model      string    `json:"model"`
    Dimensions int       `json:"dimensions"`
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`
}
```

### Phase 4: Model Switch Handling

**Files to modify:**
- `internal/embedding/store.go` - Add migration logic
- `cmd/codetect-index/main.go` - Handle model switches

**When user switches models:**
1. Detect dimension change (old config vs new embedder dimensions)
2. Delete all entries for repo from OLD dimension table
3. Update repo config to new model/dimensions
4. Re-embed into NEW dimension table

**New function:**
```go
// MigrateRepoDimensions handles switching a repo to a different dimension group
func (s *EmbeddingStore) MigrateRepoDimensions(repoRoot string, oldDim, newDim int) error {
    // 1. Delete from old table
    oldTable := tableNameForDimensions(oldDim)
    _, err := s.db.Exec(fmt.Sprintf("DELETE FROM %s WHERE repo_root = $1", oldTable), repoRoot)

    // 2. Update config (embeddings will be inserted by re-indexing)
    // Note: Actual embeddings inserted by subsequent embed command
}
```

### Phase 5: Cross-Repo Search

**Files to modify:**
- `internal/embedding/search.go` - Add cross-repo search
- `internal/mcp/server.go` - Expose via MCP tool (optional)

**New search modes:**
```go
// SearchOptions configures semantic search behavior
type SearchOptions struct {
    RepoRoot    string   // Empty = search all repos in dimension group
    RepoRoots   []string // Search specific repos (empty = all)
    Limit       int
    MinScore    float64
}

// SearchAcrossRepos searches all repos using the same dimension model
func (s *SemanticSearcher) SearchAcrossRepos(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error)
```

### Phase 6: SQLite Compatibility

**Files to modify:**
- `internal/embedding/store.go` - Ensure SQLite still works

SQLite doesn't have native vector type, so dimension grouping is less critical. Options:
1. **Keep single table for SQLite** - JSON storage doesn't have dimension constraints
2. **Use same pattern** - Consistency across backends

**Recommendation:** Keep single table for SQLite (simpler), dimension-grouped for PostgreSQL only.

```go
func (s *EmbeddingStore) tableName() string {
    if s.dialect.Name() == "postgres" {
        return tableNameForDimensions(s.vectorDim)
    }
    return "embeddings" // SQLite uses single table
}
```

### Phase 7: Migration Tool

**Files to modify:**
- `cmd/codetect/main.go` - Add migrate subcommand
- `internal/embedding/migrate.go` (new) - Migration logic

**New command:**
```bash
codetect migrate-embeddings [--from-table embeddings] [--dry-run]
```

**Migration steps:**
1. Read all records from old `embeddings` table
2. Group by repo_root
3. Determine dimensions for each repo (from embedding vector length or model name)
4. Insert into appropriate dimension-grouped table
5. Create repo_embedding_configs entries
6. Optionally drop old table

---

## File Changes Summary

### New Files
- `internal/embedding/repo_config.go` - Repo config management
- `internal/embedding/migrate.go` - Migration utilities

### Modified Files
- `internal/embedding/store.go` - Major refactor for dimension-grouped tables
- `internal/embedding/search.go` - Cross-repo search support
- `internal/db/dialect_postgres.go` - Table name helpers (minor)
- `cmd/codetect-index/main.go` - Model switch detection
- `cmd/codetect/main.go` - Add migrate-embeddings command
- `install.sh` - Update dimension mismatch handling

---

## Testing Plan

### Unit Tests
1. `TestTableNameForDimensions` - Verify naming convention
2. `TestRepoConfigCRUD` - Create/read/update repo configs
3. `TestDimensionGroupedInsert` - Insert into correct table
4. `TestDimensionGroupedSearch` - Search correct table
5. `TestModelSwitchMigration` - Data moves between tables
6. `TestCrossRepoSearch` - Search across multiple repos

### Integration Tests
1. **Multi-repo PostgreSQL test** - Two repos, different models, verify isolation
2. **Model switch test** - Switch repo from 768→1024, verify migration
3. **Cross-repo search test** - Search across repos with same dimensions
4. **SQLite fallback test** - Verify SQLite still works with single table

### Manual Testing
1. Fresh install with bge-m3 (1024) - verify `embeddings_1024` created
2. Switch to nomic-embed-text (768) - verify data migrates to `embeddings_768`
3. Index second repo with bge-m3 - verify both repos searchable together
4. Run cross-repo search - verify results from multiple repos

---

## Rollout Strategy

### Phase 1: Backward Compatible Release
- New tables created alongside existing `embeddings` table
- Existing installations continue working
- New installs use dimension-grouped tables
- Add `codetect migrate-embeddings` command

### Phase 2: Migration Period
- Encourage users to run migration
- Document migration process
- Monitor for issues

### Phase 3: Deprecation
- Warn if old `embeddings` table detected
- Auto-migrate on `codetect embed` if old table exists
- Eventually remove old table support

---

## Success Criteria

- ✅ No dimension mismatch errors when switching models
- ✅ Cross-repo search works within dimension groups
- ✅ Existing SQLite users unaffected
- ✅ Migration path from old schema is smooth
- ✅ Repo configs accurately track model/dimensions per repo
- ✅ Performance comparable to current implementation
- ✅ 3000+ repo scale is supported without table proliferation

---

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Migration corrupts data | Backup before migration, dry-run mode, transaction rollback |
| Performance regression | Benchmark before/after, indexes on repo_root |
| SQLite compatibility breaks | Keep SQLite path separate, thorough testing |
| Cross-repo search too slow | Add HNSW indexes per dimension table |
| Unknown dimension sizes | Dynamic table creation for new dimensions |

---

## Open Questions

1. **Should we support cross-dimension search?** (Probably not - results aren't comparable)
2. **Should SQLite also use dimension-grouped tables?** (Leaning no - unnecessary complexity)
3. **What happens if a model has unusual dimensions (e.g., 384, 1536)?** (Dynamic table creation)
4. **Should we add a "default org model" config for Justworks deployment?** (Future enhancement)

---

## Estimated Effort

| Phase | Complexity | Notes |
|-------|------------|-------|
| Phase 1: Schema | Medium | Core table changes |
| Phase 2: Store refactor | High | Many methods to update |
| Phase 3: Repo config | Low | New table, simple CRUD |
| Phase 4: Model switch | Medium | Migration logic |
| Phase 5: Cross-repo search | Medium | New search mode |
| Phase 6: SQLite compat | Low | Conditional logic |
| Phase 7: Migration tool | Medium | One-time migration |

**Total: ~2-3 focused sessions**

---

## References

- Current store implementation: `internal/embedding/store.go`
- PostgreSQL dialect: `internal/db/dialect_postgres.go`
- Installer model selection: `install.sh:424-459`
- PR #28 dimension mismatch detection: Already merged
