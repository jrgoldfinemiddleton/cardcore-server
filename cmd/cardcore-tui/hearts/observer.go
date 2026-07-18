package heartstui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// RenderObserverView renders the full observer view as a square table: the top
// and bottom hands are shown as centered horizontal boxes with seat labels
// above them, the left and right hands as vertical boxes sorted by suit, and
// the current trick is arranged in a diamond formation in the center with
// minimal status info (turn or winner only).
//
// Seat 0 is at top, seat 1 at right, seat 2 at bottom, seat 3 at left.
//
// It does not panic when Hands has fewer than 4 entries — it treats missing
// seats as empty hands.
func RenderObserverView(snap heartsclient.ObserverSnapshot, theme Theme, width, height int) string {
	const sideBlockWidth = 26
	centerWidth := max(width-2*sideBlockWidth, 0)

	// Top: Seat 0 label + hand centered across full width.
	topLabel := centeredSeatLabel(0, width, theme)
	topHand := centeredHandBlock(safeHand(snap.Hands, 0), theme, width)

	// Middle: left box (Seat 3) | center diamond | right box (Seat 1).
	middleHeight := max(height-8, 0)
	leftBox := renderVerticalHandBox(
		safeHand(snap.Hands, 3), 3, theme, sideBlockWidth, middleHeight, lipgloss.Left)
	rightBox := renderVerticalHandBox(
		safeHand(snap.Hands, 1), 1, theme, sideBlockWidth, middleHeight, lipgloss.Right)
	centerBox := renderObserverCenter(snap, theme, centerWidth, middleHeight)
	middleRow := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, centerBox, rightBox)

	// Bottom: Seat 2 label + hand centered across full width.
	bottomLabel := centeredSeatLabel(2, width, theme)
	bottomHand := centeredHandBlock(safeHand(snap.Hands, 2), theme, width)

	content := joinLines([]string{topLabel, topHand, middleRow, bottomLabel, bottomHand})
	return placeContent(content, width, height, lipgloss.Center, theme)
}

// safeHand returns the hand for the given seat, or nil if the seat index is
// out of range.
func safeHand(hands [][]heartsclient.Card, seat int) []heartsclient.Card {
	if seat < 0 || seat >= len(hands) {
		return nil
	}
	return hands[seat]
}

// maxLineWidth returns the maximum visual width of any line in the given
// multi-line string, accounting for ANSI escape sequences.
func maxLineWidth(s string) int {
	max := 0
	for _, line := range strings.Split(s, "\n") {
		if w := lipgloss.Width(line); w > max {
			max = w
		}
	}
	return max
}

// centeredSeatLabel renders a "Seat N" label centered across the given width.
func centeredSeatLabel(seat int, width int, theme Theme) string {
	return lipgloss.NewStyle().
		Foreground(theme.Text).
		Background(theme.Background).
		Width(width).
		Align(lipgloss.Center).
		Render(fmt.Sprintf("Seat %d", seat))
}

// centeredHandBlock renders a hand as a 3-line block centered horizontally
// within the given width.
func centeredHandBlock(hand []heartsclient.Card, theme Theme, width int) string {
	bgStyle := lipgloss.NewStyle().Background(theme.Background)
	rendered := RenderHand(hand, -1, nil, nil, false, theme, width)
	return lipgloss.Place(width, 3, lipgloss.Center, lipgloss.Center, rendered,
		lipgloss.WithWhitespaceStyle(bgStyle))
}

