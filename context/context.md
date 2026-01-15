# Current Work Summary

Executing: Fix PostgreSQL Indexing Bug

**Branch:** `para/fix-postgres-indexing`
**Plan:** context/plans/2026-01-14-fix-postgres-indexing.md

## To-Do List

### Phase 1: Refactor Symbol Index for Adapter Pattern Compliance

- [ ] Add `getSQLDB()` helper method for transaction support
- [ ] Refactor `FindSymbol()` to use adapter + dialect-aware placeholders
- [ ] Refactor `ListDefsInFile()` to use adapter + dialect-aware placeholders
- [ ] Refactor `Update()` to use getSQLDB for transactions + convert placeholders
- [ ] Refactor `FullReindex()` to use getSQLDB for transactions + convert placeholders
- [ ] Refactor `Stats()` to use adapter
- [ ] Update `NewIndexWithConfig` documentation and add deprecation comment to `NewIndex`

### Phase 2: Update codetect-index Commands

- [ ] Update `runIndex()` to load config and use `NewIndexWithConfig`
- [ ] Update `runEmbed()` to load config and use dialect-aware store
- [ ] Update `runStats()` to load config and open correct database

### Phase 3: SQLite Backward Compatibility

- [ ] Add SQLite default path handling when no env vars set

### Phase 4: Update Symbol Tools

- [ ] Update `openIndex()` in `internal/tools/symbols.go` to use database config

### Phase 5: Testing & Verification

- [ ] Run existing tests to ensure no regressions
- [ ] Test end-to-end PostgreSQL indexing
- [ ] Test end-to-end SQLite backward compatibility

## Progress Notes

_Update this section as you complete items._

---

```json
{
  "active_context": [
    "context/plans/2026-01-14-fix-postgres-indexing.md"
  ],
  "completed_summaries": [
    "context/summaries/2026-01-14-postgres-pgvector-support-complete-summary.md"
  ],
  "archived_contexts": [
    "context/archives/2026-01-14-2206-postgres-pgvector-complete.md"
  ],
  "execution_branch": "para/fix-postgres-indexing",
  "execution_started": "2026-01-14T23:20:00Z",
  "last_updated": "2026-01-14T23:20:00Z"
}
```
