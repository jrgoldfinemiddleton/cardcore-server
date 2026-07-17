package heartstui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// RenderTrick renders the cards played to the current trick, in play order.
// Each card is preceded by a styled seat label on its own line (e.g.,
// "Seat 2 (You)" followed by the bordered card). An empty trick returns a short
// placeholder: "(no cards played yet)". When highlightSeat is non-negative and
// matches an entry's seat, that card is rendered with the CardWinner state. The
// viewer's seat is marked with "(You)" when viewerSeat is non-negative and
// matches the entry.
func RenderTrick(
	trick []heartsclient.TrickEntry,
	viewerSeat, highlightSeat int,
	theme Theme,
) string {
	if len(trick) == 0 {
		return lipgloss.NewStyle().
			Foreground(theme.Text).
			Background(theme.Background).
			Render("(no cards played yet)")
	}

	lines := make([]string, len(trick))
	for i, entry := range trick {
		state := CardNormal
		if entry.Seat == highlightSeat {
			state = CardWinner
		}
		lines[i] = joinLines([]string{
			seatLabel(entry.Seat, viewerSeat, theme),
			RenderCard(entry.Card, state, theme),
		})
	}
	return joinLines(lines)
}

// RenderPassingView renders the passing phase view for a seated player, using
// the provided theme for colors and scaling the hand to the given terminal
// width.
//
// It shows a header with the round number and pass direction, the player's
// hand (with cursor and selected cards highlighted), and a status line
// indicating how many more cards need to be selected or that Enter can be
// pressed to pass.
//
// Contract: selected must contain at most 3 cards. The caller
// (Client.toggleSelected) enforces this. If >3 cards are passed, the status
// line will show a negative count, which is a debugging signal that the
// caller violated the contract.
func RenderPassingView(
	snap heartsclient.PlayerSnapshot,
	cursor int,
	selected []heartsclient.Card,
	inputDisabled bool,
	theme Theme,
	width, height int,
) string {
	dir := formatPassDirection(snap.PassDirection)
	textStyle := lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Background)
	header := textStyle.Render(fmt.Sprintf("Round %d — %s", snap.RoundNumber, dir))
	hand := RenderHand(snap.Hand, cursor, selected, nil, inputDisabled, theme, width)

	remaining := max(3-len(selected), 0)

	var status string
	switch {
	case inputDisabled:
		status = "Waiting for other players…"
	case len(selected) == 3:
		status = "Press Enter to pass"
	default:
		status = fmt.Sprintf("Select %d more card(s) to pass", remaining)
	}

	content := joinLines([]string{header, "", hand, "", textStyle.Render(status)})
	return placeContent(content, width, height, lipgloss.Bottom, theme)
}

// RenderPlayingView renders the playing phase view for a seated player, using
// the provided theme for colors and scaling the hand to the given terminal
// width.
//
// It shows the current trick on top, the player's hand (with illegal cards
// dimmed), and a status line indicating whose turn it is.
func RenderPlayingView(
	snap heartsclient.PlayerSnapshot,
	seat, cursor int,
	inputDisabled bool,
	theme Theme,
	width, height int,
) string {
	textStyle := lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Background)
	trick := RenderTrick(snap.Trick, seat, -1, theme)
	hand := RenderHand(snap.Hand, cursor, nil, snap.LegalActions, inputDisabled, theme, width)

	var status string
	switch {
	case inputDisabled:
		status = "Waiting for other players…"
	case snap.Turn == seat:
		status = "Your turn — select a card and press Enter"
	default:
		status = fmt.Sprintf("Waiting for seat %d…", snap.Turn)
	}

	content := joinLines([]string{trick, "", hand, "", textStyle.Render(status)})
	return placeContent(content, width, height, lipgloss.Bottom, theme)
}

// RenderTrickCompleteView renders the view shown when a trick is complete,
// using the provided theme for colors and sizing the summary box to the given
// terminal width.
//
// It displays the completed trick with seat labels and a status line inside a
// bordered box. The winner is provided by the server in snap.TrickWinner; the
// fallback generic message is used when the trick is not complete or the
// server did not provide a winner.
func RenderTrickCompleteView(
	snap heartsclient.PlayerSnapshot,
	seat int,
	theme Theme,
	width, height int,
) string {
	textStyle := lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Background)
	trick := RenderTrick(snap.Trick, seat, snap.TrickWinner, theme)

	var status string
	if len(snap.Trick) == 4 && snap.TrickWinner >= 0 {
		status = fmt.Sprintf("Trick complete — Seat %d won", snap.TrickWinner)
	} else {
		status = "Trick complete"
	}

	content := joinLines([]string{trick, textStyle.Render(status)})
	boxed := summaryBoxStyle(theme, width).Render(content)
	return placeContent(boxed, width, height, lipgloss.Bottom, theme)
}

