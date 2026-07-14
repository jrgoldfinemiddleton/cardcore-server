package session

import (
	"bytes"
	"container/list"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
)

// pauseSpyGame is a mock Game that tracks SetPaused/Paused state.
type pauseSpyGame struct {
	mockGame
	paused bool
}

// TestHumanCount verifies that humanCount returns the number of human seats.
func TestHumanCount(t *testing.T) {
	cases := []struct {
		name  string
		seats []SeatConfig
		want  int
	}{
		{
			name:  "1 human 3 AI",
			seats: []SeatConfig{{Type: SeatHuman}, {Type: SeatAI}, {Type: SeatAI}, {Type: SeatAI}},
			want:  1,
		},
		{
			name: "2 human 2 AI",
			seats: []SeatConfig{
				{Type: SeatHuman},
				{Type: SeatHuman},
				{Type: SeatAI},
				{Type: SeatAI},
			},
			want: 2,
		},
		{
			name:  "0 human 4 AI",
			seats: []SeatConfig{{Type: SeatAI}, {Type: SeatAI}, {Type: SeatAI}, {Type: SeatAI}},
			want:  0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &session{config: Config{Seats: tc.seats}}
			if got := s.humanCount(); got != tc.want {
				t.Errorf("humanCount() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestSessionHandlePlayIncrementsSeq verifies that a valid play command
// increments the sequence number.
func TestSessionHandlePlayIncrementsSeq(t *testing.T) {
	g := &mockGame{}
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
	defer close(s.cancel)

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      1,
		},
		resp: resp,
	}

	<-resp

	if got, want := s.seq, 2; got != want {
		t.Errorf("seq: got %d, want %d", got, want)
	}
}

// TestSessionStaleSeqReturnsSnapshot verifies that a command with a stale
// seq returns a snapshot synchronously.
func TestSessionStaleSeqReturnsSnapshot(t *testing.T) {
	g := &mockGame{}
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
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
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
	defer close(s.cancel)

	ch := make(chan SubscriberMessage, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch}

	resp1 := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      1,
		},
		resp: resp1,
	}
	<-resp1

	var broadcast []byte
	select {
	case msg := <-ch:
		broadcast = msg.Data
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broadcast snapshot")
	}

	resp2 := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      2,
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
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
	defer close(s.cancel)

	ch1 := make(chan SubscriberMessage, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch1}

	time.Sleep(100 * time.Millisecond)

	for len(ch1) > 0 {
		<-ch1
	}

	ch2 := make(chan SubscriberMessage, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch2}

	time.Sleep(100 * time.Millisecond)

	_, ok := <-ch1
	if ok {
		t.Error("expected ch1 to be closed, but it was not")
	}
}

// TestSessionInitialSnapshotSeq verifies that handleSubscribePlayer
// sends a snapshot with seq >= 1 on the initial subscription.
func TestSessionInitialSnapshotSeq(t *testing.T) {
	g := &seqSnapshotGame{}
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
	defer close(s.cancel)

	ch := make(chan SubscriberMessage, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch}

	var msg SubscriberMessage
	select {
	case msg = <-ch:
		if msg.CloseCode != 0 {
			t.Fatalf("got close code %d, want snapshot", msg.CloseCode)
		}
		if len(msg.Data) == 0 {
			t.Error("got empty initial snapshot, want non-empty")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial snapshot")
	}

	var snap struct {
		Seq int `json:"seq"`
	}
	if err := json.Unmarshal(msg.Data, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.Seq < 1 {
		t.Errorf("initial snapshot seq: got %d, want >= 1", snap.Seq)
	}
}

// TestSessionGameOverClosesSubscribers verifies that StepFinished closes
// all subscriber channels.
func TestSessionGameOverClosesSubscribers(t *testing.T) {
	g := &stepFinishedGame{}
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
	defer close(s.cancel)

	ch := make(chan SubscriberMessage, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch}

	obsCh := make(chan SubscriberMessage, subChanSize)
	s.cmds <- subscribeObserverCmd{ch: obsCh}

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      1,
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
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      1,
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
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      1,
		},
		resp: resp,
	}

	ch := make(chan SubscriberMessage, subChanSize)
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
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)

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
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
	defer close(s.cancel)

	ch := make(chan SubscriberMessage, subChanSize)
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
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
	defer close(s.cancel)

	ch := make(chan SubscriberMessage, subChanSize)
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
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
	close(s.cancel)
	<-s.done // Wait for the goroutine to exit so it does not race on s.cmds.

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      1,
		},
		resp: resp,
	}

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
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
	defer close(s.cancel)

	for i := range 5 {
		resp := make(chan SubmitResult, 1)
		s.cmds <- playCmd{
			seat: 0,
			msg: &api.InboundMessage{
				Type:     "test",
				ActionID: "action" + string(rune('0'+i)),
				Seq:      i + 1,
			},
			resp: resp,
		}
		<-resp
	}

	if got, want := s.seq, 6; got != want {
		t.Errorf("seq: got %d, want %d", got, want)
	}
}

