package heartstui

import (
	"strings"
	"testing"
	"unicode/utf8"

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
		CardNormal, CardCursor, CardCursorDimmed,
		CardCursorSelected, CardSelected, CardDimmed,
		CardWinner,
	}
	for _, state := range states {
		got := RenderCard(card, state, NewDarkTheme())
		if got == "" {
			t.Errorf("RenderCard(%+v, %v) returned empty string", card, state)
		}
		if !strings.Contains(stripANSI(got), label) {
			t.Errorf("RenderCard(%+v, %v) = %q, want to contain %q", card, state, got, label)
		}
	}
}

// TestRenderCardBorderedBox verifies that RenderCard draws a rounded border
// around the card label for each state.
func TestRenderCardBorderedBox(t *testing.T) {
	card := heartsclient.Card{Rank: "ace", Suit: "spades"}
	label := CardLabel(card)

	states := []CardState{
		CardNormal, CardCursor, CardCursorDimmed,
		CardCursorSelected, CardSelected, CardDimmed,
		CardWinner,
	}
	for _, state := range states {
		got := stripANSI(RenderCard(card, state, NewDarkTheme()))
		if !strings.Contains(got, "╭") || !strings.Contains(got, "╮") {
			t.Errorf("RenderCard(%v) missing top border: %q", state, got)
		}
		if !strings.Contains(got, "╰") || !strings.Contains(got, "╯") {
			t.Errorf("RenderCard(%v) missing bottom border: %q", state, got)
		}
		if !strings.Contains(got, label) {
			t.Errorf("RenderCard(%v) missing label: %q", state, got)
		}
	}
}

// TestRenderHandGapWidth verifies that adjacent cards are separated by a
// one-space gap between their rounded borders.
func TestRenderHandGapWidth(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
		{Rank: "two", Suit: "clubs"},
	}

	got := stripANSI(RenderHand(hand, -1, nil, nil, false, NewDarkTheme(), 80))
	want := " ╭──╮ ╭──╮ ╭──╮\n │♥K│ │♠A│ │♣2│\n ╰──╯ ╰──╯ ╰──╯"
	if got != want {
		t.Errorf("RenderHand = %q, want %q", got, want)
	}
}

// TestRenderHandTenCard verifies that 3-character labels (e.g., "♥10") widen
// the card box while keeping a one-space gap.
func TestRenderHandTenCard(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "ten", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}

	got := stripANSI(RenderHand(hand, -1, nil, nil, false, NewDarkTheme(), 80))
	want := " ╭───╮ ╭──╮\n │♥10│ │♠A│\n ╰───╯ ╰──╯"
	if got != want {
		t.Errorf("RenderHand = %q, want %q", got, want)
	}

	got = stripANSI(RenderHand(hand, 0, nil, nil, false, NewDarkTheme(), 80))
	want = " ╭───╮ ╭──╮\n │♥10│ │♠A│\n ╰───╯ ╰──╯"
	if got != want {
		t.Errorf("RenderHand(cursor on ten) = %q, want %q", got, want)
	}
}

// TestRenderHandTenCardNotFirst verifies that the 3-character "10" label is
// rendered correctly when the cursor is on it and it is not the first card.
func TestRenderHandTenCardNotFirst(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "ace", Suit: "spades"},
		{Rank: "ten", Suit: "hearts"},
		{Rank: "two", Suit: "clubs"},
	}

	got := stripANSI(RenderHand(hand, 1, nil, nil, false, NewDarkTheme(), 80))
	want := " ╭──╮ ╭───╮ ╭──╮\n │♠A│ │♥10│ │♣2│\n ╰──╯ ╰───╯ ╰──╯"
	if got != want {
		t.Errorf("RenderHand(cursor on ten) = %q, want %q", got, want)
	}
}

// TestRenderHandMultipleTenCards verifies that a hand with multiple
// 3-character labels renders with wider boxes and one-space gaps. The visible
// layout does not shift between cursor positions because the cursor is expressed
// through styling changes.
func TestRenderHandMultipleTenCards(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "ten", Suit: "hearts"},
		{Rank: "ten", Suit: "spades"},
		{Rank: "ten", Suit: "clubs"},
		{Rank: "ten", Suit: "diamonds"},
	}

	want := " ╭───╮ ╭───╮ ╭───╮ ╭───╮\n │♥10│ │♠10│ │♣10│ │♦10│\n ╰───╯ ╰───╯ ╰───╯ ╰───╯"
	for _, cursor := range []int{0, 1, 2, 3} {
		got := stripANSI(RenderHand(hand, cursor, nil, nil, false, NewDarkTheme(), 80))
		if got != want {
			t.Errorf("cursor=%d: RenderHand = %q, want %q", cursor, got, want)
		}
	}
}

