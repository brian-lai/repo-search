# Performance Evaluation Guide

repo-search includes a comprehensive evaluation tool to measure the performance improvement of MCP tools vs. standard CLI tools (grep, find, etc.) when working with Claude Code.

## Overview

The `repo-search-eval` tool runs test cases against your codebase twice: once with MCP tools enabled, and once with only standard CLI tools. This provides quantitative data on the benefits of using repo-search.

## Quick Start

```bash
# Run all test cases on a target repository
repo-search-eval run --repo /path/to/project

# Run only specific categories
repo-search-eval run --repo /path/to/project --category search,navigate

# View the most recent results
repo-search-eval report

# List available test cases
repo-search-eval list --repo /path/to/project
```

## Commands

### run

Run evaluation test cases against a repository.

```bash
repo-search-eval run [options]
```

**Options:**
- `--repo <path>` - Repository to evaluate (default: current directory)
- `--cases <dir>` - Test cases directory (default: evals/cases)
- `--output <dir>` - Output directory for results (default: evals/results)
- `--category <cat>` - Filter by category (search, navigate, understand)
- `--timeout <dur>` - Timeout per test case (default: 5m)
- `--verbose` - Verbose output

**Examples:**

```bash
# Evaluate current repository with all test cases
repo-search-eval run

# Evaluate specific repository with only search tests
repo-search-eval run --repo /path/to/project --category search

# Run with custom timeout and verbose output
repo-search-eval run --repo /path/to/project --timeout 10m --verbose
```

### report

Display a saved evaluation report.

```bash
repo-search-eval report [options]
```

**Options:**
- `--results <path>` - Path to specific results JSON file (default: most recent)

**Examples:**

```bash
# Show the most recent report
repo-search-eval report

# Show a specific report
repo-search-eval report --results .repo-search/evals/results/2024-01-10-120000-results.json
```

### list

List available test cases.

```bash
repo-search-eval list [options]
```

**Options:**
- `--repo <path>` - Repository path (default: current directory)
- `--cases <dir>` - Test cases directory (default: evals/cases)
- `--category <cat>` - Filter by category

**Examples:**

```bash
# List all test cases in current directory
repo-search-eval list

# List test cases for a specific repository
repo-search-eval list --repo /path/to/project

# List only navigation test cases
repo-search-eval list --category navigate
```

## Creating Eval Cases for Your Repository

When you run evals on a repository without test cases, you'll see a helpful error message suggesting how to create them.

### Repository-Specific Storage

Eval data is stored in a `.repo-search/` directory within the target repository:

```
your-project/
├── .repo-search/           # Auto-added to .gitignore
│   └── evals/
│       ├── cases/          # Test case JSONL files
│       │   ├── search.jsonl
│       │   ├── navigate.jsonl
│       │   └── understand.jsonl
│       └── results/        # Evaluation results (JSON)
│           └── 2024-01-10-120000-results.json
```

This approach:
- Keeps eval cases version-controlled with your codebase
- Stores results locally (via .gitignore)
- Allows different repos to have different test cases

### Manual Creation

Create the directory structure and add JSONL files:

```bash
mkdir -p .repo-search/evals/cases
```

Create test case files (e.g., `.repo-search/evals/cases/search.jsonl`):

```jsonl
{"id":"search-001","category":"search","description":"Find error handling","prompt":"Find all error handling code in this repository","ground_truth":{"files":["internal/errors.go","pkg/handler.go"]},"difficulty":"easy"}
{"id":"search-002","category":"search","description":"Find HTTP handlers","prompt":"Find all HTTP request handlers","ground_truth":{"files":["internal/handlers/","pkg/api/"]},"difficulty":"medium"}
```

### AI-Assisted Creation

Use Claude or another AI assistant to generate test cases:

```
Create eval test cases for this repository in .repo-search/evals/cases/
Include search, navigation, and code understanding test cases in JSONL format.

Categories:
- search: Finding code patterns, keywords, or concepts
- navigate: Locating specific symbols, definitions, or implementations
- understand: Comprehending code structure, relationships, or architecture

Create at least 10 test cases across these categories with realistic prompts
that developers would actually ask when working with this codebase.
```

