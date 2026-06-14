package main

import (
	"testing"

	"charm.land/bubbletea/v2"
)

// TestModelUpdateSnapshot verifies snapshot messages update the model.
func TestModelUpdateSnapshot(t *testing.T) {
	m := &model{phase: "connecting"}

	newM, _ := m.Update(wsSnapshotMsg{raw: []byte(`{"phase":"playing"}`)})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	if mm.phase != "playing" {
		t.Errorf("phase = %q, want playing", mm.phase)
	}
}

// TestModelUpdateError verifies error messages set the flash message.
func TestModelUpdateError(t *testing.T) {
	m := &model{}

	newM, cmd := m.Update(wsErrorMsg{code: "out_of_turn", message: "Not your turn"})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	if mm.errMsg != "Not your turn" {
		t.Errorf("errMsg = %q, want Not your turn", mm.errMsg)
	}
	isFlashTimer(t, cmd)
}

// TestModelUpdateFlashTimeout verifies flash timeout clears the error.
func TestModelUpdateFlashTimeout(t *testing.T) {
	m := &model{errMsg: "some error"}

	newM, _ := m.Update(flashTimeoutMsg{})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	if mm.errMsg != "" {
		t.Errorf("errMsg = %q, want empty after timeout", mm.errMsg)
	}
}

// TestModelUpdateKeyPressCtrlC verifies ctrl+c quits the program.
func TestModelUpdateKeyPressCtrlC(t *testing.T) {
	m := &model{}

	newM, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	_, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	isQuitMsg(t, cmd)
}

// TestModelUpdateWSClose verifies WebSocket close quits the program.
func TestModelUpdateWSClose(t *testing.T) {
	m := &model{}

	newM, cmd := m.Update(wsCloseMsg{code: 1000})
	_, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	isQuitMsg(t, cmd)
}

// TestModelHandleSnapshot verifies snapshot decoding.
func TestModelHandleSnapshot(t *testing.T) {
	m := &model{phase: "connecting"}

	m.handleSnapshot([]byte(`{"phase":"playing","seq":5}`))

	if m.phase != "playing" {
		t.Errorf("phase = %q, want playing", m.phase)
	}
	if m.snapshot == nil {
		t.Error("snapshot is nil, want non-nil")
	}
}

// TestModelHandleSnapshotInvalid verifies invalid JSON sets error.
func TestModelHandleSnapshotInvalid(t *testing.T) {
	m := &model{}

	m.handleSnapshot([]byte(`not json`))

	if m.errMsg == "" {
		t.Error("errMsg is empty, want error message")
	}
}
