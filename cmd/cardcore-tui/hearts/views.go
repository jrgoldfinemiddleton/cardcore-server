package heartstui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// cardBackWidth is the visible width of a single card-back box.
const cardBackWidth = 4

// RenderTrick renders the cards played to the current trick, in play order.
// Each card is preceded by a styled seat label on its own line (e.g.,
// "Seat 2 (You)" followed by the bordered card). An empty trick returns a short
// placeholder: "(no cards played yet)". When highlightSeat is non-negative and
// matches an entry's seat, that card is rendered with the CardWinner state. The
// viewer's seat is marked with "(You)" when viewerSeat is non-negative and
// matches the entry.
func RenderTrick(
	trick []heartsclient.TrickEntry,
	viewerSeat, highlightSeat int,
	theme Theme,
) string {
	if len(trick) == 0 {
		return lipgloss.NewStyle().
			Foreground(theme.Text).
			Background(theme.Background).
			Render("(no cards played yet)")
	}

	lines := make([]string, len(trick))
	for i, entry := range trick {
		state := CardNormal
		if entry.Seat == highlightSeat {
			state = CardWinner
		}
		lines[i] = joinLines([]string{
			seatLabel(entry.Seat, viewerSeat, theme),
			RenderCard(entry.Card, state, theme),
		})
	}
	return joinLines(lines)
}

// RenderPassingView renders the passing phase view for a seated player, using
// the provided theme for colors and scaling the hand to the given terminal
// width.
//
// It shows a header with the round number and pass direction, the player's
// hand (with cursor and selected cards highlighted), and a status line
// indicating how many more cards need to be selected or that Enter can be
// pressed to pass.
//
// Contract: selected must contain at most 3 cards. The caller
// (Client.toggleSelected) enforces this. If >3 cards are passed, the status
// line will show a negative count, which is a debugging signal that the
// caller violated the contract.
func RenderPassingView(
	snap heartsclient.PlayerSnapshot,
	seat, cursor int,
	selected []heartsclient.Card,
	inputDisabled bool,
	theme Theme,
	width, height int,
) string {
	dir := formatPassDirection(snap.PassDirection)
	textStyle := lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Background)
	bgStyle := lipgloss.NewStyle().Background(theme.Background)
	header := textStyle.Render(fmt.Sprintf("Round %d — %s", snap.RoundNumber, dir))
	label := lipgloss.NewStyle().
		Foreground(theme.Text).
		Background(theme.Background).
		Width(width).
		Align(lipgloss.Center).
		Render(fmt.Sprintf("Seat %d (You)", seat))
	hand := RenderHand(snap.Hand, cursor, selected, nil, inputDisabled, theme, width)
	handCentered := lipgloss.Place(width, 3, lipgloss.Center, lipgloss.Center, hand,
		lipgloss.WithWhitespaceStyle(bgStyle))

	remaining := max(3-len(selected), 0)

	var status string
	switch {
	case inputDisabled:
		status = "Waiting for other players…"
	case len(selected) == 3:
		status = "Press Enter to pass"
	default:
		status = fmt.Sprintf("Select %d more card(s) to pass", remaining)
	}

	content := joinLines([]string{header, "", label, handCentered, "", textStyle.Render(status)})
	return placeContent(content, width, height, lipgloss.Bottom, theme)
}