// TestSessionStaleSeqMarshalFailure terminates the session when a
// stale seq command arrives but the snapshot fails to marshal. A
// marshal failure is always fatal; the client receives an internal
// error and the session terminates.
func TestSessionStaleSeqMarshalFailure(t *testing.T) {
	g := &playerSnapshotUnmarshalableGame{}
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
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
	if res.Snapshot != nil {
		t.Error("got snapshot for marshal failure, want nil")
	}

	// Session should terminate after marshal failure.
	select {
	case <-s.done:
		// Expected: goroutine exited.
	case <-time.After(time.Second):
		t.Fatal("goroutine did not exit after marshal failure")
	}
}

// TestSessionMarshalFailureTerminates verifies that the session
// terminates when a snapshot fails to marshal after a successful
// action. The client receives an internal error and the session
// goroutine exits.
func TestSessionMarshalFailureTerminates(t *testing.T) {
	g := &unmarshalableGame{}
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
	defer close(s.cancel)

	// Allow the goroutine to attempt the initial broadcast and exit.
	select {
	case <-s.done:
	case <-time.After(time.Second):
		t.Fatal("goroutine did not exit after initial broadcast marshal failure")
	}

	if !s.finished {
		t.Error("expected session to be finished after initial broadcast marshal failure")
	}
}

// TestSessionMarshalFailureBroadcastsError verifies that when a
// snapshot fails to marshal after subscribers are present, all
// subscribers receive a 1011 close code broadcast before their channels
// are closed.
func TestSessionMarshalFailureBroadcastsError(t *testing.T) {
	g := &playerSnapshotUnmarshalableGame{}
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
	defer close(s.cancel)

	ch := make(chan SubscriberMessage, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch}

	obsCh := make(chan SubscriberMessage, subChanSize)
	s.cmds <- subscribeObserverCmd{ch: obsCh}

	// Consume the initial broadcast (observer snapshot succeeds, player
	// snapshot fails — but handleSubscribePlayer checks for nil and calls
	// terminateOnMarshalFailure if it fails).
	time.Sleep(50 * time.Millisecond)

	// Wait for goroutine to exit after terminateOnMarshalFailure.
	select {
	case <-s.done:
		// Goroutine exited as expected.
	case <-time.After(time.Second):
		t.Fatal("goroutine did not exit within 1 second after marshal failure")
	}

	// Verify subscribers received the 1011 close code broadcast.
	var gotCloseCode bool
	for len(ch) > 0 {
		msg := <-ch
		if msg.CloseCode == 1011 {
			gotCloseCode = true
		}
	}
	if !gotCloseCode {
		t.Error("expected player subscriber to receive 1011 close code broadcast")
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
	}, DefaultDelays{}, nil)

	// driveTurns() is called at goroutine startup, so the first turn
	// timeout is already set for seat 0 (human). No command needed.

	// Wait for the timeout to fire and AI to play.
	time.Sleep(200 * time.Millisecond)

	// Stop the goroutine and wait for it to exit before reading seq
	// to avoid data race.
	close(s.cancel)
	<-s.done

	if got, want := s.seq, 2; got < want {
		t.Errorf("seq: got %d, want at least %d after timeout AI play", got, want)
	}
}

