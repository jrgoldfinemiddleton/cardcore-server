package session

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
)

// TestSessionHandlePlayIncrementsSeq verifies that a valid play command
// increments the sequence number.
func TestSessionHandlePlayIncrementsSeq(t *testing.T) {
	g := &mockGame{}
	s := newSession("test", g, Config{}, nil)
	defer close(s.cancel)

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      0,
		},
		resp: resp,
	}

	<-resp

	if got, want := s.seq, 1; got != want {
		t.Errorf("seq: got %d, want %d", got, want)
	}
}

// TestSessionStaleSeqReturnsSnapshot verifies that a command with a stale
// seq returns a snapshot synchronously.
func TestSessionStaleSeqReturnsSnapshot(t *testing.T) {
	g := &mockGame{}
	s := newSession("test", g, Config{}, nil)
	defer close(s.cancel)

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      0,
		},
		resp: resp,
	}
	<-resp

	resp2 := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action2",
			Seq:      0,
		},
		resp: resp2,
	}

	res := <-resp2
	if res.Snapshot == nil {
		t.Error("got nil snapshot for stale_seq, want non-nil")
	}
	if res.Err == nil {
		t.Error("got nil error for stale_seq, want error")
	}
}

// TestSessionDuplicateActionIDReturnsCachedSnapshot verifies that a
// duplicate action_id returns the exact cached snapshot that was
// broadcast on the first play.
func TestSessionDuplicateActionIDReturnsCachedSnapshot(t *testing.T) {
	g := &mockGame{}
	s := newSession("test", g, Config{}, nil)
	defer close(s.cancel)

	ch := make(chan []byte, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch}

	resp1 := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      0,
		},
		resp: resp1,
	}
	<-resp1

	var broadcast []byte
	select {
	case broadcast = <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broadcast snapshot")
	}

	resp2 := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      1,
		},
		resp: resp2,
	}

	res := <-resp2
	if res.Snapshot == nil {
		t.Fatal("got nil snapshot for duplicate action_id, want non-nil")
	}
	if res.Err != nil {
		t.Errorf("got error for duplicate action_id, want nil")
	}
	if !bytes.Equal(res.Snapshot, broadcast) {
		t.Error("duplicate action_id snapshot does not match cached broadcast")
	}
}

// TestSessionSubscribePlayerKicksPrevious verifies that subscribing a
// player kicks the previous subscriber for that seat.
func TestSessionSubscribePlayerKicksPrevious(t *testing.T) {
	g := &mockGame{}
	s := newSession("test", g, Config{}, nil)
	defer close(s.cancel)

	ch1 := make(chan []byte, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch1}

	time.Sleep(100 * time.Millisecond)

	for len(ch1) > 0 {
		<-ch1
	}

	ch2 := make(chan []byte, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch2}

	time.Sleep(100 * time.Millisecond)

	_, ok := <-ch1
	if ok {
		t.Error("expected ch1 to be closed, but it was not")
	}
}

// TestSessionGameOverClosesSubscribers verifies that StepFinished closes
// all subscriber channels.
func TestSessionGameOverClosesSubscribers(t *testing.T) {
	g := &stepFinishedGame{}
	s := newSession("test", g, Config{}, nil)
	defer close(s.cancel)

	ch := make(chan []byte, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch}

	obsCh := make(chan []byte, subChanSize)
	s.cmds <- subscribeObserverCmd{ch: obsCh}

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      0,
		},
		resp: resp,
	}
	<-resp

	time.Sleep(100 * time.Millisecond)

	for len(ch) > 0 {
		<-ch
	}
	for len(obsCh) > 0 {
		<-obsCh
	}

	_, ok := <-ch
	if ok {
		t.Error("expected player channel to be closed after game over")
	}
	_, ok = <-obsCh
	if ok {
		t.Error("expected observer channel to be closed after game over")
	}
}

// TestSessionGoroutineExitsOnStepFinished verifies that the session
// goroutine exits after StepFinished without requiring cancel.
func TestSessionGoroutineExitsOnStepFinished(t *testing.T) {
	g := &stepFinishedGame{}
	s := newSession("test", g, Config{}, nil)

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      0,
		},
		resp: resp,
	}
	<-resp

	select {
	case <-s.done:
		// Goroutine exited as expected.
	case <-time.After(time.Second):
		t.Fatal("goroutine did not exit within 1 second after StepFinished")
	}
}

