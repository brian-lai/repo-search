# Structured Logging Implementation

**Date:** 2026-01-15
**Status:** Complete
**Branch:** para/structured-logging

## Objective

Replace ad-hoc `fmt.Fprintf` and `log.*` calls with Go's `log/slog` structured logging to enable:
- Consistent log levels (DEBUG, INFO, WARN, ERROR)
- Structured fields for machine-readable logs
- Configurable output (text for development, JSON for production)
- Centralized log level control via environment variables

## Current State

- **150+ `fmt.Fprintf/Printf` calls** across 11 files (primary logging method)
- **19 `log.*` calls** across 4 files
- **No log levels** - all output treated the same
- **Manual prefixes** like `[codetect-index]` added inconsistently
- **Stderr is correctly used** - stdout reserved for MCP protocol and data output

## Approach

### Phase 1: Logging Infrastructure

Create `internal/logging/` package with:

```go
// internal/logging/logging.go
package logging

import (
    "log/slog"
    "os"
    "strings"
)

// Log levels
const (
    LevelDebug = slog.LevelDebug
    LevelInfo  = slog.LevelInfo
    LevelWarn  = slog.LevelWarn
    LevelError = slog.LevelError
)

// Config holds logging configuration
type Config struct {
    Level   slog.Level
    Format  string // "text" or "json"
    Output  *os.File
    Source  string // component name for context
}

// DefaultConfig returns sensible defaults
func DefaultConfig(source string) Config {
    return Config{
        Level:  LevelInfo,
        Format: "text",
        Output: os.Stderr,
        Source: source,
    }
}

// LoadConfigFromEnv reads logging config from environment
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

// New creates a configured slog.Logger
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

// Default returns a logger with default configuration
func Default(source string) *slog.Logger {
    return New(LoadConfigFromEnv(source))
}
```

### Phase 2: CLI Entry Points

Update each CLI to initialize logging:

```go
// cmd/codetect-index/main.go
func main() {
    logger := logging.Default("codetect-index")

    // Replace: fmt.Fprintf(os.Stderr, "[codetect-index] indexing %s\n", path)
    // With:    logger.Info("indexing", "path", path)

    // Replace: fmt.Fprintf(os.Stderr, "error: %v\n", err)
    // With:    logger.Error("indexing failed", "error", err)
}
```

**Files to update:**
- `cmd/codetect/main.go` - MCP server
- `cmd/codetect-index/main.go` - Indexer (heaviest user)
- `cmd/codetect-daemon/main.go` - Daemon
- `cmd/codetect-eval/main.go` - Eval runner
- `cmd/migrate-to-postgres/main.go` - Migration tool

### Phase 3: Internal Packages

Pass logger to internal packages via context or struct fields:

```go
// Option 1: Struct field (preferred for long-lived components)
type Index struct {
    // ... existing fields ...
    logger *slog.Logger
}

func NewIndexWithConfig(cfg db.Config, repoRoot string, logger *slog.Logger) (*Index, error) {
    // ...
}

// Option 2: Context (for request-scoped logging)
func (idx *Index) FindSymbol(ctx context.Context, name string) ([]Symbol, error) {
    logger := logging.FromContext(ctx)
    logger.Debug("finding symbol", "name", name)
    // ...
}
```

**Packages to update:**
- `internal/mcp/server.go` - Already has logger, migrate to slog
- `internal/daemon/daemon.go` - Has file logging, extend with slog
- `internal/tools/*.go` - Tool handlers
- `internal/embedding/provider.go` - Has warning output
- `internal/search/symbols/index.go` - Optional debug logging

### Phase 4: Log Level Guidelines

| Level | When to Use | Examples |
|-------|-------------|----------|
| DEBUG | Detailed execution flow | "parsing ctags output", "executing query", "chunk 5/100" |
| INFO | Normal operations | "indexing started", "server listening", "migration complete" |
| WARN | Recoverable issues | "ctags not found, symbol search disabled", "embedding skipped" |
| ERROR | Failures requiring attention | "database connection failed", "tool execution error" |

## Files to Modify

| File | Changes | Priority |
|------|---------|----------|
| `internal/logging/logging.go` | NEW - logging package | P0 |
| `cmd/codetect-index/main.go` | Replace 40+ log calls | P0 |
| `cmd/codetect/main.go` | Replace log setup | P0 |
| `internal/mcp/server.go` | Migrate to slog | P1 |
| `cmd/codetect-daemon/main.go` | Replace 15+ log calls | P1 |
| `internal/daemon/daemon.go` | Migrate to slog | P1 |
| `cmd/codetect-eval/main.go` | Replace 30+ log calls | P2 |
| `cmd/migrate-to-postgres/main.go` | Replace log calls | P2 |
| `internal/tools/*.go` | Add structured logging | P2 |
| `internal/embedding/provider.go` | Replace warnings | P2 |

## Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `CODETECT_LOG_LEVEL` | Minimum log level (debug/info/warn/error) | `info` |
| `CODETECT_LOG_FORMAT` | Output format (text/json) | `text` |

## Constraints

1. **stdout must stay clean** - MCP protocol uses stdout for JSON-RPC
2. **All logging to stderr** - Already enforced, must maintain
3. **Progress output** - Some commands use `\r` for progress bars; handle gracefully
4. **Backward compatibility** - Existing scripts may parse stderr; text format preserves readability

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Breaking MCP protocol | Test with Claude Code after changes |
| Performance regression | DEBUG level compiled out or disabled by default |
| Log spam in production | Default to INFO level |
| Progress bars broken | Keep `fmt.Fprintf` for progress-specific output |

## Success Criteria

- [x] All `fmt.Fprintf` logging calls migrated to slog
- [x] All `log.*` calls migrated to slog
- [x] Log level configurable via `CODETECT_LOG_LEVEL`
- [x] JSON output available via `CODETECT_LOG_FORMAT=json`
- [x] MCP server still works correctly with Claude Code
- [x] Tests pass
- [x] No performance regression for normal operations

## Review Checklist

- [x] Logging package has tests
- [x] All CLIs initialize logger consistently
- [x] Structured fields used (not string interpolation)
- [x] Error logs include relevant context (file path, operation, etc.)
- [x] DEBUG logs don't fire in production (default INFO)
- [x] Documentation updated with new env vars (added to CLI help)
