package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"

	"charm.land/bubbletea/v2"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
)

// WSReader is the interface between the WebSocket reader goroutine and the
// WebSocket connection. It allows unit testing without a real WebSocket server.
type WSReader interface {
	// ReadSnapshot reads a single snapshot from the WebSocket connection.
	// It returns the raw JSON snapshot or an error if the connection closes.
	ReadSnapshot(ctx context.Context) (json.RawMessage, error)
}

// *client.Conn satisfies WSReader (ReadSnapshot is already defined on Conn).

// wsSnapshotMsg carries a fresh snapshot (already filtered by Conn.ReadSnapshot).
//
// The snapshot is raw JSON — the model decodes it based on player/observer mode.
// This avoids the wsbridge needing to know about game-specific DTOs.
type wsSnapshotMsg struct {
	// raw is the raw JSON snapshot received from the server.
	// It is decoded by the model based on the current phase and mode.
	raw json.RawMessage
}

// wsErrorMsg carries a server error message.
//
// The code is one of: stale_seq, out_of_turn, illegal_move, wrong_phase, game_over.
// The message is the human-readable text from the server.
type wsErrorMsg struct {
	// code is the server error code (e.g., "out_of_turn", "illegal_move").
	code string
	// message is the human-readable error text from the server.
	message string
}

// wsCloseMsg carries a WebSocket close code.
//
// Close codes: 1000=normal, 1001=shutdown, 1011=internal error.
type wsCloseMsg struct {
	// code is the WebSocket close code per RFC 6455.
	code int
}

// startWSReader reads snapshots from the WebSocket and sends them to the
// Bubble Tea program via program.Send().
//
// This function runs in a dedicated goroutine. It reads from the WebSocket
// in a loop, and sends typed tea.Msg values into the Bubble Tea model via
// program.Send().
//
// Key design: Conn.ReadSnapshot() owns ALL maxSeenSeq filtering (ADR-011).
// The wsbridge trusts it — no re-filtering, no seq comparison.
// Conn.ReadSnapshot() also returns server errors as *client.ErrorMessage,
// which the wsbridge routes as wsErrorMsg for model-level classification.
// Everything else is sent as wsSnapshotMsg.
//
// The goroutine exits when the WebSocket closes or the context is cancelled.
func startWSReader(ctx context.Context, r WSReader, p *tea.Program) {
	logger := slog.With("component", "wsbridge")

	for {
		raw, err := r.ReadSnapshot(ctx)
		if err != nil {
			var errMsg *client.ErrorMessage
			if errors.As(err, &errMsg) {
				logger.Warn("server error",
					"error_code", errMsg.ErrorCode,
					"message", errMsg.Message)
				p.Send(wsErrorMsg{code: errMsg.ErrorCode, message: errMsg.Message})
				continue
			}

			var closeErr *client.ConnectionClosedError
			if errors.As(err, &closeErr) {
				logger.Info("websocket closed",
					"code", closeErr.Code,
					"reason", closeErr.Reason)
				p.Send(wsCloseMsg{code: closeErr.Code})
				return
			}

			// EOF from the server without a close frame is a normal end of
			// stream after the session finishes; treat it like a graceful
			// closure instead of an internal server error.
			if errors.Is(err, io.EOF) {
				logger.Info("websocket closed", "code", 1000, "reason", "EOF")
				p.Send(wsCloseMsg{code: 1000})
				return
			}

			logger.Error("websocket read error", "error", err)
			p.Send(wsCloseMsg{code: 1011})
			return
		}

		// Fresh snapshot.
		p.Send(wsSnapshotMsg{raw: raw})
	}
}