// TestRenderHandCursorVisible verifies that the cursor position is visible
// through styling and that the card labels do not shift when the cursor moves.
func TestRenderHandCursorVisible(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
		{Rank: "two", Suit: "clubs"},
	}

	plain := RenderHand(hand, -1, nil, nil, false, NewDarkTheme(), 80)
	visible := stripANSI(plain)

	for _, cursor := range []int{0, 1, 2} {
		got := RenderHand(hand, cursor, nil, nil, false, NewDarkTheme(), 80)
		if stripANSI(got) != visible {
			t.Errorf("cursor=%d: visible text shifted: %q", cursor, stripANSI(got))
		}
		if got == plain {
			t.Errorf("cursor=%d: cursor styling not visible in raw output", cursor)
		}
	}
}

// TestRenderHandSelectedVisible verifies that selected cards are visible
// through styling and that the card labels do not shift when cards are selected.
func TestRenderHandSelectedVisible(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}
	selected := []heartsclient.Card{{Rank: "king", Suit: "hearts"}}

	plain := RenderHand(hand, -1, nil, nil, false, NewDarkTheme(), 80)
	got := RenderHand(hand, -1, selected, nil, false, NewDarkTheme(), 80)
	if stripANSI(got) != stripANSI(plain) {
		t.Errorf("selected hand visible text shifted: %q", stripANSI(got))
	}
	if got == plain {
		t.Errorf("selected hand styling not visible in raw output")
	}
}

// TestRenderHandCursorSelectionStyling verifies that the combined cursor and
// selection states produce visible styling without shifting the card labels.
func TestRenderHandCursorSelectionStyling(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}

	cases := []struct {
		name     string
		cursor   int
		selected []heartsclient.Card
	}{
		{
			name:     "same card selected and cursor",
			cursor:   0,
			selected: []heartsclient.Card{{Rank: "king", Suit: "hearts"}},
		},
		{
			name:     "different cards selected and cursor",
			cursor:   1,
			selected: []heartsclient.Card{{Rank: "king", Suit: "hearts"}},
		},
		{
			name:     "cursor on last selected card",
			cursor:   1,
			selected: []heartsclient.Card{{Rank: "ace", Suit: "spades"}},
		},
		{
			name:     "cursor on first selected second",
			cursor:   0,
			selected: []heartsclient.Card{{Rank: "ace", Suit: "spades"}},
		},
	}

	plain := RenderHand(hand, -1, nil, nil, false, NewDarkTheme(), 80)
	visiblePlain := stripANSI(plain)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderHand(hand, tc.cursor, tc.selected, nil, false, NewDarkTheme(), 80)
			if stripANSI(got) != visiblePlain {
				t.Errorf("visible text shifted: %q", stripANSI(got))
			}
			if got == plain {
				t.Errorf("cursor/selection styling not visible in raw output")
			}
		})
	}
}

// TestRenderHandFirstCardMargin verifies that the first card has a one-space
// leading margin and a rounded border.
func TestRenderHandFirstCardMargin(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "ace", Suit: "spades"},
		{Rank: "two", Suit: "clubs"},
	}

	noCursor := stripANSI(RenderHand(hand, -1, nil, nil, false, NewDarkTheme(), 80))
	if !strings.HasPrefix(noCursor, " ╭──") {
		t.Errorf("RenderHand(no cursor) = %q, want leading space and border", noCursor)
	}

	withCursor := stripANSI(RenderHand(hand, 0, nil, nil, false, NewDarkTheme(), 80))
	if !strings.HasPrefix(withCursor, " ╭──") {
		t.Errorf("RenderHand(cursor on first) = %q, want leading space and border", withCursor)
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

	withLegal := RenderHand(hand, -1, nil, legal, false, NewDarkTheme(), 80)
	withoutLegal := RenderHand(hand, -1, nil, nil, false, NewDarkTheme(), 80)

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

// TestRenderHandCursorOnIllegalCard verifies that the cursor on an illegal card
// is visible through styling and the visible layout does not shift.
func TestRenderHandCursorOnIllegalCard(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "ace", Suit: "spades"},
		{Rank: "king", Suit: "hearts"},
	}
	legal := []heartsclient.Card{{Rank: "ace", Suit: "spades"}}

	got := stripANSI(RenderHand(hand, 1, nil, legal, false, NewDarkTheme(), 80))
	want := " ╭──╮ ╭──╮\n │♠A│ │♥K│\n ╰──╯ ╰──╯"
	if got != want {
		t.Errorf("RenderHand(cursor on illegal card) = %q, want %q", got, want)
	}
}

