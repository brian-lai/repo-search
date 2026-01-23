# PostgreSQL + pgvector Support Implementation Plan

## Objective
Add PostgreSQL with pgvector extension support to codetect for efficient vector similarity search at scale, replacing the current SQLite + brute-force approach when performance is needed.

## Current State

### What Works
- SQLite database with embeddings stored as JSON-encoded vectors
- Brute-force semantic search (O(n) scan of all vectors)
- Multi-database adapter layer with dialect abstraction
- PostgreSQL dialect stub (SQL generation only, no driver)
- Configuration system supporting multiple database types

### Performance Characteristics
**Current (SQLite + Brute-Force):**
- Search time: O(n) linear scan
- Memory: All embeddings loaded into memory per query
- Suitable for: ~10k embeddings (< 100MB with 768-dim vectors)

**Target (PostgreSQL + pgvector):**
- Search time: O(log n) with HNSW index or O(n) with IVFFlat
- Memory: Server-side search, minimal client memory
- Suitable for: Millions of embeddings, large codebases

## Architecture Overview

### Database Layer
```
Config → Open() → DB interface → Dialect implementation
                                 ↓
                    SQLWrapper (sql.DB adapter)
                                 ↓
                    PostgreSQL driver (lib/pq or pgx)
```

### Vector Search Layer
```
SemanticSearcher → VectorDB interface → PgVectorDB implementation
                                        ↓
                            pgvector extension (<-> operator)
```

### Key Files
- `internal/db/open.go` - Database routing (needs PostgreSQL driver)
- `internal/db/dialect_postgres.go` - SQL generation (complete)
- `internal/db/vector.go` - VectorDB interface (needs pgvector impl)
- `internal/embedding/store.go` - Embedding storage (needs vector type support)
- `internal/embedding/search.go` - SemanticSearcher (needs config to choose VectorDB)

## Implementation Plan

### Phase 1: PostgreSQL Driver Support
**Goal:** Enable basic PostgreSQL connectivity

**Tasks:**
1. Add PostgreSQL driver dependency (`github.com/lib/pq` or `github.com/jackc/pgx/v5`)
2. Implement `openPostgres()` in `internal/db/open.go`
3. Add connection string validation and parsing
4. Test basic CRUD operations with PostgreSQL dialect
5. Verify placeholder substitution ($1, $2, etc.) works correctly

**Files to modify:**
- `internal/db/open.go:48-60` - Remove stub, add driver implementation
- `go.mod` - Add PostgreSQL driver dependency

**Success criteria:**
- Can connect to PostgreSQL with DSN
- Can create tables using PostgreSQL dialect
- Can insert/query data with parameterized queries

### Phase 2: pgvector Extension Setup
**Goal:** Add vector type support to PostgreSQL schema

**Tasks:**
1. Add pgvector extension initialization to PostgreSQL dialect
2. Extend schema builder to create vector columns
3. Update embeddings table schema for vector type
4. Implement migration path from TEXT to vector column

**Files to modify:**
- `internal/db/dialect_postgres.go` - Add `CREATE EXTENSION IF NOT EXISTS vector`
- `internal/db/schema.go` - Add vector column type support
- `internal/db/adapter.go` - Add vector dimensions to Config

**New schema:**
```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE embeddings (
    id SERIAL PRIMARY KEY,
    path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    content_hash TEXT NOT NULL,
    embedding vector(768),  -- pgvector type
    model TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    UNIQUE(path, start_line, end_line, model)
);
```

**Success criteria:**
- PostgreSQL database has pgvector extension enabled
- Can create embeddings table with vector column
- Can insert vector data using Go pgvector types

### Phase 3: pgvector VectorDB Implementation
**Goal:** Implement efficient vector search using pgvector operators

**Tasks:**
1. Create `PgVectorDB` struct implementing `VectorDB` interface
2. Implement `SearchKNN()` using pgvector distance operators
3. Add support for multiple distance metrics (cosine, L2, inner product)
4. Implement vector index creation (IVFFlat or HNSW)
5. Add batch embedding insertion optimized for PostgreSQL

