package main

import (
	"fmt"
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
)

// turnTickMsg is emitted on timer ticks to allow the UI to refresh the countdown.
type turnTickMsg struct{}

// turnTickInterval is the cadence for the on-screen turn countdown updates.
const turnTickInterval = 250 * time.Millisecond

// clientCutoffBeforeServer is how much earlier than the server-side auto-play
// deadline the client locks input and shows the timeout UI. This avoids racing
// the server's own timeout action.
const clientCutoffBeforeServer = time.Second

// handleTurnTick is the per-tick handler for the model. It updates the countdown
// state and disables input when appropriate. It returns a new tea.Cmd to keep
// ticking while a countdown is active.
func (m *model) handleTurnTick() tea.Cmd {
	// If no active countdown or feature disabled, do nothing.
	if m.turnDeadline.IsZero() || m.turnTimeoutMS <= 0 {
		return nil
	}

	remaining := time.Until(m.turnDeadline)
	if remaining <= 0 {
		// Server-side deadline has passed. The server should have auto-played;
		// keep input disabled until the next snapshot resets state.
		m.turnDeadline = time.Time{}
		m.timeoutDisabled = true
		m.game.SetInputDisabled(true)
		return nil
	}
	// Client-side cutoff: lock input and show the timeout UI one second before
	// the server-side deadline so the human cannot race the server auto-play.
	if remaining <= clientCutoffBeforeServer && !m.timeoutDisabled {
		m.timeoutDisabled = true
		m.game.SetInputDisabled(true)
		// Do not stop ticking; we still want a UI refresh every 250ms.
	}

	// Still waiting; re-arm the timer.
	return startTurnTick()
}

// countdownStatus renders a human-friendly countdown string for the footer.
func (m *model) countdownStatus() string {
	// Disabled feature or no deadline yields no status.
	if m.turnDeadline.IsZero() || m.turnTimeoutMS <= 0 {
		return ""
	}
	// Show the remaining time until the client-side cutoff (one second before
	// the server auto-play deadline), because that is the last moment the
	// human can still act.
	remaining := time.Until(m.turnDeadline) - clientCutoffBeforeServer
	secs := max(int(math.Ceil(remaining.Seconds())), 0)
	return fmt.Sprintf("Your turn (%ds)", secs)
}

// startTurnTick arms a periodic timer that emits turnTickMsg to drive the
// countdown UI.
func startTurnTick() tea.Cmd {
	return tea.Tick(turnTickInterval, func(_ time.Time) tea.Msg {
		return turnTickMsg{}
	})
}
