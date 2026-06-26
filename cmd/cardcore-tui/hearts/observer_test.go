package heartstui

import (
	"strings"
	"testing"

	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// TestRenderObserverViewFourHands verifies that a 4-hand ObserverSnapshot
// includes all four "Seat N" labels and a scores summary.
//
// Round 2, Trick 13 (final trick), pass right. Scores sum to 26
// (from round 1). RoundPoints sum to 11 (e.g., 11 hearts taken). One card
// per hand (final trick of the round).
func TestRenderObserverViewFourHands(t *testing.T) {
	snap := heartsclient.ObserverSnapshot{
		RoundNumber:   2,
		TrickNumber:   13,
		PassDirection: "right",
		Turn:          0,
		// Renderer contract: Hands is expected to contain exactly 4
		// entries for a 4-player game. Fewer entries render without error but
		// represent an invalid game state that should be caught upstream.
		Hands: [][]heartsclient.Card{
			{{Rank: "queen", Suit: "spades"}},
			{{Rank: "two", Suit: "hearts"}},
			{{Rank: "three", Suit: "diamonds"}},
			{{Rank: "four", Suit: "hearts"}},
		},
		// Renderer contract: Trick is expected to be non-nil. A nil
		// trick renders the same as an empty trick (placeholder text), but nil
		// should be avoided by the caller.
		Trick:       []heartsclient.TrickEntry{},
		Scores:      []int{13, 0, 13, 0},
		RoundPoints: []int{11, 0, 0, 0},
	}

	got := RenderObserverView(snap)

	for i := 0; i < 4; i++ {
		label := "Seat " + string(rune('0'+i))
		if !strings.Contains(got, label) {
			t.Errorf("RenderObserverView = %q, want to contain %q", got, label)
		}
	}

	if !strings.Contains(got, "Scores:") {
		t.Errorf("RenderObserverView = %q, want to contain %q", got, "Scores:")
	}
	if !strings.Contains(got, "S0=13") {
		t.Errorf("RenderObserverView = %q, want to contain %q", got, "S0=13")
	}
}

// TestRenderObserverViewFewerHands verifies that ObserverView does not panic
// when Hands has fewer than 4 entries.
//
// This tests defensive rendering, not valid game state. The renderer
// assumes valid input; fewer than 4 hands is a caller bug.
func TestRenderObserverViewFewerHands(t *testing.T) {
	snap := heartsclient.ObserverSnapshot{
		RoundNumber:   1,
		TrickNumber:   1,
		PassDirection: "none",
		Turn:          0,
		Hands: [][]heartsclient.Card{
			{{Rank: "ace", Suit: "spades"}},
		},
		Trick:       []heartsclient.TrickEntry{},
		Scores:      []int{0},
		RoundPoints: []int{0},
	}

	got := RenderObserverView(snap)
	if !strings.Contains(got, "Seat 0") {
		t.Errorf("RenderObserverView with 1 hand = %q, want to contain %q", got, "Seat 0")
	}
}
