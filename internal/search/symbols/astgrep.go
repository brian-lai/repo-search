package symbols

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// AstGrepEntry represents a single match from ast-grep JSON output
type AstGrepEntry struct {
	Text  string            `json:"text"`
	Range AstGrepRange      `json:"range"`
	File  string            `json:"file"`
	Meta  map[string]string `json:"metaVariables,omitempty"`
}

// AstGrepRange represents the location of a match
type AstGrepRange struct {
	ByteOffset struct {
		Start int `json:"start"`
		End   int `json:"end"`
	} `json:"byteOffset"`
	Start AstGrepPosition `json:"start"`
	End   AstGrepPosition `json:"end"`
}

// AstGrepPosition represents a line/column position
type AstGrepPosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// LanguagePattern defines symbol extraction patterns for a language
type LanguagePattern struct {
	Language string
	Patterns []SymbolPattern
}

// SymbolPattern defines a pattern for extracting a specific symbol type
type SymbolPattern struct {
	Kind    string // function, class, struct, etc.
	Pattern string // ast-grep pattern
}

// AstGrepAvailable checks if ast-grep (or sg) is installed
func AstGrepAvailable() bool {
	// Try "ast-grep" first
	if _, err := exec.LookPath("ast-grep"); err == nil {
		return true
	}
	// Try "sg" as alternative
	if _, err := exec.LookPath("sg"); err == nil {
		return true
	}
	return false
}

// getAstGrepBinary returns the available ast-grep binary name
func getAstGrepBinary() string {
	if _, err := exec.LookPath("ast-grep"); err == nil {
		return "ast-grep"
	}
	return "sg"
}

