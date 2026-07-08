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
	// CardSelected means the card is selected (e.g., chosen to pass).
	CardSelected
	// CardDimmed means the card is not currently legal or selectable.
	CardDimmed
)

// redSuitHex is the hex color for hearts and diamonds on the dark background.
const redSuitHex = "#E94560"

// lightSuitHex is the hex color for clubs and spades on the dark background.
const lightSuitHex = "#FAFAFA"

// dimColorHex is the muted hex color for dimmed cards.
const dimColorHex = "#555555"

// handBgHex is the dark background color for the hand line. It matches
// layoutStyle's background so cards and separator spaces render with a
// consistent fill instead of a patchy appearance.
const handBgHex = "#1A1A2E"

// selectedMarker is the checkmark appended to selected cards.
const selectedMarker = "✓"

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

// RenderCard returns a styled string for a single card with the given visual
// state. Hearts and diamonds are colored red; clubs and spades are colored
// light gray/white. CardCursor adds brackets and bold. CardSelected appends a
// checkmark. CardDimmed renders the card in a muted gray.
func RenderCard(c heartsclient.Card, state CardState) string {
	label := CardLabel(c)
	color := suitColor(c.Suit)
	bg := lipgloss.Color(handBgHex)

	switch state {
	case CardDimmed:
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(dimColorHex)).
			Background(bg).
			Render(label)
	case CardCursor:
		return lipgloss.NewStyle().
			Foreground(color).
			Background(bg).
			Bold(true).
			Render("[" + label + "]")
	case CardSelected:
		return lipgloss.NewStyle().
			Foreground(color).
			Background(bg).
			Render(label + selectedMarker)
	default:
		return lipgloss.NewStyle().
			Foreground(color).
			Background(bg).
			Render(label)
	}
}

// RenderHand renders a hand of cards as a horizontal spread separated by
// spaces, fitting within 80 columns.
//
// The cursor index highlights the card under the cursor. If cursor is negative
// or out of range, no cursor highlight is drawn. A card whose value is in
// selected gets the selected decoration. If legal is non-empty, any card NOT
// in legal gets CardDimmed; if legal is nil/empty, nothing is dimmed. The
// cursor highlight composes with and is visible over other states. Card
// equality is by value (Rank AND Suit both match).
func RenderHand(
	hand []heartsclient.Card,
	cursor int,
	selected []heartsclient.Card,
	legal []heartsclient.Card,
	inputDisabled bool,
) string {
	if len(hand) == 0 {
		return ""
	}

	// If input is disabled (timeout), render all cards as dimmed to reflect
	// that no user interaction is allowed.
	if inputDisabled {
		if len(hand) == 0 {
			return ""
		}
		parts := make([]string, len(hand))
		sep := lipgloss.NewStyle().Background(lipgloss.Color(handBgHex)).Render(" ")
		for i := range hand {
			parts[i] = RenderCard(hand[i], CardDimmed)
		}
		return strings.Join(parts, sep)
	}

	selectedSet := cardSet(selected)
	legalSet := cardSet(legal)
	hasLegal := len(legal) > 0

	parts := make([]string, len(hand))
	for i, c := range hand {
		state := cardStateFor(i, c, cursor, selectedSet, legalSet, hasLegal)
		parts[i] = RenderCard(c, state)
	}

	sep := lipgloss.NewStyle().Background(lipgloss.Color(handBgHex)).Render(" ")
	return strings.Join(parts, sep)
}

// cardStateFor determines the CardState for a card at index i in the hand.
//
// The cursor highlight composes with other states: if the cursor is on this
// card, CardCursor is returned regardless of selected/dimmed status.
// Otherwise, CardSelected takes priority over CardDimmed, and CardDimmed
// takes priority over CardNormal.
func cardStateFor(
	i int,
	c heartsclient.Card,
	cursor int,
	selectedSet map[heartsclient.Card]bool,
	legalSet map[heartsclient.Card]bool,
	hasLegal bool,
) CardState {
	if i == cursor && cursor >= 0 {
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

// suitColor returns the lipgloss terminal color for a card suit.
//
// Hearts and diamonds are red (#E94560); clubs and spades are light (#FAFAFA).
func suitColor(suit string) color.Color {
	switch suit {
	case "hearts", "diamonds":
		return lipgloss.Color(redSuitHex)
	default:
		return lipgloss.Color(lightSuitHex)
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
