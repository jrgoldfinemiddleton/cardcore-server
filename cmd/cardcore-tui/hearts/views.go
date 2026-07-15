package heartstui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// RenderTrick renders the cards played to the current trick, in play order,
// each labeled with its seat (e.g., "Seat 2: ♦K"), using the provided theme
// for colors. An empty trick returns a short placeholder: "(no cards played
// yet)". When highlightSeat is non-negative and matches an entry's seat, that
// card is rendered with the CardWinner state.
func RenderTrick(trick []heartsclient.TrickEntry, highlightSeat int, theme Theme) string {
	if len(trick) == 0 {
		return "(no cards played yet)"
	}

	lines := make([]string, len(trick))
	for i, entry := range trick {
		state := CardNormal
		if entry.Seat == highlightSeat {
			state = CardWinner
		}
		lines[i] = fmt.Sprintf("Seat %d: %s", entry.Seat, RenderCard(entry.Card, state, theme))
	}
	return joinLines(lines)
}

// RenderPassingView renders the passing phase view for a seated player, using
// the provided theme for colors.
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
	seat, cursor int,
	selected []heartsclient.Card,
	inputDisabled bool,
	theme Theme,
) string {
	dir := formatPassDirection(snap.PassDirection)
	header := fmt.Sprintf("Round %d — %s", snap.RoundNumber, dir)
	hand := RenderHand(snap.Hand, cursor, selected, nil, inputDisabled, theme)

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

	return joinLines([]string{header, "", hand, "", status})
}

// RenderPlayingView renders the playing phase view for a seated player, using
// the provided theme for colors.
//
// It shows the current trick on top, the player's hand (with illegal cards
// dimmed), and a status line indicating whose turn it is.
func RenderPlayingView(
	snap heartsclient.PlayerSnapshot,
	seat, cursor int,
	inputDisabled bool,
	theme Theme,
) string {
	trick := RenderTrick(snap.Trick, -1, theme)
	hand := RenderHand(snap.Hand, cursor, nil, snap.LegalActions, inputDisabled, theme)

	var status string
	switch {
	case inputDisabled:
		status = "Waiting for other players…"
	case snap.Turn == seat:
		status = "Your turn — select a card and press Enter"
	default:
		status = fmt.Sprintf("Waiting for seat %d…", snap.Turn)
	}

	return joinLines([]string{trick, "", hand, "", status})
}

// RenderTrickCompleteView renders the view shown when a trick is complete,
// using the provided theme for colors.
//
// It displays the completed trick with seat labels and a status line inside a
// bordered box. The winner is provided by the server in snap.TrickWinner; the
// fallback generic message is used when the trick is not complete or the
// server did not provide a winner.
func RenderTrickCompleteView(snap heartsclient.PlayerSnapshot, seat int, theme Theme) string {
	trick := RenderTrick(snap.Trick, snap.TrickWinner, theme)

	var status string
	if len(snap.Trick) == 4 && snap.TrickWinner >= 0 {
		status = fmt.Sprintf("Trick complete — Seat %d won", snap.TrickWinner)
	} else {
		status = "Trick complete"
	}

	content := joinLines([]string{trick, status})
	return summaryBoxStyle(theme).Render(content)
}

// RenderRoundCompleteView renders the round scores overlay, using the provided
// theme for colors.
//
// It shows the scores for each seat and the round points accumulated inside a
// bordered box. The next snapshot (deal/passing) transitions naturally.
func RenderRoundCompleteView(snap heartsclient.PlayerSnapshot, theme Theme) string {
	if len(snap.RoundPoints) != len(snap.Scores) {
		return "ERROR: Invalid snapshot (score data mismatch)"
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Round %d complete", snap.RoundNumber))

	for i := 0; i < len(snap.Scores); i++ {
		lines = append(lines,
			fmt.Sprintf("Seat %d: %d (+%d)", i, snap.Scores[i], snap.RoundPoints[i]))
	}

	return summaryBoxStyle(theme).Render(joinLines(lines))
}

// RenderGameOverView renders the final game-over screen, using the provided
// theme for colors.
//
// It shows the final scores for all seats and a prompt to exit inside a
// bordered box.
func RenderGameOverView(snap heartsclient.PlayerSnapshot, theme Theme) string {
	var lines []string
	lines = append(lines, "Game Over")

	for i := 0; i < len(snap.Scores); i++ {
		lines = append(lines, fmt.Sprintf("Seat %d: %d", i, snap.Scores[i]))
	}

	lines = append(lines, "Press Enter to exit")
	return summaryBoxStyle(theme).Render(joinLines(lines))
}

// RenderPausedView renders the pause overlay, using the provided theme for
// colors.
func RenderPausedView(theme Theme) string {
	return summaryBoxStyle(theme).Render("Game paused — press P to resume")
}

// RenderDealView renders a brief overlay shown while the deck is being dealt.
func RenderDealView() string {
	return "Dealing..."
}

// summaryBoxStyle returns the bordered container style for pause/summary views,
// using the provided theme for the border color.
func summaryBoxStyle(theme Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.PanelBorder).
		Padding(0, 1).
		Width(78)
}

// joinLines joins lines with newlines for multi-line view output.
func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}
