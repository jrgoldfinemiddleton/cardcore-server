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

// TestRenderTrickCompleteViewWinner verifies the winner is shown when the
// trick has all 4 cards.
func TestRenderTrickCompleteViewWinner(t *testing.T) {
	snap := heartsclient.PlayerSnapshot{
		Trick: []heartsclient.TrickEntry{
			{Seat: 0, Card: heartsclient.Card{Rank: "two", Suit: "clubs"}},
			{Seat: 1, Card: heartsclient.Card{Rank: "ace", Suit: "clubs"}},
			{Seat: 2, Card: heartsclient.Card{Rank: "king", Suit: "clubs"}},
			{Seat: 3, Card: heartsclient.Card{Rank: "queen", Suit: "clubs"}},
		},
		Turn: 1,
	}
	got := RenderTrickCompleteView(snap, 0)
	if !strings.Contains(got, "Seat 1 won") {
		t.Errorf("RenderTrickCompleteView = %q, want to contain 'Seat 1 won'", got)
	}
}

// TestRenderTrickCompleteViewIncomplete verifies no winner is claimed when
// the trick is not complete.
func TestRenderTrickCompleteViewIncomplete(t *testing.T) {
	snap := heartsclient.PlayerSnapshot{
		Trick: []heartsclient.TrickEntry{
			{Seat: 0, Card: heartsclient.Card{Rank: "two", Suit: "clubs"}},
			{Seat: 1, Card: heartsclient.Card{Rank: "ace", Suit: "clubs"}},
		},
	}
	got := RenderTrickCompleteView(snap, 0)
	if strings.Contains(got, "won") {
		t.Errorf("RenderTrickCompleteView = %q, should not claim winner for incomplete trick", got)
	}
	if !strings.Contains(got, "Trick complete") {
		t.Errorf("RenderTrickCompleteView = %q, want to contain 'Trick complete'", got)
	}
}

// TestRenderRoundCompleteView verifies the round-complete view shows scores.
func TestRenderRoundCompleteView(t *testing.T) {
	snap := heartsclient.PlayerSnapshot{
		RoundNumber: 1,
		Scores:      []int{13, 0, 13, 0},
		RoundPoints: []int{11, 0, 0, 0},
	}
	got := RenderRoundCompleteView(snap)
	if !strings.Contains(got, "Round 1 complete") {
		t.Errorf("RenderRoundCompleteView = %q, want 'Round 1 complete'", got)
	}
	if !strings.Contains(got, "Seat 0: 13 (+11)") {
		t.Errorf("RenderRoundCompleteView = %q, want Seat 0 score", got)
	}
}

// TestRenderRoundCompleteViewMismatch verifies an explicit error is shown
// when RoundPoints length does not match Scores length.
func TestRenderRoundCompleteViewMismatch(t *testing.T) {
	snap := heartsclient.PlayerSnapshot{
		RoundNumber: 1,
		Scores:      []int{13, 0, 13, 0},
		RoundPoints: []int{11, 0},
	}
	got := RenderRoundCompleteView(snap)
	if !strings.Contains(got, "ERROR") {
		t.Errorf("RenderRoundCompleteView = %q, want to contain 'ERROR'", got)
	}
}

// TestRenderGameOverView verifies the game-over view shows final scores and
// an exit prompt.
func TestRenderGameOverView(t *testing.T) {
	snap := heartsclient.PlayerSnapshot{
		Scores: []int{26, 0, 0, 0},
	}
	got := RenderGameOverView(snap)
	if !strings.Contains(got, "Game Over") {
		t.Errorf("RenderGameOverView = %q, want 'Game Over'", got)
	}
	if !strings.Contains(got, "Seat 0: 26") {
		t.Errorf("RenderGameOverView = %q, want Seat 0 score", got)
	}
	if !strings.Contains(got, "Press Enter to exit") {
		t.Errorf("RenderGameOverView = %q, want exit prompt", got)
	}
}
