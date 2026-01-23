package logging

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("test-source")

	if cfg.Level != LevelInfo {
		t.Errorf("expected level INFO, got %v", cfg.Level)
	}
	if cfg.Format != "text" {
		t.Errorf("expected format text, got %s", cfg.Format)
	}
	if cfg.Output != os.Stderr {
		t.Errorf("expected output stderr")
	}
	if cfg.Source != "test-source" {
		t.Errorf("expected source test-source, got %s", cfg.Source)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	tests := []struct {
		name          string
		levelEnv      string
		formatEnv     string
		expectedLevel slog.Level
		expectedFmt   string
	}{
		{
			name:          "defaults",
			levelEnv:      "",
			formatEnv:     "",
			expectedLevel: LevelInfo,
			expectedFmt:   "text",
		},
		{
			name:          "debug level",
			levelEnv:      "debug",
			formatEnv:     "",
			expectedLevel: LevelDebug,
			expectedFmt:   "text",
		},
		{
			name:          "warn level",
			levelEnv:      "warn",
			formatEnv:     "",
			expectedLevel: LevelWarn,
			expectedFmt:   "text",
		},
		{
			name:          "warning level alias",
			levelEnv:      "warning",
			formatEnv:     "",
			expectedLevel: LevelWarn,
			expectedFmt:   "text",
		},
		{
			name:          "error level",
			levelEnv:      "ERROR",
			formatEnv:     "",
			expectedLevel: LevelError,
			expectedFmt:   "text",
		},
		{
			name:          "json format",
			levelEnv:      "",
			formatEnv:     "json",
			expectedLevel: LevelInfo,
			expectedFmt:   "json",
		},
		{
			name:          "JSON format uppercase",
			levelEnv:      "",
			formatEnv:     "JSON",
			expectedLevel: LevelInfo,
			expectedFmt:   "json",
		},
		{
			name:          "debug + json",
			levelEnv:      "debug",
			formatEnv:     "json",
			expectedLevel: LevelDebug,
			expectedFmt:   "json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env vars
			oldLevel := os.Getenv("CODETECT_LOG_LEVEL")
			oldFormat := os.Getenv("CODETECT_LOG_FORMAT")
			defer func() {
				os.Setenv("CODETECT_LOG_LEVEL", oldLevel)
				os.Setenv("CODETECT_LOG_FORMAT", oldFormat)
			}()

			if tt.levelEnv != "" {
				os.Setenv("CODETECT_LOG_LEVEL", tt.levelEnv)
			} else {
				os.Unsetenv("CODETECT_LOG_LEVEL")
			}
			if tt.formatEnv != "" {
				os.Setenv("CODETECT_LOG_FORMAT", tt.formatEnv)
			} else {
				os.Unsetenv("CODETECT_LOG_FORMAT")
			}

			cfg := LoadConfigFromEnv("test")

			if cfg.Level != tt.expectedLevel {
				t.Errorf("level: expected %v, got %v", tt.expectedLevel, cfg.Level)
			}
			if cfg.Format != tt.expectedFmt {
				t.Errorf("format: expected %s, got %s", tt.expectedFmt, cfg.Format)
			}
		})
	}
}

func TestNew(t *testing.T) {
	var buf bytes.Buffer

	cfg := Config{
		Level:  LevelInfo,
		Format: "text",
		Output: &buf,
		Source: "test-component",
	}

	logger := New(cfg)
	logger.Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("output should contain message: %s", output)
	}
	if !strings.Contains(output, "source=test-component") {
		t.Errorf("output should contain source: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("output should contain key=value: %s", output)
	}
}

func TestNewJSON(t *testing.T) {
	var buf bytes.Buffer

	cfg := Config{
		Level:  LevelInfo,
		Format: "json",
		Output: &buf,
		Source: "json-test",
	}

	logger := New(cfg)
	logger.Info("json test")

	output := buf.String()
	if !strings.Contains(output, `"msg":"json test"`) {
		t.Errorf("JSON output should contain msg field: %s", output)
	}
	if !strings.Contains(output, `"source":"json-test"`) {
		t.Errorf("JSON output should contain source field: %s", output)
	}
}

func TestLogLevelFiltering(t *testing.T) {
	var buf bytes.Buffer

	cfg := Config{
		Level:  LevelWarn,
		Format: "text",
		Output: &buf,
		Source: "filter-test",
	}

	logger := New(cfg)

	// These should be filtered out
	logger.Debug("debug message")
	logger.Info("info message")

	if strings.Contains(buf.String(), "debug message") {
		t.Error("debug message should be filtered")
	}
	if strings.Contains(buf.String(), "info message") {
		t.Error("info message should be filtered")
	}

	// These should appear
	logger.Warn("warn message")
	logger.Error("error message")

	if !strings.Contains(buf.String(), "warn message") {
		t.Error("warn message should appear")
	}
	if !strings.Contains(buf.String(), "error message") {
		t.Error("error message should appear")
	}
}

func TestNop(t *testing.T) {
	logger := Nop()

	// Should not panic
	logger.Info("this goes nowhere")
	logger.Error("neither does this")
	logger.With("key", "value").Debug("or this")
}

func TestDefault(t *testing.T) {
	// Save and restore env vars
	oldLevel := os.Getenv("CODETECT_LOG_LEVEL")
	oldFormat := os.Getenv("CODETECT_LOG_FORMAT")
	defer func() {
		os.Setenv("CODETECT_LOG_LEVEL", oldLevel)
		os.Setenv("CODETECT_LOG_FORMAT", oldFormat)
	}()

	os.Unsetenv("CODETECT_LOG_LEVEL")
	os.Unsetenv("CODETECT_LOG_FORMAT")

	logger := Default("default-test")
	if logger == nil {
		t.Error("Default should return a logger")
	}
}
