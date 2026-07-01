package main

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
)

// flashTimeoutMsg is sent when the error flash timer expires.
type flashTimeoutMsg struct{}

// setErrorFlash sets an error message and starts a 3-second flash timer.
//
// This is used for transient local validation errors like "Not your turn"
// or "Select exactly 3 cards" — the user sees the error, then it disappears
// automatically. It is NOT used for server error modals (those are persistent
// until Enter is pressed).
func (m *model) setErrorFlash(msg string) tea.Cmd {
	m.errMsg = msg
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return flashTimeoutMsg{}
	})
}

// clearErrorFlash clears the error message immediately.
func (m *model) clearErrorFlash() {
	m.errMsg = ""
}

// handleWSError processes a server error message and sets the appropriate
// recovery behavior.
func (m *model) handleWSError(msg wsErrorMsg) {
	action := client.ClassifyError(msg.code)

	switch action {
	case client.RecoveryResync:
		// stale_seq — silent resync, no UI. Reset submitted so the human
		// can retry if the next snapshot is delayed.
		m.game.ResetSubmitted()

	case client.RecoveryWait:
		// out_of_turn or wrong_phase — recoverable but unexpected.
		// Show persistent continue modal (no auto-clear timer).
		m.modalContinue = true
		m.errMsg = errorMessageForCode(msg.code, msg.message)
		m.game.ResetSubmitted()

	case client.RecoveryTerminal, client.RecoveryRetryDifferent, client.RecoveryFixAndRetry:
		// Fatal errors (illegal_move, game_over, malformed_message).
		// game_over is handled by the phase gate; for other fatal codes
		// show a persistent fatal modal.
		if msg.code == client.ErrGameOver {
			// Phase gate in handleKeyPress handles exit; no flash needed.
			return
		}
		m.modalFatal = true
		m.errMsg = errorMessageForCode(msg.code, msg.message)

	default:
		// Unknown code — treat as fatal.
		m.modalFatal = true
		m.errMsg = errorMessageForCode(msg.code, msg.message)
	}
}

// handleWSClose handles a WebSocket close message.
func (m *model) handleWSClose(msg wsCloseMsg) tea.Cmd {
	m.disconnected = true

	if msg.code == 1011 {
		// Fatal close — show modal, do not auto-quit.
		m.modalFatal = true
		m.statusMsg = closeMessageForCode(msg.code)
		if m.conn != nil {
			_ = m.conn.Close()
		}
		return nil
	}

	m.statusMsg = closeMessageForCode(msg.code)
	if m.conn != nil {
		_ = m.conn.Close()
	}
	return tea.Quit
}

// errorMessageForCode returns a human-readable error message for a server
// error code.
func errorMessageForCode(code, serverMsg string) string {
	switch code {
	case "out_of_turn":
		return "AI played for you — your turn has passed. Press Enter to continue."
	case "illegal_move":
		return "Bug: server rejected a valid card. Press Enter to exit."
	case "wrong_phase":
		return "AI played for you — phase has changed. Press Enter to continue."
	case "stale_seq":
		// Auto-resync — no flash message needed.
		return ""
	case "game_over":
		return "Game over. Press Enter to exit."
	case "malformed_message":
		return "Internal error: invalid command format. Press Enter to exit."
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
		return "Internal server error. Press Enter to exit."
	default:
		return fmt.Sprintf("Connection closed (code %d)", code)
	}
}