// renderVerticalHandBox renders a vertical hand box for the observer square
// layout. The seat label is centered at the top and the hand content fills the
// remaining height, aligned to hAlign (left for the left seat, right for the
// right seat).
func renderVerticalHandBox(
	hand []heartsclient.Card,
	seat int,
	theme Theme,
	width, height int,
	hAlign lipgloss.Position,
) string {
	bgStyle := lipgloss.NewStyle().Background(theme.Background)
	handContent := renderVerticalHand(hand, theme, max(height-1, 0))
	handContentHeight := strings.Count(handContent, "\n") + 1

	var handBlock string
	if handContent == "" {
		label := lipgloss.NewStyle().
			Foreground(theme.Text).
			Background(theme.Background).
			Width(width).
			Align(lipgloss.Center).
			Render(fmt.Sprintf("Seat %d", seat))
		handBlock = lipgloss.JoinVertical(lipgloss.Left, label,
			lipgloss.Place(width, handContentHeight, hAlign, lipgloss.Top, "",
				lipgloss.WithWhitespaceStyle(bgStyle)))
	} else {
		handContentWidth := maxLineWidth(handContent)
		labelText := fmt.Sprintf("Seat %d", seat)
		labelWidth := max(handContentWidth, lipgloss.Width(labelText))
		label := lipgloss.NewStyle().
			Foreground(theme.Text).
			Background(theme.Background).
			Width(labelWidth).
			Align(lipgloss.Center).
			Render(labelText)
		block := lipgloss.JoinVertical(lipgloss.Left, label, handContent)
		handBlock = lipgloss.Place(width, handContentHeight+1, hAlign, lipgloss.Top, block,
			lipgloss.WithWhitespaceStyle(bgStyle))
	}

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, handBlock,
		lipgloss.WithWhitespaceStyle(bgStyle))
}

// renderVerticalHand renders a hand vertically inside a box of the given width
// and height. Cards are sorted by suit (clubs, diamonds, hearts, spades) and
// rank, then laid out in rows of four cards each; any remaining cards go in the
// bottom row. The result is clipped to the available height.
func renderVerticalHand(hand []heartsclient.Card, theme Theme, height int) string {
	if len(hand) == 0 {
		return ""
	}
	sorted := sortBySuitOrder(hand)
	rows := renderCardRows(sorted, theme)
	if len(rows)*3 <= height {
		return joinLines(rows)
	}
	maxRows := height / 3
	if maxRows > 0 && maxRows <= len(rows) {
		return joinLines(rows[:maxRows])
	}
	return ""
}

// sortBySuitOrder returns the hand sorted by the cardcore engine suit order
// (clubs, diamonds, hearts, spades) and rank within each suit.
func sortBySuitOrder(hand []heartsclient.Card) []heartsclient.Card {
	suits := groupAndSortHand(hand)
	suitOrder := []string{suitClubs, suitDiamonds, suitHearts, suitSpades}
	sorted := make([]heartsclient.Card, 0, len(hand))
	for _, suit := range suitOrder {
		sorted = append(sorted, suits[suit]...)
	}
	return sorted
}

// groupAndSortHand groups cards by suit and sorts each suit by rank.
func groupAndSortHand(hand []heartsclient.Card) map[string][]heartsclient.Card {
	suitOrder := []string{suitClubs, suitDiamonds, suitHearts, suitSpades}
	result := make(map[string][]heartsclient.Card, len(suitOrder))
	for _, suit := range suitOrder {
		for _, c := range hand {
			if c.Suit == suit {
				result[suit] = append(result[suit], c)
			}
		}
		sort.Slice(result[suit], func(i, j int) bool {
			return rankValue(result[suit][i].Rank) < rankValue(result[suit][j].Rank)
		})
	}
	return result
}

// renderCardRows splits cards into groups of five and renders each group as a
// horizontal row of touching cards. The remaining cards (fewer than five) go in
// the bottom row.
func renderCardRows(cards []heartsclient.Card, theme Theme) []string {
	const n = 5
	rows := make([]string, 0, (len(cards)+n-1)/n)
	for i := 0; i < len(cards); i += n {
		end := min(i+n, len(cards))
		group := cards[i:end]
		rendered := make([]string, len(group))
		for j, c := range group {
			rendered[j] = RenderCard(c, CardNormal, theme)
		}
		rows = append(rows, joinCards(rendered, "", ""))
	}
	return rows
}

