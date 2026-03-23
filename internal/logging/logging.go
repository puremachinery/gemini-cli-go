// Package logging provides structured logging with opt-in output.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

const (
	envLogLevel  = "GEMINI_CLI_LOG_LEVEL"
	envLogFormat = "GEMINI_CLI_LOG_FORMAT"
)

var (
	logger *slog.Logger
	once   sync.Once
)

// Logger returns the configured logger (defaults to discard output).
func Logger() *slog.Logger {
	once.Do(initLogger)
	return logger
}

func initLogger() {
	level := strings.ToLower(strings.TrimSpace(os.Getenv(envLogLevel)))
	if level == "" {
		logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn}))
		return
	}
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	format := strings.ToLower(strings.TrimSpace(os.Getenv(envLogFormat)))
	if format == "json" {
		logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
		return
	}
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
