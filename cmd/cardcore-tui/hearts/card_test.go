package heartstui

import (
	"strings"
	"testing"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// TestCardLabel verifies exact card label output.
func TestCardLabel(t *testing.T) {
	tests := []struct {
		name string
		card heartsclient.Card
		want string
	}{
		{"two of clubs", heartsclient.Card{Rank: "two", Suit: "clubs"}, "♣2"},
		{"ten of hearts", heartsclient.Card{Rank: "ten", Suit: "hearts"}, "♥10"},
		{"ace of spades", heartsclient.Card{Rank: "ace", Suit: "spades"}, "♠A"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CardLabel(tt.card)
			if got != tt.want {
				t.Errorf("CardLabel(%+v) = %q, want %q", tt.card, got, tt.want)
			}
		})
	}
}

// TestRenderCardStates verifies RenderCard returns non-empty strings containing
// the card label for each CardState and does not panic.
func TestRenderCardStates(t *testing.T) {
	card := heartsclient.Card{Rank: "ace", Suit: "spades"}
	label := CardLabel(card)

	states := []CardState{CardNormal, CardCursor, CardSelected, CardDimmed}
	for _, state := range states {
		got := RenderCard(card, state)
		if got == "" {
			t.Errorf("RenderCard(%+v, %v) returned empty string", card, state)
		}
		if !strings.Contains(got, label) {
			t.Errorf("RenderCard(%+v, %v) = %q, want to contain %q", card, state, got, label)
		}
	}
}

// TestRenderHandCursor verifies that a cursor in range produces different
// output than cursor=-1.
func TestRenderHandCursor(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}

	withCursor := RenderHand(hand, 0, nil, nil, false)
	withoutCursor := RenderHand(hand, -1, nil, nil, false)

	if withCursor == withoutCursor {
		t.Errorf("cursor in range produced same output as cursor=-1, want different")
	}
}

// TestRenderHandSelected verifies that a selected card's output differs from
// when it is not selected.
func TestRenderHandSelected(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}
	selected := []heartsclient.Card{{Rank: "ace", Suit: "spades"}}

	withSelected := RenderHand(hand, -1, selected, nil, false)
	withoutSelected := RenderHand(hand, -1, nil, nil, false)

	if withSelected == withoutSelected {
		t.Errorf("selected card produced same output as unselected, want different")
	}
}

// TestRenderHandLegalDimming verifies that with a legal list excluding some
// cards, output differs from legal=nil.
func TestRenderHandLegalDimming(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "two", Suit: "clubs"},
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}
	legal := []heartsclient.Card{{Rank: "ace", Suit: "spades"}}

	withLegal := RenderHand(hand, -1, nil, legal, false)
	withoutLegal := RenderHand(hand, -1, nil, nil, false)

	if withLegal == withoutLegal {
		t.Errorf("legal dimming produced same output as legal=nil, want different")
	}
}

// TestRenderHandCursorNegative verifies that cursor=-1 works (observer reuse).
func TestRenderHandCursorNegative(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}

	got := RenderHand(hand, -1, nil, nil, false)
	if got == "" {
		t.Errorf("RenderHand with cursor=-1 returned empty string, want non-empty")
	}
}
