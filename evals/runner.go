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
// It first checks for a repo-specific .codetect/evals/cases directory,
// and falls back to the provided casesDir if not found.
func (r *Runner) LoadTestCases(casesDir string) ([]TestCase, error) {
	var cases []TestCase

	// Check for repo-specific eval cases first
	repoEvalDir := filepath.Join(r.config.RepoPath, ".codetect", "evals", "cases")
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

	// Save raw stdout to log file for later inspection
	if err := r.saveLog(tc.ID, mode, start, stdout.Bytes()); err != nil && r.config.Verbose {
		fmt.Fprintf(os.Stderr, "warning: could not save log for %s: %v\n", tc.ID, err)
	}

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
				result.TokensUsed = event.Usage.InputTokens + event.Usage.OutputTokens +
					event.Usage.CacheReadInputTokens + event.Usage.CacheCreationInputTokens
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
		// Enable codetect MCP tools
		mcpConfig := `{"mcpServers":{"codetect":{"command":"codetect","args":["mcp"]}}}`
		args = append(args,
			"--mcp-config", mcpConfig,
			"--allowedTools", "mcp__codetect__search_keyword,mcp__codetect__find_symbol,mcp__codetect__list_defs_in_file,mcp__codetect__search_semantic,mcp__codetect__hybrid_search,mcp__codetect__get_file,Read",
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
// It always uses the repo-specific .codetect/evals/results directory.
func (r *Runner) SaveResults(report *EvalReport) error {
	// Always use repo-specific results directory to keep results with cases
	outputDir := filepath.Join(r.config.RepoPath, ".codetect", "evals", "results")

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

// saveLog writes the raw Claude stdout to a log file for later inspection.
func (r *Runner) saveLog(testCaseID string, mode ExecutionMode, timestamp time.Time, data []byte) error {
	logsDir := filepath.Join(r.config.RepoPath, ".codetect", "evals", "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("creating logs dir: %w", err)
	}

	filename := fmt.Sprintf("%s-%s-%s.log", timestamp.Format("2006-01-02-150405"), testCaseID, mode)
	path := filepath.Join(logsDir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing log: %w", err)
	}

	return nil
}

// LogEntry represents a single log file with metadata.
type LogEntry struct {
	Path      string        `json:"path"`
	TestCase  string        `json:"test_case"`
	Mode      ExecutionMode `json:"mode"`
	Timestamp time.Time     `json:"timestamp"`
	Size      int64         `json:"size"`
}

// ListLogs returns all log files for a given repo, sorted by timestamp (newest first).
func (r *Runner) ListLogs() ([]LogEntry, error) {
	logsDir := filepath.Join(r.config.RepoPath, ".codetect", "evals", "logs")

	files, err := filepath.Glob(filepath.Join(logsDir, "*.log"))
	if err != nil {
		return nil, fmt.Errorf("finding log files: %w", err)
	}

	var entries []LogEntry
	for _, file := range files {
		entry, err := parseLogFilename(file)
		if err != nil {
			continue // Skip unparseable filenames
		}

		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		entry.Size = info.Size()
		entries = append(entries, entry)
	}

	// Sort by timestamp descending (newest first)
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].Timestamp.After(entries[i].Timestamp) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	return entries, nil
}

// parseLogFilename extracts metadata from a log filename.
// Format: 2006-01-02-150405-testcase-mode.log
func parseLogFilename(path string) (LogEntry, error) {
	filename := filepath.Base(path)
	filename = strings.TrimSuffix(filename, ".log")

	// Split into timestamp-testcase-mode
	// Timestamp is fixed format: 2006-01-02-150405 (19 chars)
	if len(filename) < 22 { // 19 + 1 (dash) + at least 1 char + 1 (dash) + mode
		return LogEntry{}, fmt.Errorf("filename too short")
	}

	timestampStr := filename[:19]
	timestamp, err := time.Parse("2006-01-02-150405", timestampStr)
	if err != nil {
		return LogEntry{}, fmt.Errorf("parsing timestamp: %w", err)
	}

	rest := filename[20:] // Skip timestamp and dash

	// Find mode suffix
	var mode ExecutionMode
	var testCase string
	if strings.HasSuffix(rest, "-with_mcp") {
		mode = ModeWithMCP
		testCase = strings.TrimSuffix(rest, "-with_mcp")
	} else if strings.HasSuffix(rest, "-without_mcp") {
		mode = ModeWithoutMCP
		testCase = strings.TrimSuffix(rest, "-without_mcp")
	} else {
		return LogEntry{}, fmt.Errorf("unknown mode suffix")
	}

	return LogEntry{
		Path:      path,
		TestCase:  testCase,
		Mode:      mode,
		Timestamp: timestamp,
	}, nil
}

// ReadLog reads and returns the contents of a log file.
func (r *Runner) ReadLog(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// EnsureGitignore ensures the .codetect directory is in the target repo's .gitignore.
func (r *Runner) EnsureGitignore() error {
	gitignorePath := filepath.Join(r.config.RepoPath, ".gitignore")

	// Read existing .gitignore if it exists
	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading .gitignore: %w", err)
	}

	// Check if .codetect is already in .gitignore
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == ".codetect" || trimmed == ".codetect/" {
			return nil // Already exists
		}
	}

	// Append .codetect to .gitignore
	var newContent string
	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		newContent = string(content) + "\n.codetect/\n"
	} else if len(content) > 0 {
		newContent = string(content) + ".codetect/\n"
	} else {
		newContent = ".codetect/\n"
	}

	if err := os.WriteFile(gitignorePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}

	return nil
}