// rankValue returns a numeric value for a rank string, used for sorting.
func rankValue(rank string) int {
	switch rank {
	case "two":
		return 2
	case "three":
		return 3
	case "four":
		return 4
	case "five":
		return 5
	case "six":
		return 6
	case "seven":
		return 7
	case "eight":
		return 8
	case "nine":
		return 9
	case rankTen:
		return 10
	case "jack":
		return 11
	case "queen":
		return 12
	case "king":
		return 13
	case "ace":
		return 14
	default:
		return 0
	}
}

// renderObserverCenter renders the central information area for the observer
// square layout: the trick in a diamond formation with minimal info text.
//
// The diamond arranges trick cards as:
//
//	       <S0 card>
//	<S3 card>  <info>  <S1 card>
//	       <S2 card>
//
// The info text is minimal: whose turn it is, or "Seat N won the trick"
// during trick_complete.
func renderObserverCenter(
	snap heartsclient.ObserverSnapshot,
	theme Theme,
	width, height int,
) string {
	bgStyle := lipgloss.NewStyle().Background(theme.Background)

	diamond := renderObserverDiamond(snap, theme, width)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, diamond,
		lipgloss.WithWhitespaceStyle(bgStyle))
}

// renderObserverDiamond renders the trick cards in a diamond/plus-sign
// formation for the observer view. Seat 0 is top, seat 1 is right, seat 2 is
// bottom, seat 3 is left. The center contains minimal info text (turn or
// winner).
func renderObserverDiamond(snap heartsclient.ObserverSnapshot, theme Theme, width int) string {
	bgStyle := lipgloss.NewStyle().Background(theme.Background)
	textStyle := lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Background)

	const cardW = 5

	winnerSeat := -1
	if snap.Phase == heartsclient.PhaseTrickComplete {
		winnerSeat = snap.TrickWinner
	}

	topCard := observerTrickCard(snap.Trick, 0, winnerSeat, theme, width)
	bottomCard := observerTrickCard(snap.Trick, 2, winnerSeat, theme, width)
	leftCard := observerTrickCard(snap.Trick, 3, winnerSeat, theme, cardW)
	rightCard := observerTrickCard(snap.Trick, 1, winnerSeat, theme, cardW)

	var infoText string
	if snap.Phase == heartsclient.PhaseTrickComplete && snap.TrickWinner >= 0 {
		infoText = fmt.Sprintf("Seat %d won", snap.TrickWinner)
	} else {
		infoText = fmt.Sprintf("Seat %d's turn", snap.Turn)
	}

	gap := gapString(theme)
	infoSlot := textStyle.Render(infoText)
	middleContent := lipgloss.JoinHorizontal(
		lipgloss.Center, leftCard, gap, infoSlot, gap, rightCard)
	middleRow := lipgloss.Place(width, 3, lipgloss.Center, lipgloss.Center, middleContent,
		lipgloss.WithWhitespaceStyle(bgStyle))

	return joinLines([]string{topCard, middleRow, bottomCard})
}

// observerTrickCard renders a single trick card for the given seat in a
// fixed-width 3-line box. If winnerSeat is non-negative and matches the seat,
// the card is rendered with the CardWinner state. An empty placeholder box is
// returned when the seat has not played a card.
func observerTrickCard(
	trick []heartsclient.TrickEntry,
	seat, winnerSeat int,
	theme Theme,
	width int,
) string {
	bgStyle := lipgloss.NewStyle().Background(theme.Background)
	for _, entry := range trick {
		if entry.Seat == seat {
			state := CardNormal
			if winnerSeat >= 0 && entry.Seat == winnerSeat {
				state = CardWinner
			}
			return lipgloss.Place(width, 3, lipgloss.Center, lipgloss.Center,
				RenderCard(entry.Card, state, theme),
				lipgloss.WithWhitespaceStyle(bgStyle))
		}
	}
	return lipgloss.Place(width, 3, lipgloss.Center, lipgloss.Center, "",
		lipgloss.WithWhitespaceStyle(bgStyle))
}