// RenderRoundCompleteView renders the round scores overlay, using the provided
// theme for colors and sizing the summary box to the given terminal width.
//
// It shows the scores for each seat and the round points accumulated inside a
// bordered box. The viewer's seat is labeled with "(You)". The next snapshot
// (deal/passing) transitions naturally.
func RenderRoundCompleteView(
	snap heartsclient.PlayerSnapshot,
	seat int,
	theme Theme,
	width, height int,
) string {
	if len(snap.RoundPoints) != len(snap.Scores) {
		return "ERROR: Invalid snapshot (score data mismatch)"
	}

	textStyle := lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Background)
	var lines []string
	lines = append(lines, textStyle.Render(fmt.Sprintf("Round %d complete", snap.RoundNumber)))

	for i := 0; i < len(snap.Scores); i++ {
		label := seatLabel(i, seat, theme)
		rest := textStyle.Render(fmt.Sprintf(": %d (+%d)", snap.Scores[i], snap.RoundPoints[i]))
		lines = append(lines, label+rest)
	}

	boxed := summaryBoxStyle(theme, width).Render(joinLines(lines))
	return placeContent(boxed, width, height, lipgloss.Center, theme)
}

// RenderGameOverView renders the final game-over screen, using the provided
// theme for colors and sizing the summary box to the given terminal width.
//
// It shows the final scores for all seats and a prompt to exit inside a
// bordered box. The viewer's seat is labeled with "(You)".
func RenderGameOverView(
	snap heartsclient.PlayerSnapshot,
	seat int,
	theme Theme,
	width, height int,
) string {
	textStyle := lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Background)
	var lines []string
	lines = append(lines, textStyle.Render("Game Over"))

	for i := 0; i < len(snap.Scores); i++ {
		label := seatLabel(i, seat, theme)
		rest := textStyle.Render(fmt.Sprintf(": %d", snap.Scores[i]))
		lines = append(lines, label+rest)
	}

	lines = append(lines, textStyle.Render("Press Enter to exit"))
	boxed := summaryBoxStyle(theme, width).Render(joinLines(lines))
	return placeContent(boxed, width, height, lipgloss.Center, theme)
}

// RenderPausedView renders the pause overlay, using the provided theme for
// colors and sizing the summary box to the given terminal width.
func RenderPausedView(theme Theme, width, height int) string {
	boxed := summaryBoxStyle(theme, width).Render("Game paused — press P to resume")
	return placeContent(boxed, width, height, lipgloss.Center, theme)
}

// RenderDealView renders a brief overlay shown while the deck is being dealt.
func RenderDealView(theme Theme, width, height int) string {
	textStyle := lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Background)
	return placeContent(textStyle.Render("Dealing..."), width, height, lipgloss.Center, theme)
}

// summaryBoxStyle returns the bordered container style for pause/summary views,
// using the provided theme for the border and background colors and sized to
// the given terminal width.
func summaryBoxStyle(theme Theme, width int) lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.PanelBorder).
		BorderBackground(theme.Background).
		Foreground(theme.Text).
		Background(theme.Background).
		Padding(0, 1).
		Width(width)
}

// joinLines joins lines with newlines for multi-line view output.
func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}

// placeContent fills a width×height box with the theme background and places
// content inside it at the requested vertical position.
func placeContent(content string, width, height int, vPos lipgloss.Position, theme Theme) string {
	bgStyle := lipgloss.NewStyle().Background(theme.Background)
	content = clipLines(content, height, vPos)
	return lipgloss.Place(
		width, height,
		lipgloss.Left, vPos,
		content,
		lipgloss.WithWhitespaceStyle(bgStyle),
	)
}

// clipLines returns at most height lines of s, choosing the slice based on the
// requested vertical position so the content that matters stays visible.
func clipLines(s string, height int, vPos lipgloss.Position) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= height {
		return s
	}
	switch vPos {
	case lipgloss.Top:
		return strings.Join(lines[:height], "\n")
	case lipgloss.Bottom:
		return strings.Join(lines[len(lines)-height:], "\n")
	default:
		// Center alignment: keep the middle portion.
		start := (len(lines) - height) / 2
		return strings.Join(lines[start:start+height], "\n")
	}
}

// seatLabel returns a styled "Seat N" label, appending "(You)" when the seat
// belongs to the viewer.
func seatLabel(seat, viewerSeat int, theme Theme) string {
	label := fmt.Sprintf("Seat %d", seat)
	if seat == viewerSeat {
		label += " (You)"
	}
	return lipgloss.NewStyle().
		Foreground(theme.Text).
		Background(theme.Background).
		Bold(true).
		Render(label)
}
