# Plan: Parallelize Eval Execution

**Created:** 2026-01-11
**Status:** Completed
**Type:** Performance Optimization

## Objective

Add optional parallelization to the evaluation runner to significantly reduce the time required to run eval test cases. Currently, each eval test case takes several minutes and runs procedurally (sequentially). By introducing a `--parallel` flag, we can run multiple test cases concurrently, improving evaluation performance while maintaining backward compatibility.

## Current State

The eval runner (`evals/runner.go`) currently:
- Runs test cases sequentially in `RunAll()` method (lines 104-142)
- Executes each test case twice: once with MCP, once without MCP
- Each test case can take several minutes to complete
- Total evaluation time = (number of test cases) × 2 × (avg test duration)

For example, 10 test cases at 3 minutes each = 60 minutes total.

## Approach

### 1. Add Configuration Option

**File:** `evals/types.go`

The `EvalConfig` struct already has a `Parallel` field (line 71) that defaults to 1. This field is ready to use.

**File:** `cmd/repo-search-eval/main.go`

Add a `--parallel` CLI flag to the `run` command (around line 42-50) to allow users to specify the number of concurrent test cases:

```go
parallel := fs.Int("parallel", 1, "Number of test cases to run in parallel")
```

Wire this flag to the config:
```go
config.Parallel = *parallel
```

### 2. Refactor RunAll to Support Parallelization

**File:** `evals/runner.go`

Current implementation runs serially:
```go
for i, tc := range cases {
    // Run with MCP
    withMCP, err := r.runTestCase(ctx, tc, ModeWithMCP)
    // Append to report

    // Run without MCP
    withoutMCP, err := r.runTestCase(ctx, tc, ModeWithoutMCP)
    // Append to report
}
```

**Proposed Approach:**

1. **Create a worker pool** with `config.Parallel` goroutines
2. **Use a job queue** to distribute test cases to workers
3. **Collect results** in a thread-safe manner
4. **Maintain order** in the final report for consistency

Implementation strategy:
- Use a buffered channel for job distribution
- Use a sync.WaitGroup to wait for all workers
- Use a mutex to protect the report.RawResults slice, OR
- Use a results channel and a separate goroutine to collect results

**Recommended approach:** Results channel pattern (cleaner, no mutex needed)

```go
func (r *Runner) RunAll(ctx context.Context, cases []TestCase) (*EvalReport, error) {
    report := &EvalReport{
        Timestamp: time.Now(),
        Config:    r.config,
    }

    // If parallel is 1, fall back to sequential execution
    if r.config.Parallel <= 1 {
        return r.runAllSequential(ctx, cases)
    }

    return r.runAllParallel(ctx, cases, report)
}
```

### 3. Implement Parallel Execution

**File:** `evals/runner.go`

Add two new methods:

#### 3.1 Sequential Execution (refactored from existing code)

```go
func (r *Runner) runAllSequential(ctx context.Context, cases []TestCase) (*EvalReport, error) {
    // This is the existing logic from RunAll
    // Extract it into this method for backward compatibility
}
```

#### 3.2 Parallel Execution (new)

```go
type testJob struct {
    index int
    testCase TestCase
    mode ExecutionMode
}

type testResult struct {
    index int
    result *RunResult
}

func (r *Runner) runAllParallel(ctx context.Context, cases []TestCase, report *EvalReport) (*EvalReport, error) {
    // Create job queue
    jobs := make(chan testJob, len(cases)*2)
    results := make(chan testResult, len(cases)*2)

    // Start worker pool
    var wg sync.WaitGroup
    for w := 0; w < r.config.Parallel; w++ {
        wg.Add(1)
        go r.worker(ctx, &wg, jobs, results)
    }

    // Enqueue all jobs
    jobIndex := 0
    for _, tc := range cases {
        jobs <- testJob{index: jobIndex, testCase: tc, mode: ModeWithMCP}
        jobIndex++
        jobs <- testJob{index: jobIndex, testCase: tc, mode: ModeWithoutMCP}
        jobIndex++
    }
    close(jobs)

    // Collect results in background
    go func() {
        wg.Wait()
        close(results)
    }()

    // Gather all results (order doesn't matter for raw results)
    allResults := make([]*RunResult, jobIndex)
    for res := range results {
        allResults[res.index] = res.result
    }

    // Add to report
    for _, res := range allResults {
        if res != nil {
            report.RawResults = append(report.RawResults, *res)
        }
    }

    return report, nil
}

func (r *Runner) worker(ctx context.Context, wg *sync.WaitGroup, jobs <-chan testJob, results chan<- testResult) {
    defer wg.Done()

    for job := range jobs {
        if r.config.Verbose {
            fmt.Fprintf(os.Stderr, "Running: %s (%s)\n", job.testCase.ID, job.mode)
        }

        result, err := r.runTestCase(ctx, job.testCase, job.mode)
        if err != nil {
            result = &RunResult{
                TestCaseID: job.testCase.ID,
                Mode:       job.mode,
                Success:    false,
                Error:      err.Error(),
            }
        }

        results <- testResult{index: job.index, result: result}
    }
}
```

