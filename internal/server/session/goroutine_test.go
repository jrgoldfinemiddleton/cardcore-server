package session

import (
	"bytes"
	"testing"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
)

// stepFinishedGame is a mock Game that always returns StepFinished.
type stepFinishedGame struct{}

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

// HandleAction implements Game.HandleAction for stepFinishedGame.
func (s *stepFinishedGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{Outcome: StepFinished}, nil
}

// AIPlay implements Game.AIPlay for stepFinishedGame.
func (s *stepFinishedGame) AIPlay(int) (StepResult, error) {
	return StepResult{}, nil
}

// Resume implements Game.Resume for stepFinishedGame.
func (s *stepFinishedGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn implements Game.Turn for stepFinishedGame.
func (s *stepFinishedGame) Turn() int {
	return 0
}

// PlayerSnapshot implements Game.PlayerSnapshot for stepFinishedGame.
func (s *stepFinishedGame) PlayerSnapshot(int, int) any {
	return nil
}

// ObserverSnapshot implements Game.ObserverSnapshot for stepFinishedGame.
func (s *stepFinishedGame) ObserverSnapshot(int) any {
	return nil
}
