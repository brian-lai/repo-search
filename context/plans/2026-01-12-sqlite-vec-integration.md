# Plan: sqlite-vec Integration for Vector Search

## Objective

Replace brute-force in-memory vector similarity search with native SQLite vector search using the `sqlite-vec` extension. This should significantly reduce semantic search latency by eliminating the need to load all embeddings into memory and compute similarity in Go.

## Background

**Current implementation:**
- Embeddings stored as JSON TEXT in SQLite (`internal/embedding/store.go:30-44`)
- `GetAll()` loads every embedding into memory
- `TopKByCosineSimilarity()` computes O(n) brute-force similarity in Go (`internal/embedding/math.go:89-119`)
- Each semantic search scans all embeddings

**Eval results showing latency issue:**
- With MCP: 42.6s avg latency
- Without MCP: 31.0s avg latency
- **37.6% latency increase** with semantic search enabled

**sqlite-vec benefits:**
- Native KNN queries in SQL (`vec_distance_cosine`)
- Virtual table (`vec0`) with optimized vector storage
- No need to load all vectors into application memory
- Potential for approximate nearest neighbor (ANN) with larger datasets

## Approach

### Step 1: Research & Dependency Setup

1. Evaluate Go integration options for sqlite-vec:
   - Option A: Use `github.com/asg017/sqlite-vec-go-bindings` (official bindings)
   - Option B: Load extension via `mattn/go-sqlite3` with `sqlite3_load_extension`
   - Option C: Use CGO to compile sqlite-vec statically

2. Add dependency to `go.mod`

3. Verify extension loads correctly with a test

### Step 2: Schema Migration

Create new vec0 virtual table alongside existing table:

```sql
-- New vec0 virtual table for vector search
CREATE VIRTUAL TABLE IF NOT EXISTS embeddings_vec USING vec0(
    embedding float[768]  -- Adjust dimensions based on model
);

-- Metadata table (path, lines, hash remain in regular table)
CREATE TABLE IF NOT EXISTS embeddings_meta (
    id INTEGER PRIMARY KEY,
    rowid_vec INTEGER NOT NULL,  -- Reference to vec0 row
    path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    content_hash TEXT NOT NULL,
    model TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE(path, start_line, end_line, model)
);
```

### Step 3: Update EmbeddingStore

Modify `internal/embedding/store.go`:

1. Update `NewEmbeddingStore()` to:
   - Load sqlite-vec extension
   - Create vec0 virtual table
   - Handle dimension detection from embedder

2. Update `Save()` / `SaveBatch()` to:
   - Insert vector into `embeddings_vec`
   - Insert metadata into `embeddings_meta` with rowid reference

3. Add new method `SearchKNN(queryVec []float32, k int)` that:
   - Uses native vec0 KNN query
   - Returns results with metadata joined

4. Keep `GetAll()` for backward compatibility (used by indexer stats)

### Step 4: Update SemanticSearcher

Modify `internal/embedding/search.go`:

1. Replace brute-force search in `SearchWithContext()`:
   ```go
   // OLD: Load all, compute in Go
   records, _ := s.store.GetAll()
   topK := TopKByCosineSimilarity(queryEmbedding, vectors, limit)

   // NEW: Native KNN query
   results, _ := s.store.SearchKNN(queryEmbedding, limit)
   ```

2. Remove dependency on `TopKByCosineSimilarity` for search (keep for tests/benchmarks)

### Step 5: Data Migration

1. Create migration function to copy existing embeddings:
   - Read from old `embeddings` table
   - Insert into new `embeddings_vec` + `embeddings_meta`
   - Verify counts match

2. Add migration check to `NewEmbeddingStore()`:
   - Detect if old schema exists without new
   - Run migration automatically on first access
   - Log migration progress

### Step 6: Testing & Validation

1. Add unit tests for:
   - vec0 extension loading
   - KNN search correctness vs brute-force
   - Migration from old schema

2. Run evals to measure latency improvement:
   - Compare before/after latency
   - Verify accuracy is unchanged

## Files to Modify

| File | Changes |
|------|---------|
| `go.mod` | Add sqlite-vec dependency |
| `internal/embedding/store.go` | vec0 schema, KNN search, migration |
| `internal/embedding/search.go` | Use native KNN instead of brute-force |
| `internal/embedding/store_test.go` | New tests for vec0 functionality |
| `docs/architecture.md` | Update to reflect vec0 usage |

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| sqlite-vec Go bindings immature | Test thoroughly; fallback to brute-force if extension fails to load |
| Dimension mismatch between models | Store dimensions in metadata; validate on load |
| Migration corrupts existing data | Keep old table until migration verified; add rollback |
| CGO compilation issues on different platforms | Document build requirements; provide pre-built binaries |
| Extension not available on user's system | Graceful degradation: fall back to brute-force if vec0 unavailable |

## Data Sources

- Existing embeddings in `.codetect/symbols.db`
- sqlite-vec documentation: https://github.com/asg017/sqlite-vec
- Current eval results in `evals/results/`

## Success Criteria

- [ ] sqlite-vec extension loads successfully on macOS and Linux
- [ ] Semantic search uses native KNN queries (no brute-force)
- [ ] Existing embeddings migrate without data loss
- [ ] Latency reduction of 30%+ on semantic search operations
- [ ] Eval accuracy remains unchanged (within 1%)
- [ ] Graceful fallback if extension unavailable

## Review Checklist

- [ ] Dependency choice justified (official bindings vs load_extension)
- [ ] Migration strategy handles edge cases (empty DB, partial migration)
- [ ] Backward compatibility maintained for users without new index
- [ ] Build/install instructions updated
- [ ] Tests cover KNN correctness and migration

## Open Questions

1. **Dimension handling**: Should we support multiple embedding dimensions (e.g., 384 vs 768 vs 1536)? Current approach assumes single dimension.

2. **Approximate vs exact**: sqlite-vec supports both exact and approximate KNN. Should we default to approximate for speed, or exact for accuracy?

3. **Hybrid table design**: Should metadata stay in separate table (current plan) or use vec0's auxiliary column feature?

---

*Created: 2026-01-12*
*Status: Pending Review*
