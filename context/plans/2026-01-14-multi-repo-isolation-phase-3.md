# Phase 3: Command Integration for Multi-Repository Isolation

**Parent Plan:** [2026-01-14-multi-repo-isolation.md](./2026-01-14-multi-repo-isolation.md)
**Status:** Pending
**Branch:** `para/multi-repo-isolation-phase-3`
**Depends On:** Phase 2 (Query Updates)

## Objective

Wire the `repoRoot` parameter through all entry points: CLI commands and MCP tools. After this phase, users can index and query multiple repositories to the same PostgreSQL database with complete isolation.

## Files to Modify

| File | Changes |
|------|---------|
| `internal/search/symbols/index.go` | Update constructors to accept `repoRoot` |
| `internal/embedding/store.go` | Update constructors to accept `repoRoot` |
| `cmd/codetect-index/main.go` | Pass `absPath` as `repoRoot` |
| `internal/tools/symbols.go` | Pass cwd as `repoRoot` |

## Implementation Steps

### 1. Update Symbol Index Constructor (`internal/search/symbols/index.go`)

#### 1.1 Add `repoRoot` to `NewIndexWithConfig()`

Change signature to accept `repoRoot`:

```go
// NewIndexWithConfig creates a symbol index using the provided configuration.
// repoRoot is the absolute path to the repository root, used for multi-repo isolation.
func NewIndexWithConfig(cfg db.Config, repoRoot string) (*Index, error) {
    database, err := db.Open(cfg)
    if err != nil {
        return nil, err
    }

    dialect := cfg.Dialect()

    if err := initSchemaWithAdapter(database, dialect); err != nil {
        database.Close()
        return nil, fmt.Errorf("initializing schema: %w", err)
    }

    return &Index{
        adapter: database,
        dialect: dialect,
        dbPath:  cfg.Path,
        root:    repoRoot,  // Set repo root
    }, nil
}
```

#### 1.2 Update Legacy `NewIndex()` (Optional)

For backward compatibility, default to empty string or current directory:

```go
func NewIndex(dbPath string) (*Index, error) {
    // ... existing code ...
    cwd, _ := os.Getwd()
    return &Index{
        sqlDB:   sqlDB,
        adapter: db.WrapSQL(sqlDB),
        dialect: db.GetDialect(db.DatabaseSQLite),
        dbPath:  dbPath,
        root:    cwd,  // Default to current directory
    }, nil
}
```

### 2. Update Embedding Store Constructor (`internal/embedding/store.go`)

#### 2.1 Update `NewEmbeddingStoreWithOptions()`

Add `repoRoot` parameter:

```go
// NewEmbeddingStoreWithOptions creates an embedding store with custom options.
func NewEmbeddingStoreWithOptions(database db.DB, dialect db.Dialect, vectorDim int, repoRoot string) (*EmbeddingStore, error) {
    useNativeVec := dialect.Name() == "postgres"

    store := &EmbeddingStore{
        db:           database,
        dialect:      dialect,
        schema:       db.NewSchemaBuilder(database, dialect),
        vectorDim:    vectorDim,
        useNativeVec: useNativeVec,
        repoRoot:     repoRoot,  // Set repo root
    }

    if err := store.initSchema(); err != nil {
        return nil, fmt.Errorf("initializing embedding schema: %w", err)
    }

    return store, nil
}
```

#### 2.2 Update Other Constructors

Update `NewEmbeddingStoreWithDialect()` and `NewEmbeddingStore()` to accept and pass `repoRoot`:

```go
func NewEmbeddingStoreWithDialect(database db.DB, dialect db.Dialect, repoRoot string) (*EmbeddingStore, error) {
    return NewEmbeddingStoreWithOptions(database, dialect, 768, repoRoot)
}

func NewEmbeddingStore(database db.DB, repoRoot string) (*EmbeddingStore, error) {
    return NewEmbeddingStoreWithDialect(database, db.GetDialect(db.DatabaseSQLite), repoRoot)
}
```

### 3. Update CLI Commands (`cmd/codetect-index/main.go`)

#### 3.1 Update `runIndex()`

Pass `absPath` as `repoRoot`:

```go
func runIndex(args []string) {
    // ... existing path handling ...
    absPath, err := filepath.Abs(path)
    // ...

    // Open or create index with repo root
    idx, err := symbols.NewIndexWithConfig(cfg, absPath)  // Pass absPath
    if err != nil {
        fmt.Fprintf(os.Stderr, "error: opening index: %v\n", err)
        os.Exit(1)
    }
    defer idx.Close()
    // ...
}
```