// RenderPlayingView renders the playing phase view for a seated player as a
// cross-shaped table. The viewer is at the bottom, the opposite seat is at the
// top, and the remaining seats are on the left and right. Non-human seats show
// card-back visuals instead of real cards. Played cards are arranged in a
// diamond formation in the center with minimal info text (turn or winner).
// The view is anchored at the bottom so the player's hand and status line stay
// fixed as the trick grows.
func RenderPlayingView(
	snap heartsclient.PlayerSnapshot,
	seat, cursor int,
	inputDisabled bool,
	theme Theme,
	width, height int,
) string {
	const sideBlockWidth = 26
	centerWidth := max(width-2*sideBlockWidth, 0)

	textStyle := lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Background)
	bgStyle := lipgloss.NewStyle().Background(theme.Background)
	spacer := bgStyle.Render(strings.Repeat(" ", width))

	leftSeat := (seat + 1) % 4
	topSeat := (seat + 2) % 4
	rightSeat := (seat + 3) % 4

	// Bottom block: "Seat N (You)" label, hand centered horizontally, and
	// status. Pinned to the bottom of the play area.
	label := lipgloss.NewStyle().
		Foreground(theme.Text).
		Background(theme.Background).
		Width(width).
		Align(lipgloss.Center).
		Render(fmt.Sprintf("Seat %d (You)", seat))
	hand := RenderHand(snap.Hand, cursor, nil, snap.LegalActions, inputDisabled, theme, width)
	handCentered := lipgloss.Place(width, 3, lipgloss.Center, lipgloss.Center, hand,
		lipgloss.WithWhitespaceStyle(bgStyle))
	var status string
	switch {
	case snap.Phase == heartsclient.PhaseTrickComplete && snap.TrickWinner >= 0:
		if snap.TrickWinner == seat {
			status = "You won the trick"
		} else {
			status = fmt.Sprintf("Seat %d won the trick", snap.TrickWinner)
		}
	case inputDisabled:
		status = "Waiting for other players…"
	case snap.Turn == seat:
		status = "Your turn — select a card and press Enter"
	default:
		status = fmt.Sprintf("Waiting for seat %d…", snap.Turn)
	}
	bottomBlock := joinLines([]string{label, handCentered, textStyle.Render(status)})
	bottomHeight := strings.Count(bottomBlock, "\n") + 1

	// Top block: centered label + optional card-backs. Pinned to the top.
	const diamondHeight = 9
	showTopBacks := height >= 1+3+diamondHeight+bottomHeight
	topBlock := renderTopSeat(topSeat, snap.HandCounts, theme, width, showTopBacks)
	topHeight := strings.Count(topBlock, "\n") + 1

	// Middle row: left card-backs | center diamond | right card-backs.
	leftBlock := renderSideSeat(leftSeat, snap.HandCounts, theme, sideBlockWidth, diamondHeight)
	rightBlock := renderSideSeat(rightSeat, snap.HandCounts, theme, sideBlockWidth, diamondHeight)
	centerBlock := renderPlayerDiamond(snap, seat, theme, centerWidth)
	middleRow := lipgloss.JoinHorizontal(lipgloss.Top, leftBlock, centerBlock, rightBlock)

	// Center the trick vertically in the full play area.
	middleArea := lipgloss.Place(width, diamondHeight, lipgloss.Center, lipgloss.Center, middleRow,
		lipgloss.WithWhitespaceStyle(bgStyle))

	// Spacers pin the top and bottom blocks to the edges while centering the
	// trick. If the terminal is too short, the spacers collapse to zero.
	extra := max(height-topHeight-bottomHeight-diamondHeight, 0)
	topSpacer := extra / 2
	bottomSpacer := extra - topSpacer

	parts := make([]string, 0, topHeight+topSpacer+diamondHeight+bottomSpacer+bottomHeight)
	parts = append(parts, topBlock)
	parts = append(parts, repeatLines(spacer, topSpacer)...)
	parts = append(parts, middleArea)
	parts = append(parts, repeatLines(spacer, bottomSpacer)...)
	parts = append(parts, bottomBlock)

	content := joinLines(parts)
	return placeContent(content, width, height, lipgloss.Top, theme)
}

// RenderTrickCompleteView renders the view shown when a trick is complete,
// using the provided theme for colors and sizing the summary box to the given
// terminal width.
//
// It displays the completed trick with seat labels and a status line inside a
// bordered box. The winner is provided by the server in snap.TrickWinner; the
// fallback generic message is used when the trick is not complete or the
// server did not provide a winner.
func RenderTrickCompleteView(
	snap heartsclient.PlayerSnapshot,
	seat int,
	theme Theme,
	width, height int,
) string {
	textStyle := lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Background)
	trick := RenderTrick(snap.Trick, seat, snap.TrickWinner, theme)

	var status string
	if len(snap.Trick) == 4 && snap.TrickWinner >= 0 {
		status = fmt.Sprintf("Trick Completed — Seat %d won", snap.TrickWinner)
	} else {
		status = "Trick Completed"
	}

	content := joinLines([]string{trick, textStyle.Render(status)})
	boxed := summaryBoxStyle(theme, width).Render(content)
	return placeContent(boxed, width, height, lipgloss.Bottom, theme)
}