// GetLanguagePatterns returns symbol extraction patterns for a language
func GetLanguagePatterns(language string) *LanguagePattern {
	patterns := map[string]LanguagePattern{
		"go": {
			Language: "go",
			Patterns: []SymbolPattern{
				{Kind: "function", Pattern: "func $NAME($$$) $$$"},
				{Kind: "function", Pattern: "func $NAME($$$) ($$$) $$$"},
				{Kind: "method", Pattern: "func ($$$) $NAME($$$) $$$"},
				{Kind: "method", Pattern: "func ($$$) $NAME($$$) ($$$) $$$"},
				{Kind: "struct", Pattern: "type $NAME struct { $$$ }"},
				{Kind: "interface", Pattern: "type $NAME interface { $$$ }"},
				{Kind: "type", Pattern: "type $NAME $$$"},
			},
		},
		"typescript": {
			Language: "typescript",
			Patterns: []SymbolPattern{
				{Kind: "function", Pattern: "function $NAME($$$) { $$$ }"},
				{Kind: "function", Pattern: "const $NAME = ($$$) => $$$"},
				{Kind: "function", Pattern: "let $NAME = ($$$) => $$$"},
				{Kind: "function", Pattern: "export function $NAME($$$) { $$$ }"},
				{Kind: "class", Pattern: "class $NAME { $$$ }"},
				{Kind: "class", Pattern: "export class $NAME { $$$ }"},
				{Kind: "interface", Pattern: "interface $NAME { $$$ }"},
				{Kind: "interface", Pattern: "export interface $NAME { $$$ }"},
				{Kind: "type", Pattern: "type $NAME = $$$"},
				{Kind: "type", Pattern: "export type $NAME = $$$"},
			},
		},
		"javascript": {
			Language: "javascript",
			Patterns: []SymbolPattern{
				{Kind: "function", Pattern: "function $NAME($$$) { $$$ }"},
				{Kind: "function", Pattern: "const $NAME = ($$$) => $$$"},
				{Kind: "function", Pattern: "let $NAME = ($$$) => $$$"},
				{Kind: "function", Pattern: "export function $NAME($$$) { $$$ }"},
				{Kind: "class", Pattern: "class $NAME { $$$ }"},
				{Kind: "class", Pattern: "export class $NAME { $$$ }"},
			},
		},
		"python": {
			Language: "python",
			Patterns: []SymbolPattern{
				{Kind: "function", Pattern: "def $NAME($$$): $$$"},
				{Kind: "class", Pattern: "class $NAME: $$$"},
				{Kind: "class", Pattern: "class $NAME($$$): $$$"},
			},
		},
		"rust": {
			Language: "rust",
			Patterns: []SymbolPattern{
				{Kind: "function", Pattern: "fn $NAME($$$) $$$"},
				{Kind: "function", Pattern: "pub fn $NAME($$$) $$$"},
				{Kind: "struct", Pattern: "struct $NAME { $$$ }"},
				{Kind: "struct", Pattern: "pub struct $NAME { $$$ }"},
				{Kind: "enum", Pattern: "enum $NAME { $$$ }"},
				{Kind: "enum", Pattern: "pub enum $NAME { $$$ }"},
				{Kind: "trait", Pattern: "trait $NAME { $$$ }"},
				{Kind: "trait", Pattern: "pub trait $NAME { $$$ }"},
			},
		},
		"java": {
			Language: "java",
			Patterns: []SymbolPattern{
				{Kind: "method", Pattern: "public $$$  $NAME($$$) { $$$ }"},
				{Kind: "method", Pattern: "private $$$ $NAME($$$) { $$$ }"},
				{Kind: "method", Pattern: "protected $$$ $NAME($$$) { $$$ }"},
				{Kind: "class", Pattern: "class $NAME { $$$ }"},
				{Kind: "class", Pattern: "public class $NAME { $$$ }"},
				{Kind: "interface", Pattern: "interface $NAME { $$$ }"},
				{Kind: "interface", Pattern: "public interface $NAME { $$$ }"},
			},
		},
		"c": {
			Language: "c",
			Patterns: []SymbolPattern{
				{Kind: "function", Pattern: "$$$  $NAME($$$) { $$$ }"},
				{Kind: "struct", Pattern: "struct $NAME { $$$ }"},
				{Kind: "type", Pattern: "typedef $$$ $NAME"},
			},
		},
		"cpp": {
			Language: "cpp",
			Patterns: []SymbolPattern{
				{Kind: "function", Pattern: "$$$  $NAME($$$) { $$$ }"},
				{Kind: "class", Pattern: "class $NAME { $$$ }"},
				{Kind: "struct", Pattern: "struct $NAME { $$$ }"},
			},
		},
		"ruby": {
			Language: "ruby",
			Patterns: []SymbolPattern{
				{Kind: "function", Pattern: "def $NAME $$$ end"},
				{Kind: "function", Pattern: "def $NAME($$$) $$$ end"},
				{Kind: "class", Pattern: "class $NAME $$$ end"},
				{Kind: "class", Pattern: "class $NAME < $$$ end"},
				{Kind: "module", Pattern: "module $NAME $$$ end"},
			},
		},
		"php": {
			Language: "php",
			Patterns: []SymbolPattern{
				{Kind: "function", Pattern: "function $NAME($$$) { $$$ }"},
				{Kind: "method", Pattern: "public function $NAME($$$) { $$$ }"},
				{Kind: "method", Pattern: "private function $NAME($$$) { $$$ }"},
				{Kind: "method", Pattern: "protected function $NAME($$$) { $$$ }"},
				{Kind: "class", Pattern: "class $NAME { $$$ }"},
			},
		},
		"csharp": {
			Language: "csharp",
			Patterns: []SymbolPattern{
				{Kind: "method", Pattern: "public $$$ $NAME($$$) { $$$ }"},
				{Kind: "method", Pattern: "private $$$ $NAME($$$) { $$$ }"},
				{Kind: "method", Pattern: "protected $$$ $NAME($$$) { $$$ }"},
				{Kind: "class", Pattern: "class $NAME { $$$ }"},
				{Kind: "class", Pattern: "public class $NAME { $$$ }"},
				{Kind: "interface", Pattern: "interface $NAME { $$$ }"},
			},
		},
		"kotlin": {
			Language: "kotlin",
			Patterns: []SymbolPattern{
				{Kind: "function", Pattern: "fun $NAME($$$) $$$"},
				{Kind: "class", Pattern: "class $NAME { $$$ }"},
				{Kind: "interface", Pattern: "interface $NAME { $$$ }"},
			},
		},
		"swift": {
			Language: "swift",
			Patterns: []SymbolPattern{
				{Kind: "function", Pattern: "func $NAME($$$) $$$"},
				{Kind: "class", Pattern: "class $NAME { $$$ }"},
				{Kind: "struct", Pattern: "struct $NAME { $$$ }"},
				{Kind: "protocol", Pattern: "protocol $NAME { $$$ }"},
			},
		},
	}

	if lp, ok := patterns[strings.ToLower(language)]; ok {
		return &lp
	}
	return nil
}

