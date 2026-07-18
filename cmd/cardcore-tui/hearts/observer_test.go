package heartstui

import (
	"strings"
	"testing"

	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// TestRenderObserverViewFourHands verifies that a 4-hand ObserverSnapshot
// includes all four "Seat N" labels.
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

	got := RenderObserverView(snap, NewDarkTheme(), 80, 30)

	for i := 0; i < 4; i++ {
		label := "Seat " + string(rune('0'+i))
		if !strings.Contains(got, label) {
			t.Errorf("RenderObserverView = %q, want to contain %q", got, label)
		}
	}

	lines := strings.Split(got, "\n")
	if len(lines) != 30 {
		t.Fatalf("RenderObserverView has %d lines, want 30", len(lines))
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

	got := RenderObserverView(snap, NewDarkTheme(), 80, 30)
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

	got := RenderObserverView(snap, NewDarkTheme(), 80, 30)
	want := "Seat 1 won"
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

	got := RenderObserverView(snap, NewDarkTheme(), 80, 30)
	if strings.Contains(got, "Trick complete") {
		t.Errorf("RenderObserverView = %q, want no winner line when TrickWinner is -1", got)
	}
	if !strings.Contains(got, "Seat 2's turn") {
		t.Errorf("RenderObserverView = %q, want turn line when no winner", got)
	}
}

// TestRenderObserverViewClearsTrickInPassingPhase verifies that the previous
// round's final trick is not shown in the central area during the passing
// phase.
func TestRenderObserverViewClearsTrickInPassingPhase(t *testing.T) {
	snap := heartsclient.ObserverSnapshot{
		RoundNumber:   2,
		TrickNumber:   1,
		PassDirection: "left",
		Phase:         heartsclient.PhasePassing,
		Turn:          0,
		Hands: [][]heartsclient.Card{
			{{Rank: "two", Suit: "clubs"}},
			{{Rank: "three", Suit: "clubs"}},
			{{Rank: "four", Suit: "clubs"}},
			{{Rank: "five", Suit: "clubs"}},
		},
		Trick: []heartsclient.TrickEntry{
			{Seat: 0, Card: heartsclient.Card{Rank: "ace", Suit: "spades"}},
			{Seat: 1, Card: heartsclient.Card{Rank: "king", Suit: "hearts"}},
			{Seat: 2, Card: heartsclient.Card{Rank: "queen", Suit: "diamonds"}},
			{Seat: 3, Card: heartsclient.Card{Rank: "jack", Suit: "clubs"}},
		},
		Scores:      []int{0, 0, 0, 0},
		RoundPoints: []int{0, 0, 0, 0},
	}

	got := stripANSI(RenderObserverView(snap, NewDarkTheme(), 80, 30))
	for _, wantAbsent := range []string{"♠A", "♥K", "♦Q", "♣J"} {
		if strings.Contains(got, wantAbsent) {
			t.Errorf(
				"RenderObserverView(passing) = %q, should not contain trick card %q",
				got, wantAbsent,
			)
		}
	}
	if strings.Contains(got, "won") {
		t.Errorf("RenderObserverView(passing) = %q, should not show a trick winner", got)
	}
	if !strings.Contains(got, "Players Passing Left") {
		t.Errorf("RenderObserverView(passing) = %q, want 'Players Passing Left'", got)
	}
	if strings.Contains(got, "Seat 0's turn") {
		t.Errorf("RenderObserverView(passing) = %q, should not show a turn line", got)
	}
}

// TestRenderObserverRoundCompleteView verifies the round-complete view shows
// scores for all seats inside a bordered box.
func TestRenderObserverRoundCompleteView(t *testing.T) {
	snap := heartsclient.ObserverSnapshot{
		RoundNumber: 1,
		Scores:      []int{13, 0, 13, 0},
		RoundPoints: []int{11, 0, 0, 0},
	}
	got := RenderObserverRoundCompleteView(snap, NewDarkTheme(), 80, 14)
	if !strings.Contains(got, "Round 1 Completed") {
		t.Errorf("RenderObserverRoundCompleteView = %q, want 'Round 1 Completed'", got)
	}
	if !strings.Contains(stripANSI(got), "Seat 0: 13 (+11)") {
		t.Errorf("RenderObserverRoundCompleteView = %q, want Seat 0 score", got)
	}
	plain := "Round 1 Completed\n" +
		"Seat 0: 13 (+11)\n" +
		"Seat 1: 0 (+0)\n" +
		"Seat 2: 13 (+0)\n" +
		"Seat 3: 0 (+0)"
	if got == plain {
		t.Errorf("RenderObserverRoundCompleteView should add a border around the content")
	}
}

// TestRenderObserverRoundCompleteViewMismatch verifies an explicit error is
// shown when RoundPoints length does not match Scores length.
func TestRenderObserverRoundCompleteViewMismatch(t *testing.T) {
	snap := heartsclient.ObserverSnapshot{
		RoundNumber: 1,
		Scores:      []int{13, 0, 13, 0},
		RoundPoints: []int{11, 0},
	}
	got := RenderObserverRoundCompleteView(snap, NewDarkTheme(), 80, 14)
	if !strings.Contains(got, "ERROR") {
		t.Errorf("RenderObserverRoundCompleteView = %q, want to contain 'ERROR'", got)
	}
}

// TestRenderObserverRoundCompleteViewMoonShot verifies the observer round summary
// shows a celebratory cow and moon emoji line when a player shoots the moon.
func TestRenderObserverRoundCompleteViewMoonShot(t *testing.T) {
	snap := heartsclient.ObserverSnapshot{
		RoundNumber: 1,
		Scores:      []int{0, 26, 26, 26},
		RoundPoints: []int{0, 26, 26, 26},
	}
	got := RenderObserverRoundCompleteView(snap, NewDarkTheme(), 80, 16)
	if !strings.Contains(got, "🐄") {
		t.Errorf("RenderObserverRoundCompleteView(moon) = %q, want cow emoji", got)
	}
	if !strings.Contains(got, "🌙") {
		t.Errorf("RenderObserverRoundCompleteView(moon) = %q, want moon emoji", got)
	}
	if !strings.Contains(stripANSI(got), "Seat 0 shot the moon") {
		t.Errorf("RenderObserverRoundCompleteView(moon) = %q, want moon shot message", got)
	}
}

// TestClientRenderObserverRoundComplete verifies that an observer client routes
// round_complete snapshots to the round summary view.
func TestClientRenderObserverRoundComplete(t *testing.T) {
	c := NewClient(0, true, NewDarkTheme())
	snap := heartsclient.ObserverSnapshot{
		Phase:       heartsclient.PhaseRoundComplete,
		RoundNumber: 1,
		Scores:      []int{13, 0, 13, 0},
		RoundPoints: []int{11, 0, 0, 0},
	}
	c.HandleSnapshot(mustMarshal(t, snap))

	got := c.Render(80, 14)
	if !strings.Contains(got, "Round 1 Completed") {
		t.Errorf("observer render = %q, want 'Round 1 Completed'", got)
	}
}
