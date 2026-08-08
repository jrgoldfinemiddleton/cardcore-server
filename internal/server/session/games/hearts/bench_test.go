package heartssession

import (
	"math/rand/v2"
	"testing"

	"github.com/jrgoldfinemiddleton/cardcore/games/hearts"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// BenchmarkAIPlay measures GameAdapter.AIPlay per-call cost: the full
// server-side adapter path the session goroutine executes for an AI
// turn, covering the AI decision (ChoosePass or ChoosePlay), the
// engine mutation, and the adapter state update. It complements the
// cardcore engine's raw ChoosePass/ChoosePlay benchmarks, which
// measure the decision alone.
//
// Sub-benchmarks split by phase (passing exercises the ChoosePass
// path, playing the ChoosePlay path) and by AI type. Each timed
// iteration performs exactly one AIPlay on the current turn seat,
// read via the Game interface Turn method. A playing-phase iteration
// that completes a trick or round also runs the Resume chain its
// StepPause outcome triggers, mirroring the session goroutine's turn
// cycle.
//
// Adapter recreation after game completion and pass-phase driving
// between rounds are harness artifacts, not adapter-path work, and
// are excluded via StopTimer/StartTimer. RNGs are seeded once per
// sub-benchmark, so the deal sequence is deterministic. ReportAllocs
// ON — per-call allocs/op tracks adapter-side allocation.
func BenchmarkAIPlay(b *testing.B) {
	for _, aiType := range []string{"random", "heuristic"} {
		b.Run("passing/"+aiType, func(b *testing.B) {
			benchAIPlayPassing(b, aiType)
		})
		b.Run("playing/"+aiType, func(b *testing.B) {
			benchAIPlayPlaying(b, aiType)
		})
	}
}

// benchAIPlayPassing runs the passing-phase AIPlay sub-benchmark for
// the given AI type. The fourth pass of a round ends the phase, so
// the adapter is recreated (untimed) whenever the phase changes.
func benchAIPlayPassing(b *testing.B, aiType string) {
	b.Helper()
	seats := benchAISeats(aiType)
	rng := rand.New(rand.NewPCG(1, 2))
	a := benchPassPhaseAdapter(b, seats, rng)
	b.ReportAllocs()
	for b.Loop() {
		seat := a.Turn()
		if _, err := a.AIPlay(seat); err != nil {
			b.Fatalf("AIPlay seat %d: %v", seat, err)
		}
		if a.game.Phase != hearts.PhasePass {
			b.StopTimer()
			a = benchPassPhaseAdapter(b, seats, rng)
			b.StartTimer()
		}
	}
}

// benchAIPlayPlaying runs the playing-phase AIPlay sub-benchmark for
// the given AI type. Resume chains triggered by StepPause outcomes
// are part of the adapter turn cycle and stay in the timed region;
// game completion and between-round passing are excluded.
func benchAIPlayPlaying(b *testing.B, aiType string) {
	b.Helper()
	seats := benchAISeats(aiType)
	rng := rand.New(rand.NewPCG(1, 2))
	a := benchPlayPhaseAdapter(b, seats, rng)
	b.ReportAllocs()
	for b.Loop() {
		seat := a.Turn()
		res, err := a.AIPlay(seat)
		if err != nil {
			b.Fatalf("AIPlay seat %d: %v", seat, err)
		}
		for res.Outcome == session.StepPause {
			res, err = a.Resume()
			if err != nil {
				b.Fatalf("Resume: %v", err)
			}
		}
		// StepFinished means the game is over; PhasePass here means the
		// Resume chain dealt a new round with a pass phase (the initial
		// pass phase was already consumed by benchPlayPhaseAdapter before
		// timing began). Both are harness artifacts, not adapter-path
		// work, so recreation and pass driving stay untimed.
		if res.Outcome == session.StepFinished ||
			a.game.Phase == hearts.PhasePass {
			b.StopTimer()
			if res.Outcome == session.StepFinished {
				a = benchNewAdapter(b, seats, rng)
			}
			for a.game.Phase == hearts.PhasePass {
				if _, err := a.AIPlay(a.Turn()); err != nil {
					b.Fatalf("AIPlay pass: %v", err)
				}
			}
			b.StartTimer()
		}
	}
}

// benchPassPhaseAdapter returns a freshly constructed adapter, which
// is always in the passing phase: a fresh deal is round 0, a
// pass-left round. The phase check guards that invariant.
func benchPassPhaseAdapter(
	b *testing.B, seats []session.SeatConfig, rng *rand.Rand,
) *GameAdapter {
	b.Helper()
	a := benchNewAdapter(b, seats, rng)
	if a.game.Phase != hearts.PhasePass {
		b.Fatalf("got phase %d, want PhasePass", a.game.Phase)
	}
	return a
}

// benchPlayPhaseAdapter returns a freshly constructed adapter driven
// through its initial passing phase into the playing phase.
func benchPlayPhaseAdapter(
	b *testing.B, seats []session.SeatConfig, rng *rand.Rand,
) *GameAdapter {
	b.Helper()
	a := benchNewAdapter(b, seats, rng)
	for a.game.Phase == hearts.PhasePass {
		if _, err := a.AIPlay(a.Turn()); err != nil {
			b.Fatalf("AIPlay pass: %v", err)
		}
	}
	if a.game.Phase != hearts.PhasePlay {
		b.Fatalf("got phase %d, want PhasePlay", a.game.Phase)
	}
	return a
}

// benchNewAdapter returns a Hearts adapter with the given seats, zero
// display delays, and the sub-benchmark's deterministic RNG stream.
func benchNewAdapter(
	b *testing.B, seats []session.SeatConfig, rng *rand.Rand,
) *GameAdapter {
	b.Helper()
	a, err := NewGameAdapter(seats, rng, 0, 0, 0)
	if err != nil {
		b.Fatalf("NewGameAdapter: %v", err)
	}
	return a
}

// benchAISeats returns a seat configuration with four AI seats of the
// given AI type.
func benchAISeats(aiType string) []session.SeatConfig {
	return []session.SeatConfig{
		{Type: session.SeatAI, AIType: aiType},
		{Type: session.SeatAI, AIType: aiType},
		{Type: session.SeatAI, AIType: aiType},
		{Type: session.SeatAI, AIType: aiType},
	}
}
