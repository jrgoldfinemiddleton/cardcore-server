package main

import (
	"encoding/json"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
)

// fakeGame is a test gameClient that records delegation calls and returns
// configured results.
type fakeGame struct {
	snapshotCalls int
	lastErr       string
	keyCmd        client.Command
	keySend       bool
	keyStatus     string
	keyCalls      int
	renderOut     string
}

// TestModelUpdateSnapshot verifies snapshot messages update the phase and are
// delegated to the game client.
func TestModelUpdateSnapshot(t *testing.T) {
	f := &fakeGame{}
	m := &model{phase: "connecting", game: f}

	newM, _ := m.Update(wsSnapshotMsg{raw: []byte(`{"phase":"playing"}`)})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	if mm.phase != "playing" {
		t.Errorf("got phase %q, want playing", mm.phase)
	}
	if f.snapshotCalls != 1 {
		t.Errorf("got snapshotCalls %d, want 1", f.snapshotCalls)
	}
}

// TestModelUpdateError verifies error messages set the flash message.
func TestModelUpdateError(t *testing.T) {
	m := &model{game: &fakeGame{}}

	newM, cmd := m.Update(wsErrorMsg{code: "out_of_turn", message: "Not your turn"})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	if mm.errMsg != "Not your turn" {
		t.Errorf("got errMsg %q, want Not your turn", mm.errMsg)
	}
	isFlashTimer(t, cmd)
}

// TestModelUpdateFlashTimeout verifies flash timeout clears the error.
func TestModelUpdateFlashTimeout(t *testing.T) {
	m := &model{errMsg: "some error", game: &fakeGame{}}

	newM, _ := m.Update(flashTimeoutMsg{})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	if mm.errMsg != "" {
		t.Errorf("got errMsg %q, want empty after timeout", mm.errMsg)
	}
}

// TestModelUpdateKeyPressCtrlC verifies ctrl+c quits the program.
func TestModelUpdateKeyPressCtrlC(t *testing.T) {
	m := &model{game: &fakeGame{}}

	newM, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	_, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	isQuitMsg(t, cmd)
}

// TestModelUpdateKeyPressGameOverEnter verifies Enter quits in game_over phase.
func TestModelUpdateKeyPressGameOverEnter(t *testing.T) {
	m := &model{game: &fakeGame{}, phase: "game_over"}

	newM, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	isQuitMsg(t, cmd)
}

// TestModelUpdateWSClose verifies WebSocket close quits the program.
func TestModelUpdateWSClose(t *testing.T) {
	m := &model{game: &fakeGame{}}

	newM, cmd := m.Update(wsCloseMsg{code: 1000})
	_, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	isQuitMsg(t, cmd)
}

// TestModelKeyDelegatesSend verifies a key that yields a command delegates to
// the game client and returns a send command.
func TestModelKeyDelegatesSend(t *testing.T) {
	f := &fakeGame{keyCmd: client.Command{Type: "play_card"}, keySend: true}
	m := &model{game: f}

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if f.keyCalls != 1 {
		t.Errorf("got keyCalls %d, want 1", f.keyCalls)
	}
	if cmd == nil {
		t.Errorf("got nil cmd, want a send command")
	}
}

// TestModelKeyDelegatesStatus verifies a key that yields a status flashes it.
func TestModelKeyDelegatesStatus(t *testing.T) {
	f := &fakeGame{keyStatus: "Not your turn"}
	m := &model{game: f}

	newM, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	if mm.errMsg != "Not your turn" {
		t.Errorf("got errMsg %q, want Not your turn", mm.errMsg)
	}
	isFlashTimer(t, cmd)
}

// TestModelHandleSnapshotInvalid verifies invalid JSON sets an error and is not
// delegated to the game client.
func TestModelHandleSnapshotInvalid(t *testing.T) {
	f := &fakeGame{}
	m := &model{game: f}

	m.handleSnapshot([]byte(`not json`))

	if m.errMsg == "" {
		t.Error("got empty errMsg, want an error message")
	}
	if f.snapshotCalls != 0 {
		t.Errorf("got snapshotCalls %d, want 0", f.snapshotCalls)
	}
}