## Test Case Format

Each test case is a JSON object on a single line (JSONL format).

### Required Fields

```json
{
  "id": "unique-test-id",
  "category": "search|navigate|understand",
  "description": "Brief description of what this tests",
  "prompt": "The actual prompt given to Claude",
  "ground_truth": {
    "files": ["expected/file1.go", "expected/file2.go"],
    "symbols": ["ExpectedSymbol1", "ExpectedSymbol2"],
    "lines": {"file.go": [10, 25, 42]},
    "content": ["expected code snippet"]
  },
  "difficulty": "easy|medium|hard"
}
```

### Field Descriptions

- **id**: Unique identifier for the test case (e.g., "search-001", "navigate-005")
- **category**: Test category (search, navigate, or understand)
- **description**: Human-readable description for reports
- **prompt**: The exact prompt given to Claude Code
- **ground_truth**: Expected results for validation
  - **files**: List of file paths that should be found/mentioned
  - **symbols**: List of symbol names (functions, types, etc.)
  - **lines**: Map of files to line numbers
  - **content**: Expected code snippets or text
- **difficulty**: Subjective difficulty rating

### Example Test Cases

**Search Example:**

```json
{"id":"search-001","category":"search","description":"Find authentication logic","prompt":"Find all code related to user authentication","ground_truth":{"files":["internal/auth/","pkg/middleware/auth.go"],"symbols":["Authenticate","ValidateToken"]},"difficulty":"medium"}
```

**Navigate Example:**

```json
{"id":"navigate-001","category":"navigate","description":"Find Server struct definition","prompt":"Find where the Server struct is defined","ground_truth":{"files":["internal/server/server.go"],"symbols":["Server"],"lines":{"internal/server/server.go":[15]}},"difficulty":"easy"}
```

**Understand Example:**

```json
{"id":"understand-001","category":"understand","description":"Explain request flow","prompt":"Explain how HTTP requests are processed from router to handler","ground_truth":{"files":["internal/router/","internal/handlers/","pkg/middleware/"]},"difficulty":"hard"}
```

## Test Case Categories

### search
Finding code patterns, keywords, or concepts across the codebase.

**Good prompts:**
- "Find all error handling code"
- "Find where we make HTTP requests"
- "Search for database query code"

**Ground truth:** Files and symbols that match the search criteria

### navigate
Locating specific symbols, definitions, or implementations.

**Good prompts:**
- "Find the definition of the Server struct"
- "Where is the HandleRequest function implemented?"
- "Find all implementations of the Handler interface"

**Ground truth:** Specific files, symbols, and line numbers

### understand
Comprehending code structure, relationships, or architecture.

**Good prompts:**
- "Explain the authentication flow"
- "How does the caching system work?"
- "What happens when a user logs in?"

**Ground truth:** Key files involved in the concept (harder to validate precisely)

## Metrics Measured

Each test case runs twice (with and without MCP tools) and measures:

### Accuracy
- **Precision**: Correct items / Total returned
- **Recall**: Correct items / Total expected
- **F1 Score**: Harmonic mean of precision and recall

Based on comparing results against the ground truth.

### Performance
- **Token usage**: Input tokens, output tokens, cache reads, cache creation
- **Latency**: Time to complete the task
- **Cost**: Estimated API cost in USD
- **Turns**: Number of back-and-forth interactions

### Success
- **Success rate**: Percentage of test cases that completed successfully
- **Error rate**: Percentage that failed or timed out

## Understanding Results

After running evaluations, you'll see a summary report:

```
Evaluation Report
=================
Total Cases: 30

With MCP Tools:
  Avg Accuracy: 92.5%
  Avg Tokens: 12,450
  Avg Cost: $0.15
  Avg Latency: 8.2s
  Success Rate: 96.7%

Without MCP Tools:
  Avg Accuracy: 78.3%
  Avg Tokens: 28,900
  Avg Cost: $0.42
  Avg Latency: 15.6s
  Success Rate: 86.7%

Improvements:
  Accuracy: +14.2%
  Token Reduction: 56.9%
  Cost Reduction: 64.3%
  Latency Reduction: 47.4%
```

