package heartstui

import (
	"encoding/json"
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// Client is the stateful Hearts adapter for the TUI. It decodes snapshots,
// holds the player's selection state, translates key presses into commands,
// and renders the main game area using the package's pure render functions.
//
// It is game-specific by design: the TUI model holds it behind a game-agnostic
// interface so the model itself stays free of Hearts knowledge.
type Client struct {
	// seat is the player's seat index.
	seat int
	// observer is true when this client renders the omniscient observer view.
	observer bool
	// theme is the color palette used by all render functions.
	theme Theme
	// playerSnap is the most recent decoded player snapshot.
	playerSnap heartsclient.PlayerSnapshot
	// observerSnap is the most recent decoded observer snapshot.
	observerSnap heartsclient.ObserverSnapshot
	// phase is the current game phase.
	phase string
	// cursor is the index of the highlighted card in the player's hand.
	cursor int
	// selected holds the cards chosen to pass during the passing phase.
	selected []heartsclient.Card
	// submitted is true after a pass or play is sent for the current snapshot,
	// preventing duplicate submissions until a fresh snapshot arrives.
	submitted bool
	// actionCounter generates unique action IDs for outgoing commands.
	actionCounter int
	// lastErr holds the most recent game-specific decode or validation
	// error. It is cleared at the start of each HandleSnapshot call.
	lastErr string
	// inputDisabled indicates whether the UI should ignore human input (e.g., timeout).
	inputDisabled bool
}

// NewClient returns a Hearts adapter for the given seat. When observer is true,
// the client renders the omniscient observer view and ignores input. The theme
// is stored and passed to all render functions.
func NewClient(seat int, observer bool, theme Theme) *Client {
	return &Client{seat: seat, observer: observer, theme: theme}
}

// SetInputDisabled toggles human input for the client. When true, the UI should
// render the hand dimmed and ignore key presses until the timeout is cleared.
func (c *Client) SetInputDisabled(disabled bool) {
	c.inputDisabled = disabled
}

// HandleSnapshot decodes the snapshot into the player or observer view and
// updates selection state. A phase change resets the cursor and selection; a
// fresh snapshot re-enables input.
func (c *Client) HandleSnapshot(raw json.RawMessage) {
	c.lastErr = ""
	var envelope struct {
		Phase string `json:"phase"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		c.lastErr = "Failed to decode snapshot envelope"
		return
	}
	phaseChanged := envelope.Phase != c.phase
	c.phase = envelope.Phase

	if c.observer {
		var snap heartsclient.ObserverSnapshot
		if err := json.Unmarshal(raw, &snap); err != nil {
			c.lastErr = "Failed to decode observer snapshot"
			return
		}
		c.observerSnap = snap
		return
	}

	var snap heartsclient.PlayerSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		c.lastErr = "Failed to decode player snapshot"
		return
	}
	c.playerSnap = snap
	c.submitted = false
	if phaseChanged {
		c.selected = nil
		c.cursor = 0
	}
	if c.cursor >= len(snap.Hand) {
		c.cursor = max(len(snap.Hand)-1, 0)
	}
	// During the human's playing turn, snap the cursor to the first legal card
	// so the player always starts from a known, playable position.
	isPlayingTurn := c.phase == heartsclient.PhasePlaying && c.playerSnap.Turn == c.seat
	if isPlayingTurn && len(c.playerSnap.LegalActions) > 0 {
		legalSet := cardSet(c.playerSnap.LegalActions)
		c.cursor = firstLegalIndex(snap.Hand, legalSet)
	}
}

// HandleKey processes a key press during an actionable phase, returning a
// command to send (with send true) or a status message to flash. Observers and
// non-actionable phases return no command and no status.
func (c *Client) HandleKey(key tea.KeyPressMsg) (client.Command, bool, string) {
	if c.observer {
		return client.Command{}, false, ""
	}
	switch c.phase {
	case heartsclient.PhasePaused:
		return c.handlePausedKey(key)
	case heartsclient.PhasePassing:
		return c.handlePassingKey(key)
	case heartsclient.PhasePlaying:
		return c.handlePlayingKey(key)
	default:
		return client.Command{}, false, ""
	}
}

// TogglePause builds a pause or resume command based on the current
// paused state.
func (c *Client) TogglePause(paused bool) (client.Command, bool) {
	if paused {
		cmd, err := heartsclient.NewResumeMessage(c.nextActionID(), 0)
		if err != nil {
			return client.Command{}, false
		}
		return cmd, true
	}
	cmd, err := heartsclient.NewPauseMessage(c.nextActionID(), 0)
	if err != nil {
		return client.Command{}, false
	}
	return cmd, true
}

// Render returns the main game area for the current snapshot and selection,
// passing the stored theme to all render functions.
func (c *Client) Render() string {
	if c.observer {
		return RenderObserverView(c.observerSnap, c.theme)
	}
	switch c.phase {
	case heartsclient.PhaseDeal:
		return RenderDealView()
	case heartsclient.PhasePaused:
		return RenderPausedView(c.theme)
	case heartsclient.PhasePassing:
		return RenderPassingView(c.playerSnap, c.seat, c.cursor,
			c.selected, c.inputDisabled, c.theme)
	case heartsclient.PhasePlaying:
		return RenderPlayingView(c.playerSnap, c.seat, c.cursor, c.inputDisabled, c.theme)
	case heartsclient.PhaseTrickComplete:
		return RenderTrickCompleteView(c.playerSnap, c.seat, c.theme)
	case heartsclient.PhaseRoundComplete:
		return RenderRoundCompleteView(c.playerSnap, c.theme)
	case heartsclient.PhaseGameOver:
		return RenderGameOverView(c.playerSnap, c.theme)
	default:
		return fmt.Sprintf("Phase: %s", c.phase)
	}
}

// LastError returns the most recent game-specific error, or an empty string
// if the last HandleSnapshot call succeeded.
func (c *Client) LastError() string {
	return c.lastErr
}

// ResetSubmitted re-enables input after a recoverable server error.
func (c *Client) ResetSubmitted() {
	c.submitted = false
}

// IsHumanTurn reports whether the local player may act right now: not an
// observer, in an actionable phase, and it is this player's turn.
func (c *Client) IsHumanTurn() bool {
	if c.observer {
		return false
	}
	if c.phase != heartsclient.PhasePassing && c.phase != heartsclient.PhasePlaying {
		return false
	}
	return c.playerSnap.Turn == c.seat
}

// handlePausedKey handles key presses during the paused phase.
// The model layer handles the 'p' resume toggle, so all direct keys are ignored.
func (c *Client) handlePausedKey(tea.KeyPressMsg) (client.Command, bool, string) {
	return client.Command{}, false, ""
}

// handlePassingKey handles navigation, selection, and submission during the
// passing phase. Input is ignored once the player has submitted or when input
// is disabled (e.g., waiting for the next snapshot or another player's turn).
func (c *Client) handlePassingKey(key tea.KeyPressMsg) (client.Command, bool, string) {
	if c.submitted || c.inputDisabled {
		return client.Command{}, false, ""
	}
	switch key.Code {
	case tea.KeyLeft:
		c.moveCursor(-1)
	case tea.KeyRight:
		c.moveCursor(1)
	case tea.KeySpace:
		c.toggleSelected()
	case tea.KeyEnter:
		return c.submitPass()
	}
	return client.Command{}, false, ""
}

// handlePlayingKey handles navigation and submission during the playing phase.
// Input is ignored once the player has submitted or when input is disabled
// (e.g., waiting for the next snapshot or another player's turn).
func (c *Client) handlePlayingKey(key tea.KeyPressMsg) (client.Command, bool, string) {
	if c.submitted || c.inputDisabled {
		return client.Command{}, false, ""
	}
	switch key.Code {
	case tea.KeyLeft:
		c.moveCursor(-1)
	case tea.KeyRight:
		c.moveCursor(1)
	case tea.KeyEnter:
		return c.submitPlay()
	}
	return client.Command{}, false, ""
}

// submitPass builds a pass_cards command for the selected cards, or returns a
// status message when the wrong number of cards is selected.
func (c *Client) submitPass() (client.Command, bool, string) {
	cmd, err := BuildPassCommand(c.nextActionID(), c.selected)
	if err != nil {
		return client.Command{}, false, "Select exactly 3 cards to pass"
	}
	c.submitted = true
	return cmd, true, ""
}

// submitPlay builds a play_card command for the card under the cursor, or
// returns a status message for out-of-turn or illegal plays.
func (c *Client) submitPlay() (client.Command, bool, string) {
	if c.playerSnap.Turn != c.seat {
		return client.Command{}, false, "Not your turn"
	}
	hand := c.playerSnap.Hand
	if c.cursor < 0 || c.cursor >= len(hand) {
		return client.Command{}, false, "Invalid state"
	}
	card := hand[c.cursor]
	if !slices.Contains(c.playerSnap.LegalActions, card) {
		return client.Command{}, false, "Illegal move"
	}
	cmd, err := BuildPlayCommand(c.nextActionID(), card)
	if err != nil {
		return client.Command{}, false, "Failed to build command"
	}
	c.submitted = true
	return cmd, true, ""
}

// moveCursor moves the hand cursor by delta, skipping illegal cards in the
// playing phase and clamping to the hand bounds otherwise.
func (c *Client) moveCursor(delta int) {
	n := len(c.playerSnap.Hand)
	if n == 0 {
		return
	}
	if c.phase == heartsclient.PhasePlaying && len(c.playerSnap.LegalActions) > 0 {
		legalSet := cardSet(c.playerSnap.LegalActions)
		c.cursor = nextLegalIndex(c.playerSnap.Hand, c.cursor, delta, legalSet)
		return
	}
	c.cursor += delta
	if c.cursor < 0 {
		c.cursor = 0
	}
	if c.cursor >= n {
		c.cursor = n - 1
	}
}

// toggleSelected adds or removes the card under the cursor from the pass
// selection. At most three cards may be selected; a fourth is ignored.
func (c *Client) toggleSelected() {
	hand := c.playerSnap.Hand
	if c.cursor < 0 || c.cursor >= len(hand) {
		return
	}
	card := hand[c.cursor]
	if i := slices.Index(c.selected, card); i >= 0 {
		c.selected = slices.Delete(c.selected, i, i+1)
		return
	}
	if len(c.selected) >= 3 {
		return
	}
	c.selected = append(c.selected, card)
}

// nextActionID returns a unique action ID for the next outgoing command.
// Includes seat number to prevent collisions between multiple TUI clients in
// the same session.
func (c *Client) nextActionID() string {
	c.actionCounter++
	return fmt.Sprintf("tui-%d-%d", c.seat, c.actionCounter)
}

// firstLegalIndex returns the index of the first legal card in hand order, or
// 0 if no legal card exists.
func firstLegalIndex(hand []heartsclient.Card, legalSet map[heartsclient.Card]bool) int {
	for i, c := range hand {
		if legalSet[c] {
			return i
		}
	}
	return 0
}

// nextLegalIndex returns the nearest legal card index in the direction of
// delta, wrapping around the hand. If no legal card exists, it returns start.
func nextLegalIndex(
	hand []heartsclient.Card,
	start, delta int,
	legalSet map[heartsclient.Card]bool,
) int {
	n := len(hand)
	if n == 0 {
		return 0
	}
	idx := start
	for range n {
		idx = (idx + delta + n) % n
		if legalSet[hand[idx]] {
			return idx
		}
	}
	return start
}