// LanguageFromExtension returns the language name for ast-grep based on file extension
func LanguageFromExtension(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	extMap := map[string]string{
		".go":     "go",
		".ts":     "typescript",
		".tsx":    "typescript",
		".js":     "javascript",
		".jsx":    "javascript",
		".mjs":    "javascript",
		".py":     "python",
		".rs":     "rust",
		".java":   "java",
		".c":      "c",
		".h":      "c",
		".cpp":    "cpp",
		".cc":     "cpp",
		".cxx":    "cpp",
		".hpp":    "cpp",
		".hh":     "cpp",
		".rb":     "ruby",
		".php":    "php",
		".cs":     "csharp",
		".kt":     "kotlin",
		".swift":  "swift",
	}
	return extMap[ext]
}

// SupportedLanguages returns list of languages supported by ast-grep
func SupportedLanguages() []string {
	return []string{
		"go", "typescript", "javascript", "python", "rust",
		"java", "c", "cpp", "ruby", "php", "csharp",
		"kotlin", "swift",
	}
}

// RunAstGrep runs ast-grep on the given files and returns symbols
func RunAstGrep(root string, files []string, language string) ([]Symbol, error) {
	if !AstGrepAvailable() {
		return nil, fmt.Errorf("ast-grep not available")
	}

	langPatterns := GetLanguagePatterns(language)
	if langPatterns == nil {
		return nil, fmt.Errorf("no patterns defined for language: %s", language)
	}

	var allSymbols []Symbol
	binary := getAstGrepBinary()

	// Run ast-grep for each pattern type
	for _, pattern := range langPatterns.Patterns {
		args := []string{
			"--json",
			"--pattern", pattern.Pattern,
			"--lang", langPatterns.Language,
		}

		// Add file paths
		args = append(args, files...)

		cmd := exec.Command(binary, args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			// ast-grep returns non-zero if no matches found, which is OK
			// Only error if stderr has content
			if stderr.Len() > 0 {
				return nil, fmt.Errorf("ast-grep error: %s", stderr.String())
			}
		}

		// Parse JSON output line by line
		scanner := bufio.NewScanner(&stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var entry AstGrepEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue // Skip malformed entries
			}

			// Convert to Symbol
			symbol := astGrepEntryToSymbol(entry, pattern.Kind, root)
			allSymbols = append(allSymbols, symbol)
		}
	}

	return deduplicateSymbols(allSymbols), nil
}

// astGrepEntryToSymbol converts an ast-grep entry to a Symbol
func astGrepEntryToSymbol(entry AstGrepEntry, kind string, root string) Symbol {
	// Extract name from meta variables if available
	name := entry.Meta["NAME"]
	if name == "" {
		// Fallback: try to extract from first line of match
		lines := strings.Split(entry.Text, "\n")
		if len(lines) > 0 {
			name = extractNameFromText(lines[0])
		}
	}

	// Make path relative to root
	relPath := entry.File
	if root != "" && root != "." {
		if rel, err := filepath.Rel(root, entry.File); err == nil {
			relPath = rel
		}
	}

	return Symbol{
		Name:     name,
		Kind:     normalizeKind(kind),
		Path:     relPath,
		Line:     entry.Range.Start.Line,
		Language: "", // Will be set by caller
		Pattern:  strings.TrimSpace(entry.Text),
	}
}

// extractNameFromText tries to extract a symbol name from text
func extractNameFromText(text string) string {
	// Remove common keywords
	text = strings.TrimSpace(text)
	for _, prefix := range []string{"func ", "function ", "def ", "class ", "interface ", "type ", "struct ", "enum ", "trait "} {
		text = strings.TrimPrefix(text, prefix)
	}

	// Extract first word (usually the name)
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == '(' || r == '{' || r == '<' || r == ':' || r == '='
	})
	if len(fields) > 0 {
		return strings.TrimSpace(fields[0])
	}

	return "unknown"
}

// deduplicateSymbols removes duplicate symbols based on path+line+name
func deduplicateSymbols(symbols []Symbol) []Symbol {
	seen := make(map[string]bool)
	var unique []Symbol

	for _, sym := range symbols {
		key := fmt.Sprintf("%s:%d:%s", sym.Path, sym.Line, sym.Name)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, sym)
		}
	}

	return unique
}
