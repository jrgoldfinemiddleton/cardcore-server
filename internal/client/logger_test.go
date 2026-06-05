package client_test

import (
	"log/slog"
	"os"
	"strings"
	"testing"
)

// TestMain configures the default logger for test runs.
//
// By default only WARN and ERROR logs are printed to stderr.
// Set TEST_LOG_LEVEL=debug to reveal all structured log output
// while debugging a failing test.
func TestMain(m *testing.M) {
	level := slog.LevelWarn
	if v := os.Getenv("TEST_LOG_LEVEL"); v != "" {
		switch strings.ToLower(v) {
		case "debug":
			level = slog.LevelDebug
		case "info":
			level = slog.LevelInfo
		case "error":
			level = slog.LevelError
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	os.Exit(m.Run())
}
