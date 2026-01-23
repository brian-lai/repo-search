// codetect-eval runs evaluation tests comparing MCP vs non-MCP performance.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codetect/evals"
	"codetect/internal/logging"
)

var logger *slog.Logger

const version = "0.1.0"

func main() {
	logger = logging.Default("codetect-eval")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		runEval(os.Args[2:])
	case "report":
		showReport(os.Args[2:])
	case "list":
		listCases(os.Args[2:])
	case "logs":
		showLogs(os.Args[2:])
	case "version":
		fmt.Printf("codetect-eval v%s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		logger.Error("unknown command", "command", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func runEval(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	repoPath := fs.String("repo", ".", "Path to repository to evaluate")
	casesDir := fs.String("cases", "evals/cases", "Directory containing test case JSONL files")
	outputDir := fs.String("output", "evals/results", "Output directory for results")
	categories := fs.String("category", "", "Filter by category (comma-separated: search,navigate,understand)")
	timeout := fs.Duration("timeout", 5*time.Minute, "Timeout per test case")
	verbose := fs.Bool("verbose", false, "Verbose output")
	fs.Parse(args)

	config := evals.DefaultConfig()
	config.RepoPath = *repoPath
	config.OutputDir = *outputDir
	config.Timeout = *timeout
	config.Verbose = *verbose

	if *categories != "" {
		config.Categories = strings.Split(*categories, ",")
	}

	// Convert to absolute paths
	absRepoPath, err := filepath.Abs(config.RepoPath)
	if err != nil {
		logger.Error("invalid repo path", "error", err)
		os.Exit(1)
	}

	// Check if evaluating a different repo before we update config
	cwd, _ := os.Getwd()
	isExternalRepo := absRepoPath != cwd

	config.RepoPath = absRepoPath

	absCasesDir, err := filepath.Abs(*casesDir)
	if err != nil {
		logger.Error("invalid cases dir", "error", err)
		os.Exit(1)
	}

	// Create runner and load test cases
	runner := evals.NewRunner(config)

	// Ensure .codetect is in .gitignore when running against a target repo
	if isExternalRepo {
		if err := runner.EnsureGitignore(); err != nil {
			logger.Warn("could not update .gitignore", "error", err)
		}
	}

	cases, err := runner.LoadTestCases(absCasesDir)
	if err != nil {
		logger.Error("error loading test cases", "error", err)
		os.Exit(1)
	}

	if len(cases) == 0 {
		fmt.Fprintln(os.Stderr, "\n"+strings.Repeat("=", 80))
		fmt.Fprintln(os.Stderr, "ERROR: No test cases found!")
		fmt.Fprintln(os.Stderr, strings.Repeat("=", 80))
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "The eval runner could not find any test cases in:\n")
		fmt.Fprintf(os.Stderr, "  %s\n", absCasesDir)
		fmt.Fprintln(os.Stderr, "")

		// Check if we're evaluating a different repo
		repoEvalDir := filepath.Join(absRepoPath, ".codetect", "evals", "cases")
		if absRepoPath != "." {
			fmt.Fprintf(os.Stderr, "For repo-specific eval cases, create them in:\n")
			fmt.Fprintf(os.Stderr, "  %s\n", repoEvalDir)
			fmt.Fprintln(os.Stderr, "")
		}

		fmt.Fprintln(os.Stderr, "To create eval cases, start a Claude Code session in the target repo and paste:")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "--------------------------------------------------------------------------------")
		fmt.Fprintln(os.Stderr, `Create eval test cases for the codetect MCP tool in .codetect/evals/cases/

These test cases will be used by codetect-eval to measure MCP search performance
against this repository (without pre-indexing). Create JSONL files organized by
category:
- search.jsonl: keyword/regex searches, file pattern matching
- navigate.jsonl: finding definitions, references, call hierarchies
- understand.jsonl: code comprehension, architectural questions

Each line should be a JSON object with this structure:
{
  "id": "unique-id",
  "category": "search|navigate|understand",
  "description": "Brief description of what this tests",
  "prompt": "The actual question/search to ask",
  "difficulty": "easy|medium|hard",
  "ground_truth": {
    "files": ["expected/file/paths.go"],
    "symbols": ["expectedFunctionName"],
    "lines": {"file.go": [10, 20]},
    "content": ["expected snippets in output"]
  }
}

Create 5-10 test cases per category based on this repository's actual code structure.
Focus on queries that have clear, verifiable answers.`)
		fmt.Fprintln(os.Stderr, "--------------------------------------------------------------------------------")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, strings.Repeat("=", 80))
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Running %d test cases against %s\n", len(cases), absRepoPath)
	fmt.Fprintf(os.Stderr, "This will run each test case twice (with and without MCP)...\n\n")

	// Run evaluation
	ctx := context.Background()
	report, err := runner.RunAll(ctx, cases)
	if err != nil {
		logger.Error("error running evaluation", "error", err)
		os.Exit(1)
	}

	// Validate results
	validator := evals.NewValidator()
	validator.ValidateAll(cases, report)

	// Generate summary
	reporter := evals.NewReporter()
	reporter.GenerateSummary(report)

	// Save results
	if err := runner.SaveResults(report); err != nil {
		logger.Warn("could not save results", "error", err)
	}

	// Print report
	reporter.PrintReportToStdout(report)
}