### Key Takeaways

**Higher accuracy** means MCP tools help Claude find the right code more reliably.

**Lower token usage** means less context switching and more efficient searches, leading to lower costs.

**Lower latency** means faster responses, improving developer productivity.

**Higher success rate** means fewer failed attempts and timeout errors.

## Best Practices

### Writing Good Test Cases

1. **Use realistic prompts** - Write prompts you'd actually use during development
2. **Cover different difficulties** - Include easy, medium, and hard test cases
3. **Test edge cases** - Include ambiguous queries, large search spaces, etc.
4. **Keep ground truth accurate** - Update when code changes
5. **Balance categories** - Mix search, navigate, and understand tests

### Running Evaluations

1. **Index first** - Run `repo-search index` before evaluating
2. **Use consistent versions** - Don't upgrade mid-evaluation
3. **Run multiple times** - Results can vary; average over multiple runs
4. **Document context** - Note repo size, language, domain in reports

### Maintaining Test Cases

1. **Update with code changes** - Keep ground truth in sync
2. **Add new tests** - When you encounter interesting queries
3. **Remove stale tests** - Delete tests for removed features
4. **Version control cases** - Commit `.repo-search/evals/cases/` to git
5. **Ignore results** - Keep `.repo-search/` in .gitignore (results are auto-added)

## Troubleshooting

### No test cases found

**Error:**
```
ERROR: No test cases found!
The eval runner could not find any test cases in: /path/to/repo/.repo-search/evals/cases
```

**Solution:** Create test cases using the manual or AI-assisted approach above.

### Timeout errors

If test cases are timing out, increase the timeout:

```bash
repo-search-eval run --repo /path/to/project --timeout 10m
```

### Low accuracy scores

If accuracy is low across the board:
1. Check that ground truth is correct
2. Verify the repo is indexed: `repo-search stats`
3. Try simpler prompts to isolate the issue

### High token usage with MCP

If MCP tools aren't reducing tokens:
1. Check that MCP is actually being used (look for tool calls in verbose output)
2. Verify `.mcp.json` is configured correctly
3. Ensure repo-search is running: `ps aux | grep repo-search`

## Advanced Usage

### Custom Test Suites

Organize test cases by feature or module:

```
.repo-search/evals/cases/
├── auth/
│   ├── search.jsonl
│   └── navigate.jsonl
├── api/
│   ├── search.jsonl
│   └── navigate.jsonl
└── database/
    └── search.jsonl
```

The eval tool will recursively find all `*.jsonl` files.

### Continuous Evaluation

Add eval runs to your CI pipeline:

```bash
# .github/workflows/eval.yml
- name: Run repo-search evals
  run: |
    repo-search index
    repo-search-eval run --repo . --output ci-results/
```

### Comparing Versions

Run evals before and after upgrading repo-search:

```bash
# Before upgrade
repo-search-eval run --repo .
cp .repo-search/evals/results/latest.json before.json

# Upgrade
repo-search update

# After upgrade
repo-search-eval run --repo .
cp .repo-search/evals/results/latest.json after.json

# Compare (manually or with custom script)
```

## FAQ

**Q: Do I need to create test cases for every project?**

A: No, you can use the same test cases across similar projects, but custom cases will be more accurate.

**Q: Can I run evals without semantic search enabled?**

A: Yes, semantic search is optional. The eval tool will work with just keyword search.

**Q: How long does it take to run evals?**

A: Depends on the number of test cases and timeout settings. 30 test cases typically take 10-15 minutes.

**Q: Are eval results deterministic?**

A: No, Claude's responses can vary. Run multiple times and average results for reliability.

**Q: Can I share eval results?**

A: Yes, results are stored as JSON files. You can share them or aggregate across teams.

**Q: What if my ground truth is wrong?**

A: Update the JSONL file with the correct ground truth and re-run. Results are only as good as your ground truth.

## Related Documentation

- [Installation Guide](installation.md) - Setup repo-search before running evals
- [Architecture](architecture.md) - Understanding how MCP tools work
- [MCP Compatibility](mcp-compatibility.md) - Supported tools and configurations
