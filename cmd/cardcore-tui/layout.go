package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// layoutStyle is the global style for the TUI layout.
//
// It defines the color scheme, border styles, and padding used across all
// layout components. This is the single source of truth for visual styling.
var layoutStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FAFAFA")).
	Background(lipgloss.Color("#1A1A2E"))

// headerStyle is the style for the top header bar.
//
// It shows the round number, phase, and score summary. The header is
// visually distinct from the main game area to provide context at a glance.
var headerStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#E94560")).
	Padding(0, 1).
	Width(80)

// footerStyle is the style for the bottom status bar.
//
// It shows error messages, connection status, and "AI thinking...".
// Error messages are rendered in red; normal status in default color.
var footerStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FAFAFA")).
	Background(lipgloss.Color("#16213E")).
	Padding(0, 1).
	Width(80)

// errorStyle is the style for error flash messages in the status bar.
//
// Error messages are rendered in bright red (#FF0000) to grab attention.
// They flash for 3 seconds, then clear.
var errorStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#FF0000")).
	Background(lipgloss.Color("#16213E")).
	Padding(0, 1).
	Width(80)

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
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		main,
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
	return headerStyle.Render(line)
}

// renderMain renders the central game area. It delegates to the game client
// once a snapshot has arrived; before then it shows a waiting message.
func (m *model) renderMain() string {
	if m.snapshot == nil {
		return layoutStyle.Render("Waiting for game state...")
	}
	return layoutStyle.Render(m.game.Render())
}

// renderFooter renders the status bar (error messages, connection status).
//
// The footer shows one of two things:
//
//  1. Error flash message (red, 3 seconds)
//  2. Connection status ("Connected" / "Disconnected")
//
// The error flash takes priority over connection status.
func (m *model) renderFooter() string {
	// Error flash takes priority.
	if m.errMsg != "" {
		return errorStyle.Render(m.errMsg)
	}

	// Connection status.
	if m.disconnected {
		return footerStyle.Render("Disconnected")
	}
	if m.statusMsg != "" {
		return footerStyle.Render(m.statusMsg)
	}

	// Default: show connected status.
	return footerStyle.Render("Connected")
}