// TestSessionDriveTurnsTerminatesOnInvalidTurn verifies that when
// game.Turn returns an invalid seat, driveTurns treats it as a fatal
// error and terminates the session.
func TestSessionDriveTurnsTerminatesOnInvalidTurn(t *testing.T) {
	g := &invalidTurnGame{}
	s := newSession("test", g, Config{
		Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		},
	}, DefaultDelays{}, nil)

	select {
	case <-s.done:
		// Goroutine exited after fatal error on invalid Turn().
	case <-time.After(time.Second):
		t.Fatal("goroutine did not exit after invalid Turn()")
	}

	if !s.finished {
		t.Error("expected session to be finished after invalid Turn()")
	}
}

// TestSessionInitialTurnTimeoutFires verifies that a human player's
// initial turn times out and an AI move is played.
func TestSessionInitialTurnTimeoutFires(t *testing.T) {
	timeout := 100
	g := &timeoutGame{turnSeat: 0, seatCount: 2}
	s := newSession("test", g, Config{
		Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		},
		TurnTimeoutMS: &timeout,
	}, DefaultDelays{}, nil)

	// No command sent — the timeout should fire from the initial
	// driveTurns() call in run().
	time.Sleep(200 * time.Millisecond)

	// Stop the goroutine and wait for it to exit before reading seq
	// to avoid data race.
	close(s.cancel)
	<-s.done

	if got, want := s.seq, 2; got < want {
		t.Errorf("seq: got %d, want at least %d after initial timeout AI play", got, want)
	}
}

// TestSessionBroadcastsDeadlineAfterHumanPlay verifies that when a
// human play completes and the next turn is human, the session
// broadcasts a snapshot carrying the newly-set turn_deadline_ms.
func TestSessionBroadcastsDeadlineAfterHumanPlay(t *testing.T) {
	timeout := 10000
	g := &deadlineBroadcastGame{}
	s := newSession("test", g, Config{
		Seats:         []SeatConfig{{Type: SeatHuman}},
		TurnTimeoutMS: &timeout,
	}, DefaultDelays{}, nil)
	defer close(s.cancel)

	ch := make(chan SubscriberMessage, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch}

	// Drain the subscription snapshot generated after the initial driveTurns.
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscription snapshot")
	}

	// Trigger a play. The handler clears the old deadline, schedules the
	// deadline for the next turn, and stamps it onto the snapshot before
	// broadcasting.
	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      1,
		},
		resp: resp,
	}
	<-resp

	deadlineSeen := false
deadline:
	for {
		select {
		case msg := <-ch:
			if msg.CloseCode != 0 {
				break deadline
			}
			var snap struct {
				TurnDeadlineMS int64 `json:"turn_deadline_ms"`
			}
			if err := json.Unmarshal(msg.Data, &snap); err != nil {
				t.Fatalf("unmarshal snapshot: %v", err)
			}
			if snap.TurnDeadlineMS > 0 {
				deadlineSeen = true
				break deadline
			}
		case <-time.After(200 * time.Millisecond):
			break deadline
		}
	}
	if !deadlineSeen {
		t.Fatal("expected a snapshot with turn_deadline_ms > 0 after play")
	}
}

// TestSessionDriveTurnsTerminatesOnFinished verifies that when AIPlay
// returns StepFinished from within driveTurns, the session terminates
// and the goroutine exits.
func TestSessionDriveTurnsTerminatesOnFinished(t *testing.T) {
	g := &aiPlayFinishedGame{}
	zeroDelay := 0
	s := newSession("test", g, Config{
		Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		},
		AIActionDelayMS: &zeroDelay,
	}, DefaultDelays{}, nil)

	ch := make(chan SubscriberMessage, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch}

	obsCh := make(chan SubscriberMessage, subChanSize)
	s.cmds <- subscribeObserverCmd{ch: obsCh}

	time.Sleep(100 * time.Millisecond)

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      1,
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