// TestSessionDrainCmdsClosesPendingSubscribers verifies that a
// subscribe command buffered while the goroutine exits on StepFinished
// has its channel closed by drainCmds so the caller does not block
// forever.
func TestSessionDrainCmdsClosesPendingSubscribers(t *testing.T) {
	g := &stepFinishedGame{}
	s := newSession("test", g, Config{}, nil)

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      0,
		},
		resp: resp,
	}

	ch := make(chan []byte, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch}

	<-resp

	select {
	case <-s.done:
		// Goroutine exited.
	case <-time.After(time.Second):
		t.Fatal("goroutine did not exit")
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected ch to be closed, got data")
		}
		// ch is closed by drainCmds.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ch not closed — drainCmds did not run")
	}
}

// TestSessionGoroutineExitsOnCancel verifies that closing the cancel
// channel causes the goroutine to exit.
func TestSessionGoroutineExitsOnCancel(t *testing.T) {
	g := &mockGame{}
	s := newSession("test", g, Config{}, nil)

	close(s.cancel)

	select {
	case <-s.done:
		// Goroutine exited as expected.
	case <-time.After(time.Second):
		t.Fatal("goroutine did not exit within 1 second")
	}
}

// TestSessionUnsubscribePlayerClosesChannel verifies that unsubscribing a
// player closes their channel.
func TestSessionUnsubscribePlayerClosesChannel(t *testing.T) {
	g := &mockGame{}
	s := newSession("test", g, Config{}, nil)
	defer close(s.cancel)

	ch := make(chan []byte, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch}

	s.cmds <- unsubscribeCmd{seat: 0, ch: ch}

	time.Sleep(50 * time.Millisecond)

	for len(ch) > 0 {
		<-ch
	}

	_, ok := <-ch
	if ok {
		t.Error("expected ch to be closed after unsubscribe, but it was not")
	}
}

// TestSessionUnsubscribeObserverClosesChannel verifies that unsubscribing
// an observer closes their channel.
func TestSessionUnsubscribeObserverClosesChannel(t *testing.T) {
	g := &mockGame{}
	s := newSession("test", g, Config{}, nil)
	defer close(s.cancel)

	ch := make(chan []byte, subChanSize)
	s.cmds <- subscribeObserverCmd{ch: ch}

	s.cmds <- unsubscribeCmd{seat: -1, ch: ch}

	time.Sleep(50 * time.Millisecond)

	for len(ch) > 0 {
		<-ch
	}

	_, ok := <-ch
	if ok {
		t.Error("expected observer ch to be closed after unsubscribe, but it was not")
	}
}

// TestDrainCmdsHandlesPlayCmd verifies that drainCmds sends an error
// result on a buffered playCmd's response channel.
func TestDrainCmdsHandlesPlayCmd(t *testing.T) {
	g := &mockGame{}
	s := newSession("test", g, Config{}, nil)
	defer close(s.cancel)

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      0,
		},
		resp: resp,
	}

	// drainCmds is called directly — no goroutine involvement needed.
	s.drainCmds()

	select {
	case result := <-resp:
		if result.Err == nil {
			t.Fatal("expected error from drainCmds, got nil")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("drainCmds did not send on playCmd.resp")
	}
}

// TestSessionSeqMonotonicity verifies that seq increases monotonically
// across multiple plays.
func TestSessionSeqMonotonicity(t *testing.T) {
	g := &mockGame{}
	s := newSession("test", g, Config{}, nil)
	defer close(s.cancel)

	for i := range 5 {
		resp := make(chan SubmitResult, 1)
		s.cmds <- playCmd{
			seat: 0,
			msg: &api.InboundMessage{
				Type:     "test",
				ActionID: "action" + string(rune('0'+i)),
				Seq:      i,
			},
			resp: resp,
		}
		<-resp
	}

	if got, want := s.seq, 5; got != want {
		t.Errorf("seq: got %d, want %d", got, want)
	}
}

