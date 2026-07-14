package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

const (
	msgOutOfTurn    = "AI played for you — your turn has passed. Press Enter to continue."
	msgWrongPhase   = "AI played for you — phase has changed. Press Enter to continue."
	msgIllegalMove  = "Bug: server rejected a valid card. Press Enter to exit."
	msgMalformedMsg = "Internal error: invalid command format. Press Enter to exit."
)

// TestErrorMessageForCode verifies the error code to message mapping.
func TestErrorMessageForCode(t *testing.T) {
	tests := []struct {
		code      string
		serverMsg string
		want      string
	}{
		{"out_of_turn", "", msgOutOfTurn},
		{"illegal_move", "Cannot play spades", msgIllegalMove},
		{"illegal_move", "", msgIllegalMove},
		{"wrong_phase", "", msgWrongPhase},
		{"stale_seq", "", ""},
		{phaseGameOver, "", "Game over. Press Enter to exit."},
		{"malformed_message", "", msgMalformedMsg},
		{"unknown", "Something bad", "Something bad"},
		{"unknown", "", "Error: unknown"},
	}

	for _, tt := range tests {
		got := errorMessageForCode(tt.code, tt.serverMsg)
		if got != tt.want {
			t.Errorf(
				"errorMessageForCode(%q, %q) = %q, want %q",
				tt.code, tt.serverMsg, got, tt.want,
			)
		}
	}
}

// TestCloseMessageForCode verifies the close code to message mapping.
func TestCloseMessageForCode(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{1000, "Game ended"},
		{1001, "Server is shutting down"},
		{1011, "Internal server error. Press Enter to exit."},
		{9999, "Connection closed (code 9999)"},
	}

	for _, tt := range tests {
		got := closeMessageForCode(tt.code)
		if got != tt.want {
			t.Errorf("closeMessageForCode(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

// TestHandleWSError verifies that handleWSError drives the correct recovery
// path (silent resync, continue modal, or fatal modal) per error code.
func TestHandleWSError(t *testing.T) {
	tests := []struct {
		code              string
		message           string
		wantErr           string
		wantModalContinue bool
		wantModalFatal    bool
	}{
		{"out_of_turn", "Not your turn", msgOutOfTurn, true, false},
		{"wrong_phase", "Bad phase", msgWrongPhase, true, false},
		{"illegal_move", "Cannot play spades", msgIllegalMove, false, true},
		{"stale_seq", "", "", false, false},
		{phaseGameOver, "", "", false, false},
		{"malformed_message", "Bad JSON", msgMalformedMsg, false, true},
	}

	for _, tt := range tests {
		m := &model{game: &fakeGame{}}
		m.handleWSError(wsErrorMsg{code: tt.code, message: tt.message})

		if m.errMsg != tt.wantErr {
			t.Errorf("%s: errMsg = %q, want %q", tt.code, m.errMsg, tt.wantErr)
		}
		if m.modalContinue != tt.wantModalContinue {
			t.Errorf("%s: modalContinue = %v, want %v",
				tt.code, m.modalContinue, tt.wantModalContinue)
		}
		if m.modalFatal != tt.wantModalFatal {
			t.Errorf("%s: modalFatal = %v, want %v",
				tt.code, m.modalFatal, tt.wantModalFatal)
		}
	}
}

// TestHandleWSClose verifies that handleWSClose sets the status message.
// 1000 returns tea.Quit unless the game is over; 1011 sets modalFatal and does
// not quit immediately.
func TestHandleWSClose(t *testing.T) {
	t.Run("normal closure quits during live game", func(t *testing.T) {
		m := &model{phase: "playing"}
		cmd := m.handleWSClose(wsCloseMsg{code: 1000})
		if m.statusMsg != "Game ended" {
			t.Errorf("statusMsg = %q, want Game ended", m.statusMsg)
		}
		isQuitMsg(t, cmd)
	})

	t.Run("normal closure stays on final screen in game_over", func(t *testing.T) {
		m := &model{phase: phaseGameOver}
		cmd := m.handleWSClose(wsCloseMsg{code: 1000})
		if m.statusMsg != "Game ended" {
			t.Errorf("statusMsg = %q, want Game ended", m.statusMsg)
		}
		if cmd != nil {
			t.Errorf("expected nil cmd for game_over close, got %v", cmd)
		}
	})

	t.Run("internal error", func(t *testing.T) {
		m := &model{}
		cmd := m.handleWSClose(wsCloseMsg{code: 1011})
		wantMsg := "Internal server error. Press Enter to exit."
		if m.statusMsg != wantMsg {
			t.Errorf("statusMsg = %q, want %q", m.statusMsg, wantMsg)
		}
		if !m.modalFatal {
			t.Errorf("modalFatal = false, want true")
		}
		if cmd != nil {
			t.Errorf("expected nil cmd for 1011 modal, got %v", cmd)
		}
	})
}

// TestClearErrorFlash verifies that clearErrorFlash clears the error message.
func TestClearErrorFlash(t *testing.T) {
	m := &model{errMsg: "some error"}

	m.clearErrorFlash()

	if m.errMsg != "" {
		t.Errorf("errMsg = %q, want empty", m.errMsg)
	}
}

// TestFlashTimeoutClearsError verifies the end-to-end flash timer flow:
// set error → timer expires → error is cleared.
func TestFlashTimeoutClearsError(t *testing.T) {
	m := &model{}

	// Set the error flash.
	m.setErrorFlash("Not your turn")
	if m.errMsg != "Not your turn" {
		t.Fatalf("errMsg = %q, want Not your turn", m.errMsg)
	}

	// Simulate the timer firing.
	newM, _ := m.Update(flashTimeoutMsg{})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	if mm.errMsg != "" {
		t.Errorf("errMsg = %q, want empty after flash timeout", mm.errMsg)
	}
}

// isQuitMsg calls cmd and verifies it returns tea.QuitMsg.
func isQuitMsg(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("cmd is nil, want tea.Quit")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("cmd returned %T, want tea.QuitMsg", msg)
	}
}

// isFlashTimer calls cmd and verifies it returns flashTimeoutMsg.
func isFlashTimer(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("cmd is nil, want flash timer")
	}
	msg := cmd()
	if _, ok := msg.(flashTimeoutMsg); !ok {
		t.Fatalf("cmd returned %T, want flashTimeoutMsg", msg)
	}
}
