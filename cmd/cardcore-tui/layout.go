package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderLayout renders the full screen layout.
//
// The layout is a vertical stack of bordered panels: header, main area, footer.
// Each section is rendered by a separate function for clarity.
func (m *model) renderLayout() string {
	header := m.renderHeader()
	main := m.renderMain()
	footer := m.renderFooter()

	// Join vertically with a blank separator between panels.
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

// renderHeader renders the top section (round, phase, and scores).
//
// The header shows the current round number, game phase, and an aligned score
// summary inside a bordered panel. Scores are shown in the default text color
// with a consistent background; any score within 26 points of 100 (the typical
// Hearts game-ending threshold) is highlighted in bold red to indicate danger.
func (m *model) renderHeader() string {
	// Never display "Round 0"; the server can send 0 before the first deal.
	displayRound := max(m.roundNumber, 1)
	roundPhase := fmt.Sprintf("Round %d | Phase: %s", displayRound, m.phase)
	roundPhaseStyled := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.Accent).
		Background(m.theme.Background).
		Render(roundPhase)
	if len(m.scores) == 0 {
		return headerPanelStyle(m.theme).Render(roundPhaseStyled)
	}

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
	bgFillStyle := lipgloss.NewStyle().
		Bold(false).
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
		Render(" | Scores: ")
	prefix := roundPhaseStyled + scoresLabel + scoreLine
	padWidth := 78 - lipgloss.Width(prefix)
	if padWidth < 0 {
		padWidth = 0
	}
	line := prefix + bgFillStyle.Render(strings.Repeat(" ", padWidth))
	return headerPanelStyle(m.theme).Render(line)
}

// renderMain renders the central game area. It delegates to the game client
// once a snapshot has arrived; before then it shows a waiting message.
func (m *model) renderMain() string {
	if m.snapshot == nil {
		return mainPanelStyle(m.theme).Render("Waiting for game state...")
	}
	return mainPanelStyle(m.theme).Render(m.game.Render())
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
		return errorPanelStyle(m.theme).Render(m.errMsg)
	}

	if m.timeoutDisabled {
		return errorPanelStyle(m.theme).Render("Timeout - AI playing")
	}

	if m.paused {
		return footerPanelStyle(m.theme).Render("Paused")
	}

	if m.statusMsg != "" {
		return footerPanelStyle(m.theme).Render(m.statusMsg)
	}

	// Countdown status (if any)
	if s := m.countdownStatus(); s != "" {
		return footerPanelStyle(m.theme).Render(s)
	}

	// Connection status.
	if m.disconnected {
		return footerPanelStyle(m.theme).Render("Disconnected")
	}

	// Default: show connected status.
	return footerPanelStyle(m.theme).Render("Connected")
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

// headerPanelStyle returns the bordered panel style for the top header bar.
func headerPanelStyle(theme Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.Accent).
		Background(theme.Background).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.PanelBorder).
		BorderBackground(theme.Background).
		Width(80)
}

// mainPanelStyle returns the bordered panel style for the central game area.
func mainPanelStyle(theme Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.Text).
		Background(theme.Background).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.PanelBorder).
		BorderBackground(theme.Background).
		Width(80)
}

// footerPanelStyle returns the bordered panel style for the bottom status bar.
func footerPanelStyle(theme Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.Text).
		Background(theme.FooterBg).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.PanelBorder).
		BorderBackground(theme.Background).
		Width(80)
}

// errorPanelStyle returns the bordered panel style for error flash messages.
func errorPanelStyle(theme Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.Error).
		Background(theme.FooterBg).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.PanelBorder).
		BorderBackground(theme.Background).
		Width(80)
}
