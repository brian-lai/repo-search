# Phase 2: Query Updates for Multi-Repository Isolation

**Parent Plan:** [2026-01-14-multi-repo-isolation.md](./2026-01-14-multi-repo-isolation.md)
**Status:** Pending
**Branch:** `para/multi-repo-isolation-phase-2`
**Depends On:** Phase 1 (Schema Changes)

## Objective

Update all database queries in symbol index and embedding store to filter by `repo_root`, ensuring complete data isolation between repositories.

## Files to Modify

| File | Changes |
|------|---------|
| `internal/search/symbols/index.go` | Update all queries to include `repo_root` |
| `internal/embedding/store.go` | Update all queries to include `repo_root` |

## Implementation Steps

### 1. Update Symbol Index Queries (`internal/search/symbols/index.go`)

#### 1.1 Update `Index` Struct

The `root` field already exists - ensure it's used as `repoRoot`:

```go
type Index struct {
    sqlDB   *sql.DB
    adapter db.DB
    dialect db.Dialect
    dbPath  string
    root    string  // This is our repo_root
}
```

#### 1.2 Update `FindSymbol()`

Add `repo_root` filter:

```go
// With kind filter
query = fmt.Sprintf(`SELECT name, kind, path, line, language, pattern, scope
    FROM symbols
    WHERE repo_root = %s AND name LIKE %s AND kind = %s
    ORDER BY ...`,
    idx.dialect.Placeholder(1),
    idx.dialect.Placeholder(2),
    idx.dialect.Placeholder(3), ...)
args = []any{idx.root, pattern, kind, name, name + "%", limit}

// Without kind filter
query = fmt.Sprintf(`SELECT name, kind, path, line, language, pattern, scope
    FROM symbols
    WHERE repo_root = %s AND name LIKE %s
    ORDER BY ...`,
    idx.dialect.Placeholder(1),
    idx.dialect.Placeholder(2), ...)
args = []any{idx.root, pattern, name, name + "%", limit}
```

#### 1.3 Update `ListDefsInFile()`

Add `repo_root` filter:

```go
query := fmt.Sprintf(`SELECT name, kind, path, line, language, pattern, scope
    FROM symbols
    WHERE repo_root = %s AND path = %s
    ORDER BY line`,
    idx.dialect.Placeholder(1),
    idx.dialect.Placeholder(2))
// args: idx.root, path
```

#### 1.4 Update `Update()` Method

##### Delete statement:
```go
deleteQuery := fmt.Sprintf("DELETE FROM symbols WHERE repo_root = %s AND path = %s",
    idx.dialect.Placeholder(1),
    idx.dialect.Placeholder(2))
// args: idx.root, path
```

##### Symbol upsert - add `repo_root` to columns:
```go
symbolUpsertSQL := idx.dialect.UpsertSQL(
    "symbols",
    []string{"repo_root", "name", "kind", "path", "line", "language", "pattern", "scope", "signature"},
    []string{"repo_root", "name", "path", "line"},  // Updated conflict columns
    []string{"kind", "language", "pattern", "scope", "signature"},
)
// Exec with: idx.root, sym.Name, sym.Kind, sym.Path, ...
```

##### File upsert - add `repo_root` to columns:
```go
fileUpsertSQL := idx.dialect.UpsertSQL(
    "files",
    []string{"repo_root", "path", "mtime", "size", "indexed_at"},
    []string{"repo_root", "path"},  // Updated conflict columns
    []string{"mtime", "size", "indexed_at"},
)
// Exec with: idx.root, path, info.mtime, info.size, now
```

#### 1.5 Update `FullReindex()`

Scope deletion to repo:

```go
deleteSymbolsQuery := fmt.Sprintf("DELETE FROM symbols WHERE repo_root = %s",
    idx.dialect.Placeholder(1))
if _, err := idx.adapter.Exec(deleteSymbolsQuery, idx.root); err != nil {
    return fmt.Errorf("clearing symbols: %w", err)
}

deleteFilesQuery := fmt.Sprintf("DELETE FROM files WHERE repo_root = %s",
    idx.dialect.Placeholder(1))
if _, err := idx.adapter.Exec(deleteFilesQuery, idx.root); err != nil {
    return fmt.Errorf("clearing files: %w", err)
}
```

#### 1.6 Update `getFilesToIndex()`

Add `repo_root` filter:

