package heartsview

import (
	"encoding/json"
	"math/rand/v2"
	"testing"

	"github.com/jrgoldfinemiddleton/cardcore/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore/games/hearts/ai"
)

// benchStage pairs a stage name with the ViewState builder that produces a
// realistic game at that stage. The builder runs once per sub-benchmark,
// outside the timed loop.
type benchStage struct {
	name  string
	build func(b *testing.B) ViewState
}

// BenchmarkPlayerSnapshotSerialization measures the per-broadcast cost of
// generating a seat-filtered player snapshot for the active seat and
// marshaling it to JSON, mirroring the server's per-send path for player
// connections. The fixture is built once per stage outside the timed loop,
// so only snapshot generation and json.Marshal are measured.
func BenchmarkPlayerSnapshotSerialization(b *testing.B) {
	for _, stage := range benchStages() {
		b.Run(stage.name, func(b *testing.B) {
			vs := stage.build(b)
			seat := int(vs.Game.Turn)
			b.ReportAllocs()
			for b.Loop() {
				snap := vs.PlayerSnapshot(seat, 1)
				if _, err := json.Marshal(snap); err != nil {
					b.Fatalf("marshal player snapshot: %v", err)
				}
			}
		})
	}
}

// BenchmarkObserverSnapshotSerialization measures the per-broadcast cost of
// generating a full-information observer snapshot and marshaling it to JSON,
// mirroring the server's per-send path for observer connections. The fixture
// is built once per stage outside the timed loop, so only snapshot
// generation and json.Marshal are measured.
func BenchmarkObserverSnapshotSerialization(b *testing.B) {
	for _, stage := range benchStages() {
		b.Run(stage.name, func(b *testing.B) {
			vs := stage.build(b)
			b.ReportAllocs()
			for b.Loop() {
				snap := vs.ObserverSnapshot(1)
				if _, err := json.Marshal(snap); err != nil {
					b.Fatalf("marshal observer snapshot: %v", err)
				}
			}
		})
	}
}

// benchStages returns the named Hearts game stages shared by the snapshot
// serialization benchmarks: pass phase with full 13-card hands, mid-round
// with a partially played trick, and late round with a long trick history.
func benchStages() []benchStage {
	return []benchStage{
		{name: "passing", build: passingViewState},
		// 22 cards: five completed tricks plus two cards into trick six.
		{name: "mid_play", build: func(b *testing.B) ViewState {
			b.Helper()
			return playForwardViewState(b, 22)
		}},
		// 46 cards: eleven completed tricks plus two cards into trick twelve.
		{name: "late_game", build: func(b *testing.B) ViewState {
			b.Helper()
			return playForwardViewState(b, 46)
		}},
	}
}

// passingViewState deals a fresh deterministically seeded game and returns
// its ViewState in the pass phase, before any seat submits a pass.
func passingViewState(b *testing.B) ViewState {
	b.Helper()
	g := hearts.New(rand.New(rand.NewPCG(1, 2)))
	if err := g.Deal(); err != nil {
		b.Fatalf("Deal: %v", err)
	}
	return ViewState{Game: g}
}

// playForwardViewState deals a fresh deterministically seeded game, submits
// a random pass for every seat, and plays the given number of cards forward
// with Random players, resolving each completed trick. It returns the
// resulting mid-round ViewState.
func playForwardViewState(b *testing.B, cards int) ViewState {
	b.Helper()
	vs := passingViewState(b)
	g := vs.Game
	players := newRandomPlayers()
	for i := range hearts.NumPlayers {
		seat := hearts.Seat(i)
		pass := players[seat].ChoosePass(g, seat)
		if err := g.SetPass(seat, pass); err != nil {
			b.Fatalf("SetPass seat %d: %v", seat, err)
		}
	}
	for range cards {
		seat := g.Turn
		if err := g.PlayCard(seat, players[seat].ChoosePlay(g, seat)); err != nil {
			b.Fatalf("PlayCard seat %d: %v", seat, err)
		}
		if g.TrickPendingResolution {
			if err := g.ResolveTrick(); err != nil {
				b.Fatalf("ResolveTrick: %v", err)
			}
		}
	}
	return vs
}

// newRandomPlayers returns four Random players, one per seat, each with its
// own deterministically seeded RNG.
func newRandomPlayers() [hearts.NumPlayers]hearts.Player {
	var players [hearts.NumPlayers]hearts.Player
	for seat := hearts.Seat(0); seat < hearts.NumPlayers; seat++ {
		players[seat] = ai.NewRandom(rand.New(rand.NewPCG(1, 1+uint64(seat))))
	}
	return players
}
