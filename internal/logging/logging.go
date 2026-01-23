// Package logging provides structured logging using Go's log/slog.
//
// Configuration is controlled via environment variables:
//   - CODETECT_LOG_LEVEL: debug, info, warn, error (default: info)
//   - CODETECT_LOG_FORMAT: text, json (default: text)
//
// All logging goes to stderr to keep stdout clean for MCP protocol.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Log levels re-exported for convenience
const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// Config holds logging configuration
type Config struct {
	Level  slog.Level
	Format string    // "text" or "json"
	Output io.Writer // defaults to os.Stderr
	Source string    // component name for context
}

// DefaultConfig returns sensible defaults for the given source component.
func DefaultConfig(source string) Config {
	return Config{
		Level:  LevelInfo,
		Format: "text",
		Output: os.Stderr,
		Source: source,
	}
}

// LoadConfigFromEnv reads logging config from environment variables.
// Returns default configuration with any overrides from:
//   - CODETECT_LOG_LEVEL: debug, info, warn, error
//   - CODETECT_LOG_FORMAT: text, json
func LoadConfigFromEnv(source string) Config {
	cfg := DefaultConfig(source)

	if level := os.Getenv("CODETECT_LOG_LEVEL"); level != "" {
		switch strings.ToLower(level) {
		case "debug":
			cfg.Level = LevelDebug
		case "info":
			cfg.Level = LevelInfo
		case "warn", "warning":
			cfg.Level = LevelWarn
		case "error":
			cfg.Level = LevelError
		}
	}

	if format := os.Getenv("CODETECT_LOG_FORMAT"); format != "" {
		cfg.Format = strings.ToLower(format)
	}

	return cfg
}

// New creates a configured slog.Logger with the given configuration.
func New(cfg Config) *slog.Logger {
	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level: cfg.Level,
	}

	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(cfg.Output, opts)
	} else {
		handler = slog.NewTextHandler(cfg.Output, opts)
	}

	return slog.New(handler).With("source", cfg.Source)
}

// Default returns a logger with configuration loaded from environment.
// This is the recommended way to create a logger in CLI entry points.
func Default(source string) *slog.Logger {
	return New(LoadConfigFromEnv(source))
}

// Nop returns a logger that discards all output.
// Useful for tests or when logging should be suppressed.
func Nop() *slog.Logger {
	return slog.New(slog.NewTextHandler(nopWriter{}, nil))
}

// nopWriter implements io.Writer and discards all data.
type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) {
	return len(p), nil
}
