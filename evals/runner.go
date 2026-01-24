package evals

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Runner executes evaluation test cases.
type Runner struct {
	config EvalConfig
}

// NewRunner creates a new evaluation runner.
func NewRunner(config EvalConfig) *Runner {
	return &Runner{config: config}
}

// LoadTestCases loads test cases from JSONL files in the cases directory.
// It first checks for a repo-specific .repo_search/evals/cases directory,
// and falls back to the provided casesDir if not found.
func (r *Runner) LoadTestCases(casesDir string) ([]TestCase, error) {
	var cases []TestCase

	// Check for repo-specific eval cases first
	repoEvalDir := filepath.Join(r.config.RepoPath, ".repo_search", "evals", "cases")
	if info, err := os.Stat(repoEvalDir); err == nil && info.IsDir() {
		casesDir = repoEvalDir
	}

	// Find all JSONL files (including in subdirectories)
	var files []string

	// First, try direct JSONL files in the cases directory
	directFiles, err := filepath.Glob(filepath.Join(casesDir, "*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("finding test case files: %w", err)
	}
	files = append(files, directFiles...)

	// Then, look for JSONL files in subdirectories
	subDirFiles, err := filepath.Glob(filepath.Join(casesDir, "*", "*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("finding test case files in subdirectories: %w", err)
	}
	files = append(files, subDirFiles...)

	for _, file := range files {
		fileCases, err := r.loadJSONLFile(file)
		if err != nil {
			return nil, fmt.Errorf("loading %s: %w", file, err)
		}

		// Filter by category if specified
		for _, tc := range fileCases {
			if len(r.config.Categories) == 0 || contains(r.config.Categories, tc.Category) {
				// Filter by test case ID if specified
				if len(r.config.TestCaseIDs) == 0 || contains(r.config.TestCaseIDs, tc.ID) {
					cases = append(cases, tc)
				}
			}
		}
	}

	return cases, nil
}

func (r *Runner) loadJSONLFile(path string) ([]TestCase, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cases []TestCase
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		var tc TestCase
		if err := json.Unmarshal([]byte(line), &tc); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		cases = append(cases, tc)
	}

	return cases, scanner.Err()
}

// RunAll executes all test cases in both modes.
func (r *Runner) RunAll(ctx context.Context, cases []TestCase) (*EvalReport, error) {
	report := &EvalReport{
		Timestamp: time.Now(),
		Config:    r.config,
	}

	// If parallel is 1 or less, use sequential execution
	if r.config.Parallel <= 1 {
		return r.runAllSequential(ctx, cases, report)
	}

	return r.runAllParallel(ctx, cases, report)
}

// runAllSequential executes test cases sequentially (original behavior).
func (r *Runner) runAllSequential(ctx context.Context, cases []TestCase, report *EvalReport) (*EvalReport, error) {
	for i, tc := range cases {
		if r.config.Verbose {
			fmt.Fprintf(os.Stderr, "[%d/%d] Running: %s\n", i+1, len(cases), tc.ID)
		}

		// Run with MCP
		withMCP, err := r.runTestCase(ctx, tc, ModeWithMCP)
		if err != nil {
			withMCP = &RunResult{
				TestCaseID: tc.ID,
				Mode:       ModeWithMCP,
				Success:    false,
				Error:      err.Error(),
			}
		}
		report.RawResults = append(report.RawResults, *withMCP)

		// Run without MCP
		withoutMCP, err := r.runTestCase(ctx, tc, ModeWithoutMCP)
		if err != nil {
			withoutMCP = &RunResult{
				TestCaseID: tc.ID,
				Mode:       ModeWithoutMCP,
				Success:    false,
				Error:      err.Error(),
			}
		}
		report.RawResults = append(report.RawResults, *withoutMCP)
	}

	return report, nil
}

// testJob represents a single test case execution job.
type testJob struct {
	index    int
	testCase TestCase
	mode     ExecutionMode
}

// testResult represents the result of a test job.
type testResult struct {
	index  int
	result *RunResult
}

// runAllParallel executes test cases in parallel using a worker pool.
func (r *Runner) runAllParallel(ctx context.Context, cases []TestCase, report *EvalReport) (*EvalReport, error) {
	// Calculate total number of jobs (each test case runs twice: with and without MCP)
	totalJobs := len(cases) * 2

	// Create job queue and results channel
	jobs := make(chan testJob, totalJobs)
	results := make(chan testResult, totalJobs)

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

	// Gather all results
	allResults := make([]*RunResult, totalJobs)
	for res := range results {
		allResults[res.index] = res.result
	}

	// Add to report in order
	for _, res := range allResults {
		if res != nil {
			report.RawResults = append(report.RawResults, *res)
		}
	}

	return report, nil
}

// worker processes test jobs from the jobs channel.
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

// runTestCase executes a single test case in the specified mode.
func (r *Runner) runTestCase(ctx context.Context, tc TestCase, mode ExecutionMode) (*RunResult, error) {
	ctx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()

	args := r.buildClaudeArgs(tc, mode)

	start := time.Now()
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = r.config.RepoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	result := &RunResult{
		TestCaseID: tc.ID,
		Mode:       mode,
		Duration:   duration,
	}

	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("%v: %s", err, stderr.String())
		return result, nil
	}

	// Parse streaming JSON output - each line is a separate JSON event
	// We look for the final "result" event which contains usage stats
	lines := bytes.Split(stdout.Bytes(), []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		var event ClaudeStreamEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue // Skip unparseable lines
		}

		// The final "result" event contains the summary
		if event.Type == "result" {
			result.Success = event.Subtype == "success"
			result.Output = event.Result
			result.SessionID = event.SessionID
			result.NumTurns = event.NumTurns
			result.CostUSD = event.TotalCost

			if event.Usage != nil {
				result.InputTokens = event.Usage.InputTokens
				result.OutputTokens = event.Usage.OutputTokens
				result.CacheReadTokens = event.Usage.CacheReadInputTokens
				result.CacheCreateTokens = event.Usage.CacheCreationInputTokens
				result.TokensUsed = event.Usage.InputTokens + event.Usage.OutputTokens
			}
		}
	}

	// If we didn't find a result event, try parsing as simple JSON
	if result.Output == "" {
		var resp ClaudeResponse
		if err := json.Unmarshal(stdout.Bytes(), &resp); err == nil {
			result.Success = true
			result.Output = resp.Result
			result.SessionID = resp.SessionID
		}
	}

	return result, nil
}

