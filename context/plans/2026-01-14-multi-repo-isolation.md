# Multi-Repository Isolation via `repo_root` Column

**Date:** 2026-01-14
**Status:** Planning
**Type:** Phased Plan (3 phases)

## Objective

Enable multiple repositories to share the same PostgreSQL database without data collisions. Users should be able to run `codetect index` and `codetect embed` on any repo without worrying about conflicts with other indexed repositories.

## Problem Statement

Currently, file paths are stored as relative paths (e.g., `main.go`, `internal/db/adapter.go`). When multiple repositories are indexed into the same PostgreSQL database, repos with identical relative paths collide and overwrite each other's data.

## Solution Overview

Add a `repo_root` column to all tables that stores the absolute path of the repository root. All queries filter by `repo_root` to isolate data per repository.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| `repo_root` format | Absolute path | Human-readable, matches existing `idx.root`, easy to debug |
| Schema version | Increment to 2 | Enables migration logic for existing databases |
| Backward compat | SQLite unchanged | Single-repo SQLite continues to work |

## Phase Overview

| Phase | Focus | Files | Dependencies |
|-------|-------|-------|--------------|
| **Phase 1** | Schema Changes | schema.go, store.go | None |
| **Phase 2** | Query Updates | index.go, store.go | Phase 1 |
| **Phase 3** | Command Integration | main.go, symbols.go | Phase 2 |

## Phase Details

### Phase 1: Schema Changes
**Plan:** [2026-01-14-multi-repo-isolation-phase-1.md](./2026-01-14-multi-repo-isolation-phase-1.md)

- Add `repo_root TEXT NOT NULL` column to symbols, files, embeddings tables
- Update unique constraints to include `repo_root`
- Add composite indexes for efficient filtering
- Increment schema version, add migration logic

### Phase 2: Query Updates
**Plan:** [2026-01-14-multi-repo-isolation-phase-2.md](./2026-01-14-multi-repo-isolation-phase-2.md)

- Update all symbol index queries to filter by `repo_root`
- Update all embedding store queries to filter by `repo_root`
- Ensure upserts include `repo_root` in conflict columns

### Phase 3: Command Integration
**Plan:** [2026-01-14-multi-repo-isolation-phase-3.md](./2026-01-14-multi-repo-isolation-phase-3.md)

- Update constructors to accept `repoRoot` parameter
- Wire `absPath` through commands to index/store constructors
- Update MCP tools to pass current working directory

## Cross-Phase Risks

| Risk | Mitigation |
|------|------------|
| Schema migration fails on existing data | Add default value for `repo_root` in migration |
| Performance regression from added column | Add composite index `(repo_root, path)` |
| Breaking existing SQLite workflows | Test SQLite backward compatibility |

## Success Criteria

1. Two repos with identical file paths can be indexed to same PostgreSQL database
2. `codetect-index stats /path/to/repo` shows only that repo's data
3. SQLite single-repo workflow continues to work unchanged
4. All existing unit tests pass
5. Database shows distinct `repo_root` values per repository

## Verification (End-to-End)

```bash
# Drop existing tables (schema change)
docker exec codetect-postgres psql -U codetect -d codetect \
  -c "DROP TABLE IF EXISTS symbols, files, embeddings, schema_version;"

# Test PostgreSQL multi-repo isolation
export CODETECT_DB_TYPE=postgres
export CODETECT_DB_DSN="postgres://codetect:codetect@localhost:5465/codetect?sslmode=disable"

# Create two test repos with same file name
mkdir -p /tmp/repo1 /tmp/repo2
echo 'package main; func hello() {}' > /tmp/repo1/main.go
echo 'package main; func hello() {}' > /tmp/repo2/main.go

# Index both
codetect-index index /tmp/repo1
codetect-index index /tmp/repo2

# Verify isolation
codetect-index stats /tmp/repo1  # Should show repo1 data only
codetect-index stats /tmp/repo2  # Should show repo2 data only

# Verify in database
docker exec codetect-postgres psql -U codetect -d codetect \
  -c "SELECT repo_root, COUNT(*) FROM symbols GROUP BY repo_root;"
```
