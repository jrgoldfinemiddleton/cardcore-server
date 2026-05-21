package session

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
)

const (
	cmdChanSize       = 64
	subChanSize       = 64
	actionIDCacheSize = 1000

	errType = "error"
)

// session owns a single game instance and serializes all access to it.
type session struct {
	// id is the session identifier.
	id string
	// game is the game adapter instance.
	game Game
	// config is the session configuration (seats, delays).
	config Config

	// seq is the monotonically increasing snapshot sequence number.
	seq int
	// actionIDs caches marshaled snapshots keyed by action_id for
	// idempotent replay.
	actionIDs map[string][]byte

	// players maps seat index to subscriber channel.
	players map[int]chan []byte
	// observers holds all observer subscriber channels.
	observers []chan []byte

	// cmds receives commands from the Manager.
	cmds chan command
	// cancel signals the goroutine to shut down.
	cancel chan struct{}
	// done is closed when the goroutine exits.
	done chan struct{}

	// onDone is called by the goroutine when the game finishes.
	onDone func(State)

	// closeOnce ensures closeSubscribers is idempotent.
	// When Delete races with natural game completion, the goroutine
	// may call closeSubscribers from handleStepResult(StepFinished)
	// and then again from the <-cancel branch in a subsequent select
	// iteration. sync.Once prevents a double-close panic.
	closeOnce sync.Once

	// finished is set when the game reaches a terminal state,
	// signaling run() to exit after the current command completes.
	finished bool
}

// run is the session goroutine event loop.
func (s *session) run() {
	// done signals that the goroutine has exited and all subscriber
	// channels have been closed. Manager methods use <-done to detect
	// shutdown and avoid blocking on a dead session.
	defer close(s.done)
	for {
		select {
		case cmd := <-s.cmds:
			s.handleCommand(cmd)
			if s.finished {
				s.drainCmds()
				return
			}
		case <-s.cancel:
			s.closeSubscribers()
			s.drainCmds()
			return
		}
	}
}

// handleCommand dispatches a command to the appropriate handler.
func (s *session) handleCommand(cmd command) {
	switch c := cmd.(type) {
	case playCmd:
		s.handlePlay(c)
	case subscribePlayerCmd:
		s.handleSubscribePlayer(c)
	case subscribeObserverCmd:
		s.handleSubscribeObserver(c)
	case unsubscribeCmd:
		s.handleUnsubscribe(c)
	}
}

// handlePlay processes a human player's action.
func (s *session) handlePlay(c playCmd) {
	defer close(c.resp)

	// Client seq is behind the server. Return the latest snapshot so
	// the client can resync, along with an error. If the snapshot fails
	// to marshal, send the error only so the client receives a valid
	// response it can distinguish from a nil field.
	if c.msg.Seq < s.seq {
		snap := s.playerSnapshot(c.seat)
		result := SubmitResult{
			Err: &api.ErrorMessage{
				Type:      errType,
				ErrorCode: api.ErrStaleSeq,
				Message:   "client seq is behind server",
				ActionID:  c.msg.ActionID,
			},
		}
		if snap != nil {
			result.Snapshot = snap
		}
		c.resp <- result
		return
	}

	// Duplicate action_id: client resent a command that already
	// succeeded. Return the cached snapshot without mutating state.
	if cached, ok := s.actionIDs[c.msg.ActionID]; ok {
		result := SubmitResult{}
		if cached != nil {
			result.Snapshot = cached
		}
		c.resp <- result
		return
	}

	// Validate and apply the action through the game adapter.
	res, cmdErr := s.game.HandleAction(c.seat, c.msg)
	if cmdErr != nil {
		// Action rejected (wrong turn, illegal move, wrong phase).
		// Send the error to the player's subscription channel and
		// return it synchronously.
		s.sendError(c.seat, cmdErr.Code, cmdErr.Message, c.msg.ActionID)
		c.resp <- SubmitResult{
			Err: &api.ErrorMessage{
				Type:      errType,
				ErrorCode: cmdErr.Code,
				Message:   cmdErr.Message,
				ActionID:  c.msg.ActionID,
			},
		}
		return
	}

	// Action accepted. Increment seq, broadcast the new state to all
	// subscribers, and cache the snapshot for idempotent replay. Skip
	// caching if the snapshot fails to marshal so the cache never
	// contains nil entries that would break duplicate action_id replay.
	s.seq++
	s.broadcastSnapshot()
	snap := s.playerSnapshot(c.seat)
	if snap != nil {
		s.actionIDs[c.msg.ActionID] = snap
		if len(s.actionIDs) > actionIDCacheSize {
			s.evictArbitraryActionID()
		}
	}
	// TODO: if snap is nil (marshal failed after successful action),
	// the session is unplayable. Should terminate cleanly and notify
	// all subscribers rather than continue silently.

	// Return success to the caller and handle game pacing (AI turns,
	// trick/round completion, or game finished).
	c.resp <- SubmitResult{}
	s.handleStepResult(res)
}

