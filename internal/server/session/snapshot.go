package session

import (
	"encoding/json"
)

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

// cacheDealSnapshots preserves the deal view for subscribers joining during
// the display window after DisplayDelay consumes the adapter's pending flag.
// It returns false when a snapshot fails to marshal; the session is then
// finished and the caller must abandon the deal phase.
func (s *session) cacheDealSnapshots() bool {
	s.dealObserverSnapshot = s.observerSnapshot()
	if s.dealObserverSnapshot == nil {
		s.terminateOnMarshalFailure("observer deal snapshot marshal failed")
		return false
	}
	s.dealPlayerSnapshots = make(map[int][]byte)
	for seat, cfg := range s.config.Seats {
		if cfg.Type != SeatHuman {
			continue
		}
		snap := s.playerSnapshot(seat)
		if snap == nil {
			s.terminateOnMarshalFailure("player deal snapshot marshal failed", "seat", seat)
			return false
		}
		s.dealPlayerSnapshots[seat] = snap
	}
	return true
}

// clearDealSnapshots ends the late-subscription deal window before the
// actionable transition is broadcast.
func (s *session) clearDealSnapshots() {
	s.dealPlayerSnapshots = nil
	s.dealObserverSnapshot = nil
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
