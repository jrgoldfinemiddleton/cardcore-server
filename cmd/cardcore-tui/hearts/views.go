package heartstui

import (
	"fmt"
	"strings"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// RenderTrick renders the cards played to the current trick, in play order,
// each labeled with its seat (e.g., "Seat 2: ♦K"). An empty trick returns a
// short placeholder: "(no cards played yet)".
func RenderTrick(trick []heartsclient.TrickEntry) string {
	if len(trick) == 0 {
		return "(no cards played yet)"
	}

	lines := make([]string, len(trick))
	for i, entry := range trick {
		lines[i] = fmt.Sprintf("Seat %d: %s", entry.Seat, CardLabel(entry.Card))
	}
	return joinLines(lines)
}

// RenderPassingView renders the passing phase view for a seated player.
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
) string {
	dir := formatPassDirection(snap.PassDirection)
	header := fmt.Sprintf("Round %d — %s", snap.RoundNumber, dir)
	hand := RenderHand(snap.Hand, cursor, selected, nil)

	remaining := max(3-len(selected), 0)

	var status string
	if len(selected) == 3 {
		status = "Press Enter to pass"
	} else {
		status = fmt.Sprintf("Select %d more card(s) to pass", remaining)
	}

	return joinLines([]string{header, hand, status})
}

// RenderPlayingView renders the playing phase view for a seated player.
//
// It shows the current trick on top, the player's hand (with illegal cards
// dimmed), and a status line indicating whose turn it is.
func RenderPlayingView(snap heartsclient.PlayerSnapshot, seat, cursor int) string {
	trick := RenderTrick(snap.Trick)
	hand := RenderHand(snap.Hand, cursor, nil, snap.LegalActions)

	var status string
	if snap.Turn == seat {
		status = "Your turn — select a card and press Enter"
	} else {
		status = fmt.Sprintf("Waiting for seat %d…", snap.Turn)
	}

	return joinLines([]string{trick, hand, status})
}

// RenderTrickCompleteView renders the view shown when a trick is complete.
//
// It displays the completed trick with seat labels and a status line.
// In Hearts, snap.Turn is the winner's seat (they lead the next trick).
// The winner is shown only when the trick has all 4 cards; otherwise a
// generic message is shown as a defensive fallback.
func RenderTrickCompleteView(snap heartsclient.PlayerSnapshot, seat int) string {
	trick := RenderTrick(snap.Trick)

	var status string
	if len(snap.Trick) == 4 {
		status = fmt.Sprintf("Trick complete — Seat %d won", snap.Turn)
	} else {
		status = "Trick complete"
	}

	return joinLines([]string{trick, status})
}

// RenderRoundCompleteView renders the round scores overlay.
//
// It shows the scores for each seat and the round points accumulated.
// The next snapshot (deal/passing) transitions naturally; no blocking.
func RenderRoundCompleteView(snap heartsclient.PlayerSnapshot) string {
	if len(snap.RoundPoints) != len(snap.Scores) {
		return "ERROR: Invalid snapshot (score data mismatch)"
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Round %d complete", snap.RoundNumber))

	for i := 0; i < len(snap.Scores); i++ {
		lines = append(lines,
			fmt.Sprintf("Seat %d: %d (+%d)", i, snap.Scores[i], snap.RoundPoints[i]))
	}

	return joinLines(lines)
}

// RenderGameOverView renders the final game-over screen.
//
// It shows the final scores for all seats and a prompt to exit.
func RenderGameOverView(snap heartsclient.PlayerSnapshot) string {
	var lines []string
	lines = append(lines, "Game Over")

	for i := 0; i < len(snap.Scores); i++ {
		lines = append(lines, fmt.Sprintf("Seat %d: %d", i, snap.Scores[i]))
	}

	lines = append(lines, "Press Enter to exit")
	return joinLines(lines)
}

// RenderDealView renders a brief overlay shown while the deck is being dealt.
func RenderDealView() string {
	return "Dealing..."
}

// joinLines joins lines with newlines for multi-line view output.
func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}