func showReport(args []string) {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	resultsPath := fs.String("results", "", "Path to results JSON file")
	fs.Parse(args)

	if *resultsPath == "" {
		// Find most recent results file
		files, err := filepath.Glob("evals/results/*-results.json")
		if err != nil || len(files) == 0 {
			logger.Error("no results file found, use -results flag or run 'eval run' first")
			os.Exit(1)
		}
		*resultsPath = files[len(files)-1] // Most recent
	}

	reporter := evals.NewReporter()
	report, err := reporter.LoadReport(*resultsPath)
	if err != nil {
		logger.Error("error loading report", "error", err)
		os.Exit(1)
	}

	reporter.PrintReportToStdout(report)
}

func listCases(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	repoPath := fs.String("repo", ".", "Path to repository to evaluate")
	casesDir := fs.String("cases", "evals/cases", "Directory containing test case JSONL files")
	category := fs.String("category", "", "Filter by category")
	fs.Parse(args)

	config := evals.DefaultConfig()
	config.RepoPath = *repoPath
	if *category != "" {
		config.Categories = []string{*category}
	}

	// Convert to absolute path
	absRepoPath, err := filepath.Abs(config.RepoPath)
	if err != nil {
		logger.Error("invalid repo path", "error", err)
		os.Exit(1)
	}
	config.RepoPath = absRepoPath

	absCasesDir, err := filepath.Abs(*casesDir)
	if err != nil {
		logger.Error("invalid cases dir", "error", err)
		os.Exit(1)
	}

	runner := evals.NewRunner(config)
	cases, err := runner.LoadTestCases(absCasesDir)
	if err != nil {
		logger.Error("error loading test cases", "error", err)
		os.Exit(1)
	}

	fmt.Printf("Test Cases (%d total):\n\n", len(cases))
	for _, tc := range cases {
		fmt.Printf("  %-15s [%-10s] %s\n", tc.ID, tc.Category, tc.Description)
	}
}

func showLogs(args []string) {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	repoPath := fs.String("repo", ".", "Path to repository")
	testCase := fs.String("case", "", "Filter by test case ID")
	mode := fs.String("mode", "", "Filter by mode (with_mcp, without_mcp)")
	latest := fs.Bool("latest", false, "Show only the latest log")
	listOnly := fs.Bool("list", false, "List logs without showing content")
	fs.Parse(args)

	config := evals.DefaultConfig()

	absRepoPath, err := filepath.Abs(*repoPath)
	if err != nil {
		logger.Error("invalid repo path", "error", err)
		os.Exit(1)
	}
	config.RepoPath = absRepoPath

	runner := evals.NewRunner(config)
	logs, err := runner.ListLogs()
	if err != nil {
		logger.Error("error listing logs", "error", err)
		os.Exit(1)
	}

	if len(logs) == 0 {
		logger.Info("no logs found, run 'codetect-eval run' first to generate logs")
		os.Exit(1)
	}

	// Apply filters
	var filtered []evals.LogEntry
	for _, log := range logs {
		if *testCase != "" && log.TestCase != *testCase {
			continue
		}
		if *mode != "" && string(log.Mode) != *mode {
			continue
		}
		filtered = append(filtered, log)
	}

	if len(filtered) == 0 {
		logger.Info("no logs match the specified filters")
		os.Exit(1)
	}

	// If --latest, only show the most recent
	if *latest {
		filtered = filtered[:1]
	}

	// If --list, just show the list
	if *listOnly {
		fmt.Printf("Logs (%d total):\n\n", len(filtered))
		for _, log := range filtered {
			fmt.Printf("  %s  %-15s  %-12s  %s\n",
				log.Timestamp.Format("2006-01-02 15:04:05"),
				log.TestCase,
				log.Mode,
				formatBytes(log.Size))
		}
		return
	}

	// Stream log contents to stdout
	for i, log := range filtered {
		if len(filtered) > 1 {
			fmt.Printf("=== %s [%s] %s ===\n", log.TestCase, log.Mode, log.Timestamp.Format("2006-01-02 15:04:05"))
		}

		content, err := runner.ReadLog(log.Path)
		if err != nil {
			logger.Error("error reading log", "path", log.Path, "error", err)
			continue
		}

		os.Stdout.Write(content)

		if len(filtered) > 1 && i < len(filtered)-1 {
			fmt.Println()
		}
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func printUsage() {
	fmt.Println(`codetect-eval - Evaluate MCP vs non-MCP performance

Usage:
  codetect-eval <command> [options]

Commands:
  run      Run evaluation test cases
  report   Display a saved report
  list     List available test cases
  logs     View raw Claude output logs from eval runs
  version  Print version
  help     Show this help

Run Options:
  --repo <path>      Repository to evaluate (default: .)
  --cases <dir>      Test cases directory (default: evals/cases)
  --output <dir>     Output directory (default: evals/results)
  --category <cat>   Filter by category (search,navigate,understand)
  --timeout <dur>    Timeout per test (default: 5m)
  --verbose          Verbose output

Report Options:
  --results <path>   Path to results JSON file

Logs Options:
  --repo <path>      Repository to view logs for (default: .)
  --case <id>        Filter by test case ID
  --mode <mode>      Filter by mode (with_mcp, without_mcp)
  --latest           Show only the most recent log
  --list             List logs without showing content

Examples:
  # Run all tests on current directory
  codetect-eval run

  # Run only search tests on a specific repo
  codetect-eval run --repo /path/to/project --category search

  # View the most recent report
  codetect-eval report

  # List available test cases
  codetect-eval list

  # List all logs for a repo
  codetect-eval logs --repo /path/to/project --list

  # View the latest log
  codetect-eval logs --repo /path/to/project --latest

  # View logs for a specific test case
  codetect-eval logs --repo /path/to/project --case search-001`)
}
