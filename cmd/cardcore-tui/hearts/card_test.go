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

	states := []CardState{
		CardNormal, CardCursor, CardCursorDimmed, CardSelected, CardDimmed, CardWinner,
	}
	for _, state := range states {
		got := RenderCard(card, state)
		if got == "" {
			t.Errorf("RenderCard(%+v, %v) returned empty string", card, state)
		}
		if !strings.Contains(stripANSI(got), label) {
			t.Errorf("RenderCard(%+v, %v) = %q, want to contain %q", card, state, got, label)
		}
	}
}

// TestRenderCardNoBrackets verifies that RenderCard does not wrap the card
// label in cursor brackets or append the selected checkmark.
func TestRenderCardNoBrackets(t *testing.T) {
	card := heartsclient.Card{Rank: "ace", Suit: "spades"}

	got := stripANSI(RenderCard(card, CardCursor))
	if strings.Contains(got, "[") || strings.Contains(got, "]") {
		t.Errorf("RenderCard(cursor) = %q, want no brackets", got)
	}
	if got := stripANSI(RenderCard(card, CardSelected)); strings.Contains(got, selectedMarker) {
		t.Errorf("RenderCard(selected) = %q, want no checkmark", got)
	}
}

// TestRenderHandGapWidth verifies that adjacent cards are separated by a
// four-space gap when no marker is present.
func TestRenderHandGapWidth(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
		{Rank: "two", Suit: "clubs"},
	}

	got := stripANSI(RenderHand(hand, -1, nil, nil, false))
	want := " ♥K    ♠A    ♣2"
	if got != want {
		t.Errorf("RenderHand = %q, want %q", got, want)
	}
}

// TestRenderHandTenCard verifies that 3-character labels (e.g., "♥10") are
// spaced correctly with the same 4-space gap.
func TestRenderHandTenCard(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "ten", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}

	got := stripANSI(RenderHand(hand, -1, nil, nil, false))
	want := " ♥10    ♠A"
	if got != want {
		t.Errorf("RenderHand = %q, want %q", got, want)
	}

	got = stripANSI(RenderHand(hand, 0, nil, nil, false))
	want = "[♥10 ]  ♠A"
	if got != want {
		t.Errorf("RenderHand(cursor on ten) = %q, want %q", got, want)
	}
}

// TestRenderHandTenCardNotFirst verifies that the 3-character "10" label is
// wrapped correctly when the cursor is on it and it is not the first card.
func TestRenderHandTenCardNotFirst(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "ace", Suit: "spades"},
		{Rank: "ten", Suit: "hearts"},
		{Rank: "two", Suit: "clubs"},
	}

	got := stripANSI(RenderHand(hand, 1, nil, nil, false))
	want := " ♠A   [♥10 ]  ♣2"
	if got != want {
		t.Errorf("RenderHand(cursor on ten) = %q, want %q", got, want)
	}
}

// TestRenderHandMultipleTenCards verifies that a hand with multiple
// 3-character labels still uses 4-space gaps and that cursor brackets wrap the
// correct card without shifting the visible start of later cards.
func TestRenderHandMultipleTenCards(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "ten", Suit: "hearts"},
		{Rank: "ten", Suit: "spades"},
		{Rank: "ten", Suit: "clubs"},
		{Rank: "ten", Suit: "diamonds"},
	}

	cases := []struct {
		cursor int
		want   string
	}{
		{0, "[♥10 ]  ♠10    ♣10    ♦10"},
		{1, " ♥10   [♠10 ]  ♣10    ♦10"},
		{2, " ♥10    ♠10   [♣10 ]  ♦10"},
		{3, " ♥10    ♠10    ♣10   [♦10 ]"},
	}

	for _, tc := range cases {
		got := stripANSI(RenderHand(hand, tc.cursor, nil, nil, false))
		if got != tc.want {
			t.Errorf("cursor=%d: RenderHand = %q, want %q", tc.cursor, got, tc.want)
		}
	}
}

// TestRenderHandCursorMarkers verifies the visible bracket placement for each
// cursor position and that card labels do not shift.
func TestRenderHandCursorMarkers(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
		{Rank: "two", Suit: "clubs"},
	}

	cases := []struct {
		cursor int
		want   string
	}{
		{0, "[♥K ]  ♠A    ♣2"},
		{1, " ♥K   [♠A ]  ♣2"},
		{2, " ♥K    ♠A   [♣2 ]"},
	}

	for _, tc := range cases {
		got := stripANSI(RenderHand(hand, tc.cursor, nil, nil, false))
		if got != tc.want {
			t.Errorf("cursor=%d: RenderHand = %q, want %q", tc.cursor, got, tc.want)
		}
	}
}