// TestSessionDriveTurnsHandlesPause verifies that when AIPlay
// returns StepPause from within driveTurns (e.g., completing a trick),
// the session calls resumePauses and continues the game.
func TestSessionDriveTurnsHandlesPause(t *testing.T) {
	g := &aiPlayPauseGame{}
	zeroDelay := 0
	s := newSession("test", g, Config{
		Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		},
		AIActionDelayMS: &zeroDelay,
	}, DefaultDelays{}, nil)

	ch := make(chan SubscriberMessage, subChanSize)
	s.cmds <- subscribePlayerCmd{seat: 0, ch: ch}

	resp := make(chan SubmitResult, 1)
	s.cmds <- playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "test",
			ActionID: "action1",
			Seq:      1,
		},
		resp: resp,
	}
	<-resp

	// Wait for resumePauses to process the pause and the second AIPlay
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
	if got, want := s.seq, 5; got != want {
		t.Errorf("seq: got %d, want %d", got, want)
	}

	// No close code should have been broadcast.
	for len(ch) > 0 {
		msg := <-ch
		if msg.CloseCode != 0 {
			t.Error("got unexpected close code broadcast for normal StepPause")
		}
	}
}

// TestEvictLRUActionID verifies that the LRU cache evicts the oldest entry
// and cleans up both the list and index.
func TestEvictLRUActionID(t *testing.T) {
	s := &session{
		id:            "test",
		game:          &mockGame{},
		config:        Config{Seats: []SeatConfig{{Type: SeatHuman}}},
		actionIDs:     make(map[string][]byte),
		actionIDList:  list.New(),
		actionIDIndex: make(map[string]*list.Element),
		logger:        slog.Default(),
	}

	// Manually populate the cache with 3 entries.
	// PushFront matches cacheActionID: front = most recent.
	for _, id := range []string{"first", "second", "third"} {
		// Store dummy snapshot as map value.
		s.actionIDs[id] = []byte(id)
		el := s.actionIDList.PushFront(id)
		s.actionIDIndex[id] = el
	}

	// Evict should remove "first" (oldest, now at back).
	s.evictLRUActionID()

	if _, ok := s.actionIDs["first"]; ok {
		t.Error("expected 'first' to be evicted from actionIDs")
	}
	if _, ok := s.actionIDIndex["first"]; ok {
		t.Error("expected 'first' to be evicted from actionIDIndex")
	}
	if s.actionIDList.Len() != 2 {
		t.Errorf("got list length %d, want 2", s.actionIDList.Len())
	}

	// Verify remaining entries are intact.
	for _, id := range []string{"second", "third"} {
		if _, ok := s.actionIDs[id]; !ok {
			t.Errorf("expected %q to remain in actionIDs", id)
		}
		if _, ok := s.actionIDIndex[id]; !ok {
			t.Errorf("expected %q to remain in actionIDIndex", id)
		}
	}
}

// TestEvictLRUActionIDEmptyList verifies that evicting from an empty list
// does not panic.
func TestEvictLRUActionIDEmptyList(t *testing.T) {
	s := &session{
		id:            "test",
		game:          &mockGame{},
		config:        Config{Seats: []SeatConfig{{Type: SeatHuman}}},
		actionIDs:     make(map[string][]byte),
		actionIDList:  list.New(),
		actionIDIndex: make(map[string]*list.Element),
		logger:        slog.Default(),
	}

	// Should not panic on empty list.
	s.evictLRUActionID()

	if s.actionIDList.Len() != 0 {
		t.Errorf("got list length %d, want 0", s.actionIDList.Len())
	}
}

// TestIsHumanSeatBounds verifies out-of-range seat handling.
func TestIsHumanSeatBounds(t *testing.T) {
	s := &session{
		config: Config{Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		}},
		logger: slog.Default(),
	}

	if s.isHumanSeat(-1) {
		t.Error("isHumanSeat(-1): got true, want false")
	}
	if s.isHumanSeat(2) {
		t.Error("isHumanSeat(2): got true, want false")
	}
	if !s.isHumanSeat(0) {
		t.Error("isHumanSeat(0): got false, want true")
	}
	if s.isHumanSeat(1) {
		t.Error("isHumanSeat(1): got false, want false")
	}
}

