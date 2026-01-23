package keyword

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Result represents a single search match
type Result struct {
	Path      string `json:"path"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	Snippet   string `json:"snippet"`
	Score     int    `json:"score"`
}

// SearchResult is the output of a keyword search
type SearchResult struct {
	Results []Result `json:"results"`
}

// RipgrepMatch represents a match from rg --json output
type RipgrepMatch struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type RipgrepMatchData struct {
	Path struct {
		Text string `json:"text"`
	} `json:"path"`
	Lines struct {
		Text string `json:"text"`
	} `json:"lines"`
	LineNumber  int `json:"line_number"`
	AbsoluteOffset int `json:"absolute_offset"`
}

// Search performs a keyword search using ripgrep
func Search(query string, root string, topK int) (*SearchResult, error) {
	if topK <= 0 {
		topK = 20
	}

	// Use rg --json for structured output
	args := []string{
		"--json",
		"--max-count", strconv.Itoa(topK * 2), // get extra in case of filtering
		"--no-messages",
		query,
	}

	if root != "" && root != "." {
		args = append(args, root)
	}

	cmd := exec.Command("rg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting ripgrep: %w", err)
	}

	var results []Result
	scanner := bufio.NewScanner(stdout)
	score := 100 // simple ranking: first match is highest

	for scanner.Scan() && len(results) < topK {
		line := scanner.Text()
		result, ok := parseRipgrepJSON(line, root)
		if ok {
			result.Score = score
			results = append(results, result)
			score--
		}
	}

	// Capture stderr for error messages
	stderrScanner := bufio.NewScanner(stderr)
	var stderrLines []string
	for stderrScanner.Scan() {
		stderrLines = append(stderrLines, stderrScanner.Text())
	}

	// Wait for command to finish, exit code 1 = no matches (not an error)
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()

			// Exit code 1 = no matches found (not an error)
			if exitCode == 1 {
				return &SearchResult{Results: []Result{}}, nil
			}

			// Exit code 2 = error (invalid regex, bad flags, etc)
			if exitCode == 2 {
				errorMsg := "invalid search pattern or parameters"
				if len(stderrLines) > 0 {
					// Use ripgrep's actual error message
					errorMsg = strings.Join(stderrLines, "; ")
				}
				return nil, fmt.Errorf("ripgrep error (exit code 2): %s", errorMsg)
			}
		}

		// Check if we got results despite the error
		if len(results) > 0 {
			return &SearchResult{Results: results}, nil
		}

		// Generic error with stderr if available
		if len(stderrLines) > 0 {
			return nil, fmt.Errorf("ripgrep error: %w (%s)", err, strings.Join(stderrLines, "; "))
		}
		return nil, fmt.Errorf("ripgrep error: %w", err)
	}

	return &SearchResult{Results: results}, nil
}

// parseRipgrepJSON parses a single line of rg --json output
func parseRipgrepJSON(line string, root string) (Result, bool) {
	var match RipgrepMatch
	if err := json.Unmarshal([]byte(line), &match); err != nil {
		return Result{}, false
	}

	if match.Type != "match" {
		return Result{}, false
	}

	var data RipgrepMatchData
	if err := json.Unmarshal(match.Data, &data); err != nil {
		return Result{}, false
	}

	path := data.Path.Text
	if root != "" && root != "." {
		// Make path relative to root if possible
		if rel, err := filepath.Rel(root, path); err == nil {
			path = rel
		}
	}

	snippet := strings.TrimRight(data.Lines.Text, "\n\r")

	return Result{
		Path:      path,
		LineStart: data.LineNumber,
		LineEnd:   data.LineNumber,
		Snippet:   snippet,
	}, true
}

// SearchBasic is a fallback using simple rg output (no --json)
func SearchBasic(query string, root string, topK int) (*SearchResult, error) {
	if topK <= 0 {
		topK = 20
	}

	args := []string{
		"--line-number",
		"--no-heading",
		"--color", "never",
		"--max-count", strconv.Itoa(topK * 2),
		query,
	}

	if root != "" && root != "." {
		args = append(args, root)
	}

	cmd := exec.Command("rg", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()

			// Exit code 1 = no matches found (not an error)
			if exitCode == 1 {
				return &SearchResult{Results: []Result{}}, nil
			}

			// Exit code 2 = error (invalid regex, bad flags, etc)
			if exitCode == 2 {
				stderr := string(exitErr.Stderr)
				errorMsg := "invalid search pattern or parameters"
				if stderr != "" {
					errorMsg = strings.TrimSpace(stderr)
				}
				return nil, fmt.Errorf("ripgrep error (exit code 2): %s", errorMsg)
			}

			// Other exit codes with stderr
			if len(exitErr.Stderr) > 0 {
				return nil, fmt.Errorf("ripgrep error: %w (%s)", err, strings.TrimSpace(string(exitErr.Stderr)))
			}
		}
		return nil, fmt.Errorf("ripgrep error: %w", err)
	}

	return parseBasicOutput(string(output), root, topK), nil
}

// parseBasicOutput parses rg --line-number --no-heading output
// Format: path:line:content
func parseBasicOutput(output string, root string, topK int) *SearchResult {
	var results []Result
	lines := strings.Split(output, "\n")
	score := 100

	for _, line := range lines {
		if len(results) >= topK {
			break
		}
		if line == "" {
			continue
		}

		result, ok := parseBasicLine(line, root)
		if ok {
			result.Score = score
			results = append(results, result)
			score--
		}
	}

	return &SearchResult{Results: results}
}

// parseBasicLine parses a single line: path:linenum:content
func parseBasicLine(line string, root string) (Result, bool) {
	// Find first colon (path separator)
	firstColon := strings.Index(line, ":")
	if firstColon == -1 {
		return Result{}, false
	}

	// Find second colon (line number separator)
	rest := line[firstColon+1:]
	secondColon := strings.Index(rest, ":")
	if secondColon == -1 {
		return Result{}, false
	}

	path := line[:firstColon]
	lineNumStr := rest[:secondColon]
	content := rest[secondColon+1:]

	lineNum, err := strconv.Atoi(lineNumStr)
	if err != nil {
		return Result{}, false
	}

	if root != "" && root != "." {
		if rel, err := filepath.Rel(root, path); err == nil {
			path = rel
		}
	}

	return Result{
		Path:      path,
		LineStart: lineNum,
		LineEnd:   lineNum,
		Snippet:   content,
	}, true
}
