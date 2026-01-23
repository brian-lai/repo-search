package evals

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Reporter generates evaluation reports.
type Reporter struct{}

// NewReporter creates a new reporter.
func NewReporter() *Reporter {
	return &Reporter{}
}

// GenerateSummary calculates aggregate statistics for the report.
func (r *Reporter) GenerateSummary(report *EvalReport) {
	var withMCP, withoutMCP ModeStats
	var mcpCount, noMcpCount int

	for _, result := range report.RawResults {
		if result.Mode == ModeWithMCP {
			mcpCount++
			if result.Success {
				withMCP.SuccessRate++
			}
			withMCP.AvgLatency += result.Duration
			withMCP.AvgInputTokens += float64(result.InputTokens)
			withMCP.AvgOutputTokens += float64(result.OutputTokens)
			withMCP.AvgCacheReadTokens += float64(result.CacheReadTokens)
			withMCP.AvgCacheCreateTokens += float64(result.CacheCreateTokens)
			withMCP.AvgTotalTokens += float64(result.TokensUsed)
			withMCP.TotalCostUSD += result.CostUSD
			withMCP.AvgTurns += float64(result.NumTurns)
			withMCP.TotalToolCalls += result.ToolCallCount
		} else {
			noMcpCount++
			if result.Success {
				withoutMCP.SuccessRate++
			}
			withoutMCP.AvgLatency += result.Duration
			withoutMCP.AvgInputTokens += float64(result.InputTokens)
			withoutMCP.AvgOutputTokens += float64(result.OutputTokens)
			withoutMCP.AvgCacheReadTokens += float64(result.CacheReadTokens)
			withoutMCP.AvgCacheCreateTokens += float64(result.CacheCreateTokens)
			withoutMCP.AvgTotalTokens += float64(result.TokensUsed)
			withoutMCP.TotalCostUSD += result.CostUSD
			withoutMCP.AvgTurns += float64(result.NumTurns)
			withoutMCP.TotalToolCalls += result.ToolCallCount
		}
	}

	// Calculate averages
	if mcpCount > 0 {
		withMCP.SuccessRate /= float64(mcpCount)
		withMCP.AvgLatency /= time.Duration(mcpCount)
		withMCP.AvgInputTokens /= float64(mcpCount)
		withMCP.AvgOutputTokens /= float64(mcpCount)
		withMCP.AvgCacheReadTokens /= float64(mcpCount)
		withMCP.AvgCacheCreateTokens /= float64(mcpCount)
		withMCP.AvgTotalTokens /= float64(mcpCount)
		withMCP.AvgCostUSD = withMCP.TotalCostUSD / float64(mcpCount)
		withMCP.AvgTurns /= float64(mcpCount)
	}
	if noMcpCount > 0 {
		withoutMCP.SuccessRate /= float64(noMcpCount)
		withoutMCP.AvgLatency /= time.Duration(noMcpCount)
		withoutMCP.AvgInputTokens /= float64(noMcpCount)
		withoutMCP.AvgOutputTokens /= float64(noMcpCount)
		withoutMCP.AvgCacheReadTokens /= float64(noMcpCount)
		withoutMCP.AvgCacheCreateTokens /= float64(noMcpCount)
		withoutMCP.AvgTotalTokens /= float64(noMcpCount)
		withoutMCP.AvgCostUSD = withoutMCP.TotalCostUSD / float64(noMcpCount)
		withoutMCP.AvgTurns /= float64(noMcpCount)
	}

	// Calculate accuracy from validation results
	var mcpAccuracy, noMcpAccuracy float64
	var mcpAccCount, noMcpAccCount int
	for _, cr := range report.Results {
		if cr.WithMCP.F1Score > 0 || cr.WithMCP.Recall > 0 {
			mcpAccuracy += cr.WithMCP.F1Score
			mcpAccCount++
		}
		if cr.WithoutMCP.F1Score > 0 || cr.WithoutMCP.Recall > 0 {
			noMcpAccuracy += cr.WithoutMCP.F1Score
			noMcpAccCount++
		}
	}
	if mcpAccCount > 0 {
		withMCP.AvgAccuracy = mcpAccuracy / float64(mcpAccCount)
	}
	if noMcpAccCount > 0 {
		withoutMCP.AvgAccuracy = noMcpAccuracy / float64(noMcpAccCount)
	}

	report.Summary = ReportSummary{
		TotalCases: len(report.Results),
		WithMCP:    withMCP,
		WithoutMCP: withoutMCP,
	}

	// Calculate improvements (positive = MCP is better)
	if withoutMCP.AvgAccuracy > 0 {
		report.Summary.AccuracyImprovement = ((withMCP.AvgAccuracy - withoutMCP.AvgAccuracy) / withoutMCP.AvgAccuracy) * 100
	}
	if withoutMCP.AvgTotalTokens > 0 {
		report.Summary.TokenReduction = ((withoutMCP.AvgTotalTokens - withMCP.AvgTotalTokens) / withoutMCP.AvgTotalTokens) * 100
	}
	if withoutMCP.TotalCostUSD > 0 {
		report.Summary.CostReduction = ((withoutMCP.TotalCostUSD - withMCP.TotalCostUSD) / withoutMCP.TotalCostUSD) * 100
	}
	if withoutMCP.AvgLatency > 0 {
		report.Summary.LatencyReduction = float64(withoutMCP.AvgLatency-withMCP.AvgLatency) / float64(withoutMCP.AvgLatency) * 100
	}
}