// TestSessionStaleSeqNilSnapshot returns error without snapshot when
// playerSnapshot fails to marshal. The client must still receive the
// stale_seq error so it knows to resync, but nil snapshots are never
// sent on channels or in responses.
func TestSessionStaleSeqNilSnapshot(t *testing.T) {
	g := &playerSnapshotUnmarshalableGame{}
	s := newSession("test", g, Config{}, nil)
	defer close(s.cancel)

	// Advance seq so the next command is stale.
	s.seq = 1

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      0,
		},
		resp: resp,
	}

	res := <-resp
	if res.Err == nil {
		t.Fatal("expected error for stale_seq, got nil")
	}
	if res.Err.ErrorCode != api.ErrStaleSeq {
		t.Errorf("got error code %q, want %q", res.Err.ErrorCode, api.ErrStaleSeq)
	}
	if res.Snapshot != nil {
		t.Error("got snapshot for stale_seq with marshal failure, want nil")
	}
}

// TestSessionMarshalFailureTerminates verifies that the session
// terminates when a snapshot fails to marshal after a successful
// action. The client receives an internal error and the session
// goroutine exits.
func TestSessionMarshalFailureTerminates(t *testing.T) {
	g := &unmarshalableGame{}
	s := newSession("test", g, Config{}, nil)
	defer close(s.cancel)

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      0,
		},
		resp: resp,
	}

	res := <-resp
	if res.Err == nil {
		t.Fatal("expected error, got nil")
	}
	if res.Err.ErrorCode != api.ErrInternal {
		t.Errorf("got error code %q, want %q", res.Err.ErrorCode, api.ErrInternal)
	}
	if !s.finished {
		t.Error("expected session to be finished after marshal failure")
	}
	if _, ok := s.actionIDs["action1"]; ok {
		t.Error("action_id should not be cached after marshal failure")
	}
}

// TestSessionMarshalFailureBroadcastsError verifies that when a
// snapshot fails to marshal, all subscribers receive an internal_error
// broadcast before their channels are closed.
func TestSessionMarshalFailureBroadcastsError(t *testing.T) {
	g := &unmarshalableGame{}
	s := newSession("test", g, Config{}, nil)

	ch := make(chan []byte, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch}

	obsCh := make(chan []byte, subChanSize)
	s.cmds <- subscribeObserverCmd{ch: obsCh}

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      0,
		},
		resp: resp,
	}
	<-resp

	// Wait for goroutine to exit after terminateOnMarshalFailure.
	select {
	case <-s.done:
		// Goroutine exited as expected.
	case <-time.After(time.Second):
		t.Fatal("goroutine did not exit within 1 second after marshal failure")
	}

	// Verify subscribers received the error broadcast.
	var errMsg *api.ErrorMessage
	for len(ch) > 0 {
		b := <-ch
		var em api.ErrorMessage
		if json.Unmarshal(b, &em) == nil && em.ErrorCode == api.ErrInternal {
			errMsg = &em
		}
	}
	if errMsg == nil {
		t.Error("expected player subscriber to receive internal_error broadcast")
	}

	for len(obsCh) > 0 {
		<-obsCh
	}

	// Verify channels are closed.
	_, ok := <-ch
	if ok {
		t.Error("expected player channel to be closed after marshal failure")
	}
	_, ok = <-obsCh
	if ok {
		t.Error("expected observer channel to be closed after marshal failure")
	}
}

// TestSessionTurnTimeoutFires verifies that when a human seat's turn
// arrives and no command is received within the timeout, the session
// auto-plays an AI move and broadcasts the resulting snapshot.
func TestSessionTurnTimeoutFires(t *testing.T) {
	timeout := 100
	g := &timeoutGame{turnSeat: 0, seatCount: 2}
	s := newSession("test", g, Config{
		Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		},
		TurnTimeoutMS: &timeout,
	}, nil)

	// scheduleAI() is called at goroutine startup, so the first turn
	// timeout is already set for seat 0 (human). No command needed.

	// Wait for the timeout to fire and AI to play.
	time.Sleep(200 * time.Millisecond)

	// Stop the goroutine and wait for it to exit before reading seq
	// to avoid data race.
	close(s.cancel)
	<-s.done

	if got, want := s.seq, 1; got < want {
		t.Errorf("seq: got %d, want at least %d after timeout AI play", got, want)
	}
}

