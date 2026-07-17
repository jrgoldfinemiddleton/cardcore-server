package heartstui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

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

	got := RenderPassingView(snap, 0, selected, false, NewDarkTheme(), 80, 14)
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

	got := RenderPassingView(snap, 0, selected, false, NewDarkTheme(), 80, 14)
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

	got := RenderPlayingView(snap, 0, 0, false, NewDarkTheme(), 80, 14)
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

	got := RenderPlayingView(snap, 0, 0, false, NewDarkTheme(), 80, 14)
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

	got := RenderTrick(trick, -1, -1, NewDarkTheme())
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
	got := RenderTrick([]heartsclient.TrickEntry{}, -1, -1, NewDarkTheme())
	want := "(no cards played yet)"
	if stripANSI(got) != want {
		t.Errorf("RenderTrick(empty) = %q, want %q", got, want)
	}
}

// TestRenderTrickHighlightsWinner verifies the winning card is styled
// differently from the other cards in the trick.
func TestRenderTrickHighlightsWinner(t *testing.T) {
	trick := []heartsclient.TrickEntry{
		{Seat: 2, Card: heartsclient.Card{Rank: "five", Suit: "diamonds"}},
		{Seat: 3, Card: heartsclient.Card{Rank: "king", Suit: "diamonds"}},
	}

	got := RenderTrick(trick, -1, 3, NewDarkTheme())
	if !strings.Contains(got, "Seat 3") {
		t.Errorf("RenderTrick = %q, want to contain %q", got, "Seat 3")
	}
	plain := RenderTrick(trick, -1, -1, NewDarkTheme())
	if got == plain {
		t.Errorf("RenderTrick with winner should differ from plain RenderTrick")
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
		Turn:        3,
		TrickWinner: 1,
	}
	got := RenderTrickCompleteView(snap, 0, NewDarkTheme(), 80, 14)
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
	got := RenderTrickCompleteView(snap, 0, NewDarkTheme(), 80, 14)
	if strings.Contains(got, "won") {
		t.Errorf("RenderTrickCompleteView = %q, should not claim winner for incomplete trick", got)
	}
	if !strings.Contains(got, "Trick complete") {
		t.Errorf("RenderTrickCompleteView = %q, want to contain 'Trick complete'", got)
	}
}

