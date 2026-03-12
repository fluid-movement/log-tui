// Package clog provides a thin slog-based debug logger for logviewer.
// Set LOG_DEBUG=1 to activate. Never writes to stdout.
package clog

import (
	"log/slog"
	"os"
	"path/filepath"
)

var logger *slog.Logger

func init() {
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// Init opens the log file and enables debug logging if LOG_DEBUG=1.
func Init() {
	if os.Getenv("LOG_DEBUG") != "1" {
		return
	}
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "logviewer-debug.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	logger = slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func Debug(msg string, args ...any) { logger.Debug(msg, args...) }
func Info(msg string, args ...any)  { logger.Info(msg, args...) }
func Error(msg string, args ...any) { logger.Error(msg, args...) }
