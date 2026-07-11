package heartsview

import (
	"github.com/jrgoldfinemiddleton/cardcore/games/hearts"

	heartsapi "github.com/jrgoldfinemiddleton/cardcore-server/internal/api/games/hearts"
)

// ViewState wraps a Hearts game with server-synthesized phase transition flags.
// TrickComplete and RoundComplete let the session layer signal transitions before
// the engine phase advances.
type ViewState struct {
	// Game is the current Hearts game state.
	Game *hearts.Game
	// TrickComplete signals a completed trick before the engine advances.
	TrickComplete bool
	// RoundComplete signals a completed round before EndRound is called.
	RoundComplete bool
}

// PlayerView generates a seat-filtered snapshot for the given player.
// The game is cloned before reading so the original is never mutated.
func PlayerView(vs ViewState, seat hearts.Seat, seq int) *heartsapi.PlayerSnapshot {
	g := vs.Game.Clone()

	snap := &heartsapi.PlayerSnapshot{
		Type:          "snapshot",
		Seq:           seq,
		Phase:         buildPhase(vs, g),
		RoundNumber:   g.Round + 1,
		TrickNumber:   g.TrickNum + 1,
		PassDirection: heartsapi.PassDirToWire(g.PassDir),
		Turn:          int(g.Turn),
		HeartsBroken:  g.HeartsBroken,
		Scores:        g.Scores[:],
		RoundPoints:   g.RoundPts[:],
		Trick:         buildTrick(snapshotTrick(vs, g)),
		LegalActions:  buildLegalActions(g, seat),
	}

	snap.Hand = []heartsapi.Card{}
	if h := g.Hands[seat]; h != nil {
		h.Sort()
		snap.Hand = make([]heartsapi.Card, 0, h.Len())
		for _, c := range h.Cards {
			snap.Hand = append(snap.Hand, heartsapi.CardFromEngine(c))
		}
	}

	snap.HandCounts = make([]int, hearts.NumPlayers)
	for i := range hearts.NumPlayers {
		if g.Hands[i] != nil {
			snap.HandCounts[i] = g.Hands[i].Len()
		}
	}

	return snap
}

// ObserverView generates a full-information snapshot for an observer.
// The game is cloned before reading so the original is never mutated.
func ObserverView(vs ViewState, seq int) *heartsapi.ObserverSnapshot {
	g := vs.Game.Clone()

	snap := &heartsapi.ObserverSnapshot{
		Type:          "snapshot",
		Seq:           seq,
		Phase:         buildPhase(vs, g),
		RoundNumber:   g.Round + 1,
		TrickNumber:   g.TrickNum + 1,
		PassDirection: heartsapi.PassDirToWire(g.PassDir),
		Turn:          int(g.Turn),
		HeartsBroken:  g.HeartsBroken,
		Scores:        g.Scores[:],
		RoundPoints:   g.RoundPts[:],
		Trick:         buildTrick(snapshotTrick(vs, g)),
		TrickHistory:  buildTrickHistory(g.TrickHistory),
		LegalActions:  buildLegalActions(g, g.Turn),
	}

	snap.Hands = make([][]heartsapi.Card, hearts.NumPlayers)
	for i := range hearts.NumPlayers {
		snap.Hands[i] = []heartsapi.Card{}
		if h := g.Hands[i]; h != nil {
			h.Sort()
			hand := make([]heartsapi.Card, 0, h.Len())
			for _, c := range h.Cards {
				hand = append(hand, heartsapi.CardFromEngine(c))
			}
			snap.Hands[i] = hand
		}
	}

	snap.HandCounts = make([]int, hearts.NumPlayers)
	for i := range hearts.NumPlayers {
		if g.Hands[i] != nil {
			snap.HandCounts[i] = g.Hands[i].Len()
		}
	}

	return snap
}

// snapshotTrick returns the trick to render in a snapshot.
//
// This is a workaround: the engine resolves the trick inside the same PlayCard
// call that plays the fourth card, so by the time the session goroutine
// broadcasts the trick_complete snapshot, g.Trick has already been cleared and
// reset for the next trick. The completed trick is preserved as the last entry
// in g.TrickHistory, so we render that instead.
//
// The proper long-term fix is to separate playing the fourth card from
// resolving the trick in the engine.
func snapshotTrick(vs ViewState, g *hearts.Game) hearts.Trick {
	if vs.TrickComplete && len(g.TrickHistory) > 0 {
		return g.TrickHistory[len(g.TrickHistory)-1]
	}
	return g.Trick
}

// buildPhase maps ViewState flags and engine phase to the wire-format phase string.
func buildPhase(vs ViewState, g *hearts.Game) string {
	if vs.TrickComplete {
		return "trick_complete"
	}
	if vs.RoundComplete {
		return "round_complete"
	}
	return heartsapi.PhaseToWire(g.Phase)
}

// buildTrick converts a trick to wire-format entries in play order from the leader.
func buildTrick(trick hearts.Trick) []heartsapi.TrickEntry {
	if trick.Count == 0 {
		return []heartsapi.TrickEntry{}
	}
	entries := make([]heartsapi.TrickEntry, 0, trick.Count)
	for i := range trick.Count {
		s := (int(trick.Leader) + i) % hearts.NumPlayers
		entries = append(entries, heartsapi.TrickEntry{
			Seat: s,
			Card: heartsapi.CardFromEngine(trick.Cards[s]),
		})
	}
	return entries
}

// buildTrickHistory converts a slice of completed tricks to wire-format entry slices.
func buildTrickHistory(history []hearts.Trick) [][]heartsapi.TrickEntry {
	if len(history) == 0 {
		return [][]heartsapi.TrickEntry{}
	}
	result := make([][]heartsapi.TrickEntry, 0, len(history))
	for _, trick := range history {
		result = append(result, buildTrick(trick))
	}
	return result
}

// buildLegalActions returns legal card choices for seat in wire format.
// In pass phase the full hand is returned; in play phase only legal plays are
// returned; in all other phases or when it is not the seat's turn an empty
// slice is returned.
func buildLegalActions(g *hearts.Game, seat hearts.Seat) []heartsapi.Card {
	if g.Phase == hearts.PhasePass {
		h := g.Hands[seat]
		result := make([]heartsapi.Card, 0, h.Len())
		for _, c := range h.Cards {
			result = append(result, heartsapi.CardFromEngine(c))
		}
		return result
	}
	moves, err := g.LegalMoves(seat)
	if err != nil {
		return []heartsapi.Card{}
	}
	result := make([]heartsapi.Card, 0, len(moves))
	for _, c := range moves {
		result = append(result, heartsapi.CardFromEngine(c))
	}
	return result
}
