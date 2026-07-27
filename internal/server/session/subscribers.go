package session

import (
	"encoding/json"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
)

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
		s.autoUnpause(c.seat)
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
