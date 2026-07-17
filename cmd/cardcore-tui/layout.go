package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// renderLayout renders the full screen layout.
//
// The layout is a vertical stack of bordered panels: header, main area, footer.
// Each section is rendered by a separate function for clarity. The terminal
// width from the model is threaded through so panels scale on resize; a zero
// width (before the first tea.WindowSizeMsg) defaults to 80 columns.
func (m *model) renderLayout() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	header := m.renderHeader(w)
	main := m.renderMain(w)
	footer := m.renderFooter(w)

	// Join vertically with a blank separator between panels.
	blank := layoutStyle(m.theme, w).Render("")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		blank,
		main,
		blank,
		footer,
	)
}

// renderHeader renders the top section (round, phase, and scores).
//
// The header spreads the three sections across the available width: the round
// is left-aligned, the phase is centered, and the scores are right-aligned.
// Scores are shown in the default text color with a consistent background; any
// score within 26 points of 100 (the typical Hearts game-ending threshold) is
// highlighted in bold red to indicate danger.
func (m *model) renderHeader(width int) string {
	// Never display "Round 0"; the server can send 0 before the first deal.
	displayRound := max(m.roundNumber, 1)

	innerWidth := width - 2

	left := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.Accent).
		Background(m.theme.Background).
		Render(fmt.Sprintf("Round %d", displayRound))

	center := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.Accent).
		Background(m.theme.Background).
		Render(fmt.Sprintf("Phase: %s", m.phase))

	var right string
	if len(m.scores) > 0 {
		const dangerThreshold = 100 - 26

		scoreStyle := lipgloss.NewStyle().
			Bold(false).
			Foreground(m.theme.Text).
			Background(m.theme.Background)
		dangerScoreStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(m.theme.Error).
			Background(m.theme.Background)
		sepStyle := lipgloss.NewStyle().
			Bold(false).
			Foreground(m.theme.Dimmed).
			Background(m.theme.Background)

		scoreParts := make([]string, len(m.scores))
		for i, s := range m.scores {
			label := fmt.Sprintf("S%d: %d", i, s)
			style := scoreStyle
			if s >= dangerThreshold {
				style = dangerScoreStyle
			}
			scoreParts[i] = style.Render(label)
		}

		scoreLine := strings.Join(scoreParts, sepStyle.Render(" • "))
		scoresLabel := lipgloss.NewStyle().
			Bold(true).
			Foreground(m.theme.Accent).
			Background(m.theme.Background).
			Render("Scores: ")
		right = scoresLabel + scoreLine
	}

	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)

	if rightWidth == 0 {
		// No scores yet: spread the round and phase across the full width.
		centerWidth := innerWidth - leftWidth
		if centerWidth < 0 {
			centerWidth = 0
		}
		centerBlock := lipgloss.NewStyle().
			Width(centerWidth).
			Align(lipgloss.Center).
			Background(m.theme.Background).
			Render(truncateText(center, centerWidth))
		line := lipgloss.JoinHorizontal(lipgloss.Top, left, centerBlock)
		return headerPanelStyle(m.theme, width).Render(line)
	}

	centerWidth := innerWidth - leftWidth - rightWidth
	if centerWidth < 0 {
		centerWidth = 0
		rightWidth = innerWidth - leftWidth
		if rightWidth < 0 {
			rightWidth = 0
		}
	}

	centerBlock := lipgloss.NewStyle().
		Width(centerWidth).
		Align(lipgloss.Center).
		Background(m.theme.Background).
		Render(truncateText(center, centerWidth))
	rightBlock := lipgloss.NewStyle().
		Width(rightWidth).
		Align(lipgloss.Right).
		Background(m.theme.Background).
		Render(truncateText(right, rightWidth))

	line := lipgloss.JoinHorizontal(lipgloss.Top, left, centerBlock, rightBlock)
	return headerPanelStyle(m.theme, width).Render(line)
}

// mainPanelHeight returns the number of rows allocated to the central game
// panel for the current terminal height. The remaining rows are reserved for
// the fixed header and footer panels plus the two blank separator lines.
func (m *model) mainPanelHeight() int {
	const (
		headerHeight    = 3 // 1 content row + 2 border rows
		footerHeight    = 3 // 1 content row + 2 border rows
		separatorHeight = 1
	)
	termHeight := m.height
	if termHeight == 0 {
		termHeight = 24
	}
	h := termHeight - headerHeight - footerHeight - 2*separatorHeight
	if h < 3 {
		return 3
	}
	return h
}

// renderMain renders the central game area. It delegates to the game client
// once a snapshot has arrived; before then it shows a waiting message.
func (m *model) renderMain(width int) string {
	mainHeight := m.mainPanelHeight()
	contentHeight := max(mainHeight-2, 0) // subtract top and bottom border rows
	innerWidth := max(width-2, 0)

	style := mainPanelStyle(m.theme, width).Height(mainHeight)

	if m.snapshot == nil {
		return style.Render("Waiting for game state...")
	}
	return style.Render(m.game.Render(innerWidth, contentHeight))
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
func (m *model) renderFooter(width int) string {
	if m.errMsg != "" {
		return errorPanelStyle(m.theme, width).Render(m.errMsg)
	}

	if m.timeoutDisabled {
		return errorPanelStyle(m.theme, width).Render("Timeout - AI playing")
	}

	if m.paused {
		return footerPanelStyle(m.theme, width).Render("Paused")
	}

	if m.statusMsg != "" {
		return footerPanelStyle(m.theme, width).Render(m.statusMsg)
	}

	// Countdown status (if any)
	if s := m.countdownStatus(); s != "" {
		return footerPanelStyle(m.theme, width).Render(s)
	}

	// Connection status.
	if m.disconnected {
		return footerPanelStyle(m.theme, width).Render("Disconnected")
	}

	// Default: show connected status.
	return footerPanelStyle(m.theme, width).Render("Connected")
}

// layoutStyle returns the global style for the TUI layout given a theme and
// terminal width.
//
// It defines the color scheme and width used across all layout components.
// This is the single source of truth for visual styling.
func layoutStyle(theme Theme, width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.Text).
		Background(theme.Background).
		Width(width)
}

// headerPanelStyle returns the bordered panel style for the top header bar,
// sized to the given terminal width and fixed at headerHeight rows.
func headerPanelStyle(theme Theme, width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.Accent).
		Background(theme.Background).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.PanelBorder).
		BorderBackground(theme.Background).
		Width(width).
		Height(3)
}

// mainPanelStyle returns the bordered panel style for the central game area,
// sized to the given terminal width.
func mainPanelStyle(theme Theme, width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.Text).
		Background(theme.Background).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.PanelBorder).
		BorderBackground(theme.Background).
		Width(width)
}

// footerPanelStyle returns the bordered panel style for the bottom status bar,
// sized to the given terminal width and fixed at footerHeight rows.
func footerPanelStyle(theme Theme, width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.Text).
		Background(theme.FooterBg).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.PanelBorder).
		BorderBackground(theme.Background).
		Width(width).
		Height(3)
}

// errorPanelStyle returns the bordered panel style for error flash messages,
// sized to the given terminal width and fixed at footerHeight rows.
func errorPanelStyle(theme Theme, width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Error).
		Background(theme.FooterBg).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.PanelBorder).
		BorderBackground(theme.Background).
		Width(width).
		Height(3)
}

// truncateText returns s truncated to at most maxWidth visible columns.
func truncateText(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	return ansi.Truncate(s, maxWidth, "")
}
