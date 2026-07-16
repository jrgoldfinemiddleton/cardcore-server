package heartstui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// CardState describes how a card should be visually decorated.
type CardState int

const (
	// CardNormal is the default card appearance.
	CardNormal CardState = iota
	// CardCursor means the cursor is currently on this card.
	CardCursor
	// CardCursorDimmed means the cursor is on a card that is not legal.
	CardCursorDimmed
	// CardCursorSelected means the cursor is on a card that is also selected.
	CardCursorSelected
	// CardSelected means the card is selected (e.g., chosen to pass).
	CardSelected
	// CardDimmed means the card is not currently legal or selectable.
	CardDimmed
	// CardWinner highlights the card that won a completed trick.
	CardWinner
)

// handGapWidth is the number of visible spaces between adjacent cards in the
// hand when the terminal is 80 columns or narrower. With bordered cards, a
// one-space gap is enough to keep a 13-card hand within 80 columns (worst
// case: 4 tens * 5 chars + 9 non-tens * 4 chars + margins + borders).
const handGapWidth = 1

// firstCardMargin is the single space before the first card in the hand.
const firstCardMargin = 1

// RankSymbol maps a rank string to its display symbol.
//
// Known ranks: "two".."ten", "jack", "queen", "king", "ace".
// Unknown ranks return "?".
func RankSymbol(rank string) string {
	switch rank {
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

// SuitSymbol maps a suit string to its Unicode symbol.
//
// Known suits: "clubs", "diamonds", "hearts", "spades".
// Unknown suits return "?".
func SuitSymbol(suit string) string {
	switch suit {
	case "clubs":
		return "♣"
	case "diamonds":
		return "♦"
	case "hearts":
		return "♥"
	case "spades":
		return "♠"
	default:
		return "?"
	}
}

// CardLabel returns the short display label for a card: suit symbol followed by
// rank symbol. For example, Card{"ace","spades"} returns "♠A" and
// Card{"ten","hearts"} returns "♥10".
func CardLabel(c heartsclient.Card) string {
	return SuitSymbol(c.Suit) + RankSymbol(c.Rank)
}

// RenderCard returns a bordered card box for the given visual state, using
// the provided theme for colors. The card label is centered inside a rounded
// box. Hearts and diamonds are colored red; clubs and spades are colored
// light/dark. CardNormal and CardDimmed use a subtle dimmed border. CardCursor
// highlights the box border in the text color and makes the label bold.
// CardCursorSelected combines the accent background fill with the text border
// and bold label so selection remains visible under the cursor. CardSelected
// fills the box background with the accent color. CardDimmed renders the box
// border and label in the dimmed color. CardWinner highlights the winning card
// with a distinct background color.
func RenderCard(c heartsclient.Card, state CardState, theme Theme) string {
	label := CardLabel(c)
	fg := suitColor(c.Suit, theme)
	bg := theme.Background

	base := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		Background(bg).
		BorderBackground(bg)

	switch state {
	case CardDimmed:
		return base.Foreground(theme.Dimmed).BorderForeground(theme.Dimmed).Render(label)
	case CardCursor:
		return base.Foreground(fg).BorderForeground(theme.Text).Bold(true).Render(label)
	case CardCursorDimmed:
		return base.Foreground(theme.Dimmed).BorderForeground(theme.Text).Bold(true).Render(label)
	case CardCursorSelected:
		return base.Foreground(theme.Text).
			Background(theme.Accent).
			BorderForeground(theme.Text).
			Bold(true).
			Render(label)
	case CardSelected:
		return base.Foreground(theme.Text).
			Background(theme.Accent).
			BorderForeground(theme.Accent).
			Render(label)
	case CardWinner:
		return base.Foreground(fg).
			Background(theme.WinnerBg).
			BorderForeground(theme.Accent).
			Bold(true).
			Render(label)
	default:
		return base.Foreground(fg).BorderForeground(theme.Dimmed).Render(label)
	}
}

// RenderHand renders a hand of cards as a horizontal spread of bordered card
// boxes separated by gap spaces, fitting within the given terminal width, using
// the provided theme for colors. A zero width defaults to 80 columns.
//
// The cursor index highlights the card under the cursor by coloring its border
// with the accent color and making the label bold. If cursor is negative or out
// of range, no cursor highlight is drawn. A card whose value is in selected gets
// the selected decoration (accent background fill). If legal is non-empty, any
// card NOT in legal gets CardDimmed; if legal is nil/empty, nothing is dimmed.
// Card equality is by value (Rank AND Suit both match).
func RenderHand(
	hand []heartsclient.Card,
	cursor int,
	selected []heartsclient.Card,
	legal []heartsclient.Card,
	inputDisabled bool,
	theme Theme,
	width int,
) string {
	if len(hand) == 0 {
		return ""
	}

	if width == 0 {
		width = 80
	}
	gap := handGapForWidth(width)

	gapStyle := lipgloss.NewStyle().Background(theme.Background)
	marginLine := gapStyle.Render(strings.Repeat(" ", firstCardMargin))
	gapLine := gapStyle.Render(strings.Repeat(" ", gap))

	// If input is disabled (timeout), render all cards as dimmed to reflect
	// that no user interaction is allowed.
	if inputDisabled {
		cards := make([]string, len(hand))
		for i := range hand {
			cards[i] = RenderCard(hand[i], CardDimmed, theme)
		}
		return joinCards(cards, marginLine, gapLine)
	}

	selectedSet := cardSet(selected)
	legalSet := cardSet(legal)
	hasLegal := len(legal) > 0

	cards := make([]string, len(hand))
	for i, c := range hand {
		state := cardStateFor(i, c, cursor, selectedSet, legalSet, hasLegal)
		cards[i] = RenderCard(c, state, theme)
	}
	return joinCards(cards, marginLine, gapLine)
}

// handGapForWidth returns the gap width between adjacent cards for the given
// terminal width. For widths of 80 or fewer, the constant handGapWidth (1) is
// returned so a 13-card hand fits within 80 columns. For wider terminals, the
// extra space beyond 80 columns is distributed across the 12 gaps of a full
// 13-card hand so the hand spreads out comfortably without truncating cards.
func handGapForWidth(width int) int {
	if width <= 80 {
		return handGapWidth
	}
	extra := width - 80
	gap := handGapWidth + extra/12
	return gap
}

// joinCards arranges bordered card strings side by side horizontally, separated
// by gap on each line, with a leading margin on each line.
func joinCards(cards []string, margin, gap string) string {
	if len(cards) == 0 {
		return ""
	}

	linesPerCard := strings.Split(cards[0], "\n")
	lines := make([]string, len(linesPerCard))
	for lineIdx := range linesPerCard {
		var b strings.Builder
		b.WriteString(margin)
		for i, card := range cards {
			b.WriteString(strings.Split(card, "\n")[lineIdx])
			if i < len(cards)-1 {
				b.WriteString(gap)
			}
		}
		lines[lineIdx] = b.String()
	}
	return strings.Join(lines, "\n")
}

// cardStateFor determines the CardState for a card at index i in the hand.
//
// The cursor highlight composes with other states: if the cursor is on this
// card, CardCursorSelected is returned when the card is also selected,
// CardCursor is returned when the card is legal, and CardCursorDimmed when it
// is not. Otherwise, CardSelected takes priority over CardDimmed, and
// CardDimmed takes priority over CardNormal.
func cardStateFor(
	i int,
	c heartsclient.Card,
	cursor int,
	selectedSet map[heartsclient.Card]bool,
	legalSet map[heartsclient.Card]bool,
	hasLegal bool,
) CardState {
	if i == cursor && cursor >= 0 {
		if selectedSet[c] {
			return CardCursorSelected
		}
		if hasLegal && !legalSet[c] {
			return CardCursorDimmed
		}
		return CardCursor
	}
	if selectedSet[c] {
		return CardSelected
	}
	if hasLegal && !legalSet[c] {
		return CardDimmed
	}
	return CardNormal
}

// suitColor returns the lipgloss terminal color for a card suit, using the
// provided theme.
//
// Hearts and diamonds are red; clubs and spades are dark.
func suitColor(suit string, theme Theme) color.Color {
	switch suit {
	case "hearts", "diamonds":
		return theme.RedSuit
	default:
		return theme.DarkSuit
	}
}

// cardSet builds a set of cards for fast lookup by value (Rank + Suit).
func cardSet(cards []heartsclient.Card) map[heartsclient.Card]bool {
	s := make(map[heartsclient.Card]bool, len(cards))
	for _, c := range cards {
		s[c] = true
	}
	return s
}

// formatPassDirection returns a human-readable label for a pass direction.
func formatPassDirection(dir string) string {
	switch dir {
	case "left":
		return "Pass left"
	case "right":
		return "Pass right"
	case "across":
		return "Pass across"
	case "none":
		return "No pass"
	default:
		return fmt.Sprintf("Pass %s", dir)
	}
}
