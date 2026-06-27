package main

import (
	"fmt"
	"time"

	"charm.land/bubbletea/v2"
)

// flashTimeoutMsg is sent when the error flash timer expires.
type flashTimeoutMsg struct{}

// setErrorFlash sets an error message and starts a 3-second flash timer.
//
// The flash timer sends a flashTimeoutMsg after 3 seconds, which clears the
// error message. This is used for transient errors like "Not your turn" or
// "Illegal move" — the user sees the error, then it disappears automatically.
func (m *model) setErrorFlash(msg string) tea.Cmd {
	m.errMsg = msg
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return flashTimeoutMsg{}
	})
}

// clearErrorFlash clears the error message immediately.
//
// This is called when the flash timer expires (flashTimeoutMsg) or when
// a new snapshot arrives that supersedes the error.
func (m *model) clearErrorFlash() {
	m.errMsg = ""
}

// handleWSError processes a server error message and sets the appropriate
// flash message.
//
// Some error codes (like "stale_seq") are handled silently by the client
// engine (Conn.ReadSnapshot auto-resyncs). Others produce a visible flash.
func (m *model) handleWSError(msg wsErrorMsg) tea.Cmd {
	errText := errorMessageForCode(msg.code, msg.message)
	if errText == "" {
		return nil
	}
	return m.setErrorFlash(errText)
}

// handleWSClose handles a WebSocket close message by cleaning up the
// connection and returning a quit command.
//
// The close message is displayed in the footer before quitting. The model
// does not enter a modal state — it exits immediately with a message.
func (m *model) handleWSClose(msg wsCloseMsg) tea.Cmd {
	m.disconnected = true
	m.statusMsg = closeMessageForCode(msg.code)
	if m.conn != nil {
		_ = m.conn.Close()
	}
	return tea.Quit
}

// errorMessageForCode returns a human-readable error message for a server
// error code.
//
// The server sends error codes like "out_of_turn", "illegal_move", etc.
// This function maps them to user-friendly messages displayed in the status
// bar. If the code is unknown, the server-provided message is used.
func errorMessageForCode(code, serverMsg string) string {
	switch code {
	case "out_of_turn":
		return "Not your turn"
	case "illegal_move":
		if serverMsg != "" {
			return serverMsg
		}
		return "Illegal move"
	case "wrong_phase":
		return "Wrong phase"
	case "stale_seq":
		// Auto-resync — no flash message needed.
		return ""
	default:
		if serverMsg != "" {
			return serverMsg
		}
		return fmt.Sprintf("Error: %s", code)
	}
}

// closeMessageForCode returns a human-readable message for a WebSocket close
// code.
//
// Close codes are defined by RFC 6455:
//
//	1000 — Normal closure (game ended)
//	1001 — Server shutdown
//	1011 — Internal server error
func closeMessageForCode(code int) string {
	switch code {
	case 1000:
		return "Game ended"
	case 1001:
		return "Server is shutting down"
	case 1011:
		return "Internal server error"
	default:
		return fmt.Sprintf("Connection closed (code %d)", code)
	}
}