// TestCardStateForCursorSelected verifies that the cursor on a selected card
// returns the combined CardCursorSelected state.
func TestCardStateForCursorSelected(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}
	selected := []heartsclient.Card{hand[0]}
	selectedSet := cardSet(selected)

	got := cardStateFor(0, hand[0], 0, selectedSet, nil, false)
	if got != CardCursorSelected {
		t.Errorf("cardStateFor(selected card under cursor) = %v, want CardCursorSelected", got)
	}
}

// TestRenderHandCursorSelectedDistinct verifies that a card that is both
// selected and under the cursor renders differently from a plain card, a
// cursor-only card, and a selected-only card.
func TestRenderHandCursorSelectedDistinct(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "king", Suit: "hearts"},
	}
	selected := []heartsclient.Card{hand[0]}

	plain := RenderHand(hand, -1, nil, nil, false, NewDarkTheme(), 80)
	cursorOnly := RenderHand(hand, 0, nil, nil, false, NewDarkTheme(), 80)
	selectedOnly := RenderHand(hand, -1, selected, nil, false, NewDarkTheme(), 80)
	combined := RenderHand(hand, 0, selected, nil, false, NewDarkTheme(), 80)

	if combined == plain || combined == cursorOnly || combined == selectedOnly {
		t.Errorf("cursor+selected styling not distinct from plain, cursor-only, or selected-only")
	}
	if stripANSI(combined) != stripANSI(plain) {
		t.Errorf("cursor+selected shifted visible text: %q", stripANSI(combined))
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

	got := stripANSI(RenderHand(hand, 0, selected, nil, true, NewDarkTheme(), 80))
	want := " ╭──╮ ╭──╮\n │♥K│ │♠A│\n ╰──╯ ╰──╯"
	if got != want {
		t.Errorf("RenderHand(inputDisabled) = %q, want %q", got, want)
	}
}

// TestRenderHandFullWidth verifies that a 13-card hand fits within the
// terminal width at various sizes, including the worst-case mix of four
// 3-character ten labels and nine 2-character non-ten labels. At 80 columns
// the hand must fit within 80; at wider terminals the expanded gaps must not
// exceed the terminal width.
func TestRenderHandFullWidth(t *testing.T) {
	hand := []heartsclient.Card{
		{Rank: "ten", Suit: "hearts"},
		{Rank: "ten", Suit: "spades"},
		{Rank: "ten", Suit: "diamonds"},
		{Rank: "ten", Suit: "clubs"},
		{Rank: "ace", Suit: "hearts"},
		{Rank: "king", Suit: "spades"},
		{Rank: "queen", Suit: "diamonds"},
		{Rank: "jack", Suit: "clubs"},
		{Rank: "nine", Suit: "hearts"},
		{Rank: "eight", Suit: "spades"},
		{Rank: "seven", Suit: "diamonds"},
		{Rank: "six", Suit: "clubs"},
		{Rank: "five", Suit: "hearts"},
	}

	for _, tc := range []struct {
		name  string
		width int
	}{
		{"80 columns", 80},
		{"100 columns", 100},
		{"120 columns", 120},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderHand(hand, -1, nil, nil, false, NewDarkTheme(), tc.width)
			for _, line := range strings.Split(got, "\n") {
				stripped := stripANSI(line)
				colCount := utf8.RuneCountInString(stripped)
				if colCount > tc.width {
					t.Errorf("hand line exceeds %d columns: %d chars: %q",
						tc.width, colCount, stripped)
				}
			}
		})
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
