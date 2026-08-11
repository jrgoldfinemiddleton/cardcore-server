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
	// dealPlayerSnapshots holds seq-stable deal snapshots for players who
	// subscribe after DisplayDelay consumes the adapter flag but before the
	// transition broadcast.
	dealPlayerSnapshots map[int][]byte
	// dealObserverSnapshot is the matching observer snapshot during that window.
	dealObserverSnapshot []byte

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
	// and cancel. handlePlay rejects gameplay commands with
	// game_paused while this is set; only resume is processed.
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

	// A game holding a fresh deal opens with the deal display phase:
	// deal snapshot, display window, then the actionable transition.
	// A game with no deal to display keeps the single initial broadcast.
	if s.game.DealPending() {
		if !s.runDealPhase() {
			s.drainCmds()
			return
		}
	} else {
		// Stamp the turn deadline onto the initial snapshot so clients
		// see the timer for the first human turn before any commands
		// are submitted, then honor the game's initial-state pacing
		// hook (zero for a game whose opening needs no pacing).
		s.scheduleTurnDeadline()
		s.broadcastSnapshot()
		if s.finished {
			s.drainCmds()
			return
		}
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
	}

	// Drive AI turns and pause chains until the game parks on a human
	// turn or finishes. The select loop below is purely reactive — it
	// blocks on commands, cancellation, or a turn timeout — so entering
	// it without driving first would deadlock any game whose first turn
	// is not a human's: no command would ever arrive and no timeout
	// would be armed.
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
			// Commands arriving while paused are rejected by the paused
			// guard in handlePlay; do not let them clear waitingForHuman,
			// or resume would skip restoring the turn deadline from
			// pauseRemaining.
			if pc, ok := cmd.(playCmd); ok && s.waitingForHuman &&
				pc.seat == s.game.Turn() && s.isHumanSeat(pc.seat) &&
				pc.msg.Type != "pause" && pc.msg.Type != "resume" && !s.paused {
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

// runDealPhase presents a freshly dealt round before play becomes
// actionable: it broadcasts the deal snapshot, holds the configured
// display window, then broadcasts the transition to the actionable phase
// with the first turn's deadline stamped. It returns false when the
// session finished or was cancelled along the way.
//
// The deal snapshot carries no turn deadline: nobody can act during the
// deal, so there is nothing to count down to. The window answers
// subscription commands immediately — clients connect right after the
// session starts and would otherwise wait out the window on a blank
// screen and never see the deal — while gameplay commands are deferred
// until the transition broadcast, because the deal phase is not
// actionable (doc/games/hearts/protocol.md). Deferral is deliberate:
// the dealt hand is already final, so an early command is valid content
// sent during a cosmetic pause, and rejecting it with wrong_phase would
// punish the fastest clients for a server-side delay. The trick/round
// display windows apply the same policy by leaving commands queued in
// the channel during their sleeps.
func (s *session) runDealPhase() bool {
	s.broadcastSnapshot()
	if s.finished {
		return false
	}
	// DisplayDelay consumes the adapter's pending-deal flag, so the deal
	// snapshots are cached first: subscribers joining during the window
	// must still receive the deal view at the current seq.
	if !s.cacheDealSnapshots() {
		return false
	}

	delay := s.game.DisplayDelay()
	var deferred []command
	if delay > 0 {
		var ok bool
		deferred, ok = s.awaitDealWindow(delay)
		if !ok {
			return false
		}
	}

	// The window has elapsed. End it before the transition so later
	// subscribers get the live actionable snapshot, not the cached deal.
	s.clearDealSnapshots()
	s.scheduleTurnDeadline()
	s.seq++
	s.broadcastSnapshot()
	if s.finished {
		return false
	}

	// Replay deferred commands. Every one of them predates the transition
	// broadcast, so seq-validated commands (play_card, pass_cards) resync
	// through the normal stale_seq path, while pause/resume bypass seq
	// validation (see handlePlay) and apply to the post-transition state.
	for _, cmd := range deferred {
		s.handleCommand(cmd)
		if s.finished {
			return false
		}
	}
	return true
}

// awaitDealWindow holds the deal display window for delay milliseconds.
// Subscription commands are answered immediately from the cached deal
// snapshots; all other commands are deferred for replay after the
// transition broadcast. It returns ok=false when the session finished
// or was cancelled during the window.
func (s *session) awaitDealWindow(delay int) (deferred []command, ok bool) {
	s.logger.Debug("deal display delay", "delay_ms", delay)
	timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
	stopTimer := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	for {
		select {
		case <-timer.C:
			return deferred, true
		case cmd := <-s.cmds:
			switch cmd.(type) {
			case subscribePlayerCmd, subscribeObserverCmd, unsubscribeCmd:
				s.handleCommand(cmd)
				if s.finished {
					stopTimer()
					return nil, false
				}
			default:
				deferred = append(deferred, cmd)
			}
		case <-s.cancel:
			stopTimer()
			s.closeSubscribers()
			return nil, false
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

// resumePauses waits for a pause display delay and calls Resume. A resumed
// fresh deal is broadcast before its own delay, followed by the actionable
// transition snapshot. It returns a driveResult so driveTurns can continue.
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
	// Remember whether Resume just dealt a fresh round: the DisplayDelay
	// call in the StepContinue branch below consumes the adapter's
	// pending-deal flag, so after that call this local is the only
	// remaining record that the broadcast above carried the deal phase.
	dealtFreshRound := s.game.DealPending()
	if res.Outcome != StepFinished && !dealtFreshRound {
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
		if dealtFreshRound {
			// DisplayDelay above must have consumed the adapter's
			// pending-deal flag by now, so this transition snapshot renders
			// passing/playing rather than a second deal.
			s.scheduleTurnDeadline()
			s.seq++
			s.broadcastSnapshot()
			if s.finished {
				return driveFinished
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
// exits. It closes the channels carried by buffered subscribe commands
// and sends an ErrGameOver result on each buffered playCmd.resp, so that
// callers waiting on those channels — a blocked SubmitAction or a
// transport writer reading a never-registered subscriber channel — do
// not wait forever.
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
