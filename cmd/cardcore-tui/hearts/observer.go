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
		handLines = append(handLines, fmt.Sprintf("Seat %d: %s", i, RenderHand(hand, -1, nil, nil)))
	}

	trick := RenderTrick(snap.Trick)

	scoreParts := make([]string, 0, len(snap.Scores))
	for i, s := range snap.Scores {
		rp := 0
		// Defensive bounds check: RoundPoints may be shorter than Scores in a
		// malformed snapshot (e.g., first round before points are populated, or a
		// server bug). Zero is a safe default.
		if i < len(snap.RoundPoints) {
			rp = snap.RoundPoints[i]
		}
		scoreParts = append(scoreParts, fmt.Sprintf("S%d=%d(+%d)", i, s, rp))
	}
	scores := "Scores: " + strings.Join(scoreParts, " ")

	turnLine := fmt.Sprintf("Seat %d's turn", snap.Turn)

	lines := make([]string, 0, 1+len(handLines)+3)
	lines = append(lines, header)
	lines = append(lines, handLines...)
	lines = append(lines, trick, scores, turnLine)
	return joinLines(lines)
}