// RenderRoundCompleteView renders the round scores overlay, using the provided
// theme for colors and sizing the summary box to the given terminal width.
//
// It shows the scores for each seat and the round points accumulated inside a
// bordered box. The viewer's seat is labeled with "(You)". The next snapshot
// (deal/passing) transitions naturally.
func RenderRoundCompleteView(
	snap heartsclient.PlayerSnapshot,
	seat int,
	theme Theme,
	width, height int,
) string {
	if len(snap.RoundPoints) != len(snap.Scores) {
		return "ERROR: Invalid snapshot (score data mismatch)"
	}

	textStyle := lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Background)
	var lines []string
	lines = append(lines, textStyle.Render(fmt.Sprintf("Round %d Completed", snap.RoundNumber)))

	for i := 0; i < len(snap.Scores); i++ {
		label := seatLabel(i, seat, theme)
		rest := textStyle.Render(fmt.Sprintf(": %d (+%d)", snap.Scores[i], snap.RoundPoints[i]))
		lines = append(lines, label+rest)
	}

	moonShooter := moonShotSeat(snap.RoundPoints)
	if moonShooter >= 0 {
		lines = append(lines, "")
		lines = append(lines, textStyle.Render(
			fmt.Sprintf("🐄 Seat %d shot the moon! 🌙", moonShooter),
		))
	}

	boxed := summaryBoxStyle(theme, width).Render(joinLines(lines))
	return placeContent(boxed, width, height, lipgloss.Center, theme)
}

// RenderGameOverView renders the final game-over screen, using the provided
// theme for colors and sizing the summary box to the given terminal width.
//
// It shows the final scores for all seats and a prompt to exit inside a
// bordered box. The viewer's seat is labeled with "(You)".
func RenderGameOverView(
	snap heartsclient.PlayerSnapshot,
	seat int,
	theme Theme,
	width, height int,
) string {
	textStyle := lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Background)
	var lines []string
	lines = append(lines, textStyle.Render("Game Over"))

	for i := 0; i < len(snap.Scores); i++ {
		label := seatLabel(i, seat, theme)
		rest := textStyle.Render(fmt.Sprintf(": %d", snap.Scores[i]))
		lines = append(lines, label+rest)
	}

	lines = append(lines, textStyle.Render("Press Enter to exit"))
	boxed := summaryBoxStyle(theme, width).Render(joinLines(lines))
	return placeContent(boxed, width, height, lipgloss.Center, theme)
}

// RenderPausedView renders the pause overlay, using the provided theme for
// colors and sizing the summary box to the given terminal width.
func RenderPausedView(theme Theme, width, height int) string {
	boxed := summaryBoxStyle(theme, width).Render("Paused — press P to resume")
	return placeContent(boxed, width, height, lipgloss.Center, theme)
}

// RenderDealView renders a brief overlay shown while the deck is being dealt.
func RenderDealView(theme Theme, width, height int) string {
	textStyle := lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Background)
	return placeContent(textStyle.Render("Dealing..."), width, height, lipgloss.Center, theme)
}

// PrettyPhase converts a raw snake_case phase string to a human-readable
// display name. Unknown phases are returned as-is.
func PrettyPhase(phase string) string {
	switch phase {
	case heartsclient.PhaseDeal:
		return "Dealing"
	case heartsclient.PhasePassing:
		return "Passing"
	case heartsclient.PhasePlaying:
		return "Playing"
	case heartsclient.PhaseTrickComplete:
		return "Trick Completed"
	case heartsclient.PhaseRoundComplete:
		return "Round Completed"
	case heartsclient.PhaseGameOver:
		return "Game Over"
	case heartsclient.PhasePaused:
		return "Paused"
	default:
		return phase
	}
}

// moonShotSeat returns the seat that shot the moon, or -1 if no moon shot was
// detected. A moon shot is detected during round_complete when exactly one seat
// has a delta of 0 and the other three seats have a delta of 26.
func moonShotSeat(roundPoints []int) int {
	if len(roundPoints) != 4 {
		return -1
	}
	shooter := -1
	for i, pts := range roundPoints {
		switch pts {
		case 0:
			if shooter >= 0 {
				return -1
			}
			shooter = i
		case 26:
		default:
			return -1
		}
	}
	return shooter
}

