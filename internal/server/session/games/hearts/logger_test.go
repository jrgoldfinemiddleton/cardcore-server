package heartssession

import (
	"io"
	"log/slog"
	"os"
	"testing"
)

// TestMain sets up the test environment and discards log output so that
// tests do not print to stderr.
//
// When debugging a failing test, set TEST_LOGS=1 to see the full
// structured log output.
func TestMain(m *testing.M) {
	var w = io.Discard
	if os.Getenv("TEST_LOGS") != "" {
		w = os.Stderr
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, nil)))
	os.Exit(m.Run())
}