// TestPlayerSnapshotMarshal verifies success and failure paths for player
// snapshot marshaling.
func TestPlayerSnapshotMarshal(t *testing.T) {
	s := &session{
		id:     "test",
		seq:    42,
		config: Config{Seats: []SeatConfig{{Type: SeatHuman}}},
		logger: slog.Default(),
	}

	// Success path: mockGame returns nil which marshals to "null".
	s.game = &mockGame{}
	got := s.playerSnapshot(0)
	if got == nil {
		t.Fatal("playerSnapshot success: got nil, want non-nil bytes")
	}
	want := []byte("null")
	if !bytes.Equal(got, want) {
		t.Errorf("playerSnapshot success: got %q, want %q", got, want)
	}

	// Failure path: unmarshalable snapshot returns nil.
	s.game = &playerSnapshotUnmarshalableGame{}
	got = s.playerSnapshot(0)
	if got != nil {
		t.Errorf("playerSnapshot failure: got %v, want nil", got)
	}
}

// TestObserverSnapshotMarshal verifies success and failure paths for observer
// snapshot marshaling.
func TestObserverSnapshotMarshal(t *testing.T) {
	s := &session{
		id:     "test",
		seq:    7,
		config: Config{Seats: []SeatConfig{{Type: SeatHuman}}},
		logger: slog.Default(),
	}

	// Success path: mockGame returns nil which marshals to "null".
	s.game = &mockGame{}
	got := s.observerSnapshot()
	if got == nil {
		t.Fatal("observerSnapshot success: got nil, want non-nil bytes")
	}
	want := []byte("null")
	if !bytes.Equal(got, want) {
		t.Errorf("observerSnapshot success: got %q, want %q", got, want)
	}

	// Failure path: unmarshalable snapshot returns nil.
	s.game = &unmarshalableGame{}
	got = s.observerSnapshot()
	if got != nil {
		t.Errorf("observerSnapshot failure: got %v, want nil", got)
	}
}

// TestSessionInitialDisplayDelay verifies that the goroutine sleeps for
// the game's DisplayDelay after the initial broadcast.
func TestSessionInitialDisplayDelay(t *testing.T) {
	g := &delayGame{delay: 50}
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
	defer close(s.cancel)

	start := time.Now()
	select {
	case <-s.done:
	case <-time.After(500 * time.Millisecond):
	}
	elapsed := time.Since(start)

	// The goroutine should have slept at least 50ms for the display delay
	// before AIPlay returned StepFinished and the session exited.
	if elapsed < 50*time.Millisecond {
		t.Errorf("goroutine did not wait for display delay: elapsed %v, want >= 50ms", elapsed)
	}
}

// TestHandlePauseCmdSuccess verifies that a valid pause command sets the
// paused state and broadcasts a snapshot.
func TestHandlePauseCmdSuccess(t *testing.T) {
	g := &pauseSpyGame{}
	timeout := 5000
	cfg := Config{Seats: []SeatConfig{{Type: SeatHuman}}}
	delays := DefaultDelays{TurnTimeoutMS: timeout}
	s := newSession("test", g, cfg, delays, nil)
	stopSessionGoroutine(s)

	s.waitingForHuman = true
	s.turnDeadline = time.Now().Add(time.Duration(timeout) * time.Millisecond)

	resp := make(chan SubmitResult, 1)
	s.handlePauseCmd(playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "pause",
			ActionID: "pause-1",
			Seq:      1,
		},
		resp: resp,
	})

	res := <-resp
	if res.Err != nil {
		t.Fatalf("handlePauseCmd: got error %v, want nil", res.Err)
	}
	if !s.paused {
		t.Fatalf("s.paused = false, want true")
	}
	if !g.Paused() {
		t.Fatalf("game.Paused() = false, want true")
	}
	if s.seq != 2 {
		t.Fatalf("seq = %d, want 2", s.seq)
	}
	if s.pauseRemaining <= 0 {
		t.Fatalf("pauseRemaining = %v, want > 0", s.pauseRemaining)
	}
}