// handleStepResult processes the outcome of a game mutation.
func (s *session) handleStepResult(res StepResult) {
	switch res.Outcome {
	case StepContinue:
		s.scheduleAI()
	case StepPause:
		s.broadcastSnapshot()
		s.autoResume()
	case StepFinished:
		s.broadcastSnapshot()
		s.closeSubscribers()
		if s.onDone != nil {
			s.onDone(Finished)
		}
		s.finished = true
	}
}

// autoResume pauses briefly then calls Resume.
func (s *session) autoResume() {
	delay := s.config.delay()
	if delay > 0 {
		select {
		case <-time.After(time.Duration(delay) * time.Millisecond):
		case <-s.cancel:
			return
		}
	}

	res, err := s.game.Resume()
	if err != nil {
		return
	}
	s.handleStepResult(res)
}

// scheduleAI checks if the next turn is AI and schedules it.
// TODO: Implement AI turn scheduling — if the next seat is AI, sleep for
// PacingDelayMS then call s.game.AIPlay(seat) and pass the returned
// StepResult to s.handleStepResult. This runs synchronously within the
// session goroutine; no additional goroutines are spawned.
// If the next seat is human, this function returns immediately and the
// session goroutine waits for the next human playCmd.
func (s *session) scheduleAI() {
	seat := s.game.Turn()
	_ = seat
}

// playerSnapshot generates a marshaled player snapshot for the given seat.
// It logs marshal errors and returns nil so the caller can skip the send.
func (s *session) playerSnapshot(seat int) []byte {
	snap := s.game.PlayerSnapshot(seat, s.seq)
	b, err := json.Marshal(snap)
	if err != nil {
		slog.Error("marshal player snapshot", "seat", seat, "error", err)
		return nil
	}
	return b
}

// observerSnapshot generates a marshaled observer snapshot.
// It logs marshal errors and returns nil so the caller can skip the send.
func (s *session) observerSnapshot() []byte {
	snap := s.game.ObserverSnapshot(s.seq)
	b, err := json.Marshal(snap)
	if err != nil {
		slog.Error("marshal observer snapshot", "error", err)
		return nil
	}
	return b
}

// broadcastSnapshot generates and sends snapshots to all subscribers.
// If a snapshot fails to marshal, the error is logged and the subscriber
// is skipped for this broadcast. Subscribers resync via a subsequent action.
// TODO: if all snapshots fail to marshal, the session is unplayable.
// Should terminate cleanly rather than continue silently.
func (s *session) broadcastSnapshot() {
	obsSnap := s.observerSnapshot()
	if obsSnap != nil {
		for _, ch := range s.observers {
			s.sendNonBlocking(ch, obsSnap)
		}
	}

	for seat, ch := range s.players {
		snap := s.playerSnapshot(seat)
		if snap != nil {
			s.sendNonBlocking(ch, snap)
		}
	}
}

