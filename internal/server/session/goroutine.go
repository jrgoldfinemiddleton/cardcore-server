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
	// defaults for aiActionDelay and turnTimeout resolution.
	defaults DefaultDelays

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
	// paused is true when the game is paused. When true, the turn
	// timeout does not fire and the select loop only accepts commands
	// and cancel.
	paused bool
	// pauseRemaining stores the remaining time on the turn deadline
	// when the game was paused. Used to recalculate the deadline on
	// resume.
	pauseRemaining time.Duration

	// logger is the per-component logger so all session goroutine log
	// lines carry session_id for filtering and correlation.
	logger *slog.Logger
}

// run is the session goroutine event loop.
func (s *session) run() {
	// done signals that the goroutine has exited and all subscriber
	// channels have been closed. Manager methods use <-done to detect
	// shutdown and avoid blocking on a dead session.
	defer close(s.done)

	s.logger.Debug("session goroutine started")

	// Stamp the turn deadline onto the initial snapshot so clients see
	// the timer for the first human turn before any commands are
	// submitted, then wait for the game's display delay.
	s.scheduleTurnDeadline()
	s.broadcastSnapshot()
	if s.finished {
		s.drainCmds()
		return
	}
	// Allow the game adapter to specify a display delay before the first
	// turn is processed.
	delay := s.game.DisplayDelay()
	if delay > 0 {
		s.logger.Debug("initial display delay", "delay_ms", delay)
		select {
		case <-time.After(time.Duration(delay) * time.Millisecond):
		case <-s.cancel:
			s.closeSubscribers()
			s.drainCmds()
			return
		}
	}

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
		if s.waitingForHuman && s.turnTimeout() > 0 && !s.paused {
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
			s.logger.Debug("command received", "type", cmdType(cmd))
			if pc, ok := cmd.(playCmd); ok && s.waitingForHuman &&
				pc.seat == s.game.Turn() && s.isHumanSeat(pc.seat) &&
				pc.msg.Type != "pause" && pc.msg.Type != "resume" {
				s.waitingForHuman = false
			}
			s.handleCommand(cmd)
			if s.finished {
				s.drainCmds()
				return
			}
		case <-s.cancel:
			s.logger.Debug("shutdown requested")
			s.closeSubscribers()
			s.drainCmds()
			return
		case <-timeoutCh:
			s.logger.Debug("turn timeout fired")
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

	s.logger.Debug("handlePlay",
		"seat", c.seat,
		"type", c.msg.Type,
		"action_id", c.msg.ActionID,
		"seq", c.msg.Seq,
		"server_seq", s.seq,
	)

	// Pause/resume are session-level meta-commands. Intercept them before
	// seq validation so the same playCmd plumbing can carry them.
	if c.msg.Type == "pause" {
		s.handlePauseCmd(c)
		return
	}
	if c.msg.Type == "resume" {
		s.handleResumeCmd(c)
		return
	}

	// Client seq is behind the server. Send the latest snapshot so
	// the client can resync, along with an error.
	if c.msg.Seq < s.seq {
		s.logger.Warn("stale seq",
			"seat", c.seat,
			"client_seq", c.msg.Seq,
			"server_seq", s.seq,
		)
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
		s.logger.Warn("duplicate action_id", "action_id", c.msg.ActionID)
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
		s.logger.Warn("action rejected",
			"seat", c.seat,
			"type", c.msg.Type,
			"error_code", cmdErr.Code,
			"message", cmdErr.Message,
		)
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

	// Action accepted. Clear the previous turn deadline, schedule the
	// deadline for the next turn, increment seq, broadcast the new state
	// to all subscribers, and cache the snapshot for idempotent replay.
	// Skip caching if the snapshot fails to marshal so the cache never
	// contains nil entries that would break duplicate action_id replay.
	// If the game just finished, leave the deadline cleared.
	s.game.SetTurnDeadline(time.Time{})
	if res.Outcome != StepFinished {
		s.scheduleTurnDeadline()
	}
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
	s.logger.Debug("driveTurns", "from_pause", fromPause)

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
		s.logger.Error("driveFatal reached, closing subscribers immediately")
		s.closeSubscribers()
		if s.onDone != nil {
			s.onDone(Finished)
		}
		s.finished = true
	}

	s.logger.Debug("driveTurns done", "status", status)

	return status
}

// resumePauses waits for the game's display delay then calls Resume. It
// returns a driveResult so driveTurns can loop again if the game is
// still in a paused state.
func (s *session) resumePauses() driveResult {
	delay := s.game.DisplayDelay()
	if delay > 0 {
		s.logger.Debug("resumePauses waiting", "delay_ms", delay)
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
		s.logger.Error("Resume failed", "error", err)
		return driveFatal
	}
	s.game.SetTurnDeadline(time.Time{})
	if res.Outcome != StepFinished {
		s.scheduleTurnDeadline()
	}
	s.seq++
	s.logger.Debug("Resume succeeded", "outcome", res.Outcome)
	s.broadcastSnapshot()
	if s.finished {
		return driveFinished
	}

	switch res.Outcome {
	case StepContinue:
		delay = s.game.DisplayDelay()
		if delay > 0 {
			s.logger.Debug("post-resume display delay", "delay_ms", delay)
			select {
			case <-time.After(time.Duration(delay) * time.Millisecond):
			case <-s.cancel:
				return driveShutdown
			}
		}
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
			s.logger.Error("driveFatal: invalid seat", "seat", seat)
			return driveFatal
		}
		isHuman := s.isHumanSeat(seat)
		s.logger.Debug("processTurns", "seat", seat, "human", isHuman)
		// If human, return so run() can schedule the timeout. The deadline
		// was already stamped onto the snapshot when the game arrived at
		// this turn.
		if isHuman {
			s.waitingForHuman = true
			return driveHuman
		}

		s.waitingForHuman = false

		// Pace AI turns for UX readability; also process any buffered
		// commands so late subscribers are not stranded.
		delay := s.aiActionDelay()
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
			s.logger.Error("AIPlay failed", "seat", seat, "error", err)
			return driveFatal
		}
		s.game.SetTurnDeadline(time.Time{})
		if res.Outcome != StepFinished {
			s.scheduleTurnDeadline()
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

// scheduleTurnDeadline sets the turn deadline for the current turn and
// updates waitingForHuman. It clears the deadline if turn timeouts are
// disabled, the current seat is invalid, or the current seat is AI.
func (s *session) scheduleTurnDeadline() {
	if s.turnTimeout() <= 0 {
		s.waitingForHuman = false
		s.game.SetTurnDeadline(time.Time{})
		return
	}
	seat := s.game.Turn()
	if seat < 0 || seat >= len(s.config.Seats) {
		s.waitingForHuman = false
		s.game.SetTurnDeadline(time.Time{})
		s.logger.Error("invalid turn seat when scheduling deadline", "seat", seat)
		return
	}
	if s.isHumanSeat(seat) {
		s.waitingForHuman = true
		s.turnDeadline = time.Now().Add(s.turnTimeout())
		s.game.SetTurnDeadline(s.turnDeadline)
		s.logger.Debug("turn timeout scheduled", "seat", seat, "deadline", s.turnDeadline)
		return
	}
	s.waitingForHuman = false
	s.game.SetTurnDeadline(time.Time{})
}

// isHumanSeat reports whether the given seat is configured as human.
func (s *session) isHumanSeat(seat int) bool {
	if seat < 0 || seat >= len(s.config.Seats) {
		s.logger.Error("game adapter returned invalid seat", "seat", seat)
		return false
	}
	return s.config.Seats[seat].Type == SeatHuman
}

// humanCount returns the number of human seats in the session.
func (s *session) humanCount() int {
	count := 0
	for _, sc := range s.config.Seats {
		if sc.Type == SeatHuman {
			count++
		}
	}
	return count
}

// handleTurnTimeout handles a turn timeout by playing an AI move for
// the current human seat.
func (s *session) handleTurnTimeout() {
	seat := s.game.Turn()
	// Guard that the seat is still human (e.g., client disconnected human
	// player since timeout was set).
	if !s.isHumanSeat(seat) {
		s.logger.Debug("turn timeout skipped: seat no longer human", "seat", seat)
		return
	}

	s.logger.Info("turn timeout, playing AI move", "seat", seat)
	s.game.SetTurnDeadline(time.Time{})
	res, err := s.game.AIPlay(seat)
	if err != nil {
		s.logger.Error("AIPlay on timeout failed", "seat", seat, "error", err)
		return
	}
	s.game.SetTurnDeadline(time.Time{})
	if res.Outcome != StepFinished {
		s.scheduleTurnDeadline()
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

// handlePauseCmd pauses the game. Only valid for single-human sessions
// when waiting for a human turn and not already paused.
func (s *session) handlePauseCmd(c playCmd) {
	if s.humanCount() > 1 {
		c.resp <- SubmitResult{Err: &api.ErrorMessage{
			Type: errType, ErrorCode: api.ErrPauseNotAllowed,
			Message:  "pause is only available in single-human games",
			ActionID: c.msg.ActionID, CurrentSeq: s.seq,
		}}
		return
	}
	if !s.waitingForHuman {
		c.resp <- SubmitResult{Err: &api.ErrorMessage{
			Type: errType, ErrorCode: api.ErrPauseNotAllowed,
			Message:  "can only pause during your turn",
			ActionID: c.msg.ActionID, CurrentSeq: s.seq,
		}}
		return
	}
	if s.paused {
		c.resp <- SubmitResult{Err: &api.ErrorMessage{
			Type: errType, ErrorCode: api.ErrPauseNotAllowed,
			Message:  "game is already paused",
			ActionID: c.msg.ActionID, CurrentSeq: s.seq,
		}}
		return
	}
	if s.turnTimeout() > 0 {
		s.pauseRemaining = time.Until(s.turnDeadline)
	}
	s.paused = true
	s.game.SetPaused(true)
	s.game.SetTurnDeadline(time.Time{})
	s.seq++
	s.broadcastSnapshot()
	c.resp <- SubmitResult{}
}

// handleResumeCmd resumes the game. Only valid for single-human sessions
// when paused.
func (s *session) handleResumeCmd(c playCmd) {
	if !s.paused {
		c.resp <- SubmitResult{Err: &api.ErrorMessage{
			Type: errType, ErrorCode: api.ErrPauseNotAllowed,
			Message:  "game is not paused",
			ActionID: c.msg.ActionID, CurrentSeq: s.seq,
		}}
		return
	}
	if s.humanCount() > 1 {
		c.resp <- SubmitResult{Err: &api.ErrorMessage{
			Type: errType, ErrorCode: api.ErrPauseNotAllowed,
			Message:  "resume is only available in single-human games",
			ActionID: c.msg.ActionID, CurrentSeq: s.seq,
		}}
		return
	}
	s.paused = false
	s.game.SetPaused(false)
	if s.waitingForHuman && s.turnTimeout() > 0 {
		s.turnDeadline = time.Now().Add(s.pauseRemaining)
		s.game.SetTurnDeadline(s.turnDeadline)
	}
	s.seq++
	s.broadcastSnapshot()
	c.resp <- SubmitResult{}
}

// autoUnpause resumes a paused game when the human player disconnects.
func (s *session) autoUnpause() {
	s.logger.Info("auto-unpausing on human disconnect")
	s.paused = false
	s.game.SetPaused(false)
	if s.waitingForHuman && s.turnTimeout() > 0 {
		s.turnDeadline = time.Now().Add(s.pauseRemaining)
		s.game.SetTurnDeadline(s.turnDeadline)
	}
	s.seq++
	s.broadcastSnapshot()
}

// playerSnapshot generates a marshaled player snapshot for the given seat.
// It logs marshal errors and returns nil so the caller can handle the
// fatal session state. A marshal failure represents an unrecoverable
// session error that terminates the game.
func (s *session) playerSnapshot(seat int) []byte {
	snap := s.game.PlayerSnapshot(seat, s.seq)
	b, err := json.Marshal(snap)
	if err != nil {
		s.logger.Error("marshal player snapshot", "seat", seat, "error", err)
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
		s.logger.Error("marshal observer snapshot", "error", err)
		return nil
	}
	return b
}

// broadcastSnapshot generates and sends snapshots to all subscribers.
// If a snapshot fails to marshal, the session is terminated because the
// game state is unplayable.
func (s *session) broadcastSnapshot() {
	s.logger.Debug("broadcastSnapshot", "seq", s.seq)

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
		s.logger.Warn("subscriber channel full, snapshot dropped",
			"queue_depth", subChanSize,
		)
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
	s.logger.Info("player subscribed", "seat", c.seat, "replaced", replaced)
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
	s.logger.Info("observer subscribed", "observer_count", len(s.observers)+1)
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
				s.logger.Info("observer unsubscribed",
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
		s.logger.Info("player unsubscribed", "seat", c.seat)
	}

	// Auto-unpause: if the game is paused and a human seat disconnects,
	// unpause so the turn timeout can fire and AIPlay handles the absent
	// human, exactly as it does for a disconnected human in a non-paused
	// game.
	if s.paused && s.isHumanSeat(c.seat) {
		s.autoUnpause()
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
	s.logger.Info("game finished")

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
	s.logger.Error("session terminating due to marshal failure",
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

// aiActionDelay returns the configured AI action delay in milliseconds.
// If the per-session config value is nil, the server-wide default is used.
// so the goroutine uses server-wide defaults instead of hardcoded 1000.
func (s *session) aiActionDelay() int {
	if s.config.AIActionDelayMS != nil {
		return *s.config.AIActionDelayMS
	}
	return s.defaults.AIActionDelayMS
}

// turnTimeout returns the turn timeout as a time.Duration. If the
// per-session config value is nil, the server-wide default is used. 0
// or negative means disabled.
// so the goroutine uses server-wide defaults instead of hardcoded 30000.
func (s *session) turnTimeout() time.Duration {
	if s.config.TurnTimeoutMS != nil {
		return time.Duration(*s.config.TurnTimeoutMS) * time.Millisecond
	}
	return time.Duration(s.defaults.TurnTimeoutMS) * time.Millisecond
}

// newSession creates a session and starts its goroutine.
// per-session overrides with server-wide fallback.
func newSession(
	id string, g Game, cfg Config, defaults DefaultDelays, onDone func(State),
) *session {
	s := &session{
		id:            id,
		seq:           1,
		game:          g,
		config:        cfg,
		defaults:      defaults,
		actionIDs:     make(map[string][]byte),
		actionIDList:  list.New(),
		actionIDIndex: make(map[string]*list.Element),
		players:       make(map[int]chan SubscriberMessage),
		cmds:          make(chan command, cmdChanSize),
		cancel:        make(chan struct{}),
		done:          make(chan struct{}),
		onDone:        onDone,
		logger:        slog.With("component", "session", "session_id", id),
	}
	go s.run()
	return s
}

// cmdType returns a human-readable name for a command value.
func cmdType(cmd command) string {
	switch cmd.(type) {
	case playCmd:
		return "play"
	case subscribePlayerCmd:
		return "subscribe_player"
	case subscribeObserverCmd:
		return "subscribe_observer"
	case unsubscribeCmd:
		return "unsubscribe"
	default:
		return "unknown"
	}
}