// TestHandlePauseCmdRejectsMultiHuman verifies that pause is rejected
// when the session has more than one human seat.
func TestHandlePauseCmdRejectsMultiHuman(t *testing.T) {
	g := &mockGame{}
	cfg := Config{Seats: []SeatConfig{
		{Type: SeatHuman},
		{Type: SeatHuman},
		{Type: SeatAI},
		{Type: SeatAI},
	}}
	s := newSession("test", g, cfg, DefaultDelays{}, nil)
	stopSessionGoroutine(s)

	s.waitingForHuman = true

	resp := make(chan SubmitResult, 1)
	s.handlePauseCmd(playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "pause",
			ActionID: "pause-1",
			Seq:      1,
		},
		resp: resp,
	})

	res := <-resp
	if res.Err == nil {
		t.Fatal("handlePauseCmd: got nil error, want pause_not_allowed")
	}
	if res.Err.ErrorCode != api.ErrPauseNotAllowed {
		t.Fatalf("error code = %q, want %q", res.Err.ErrorCode, api.ErrPauseNotAllowed)
	}
	if s.paused {
		t.Fatal("s.paused = true, want false")
	}
}

// TestHandlePauseCmdRejectsNotHumanTurn verifies that pause is rejected
// when the session is not waiting for a human turn.
func TestHandlePauseCmdRejectsNotHumanTurn(t *testing.T) {
	g := &mockGame{}
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
	stopSessionGoroutine(s)

	s.waitingForHuman = false

	resp := make(chan SubmitResult, 1)
	s.handlePauseCmd(playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "pause",
			ActionID: "pause-1",
			Seq:      1,
		},
		resp: resp,
	})

	res := <-resp
	if res.Err == nil {
		t.Fatal("handlePauseCmd: got nil error, want pause_not_allowed")
	}
	if res.Err.ErrorCode != api.ErrPauseNotAllowed {
		t.Fatalf("error code = %q, want %q", res.Err.ErrorCode, api.ErrPauseNotAllowed)
	}
}

// TestHandlePauseCmdRejectsAlreadyPaused verifies that pause is rejected
// when the game is already paused.
func TestHandlePauseCmdRejectsAlreadyPaused(t *testing.T) {
	g := &mockGame{}
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
	stopSessionGoroutine(s)

	s.waitingForHuman = true
	s.paused = true

	resp := make(chan SubmitResult, 1)
	s.handlePauseCmd(playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "pause",
			ActionID: "pause-1",
			Seq:      1,
		},
		resp: resp,
	})

	res := <-resp
	if res.Err == nil {
		t.Fatal("handlePauseCmd: got nil error, want pause_not_allowed")
	}
	if res.Err.ErrorCode != api.ErrPauseNotAllowed {
		t.Fatalf("error code = %q, want %q", res.Err.ErrorCode, api.ErrPauseNotAllowed)
	}
}

// TestHandleResumeCmdSuccess verifies that resume restores the paused state
// and recalculates the turn deadline.
func TestHandleResumeCmdSuccess(t *testing.T) {
	g := &mockGame{}
	timeout := 5000
	cfg := Config{Seats: []SeatConfig{{Type: SeatHuman}}}
	delays := DefaultDelays{TurnTimeoutMS: timeout}
	s := newSession("test", g, cfg, delays, nil)
	stopSessionGoroutine(s)

	s.waitingForHuman = true
	s.paused = true
	s.pauseRemaining = 2 * time.Second

	resp := make(chan SubmitResult, 1)
	s.handleResumeCmd(playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "resume",
			ActionID: "resume-1",
			Seq:      1,
		},
		resp: resp,
	})

	res := <-resp
	if res.Err != nil {
		t.Fatalf("handleResumeCmd: got error %v, want nil", res.Err)
	}
	if s.paused {
		t.Fatalf("s.paused = true, want false")
	}
	if g.Paused() {
		t.Fatalf("game.Paused() = true, want false")
	}
	if s.turnDeadline.IsZero() {
		t.Fatalf("turnDeadline is zero, want non-zero")
	}
	if s.seq != 2 {
		t.Fatalf("seq = %d, want 2", s.seq)
	}
}

