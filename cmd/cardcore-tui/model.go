package main

import (
	"context"
	"encoding/json"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
)

// gameClient encapsulates all game-specific behavior for the TUI so the model
// stays game-agnostic. Implementations decode snapshots, translate key presses
// into commands, and render the main game area.
type gameClient interface {
	// HandleSnapshot decodes and stores a new snapshot.
	HandleSnapshot(raw json.RawMessage)
	// LastError returns the most recent error from the game client, or an
	// empty string if there is none. It is called after HandleSnapshot so
	// the model can flash decode failures to the user.
	LastError() string
	// HandleKey processes a key press, optionally producing a command to send
	// and a status message to flash. send is true when cmd should be sent;
	// status is non-empty when a message should flash.
	HandleKey(key tea.KeyPressMsg) (cmd client.Command, send bool, status string)
	// Render returns the main game area for the current state.
	Render() string
}

// model is the main Bubble Tea model for the cardcore TUI.
//
// It is game-agnostic: it owns the connection lifecycle, phase tracking, error
// handling, and Bubble Tea plumbing, and delegates all game-specific behavior
// to a gameClient. It uses pointer receivers so the program reference can be
// set after tea.NewProgram is called (see the comment in main.go).
type model struct {
	// program is the Bubble Tea program reference. It is set after
	// tea.NewProgram is called so the WebSocket goroutine can send
	// messages into the model via program.Send().
	program *tea.Program

	// conn is the WebSocket connection to the server.
	conn *client.Conn

	// game handles all game-specific decoding, input, and rendering.
	game gameClient

	// snapshot is the raw JSON of the most recent snapshot. It is retained
	// to detect whether any game state has arrived yet.
	snapshot json.RawMessage

	// phase is the current game phase, decoded from the generic snapshot
	// envelope. It drives the header and connection-state decisions.
	phase string

	// roundNumber is the current round number (1-based), from the envelope.
	roundNumber int

	// scores is the cumulative scores per seat, decoded from the snapshot
	// envelope. It is used to render the header score summary.
	scores []int

	// errMsg is the current error flash message. It is displayed in the
	// status bar for 3 seconds, then cleared.
	errMsg string

	// statusMsg is a persistent status message (e.g., a close reason).
	statusMsg string

	// disconnected is true when the WebSocket has closed. It drives the
	// footer connection-state display.
	disconnected bool
	// escConfirm is true when the user has pressed Escape once and is
	// waiting for Enter to confirm quit.
	escConfirm bool
}

// commandSentMsg is delivered after an outgoing command send completes. A
// non-nil err indicates the send failed.
type commandSentMsg struct {
	// err is the send error, or nil on success.
	err error
}

// Init is the first function called by the Bubble Tea framework. It returns
// an optional initial command.
//
// The WebSocket connection and reader goroutine are already established in
// main.go before the program starts, so Init returns nil.
func (m *model) Init() tea.Cmd {
	return nil
}

// Update handles all messages that flow into the model.
//
// Every event (snapshot arrival, error message, keypress, timer) arrives here
// as a tea.Msg. Game-agnostic concerns are handled here; game-specific input
// is delegated to the gameClient. All state mutations happen on the single
// Bubble Tea program goroutine, so no locks are needed.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case wsSnapshotMsg:
		m.handleSnapshot(msg.raw)
		return m, nil

	case wsErrorMsg:
		return m, m.handleWSError(msg)

	case wsCloseMsg:
		return m, m.handleWSClose(msg)

	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)

	case commandSentMsg:
		if msg.err != nil {
			return m, m.setErrorFlash("Failed to send command")
		}
		return m, nil

	case flashTimeoutMsg:
		m.errMsg = ""
		m.escConfirm = false
		return m, nil
	}

	return m, nil
}

// View renders the current model state as a terminal screen.
//
// In Bubble Tea v2, View returns a tea.View struct holding the content plus
// options like AltScreen. The layout (header, main, footer) is assembled in
// layout.go; the main area is produced by the gameClient.
func (m *model) View() tea.View {
	v := tea.NewView(m.renderLayout())
	v.AltScreen = true
	return v
}

// handleSnapshot decodes the generic snapshot envelope for the header/footer
// and delegates the full snapshot to the gameClient for game-specific decoding.
func (m *model) handleSnapshot(raw []byte) {
	var envelope struct {
		Phase       string `json:"phase"`
		RoundNumber int    `json:"round_number"`
		Scores      []int  `json:"scores"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		m.errMsg = "Failed to decode snapshot"
		return
	}

	m.phase = envelope.Phase
	m.roundNumber = envelope.RoundNumber
	m.scores = envelope.Scores
	m.snapshot = raw
	m.game.HandleSnapshot(raw)
	if errMsg := m.game.LastError(); errMsg != "" {
		m.errMsg = errMsg
	}
}

// handleKeyPress handles keyboard input. ctrl+c always quits; Esc then Enter
// quits from any state; Enter quits in game_over phase. All other keys are
// delegated to the gameClient.
func (m *model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		if m.conn != nil {
			_ = m.conn.Close()
		}
		return m, tea.Quit
	}

	if msg.Code == tea.KeyEscape {
		if m.escConfirm {
			m.escConfirm = false
			m.errMsg = ""
			return m, nil
		}
		m.escConfirm = true
		return m, m.setErrorFlash("Press Enter to quit")
	}

	if m.escConfirm && msg.Code == tea.KeyEnter {
		if m.conn != nil {
			_ = m.conn.Close()
		}
		return m, tea.Quit
	}

	m.escConfirm = false

	if m.phase == "game_over" && msg.Code == tea.KeyEnter {
		if m.conn != nil {
			_ = m.conn.Close()
		}
		return m, tea.Quit
	}

	cmd, send, status := m.game.HandleKey(msg)
	if status != "" {
		return m, m.setErrorFlash(status)
	}
	if send {
		return m, m.sendCommandCmd(cmd)
	}
	return m, nil
}

// sendCommandCmd returns a tea.Cmd that sends the command on the WebSocket in a
// background goroutine and reports the result as a commandSentMsg. Running the
// send off the event loop keeps the UI responsive and avoids blocking.
func (m *model) sendCommandCmd(cmd client.Command) tea.Cmd {
	conn := m.conn
	return func() tea.Msg {
		if conn == nil {
			return commandSentMsg{}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := conn.SendCommand(ctx, cmd); err != nil {
			return commandSentMsg{err: err}
		}
		return commandSentMsg{}
	}
}