// repeatLines returns a new slice containing s repeated n times.
func repeatLines(s string, n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = s
	}
	return lines
}

// renderCardBack renders a single card-back: a small bordered box with a
// dimmed fill pattern, matching the height of a normal card (3 lines).
func renderCardBack(theme Theme) string {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.Dimmed).
		BorderBackground(theme.Background).
		Foreground(theme.Dimmed).
		Background(theme.Background).
		Render("▓▓")
}

// renderCardBackRow renders a horizontal row of card-backs, clipped to the
// given maximum width. Returns an empty string when count is zero or negative.
func renderCardBackRow(count int, theme Theme, maxWidth int) string {
	if count <= 0 {
		return ""
	}
	back := renderCardBack(theme)
	const gap = 1
	maxCards := (maxWidth + gap) / (cardBackWidth + gap)
	if maxCards < 1 {
		maxCards = 1
	}
	if count > maxCards {
		count = maxCards
	}
	cards := make([]string, count)
	for i := range cards {
		cards[i] = back
	}
	gapLine := strings.Repeat(" ", gap)
	margin := gapString(theme)
	return joinCards(cards, margin, gapLine)
}

// renderCardBackColumn renders a vertical column of card-backs, clipped to the
// given maximum height. Each card-back is 3 lines tall. Returns an empty
// string when count is zero or negative.
func renderCardBackColumn(count int, theme Theme, width, maxHeight int) string {
	if count <= 0 {
		return ""
	}
	back := renderCardBack(theme)
	const cardHeight = 3
	maxCards := maxHeight / cardHeight
	if maxCards < 1 {
		maxCards = 1
	}
	if count > maxCards {
		count = maxCards
	}
	bgStyle := lipgloss.NewStyle().Background(theme.Background)
	lines := make([]string, count)
	for i := range lines {
		lines[i] = lipgloss.Place(width, cardHeight, lipgloss.Center, lipgloss.Center, back,
			lipgloss.WithWhitespaceStyle(bgStyle))
	}
	return joinLines(lines)
}

// renderPlayerDiamond renders the trick cards in a diamond/plus-sign formation
// for the player view. The top seat's card is at top, the viewer's card at
// bottom, and the left/right seats' cards on the sides. The center contains
// minimal info text (whose turn it is).
func renderPlayerDiamond(
	snap heartsclient.PlayerSnapshot,
	seat int,
	theme Theme,
	width int,
) string {
	bgStyle := lipgloss.NewStyle().Background(theme.Background)
	textStyle := lipgloss.NewStyle().Foreground(theme.Text).Background(theme.Background)

	leftSeat := (seat + 1) % 4
	topSeat := (seat + 2) % 4
	rightSeat := (seat + 3) % 4

	const cardW = 5

	topCard := trickCardForSeat(snap.Trick, topSeat, theme, width)
	bottomCard := trickCardForSeat(snap.Trick, seat, theme, width)
	leftCard := trickCardForSeat(snap.Trick, leftSeat, theme, cardW)
	rightCard := trickCardForSeat(snap.Trick, rightSeat, theme, cardW)

	var infoText string
	if snap.Phase == heartsclient.PhaseTrickComplete && snap.TrickWinner >= 0 {
		if snap.TrickWinner == seat {
			infoText = "You won"
		} else {
			infoText = fmt.Sprintf("Seat %d won", snap.TrickWinner)
		}
	} else if snap.Turn == seat {
		infoText = "Your turn"
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

// summaryBoxStyle returns the bordered container style for pause/summary views,
// using the provided theme for the border and background colors and sized to
// the given terminal width.
func summaryBoxStyle(theme Theme, width int) lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.PanelBorder).
		BorderBackground(theme.Background).
		Foreground(theme.Text).
		Background(theme.Background).
		Padding(0, 1).
		Width(width)
}

// joinLines joins lines with newlines for multi-line view output.
func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}

// placeContent fills a width×height box with the theme background and places
// content inside it at the requested vertical position.
func placeContent(content string, width, height int, vPos lipgloss.Position, theme Theme) string {
	bgStyle := lipgloss.NewStyle().Background(theme.Background)
	content = clipLines(content, height, vPos)
	return lipgloss.Place(
		width, height,
		lipgloss.Left, vPos,
		content,
		lipgloss.WithWhitespaceStyle(bgStyle),
	)
}