```go
query := fmt.Sprintf("SELECT path, mtime, size FROM files WHERE repo_root = %s",
    idx.dialect.Placeholder(1))
rows, err := idx.adapter.Query(query, idx.root)
```

#### 1.7 Update `Stats()`

Scope stats to repo:

```go
symbolQuery := fmt.Sprintf("SELECT COUNT(*) FROM symbols WHERE repo_root = %s",
    idx.dialect.Placeholder(1))
if err := idx.adapter.QueryRow(symbolQuery, idx.root).Scan(&symbolCount); err != nil {
    return 0, 0, err
}

fileQuery := fmt.Sprintf("SELECT COUNT(*) FROM files WHERE repo_root = %s",
    idx.dialect.Placeholder(1))
if err := idx.adapter.QueryRow(fileQuery, idx.root).Scan(&fileCount); err != nil {
    return 0, 0, err
}
```

### 2. Update Embedding Store Queries (`internal/embedding/store.go`)

#### 2.1 Update `Save()`

Add `repo_root` to upsert:

```go
columns := []string{"repo_root", "path", "start_line", "end_line", "content_hash", "embedding", "model", "created_at"}
conflictColumns := []string{"repo_root", "path", "start_line", "end_line", "model"}
updateColumns := []string{"content_hash", "embedding", "created_at"}

sql := s.dialect.UpsertSQL("embeddings", columns, conflictColumns, updateColumns)
// Exec with: s.repoRoot, chunk.Path, ...
```

#### 2.2 Update `SaveBatch()`

Same changes as `Save()`.

#### 2.3 Update `GetByPath()`

Add `repo_root` filter:

```go
query := s.schema.SubstitutePlaceholders(`
    SELECT id, path, start_line, end_line, content_hash, embedding, model, created_at
    FROM embeddings
    WHERE repo_root = ? AND path = ?
    ORDER BY start_line`)
rows, err := s.db.Query(query, s.repoRoot, path)
```

#### 2.4 Update `GetAll()`

Add `repo_root` filter:

```go
query := s.schema.SubstitutePlaceholders(`
    SELECT id, path, start_line, end_line, content_hash, embedding, model, created_at
    FROM embeddings
    WHERE repo_root = ?
    ORDER BY path, start_line`)
rows, err := s.db.Query(query, s.repoRoot)
```

#### 2.5 Update `HasEmbedding()`

Add `repo_root` filter:

```go
query := s.schema.SubstitutePlaceholders(`
    SELECT COUNT(*) FROM embeddings
    WHERE repo_root = ? AND path = ? AND start_line = ? AND end_line = ?
    AND content_hash = ? AND model = ?`)
err := s.db.QueryRow(query, s.repoRoot, chunk.Path, ...).Scan(&count)
```

#### 2.6 Update `DeleteByPath()`

Add `repo_root` filter:

```go
query := s.schema.SubstitutePlaceholders("DELETE FROM embeddings WHERE repo_root = ? AND path = ?")
_, err := s.db.Exec(query, s.repoRoot, path)
```

#### 2.7 Update `DeleteAll()`

Scope to repo:

```go
query := s.schema.SubstitutePlaceholders("DELETE FROM embeddings WHERE repo_root = ?")
_, err := s.db.Exec(query, s.repoRoot)
```

#### 2.8 Update `Stats()`

Scope to repo:

```go
query := s.schema.SubstitutePlaceholders(
    "SELECT COUNT(*), COUNT(DISTINCT path) FROM embeddings WHERE repo_root = ?")
err = s.db.QueryRow(query, s.repoRoot).Scan(&count, &fileCount)
```

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Missed query causes cross-repo data leak | Comprehensive grep for all SQL queries |
| Placeholder numbering errors | Careful review, test each query |
| `idx.root` not set | Validate in constructor or early in methods |

## Success Criteria

- [ ] All symbol index queries include `repo_root` filter
- [ ] All embedding store queries include `repo_root` filter
- [ ] Upsert conflict columns include `repo_root`
- [ ] Delete operations scoped to `repo_root`
- [ ] Stats methods scoped to `repo_root`
- [ ] Unit tests pass

## Review Checklist

- [ ] Every `SELECT` has `WHERE repo_root = ?`
- [ ] Every `DELETE` has `WHERE repo_root = ?`
- [ ] Every `INSERT/UPSERT` includes `repo_root` column
- [ ] Placeholder numbers are correct for each dialect
- [ ] No hardcoded queries without repo_root filter
