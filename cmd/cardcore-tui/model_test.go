package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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
	humanTurn     bool
	inputDisabled bool
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

// TestModelUpdateError verifies recoverable error messages set a continue modal.
func TestModelUpdateError(t *testing.T) {
	m := &model{game: &fakeGame{}}

	newM, cmd := m.Update(wsErrorMsg{code: "out_of_turn", message: "Not your turn"})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	wantMsg := "AI played for you — your turn has passed. Press Enter to continue."
	if mm.errMsg != wantMsg {
		t.Errorf("got errMsg %q, want %q", mm.errMsg, wantMsg)
	}
	if !mm.modalContinue {
		t.Errorf("modalContinue = false, want true")
	}
	if cmd != nil {
		t.Errorf("expected no command for persistent modal, got %v", cmd)
	}
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
	m := &model{game: &fakeGame{}, phase: phaseGameOver}

	newM, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	isQuitMsg(t, cmd)
}

// TestModelUpdateWSClose verifies WebSocket close quits the program during a
// live game, but stays on the final screen once the game has ended.
func TestModelUpdateWSClose(t *testing.T) {
	t.Run("live game", func(t *testing.T) {
		m := &model{game: &fakeGame{}, phase: "playing"}

		newM, cmd := m.Update(wsCloseMsg{code: 1000})
		_, ok := newM.(*model)
		if !ok {
			t.Fatalf("Update returned %T, want *model", newM)
		}
		isQuitMsg(t, cmd)
	})

	t.Run("game over", func(t *testing.T) {
		m := &model{game: &fakeGame{}, phase: phaseGameOver}

		newM, cmd := m.Update(wsCloseMsg{code: 1000})
		_, ok := newM.(*model)
		if !ok {
			t.Fatalf("Update returned %T, want *model", newM)
		}
		if cmd != nil {
			t.Fatalf("expected nil cmd, got %v", cmd)
		}
	})
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

// TestModelCountdownStartsOnHumanTurn verifies a snapshot that yields a human
// turn with a server deadline starts the countdown timer and sets the deadline.
func TestModelCountdownStartsOnHumanTurn(t *testing.T) {
	f := &fakeGame{humanTurn: true}
	m := &model{game: f}
	deadline := time.Now().Add(30 * time.Second).UnixMilli()
	raw := fmt.Sprintf(`{"phase":"playing","turn_deadline_ms":%d}`, deadline)

	newM, cmd := m.Update(wsSnapshotMsg{raw: []byte(raw)})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	if !mm.humanTurn {
		t.Errorf("humanTurn = false, want true")
	}
	if mm.turnDeadline.IsZero() {
		t.Errorf("turnDeadline zero, want set")
	}
	if mm.turnDeadline.UnixMilli() != deadline {
		t.Errorf("turnDeadline = %d, want %d", mm.turnDeadline.UnixMilli(), deadline)
	}
	if mm.timeoutDisabled {
		t.Errorf("timeoutDisabled = true, want false")
	}
	if cmd == nil {
		t.Errorf("got nil cmd, want turnTick command")
	}
}

// TestModelCountdownDisabledWhenNoDeadline verifies the countdown feature is
// off when the snapshot carries no turn deadline.
func TestModelCountdownDisabledWhenNoDeadline(t *testing.T) {
	f := &fakeGame{humanTurn: true}
	m := &model{game: f}

	newM, cmd := m.Update(wsSnapshotMsg{raw: []byte(`{"phase":"playing"}`)})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	if !mm.turnDeadline.IsZero() {
		t.Errorf("turnDeadline set, want zero when timeout disabled")
	}
	if cmd != nil {
		t.Errorf("got cmd %v, want nil", cmd)
	}
}

// TestModelCountdownStopsOnNonHumanTurn verifies a snapshot that is not the
// human's turn clears the deadline.
func TestModelCountdownStopsOnNonHumanTurn(t *testing.T) {
	f := &fakeGame{humanTurn: false}
	m := &model{game: f, turnDeadline: time.Now().Add(time.Minute)}

	newM, cmd := m.Update(wsSnapshotMsg{raw: []byte(`{"phase":"playing"}`)})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	if !mm.turnDeadline.IsZero() {
		t.Errorf("turnDeadline set, want zero")
	}
	if cmd != nil {
		t.Errorf("got cmd %v, want nil", cmd)
	}
}

// TestModelTurnTickDisablesInputAtOneSecond verifies a tick when the deadline
// is within one second disables input and re-arms the timer.
func TestModelTurnTickDisablesInputAtOneSecond(t *testing.T) {
	f := &fakeGame{humanTurn: true}
	m := &model{game: f, turnDeadline: time.Now().Add(500 * time.Millisecond)}

	cmd := m.handleTurnTick()
	if !m.timeoutDisabled {
		t.Errorf("timeoutDisabled = false, want true")
	}
	if !f.inputDisabled {
		t.Errorf("inputDisabled = false on game client, want true")
	}
	if cmd == nil {
		t.Errorf("got nil cmd, want re-armed tick")
	}
}

// TestModelInputDisabledOnNonHumanTurn verifies a snapshot that is not the
// human's turn disables input so the hand is dimmed and keys are ignored.
func TestModelInputDisabledOnNonHumanTurn(t *testing.T) {
	f := &fakeGame{humanTurn: true}
	m := &model{game: f, phase: "playing", humanTurn: true}

	m.Update(wsSnapshotMsg{raw: []byte(`{"phase":"playing","turn":0}`)})
	if f.inputDisabled {
		t.Errorf("inputDisabled = true, want false on human turn")
	}

	f.humanTurn = false
	m.Update(wsSnapshotMsg{raw: []byte(`{"phase":"playing","turn":2}`)})
	if !f.inputDisabled {
		t.Errorf("inputDisabled = false, want true when not human turn")
	}
}

// TestModelTurnTickStopsWhenExpired verifies a tick at or after the deadline
// stops the countdown and disables input.
func TestModelTurnTickStopsWhenExpired(t *testing.T) {
	f := &fakeGame{humanTurn: true}
	m := &model{game: f, turnDeadline: time.Now().Add(-time.Millisecond)}

	cmd := m.handleTurnTick()
	if !m.timeoutDisabled {
		t.Errorf("timeoutDisabled = false, want true")
	}
	if !f.inputDisabled {
		t.Errorf("inputDisabled = false on game client, want true")
	}
	if !m.turnDeadline.IsZero() {
		t.Errorf("turnDeadline set, want zero after expiry")
	}
	if cmd != nil {
		t.Errorf("got cmd %v, want nil", cmd)
	}
}

// TestModelKeyPressIgnoredWhenTimeoutDisabled verifies game input is blocked
// when the timeout window has closed.
func TestModelKeyPressIgnoredWhenTimeoutDisabled(t *testing.T) {
	f := &fakeGame{keySend: true}
	m := &model{game: f, timeoutDisabled: true}

	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if f.keyCalls != 0 {
		t.Errorf("got keyCalls %d, want 0 (input blocked)", f.keyCalls)
	}
}

// TestModelKeyPressSetsPendingHumanAction verifies sending a command clears the
// deadline, marks a human action as pending, and disables input until the next
// snapshot so the hand is rendered dimmed immediately.
func TestModelKeyPressSetsPendingHumanAction(t *testing.T) {
	f := &fakeGame{keySend: true}
	m := &model{game: f, turnDeadline: time.Now().Add(time.Minute)}

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.pendingHumanAction {
		t.Errorf("pendingHumanAction = false, want true")
	}
	if !m.turnDeadline.IsZero() {
		t.Errorf("turnDeadline set, want zero after human action")
	}
	if !f.inputDisabled {
		t.Errorf("inputDisabled = false, want true after send")
	}
	if cmd == nil {
		t.Errorf("got nil cmd, want send command")
	}
}

// TestModelCommandSendFailureReEnablesInput verifies that a failed command send
// re-enables input and resets the pending/submitted state so the player can
// retry.
func TestModelCommandSendFailureReEnablesInput(t *testing.T) {
	f := &fakeGame{keySend: true}
	m := &model{game: f, turnDeadline: time.Now().Add(time.Minute)}

	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !f.inputDisabled {
		t.Fatal("inputDisabled = false, want true after send")
	}

	newM, cmd := m.Update(commandSentMsg{err: errors.New("send failed")})
	mm, ok := newM.(*model)
	if !ok {
		t.Fatalf("Update returned %T, want *model", newM)
	}
	if mm.pendingHumanAction {
		t.Errorf("pendingHumanAction = true, want false after send failure")
	}
	if f.inputDisabled {
		t.Errorf("inputDisabled = true, want false after send failure")
	}
	if mm.errMsg != "Failed to send command" {
		t.Errorf("errMsg = %q, want %q", mm.errMsg, "Failed to send command")
	}
	isFlashTimer(t, cmd)
}

// TestModelTimeoutDetectedWhenNoHumanAction verifies a transition from human
// turn to non-human turn without a pending action sets the AI-played status.
func TestModelTimeoutDetectedWhenNoHumanAction(t *testing.T) {
	f := &fakeGame{humanTurn: false}
	m := &model{game: f, phase: "playing", humanTurn: true}

	m.Update(wsSnapshotMsg{raw: []byte(`{"phase":"playing"}`)})
	if m.statusMsg != aiPlayedStatusMsg {
		t.Errorf("statusMsg = %q, want %q", m.statusMsg, aiPlayedStatusMsg)
	}
}

// TestModelTimeoutNotDetectedWhenPending verifies a human action in flight
// prevents the AI-played status from being shown.
func TestModelTimeoutNotDetectedWhenPending(t *testing.T) {
	f := &fakeGame{humanTurn: false}
	m := &model{
		game: f, phase: "playing", humanTurn: true,
		pendingHumanAction: true,
	}

	m.Update(wsSnapshotMsg{raw: []byte(`{"phase":"playing"}`)})
	if m.statusMsg != "" {
		t.Errorf("statusMsg = %q, want empty", m.statusMsg)
	}
	if m.pendingHumanAction {
		t.Errorf("pendingHumanAction = true, want false after snapshot")
	}
}

// TestModelPhaseChangeClearsStatus verifies a phase transition clears a
// generic status message.
func TestModelPhaseChangeClearsStatus(t *testing.T) {
	f := &fakeGame{humanTurn: false}
	m := &model{
		game: f, phase: "playing", humanTurn: false, statusMsg: "previous",
	}

	m.Update(wsSnapshotMsg{raw: []byte(`{"phase":"trick_complete"}`)})
	if m.statusMsg != "" {
		t.Errorf("statusMsg = %q, want empty after phase change", m.statusMsg)
	}
}

// TestModelAIPlayedHoldPreventsPhaseChangeClear verifies the AI-played status
// message is held for a minimum duration and survives a phase change.
func TestModelAIPlayedHoldPreventsPhaseChangeClear(t *testing.T) {
	f := &fakeGame{humanTurn: false}
	m := &model{game: f, phase: "playing", humanTurn: true}

	newM, cmd := m.Update(wsSnapshotMsg{raw: []byte(`{"phase":"trick_complete"}`)})
	mm := newM.(*model)
	if mm.statusMsg != aiPlayedStatusMsg {
		t.Errorf("statusMsg = %q, want %q", mm.statusMsg, aiPlayedStatusMsg)
	}
	if mm.aiPlayedHoldUntil.IsZero() {
		t.Error("aiPlayedHoldUntil zero, want set")
	}
	if cmd == nil {
		t.Fatal("got nil cmd, want hold timer")
	}
	msg := runCmd(t, cmd)
	if _, ok := msg.(aiPlayedHoldMsg); !ok {
		t.Fatalf("cmd returned %T, want aiPlayedHoldMsg", msg)
	}
}

// TestModelAIPlayedHoldExpires verifies that when the hold timer expires the
// AI-played status message is cleared.
func TestModelAIPlayedHoldExpires(t *testing.T) {
	m := &model{
		statusMsg:         aiPlayedStatusMsg,
		aiPlayedHoldUntil: time.Now().Add(-time.Millisecond),
	}

	newM, _ := m.Update(aiPlayedHoldMsg{})
	mm := newM.(*model)
	if mm.statusMsg != "" {
		t.Errorf("statusMsg = %q, want empty after hold expires", mm.statusMsg)
	}
}

// TestModelAIPlayedHoldClearedByHumanTurn verifies that the AI-played status
// message is cleared immediately when the human turn starts, even if the hold
// duration has not expired.
func TestModelAIPlayedHoldClearedByHumanTurn(t *testing.T) {
	f := &fakeGame{humanTurn: true}
	m := &model{
		game: f, phase: "playing", humanTurn: false,
		statusMsg:         aiPlayedStatusMsg,
		aiPlayedHoldUntil: time.Now().Add(time.Hour),
	}

	deadline := time.Now().Add(30 * time.Second).UnixMilli()
	raw := fmt.Appendf(nil, `{"phase":"playing","turn_deadline_ms":%d}`, deadline)
	newM, _ := m.Update(wsSnapshotMsg{raw: raw})
	mm := newM.(*model)
	if mm.statusMsg != "" {
		t.Errorf("statusMsg = %q, want empty on human turn", mm.statusMsg)
	}
}

// TestModelCountdownStatus verifies the countdown shows the remaining time
// until the client-side cutoff, which is one second before the server-side
// deadline.
func TestModelCountdownStatus(t *testing.T) {
	m := &model{turnDeadline: time.Now().Add(5500 * time.Millisecond)}
	got := m.countdownStatus()
	want := "Your turn (5s)"
	if got != want {
		t.Errorf("countdownStatus() = %q, want %q", got, want)
	}
}

// TestModelCountdownStatusExpired verifies the countdown status is clamped to
// zero when the deadline has passed.
func TestModelCountdownStatusExpired(t *testing.T) {
	m := &model{turnDeadline: time.Now().Add(-time.Millisecond)}
	got := m.countdownStatus()
	want := "Your turn (0s)"
	if got != want {
		t.Errorf("countdownStatus() = %q, want %q", got, want)
	}
}

// TestModelRenderFooterTimeoutDisabled verifies the footer shows the timeout
// message when input is disabled.
func TestModelRenderFooterTimeoutDisabled(t *testing.T) {
	m := &model{timeoutDisabled: true}
	got := m.renderFooter()
	if !strings.Contains(got, "Timeout - AI playing") {
		t.Errorf("renderFooter() = %q, want to contain 'Timeout - AI playing'", got)
	}
}

// runCmd executes a tea.Cmd and returns the resulting message.
func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("cmd is nil")
	}
	msg := cmd()
	if msg == nil {
		t.Fatal("cmd returned nil message")
	}
	return msg
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

// ResetSubmitted is a no-op for the fake game client.
func (f *fakeGame) ResetSubmitted() {}

// SetInputDisabled records the input-disabled flag.
func (f *fakeGame) SetInputDisabled(disabled bool) {
	f.inputDisabled = disabled
}

// IsHumanTurn returns the configured human-turn flag.
func (f *fakeGame) IsHumanTurn() bool {
	return f.humanTurn
}
