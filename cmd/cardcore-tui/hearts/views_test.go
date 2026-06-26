package heartstui

import (
	"strings"
	"testing"

	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// TestRenderPassingViewThreeSelected verifies that with 3 selected cards the
// status line says "Press Enter to pass".
func TestRenderPassingViewThreeSelected(t *testing.T) {
	snap := heartsclient.PlayerSnapshot{
		RoundNumber:   1,
		PassDirection: "left",
		Hand: []heartsclient.Card{
			{Rank: "queen", Suit: "diamonds"},
			{Rank: "king", Suit: "hearts"},
			{Rank: "ace", Suit: "spades"},
		},
	}
	selected := []heartsclient.Card{
		{Rank: "queen", Suit: "diamonds"},
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}

	got := RenderPassingView(snap, 0, 0, selected)
	want := "Press Enter to pass"
	if !strings.Contains(got, want) {
		t.Errorf("RenderPassingView with 3 selected = %q, want to contain %q", got, want)
	}
}

// TestRenderPassingViewOneSelected verifies that with 1 selected card the
// status line says "Select 2 more card(s) to pass".
func TestRenderPassingViewOneSelected(t *testing.T) {
	snap := heartsclient.PlayerSnapshot{
		RoundNumber:   1,
		PassDirection: "left",
		Hand: []heartsclient.Card{
			{Rank: "queen", Suit: "diamonds"},
			{Rank: "king", Suit: "hearts"},
			{Rank: "ace", Suit: "spades"},
		},
	}
	selected := []heartsclient.Card{
		{Rank: "queen", Suit: "diamonds"},
	}

	got := RenderPassingView(snap, 0, 0, selected)
	want := "Select 2 more card(s) to pass"
	if !strings.Contains(got, want) {
		t.Errorf("RenderPassingView with 1 selected = %q, want to contain %q", got, want)
	}
}

// TestRenderPlayingViewYourTurn verifies that when snap.Turn == seat the
// status line says "Your turn".
func TestRenderPlayingViewYourTurn(t *testing.T) {
	snap := heartsclient.PlayerSnapshot{
		Phase:        "playing",
		Turn:         0,
		Hand:         []heartsclient.Card{{Rank: "ace", Suit: "spades"}},
		Trick:        []heartsclient.TrickEntry{},
		LegalActions: []heartsclient.Card{{Rank: "ace", Suit: "spades"}},
	}

	got := RenderPlayingView(snap, 0, 0)
	want := "Your turn"
	if !strings.Contains(got, want) {
		t.Errorf("RenderPlayingView(your turn) = %q, want to contain %q", got, want)
	}
}

// TestRenderPlayingViewWaiting verifies that when snap.Turn != seat the
// status line says "Waiting for seat".
func TestRenderPlayingViewWaiting(t *testing.T) {
	snap := heartsclient.PlayerSnapshot{
		Phase:        "playing",
		Turn:         2,
		Hand:         []heartsclient.Card{{Rank: "ace", Suit: "spades"}},
		Trick:        []heartsclient.TrickEntry{},
		LegalActions: []heartsclient.Card{},
	}

	got := RenderPlayingView(snap, 0, 0)
	want := "Waiting for seat 2"
	if !strings.Contains(got, want) {
		t.Errorf("RenderPlayingView(waiting) = %q, want to contain %q", got, want)
	}
}

// TestRenderTrickWithEntries verifies that a non-empty trick includes seat
// labels.
func TestRenderTrickWithEntries(t *testing.T) {
	trick := []heartsclient.TrickEntry{
		{Seat: 2, Card: heartsclient.Card{Rank: "five", Suit: "diamonds"}},
		{Seat: 3, Card: heartsclient.Card{Rank: "king", Suit: "diamonds"}},
	}

	got := RenderTrick(trick)
	want := "Seat 2"
	if !strings.Contains(got, want) {
		t.Errorf("RenderTrick = %q, want to contain %q", got, want)
	}
	want = "Seat 3"
	if !strings.Contains(got, want) {
		t.Errorf("RenderTrick = %q, want to contain %q", got, want)
	}
}

// TestRenderTrickEmpty verifies that an empty trick shows the placeholder.
func TestRenderTrickEmpty(t *testing.T) {
	got := RenderTrick([]heartsclient.TrickEntry{})
	want := "(no cards played yet)"
	if got != want {
		t.Errorf("RenderTrick(empty) = %q, want %q", got, want)
	}
}