**Files to create/modify:**
- `internal/db/vector_pgvector.go` - New file with PgVectorDB implementation
- `internal/db/vector.go` - Update interface if needed

**pgvector operators:**
- `<->` - L2 distance (Euclidean)
- `<#>` - Negative inner product (for similarity)
- `<=>` - Cosine distance

**Example query:**
```sql
SELECT id, path, start_line, end_line,
       embedding <=> $1::vector AS distance
FROM embeddings
WHERE model = $2
ORDER BY embedding <=> $1::vector
LIMIT $3;
```

**Index options:**
```sql
-- IVFFlat (faster build, good for medium datasets)
CREATE INDEX embedding_idx ON embeddings
USING ivfflat (embedding vector_cosine_ops)
WITH (lists = 100);

-- HNSW (slower build, better accuracy and speed)
CREATE INDEX embedding_idx ON embeddings
USING hnsw (embedding vector_cosine_ops);
```

**Success criteria:**
- Can perform KNN search with pgvector operators
- Search is significantly faster than brute-force for large datasets
- Distance metrics match brute-force results (within floating point precision)

### Phase 4: EmbeddingStore Integration
**Goal:** Update embedding storage to use vector type

**Tasks:**
1. Add vector type encoding/decoding in EmbeddingStore
2. Update upsert logic to handle vector columns
3. Add PostgreSQL-specific batch insertion optimizations
4. Implement embedding migration tool (SQLite → PostgreSQL)

**Files to modify:**
- `internal/embedding/store.go` - Add vector type support
- `internal/embedding/store.go` - Update batch insertion for PostgreSQL

**Implementation notes:**
- pgvector vectors are stored natively, no JSON encoding needed
- Use prepared statements for batch insertion
- Consider COPY protocol for large bulk loads

**Success criteria:**
- Can store embeddings in PostgreSQL with vector type
- Batch insertion is performant (1000+ vectors/sec)
- Can migrate existing SQLite embeddings to PostgreSQL

### Phase 5: SemanticSearcher Configuration
**Goal:** Make database backend configurable

**Tasks:**
1. Add database configuration to MCP server initialization
2. Update `openSemanticSearcher()` to choose VectorDB based on config
3. Add environment variables for PostgreSQL connection
4. Implement automatic database detection from DSN
5. Add fallback logic (PostgreSQL → SQLite if unavailable)

**Files to modify:**
- `cmd/codetect/main.go` - Add database config
- `internal/tools/semantic.go` - Update openSemanticSearcher()
- `internal/embedding/search.go` - Support multiple VectorDB backends

**Environment variables:**
```bash
CODETECT_DB_TYPE=postgres|sqlite
CODETECT_DB_DSN=postgresql://user:pass@localhost/codetect
CODETECT_DB_PATH=.codetect/symbols.db  # fallback for SQLite
CODETECT_VECTOR_DIMENSIONS=768
```

**Success criteria:**
- Can configure PostgreSQL via environment variables
- Seamless fallback to SQLite if PostgreSQL unavailable
- MCP tools work with both database backends

### Phase 6: Testing & Benchmarking
**Goal:** Verify correctness and performance improvements

**Tasks:**
1. Create test suite for PostgreSQL adapter
2. Create test suite for pgvector search
3. Benchmark brute-force vs pgvector search
4. Test with large embedding datasets (100k+ vectors)
5. Verify search result consistency across backends

**Files to create:**
- `internal/db/postgres_test.go`
- `internal/db/vector_pgvector_test.go`
- `internal/embedding/search_benchmark_test.go`

**Benchmark scenarios:**
- 1k, 10k, 100k, 1M embeddings
- Different query patterns (common vs rare)
- Different vector dimensions (384, 768, 1536)

**Success criteria:**
- All tests pass
- pgvector is 10x+ faster than brute-force at 100k+ embeddings
- Results are consistent (same top-k, similar scores)

### Phase 7: Documentation & Tooling
**Goal:** Make PostgreSQL setup easy for users

