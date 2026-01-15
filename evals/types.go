// Package evals provides evaluation tools for measuring codetect MCP performance.
package evals

import (
	"time"
)

// TestCase represents a single evaluation test case.
type TestCase struct {
	ID          string      `json:"id"`
	Category    string      `json:"category"` // "search", "navigate", "understand"
	Description string      `json:"description"`
	Prompt      string      `json:"prompt"`
	GroundTruth GroundTruth `json:"ground_truth"`
	Difficulty  string      `json:"difficulty"` // "easy", "medium", "hard"
}

// GroundTruth contains the expected results for a test case.
type GroundTruth struct {
	Files   []string         `json:"files,omitempty"`   // Expected files to be found
	Symbols []string         `json:"symbols,omitempty"` // Expected symbols to be found
	Lines   map[string][]int `json:"lines,omitempty"`   // File -> line numbers
	Content []string         `json:"content,omitempty"` // Expected content snippets
}

// ExecutionMode represents whether MCP tools are enabled.
type ExecutionMode string

const (
	ModeWithMCP    ExecutionMode = "with_mcp"
	ModeWithoutMCP ExecutionMode = "without_mcp"
)

// RunResult represents the result of running a single test case.
type RunResult struct {
	TestCaseID    string        `json:"test_case_id"`
	Mode          ExecutionMode `json:"mode"`
	Success       bool          `json:"success"`
	Output        string        `json:"output"`
	SessionID     string        `json:"session_id,omitempty"`
	Duration      time.Duration `json:"duration_ns"`
	TokensUsed    int           `json:"tokens_used,omitempty"`
	InputTokens   int           `json:"input_tokens,omitempty"`
	OutputTokens  int           `json:"output_tokens,omitempty"`
	CacheReadTokens int         `json:"cache_read_tokens,omitempty"`
	CacheCreateTokens int       `json:"cache_create_tokens,omitempty"`
	CostUSD       float64       `json:"cost_usd,omitempty"`
	NumTurns      int           `json:"num_turns,omitempty"`
	ToolCallCount int           `json:"tool_call_count,omitempty"`
	Error         string        `json:"error,omitempty"`
}

// ValidationResult contains the validation metrics for a run.
type ValidationResult struct {
	TestCaseID string        `json:"test_case_id"`
	Mode       ExecutionMode `json:"mode"`
	Precision  float64       `json:"precision"` // Correct items / Total returned
	Recall     float64       `json:"recall"`    // Correct items / Total expected
	F1Score    float64       `json:"f1_score"`  // Harmonic mean of precision and recall
	FilesFound []string      `json:"files_found"`
	FilesMissed []string     `json:"files_missed"`
	SymbolsFound []string    `json:"symbols_found"`
	SymbolsMissed []string   `json:"symbols_missed"`
}

// EvalConfig holds configuration for an evaluation run.
type EvalConfig struct {
	RepoPath      string   `json:"repo_path"`
	Categories    []string `json:"categories,omitempty"` // Empty = all categories
	TestCaseIDs   []string `json:"test_case_ids,omitempty"` // Empty = all test cases
	Parallel      int      `json:"parallel"`       // Number of parallel runs (default: 1)
	Timeout       time.Duration `json:"timeout"`   // Timeout per test case
	OutputDir     string   `json:"output_dir"`
	Verbose       bool     `json:"verbose"`
}

// DefaultConfig returns the default evaluation configuration.
func DefaultConfig() EvalConfig {
	return EvalConfig{
		RepoPath:  ".",
		Parallel:  1,
		Timeout:   5 * time.Minute,
		OutputDir: "evals/results",
		Verbose:   false,
	}
}

// EvalReport contains the full evaluation report.
type EvalReport struct {
	Timestamp   time.Time          `json:"timestamp"`
	Config      EvalConfig         `json:"config"`
	Summary     ReportSummary      `json:"summary"`
	Results     []ComparisonResult `json:"results"`
	RawResults  []RunResult        `json:"raw_results,omitempty"`
}

// ReportSummary contains aggregate metrics.
type ReportSummary struct {
	TotalCases          int       `json:"total_cases"`
	WithMCP             ModeStats `json:"with_mcp"`
	WithoutMCP          ModeStats `json:"without_mcp"`
	AccuracyImprovement float64   `json:"accuracy_improvement_pct"`
	TokenReduction      float64   `json:"token_reduction_pct"`
	CostReduction       float64   `json:"cost_reduction_pct"`
	LatencyReduction    float64   `json:"latency_reduction_pct"`
}

// ModeStats contains aggregate stats for a single execution mode.
type ModeStats struct {
	AvgAccuracy         float64       `json:"avg_accuracy"`
	AvgInputTokens      float64       `json:"avg_input_tokens"`
	AvgOutputTokens     float64       `json:"avg_output_tokens"`
	AvgCacheReadTokens  float64       `json:"avg_cache_read_tokens"`
	AvgCacheCreateTokens float64      `json:"avg_cache_create_tokens"`
	AvgTotalTokens      float64       `json:"avg_total_tokens"`
	AvgCostUSD          float64       `json:"avg_cost_usd"`
	TotalCostUSD        float64       `json:"total_cost_usd"`
	AvgLatency          time.Duration `json:"avg_latency_ns"`
	AvgTurns            float64       `json:"avg_turns"`
	SuccessRate         float64       `json:"success_rate"`
	TotalToolCalls      int           `json:"total_tool_calls"`
}

// ComparisonResult compares results between modes for a single test case.
type ComparisonResult struct {
	TestCaseID        string           `json:"test_case_id"`
	Category          string           `json:"category"`
	Description       string           `json:"description"`
	WithMCP           ValidationResult `json:"with_mcp"`
	WithoutMCP        ValidationResult `json:"without_mcp"`
	AccuracyDiff      float64          `json:"accuracy_diff"`
	TokenDiff         int              `json:"token_diff"`
	LatencyDiff       time.Duration    `json:"latency_diff_ns"`
	Winner            ExecutionMode    `json:"winner"`
}

// ClaudeResponse represents the JSON output from Claude Code.
type ClaudeResponse struct {
	Result           string         `json:"result"`
	SessionID        string         `json:"session_id"`
	StructuredOutput map[string]any `json:"structured_output,omitempty"`
}

// ClaudeStreamEvent represents a single event from Claude's streaming JSON output.
type ClaudeStreamEvent struct {
	Type      string  `json:"type"`
	Subtype   string  `json:"subtype,omitempty"`
	SessionID string  `json:"session_id,omitempty"`
	Result    string  `json:"result,omitempty"`
	NumTurns  int     `json:"num_turns,omitempty"`
	TotalCost float64 `json:"total_cost_usd,omitempty"`
	Usage     *ClaudeUsage `json:"usage,omitempty"`
}

// ClaudeUsage represents token usage from Claude's output.
type ClaudeUsage struct {
	InputTokens            int `json:"input_tokens"`
	OutputTokens           int `json:"output_tokens"`
	CacheReadInputTokens   int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}