// sendNonBlocking sends data to a channel without blocking.
// If the channel is full, the data is dropped.
func (s *session) sendNonBlocking(ch chan []byte, data []byte) {
	select {
	case ch <- data:
	default:
	}
}

// sendError sends an error message to a player subscriber.
func (s *session) sendError(seat int, code, message, actionID string) {
	ch, ok := s.players[seat]
	if !ok {
		return
	}

	em := api.ErrorMessage{
		Type:      errType,
		ErrorCode: code,
		Message:   message,
		ActionID:  actionID,
	}
	b, err := json.Marshal(em)
	if err != nil {
		return
	}
	s.sendNonBlocking(ch, b)
}

// handleSubscribePlayer registers a new player subscriber.
// If the seat already has a subscriber, the old channel is closed.
func (s *session) handleSubscribePlayer(c subscribePlayerCmd) {
	if old, ok := s.players[c.seat]; ok {
		close(old)
	}
	s.players[c.seat] = c.ch
	snap := s.playerSnapshot(c.seat)
	if snap != nil {
		s.sendNonBlocking(c.ch, snap)
	}
}

// handleSubscribeObserver registers a new observer subscriber.
func (s *session) handleSubscribeObserver(c subscribeObserverCmd) {
	s.observers = append(s.observers, c.ch)
	snap := s.observerSnapshot()
	if snap != nil {
		s.sendNonBlocking(c.ch, snap)
	}
}

// handleUnsubscribe removes a subscriber.
func (s *session) handleUnsubscribe(c unsubscribeCmd) {
	if c.seat == -1 {
		for i, ch := range s.observers {
			if ch == c.ch {
				close(ch)
				last := len(s.observers) - 1
				s.observers[i] = s.observers[last]
				s.observers = s.observers[:last]
				return
			}
		}
		return
	}

	if ch, ok := s.players[c.seat]; ok {
		close(ch)
		delete(s.players, c.seat)
	}
}

// closeSubscribers closes all subscriber channels exactly once and
// clears the subscriber maps so that any later unsubscribe does not
// attempt to close an already-closed channel.
func (s *session) closeSubscribers() {
	s.closeOnce.Do(func() {
		for _, ch := range s.players {
			close(ch)
		}
		for _, ch := range s.observers {
			close(ch)
		}
		s.players = make(map[int]chan []byte)
		s.observers = nil
	})
}

// drainCmds processes commands left in the buffer after the event loop
// exits. It closes subscriber channels and sends errors on playCmd.resp
// so that external callers do not block forever on a dead goroutine.
func (s *session) drainCmds() {
	for {
		select {
		case cmd := <-s.cmds:
			switch c := cmd.(type) {
			case playCmd:
				select {
				case c.resp <- SubmitResult{Err: &api.ErrorMessage{
					Type:      errType,
					ErrorCode: api.ErrGameOver,
					Message:   "session finished",
				}}:
				default:
				}
			case subscribePlayerCmd:
				close(c.ch)
			case subscribeObserverCmd:
				close(c.ch)
			}
		default:
			return
		}
	}
}

// evictArbitraryActionID removes an arbitrary entry from the action ID cache.
func (s *session) evictArbitraryActionID() {
	for k := range s.actionIDs {
		delete(s.actionIDs, k)
		return
	}
}

// delay returns the configured delay in milliseconds.
func (c Config) delay() int {
	if c.PacingDelayMS != nil {
		return *c.PacingDelayMS
	}
	return 500
}

// newSession creates a session and starts its goroutine.
func newSession(
	id string, g Game, cfg Config, onDone func(State),
) *session {
	s := &session{
		id:        id,
		game:      g,
		config:    cfg,
		actionIDs: make(map[string][]byte),
		players:   make(map[int]chan []byte),
		cmds:      make(chan command, cmdChanSize),
		cancel:    make(chan struct{}),
		done:      make(chan struct{}),
		onDone:    onDone,
	}
	go s.run()
	return s
}
