# Current Work Summary

Planning: Parallel Eval Execution

**Plan:** context/plans/2026-01-23-parallel-eval-execution.md

## Objective

Add configurable parallel execution to the eval runner to significantly reduce evaluation time. Currently, evals run sequentially (one after another) which takes a very long time. Target initial concurrency: 10 parallel evals.

## To-Do List

Planning phase - pending user approval to proceed with execution.

## Progress Notes

### 2026-01-23 - Execution Completed ✅

**Implemented all 5 priorities:**
1. ✅ Config preservation with backup and old value tracking (install.sh:1110-1145)
2. ✅ Dimension mismatch detection with warning box (install.sh:422-453)
3. ✅ Repository detection from registry + file system (install.sh:1356-1380)
4. ✅ Batch re-embedding workflow with progress tracking (install.sh:1381-1452)
5. ✅ Config summary display showing changes (install.sh:1187-1218)

**Testing completed:**
- Component testing: Repository detection verified (6 repos found)
- Bug found & fixed: Registry structure used `.projects[]?.path` not `.repositories[]?.path`
- Code review: All features verified against plan requirements
- Manual testing guide created for user verification

**Commits:**
- 421deb6: Store old model and dimensions for mismatch detection
- 81dbb85: Add dimension mismatch detection after model selection
- 26b47a3: Add repository detection and batch re-embedding workflow
- e5f6d58: Add config summary display before writing
- 72b7a6a: Fix registry structure bug (critical fix)

**PR #28 created:** https://github.com/brian-lai/codetect/pull/28
- +220 lines in install.sh
- All changes backward compatible
- Ready for review and merge

### 2026-01-22 - Execution Started

**Goal:** Fix installer reinstallation issues

**Problems to solve:**
1. Config overwrites (line 965: `cat >` destroys existing config despite line 132 claiming preservation)
2. No dimension mismatch detection when changing models
3. No guidance for re-embedding after model changes
4. Users lose custom settings (URLs, DB connections, API keys)

**Technical approach:**
- **Priority 1:** Config preservation with backup/merge logic
- **Priority 2:** Dimension mismatch detection and warnings
- **Priority 3:** Repository detection (registry.json + file search)
- **Priority 4:** Automated re-embedding with progress tracking
- **Priority 5:** UX polish (diff display, clear messaging)

**Success criteria:**
- Existing config backed up before modification
- Custom settings preserved
- Dimension mismatches detected and warned
- Batch re-embedding offered when needed
- Clear, actionable user guidance

---

```json
{
  "active_context": [
    "context/plans/2026-01-23-parallel-eval-execution.md"
  ],
  "completed_summaries": [
    "context/plans/2026-01-22-installer-config-preservation-and-reembedding.md"
  ],
  "execution_branch": null,
  "last_updated": "2026-01-23T18:45:00Z"
}
```