// TestSessionScheduleAITerminatesOnFinished verifies that when AIPlay
// returns StepFinished from within scheduleAI, the session terminates
// and the goroutine exits.
func TestSessionScheduleAITerminatesOnFinished(t *testing.T) {
	g := &aiPlayFinishedGame{}
	s := newSession("test", g, Config{
		Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		},
	}, nil)

	ch := make(chan []byte, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch}

	obsCh := make(chan []byte, subChanSize)
	s.cmds <- subscribeObserverCmd{ch: obsCh}

	time.Sleep(100 * time.Millisecond)

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      0,
		},
		resp: resp,
	}
	<-resp

	select {
	case <-s.done:
		// Goroutine exited as expected.
	case <-time.After(time.Second):
		t.Fatal("goroutine did not exit within 1 second after AIPlay StepFinished")
	}

	if !s.finished {
		t.Error("expected session to be finished after AIPlay StepFinished")
	}

	for len(ch) > 0 {
		<-ch
	}
	for len(obsCh) > 0 {
		<-obsCh
	}

	_, ok := <-ch
	if ok {
		t.Error("expected player channel to be closed after game over")
	}
	_, ok = <-obsCh
	if ok {
		t.Error("expected observer channel to be closed after game over")
	}
}

// TestSessionScheduleAIHandlesPause verifies that when AIPlay
// returns StepPause from within scheduleAI (e.g., completing a trick),
// the session calls autoResume and continues the game.
func TestSessionScheduleAIHandlesPause(t *testing.T) {
	g := &aiPlayPauseGame{}
	zeroDelay := 0
	s := newSession("test", g, Config{
		Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		},
		PacingDelayMS: &zeroDelay,
	}, nil)

	ch := make(chan []byte, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch}

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      0,
		},
		resp: resp,
	}
	<-resp

	// Wait for autoResume to process the pause and the second AIPlay
	// to finish the game.
	select {
	case <-s.done:
		// Goroutine exited as expected.
	case <-time.After(time.Second):
		t.Fatal("goroutine did not exit within 1 second")
	}

	if !s.finished {
		t.Error("expected session to be finished after AIPlay StepPause + Resume")
	}
	if got, want := s.seq, 3; got != want {
		t.Errorf("seq: got %d, want %d", got, want)
	}

	// No internal_error should have been broadcast.
	for len(ch) > 0 {
		b := <-ch
		var em api.ErrorMessage
		if json.Unmarshal(b, &em) == nil && em.ErrorCode == api.ErrInternal {
			t.Error("got unexpected internal_error broadcast for normal StepPause")
		}
	}
}

// TestSessionScheduleAIStopsOnInvalidTurn verifies that when game.Turn
// returns an invalid seat, scheduleAI returns without terminating the
// session.
func TestSessionScheduleAIStopsOnInvalidTurn(t *testing.T) {
	g := &invalidTurnGame{}
	s := newSession("test", g, Config{
		Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		},
	}, nil)

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      0,
		},
		resp: resp,
	}
	<-resp

	// Give scheduleAI time to check Turn() and return.
	time.Sleep(100 * time.Millisecond)

	if s.finished {
		t.Error("expected session to not be finished after invalid Turn()")
	}

	close(s.cancel)
	select {
	case <-s.done:
		// Goroutine exited after cancel.
	case <-time.After(time.Second):
		t.Fatal("goroutine did not exit after cancel")
	}
}

// TestSessionInitialTurnTimeoutFires verifies that the turn timeout
// fires on the very first turn even when no command has been received
// yet.
func TestSessionInitialTurnTimeoutFires(t *testing.T) {
	timeout := 100
	g := &timeoutGame{turnSeat: 0, seatCount: 2}
	s := newSession("test", g, Config{
		Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		},
		TurnTimeoutMS: &timeout,
	}, nil)

	// No command sent — the timeout should fire from the initial
	// scheduleAI() call in run().
	time.Sleep(200 * time.Millisecond)

	// Stop the goroutine and wait for it to exit before reading seq
	// to avoid data race.
	close(s.cancel)
	<-s.done

	if got, want := s.seq, 1; got < want {
		t.Errorf("seq: got %d, want at least %d after initial timeout AI play", got, want)
	}
}