// TestModelHandleSnapshotScores verifies scores are decoded from the envelope.
func TestModelHandleSnapshotScores(t *testing.T) {
	f := &fakeGame{}
	m := &model{game: f}

	m.handleSnapshot([]byte(`{"phase":"playing","round_number":2,"scores":[13,0,13,0]}`))

	if m.phase != "playing" {
		t.Errorf("got phase %q, want playing", m.phase)
	}
	if m.roundNumber != 2 {
		t.Errorf("got roundNumber %d, want 2", m.roundNumber)
	}
	want := []int{13, 0, 13, 0}
	if len(m.scores) != len(want) {
		t.Fatalf("got scores %v, want %v", m.scores, want)
	}
	for i := range want {
		if m.scores[i] != want[i] {
			t.Errorf("scores[%d] = %d, want %d", i, m.scores[i], want[i])
		}
	}
}

// TestModelHandleSnapshotGameClientError verifies a game-client decode error
// is flashed to the user.
func TestModelHandleSnapshotGameClientError(t *testing.T) {
	f := &fakeGame{lastErr: "Failed to decode player snapshot"}
	m := &model{game: f}

	m.handleSnapshot([]byte(`{"phase":"playing"}`))

	if m.errMsg != "Failed to decode player snapshot" {
		t.Errorf("got errMsg %q, want Failed to decode player snapshot", m.errMsg)
	}
}

// TestModelRenderMainDelegates verifies the main area is produced by the game
// client once a snapshot has arrived.
func TestModelRenderMainDelegates(t *testing.T) {
	f := &fakeGame{renderOut: "GAMEAREA"}
	m := &model{game: f, snapshot: json.RawMessage(`{}`)}

	got := m.renderMain()
	if !strings.Contains(got, "GAMEAREA") {
		t.Errorf("got render %q, want to contain %q", got, "GAMEAREA")
	}
}

// TestModelUpdateKeyPressEscFirstPress verifies the first Escape sets the
// confirmation state and flashes a message.
func TestModelUpdateKeyPressEscFirstPress(t *testing.T) {
	m := &model{game: &fakeGame{}}

	newM, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	if !mm.escConfirm {
		t.Error("got escConfirm=false, want true")
	}
	if mm.errMsg != "Press Enter to quit" {
		t.Errorf("got errMsg %q, want Press Enter to quit", mm.errMsg)
	}
	isFlashTimer(t, cmd)
}

// TestModelUpdateKeyPressEscEnterQuits verifies Escape then Enter quits the
// program.
func TestModelUpdateKeyPressEscEnterQuits(t *testing.T) {
	m := &model{game: &fakeGame{}, escConfirm: true}

	newM, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	isQuitMsg(t, cmd)
}

// TestModelUpdateKeyPressEscCancelsOnOtherKey verifies that pressing a
// non-Enter key after the first Esc cancels the quit confirmation.
func TestModelUpdateKeyPressEscCancelsOnOtherKey(t *testing.T) {
	m := &model{game: &fakeGame{}, escConfirm: true}

	newM, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	if mm.escConfirm {
		t.Error("got escConfirm=true, want false after other key")
	}
}

// TestModelFlashTimeoutClearsEscConfirm verifies the flash timeout resets
// the quit confirmation state.
func TestModelFlashTimeoutClearsEscConfirm(t *testing.T) {
	m := &model{game: &fakeGame{}, escConfirm: true, errMsg: "Press Esc again to quit"}

	newM, _ := m.Update(flashTimeoutMsg{})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	if mm.escConfirm {
		t.Error("got escConfirm=true, want false after flash timeout")
	}
}

// HandleSnapshot records the snapshot delegation call.
func (f *fakeGame) HandleSnapshot(raw json.RawMessage) {
	f.snapshotCalls++
}

// LastError returns the configured last error.
func (f *fakeGame) LastError() string {
	return f.lastErr
}

// HandleKey records the key delegation call and returns configured results.
func (f *fakeGame) HandleKey(key tea.KeyPressMsg) (client.Command, bool, string) {
	f.keyCalls++
	return f.keyCmd, f.keySend, f.keyStatus
}

// Render returns the configured render output.
func (f *fakeGame) Render() string {
	return f.renderOut
}
