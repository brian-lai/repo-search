# Parallel Eval Execution

**Date:** 2026-01-23
**Status:** ✅ Completed

## Objective

Add configurable parallel execution to the eval runner to significantly reduce evaluation time. Currently, evals run sequentially (one after another), which takes a very long time for large test suites. Target initial concurrency: 10 parallel evals.

## Current Architecture

**Sequential Execution Flow:**
```
RunAll() in evals/runner.go:104-142
├── For each test case (sequential loop)
│   ├── Run with MCP (runTestCase)
│   └── Run without MCP (runTestCase)
```

Each test case runs:
- **With MCP** mode - uses codetect MCP tools
- **Without MCP** mode - uses standard Claude Code tools
- Both modes run sequentially, one after another

**Current Performance:**
- Each test case: 2 runs (with/without MCP)
- Each run: up to 5 minutes (default timeout)
- 10 test cases = 20 runs = potentially 100 minutes if all hit timeout

## Approach

### Phase 1: Add Concurrency Infrastructure

1. **Update `EvalConfig` type** (`evals/types.go:66-75`)
   - `Parallel` field already exists (line 71) with default value of 1
   - No changes needed to config structure

2. **Add command-line flag** (`cmd/codetect-eval/main.go:50-58`)
   - Add `--parallel` or `-j` flag to `runEval` function
   - Wire flag to `config.Parallel`
   - Default: 10 (as requested)

3. **Implement worker pool in `RunAll`** (`evals/runner.go:104-142`)
   - Create buffered channel for test case jobs
   - Spawn N worker goroutines (where N = config.Parallel)
   - Each worker processes test cases and runs both modes
   - Collect results via results channel
   - Preserve order of results for consistent reporting

### Implementation Strategy

**Worker Pool Pattern:**
```go
type testJob struct {
    index    int
    testCase TestCase
}

type testResult struct {
    index      int
    withMCP    *RunResult
    withoutMCP *RunResult
}

// Create channels
jobs := make(chan testJob, len(cases))
results := make(chan testResult, len(cases))

// Spawn workers
var wg sync.WaitGroup
for i := 0; i < config.Parallel; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        for job := range jobs {
            // Run both modes
            withMCP := runTestCase(ctx, job.testCase, ModeWithMCP)
            withoutMCP := runTestCase(ctx, job.testCase, ModeWithoutMCP)
            results <- testResult{job.index, withMCP, withoutMCP}
        }
    }()
}

// Send jobs
for i, tc := range cases {
    jobs <- testJob{i, tc}
}
close(jobs)

// Collect results (in separate goroutine)
go func() {
    wg.Wait()
    close(results)
}()

// Gather results and sort by index to preserve order
```

**Progress Reporting:**
- Use atomic counter for concurrent-safe progress updates
- Update verbose output to show: `[completed/total] Running: ...`
- Thread-safe writes to stderr for progress

### Phase 2: Testing & Validation

1. **Test with different concurrency levels**
   - Run with `--parallel 1` (sequential, baseline)
   - Run with `--parallel 5` (medium concurrency)
   - Run with `--parallel 10` (requested default)
   - Verify results are identical regardless of concurrency

2. **Verify result ordering**
   - Ensure report output maintains consistent test case ordering
   - Check that summary statistics are correct

3. **Test edge cases**
   - Single test case
   - Concurrency > number of test cases
   - Context cancellation during parallel execution

## Files to Modify

| File | Purpose | Changes |
|------|---------|---------|
| `evals/runner.go` | Core runner logic | Replace sequential loop with worker pool |
| `cmd/codetect-eval/main.go` | CLI flags | Add `--parallel` flag (default: 10) |

## Risks & Mitigations

### Risk: Resource Contention
- **Issue:** 10 concurrent Claude instances may overwhelm system resources
- **Mitigation:** Make concurrency configurable; users can tune based on their system
- **Note:** Each Claude instance is independent, so no shared state concerns

### Risk: Log File Conflicts
- **Issue:** Multiple workers writing logs simultaneously
- **Mitigation:** Existing `saveLog()` uses unique filenames (timestamp + ID + mode), no conflicts

### Risk: Result Ordering
- **Issue:** Results may complete out of order
- **Mitigation:** Use indexed results and sort before appending to report

### Risk: Progress Output Interleaving
- **Issue:** Concurrent stderr writes may interleave
- **Mitigation:** Use mutex for progress output or atomic counter with single writer

## Success Criteria

1. **Correctness:**
   - Parallel execution produces identical results to sequential execution
   - All test cases run to completion
   - Results maintain correct ordering in report

2. **Performance:**
   - 10 parallel evals complete in ~1/10th the time of sequential (accounting for overhead)
   - No deadlocks or race conditions

3. **Usability:**
   - `--parallel` flag is easy to understand and use
   - Progress output is readable (not garbled from concurrent writes)
   - Default of 10 works well for typical systems

4. **Testing:**
   - Can verify with `go test -race` for race condition detection
   - Manual testing with varying concurrency levels shows expected behavior

## Example Usage

```bash
# Sequential (current behavior, backward compatible)
codetect-eval run

# Parallel with 10 workers (new default)
codetect-eval run --parallel 10

# Parallel with 5 workers (conservative)
codetect-eval run --parallel 5

# Maximum parallelism (equal to number of test cases)
codetect-eval run --parallel 0  # 0 = unlimited
```

## Implementation Notes

- The `Parallel` field already exists in `EvalConfig` (line 71) but is never used
- Need to wire the CLI flag to this existing config field
- Worker pool is idiomatic Go pattern, well-tested
- No database concerns - each eval is independent
- Logs already have unique filenames, no conflicts
- Each Claude invocation is isolated (separate process), no shared state

## Performance Expectations

**Before (Sequential):**
- 20 test cases × 2 modes = 40 runs
- Average 2 minutes per run = 80 minutes total

**After (Parallel with 10 workers):**
- Same 40 runs, but 10 concurrent
- Estimated time: ~8-10 minutes (10x speedup)
- Actual speedup depends on test case duration distribution

## Dependencies

None - this is a self-contained change to the eval runner.

## References

- Current sequential implementation: `evals/runner.go:104-142`
- Config structure: `evals/types.go:66-75`
- CLI entry point: `cmd/codetect-eval/main.go:50-58`
