package heartscli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/games/hearts"
)

// Formatter implements game-specific snapshot formatting for Hearts.
type Formatter struct{}

// snapshotEnvelope captures the fields we need for compact formatting.
type snapshotEnvelope struct {
	// Seq is the snapshot sequence number.
	Seq int `json:"seq"`
	// Phase is the current game phase.
	Phase string `json:"phase"`
	// Turn is the seat index whose turn it is.
	Turn int `json:"turn"`
	// RoundNumber is the current round, 1-indexed.
	RoundNumber int `json:"round_number,omitempty"`
	// TrickNumber is the current trick within the round, 1-indexed.
	TrickNumber int `json:"trick_number,omitempty"`
	// Scores are the cumulative scores for each seat.
	Scores []int `json:"scores,omitempty"`
	// Winners lists the seat indexes tied for the lowest score; meaningful
	// only during game_over.
	Winners []int `json:"winners,omitempty"`
	// Hand is the player's own hand (player snapshot only).
	Hand []heartsclient.Card `json:"hand,omitempty"`
	// LegalActions are the cards the player may legally play.
	LegalActions []heartsclient.Card `json:"legal_actions,omitempty"`
	// Hands contains all seats' hands (observer snapshot only).
	Hands [][]heartsclient.Card `json:"hands,omitempty"`
	// Trick is the cards played so far in the current trick.
	Trick []heartsclient.TrickEntry `json:"trick,omitempty"`
	// TrickWinner is the seat index of the winner of the completed trick.
	// Only meaningful during the trick_complete phase; -1 in other phases.
	TrickWinner int `json:"trick_winner,omitempty"`
	// RoundPoints are the points taken this round, per seat.
	RoundPoints []int `json:"round_points,omitempty"`
}

// NewFormatter returns a Hearts snapshot formatter.
func NewFormatter() *Formatter {
	return &Formatter{}
}

// FormatSnapshot returns a compact one-line string for a Hearts snapshot.
// It handles both player and observer snapshots and produces
// deterministic output suitable for golden tests and diffing.
func (f *Formatter) FormatSnapshot(snapshot []byte) string {
	var env snapshotEnvelope
	if err := json.Unmarshal(snapshot, &env); err != nil {
		return fmt.Sprintf("malformed: %v", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "seq=%d phase=%s turn=%d", env.Seq, env.Phase, env.Turn)
	if env.RoundNumber > 0 {
		fmt.Fprintf(&b, " round=%d", env.RoundNumber)
	}
	if env.TrickNumber > 0 {
		fmt.Fprintf(&b, " trick_num=%d", env.TrickNumber)
	}

	if env.Phase == "game_over" {
		fmt.Fprintf(&b, " scores=%v", env.Scores)
		if decl := WinnerDeclaration(env.Winners, env.Scores); decl != "" {
			fmt.Fprintf(&b, " winner=%s", decl)
		}
		return b.String()
	}

	if env.Hand != nil {
		fmt.Fprintf(&b, " hand=%s", formatCards(env.Hand))
		if len(env.LegalActions) > 0 {
			fmt.Fprintf(&b, " legal=%s", formatCards(env.LegalActions))
		}
	}

	if env.Hands != nil {
		for i, h := range env.Hands {
			fmt.Fprintf(&b, " seat%d=%s", i, formatCards(h))
		}
	}

	if len(env.Trick) > 0 {
		fmt.Fprintf(&b, " trick=%s", formatTrick(env.Trick))
	}
	if env.Phase == "trick_complete" && env.TrickWinner >= 0 {
		fmt.Fprintf(&b, " trick_winner=%d", env.TrickWinner)
	}

	if len(env.RoundPoints) > 0 {
		fmt.Fprintf(&b, " round_points=%v", env.RoundPoints)
	}

	if len(env.Scores) > 0 {
		fmt.Fprintf(&b, " scores=%v", env.Scores)
	}

	return b.String()
}

// WinnerDeclaration returns the human-readable winner declaration for a
// game-over snapshot: "Seat Z wins" for a sole winner, or "Draw between seats
// X, Y" when multiple seats tie for the lowest score. When winners is empty
// (older servers omit the field), it derives the winners from scores by
// collecting every seat at the minimum score. It returns an empty string when
// no winner can be determined.
func WinnerDeclaration(winners, scores []int) string {
	if len(winners) == 0 {
		winners = derivedWinners(scores)
	}
	switch len(winners) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("Seat %d wins", winners[0])
	default:
		seats := make([]string, len(winners))
		for i, w := range winners {
			seats[i] = strconv.Itoa(w)
		}
		return "Draw between seats " + strings.Join(seats, ", ")
	}
}

// derivedWinners returns the seat indexes tied for the lowest score, or nil
// when scores is empty.
func derivedWinners(scores []int) []int {
	if len(scores) == 0 {
		return nil
	}
	min := scores[0]
	for _, s := range scores[1:] {
		if s < min {
			min = s
		}
	}
	var winners []int
	for i, s := range scores {
		if s == min {
			winners = append(winners, i)
		}
	}
	return winners
}

// formatCards formats a slice of cards into compact notation.
func formatCards(cards []heartsclient.Card) string {
	if len(cards) == 0 {
		return "[]"
	}
	parts := make([]string, len(cards))
	for i, c := range cards {
		parts[i] = formatCard(c)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// formatCard returns a compact representation like "2♣" or "A♠".
func formatCard(c heartsclient.Card) string {
	return heartsclient.RankSymbol(c.Rank) + heartsclient.SuitSymbol(c.Suit)
}

// formatTrick formats a trick as a list of played cards.
func formatTrick(trick []heartsclient.TrickEntry) string {
	if len(trick) == 0 {
		return "[]"
	}
	parts := make([]string, len(trick))
	for i, e := range trick {
		parts[i] = formatCard(e.Card)
	}
	return "[" + strings.Join(parts, " ") + "]"
}