### 4. Update Documentation

**File:** `docs/evaluation.md`

Add documentation for the `--parallel` flag:

**In the "Run Options" section (around line 35-41):**

```markdown
- `--parallel <n>` - Number of test cases to run in parallel (default: 1)
```

**Add a new "Performance Tips" section before "Best Practices":**

```markdown
## Performance Tips

### Parallel Execution

By default, eval test cases run sequentially. You can significantly reduce evaluation time by running multiple test cases in parallel:

```bash
# Run with 4 parallel workers
repo-search-eval run --parallel 4

# Run with 8 parallel workers for faster execution
repo-search-eval run --parallel 8
```

**Recommendations:**
- For most machines, `--parallel 4` to `--parallel 8` provides a good balance
- Higher parallelism may cause resource contention (CPU, memory, API rate limits)
- Each test case spawns a Claude process, so monitor system resources
- Parallel execution maintains the same accuracy as sequential execution
```

**File:** `README.md`

Update the eval example (around line 87):

```markdown
repo-search-eval run --repo <path> --parallel 4    # Run with parallelization
```

## Risks & Considerations

### 1. Resource Contention
- **Risk:** Running many Claude processes in parallel may exhaust system resources
- **Mitigation:** Document recommended parallelism levels (4-8 workers)
- **Mitigation:** Each worker has independent timeout, preventing runaway processes

### 2. API Rate Limits
- **Risk:** Parallel execution might hit Anthropic API rate limits
- **Mitigation:** Users can control parallelism level via flag
- **Mitigation:** Context timeouts still apply to each test case

### 3. Result Ordering
- **Risk:** Results may be in different order than sequential execution
- **Mitigation:** This doesn't affect report accuracy - validation and summary aggregation don't depend on order

### 4. Verbose Output Interleaving
- **Risk:** Verbose output from parallel workers may interleave and be hard to read
- **Mitigation:** Consider using structured logging or per-worker prefixes
- **Low priority:** Verbose mode is primarily for debugging

### 5. Backward Compatibility
- **Risk:** Changing behavior might break existing workflows
- **Mitigation:** Default to `--parallel 1` (sequential), making parallelization opt-in
- **Mitigation:** Extract sequential logic into separate method for easy maintenance

## Data Sources

- `evals/runner.go:104-142` - Current RunAll implementation
- `evals/types.go:71` - Existing Parallel field in EvalConfig
- `cmd/repo-search-eval/main.go:42-60` - CLI flag parsing
- `docs/evaluation.md` - Documentation to update

## Success Criteria

1. ✅ CLI accepts `--parallel <n>` flag
2. ✅ Sequential execution (parallel=1) maintains current behavior
3. ✅ Parallel execution (parallel>1) runs multiple test cases concurrently
4. ✅ All test results are collected correctly regardless of completion order
5. ✅ No race conditions or data corruption in report generation
6. ✅ Documentation updated with usage examples and recommendations
7. ✅ Evaluation time is significantly reduced with parallel execution (e.g., 60 min → 15 min with parallel=4)
8. ✅ Verbose output indicates which test cases are running

## Implementation Checklist

- [ ] Add `--parallel` flag to `cmd/repo-search-eval/main.go`
- [ ] Wire flag to `config.Parallel` in main.go
- [ ] Refactor `RunAll()` to dispatch to sequential or parallel implementation
- [ ] Extract current logic into `runAllSequential()`
- [ ] Implement `runAllParallel()` with worker pool pattern
- [ ] Implement `worker()` goroutine function
- [ ] Test with `--parallel 1` to verify sequential behavior unchanged
- [ ] Test with `--parallel 4` to verify parallel execution works
- [ ] Verify no race conditions using `go run -race`
- [ ] Update `docs/evaluation.md` with `--parallel` option and performance tips
- [ ] Update `README.md` with parallel example
- [ ] Run full eval suite to verify correctness

## Testing Plan

1. **Sequential test:** Run with `--parallel 1`, verify results match current behavior
2. **Parallel test:** Run with `--parallel 4`, verify same results, faster completion
3. **Race detection:** Run with `go run -race` to check for race conditions
4. **Edge cases:**
   - Single test case with parallel > 1
   - Parallel = 0 or negative (should default to 1)
   - Very high parallelism (e.g., 100) to test resource limits
5. **Performance measurement:** Compare execution time for 10 test cases:
   - Sequential: baseline
   - Parallel 2: ~50% time reduction expected
   - Parallel 4: ~75% time reduction expected
   - Parallel 8: ~85% time reduction expected

## Notes

- The `Parallel` field already exists in `EvalConfig` (line 71), so minimal changes to the type system are needed
- The worker pool pattern is a standard Go concurrency pattern, well-suited for this use case
- Results don't need to be in order since the report aggregates metrics across all test cases
- Consider future enhancement: Progress bar showing "X/Y completed" for parallel execution
