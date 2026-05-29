package session

import (
	"container/list"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
)

const (
	cmdChanSize       = 64
	subChanSize       = 64
	actionIDCacheSize = 1000

	errType = "error"
)

// driveResult tells driveTurns what happened inside processTurns or
// resumePauses so it can decide whether to loop, return to run(), or
// terminate the session.
//
// driveFatal triggers session cleanup inside
// driveTurns. driveShutdown means the cancel channel fired during a
// pacing delay and the handler should return to run()'s select loop
// immediately without further processing so the goroutine can exit
// without mutating state.
type driveResult int

const (
	driveHuman driveResult = iota
	drivePaused
	driveFinished
	driveFatal
	driveShutdown
)

// SubscriberMessage carries either a data payload or a close code
// for the transport layer to use when closing the WebSocket.
type SubscriberMessage struct {
	Data      []byte
	CloseCode int
}

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
	// actionIDList holds action IDs in LRU order (front = most recent)
	// for bounded eviction of the idempotent replay cache.
	actionIDList *list.List
	// actionIDIndex maps action_id to its position in actionIDList so
	// that duplicate hits can be promoted in O(1).
	actionIDIndex map[string]*list.Element

	// players maps seat index to subscriber channel.
	players map[int]chan SubscriberMessage
	// observers holds all observer subscriber channels.
	observers []chan SubscriberMessage

	// cmds receives commands from the Manager.
	cmds chan command
	// cancel signals the goroutine to shut down.
	cancel chan struct{}
	// done is closed when the goroutine exits.
	done chan struct{}

	// onDone is called by the goroutine when the session reaches a
	// terminal state, either because the game finished or an
	// unrecoverable error forced termination.
	onDone func(State)

	// finished is set when the game reaches a terminal state,
	// signaling run() to exit after the current command completes.
	finished bool

	// waitingForHuman is true when the goroutine is waiting for a
	// human player to act and a turn timeout is active.
	waitingForHuman bool
	// turnDeadline is the time at which the turn timeout fires.
	turnDeadline time.Time
}