**Tasks:**
1. Document PostgreSQL + pgvector installation
2. Document configuration options
3. Create migration script (SQLite → PostgreSQL)
4. Add docker-compose.yml for easy PostgreSQL setup
5. Update README with performance comparison

**Files to create/modify:**
- `docs/postgres-setup.md` - Installation guide
- `scripts/migrate-to-postgres.go` - Migration tool
- `docker-compose.yml` - PostgreSQL + pgvector container
- `README.md` - Add PostgreSQL section

**Docker setup example:**
```yaml
version: '3.8'
services:
  postgres:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_DB: codetect
      POSTGRES_USER: codetect
      POSTGRES_PASSWORD: codetect
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
```

**Success criteria:**
- Users can set up PostgreSQL + pgvector in < 5 minutes
- Migration from SQLite is one command
- Documentation is clear and complete

## Technical Decisions

### PostgreSQL Driver Choice
**Options:**
1. `lib/pq` - Pure Go, mature, simple
2. `pgx/v5` - Modern, better performance, more features

**Recommendation:** Start with `lib/pq` for simplicity, consider `pgx/v5` if performance needs justify it.

### Vector Index Choice
**Options:**
1. IVFFlat - Faster build time, good for medium datasets
2. HNSW - Better accuracy and query speed, slower build

**Recommendation:** Default to HNSW, provide IVFFlat as option for faster indexing.

### Migration Strategy
**Options:**
1. Parallel support - SQLite and PostgreSQL coexist
2. Full migration - Replace SQLite with PostgreSQL

**Recommendation:** Parallel support. Keep SQLite for simple deployments, PostgreSQL for scale.

### Configuration Approach
**Options:**
1. Environment variables only
2. Config file + environment variables
3. Auto-detection from database file/DSN

**Recommendation:** Environment variables + auto-detection. Simple and flexible.

## Dependencies

### New Go Dependencies
```
github.com/lib/pq         v1.10.9   (PostgreSQL driver)
github.com/pgvector/pgvector-go v0.1.1   (pgvector types)
```

### External Dependencies
- PostgreSQL 12+ with pgvector extension
- Docker (optional, for easy setup)

## Risks & Mitigations

### Risk: pgvector extension not available
**Mitigation:** Fallback to brute-force search with PostgreSQL, or fall back to SQLite

### Risk: Breaking changes to existing users
**Mitigation:** SQLite remains default, PostgreSQL is opt-in

### Risk: Migration complexity
**Mitigation:** Provide automated migration tool with dry-run mode

### Risk: Performance regression
**Mitigation:** Comprehensive benchmarking before merging

## Success Metrics

1. PostgreSQL + pgvector search is 10x+ faster at 100k+ embeddings
2. Setup time < 5 minutes with Docker
3. Zero breaking changes to existing SQLite users
4. All existing tests pass with PostgreSQL backend
5. Search accuracy matches brute-force (top-10 results ≥ 90% overlap)

## Rollout Plan

1. Merge Phase 1-3 (core functionality) in one PR
2. Merge Phase 4-5 (integration) in second PR
3. Merge Phase 6-7 (polish) in final PR
4. Announce PostgreSQL support in release notes
5. Gather user feedback and iterate

## Open Questions

1. Should we support multiple vector index types (IVFFlat vs HNSW)?
2. Should we provide a managed PostgreSQL option (RDS, Supabase)?
3. Should we support other vector databases (Qdrant, Milvus, Weaviate)?
4. Should we add monitoring/observability for vector search performance?
5. Should we support partial updates (update embedding without recreating)?

## Timeline Estimate

- Phase 1: 1 implementation session
- Phase 2: 1 implementation session
- Phase 3: 2 implementation sessions (complex, needs testing)
- Phase 4: 1 implementation session
- Phase 5: 1 implementation session
- Phase 6: 1 implementation session (testing/benchmarking)
- Phase 7: 1 implementation session (documentation)

**Total:** 7-9 implementation sessions
