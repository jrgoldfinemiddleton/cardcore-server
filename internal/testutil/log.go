package testutil

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
)

// LogSnapshot logs a received snapshot envelope at Debug level.
// It extracts seq and phase so test runs can be traced with
// TEST_LOG_LEVEL=debug.
func LogSnapshot(t *testing.T, role string, data []byte) {
	t.Helper()
	var env struct {
		Seq   int    `json:"seq"`
		Phase string `json:"phase"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		slog.Debug("snapshot received", "role", role, "raw", string(data), "unmarshal_error", err)
		return
	}
	slog.Debug("snapshot received", "role", role, "seq", env.Seq, "phase", env.Phase)
}

// LogCommand logs a sent command envelope at Debug level.
func LogCommand(t *testing.T, role string, cmd client.Command) {
	t.Helper()
	slog.Debug("command sent",
		"role", role,
		"type", cmd.Type,
		"action_id", cmd.ActionID,
		"seq", cmd.Seq,
	)
}
