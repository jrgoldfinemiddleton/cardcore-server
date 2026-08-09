package session

import (
	"container/list"
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
	// Data is a marshaled snapshot to write to the subscriber's
	// WebSocket connection. It is empty when CloseCode is set.
	Data []byte
	// CloseCode is a WebSocket close status code. When nonzero, the
	// transport layer closes the connection with this code instead of
	// writing a snapshot (e.g., 1011 on snapshot marshal failure).
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
	// player. When a play command arrives, handleCommand -> handlePlay
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

// aiActionDelay returns the configured AI action delay in milliseconds.
// If the per-session config value is nil, the server-wide default is used.
func (s *session) aiActionDelay() int {
	if s.config.AIActionDelayMS != nil {
		return *s.config.AIActionDelayMS
	}
	return s.defaults.AIActionDelayMS
}

// turnTimeout returns the turn timeout as a time.Duration. If the
// per-session config value is nil, the server-wide default is used. 0
// or negative means disabled.
func (s *session) turnTimeout() time.Duration {
	if s.config.TurnTimeoutMS != nil {
		return time.Duration(*s.config.TurnTimeoutMS) * time.Millisecond
	}
	return time.Duration(s.defaults.TurnTimeoutMS) * time.Millisecond
}

// newSession creates a session and starts its goroutine.
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