// PrintReport writes a formatted report to the given writer.
func (r *Reporter) PrintReport(report *EvalReport, w io.Writer) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "codetect MCP Evaluation Report")
	fmt.Fprintln(w, "=================================")
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "Timestamp: %s\n", report.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(w, "Repository: %s\n", report.Config.RepoPath)
	fmt.Fprintf(w, "Test Cases: %d\n", report.Summary.TotalCases)
	fmt.Fprintln(w, "")

	// Summary table
	fmt.Fprintln(w, "Results Summary:")
	fmt.Fprintln(w, strings.Repeat("-", 75))
	fmt.Fprintf(w, "| %-18s | %-15s | %-15s | %-15s |\n", "Metric", "With MCP", "Without MCP", "Improvement")
	fmt.Fprintln(w, strings.Repeat("-", 75))
	fmt.Fprintf(w, "| %-18s | %14.1f%% | %14.1f%% | %+14.1f%% |\n",
		"Accuracy (F1)",
		report.Summary.WithMCP.AvgAccuracy*100,
		report.Summary.WithoutMCP.AvgAccuracy*100,
		report.Summary.AccuracyImprovement)
	fmt.Fprintf(w, "| %-18s | %15.0f | %15.0f | %+14.1f%% |\n",
		"Input Tokens",
		report.Summary.WithMCP.AvgInputTokens,
		report.Summary.WithoutMCP.AvgInputTokens,
		calcReduction(report.Summary.WithoutMCP.AvgInputTokens, report.Summary.WithMCP.AvgInputTokens))
	fmt.Fprintf(w, "| %-18s | %15.0f | %15.0f | %+14.1f%% |\n",
		"Output Tokens",
		report.Summary.WithMCP.AvgOutputTokens,
		report.Summary.WithoutMCP.AvgOutputTokens,
		calcReduction(report.Summary.WithoutMCP.AvgOutputTokens, report.Summary.WithMCP.AvgOutputTokens))
	fmt.Fprintf(w, "| %-18s | %15.0f | %15.0f | %+14.1f%% |\n",
		"Cache Read Tokens",
		report.Summary.WithMCP.AvgCacheReadTokens,
		report.Summary.WithoutMCP.AvgCacheReadTokens,
		calcReduction(report.Summary.WithoutMCP.AvgCacheReadTokens, report.Summary.WithMCP.AvgCacheReadTokens))
	fmt.Fprintf(w, "| %-18s | %15.0f | %15.0f | %+14.1f%% |\n",
		"Cache Create Tokens",
		report.Summary.WithMCP.AvgCacheCreateTokens,
		report.Summary.WithoutMCP.AvgCacheCreateTokens,
		calcReduction(report.Summary.WithoutMCP.AvgCacheCreateTokens, report.Summary.WithMCP.AvgCacheCreateTokens))
	fmt.Fprintf(w, "| %-18s | %15.0f | %15.0f | %+14.1f%% |\n",
		"Total Tokens",
		report.Summary.WithMCP.AvgTotalTokens,
		report.Summary.WithoutMCP.AvgTotalTokens,
		report.Summary.TokenReduction)
	fmt.Fprintf(w, "| %-18s | $%14.4f | $%14.4f | %+14.1f%% |\n",
		"Avg Cost",
		report.Summary.WithMCP.AvgCostUSD,
		report.Summary.WithoutMCP.AvgCostUSD,
		report.Summary.CostReduction)
	fmt.Fprintf(w, "| %-18s | $%14.4f | $%14.4f | %+14.1f%% |\n",
		"Total Cost",
		report.Summary.WithMCP.TotalCostUSD,
		report.Summary.WithoutMCP.TotalCostUSD,
		report.Summary.CostReduction)
	fmt.Fprintf(w, "| %-18s | %15s | %15s | %+14.1f%% |\n",
		"Avg Latency",
		formatDuration(report.Summary.WithMCP.AvgLatency),
		formatDuration(report.Summary.WithoutMCP.AvgLatency),
		report.Summary.LatencyReduction)
	fmt.Fprintf(w, "| %-18s | %15.1f | %15.1f | %+14.1f |\n",
		"Avg Turns",
		report.Summary.WithMCP.AvgTurns,
		report.Summary.WithoutMCP.AvgTurns,
		report.Summary.WithoutMCP.AvgTurns-report.Summary.WithMCP.AvgTurns)
	fmt.Fprintf(w, "| %-18s | %14.1f%% | %14.1f%% | %+14.1f%% |\n",
		"Success Rate",
		report.Summary.WithMCP.SuccessRate*100,
		report.Summary.WithoutMCP.SuccessRate*100,
		(report.Summary.WithMCP.SuccessRate-report.Summary.WithoutMCP.SuccessRate)*100)
	fmt.Fprintln(w, strings.Repeat("-", 75))
	fmt.Fprintln(w, "")

	// Per-test breakdown
	fmt.Fprintln(w, "Per-Test Results:")
	fmt.Fprintln(w, strings.Repeat("-", 90))
	fmt.Fprintf(w, "| %-12s | %-10s | %-30s | %-8s | %-8s | %-8s |\n",
		"ID", "Category", "Description", "MCP F1", "No-MCP", "Winner")
	fmt.Fprintln(w, strings.Repeat("-", 90))

	for _, cr := range report.Results {
		desc := cr.Description
		if len(desc) > 28 {
			desc = desc[:28] + ".."
		}
		winner := "-"
		if cr.Winner == ModeWithMCP {
			winner = "MCP"
		} else if cr.Winner == ModeWithoutMCP {
			winner = "No-MCP"
		}
		fmt.Fprintf(w, "| %-12s | %-10s | %-30s | %7.1f%% | %7.1f%% | %-8s |\n",
			cr.TestCaseID,
			cr.Category,
			desc,
			cr.WithMCP.F1Score*100,
			cr.WithoutMCP.F1Score*100,
			winner)
	}
	fmt.Fprintln(w, strings.Repeat("-", 90))
}

// PrintReportToStdout prints the report to stdout.
func (r *Reporter) PrintReportToStdout(report *EvalReport) {
	r.PrintReport(report, os.Stdout)
}

// SaveJSONReport saves the full report as JSON.
func (r *Reporter) SaveJSONReport(report *EvalReport, path string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling report: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// LoadReport loads a report from a JSON file.
func (r *Reporter) LoadReport(path string) (*EvalReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var report EvalReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parsing report: %w", err)
	}

	return &report, nil
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// calcReduction calculates the percentage reduction from baseline to new value.
// Positive result means reduction (new is smaller), negative means increase.
func calcReduction(baseline, new float64) float64 {
	if baseline == 0 {
		return 0
	}
	return ((baseline - new) / baseline) * 100
}
