package heartstui

import (
	"fmt"
	"strings"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// RenderObserverView renders the full observer view showing all four seats'
// hands, the current trick, scores, and whose turn it is.
//
// It does not panic when Hands has fewer than 4 entries — it iterates over what
// is present.
func RenderObserverView(snap heartsclient.ObserverSnapshot) string {
	header := fmt.Sprintf("Round %d — Trick %d — %s",
		snap.RoundNumber, snap.TrickNumber, formatPassDirection(snap.PassDirection))

	handLines := make([]string, 0, len(snap.Hands))
	for i, hand := range snap.Hands {
		handLines = append(handLines,
			fmt.Sprintf("Seat %d: %s", i, RenderHand(hand, -1, nil, nil, false)))
	}

	trick := RenderTrick(snap.Trick)

	var scores string
	if len(snap.RoundPoints) != len(snap.Scores) {
		scores = "Scores: ERROR (data mismatch)"
	} else {
		scoreParts := make([]string, 0, len(snap.Scores))
		for i, s := range snap.Scores {
			scoreParts = append(scoreParts, fmt.Sprintf("S%d=%d(+%d)", i, s, snap.RoundPoints[i]))
		}
		scores = "Scores: " + strings.Join(scoreParts, " ")
	}

	var winnerLine string
	if snap.Phase == heartsclient.PhaseTrickComplete && snap.TrickWinner >= 0 {
		winnerLine = fmt.Sprintf("Trick complete — Seat %d won", snap.TrickWinner)
	}

	lines := make([]string, 0, 1+len(handLines)+3)
	lines = append(lines, header)
	lines = append(lines, handLines...)
	lines = append(lines, trick, scores)
	if winnerLine != "" {
		lines = append(lines, winnerLine)
	} else {
		lines = append(lines, fmt.Sprintf("Seat %d's turn", snap.Turn))
	}
	return joinLines(lines)
}
