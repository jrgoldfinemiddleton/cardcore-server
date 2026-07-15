package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderLayout renders the full screen layout.
//
// The layout is a vertical stack: header, main area, footer.
// Each section is rendered by a separate function for clarity.
//
// The layout uses lipgloss to style each section. The header and footer
// are fixed-height; the main area expands to fill the remaining space.
func (m *model) renderLayout() string {
	header := m.renderHeader()
	main := m.renderMain()
	footer := m.renderFooter()

	// Join vertically with lipgloss.
	// The header is bold, the main area is the game state, and the footer
	// is the status bar.
	blank := layoutStyle(m.theme).Render("")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		blank,
		main,
		blank,
		footer,
	)
}

// renderHeader renders the top section (scores, phase, round info).
//
// The header shows the current round number, game phase, and a score summary.
// It is styled with bold red text to make it visually distinct.
func (m *model) renderHeader() string {
	var line string
	if len(m.scores) > 0 {
		scoreParts := make([]string, len(m.scores))
		for i, s := range m.scores {
			scoreParts[i] = fmt.Sprintf("S%d=%d", i, s)
		}
		line = fmt.Sprintf("Round %d | Phase: %s | Scores: %s",
			m.roundNumber, m.phase, strings.Join(scoreParts, " "))
	} else {
		line = fmt.Sprintf("Round %d | Phase: %s", m.roundNumber, m.phase)
	}
	return headerStyle(m.theme).Render(line)
}

// renderMain renders the central game area. It delegates to the game client
// once a snapshot has arrived; before then it shows a waiting message.
func (m *model) renderMain() string {
	if m.snapshot == nil {
		return layoutStyle(m.theme).Render("Waiting for game state...")
	}
	return layoutStyle(m.theme).Render(m.game.Render())
}

// renderFooter renders the status bar (error messages, connection status).
//
// The footer shows one of the following, in priority order:
//
//  1. Error message (red, bold) — may be a transient 3-second flash or a
//     persistent modal message that stays until Enter is pressed.
//  2. Persistent status message (e.g., the mapped WebSocket close reason).
//  3. Connection status ("Connected" / "Disconnected")
//
// The status message takes priority over the generic "Disconnected" label
// so that the user sees the reason the connection closed ("Game ended",
// "Server is shutting down", etc.) rather than a plain label.
func (m *model) renderFooter() string {
	if m.errMsg != "" {
		return errorStyle(m.theme).Render(m.errMsg)
	}

	if m.timeoutDisabled {
		return errorStyle(m.theme).Render("Timeout - AI playing")
	}

	if m.paused {
		return footerStyle(m.theme).Render("Paused")
	}

	if m.statusMsg != "" {
		return footerStyle(m.theme).Render(m.statusMsg)
	}

	// Countdown status (if any)
	if s := m.countdownStatus(); s != "" {
		return footerStyle(m.theme).Render(s)
	}

	// Connection status.
	if m.disconnected {
		return footerStyle(m.theme).Render("Disconnected")
	}

	// Default: show connected status.
	return footerStyle(m.theme).Render("Connected")
}

// layoutStyle returns the global style for the TUI layout given a theme.
//
// It defines the color scheme and width used across all layout components.
// This is the single source of truth for visual styling.
func layoutStyle(theme Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.Text).
		Background(theme.Background).
		Width(80)
}

// headerStyle returns the style for the top header bar given a theme.
//
// It shows the round number, phase, and score summary. The header is
// visually distinct from the main game area to provide context at a glance.
func headerStyle(theme Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Accent).
		Padding(0, 1).
		Width(80)
}

// footerStyle returns the style for the bottom status bar given a theme.
//
// It shows error messages, connection status, and "AI thinking...".
// Error messages are rendered in red; normal status in default color.
func footerStyle(theme Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.Text).
		Background(theme.FooterBg).
		Padding(0, 1).
		Width(80)
}

// errorStyle returns the style for error flash messages in the status bar
// given a theme.
//
// Error messages are rendered in bright red to grab attention.
// They flash for 3 seconds, then clear.
func errorStyle(theme Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Error).
		Background(theme.FooterBg).
		Padding(0, 1).
		Width(80)
}
