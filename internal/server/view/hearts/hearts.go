package heartsview

import (
	"time"

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
	// PreviousScores is the cumulative score per seat at the start of the
	// current round. It is used to derive the actual score delta shown in the
	// round_complete snapshot.
	PreviousScores [hearts.NumPlayers]int
	// TurnDeadline is the authoritative deadline for the current human turn.
	// It is zero when no deadline is active.
	TurnDeadline time.Time
	// Paused indicates whether the game is currently paused by the UX/view layer.
	Paused bool `json:"paused"`
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
		TrickNumber:   trickNumber(g.TrickNum),
		PassDirection: heartsapi.PassDirToWire(g.PassDir),
		Turn:          int(g.Turn),
		TrickWinner:   -1,
		HeartsBroken:  g.HeartsBroken,
		Scores:        g.Scores[:],
		RoundPoints:   buildRoundPoints(vs, g),
		Trick:         buildTrick(g.Trick),
		LegalActions:  buildLegalActions(g, seat),
	}
	if vs.TrickComplete {
		winner := trickWinner(g)
		snap.TrickWinner = winner
		snap.Turn = winner
	}
	if !vs.TurnDeadline.IsZero() {
		snap.TurnDeadlineMS = vs.TurnDeadline.UnixMilli()
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

	snap.Paused = vs.Paused

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
		TrickNumber:   trickNumber(g.TrickNum),
		PassDirection: heartsapi.PassDirToWire(g.PassDir),
		Turn:          int(g.Turn),
		TrickWinner:   -1,
		HeartsBroken:  g.HeartsBroken,
		Scores:        g.Scores[:],
		RoundPoints:   buildRoundPoints(vs, g),
		Trick:         buildTrick(g.Trick),
		TrickHistory:  buildTrickHistory(g.TrickHistory),
		LegalActions:  buildLegalActions(g, g.Turn),
	}
	if vs.TrickComplete {
		winner := trickWinner(g)
		snap.TrickWinner = winner
		snap.Turn = winner
		snap.LegalActions = buildLegalActions(g, hearts.Seat(winner))
	}
	if !vs.TurnDeadline.IsZero() {
		snap.TurnDeadlineMS = vs.TurnDeadline.UnixMilli()
	}
	snap.Paused = vs.Paused

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

// buildPhase maps ViewState flags and engine phase to the wire-format phase string.
func buildPhase(vs ViewState, g *hearts.Game) string {
	if vs.Paused {
		return "paused"
	}
	if vs.TrickComplete {
		return "trick_complete"
	}
	if vs.RoundComplete {
		return "round_complete"
	}
	return heartsapi.PhaseToWire(g.Phase)
}

// buildRoundPoints returns the per-seat round points to display.
//
// During round_complete, the value is the actual score delta applied to each
// seat for the round (e.g., 0 for the moon shooter and 26 for the other seats
// when the moon is shot). During all other phases, it is the raw penalty points
// captured in tricks so far.
func buildRoundPoints(vs ViewState, g *hearts.Game) []int {
	if vs.RoundComplete {
		pts := make([]int, hearts.NumPlayers)
		for i := range hearts.NumPlayers {
			pts[i] = g.Scores[i] - vs.PreviousScores[i]
		}
		return pts
	}
	return g.RoundPts[:]
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

// trickWinner returns the seat that won the current trick, or -1 if the trick
// is empty. It follows the same rule as the engine: highest card of the led suit.
func trickWinner(g *hearts.Game) int {
	if g.Trick.Count == 0 {
		return -1
	}
	ledSuit := g.Trick.LedSuit()
	winner := g.Trick.Leader
	highRank := g.Trick.Cards[winner].Rank
	for i := 1; i < hearts.NumPlayers; i++ {
		seat := hearts.Seat((int(g.Trick.Leader) + i) % hearts.NumPlayers)
		c := g.Trick.Cards[seat]
		if c.Suit == ledSuit && c.Rank > highRank {
			winner = seat
			highRank = c.Rank
		}
	}
	return int(winner)
}

// trickNumber returns the 1-indexed trick number to display for a round, capped
// at the maximum number of tricks in a Hearts round (the hand size). The engine
// advances TrickNum to the hand size after the final trick, so without this cap
// the snapshot would expose a non-existent "trick 14".
func trickNumber(trickNum int) int {
	return min(trickNum+1, hearts.HandSize)
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
