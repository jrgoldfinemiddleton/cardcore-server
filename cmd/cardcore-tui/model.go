package main

import (
	"encoding/json"
	"time"

	"charm.land/bubbletea/v2"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
)

// model is the main Bubble Tea model for the cardcore TUI.
//
// It uses pointer receivers so that the program reference can be set after
// tea.NewProgram is called. See the detailed comment in main.go for why
// this matters.
//
// The model holds three categories of state:
//
//	Connection state: the WebSocket connection and Bubble Tea program reference.
//	Game state: the current snapshot, phase, round number.
//	UI state: cursor position, selected cards, error messages, flash timers.
type model struct {
	// program is the Bubble Tea program reference. It is set after
	// tea.NewProgram is called so the WebSocket goroutine can send
	// messages into the model via program.Send().
	program *tea.Program

	// conn is the WebSocket connection to the server.
	conn *client.Conn

	// seat is the player's seat index. The valid range depends on the game.
	seat int

	// observer is true when the TUI is in receive-only observer mode.
	observer bool

	// snapshot is the raw JSON of the most recent snapshot. The model
	// decodes it on demand based on the current phase and player/observer
	// mode.
	snapshot json.RawMessage

	// phase is the current game phase: "dealing", "passing", "playing",
	// "trick_complete", "round_complete", "game_over".
	phase string

	// roundNumber is the current round number (1-based).
	roundNumber int

	// errMsg is the current error flash message. It is displayed in the
	// status bar for 3 seconds, then cleared.
	errMsg string

	// statusMsg is a persistent status message (e.g., "AI thinking...").
	statusMsg string
}

// Init is the first function called by the Bubble Tea framework. It returns
// an optional initial command.
//
// For the cardcore TUI, the WebSocket connection is already established in
// main.go before the program starts. The WebSocket reader goroutine is also
// started in main.go. Therefore, Init returns nil — there is no initial
// command needed.
//
// Why no command here? The model is already fully initialized (connection
// open, goroutine running). The first message the model receives will be
// a wsSnapshotMsg from the goroutine, which transitions the model from
// "connecting" to the actual game phase.
func (m *model) Init() tea.Cmd {
	return nil
}

// Update handles all messages that flow into the model.
//
// This is the heart of the state machine. Every event (snapshot arrival,
// error message, keypress, timer) arrives here as a tea.Msg. The model
// updates its state and returns a new model and an optional command.
//
// All state mutations happen on the single Bubble Tea program goroutine.
// There are no locks because Update is never called concurrently.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case wsSnapshotMsg:
		m.handleSnapshot(msg.raw)
		return m, nil

	case wsErrorMsg:
		// TODO: wire in handleWSError (from status.go) for code-based flash
		m.errMsg = msg.message
		return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return flashTimeoutMsg{}
		})

	case wsCloseMsg:
		// TODO: wire in handleWSClose (from status.go) for close code mapping
		if m.conn != nil {
			_ = m.conn.Close()
		}
		return m, tea.Quit

	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)

	case flashTimeoutMsg:
		m.errMsg = ""
		return m, nil
	}

	return m, nil
}

// View renders the current model state as a terminal screen.
//
// In Bubble Tea v2, View returns a tea.View struct (not a plain string).
// The struct holds the content string plus options like AltScreen.
//
// The layout is: header (scores/phase), main area (game state), footer
// (status bar). Each area is rendered by a separate function in layout.go.
func (m *model) View() tea.View {
	v := tea.NewView(m.renderLayout())
	v.AltScreen = true
	return v
}

// handleSnapshot decodes the snapshot envelope and stores the raw bytes.
//
// TODO: Decode full snapshot into game-specific DTOs when rendering is ready.
// It extracts the phase and sequence from the envelope, then stores the
// full raw JSON for later decoding. The actual decoding into game-specific
// DTOs (PlayerSnapshot, ObserverSnapshot) is deferred to the rendering
// layer.
func (m *model) handleSnapshot(raw []byte) {
	var envelope struct {
		Phase string `json:"phase"`
		Seq   int    `json:"seq"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		m.errMsg = "Failed to decode snapshot"
		return
	}

	m.phase = envelope.Phase
	m.snapshot = raw
}

// handleKeyPress handles keyboard input.
//
// TODO: Add arrow keys, space, enter for card navigation and selection.
// Navigation: Left/Right arrows move the cursor.
// Selection: Space toggles the current card.
// Action: Enter confirms the selection (pass or play).
// Quit: ctrl+c exits the program.
func (m *model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.conn != nil {
			_ = m.conn.Close()
		}
		return m, tea.Quit
	}

	return m, nil
}