// run is the session goroutine event loop.
func (s *session) run() {
	// done signals that the goroutine has exited and all subscriber
	// channels have been closed. Manager methods use <-done to detect
	// shutdown and avoid blocking on a dead session.
	defer close(s.done)

	driveTurnsResult := s.driveTurns(false)
	if driveTurnsResult == driveFinished || s.finished {
		s.drainCmds()
		return
	}

	// This select loop runs only when the game is waiting on a human
	// player. When a play command arrives, handleCommand → handlePlay
	// processes it and then calls driveTurns to handle any subsequent
	// AI turns or Resume chains before returning. When a timeout fires,
	// handleTurnTimeout does the same. In both cases, by the time the
	// handler returns, the game is back in a human-waiting state or
	// finished, so the loop can block again safely.
	for {
		var timeoutCh <-chan time.Time
		// If waiting for a human turn and a turn timeout is configured,
		// set the timeout channel to fire when the deadline is reached.
		// If the deadline has already passed, handle the timeout
		// immediately without waiting.
		if s.waitingForHuman && s.config.turnTimeout() > 0 {
			remaining := time.Until(s.turnDeadline)
			if remaining > 0 {
				// Deadline is in the future; set the timeout channel to fire when it expires.
				timeoutCh = time.After(remaining)
			} else {
				// Deadline has already passed; let the select fire immediately.
				timeoutCh = time.After(0)
			}
		}

		select {
		case cmd := <-s.cmds:
			if pc, ok := cmd.(playCmd); ok && s.waitingForHuman &&
				pc.seat == s.game.Turn() && s.isHumanSeat(pc.seat) {
				s.waitingForHuman = false
			}
			s.handleCommand(cmd)
			if s.finished {
				s.drainCmds()
				return
			}
		case <-s.cancel:
			s.closeSubscribers()
			s.drainCmds()
			return
		case <-timeoutCh:
			s.waitingForHuman = false
			s.handleTurnTimeout()
			if s.finished {
				s.drainCmds()
				return
			}
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

	// Client seq is behind the server. Send the latest snapshot so
	// the client can resync, along with an error.
	if c.msg.Seq < s.seq {
		snap := s.playerSnapshot(c.seat)
		if snap == nil {
			s.terminateOnMarshalFailure(
				"stale seq player snapshot marshal failed",
				"seat", c.seat,
			)
			c.resp <- SubmitResult{
				Err: &api.ErrorMessage{
					Type:       errType,
					ErrorCode:  api.ErrInternal,
					Message:    "session terminated: snapshot generation failed",
					ActionID:   c.msg.ActionID,
					CurrentSeq: s.seq,
				},
			}
			return
		}
		c.resp <- SubmitResult{
			Err: &api.ErrorMessage{
				Type:       errType,
				ErrorCode:  api.ErrStaleSeq,
				Message:    "client seq is behind server",
				ActionID:   c.msg.ActionID,
				CurrentSeq: s.seq,
			},
			Snapshot: snap,
		}
		return
	}

	// Duplicate action_id: client resent a command that already
	// succeeded. Send the cached snapshot without mutating state.
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
		// send it on the command response channel so the blocked
		// submitter receives the rejection.
		s.sendError(c.seat, cmdErr.Code, cmdErr.Message, c.msg.ActionID)
		c.resp <- SubmitResult{
			Err: &api.ErrorMessage{
				Type:       errType,
				ErrorCode:  cmdErr.Code,
				Message:    cmdErr.Message,
				ActionID:   c.msg.ActionID,
				CurrentSeq: s.seq,
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
	if s.finished {
		c.resp <- SubmitResult{
			Err: &api.ErrorMessage{
				Type:       errType,
				ErrorCode:  api.ErrInternal,
				Message:    "session terminated: snapshot generation failed",
				ActionID:   c.msg.ActionID,
				CurrentSeq: s.seq,
			},
		}
		return
	}
	// broadcastSnapshot already generated a player snapshot for this
	// seat and would have terminated the session if it failed to
	// marshal, so snap == nil is unreachable here.
	snap := s.playerSnapshot(c.seat)
	s.cacheActionID(c.msg.ActionID, snap)

	// Send success response to the blocked command submitter and
	// handle game pacing (AI turns, trick/round completion, or game
	// finished).
	c.resp <- SubmitResult{}
	switch res.Outcome {
	case StepContinue:
		s.driveTurns(false)
	case StepPause:
		s.driveTurns(true)
	case StepFinished:
		s.finishWithGrace()
	}
}

// driveTurns is the central orchestrator for the game event loop.
// If fromPause is false it first calls processTurns to check the
// current seat; if true it enters the resume cycle immediately because
// the game is in a paused state. The status enums prevent unbounded
// mutual recursion and keep all state transitions synchronous on the
// session goroutine.
func (s *session) driveTurns(fromPause bool) driveResult {
	var status driveResult
	if fromPause {
		status = s.resumePauses()
	} else {
		status = s.processTurns()
	}
	for status == drivePaused {
		status = s.resumePauses()
	}
	if status == driveFatal {
		slog.Error("driveFatal reached, closing subscribers immediately", "session", s.id)
		s.closeSubscribers()
		if s.onDone != nil {
			s.onDone(Finished)
		}
		s.finished = true
	}
	return status
}

// resumePauses waits for the pacing delay then calls Resume. It returns
// a driveResult so driveTurns can loop again if the game is still in a
// paused state.
func (s *session) resumePauses() driveResult {
	delay := s.config.delay()
	if delay > 0 {
		select {
		case <-time.After(time.Duration(delay) * time.Millisecond):
		case <-s.cancel:
			return driveShutdown
		}
	}
	if s.finished {
		return driveFinished
	}

	res, err := s.game.Resume()
	if err != nil {
		slog.Error("Resume failed", "error", err)
		return driveFatal
	}
	s.seq++
	s.broadcastSnapshot()
	if s.finished {
		return driveFinished
	}

	switch res.Outcome {
	case StepContinue:
		return s.processTurns()
	case StepPause:
		return drivePaused
	case StepFinished:
		return s.finishWithGrace()
	}
	return driveFatal
}

// processTurns advances the game state by checking the current seat
// and playing AI turns if necessary. It sets the turn timeout when a
// human seat is reached and returns a driveResult to indicate the
// session should wait for a human action, a pause occurred, an error
// happened, or the game finished.
func (s *session) processTurns() driveResult {
	for {
		seat := s.game.Turn()
		// Guard: if seat is out of range (e.g., empty config), stop.
		if seat < 0 || seat >= len(s.config.Seats) {
			slog.Error("driveFatal: invalid seat", "seat", seat, "session", s.id)
			return driveFatal
		}
		// If human, set turn timeout and return so run() can handle it.
		if s.isHumanSeat(seat) {
			if s.config.turnTimeout() > 0 {
				s.waitingForHuman = true
				s.turnDeadline = time.Now().Add(s.config.turnTimeout())
			}
			return driveHuman
		}

		// Pace AI turns for UX readability; also process any buffered
		// commands so late subscribers are not stranded.
		delay := s.config.delay()
		if delay > 0 {
			select {
			case <-time.After(time.Duration(delay) * time.Millisecond):
			case <-s.cancel:
				return driveShutdown
			case cmd := <-s.cmds:
				s.handleCommand(cmd)
			}
		} else {
			select {
			case <-s.cancel:
				return driveShutdown
			case cmd := <-s.cmds:
				s.handleCommand(cmd)
			default:
			}
		}
		if s.finished {
			return driveFinished
		}

		res, err := s.game.AIPlay(seat)
		if err != nil {
			slog.Error("AIPlay failed", "seat", seat, "error", err)
			return driveFatal
		}
		s.seq++
		s.broadcastSnapshot()
		if s.finished {
			return driveFinished
		}

		switch res.Outcome {
		case StepContinue:
		case StepPause:
			return drivePaused
		case StepFinished:
			return s.finishWithGrace()
		}
	}
}

// isHumanSeat reports whether the given seat is configured as human.
func (s *session) isHumanSeat(seat int) bool {
	if seat < 0 || seat >= len(s.config.Seats) {
		slog.Error("game adapter returned invalid seat", "seat", seat, "session", s.id)
		return false
	}
	return s.config.Seats[seat].Type == SeatHuman
}

// handleTurnTimeout handles a turn timeout by playing an AI move for
// the current human seat.
func (s *session) handleTurnTimeout() {
	seat := s.game.Turn()
	// Guard that the seat is still human (e.g., client disconnected human
	// player since timeout was set).
	if !s.isHumanSeat(seat) {
		return
	}

	slog.Info("turn timeout, playing AI move", "seat", seat, "session", s.id)
	res, err := s.game.AIPlay(seat)
	if err != nil {
		slog.Error("AIPlay on timeout failed", "seat", seat, "error", err)
		return
	}
	s.seq++
	s.broadcastSnapshot()
	if s.finished {
		return
	}
	switch res.Outcome {
	case StepContinue:
		s.driveTurns(false)
	case StepPause:
		s.driveTurns(true)
	case StepFinished:
		s.finishWithGrace()
	}
}

// playerSnapshot generates a marshaled player snapshot for the given seat.
// It logs marshal errors and returns nil so the caller can handle the
// fatal session state. A marshal failure represents an unrecoverable
// session error that terminates the game.
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
// It logs marshal errors and returns nil so the caller can handle the
// fatal session state. A marshal failure represents an unrecoverable
// session error that terminates the game.
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
// If a snapshot fails to marshal, the session is terminated because the
// game state is unplayable.
func (s *session) broadcastSnapshot() {
	obsSnap := s.observerSnapshot()
	if obsSnap == nil {
		s.terminateOnMarshalFailure("observer snapshot marshal failed")
		return
	}
	for _, ch := range s.observers {
		s.sendNonBlocking(ch, obsSnap)
	}

	for seat, ch := range s.players {
		snap := s.playerSnapshot(seat)
		if snap == nil {
			s.terminateOnMarshalFailure(
				"player snapshot marshal failed",
				"seat", seat,
			)
			return
		}
		s.sendNonBlocking(ch, snap)
	}
}

// sendNonBlocking sends data to a channel without blocking.
// If the channel is full, the data is dropped.
func (s *session) sendNonBlocking(ch chan SubscriberMessage, data []byte) {
	select {
	case ch <- SubscriberMessage{Data: data}:
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
		Type:       errType,
		ErrorCode:  code,
		Message:    message,
		ActionID:   actionID,
		CurrentSeq: s.seq,
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
	replaced := false
	if old, ok := s.players[c.seat]; ok {
		close(old)
		replaced = true
	}
	slog.Info("player subscribed", "session", s.id, "seat", c.seat, "replaced", replaced)
	s.players[c.seat] = c.ch
	snap := s.playerSnapshot(c.seat)
	if snap == nil {
		s.terminateOnMarshalFailure(
			"player snapshot marshal failed",
			"seat", c.seat,
		)
		return
	}
	s.sendNonBlocking(c.ch, snap)
}

// handleSubscribeObserver registers a new observer subscriber.
func (s *session) handleSubscribeObserver(c subscribeObserverCmd) {
	slog.Info("observer subscribed", "session", s.id, "observer_count", len(s.observers)+1)
	s.observers = append(s.observers, c.ch)
	snap := s.observerSnapshot()
	if snap == nil {
		s.terminateOnMarshalFailure("observer snapshot marshal failed")
		return
	}
	s.sendNonBlocking(c.ch, snap)
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
				slog.Info("observer unsubscribed",
					"session", s.id,
					"observer_count", len(s.observers),
				)
				return
			}
		}
		return
	}

	if ch, ok := s.players[c.seat]; ok {
		close(ch)
		delete(s.players, c.seat)
		slog.Info("player unsubscribed", "session", s.id, "seat", c.seat)
	}
}

// closeSubscribers closes all subscriber channels and clears the
// subscriber maps so that any later unsubscribe does not attempt to
// close an already-closed channel. The caller must ensure this is only
// called once per session — all exit paths in run() return
// immediately after calling it.
func (s *session) closeSubscribers() {
	for _, ch := range s.players {
		close(ch)
	}
	for _, ch := range s.observers {
		close(ch)
	}
	s.players = make(map[int]chan SubscriberMessage)
	s.observers = nil
}

// finishWithGrace closes subscribers after a brief grace period so
// observers can read the final snapshot before connections close.
func (s *session) finishWithGrace() driveResult {
	select {
	case <-time.After(100 * time.Millisecond):
	case <-s.cancel:
	}
	s.closeSubscribers()
	if s.onDone != nil {
		s.onDone(Finished)
	}
	s.finished = true
	return driveFinished
}

// drainCmds processes commands left in the buffer after the event loop
// exits. It closes subscriber channels and sends errors on playCmd.resp
// so that blocked command submitters do not wait forever.
func (s *session) drainCmds() {
	for {
		select {
		case cmd := <-s.cmds:
			switch c := cmd.(type) {
			case playCmd:
				select {
				case c.resp <- SubmitResult{Err: &api.ErrorMessage{
					Type:       errType,
					ErrorCode:  api.ErrGameOver,
					Message:    "session finished",
					CurrentSeq: s.seq,
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

// terminateOnMarshalFailure logs the error, sends a close code to
// all subscribers so the transport layer closes the WebSocket with
// 1011 Internal Error, closes all subscriber channels, notifies the
// Manager that the session is finished, and marks the session as
// finished so the goroutine exits.
func (s *session) terminateOnMarshalFailure(msg string, args ...any) {
	slog.Error("session terminating due to marshal failure",
		append([]any{"message", msg}, args...)...)
	for _, ch := range s.players {
		select {
		case ch <- SubscriberMessage{CloseCode: 1011}:
		default:
		}
	}
	for _, ch := range s.observers {
		select {
		case ch <- SubscriberMessage{CloseCode: 1011}:
		default:
		}
	}
	s.closeSubscribers()
	if s.onDone != nil {
		s.onDone(Finished)
	}
	s.finished = true
}

// cacheActionID stores a snapshot for the given action ID, promoting it
// to the front of the LRU list. If the cache exceeds the size limit, the
// least-recently-used entry is evicted.
func (s *session) cacheActionID(id string, snap []byte) {
	if el, ok := s.actionIDIndex[id]; ok {
		// Already cached — promote to front.
		s.actionIDList.MoveToFront(el)
		return
	}
	// New entry.
	s.actionIDs[id] = snap
	el := s.actionIDList.PushFront(id)
	s.actionIDIndex[id] = el
	if s.actionIDList.Len() > actionIDCacheSize {
		s.evictLRUActionID()
	}
}

// evictLRUActionID removes the least-recently-used entry from the cache.
func (s *session) evictLRUActionID() {
	back := s.actionIDList.Back()
	if back == nil {
		return
	}
	id := back.Value.(string)
	s.actionIDList.Remove(back)
	delete(s.actionIDIndex, id)
	delete(s.actionIDs, id)
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
		id:            id,
		game:          g,
		config:        cfg,
		actionIDs:     make(map[string][]byte),
		actionIDList:  list.New(),
		actionIDIndex: make(map[string]*list.Element),
		players:       make(map[int]chan SubscriberMessage),
		cmds:          make(chan command, cmdChanSize),
		cancel:        make(chan struct{}),
		done:          make(chan struct{}),
		onDone:        onDone,
	}
	go s.run()
	return s
}
