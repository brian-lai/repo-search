package config

import (
	"os"
	"strings"
)

// IndexBackend specifies which symbol indexing backend to use
type IndexBackend string

const (
	// IndexBackendAuto uses ast-grep for supported languages, ctags for others (default)
	IndexBackendAuto IndexBackend = "auto"

	// IndexBackendAstGrep uses ast-grep only (errors on unsupported languages)
	IndexBackendAstGrep IndexBackend = "ast-grep"

	// IndexBackendCtags uses universal-ctags only (legacy behavior)
	IndexBackendCtags IndexBackend = "ctags"
)

// IndexConfig holds configuration for symbol indexing
type IndexConfig struct {
	// Backend specifies which indexing tool to use
	Backend IndexBackend
}

// LoadIndexConfigFromEnv loads indexing configuration from environment variables.
// Supports the following variable:
//   - CODETECT_INDEX_BACKEND: Backend to use ("auto", "ast-grep", or "ctags")
//
// If no environment variable is set, defaults to "auto" (hybrid approach).
func LoadIndexConfigFromEnv() IndexConfig {
	cfg := IndexConfig{
		Backend: IndexBackendAuto, // Default to hybrid
	}

	if backend := os.Getenv("CODETECT_INDEX_BACKEND"); backend != "" {
		switch strings.ToLower(backend) {
		case "auto", "hybrid":
			cfg.Backend = IndexBackendAuto
		case "ast-grep", "astgrep", "sg":
			cfg.Backend = IndexBackendAstGrep
		case "ctags", "universal-ctags":
			cfg.Backend = IndexBackendCtags
		default:
			// Unknown backend, use default
			cfg.Backend = IndexBackendAuto
		}
	}

	return cfg
}

// UseAstGrep returns true if ast-grep should be used for indexing
func (c IndexConfig) UseAstGrep() bool {
	return c.Backend == IndexBackendAuto || c.Backend == IndexBackendAstGrep
}

// UseCtags returns true if ctags should be used for indexing
func (c IndexConfig) UseCtags() bool {
	return c.Backend == IndexBackendAuto || c.Backend == IndexBackendCtags
}

// RequireAstGrep returns true if ast-grep is required (not optional)
func (c IndexConfig) RequireAstGrep() bool {
	return c.Backend == IndexBackendAstGrep
}

// String returns a human-readable description of the index configuration
func (c IndexConfig) String() string {
	switch c.Backend {
	case IndexBackendAstGrep:
		return "ast-grep only"
	case IndexBackendCtags:
		return "universal-ctags only"
	default:
		return "auto (ast-grep + ctags fallback)"
	}
}
