package heartstui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// TestClientHandleSnapshotPlayer verifies a player snapshot is decoded and the
// phase is recorded.
func TestClientHandleSnapshotPlayer(t *testing.T) {
	c := NewClient(0, false)
	snap := heartsclient.PlayerSnapshot{
		Phase: heartsclient.PhasePassing,
		Hand:  []heartsclient.Card{{Rank: "two", Suit: "clubs"}},
	}

	c.HandleSnapshot(mustMarshal(t, snap))

	if c.phase != heartsclient.PhasePassing {
		t.Errorf("got phase %q, want %q", c.phase, heartsclient.PhasePassing)
	}
	if len(c.playerSnap.Hand) != 1 {
		t.Errorf("got hand len %d, want 1", len(c.playerSnap.Hand))
	}
}

// TestClientNavigation verifies that Right and Left move the cursor, clamped to
// the hand bounds.
func TestClientNavigation(t *testing.T) {
	c := newPassingClient(t)

	c.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if c.cursor != 1 {
		t.Errorf("got cursor %d, want 1", c.cursor)
	}

	c.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	c.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if c.cursor != 0 {
		t.Errorf("got cursor %d after clamping left, want 0", c.cursor)
	}

	// Test clamping at the max card index.
	for range 10 {
		c.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	}
	wantMax := len(c.playerSnap.Hand) - 1
	if c.cursor != wantMax {
		t.Errorf("got cursor %d after clamping right, want %d", c.cursor, wantMax)
	}
}

// TestClientToggleSelect verifies that Space selects and deselects the card
// under the cursor.
func TestClientToggleSelect(t *testing.T) {
	c := newPassingClient(t)

	c.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace})
	if len(c.selected) != 1 {
		t.Errorf("got %d selected after first toggle, want 1", len(c.selected))
	}

	c.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace})
	if len(c.selected) != 0 {
		t.Errorf("got %d selected after second toggle, want 0", len(c.selected))
	}
}

// TestClientMaxThreeSelected verifies that a fourth selection is ignored.
func TestClientMaxThreeSelected(t *testing.T) {
	c := newPassingClient(t)

	for range 4 {
		c.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace})
		c.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	}

	if len(c.selected) != 3 {
		t.Errorf("got %d selected, want 3", len(c.selected))
	}
}

// TestClientSubmitPass verifies that Enter with three cards selected returns a
// pass_cards command and marks the client submitted.
func TestClientSubmitPass(t *testing.T) {
	c := newPassingClient(t)

	for range 3 {
		c.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace})
		c.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	}

	cmd, send, status := c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !send {
		t.Errorf("got send false, want true")
	}
	if status != "" {
		t.Errorf("got status %q, want empty", status)
	}
	if cmd.Type != "pass_cards" {
		t.Errorf("got cmd type %q, want pass_cards", cmd.Type)
	}
	if !c.submitted {
		t.Errorf("got submitted false, want true")
	}
}

// TestClientSubmitPassWrongCount verifies that Enter with fewer than three
// cards selected flashes a status and does not submit.
func TestClientSubmitPassWrongCount(t *testing.T) {
	c := newPassingClient(t)

	c.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace})

	_, send, status := c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if send {
		t.Errorf("got send true, want false")
	}
	if status != "Select exactly 3 cards to pass" {
		t.Errorf("got status %q, want %q", status, "Select exactly 3 cards to pass")
	}
	if c.submitted {
		t.Errorf("got submitted true, want false")
	}
}

// TestClientSubmitPlay verifies that Enter on a legal card during the player's
// turn returns a play_card command.
func TestClientSubmitPlay(t *testing.T) {
	c := NewClient(0, false)
	card := heartsclient.Card{Rank: "two", Suit: "clubs"}
	snap := heartsclient.PlayerSnapshot{
		Phase:        heartsclient.PhasePlaying,
		Turn:         0,
		Hand:         []heartsclient.Card{card},
		LegalActions: []heartsclient.Card{card},
	}
	c.HandleSnapshot(mustMarshal(t, snap))

	cmd, send, status := c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !send {
		t.Errorf("got send false, want true")
	}
	if status != "" {
		t.Errorf("got status %q, want empty", status)
	}
	if cmd.Type != "play_card" {
		t.Errorf("got cmd type %q, want play_card", cmd.Type)
	}
}

// TestClientSubmitPlayNotYourTurn verifies that Enter when it is not the
// player's turn flashes a status and does not submit.
func TestClientSubmitPlayNotYourTurn(t *testing.T) {
	c := NewClient(0, false)
	card := heartsclient.Card{Rank: "two", Suit: "clubs"}
	snap := heartsclient.PlayerSnapshot{
		Phase:        heartsclient.PhasePlaying,
		Turn:         2,
		Hand:         []heartsclient.Card{card},
		LegalActions: []heartsclient.Card{card},
	}
	c.HandleSnapshot(mustMarshal(t, snap))

	_, send, status := c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if send {
		t.Errorf("got send true, want false")
	}
	if status != "Not your turn" {
		t.Errorf("got status %q, want %q", status, "Not your turn")
	}
}

// TestClientSubmitPlayIllegal verifies that Enter on a card not in the legal
// actions flashes "Illegal move".
func TestClientSubmitPlayIllegal(t *testing.T) {
	c := NewClient(0, false)
	snap := heartsclient.PlayerSnapshot{
		Phase: heartsclient.PhasePlaying,
		Turn:  0,
		Hand: []heartsclient.Card{
			{Rank: "two", Suit: "clubs"},
			{Rank: "three", Suit: "clubs"},
		},
		LegalActions: []heartsclient.Card{{Rank: "three", Suit: "clubs"}},
	}
	c.HandleSnapshot(mustMarshal(t, snap))

	_, send, status := c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if send {
		t.Errorf("got send true, want false")
	}
	if status != "Illegal move" {
		t.Errorf("got status %q, want %q", status, "Illegal move")
	}
}

