package session

import (
	"encoding/json"
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
	closeOnce sync.Once
}

// run is the session goroutine event loop.
func (s *session) run() {
	defer close(s.done)
	for {
		select {
		case cmd := <-s.cmds:
			s.handleCommand(cmd)
		case <-s.cancel:
			s.closeSubscribers()
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
	// the client can resync, along with an error.
	if c.msg.Seq < s.seq {
		snap := s.playerSnapshot(c.seat)
		c.resp <- SubmitResult{
			Snapshot: snap,
			Err: &api.ErrorMessage{
				Type:      errType,
				ErrorCode: api.ErrStaleSeq,
				Message:   "client seq is behind server",
				ActionID:  c.msg.ActionID,
			},
		}
		return
	}

	// Duplicate action_id: client resent a command that already
	// succeeded. Return the cached snapshot without mutating state.
	if cached, ok := s.actionIDs[c.msg.ActionID]; ok {
		c.resp <- SubmitResult{Snapshot: cached}
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
	// subscribers, and cache the snapshot for idempotent replay.
	s.seq++
	s.broadcastSnapshot()
	s.actionIDs[c.msg.ActionID] = s.playerSnapshot(c.seat)
	if len(s.actionIDs) > actionIDCacheSize {
		s.evictArbitraryActionID()
	}

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
// TODO: Implement AI turn scheduling — if the next seat is AI, spawn a
// goroutine that sleeps for AIDelayMS then submits an AI move via s.cmds.
// If the next seat is human, this function returns immediately and the
// session goroutine waits for the next human playCmd.
func (s *session) scheduleAI() {
	seat := s.game.Turn()
	_ = seat
}

// playerSnapshot generates a marshaled player snapshot for the given seat.
func (s *session) playerSnapshot(seat int) []byte {
	snap := s.game.PlayerSnapshot(seat, s.seq)
	b, err := json.Marshal(snap)
	if err != nil {
		return nil
	}
	return b
}

// observerSnapshot generates a marshaled observer snapshot.
func (s *session) observerSnapshot() []byte {
	snap := s.game.ObserverSnapshot(s.seq)
	b, err := json.Marshal(snap)
	if err != nil {
		return nil
	}
	return b
}

// broadcastSnapshot generates and sends snapshots to all subscribers.
func (s *session) broadcastSnapshot() {
	obsSnap := s.observerSnapshot()
	for _, ch := range s.observers {
		s.sendNonBlocking(ch, obsSnap)
	}

	for seat, ch := range s.players {
		snap := s.playerSnapshot(seat)
		s.sendNonBlocking(ch, snap)
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
	s.sendNonBlocking(c.ch, snap)
}

// handleSubscribeObserver registers a new observer subscriber.
func (s *session) handleSubscribeObserver(c subscribeObserverCmd) {
	s.observers = append(s.observers, c.ch)
	snap := s.observerSnapshot()
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

// closeSubscribers closes all subscriber channels exactly once.
func (s *session) closeSubscribers() {
	s.closeOnce.Do(func() {
		for _, ch := range s.players {
			close(ch)
		}
		for _, ch := range s.observers {
			close(ch)
		}
	})
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
	if c.AIDelayMS != nil {
		return *c.AIDelayMS
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