#### 3.2 Update `runEmbed()`

Pass `absPath` as `repoRoot` to both index and embedding store:

```go
func runEmbed(args []string) {
    // ... existing path handling ...
    absPath, err := filepath.Abs(path)
    // ...

    // Open index with repo root
    idx, err := symbols.NewIndexWithConfig(dbCfg, absPath)  // Pass absPath
    // ...

    // Create embedding store with repo root
    store, err := embedding.NewEmbeddingStoreWithOptions(
        idx.DBAdapter(),
        idx.Dialect(),
        dbConfig.VectorDimensions,
        absPath,  // Pass absPath as repoRoot
    )
    // ...
}
```

#### 3.3 Update `runStats()`

Pass `absPath` as `repoRoot`:

```go
func runStats(args []string) {
    // ... existing path handling ...
    absPath, err := filepath.Abs(path)
    // ...

    // Open index with repo root
    idx, err := symbols.NewIndexWithConfig(dbCfg, absPath)  // Pass absPath
    // ...

    // Create embedding store with repo root for stats
    store, err := embedding.NewEmbeddingStoreWithOptions(
        idx.DBAdapter(),
        idx.Dialect(),
        dbConfig.VectorDimensions,
        absPath,  // Pass absPath
    )
    // ...
}
```

### 4. Update MCP Tools (`internal/tools/symbols.go`)

#### 4.1 Update `openIndex()`

Pass current working directory as `repoRoot`:

```go
func openIndex() (*symbols.Index, error) {
    dbConfig := config.LoadDatabaseConfigFromEnv()

    // Get current working directory as repo root
    cwd, err := os.Getwd()
    if err != nil {
        return nil, fmt.Errorf("getting working directory: %w", err)
    }

    if dbConfig.Type == db.DatabaseSQLite {
        dbPath := filepath.Join(cwd, ".codetect", "symbols.db")
        if _, err := os.Stat(dbPath); os.IsNotExist(err) {
            return nil, fmt.Errorf("no symbol index found - run 'make index' first")
        }
        dbConfig.Path = dbPath
    }

    cfg := dbConfig.ToDBConfig()
    return symbols.NewIndexWithConfig(cfg, cwd)  // Pass cwd as repoRoot
}
```

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| API signature changes break callers | Update all call sites in same phase |
| Empty repoRoot causes issues | Validate repoRoot is non-empty in constructors |
| Relative path passed instead of absolute | Use `filepath.Abs()` at entry points |

## Success Criteria

- [ ] `NewIndexWithConfig()` accepts and uses `repoRoot`
- [ ] `NewEmbeddingStoreWithOptions()` accepts and uses `repoRoot`
- [ ] `runIndex()` passes `absPath` to index constructor
- [ ] `runEmbed()` passes `absPath` to index and store constructors
- [ ] `runStats()` passes `absPath` to index and store constructors
- [ ] MCP tools pass `cwd` to index constructor
- [ ] End-to-end multi-repo test passes

## Verification

```bash
# Build updated binary
go build -o /tmp/codetect-index ./cmd/codetect-index

# Test multi-repo isolation
export CODETECT_DB_TYPE=postgres
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5465/codetect?sslmode=disable"

# Create two repos with same file
mkdir -p /tmp/repo1 /tmp/repo2
echo 'package main; func hello() {}' > /tmp/repo1/main.go
echo 'package main; func hello() {}' > /tmp/repo2/main.go

# Index both
/tmp/codetect-index index /tmp/repo1
/tmp/codetect-index index /tmp/repo2

# Verify isolation
/tmp/codetect-index stats /tmp/repo1  # Should show ~3 symbols, 1 file
/tmp/codetect-index stats /tmp/repo2  # Should show ~3 symbols, 1 file

# Verify in database
docker exec codetect-postgres psql -U codetect -d codetect \
  -c "SELECT repo_root, COUNT(*) as symbols FROM symbols GROUP BY repo_root;"
```

## Review Checklist

- [ ] All constructors updated with `repoRoot` parameter
- [ ] All call sites updated to pass `repoRoot`
- [ ] `filepath.Abs()` used at entry points
- [ ] No hardcoded or empty `repoRoot` values
- [ ] End-to-end test demonstrates isolation