// TestHandleResumeCmdRejectsNotPaused verifies that resume is rejected when
// the game is not paused.
func TestHandleResumeCmdRejectsNotPaused(t *testing.T) {
	g := &mockGame{}
	s := newSession("test", g, Config{Seats: []SeatConfig{{Type: SeatHuman}}}, DefaultDelays{}, nil)
	stopSessionGoroutine(s)

	s.waitingForHuman = true
	s.paused = false

	resp := make(chan SubmitResult, 1)
	s.handleResumeCmd(playCmd{
		seat: 0,
		msg: &api.InboundMessage{
			Type:     "resume",
			ActionID: "resume-1",
			Seq:      1,
		},
		resp: resp,
	})

	res := <-resp
	if res.Err == nil {
		t.Fatal("handleResumeCmd: got nil error, want pause_not_allowed")
	}
	if res.Err.ErrorCode != api.ErrPauseNotAllowed {
		t.Fatalf("error code = %q, want %q", res.Err.ErrorCode, api.ErrPauseNotAllowed)
	}
}

// TestAutoUnpauseOnHumanDisconnect verifies that a paused game auto-unpauses
// when a human seat disconnects.
func TestAutoUnpauseOnHumanDisconnect(t *testing.T) {
	g := &mockGame{}
	timeout := 5000
	cfg := Config{Seats: []SeatConfig{{Type: SeatHuman}}}
	delays := DefaultDelays{TurnTimeoutMS: timeout}
	s := newSession("test", g, cfg, delays, nil)
	stopSessionGoroutine(s)

	s.waitingForHuman = true
	s.paused = true
	s.pauseRemaining = 2 * time.Second

	ch := make(chan SubscriberMessage, subChanSize)
	s.players[0] = ch
	s.handleUnsubscribe(unsubscribeCmd{seat: 0, ch: ch})

	if s.paused {
		t.Fatalf("s.paused = true after human disconnect, want false")
	}
	if g.Paused() {
		t.Fatalf("game.Paused() = true after human disconnect, want false")
	}
	if s.turnDeadline.IsZero() {
		t.Fatalf("turnDeadline is zero after human disconnect, want non-zero")
	}
	if s.seq != 2 {
		t.Fatalf("seq = %d, want 2", s.seq)
	}
}

// TestAutoUnpauseSkippedForObserverDisconnect verifies that a paused game
// does not auto-unpause when an observer disconnects.
func TestAutoUnpauseSkippedForObserverDisconnect(t *testing.T) {
	g := &mockGame{}
	cfg := Config{Seats: []SeatConfig{{Type: SeatHuman}}}
	s := newSession("test", g, cfg, DefaultDelays{}, nil)
	stopSessionGoroutine(s)

	s.waitingForHuman = true
	s.paused = true

	ch := make(chan SubscriberMessage, subChanSize)
	s.observers = append(s.observers, ch)
	s.handleUnsubscribe(unsubscribeCmd{seat: -1, ch: ch})

	if !s.paused {
		t.Fatalf("s.paused = false after observer disconnect, want true")
	}
}

// SetPaused implements Game.SetPaused for pauseSpyGame.
func (p *pauseSpyGame) SetPaused(paused bool) {
	p.paused = paused
}

// Paused implements Game.Paused for pauseSpyGame.
func (p *pauseSpyGame) Paused() bool {
	return p.paused
}

// stopSessionGoroutine signals the session goroutine to exit and waits for it
// to finish. Use this in unit tests that call handlers directly and need to
// mutate session state without racing the goroutine.
func stopSessionGoroutine(s *session) {
	close(s.cancel)
	<-s.done
}