// buildClaudeArgs constructs the command-line arguments for Claude.
func (r *Runner) buildClaudeArgs(tc TestCase, mode ExecutionMode) []string {
	args := []string{
		"-p", tc.Prompt,
		"--output-format", "stream-json",
		"--verbose",
	}

	if mode == ModeWithMCP {
		// Enable repo-search MCP tools
		mcpConfig := `{"mcpServers":{"repo-search":{"command":"repo-search","args":["mcp"]}}}`
		args = append(args,
			"--mcp-config", mcpConfig,
			"--allowedTools", "mcp__repo-search__search_keyword,mcp__repo-search__find_symbol,mcp__repo-search__list_defs_in_file,mcp__repo-search__search_semantic,mcp__repo-search__hybrid_search,mcp__repo-search__get_file,Read",
		)
	} else {
		// Standard tools only
		args = append(args,
			"--allowedTools", "Bash(rg:*),Bash(grep:*),Bash(find:*),Read,Glob,Grep",
		)
	}

	// Add max turns to prevent runaway execution
	args = append(args, "--max-turns", "20")

	return args
}

// SaveResults writes the raw results to a JSON file.
// It always uses the repo-specific .repo_search/evals/results directory.
func (r *Runner) SaveResults(report *EvalReport) error {
	// Always use repo-specific results directory to keep results with cases
	outputDir := filepath.Join(r.config.RepoPath, ".repo_search", "evals", "results")

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	filename := fmt.Sprintf("%s-results.json", time.Now().Format("2006-01-02-150405"))
	path := filepath.Join(outputDir, filename)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling results: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing results: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Results saved to: %s\n", path)
	return nil
}

func contains(slice []string, item string) bool {
	return slices.Contains(slice, item)
}

// EnsureGitignore ensures the .repo_search directory is in the target repo's .gitignore.
func (r *Runner) EnsureGitignore() error {
	gitignorePath := filepath.Join(r.config.RepoPath, ".gitignore")

	// Read existing .gitignore if it exists
	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading .gitignore: %w", err)
	}

	// Check if .repo_search is already in .gitignore
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == ".repo_search" || trimmed == ".repo_search/" {
			return nil // Already exists
		}
	}

	// Append .repo_search to .gitignore
	var newContent string
	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		newContent = string(content) + "\n.repo_search/\n"
	} else if len(content) > 0 {
		newContent = string(content) + ".repo_search/\n"
	} else {
		newContent = ".repo_search/\n"
	}

	if err := os.WriteFile(gitignorePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}

	return nil
}