// TestClientObserverIgnoresKeys verifies that an observer client produces no
// command or status from key presses.
func TestClientObserverIgnoresKeys(t *testing.T) {
	c := NewClient(0, true)
	snap := heartsclient.ObserverSnapshot{Phase: heartsclient.PhasePlaying}
	c.HandleSnapshot(mustMarshal(t, snap))

	_, send, status := c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if send {
		t.Errorf("got send true, want false")
	}
	if status != "" {
		t.Errorf("got status %q, want empty", status)
	}
}

// TestClientRenderObserver verifies the observer render includes seat labels.
func TestClientRenderObserver(t *testing.T) {
	c := NewClient(0, true)
	snap := heartsclient.ObserverSnapshot{
		Phase: heartsclient.PhasePlaying,
		Hands: [][]heartsclient.Card{
			{{Rank: "jack", Suit: "clubs"}},
			{{Rank: "queen", Suit: "diamonds"}},
			{{Rank: "king", Suit: "hearts"}},
			{{Rank: "ace", Suit: "spades"}},
		},
		Scores:      []int{0, 0, 0, 0},
		RoundPoints: []int{0, 0, 0, 0},
	}
	c.HandleSnapshot(mustMarshal(t, snap))

	got := c.Render()
	if !strings.Contains(got, "Seat 0") {
		t.Errorf("got render %q, want to contain %q", got, "Seat 0")
	}
}

// TestClientPhaseResetSelection verifies that a phase change clears the pass
// selection.
func TestClientPhaseResetSelection(t *testing.T) {
	c := newPassingClient(t)
	c.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace})
	if len(c.selected) != 1 {
		t.Fatalf("got %d selected before transition, want 1", len(c.selected))
	}

	playing := heartsclient.PlayerSnapshot{
		Phase: heartsclient.PhasePlaying,
		Hand:  []heartsclient.Card{{Rank: "two", Suit: "clubs"}},
	}
	c.HandleSnapshot(mustMarshal(t, playing))

	if len(c.selected) != 0 {
		t.Errorf("got %d selected after phase change, want 0", len(c.selected))
	}
}

// TestClientHandleSnapshotInvalidJSON verifies that invalid JSON sets lastErr
// and does not update playerSnap.
func TestClientHandleSnapshotInvalidJSON(t *testing.T) {
	c := NewClient(0, false)
	c.HandleSnapshot(json.RawMessage(`not json`))

	if c.LastError() != "Failed to decode snapshot envelope" {
		t.Errorf("got lastErr %q, want %q", c.LastError(), "Failed to decode snapshot envelope")
	}
}

// TestClientHandleSnapshotPlayerDecodeError verifies that a valid envelope with
// an unmarshalable player snapshot sets lastErr.
func TestClientHandleSnapshotPlayerDecodeError(t *testing.T) {
	c := NewClient(0, false)
	c.HandleSnapshot(json.RawMessage(`{"phase":"playing","turn":0,"hand":["not-a-card"]}`))

	if c.LastError() != "Failed to decode player snapshot" {
		t.Errorf("got lastErr %q, want %q", c.LastError(), "Failed to decode player snapshot")
	}
}

// TestClientHandleSnapshotObserverDecodeError verifies that an unmarshalable
// observer snapshot sets lastErr.
func TestClientHandleSnapshotObserverDecodeError(t *testing.T) {
	c := NewClient(0, true)
	c.HandleSnapshot(json.RawMessage(`{"phase":"playing","hands":"not-an-array"}`))

	if c.LastError() != "Failed to decode observer snapshot" {
		t.Errorf("got lastErr %q, want %q", c.LastError(), "Failed to decode observer snapshot")
	}
}

// TestClientLastErrorClearedOnSuccess verifies that a successful HandleSnapshot
// clears any previous lastErr.
func TestClientLastErrorClearedOnSuccess(t *testing.T) {
	c := NewClient(0, false)
	c.HandleSnapshot(json.RawMessage(`not json`))
	if c.LastError() == "" {
		t.Fatal("expected lastErr after invalid JSON")
	}

	snap := heartsclient.PlayerSnapshot{
		Phase: heartsclient.PhasePassing,
		Hand:  []heartsclient.Card{{Rank: "two", Suit: "clubs"}},
	}
	c.HandleSnapshot(mustMarshal(t, snap))

	if c.LastError() != "" {
		t.Errorf("got lastErr %q, want empty after successful decode", c.LastError())
	}
}

// newPassingClient returns a player client with a four-card hand in the
// passing phase, ready for navigation and selection tests.
func newPassingClient(t *testing.T) *Client {
	t.Helper()
	c := NewClient(0, false)
	snap := heartsclient.PlayerSnapshot{
		Phase: heartsclient.PhasePassing,
		Hand: []heartsclient.Card{
			{Rank: "two", Suit: "clubs"},
			{Rank: "three", Suit: "clubs"},
			{Rank: "four", Suit: "clubs"},
			{Rank: "five", Suit: "clubs"},
		},
	}
	c.HandleSnapshot(mustMarshal(t, snap))
	return c
}

// mustMarshal marshals v to JSON, failing the test on error.
func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}
