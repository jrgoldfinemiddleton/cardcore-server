package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestErrorMessageForCode verifies the error code to message mapping.
func TestErrorMessageForCode(t *testing.T) {
	tests := []struct {
		code      string
		serverMsg string
		want      string
	}{
		{"out_of_turn", "", "Not your turn"},
		{"illegal_move", "Cannot play spades", "Cannot play spades"},
		{"illegal_move", "", "Illegal move"},
		{"wrong_phase", "", "Wrong phase"},
		{"stale_seq", "", ""},
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
		{1011, "Internal server error"},
		{9999, "Connection closed (code 9999)"},
	}

	for _, tt := range tests {
		got := closeMessageForCode(tt.code)
		if got != tt.want {
			t.Errorf("closeMessageForCode(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

// TestHandleWSError verifies that handleWSError sets the correct flash
// message for each error code.
func TestHandleWSError(t *testing.T) {
	tests := []struct {
		code    string
		message string
		wantErr string
		wantCmd bool
	}{
		{"out_of_turn", "Not your turn", "Not your turn", true},
		{"illegal_move", "Cannot play spades", "Cannot play spades", true},
		{"stale_seq", "", "", false},
	}

	for _, tt := range tests {
		m := &model{}
		cmd := m.handleWSError(wsErrorMsg{code: tt.code, message: tt.message})

		if m.errMsg != tt.wantErr {
			t.Errorf("%s: errMsg = %q, want %q", tt.code, m.errMsg, tt.wantErr)
		}
		if tt.wantCmd {
			isFlashTimer(t, cmd)
		} else if cmd != nil {
			t.Errorf("%s: expected no command, got %v", tt.code, cmd)
		}
	}
}

// TestHandleWSClose verifies that handleWSClose sets the status message and
// returns tea.Quit.
func TestHandleWSClose(t *testing.T) {
	m := &model{}

	cmd := m.handleWSClose(wsCloseMsg{code: 1000})

	if m.statusMsg != "Game ended" {
		t.Errorf("statusMsg = %q, want Game ended", m.statusMsg)
	}
	isQuitMsg(t, cmd)
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
