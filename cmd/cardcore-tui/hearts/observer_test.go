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

	got := RenderObserverView(snap, NewDarkTheme(), 80)

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

	got := RenderObserverView(snap, NewDarkTheme(), 80)
	if !strings.Contains(got, "Seat 0") {
		t.Errorf("RenderObserverView with 1 hand = %q, want to contain %q", got, "Seat 0")
	}
}

// TestRenderObserverViewTrickCompleteWinner verifies that the observer view
// shows the winner line during the trick_complete phase.
func TestRenderObserverViewTrickCompleteWinner(t *testing.T) {
	snap := heartsclient.ObserverSnapshot{
		RoundNumber:   1,
		TrickNumber:   1,
		PassDirection: "left",
		Phase:         heartsclient.PhaseTrickComplete,
		Turn:          2,
		TrickWinner:   1,
		Hands: [][]heartsclient.Card{
			{{Rank: "seven", Suit: "spades"}},
			{{Rank: "two", Suit: "hearts"}},
			{{Rank: "three", Suit: "diamonds"}},
			{{Rank: "four", Suit: "hearts"}},
		},
		Trick: []heartsclient.TrickEntry{
			{Seat: 0, Card: heartsclient.Card{Rank: "two", Suit: "clubs"}},
			{Seat: 1, Card: heartsclient.Card{Rank: "ace", Suit: "clubs"}},
			{Seat: 2, Card: heartsclient.Card{Rank: "king", Suit: "clubs"}},
			{Seat: 3, Card: heartsclient.Card{Rank: "queen", Suit: "clubs"}},
		},
		Scores:      []int{0, 0, 0, 0},
		RoundPoints: []int{0, 0, 0, 0},
	}

	got := RenderObserverView(snap, NewDarkTheme(), 80)
	want := "Trick complete — Seat 1 won"
	if !strings.Contains(got, want) {
		t.Errorf("RenderObserverView = %q, want to contain %q", got, want)
	}
	if strings.Contains(got, "Seat 2's turn") {
		t.Errorf("RenderObserverView = %q, want no turn line during trick_complete", got)
	}
}

// TestRenderObserverViewTrickCompleteNoWinner verifies that the observer view
// shows the turn line when the trick is complete but no winner is provided.
func TestRenderObserverViewTrickCompleteNoWinner(t *testing.T) {
	snap := heartsclient.ObserverSnapshot{
		RoundNumber:   1,
		TrickNumber:   1,
		PassDirection: "left",
		Phase:         heartsclient.PhaseTrickComplete,
		Turn:          2,
		TrickWinner:   -1,
		Hands: [][]heartsclient.Card{
			{{Rank: "seven", Suit: "spades"}},
		},
		Trick:       []heartsclient.TrickEntry{},
		Scores:      []int{0},
		RoundPoints: []int{0},
	}

	got := RenderObserverView(snap, NewDarkTheme(), 80)
	if strings.Contains(got, "Trick complete") {
		t.Errorf("RenderObserverView = %q, want no winner line when TrickWinner is -1", got)
	}
	if !strings.Contains(got, "Seat 2's turn") {
		t.Errorf("RenderObserverView = %q, want turn line when no winner", got)
	}
}