// TestRenderRoundCompleteView verifies the round-complete view shows scores
// inside a bordered box.
func TestRenderRoundCompleteView(t *testing.T) {
	snap := heartsclient.PlayerSnapshot{
		RoundNumber: 1,
		Scores:      []int{13, 0, 13, 0},
		RoundPoints: []int{11, 0, 0, 0},
	}
	got := RenderRoundCompleteView(snap, 0, NewDarkTheme(), 80, 14)
	if !strings.Contains(got, "Round 1 complete") {
		t.Errorf("RenderRoundCompleteView = %q, want 'Round 1 complete'", got)
	}
	if !strings.Contains(stripANSI(got), "Seat 0 (You): 13 (+11)") {
		t.Errorf("RenderRoundCompleteView = %q, want Seat 0 score", got)
	}
	plain := "Round 1 complete\n" +
		"Seat 0 (You): 13 (+11)\n" +
		"Seat 1: 0 (+0)\n" +
		"Seat 2: 13 (+0)\n" +
		"Seat 3: 0 (+0)"
	if got == plain {
		t.Errorf("RenderRoundCompleteView should add a border around the content")
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
	got := RenderRoundCompleteView(snap, 0, NewDarkTheme(), 80, 14)
	if !strings.Contains(got, "ERROR") {
		t.Errorf("RenderRoundCompleteView = %q, want to contain 'ERROR'", got)
	}
}

// TestRenderGameOverView verifies the game-over view shows final scores and
// an exit prompt inside a bordered box.
func TestRenderGameOverView(t *testing.T) {
	snap := heartsclient.PlayerSnapshot{
		Scores: []int{26, 0, 0, 0},
	}
	got := RenderGameOverView(snap, 0, NewDarkTheme(), 80, 14)
	if !strings.Contains(got, "Game Over") {
		t.Errorf("RenderGameOverView = %q, want 'Game Over'", got)
	}
	if !strings.Contains(stripANSI(got), "Seat 0 (You): 26") {
		t.Errorf("RenderGameOverView = %q, want Seat 0 score", got)
	}
	if !strings.Contains(got, "Press Enter to exit") {
		t.Errorf("RenderGameOverView = %q, want exit prompt", got)
	}
	plain := "Game Over\nSeat 0 (You): 26\nSeat 1: 0\nSeat 2: 0\nSeat 3: 0\nPress Enter to exit"
	if got == plain {
		t.Errorf("RenderGameOverView should add a border around the content")
	}
}

// TestRenderPassingViewInputDisabled verifies the status line when input is
// disabled after the player has submitted.
func TestRenderPassingViewInputDisabled(t *testing.T) {
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

	got := RenderPassingView(snap, 0, selected, true, NewDarkTheme(), 80, 14)
	want := "Waiting for other players"
	if !strings.Contains(got, want) {
		t.Errorf("RenderPassingView(inputDisabled) = %q, want to contain %q", got, want)
	}
}

// TestRenderPlayingViewInputDisabled verifies the status line when input is
// disabled after the player has played a card.
func TestRenderPlayingViewInputDisabled(t *testing.T) {
	snap := heartsclient.PlayerSnapshot{
		Phase:        "playing",
		Turn:         0,
		Hand:         []heartsclient.Card{{Rank: "ace", Suit: "spades"}},
		Trick:        []heartsclient.TrickEntry{},
		LegalActions: []heartsclient.Card{{Rank: "ace", Suit: "spades"}},
	}

	got := RenderPlayingView(snap, 0, 0, true, NewDarkTheme(), 80, 14)
	want := "Waiting for other players"
	if !strings.Contains(got, want) {
		t.Errorf("RenderPlayingView(inputDisabled) = %q, want to contain %q", got, want)
	}
}

// TestRenderPassingViewBlankLines verifies blank lines between the header,
// hand, and status sections. The bordered hand is rendered as a 3-line block,
// so the blank separator lines are at positions 1 and 5.
func TestRenderPassingViewBlankLines(t *testing.T) {
	snap := heartsclient.PlayerSnapshot{
		RoundNumber:   1,
		PassDirection: "left",
		Hand:          []heartsclient.Card{{Rank: "ace", Suit: "spades"}},
	}
	got := RenderPassingView(snap, -1, nil, false, NewDarkTheme(), 80, 14)
	lines := strings.Split(got, "\n")
	if len(lines) != 14 {
		t.Fatalf("RenderPassingView has %d lines, want 14", len(lines))
	}
	contentLines := lines[len(lines)-7:]
	blankOne := strings.TrimSpace(stripANSI(contentLines[1])) == ""
	blankFive := strings.TrimSpace(stripANSI(contentLines[5])) == ""
	if !blankOne || !blankFive {
		t.Errorf("RenderPassingView blank lines missing: %q", got)
	}
}

// TestRenderPlayingViewBlankLines verifies blank lines between the trick,
// hand, and status sections. The bordered hand is rendered as a 3-line block,
// so the blank separator lines are at positions 1 and 5.
func TestRenderPlayingViewBlankLines(t *testing.T) {
	snap := heartsclient.PlayerSnapshot{
		Phase:        "playing",
		Turn:         0,
		Hand:         []heartsclient.Card{{Rank: "ace", Suit: "spades"}},
		Trick:        []heartsclient.TrickEntry{},
		LegalActions: []heartsclient.Card{{Rank: "ace", Suit: "spades"}},
	}
	got := RenderPlayingView(snap, 0, 0, false, NewDarkTheme(), 80, 14)
	lines := strings.Split(got, "\n")
	if len(lines) != 14 {
		t.Fatalf("RenderPlayingView has %d lines, want 14", len(lines))
	}
	contentLines := lines[len(lines)-7:]
	blankOne := strings.TrimSpace(stripANSI(contentLines[1])) == ""
	blankFive := strings.TrimSpace(stripANSI(contentLines[5])) == ""
	if !blankOne || !blankFive {
		t.Errorf("RenderPlayingView blank lines missing: %q", got)
	}
}

// TestRenderPlayingViewHandAnchoredAtBottom verifies the status line (and the
// hand above it) stays at the same bottom-relative position as the trick grows.
func TestRenderPlayingViewHandAnchoredAtBottom(t *testing.T) {
	theme := NewDarkTheme()
	width := 80
	height := 10

	baseSnap := heartsclient.PlayerSnapshot{
		Phase:        "playing",
		Turn:         0,
		Hand:         []heartsclient.Card{{Rank: "ace", Suit: "spades"}},
		LegalActions: []heartsclient.Card{{Rank: "ace", Suit: "spades"}},
	}

	emptyTrick := baseSnap
	emptyTrick.Trick = []heartsclient.TrickEntry{}

	oneCardTrick := baseSnap
	oneCardTrick.Trick = []heartsclient.TrickEntry{
		{Seat: 0, Card: heartsclient.Card{Rank: "two", Suit: "clubs"}},
	}

	emptyGot := RenderPlayingView(emptyTrick, 0, 0, false, theme, width, height)
	oneGot := RenderPlayingView(oneCardTrick, 0, 0, false, theme, width, height)

	emptyLines := strings.Split(emptyGot, "\n")
	oneLines := strings.Split(oneGot, "\n")

	if len(emptyLines) != height {
		t.Fatalf("empty trick height = %d, want %d", len(emptyLines), height)
	}
	if len(oneLines) != height {
		t.Fatalf("one-card trick height = %d, want %d", len(oneLines), height)
	}

	emptyStatus := stripANSI(emptyLines[len(emptyLines)-1])
	oneStatus := stripANSI(oneLines[len(oneLines)-1])
	if emptyStatus != oneStatus {
		t.Errorf("status line differs: empty=%q, one=%q", emptyStatus, oneStatus)
	}
}

// TestPlaceContentClipsTallContent verifies that placeContent truncates
// content taller than the requested height to keep the layout from overflowing.
func TestPlaceContentClipsTallContent(t *testing.T) {
	theme := NewDarkTheme()
	content := strings.Repeat("line\n", 20)
	got := placeContent(content, 80, 10, lipgloss.Bottom, theme)
	lines := strings.Split(got, "\n")
	if len(lines) != 10 {
		t.Errorf("placeContent produced %d lines, want 10", len(lines))
	}
}

// TestRenderPausedView verifies the paused view shows the paused message and
// resume prompt.
func TestRenderPausedView(t *testing.T) {
	got := RenderPausedView(NewDarkTheme(), 80, 14)
	if !strings.Contains(got, "paused") {
		t.Errorf("RenderPausedView = %q, want to contain 'paused'", got)
	}
	if !strings.Contains(got, "resume") {
		t.Errorf("RenderPausedView = %q, want resume prompt", got)
	}
}

// TestRenderPausedViewBordered verifies the paused view is wrapped in a bordered
// box.
func TestRenderPausedViewBordered(t *testing.T) {
	got := RenderPausedView(NewDarkTheme(), 80, 14)
	plain := "Game paused — press P to resume"
	if got == plain {
		t.Errorf("RenderPausedView should add a border around the content")
	}
}

// TestRenderTrickCompleteViewBordered verifies the trick-complete view is
// wrapped in a bordered box.
func TestRenderTrickCompleteViewBordered(t *testing.T) {
	snap := heartsclient.PlayerSnapshot{
		Trick: []heartsclient.TrickEntry{
			{Seat: 0, Card: heartsclient.Card{Rank: "two", Suit: "clubs"}},
		},
	}
	got := RenderTrickCompleteView(snap, 0, NewDarkTheme(), 80, 14)
	if !strings.Contains(stripANSI(got), "Seat 0") {
		t.Errorf("RenderTrickCompleteView = %q, want to contain 'Seat 0'", got)
	}
	if !strings.Contains(stripANSI(got), "Trick complete") {
		t.Errorf("RenderTrickCompleteView = %q, want to contain 'Trick complete'", got)
	}
	// The box adds border characters to the output.
	if got == RenderTrick(snap.Trick, -1, -1, NewDarkTheme())+"\nTrick complete" {
		t.Errorf("RenderTrickCompleteView should add a border around the content")
	}
}

// TestRenderPausedViewLightTheme verifies the paused view renders with the
// light theme and produces the same visible text but different ANSI styling.
func TestRenderPausedViewLightTheme(t *testing.T) {
	dark := RenderPausedView(NewDarkTheme(), 80, 14)
	light := RenderPausedView(NewLightTheme(), 80, 14)

	if !strings.Contains(light, "paused") {
		t.Errorf("RenderPausedView(light) = %q, want to contain 'paused'", light)
	}
	if !strings.Contains(light, "resume") {
		t.Errorf("RenderPausedView(light) = %q, want to contain 'resume'", light)
	}
	darkText := stripANSI(dark)
	lightText := stripANSI(light)
	if darkText != lightText {
		t.Errorf("dark and light visible text differ: dark=%q, light=%q", darkText, lightText)
	}
	if dark == light {
		t.Errorf("dark and light raw output should differ due to colors")
	}
}
