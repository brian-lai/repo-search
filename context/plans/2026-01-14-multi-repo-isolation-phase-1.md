# Phase 1: Schema Changes for Multi-Repository Isolation

**Parent Plan:** [2026-01-14-multi-repo-isolation.md](./2026-01-14-multi-repo-isolation.md)
**Status:** Pending
**Branch:** `para/multi-repo-isolation-phase-1`

## Objective

Add `repo_root` column to all database tables (symbols, files, embeddings) with proper constraints and indexes to support multi-repository isolation.

## Files to Modify

| File | Changes |
|------|---------|
| `internal/search/symbols/schema.go` | Add repo_root to symbols/files tables, update constraints |
| `internal/embedding/store.go` | Add repo_root to embeddings table, update constraints |

## Implementation Steps

### 1. Update Symbol Index Schema (`internal/search/symbols/schema.go`)

#### 1.1 Update `initSchemaWithAdapter()` - Symbols Table

Add `repo_root` column to symbols table definition:

```go
symbolColumns := []db.ColumnDef{
    {Name: "id", Type: db.ColTypeAutoIncrement},
    {Name: "repo_root", Type: db.ColTypeText, Nullable: false},  // NEW
    {Name: "name", Type: db.ColTypeText, Nullable: false},
    {Name: "kind", Type: db.ColTypeText, Nullable: false},
    {Name: "path", Type: db.ColTypeText, Nullable: false},
    // ... rest unchanged
}
```

#### 1.2 Update Unique Index

Change unique index from `(name, path, line)` to `(repo_root, name, path, line)`:

```go
uniqueIdxSQL := dialect.CreateIndexSQL("symbols", "idx_symbols_unique",
    []string{"repo_root", "name", "path", "line"}, true)
```

#### 1.3 Update Files Table

Add `repo_root` column and make primary key composite:

```go
fileColumns := []db.ColumnDef{
    {Name: "repo_root", Type: db.ColTypeText, Nullable: false},  // NEW
    {Name: "path", Type: db.ColTypeText, Nullable: false},
    {Name: "mtime", Type: db.ColTypeInteger, Nullable: false},
    {Name: "size", Type: db.ColTypeInteger, Nullable: false},
    {Name: "indexed_at", Type: db.ColTypeInteger, Nullable: false},
}
```

Note: Files table needs composite primary key `(repo_root, path)` - may need to create unique index instead since dialect's PrimaryKey only supports single column.

#### 1.4 Add Composite Indexes

Add index for efficient `(repo_root, path)` queries:

```go
idxRepoPath := dialect.CreateIndexSQL("symbols", "idx_symbols_repo_path",
    []string{"repo_root", "path"}, false)
```

#### 1.5 Update SQLite Schema (Backward Compat)

Update the hardcoded SQLite schema string to include `repo_root`:

```sql
CREATE TABLE IF NOT EXISTS symbols (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_root TEXT NOT NULL,
    name TEXT NOT NULL,
    -- ...
    UNIQUE(repo_root, name, path, line)
);

CREATE TABLE IF NOT EXISTS files (
    repo_root TEXT NOT NULL,
    path TEXT NOT NULL,
    mtime INTEGER NOT NULL,
    size INTEGER NOT NULL,
    indexed_at INTEGER NOT NULL,
    PRIMARY KEY (repo_root, path)
);
```

#### 1.6 Increment Schema Version

Change `schemaVersion` from 1 to 2.

### 2. Update Embedding Store Schema (`internal/embedding/store.go`)

#### 2.1 Update `embeddingColumnsForDialect()`

Add `repo_root` column:

```go
return []db.ColumnDef{
    {Name: "id", Type: db.ColTypeAutoIncrement},
    {Name: "repo_root", Type: db.ColTypeText, Nullable: false},  // NEW
    {Name: "path", Type: db.ColTypeText, Nullable: false},
    // ... rest unchanged
}
```

#### 2.2 Update Unique Index

Change from `(path, start_line, end_line, model)` to `(repo_root, path, start_line, end_line, model)`:

```go
idxUnique := s.dialect.CreateIndexSQL("embeddings", "idx_embeddings_unique",
    []string{"repo_root", "path", "start_line", "end_line", "model"}, true)
```

#### 2.3 Update SQLite Schema

Update the hardcoded SQLite embeddings schema:

```sql
CREATE TABLE IF NOT EXISTS embeddings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_root TEXT NOT NULL,
    path TEXT NOT NULL,
    -- ...
    UNIQUE(repo_root, path, start_line, end_line, model)
);
```

#### 2.4 Add `repoRoot` Field to Struct

Add field to `EmbeddingStore` struct:

```go
type EmbeddingStore struct {
    db            db.DB
    dialect       db.Dialect
    schema        *db.SchemaBuilder
    vectorDim     int
    useNativeVec  bool
    repoRoot      string  // NEW
}
```

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Existing data has no `repo_root` | Use `NOT NULL DEFAULT ''` during migration, then update |
| Composite PK not supported in dialect | Use unique index instead of PK for files table |

## Success Criteria

- [ ] Schema creates successfully on fresh PostgreSQL database
- [ ] Schema creates successfully on fresh SQLite database
- [ ] `repo_root` column exists in all three tables
- [ ] Unique constraints include `repo_root`
- [ ] Unit tests pass: `go test ./internal/search/symbols/... ./internal/embedding/...`

## Review Checklist

- [ ] Schema version incremented
- [ ] Both PostgreSQL and SQLite schemas updated
- [ ] Indexes created for `repo_root` queries
- [ ] No breaking changes to existing API signatures (yet)