// TestRenderHandSelectedMarker verifies that the selection checkmark appears
// in the gap after the selected card, not in the card label.
func TestRenderHandSelectedMarker(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}
	selected := []heartsclient.Card{{Rank: "king", Suit: "hearts"}}

	got := stripANSI(RenderHand(hand, -1, selected, nil, false))
	want := " ♥K✓   ♠A"
	if got != want {
		t.Errorf("RenderHand = %q, want %q", got, want)
	}
}

// TestRenderHandCursorAndSelection verifies interactions between cursor and
// selection markers. A card that is both selected and under the cursor shows
// the checkmark in the gap immediately after its label and the closing bracket
// two positions after its label. A selected card plus a cursor on a different
// card shows both markers in the correct gap.
func TestRenderHandCursorAndSelection(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}

	cases := []struct {
		name     string
		cursor   int
		selected []heartsclient.Card
		want     string
	}{
		{
			name:     "same card selected and cursor",
			cursor:   0,
			selected: []heartsclient.Card{{Rank: "king", Suit: "hearts"}},
			want:     "[♥K✓]  ♠A",
		},
		{
			name:     "different cards selected and cursor",
			cursor:   1,
			selected: []heartsclient.Card{{Rank: "king", Suit: "hearts"}},
			want:     " ♥K✓  [♠A ]",
		},
		{
			name:     "cursor on last selected card",
			cursor:   1,
			selected: []heartsclient.Card{{Rank: "ace", Suit: "spades"}},
			want:     " ♥K   [♠A✓]",
		},
		{
			name:     "cursor on first selected second",
			cursor:   0,
			selected: []heartsclient.Card{{Rank: "ace", Suit: "spades"}},
			want:     "[♥K ]  ♠A✓",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripANSI(RenderHand(hand, tc.cursor, tc.selected, nil, false))
			if got != tc.want {
				t.Errorf("RenderHand = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRenderHandFirstCardMargin verifies that the first card has a one-space
// leading margin and that the cursor opening bracket replaces it.
func TestRenderHandFirstCardMargin(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "ace", Suit: "spades"},
		{Rank: "two", Suit: "clubs"},
	}

	noCursor := stripANSI(RenderHand(hand, -1, nil, nil, false))
	if !strings.HasPrefix(noCursor, " ♠") {
		t.Errorf("RenderHand(no cursor) = %q, want leading space", noCursor)
	}

	withCursor := stripANSI(RenderHand(hand, 0, nil, nil, false))
	if !strings.HasPrefix(withCursor, "[♠") {
		t.Errorf("RenderHand(cursor on first) = %q, want leading bracket", withCursor)
	}
}

// TestRenderHandLegalDimming verifies that with a legal list excluding some
// cards, the rendered output uses different styling.
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
		t.Errorf("legal dimming produced same raw output as legal=nil, want different")
	}
}

// TestCardStateForCursorOnIllegalCard verifies that the cursor on an illegal
// card returns the combined CardCursorDimmed state.
func TestCardStateForCursorOnIllegalCard(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "ace", Suit: "spades"},
		{Rank: "king", Suit: "hearts"},
	}
	legal := []heartsclient.Card{{Rank: "ace", Suit: "spades"}}
	legalSet := cardSet(legal)
	selectedSet := cardSet(nil)

	got := cardStateFor(1, hand[1], 1, selectedSet, legalSet, true)
	if got != CardCursorDimmed {
		t.Errorf("cardStateFor(illegal card under cursor) = %v, want CardCursorDimmed", got)
	}

	got = cardStateFor(0, hand[0], 0, selectedSet, legalSet, true)
	if got != CardCursor {
		t.Errorf("cardStateFor(legal card under cursor) = %v, want CardCursor", got)
	}
}

// TestRenderHandCursorOnIllegalCard verifies that the cursor brackets remain
// on an illegal card and the visible layout does not shift.
func TestRenderHandCursorOnIllegalCard(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "ace", Suit: "spades"},
		{Rank: "king", Suit: "hearts"},
	}
	legal := []heartsclient.Card{{Rank: "ace", Suit: "spades"}}

	got := stripANSI(RenderHand(hand, 1, nil, legal, false))
	want := " ♠A   [♥K ]"
	if got != want {
		t.Errorf("RenderHand(cursor on illegal card) = %q, want %q", got, want)
	}
}

// TestRenderHandInputDisabled verifies that inputDisabled renders all cards
// dimmed and ignores cursor/selection.
func TestRenderHandInputDisabled(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}
	selected := []heartsclient.Card{{Rank: "king", Suit: "hearts"}}

	got := stripANSI(RenderHand(hand, 0, selected, nil, true))
	want := " ♥K    ♠A"
	if got != want {
		t.Errorf("RenderHand(inputDisabled) = %q, want %q", got, want)
	}
}

// stripANSI removes ANSI escape sequences from s so tests can assert on visible
// characters.
func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\u001b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