// clipLines returns at most height lines of s, choosing the slice based on the
// requested vertical position so the content that matters stays visible.
func clipLines(s string, height int, vPos lipgloss.Position) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= height {
		return s
	}
	switch vPos {
	case lipgloss.Top:
		return strings.Join(lines[:height], "\n")
	case lipgloss.Bottom:
		return strings.Join(lines[len(lines)-height:], "\n")
	default:
		// Center alignment: keep the middle portion.
		start := (len(lines) - height) / 2
		return strings.Join(lines[start:start+height], "\n")
	}
}

// seatLabel returns a styled "Seat N" label, appending "(You)" when the seat
// belongs to the viewer.
func seatLabel(seat, viewerSeat int, theme Theme) string {
	label := fmt.Sprintf("Seat %d", seat)
	if seat == viewerSeat {
		label += " (You)"
	}
	return lipgloss.NewStyle().
		Foreground(theme.Text).
		Background(theme.Background).
		Bold(true).
		Render(label)
}

// safeHandCount returns the hand count for the given seat, or 0 if the seat
// index is out of range.
func safeHandCount(counts []int, seat int) int {
	if seat < 0 || seat >= len(counts) {
		return 0
	}
	return counts[seat]
}

// trickCardForSeat returns the rendered card for the given seat in the trick,
// or an invisible placeholder of the requested size if the seat has not played
// a card yet.
func trickCardForSeat(trick []heartsclient.TrickEntry, seat int, theme Theme, width int) string {
	bgStyle := lipgloss.NewStyle().Background(theme.Background)
	for _, entry := range trick {
		if entry.Seat == seat {
			return lipgloss.Place(width, 3, lipgloss.Center, lipgloss.Center,
				RenderCard(entry.Card, CardNormal, theme),
				lipgloss.WithWhitespaceStyle(bgStyle))
		}
	}
	return lipgloss.Place(width, 3, lipgloss.Center, lipgloss.Center, "",
		lipgloss.WithWhitespaceStyle(bgStyle))
}

// renderSideSeat renders a side seat block (left or right) with a seat label
// and a vertical column of card-backs representing the hidden hand. The block
// is sized to fill the given width and height.
func renderSideSeat(
	seat int,
	counts []int,
	theme Theme,
	width, height int,
) string {
	count := safeHandCount(counts, seat)
	labelText := fmt.Sprintf("Seat %d — %d cards", seat, count)
	labelLine := lipgloss.NewStyle().
		Foreground(theme.Text).
		Background(theme.Background).
		Width(width).
		Align(lipgloss.Center).
		Render(labelText)
	backsHeight := max(height-1, 0)
	backs := renderCardBackColumn(count, theme, width, backsHeight)
	box := lipgloss.JoinVertical(lipgloss.Left, labelLine, backs)
	bgStyle := lipgloss.NewStyle().Background(theme.Background)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, box,
		lipgloss.WithWhitespaceStyle(bgStyle))
}

// renderTopSeat renders the opposite seat block at the top of the cross
// layout. When showBacks is true, a horizontal row of card-backs is shown
// below the label; otherwise only the label is rendered.
func renderTopSeat(
	seat int,
	counts []int,
	theme Theme,
	width int,
	showBacks bool,
) string {
	count := safeHandCount(counts, seat)
	labelText := fmt.Sprintf("Seat %d — %d cards", seat, count)
	labelLine := lipgloss.NewStyle().
		Foreground(theme.Text).
		Background(theme.Background).
		Width(width).
		Align(lipgloss.Center).
		Render(labelText)
	if !showBacks {
		return labelLine
	}
	backs := renderCardBackRow(count, theme, width)
	bgStyle := lipgloss.NewStyle().Background(theme.Background)
	backsBlock := lipgloss.Place(width, 3, lipgloss.Center, lipgloss.Center, backs,
		lipgloss.WithWhitespaceStyle(bgStyle))
	return lipgloss.JoinVertical(lipgloss.Left, labelLine, backsBlock)
}

// gapString returns a horizontal gap string of the given width using the theme
// background color.
func gapString(theme Theme) string {
	return lipgloss.NewStyle().Background(theme.Background).Width(1).Render("")
}
