package main

import (
	"encoding/json"
	"fmt"
	"strings"

	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// snapshotEnvelope captures the fields we need for compact formatting.
type snapshotEnvelope struct {
	Seq          int                       `json:"seq"`
	Phase        string                    `json:"phase"`
	Turn         int                       `json:"turn"`
	RoundNumber  int                       `json:"round_number,omitempty"`
	TrickNumber  int                       `json:"trick_number,omitempty"`
	Scores       []int                     `json:"scores,omitempty"`
	Hand         []heartsclient.Card       `json:"hand,omitempty"`
	LegalActions []heartsclient.Card       `json:"legal_actions,omitempty"`
	Hands        [][]heartsclient.Card     `json:"hands,omitempty"`
	Trick        []heartsclient.TrickEntry `json:"trick,omitempty"`
	RoundPoints  []int                     `json:"round_points,omitempty"`
}

// formatSnapshot returns a compact one-line string for a snapshot.
// It handles both player and observer snapshots and produces
// deterministic output suitable for golden tests and diffing.
func formatSnapshot(snapshot []byte) string {
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

	if env.Phase == phaseGameOver {
		fmt.Fprintf(&b, " scores=%v", env.Scores)
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

	if len(env.RoundPoints) > 0 {
		fmt.Fprintf(&b, " round_points=%v", env.RoundPoints)
	}

	if len(env.Scores) > 0 {
		fmt.Fprintf(&b, " scores=%v", env.Scores)
	}

	return b.String()
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
	return formatRank(c.Rank) + formatSuit(c.Suit)
}

// formatRank maps a rank name to its compact symbol.
func formatRank(r string) string {
	switch r {
	case "two":
		return "2"
	case "three":
		return "3"
	case "four":
		return "4"
	case "five":
		return "5"
	case "six":
		return "6"
	case "seven":
		return "7"
	case "eight":
		return "8"
	case "nine":
		return "9"
	case "ten":
		return "10"
	case "jack":
		return "J"
	case "queen":
		return "Q"
	case "king":
		return "K"
	case "ace":
		return "A"
	default:
		return "?"
	}
}

// formatSuit maps a suit name to its compact symbol.
func formatSuit(s string) string {
	switch s {
	case "clubs":
		return "♣"
	case "spades":
		return "♠"
	case "hearts":
		return "♥"
	case "diamonds":
		return "♦"
	default:
		return "?"
	}
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
