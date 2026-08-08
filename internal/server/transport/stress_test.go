//go:build stress

package transport

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/testutil"
)

const (
	// stressTotalGames is the number of measured all-AI Hearts games.
	stressTotalGames = 100
	// stressBatchSize bounds concurrency: only this many games run at once.
	stressBatchSize = 10
	// stressObserversPerGame is the number of observer WebSocket
	// connections drained per game.
	stressObserversPerGame = 2
	// stressDealDelayMS is the deal display delay wired into each Hearts
	// adapter. At zero AI pacing a game can otherwise complete before the
	// observer WebSocket handshake finishes, making subscription fail with
	// ErrNotActive; the deal delay is the startup grace window that lets
	// both observers subscribe before the first AI turn. AI turn pacing
	// stays zero, so the stress condition is preserved.
	stressDealDelayMS = 250
	// stressGameWatchdog is the per-game deadline, sized for the race
	// detector per doc/integration-testing.md §5. Zero-pacing games finish
	// in well under a second normally, so this only catches true hangs.
	stressGameWatchdog = 120 * time.Second
	// stressFinishedWait bounds the poll for the asynchronous onDone
	// state transition after all observer streams have closed.
	stressFinishedWait = 2 * time.Second
	// stressFinishedPoll is the interval between Finished-state polls.
	stressFinishedPoll = 10 * time.Millisecond
	// stressCensusWait bounds the post-run goroutine census poll.
	stressCensusWait = 10 * time.Second
	// stressCensusPoll is the interval between goroutine census samples.
	stressCensusPoll = 100 * time.Millisecond
	// stressGoroutineSlack is the tolerated excess over the goroutine
	// baseline. It absorbs goroutines that briefly outlive the final game:
	// the session goroutine's deferred done close and observer connection
	// teardown trailing the last close frame by milliseconds. It is far
	// too small to hide a per-game leak, which would accumulate by the
	// hundreds (one session goroutine plus three observer connection
	// goroutines per connection).
	stressGoroutineSlack = 4
	// stressWarmupSeed2 is the second PCG seed component for the warmup
	// game, chosen far outside the measured iteration range.
	stressWarmupSeed2 uint64 = 0xC0FFEE
)

// observerResult is the final outcome of one observer drain goroutine.
// Data and the terminal error travel together in a single message so
// wire-read order is preserved (doc/integration-testing.md §7).
type observerResult struct {
	// lastPhase is the phase of the final snapshot observed before close.
	lastPhase string
	// lastSeq is the highest seq observed.
	lastSeq int
	// gaps is the number of seq values skipped between observed snapshots
	// (intermediate drops).
	gaps int
	// snapshots is the number of snapshots observed.
	snapshots int
	// readErr is the unexpected read error that ended the stream, or nil
	// for a normal close/EOF.
	readErr error
}

// gameResult is the measured outcome of one stress-test game.
type gameResult struct {
	// obs holds one result per observer stream, in drain-completion order.
	obs []observerResult
	// terminalLoss is true when any cleanly drained observer's connection
	// closed while its last observed phase was not "game_over".
	terminalLoss bool
	// err is the harness failure (create/start/dial/state), if any.
	err error
}

// TestStressAllAIGames plays stressTotalGames zero-pacing all-AI Hearts
// games against one real server (stressBatchSize games at a time), with
// stressObserversPerGame observer WebSocket connections per game drained
// by dedicated goroutines. It measures terminal-snapshot loss (an
// observer's connection closing while its last observed phase is not
// "game_over") and intermediate drops (seq gaps in each observer stream),
// then reports both via a greppable sentinel line.
//
// Snapshot delivery is lossy by design: the session goroutine sends into a
// 64-slot subscriber buffer non-blocking and drops on overflow, and the
// terminal broadcast is droppable because subscriber channels are closed
// roughly 100ms after game over. This test MEASURES that behavior; it
// never asserts a loss bound. The hard assertions cover harness integrity
// only: every game reaches Finished server-side, every observer stream
// drains until a normal close without unexpected read errors, and the
// goroutine census returns to baseline.
//
// This test does not call t.Parallel(): the goroutine census requires a
// quiet process (justified exception to doc/integration-testing.md §1).
func TestStressAllAIGames(t *testing.T) {
	// The batch loop relies on exact divisibility; refuse to run rather
	// than silently skipping remainder games if the constants drift apart.
	if stressTotalGames%stressBatchSize != 0 {
		t.Fatalf("stressTotalGames (%d) must be a multiple of "+
			"stressBatchSize (%d)", stressTotalGames, stressBatchSize)
	}

	baseSeed := testutil.HashTestName(t.Name())
	srv, mgr := setupStressServer(t, baseSeed)
	httpSrv := mustStartTestServer(t, srv)

	// One warmup game absorbs one-time library goroutines (WebSocket and
	// httptest bookkeeping) so the baseline census only measures
	// steady-state per-game churn. It runs the same harness and is held to
	// the same integrity bar, but its loss data is excluded from the
	// sentinel.
	t.Logf("stress: warmup game seed pcg(%d, %d)", baseSeed, stressWarmupSeed2)
	warmup := playStressGame(httpSrv.URL, mgr, -1)
	if warmup.err != nil {
		t.Fatalf("warmup game: %v", warmup.err)
	}
	for i, o := range warmup.obs {
		if o.readErr != nil {
			t.Fatalf("warmup game observer %d read error: %v", i, o.readErr)
		}
	}

	baseline := runtime.NumGoroutine()
	t.Logf("stress: goroutine baseline %d (after warmup)", baseline)

	var lossGames, totalDrops int
	for batch := range stressTotalGames / stressBatchSize {
		// The batch boundary is the concurrency bound: exactly
		// stressBatchSize games run between Wait calls.
		var wg sync.WaitGroup
		batchResults := make([]gameResult, stressBatchSize)
		for lane := range stressBatchSize {
			i := batch*stressBatchSize + lane
			t.Logf("stress: game %d seed pcg(%d, %d)", i, baseSeed, i)
			wg.Go(func() {
				batchResults[lane] = playStressGame(httpSrv.URL, mgr, i)
			})
		}
		wg.Wait()

		for lane := range stressBatchSize {
			i := batch*stressBatchSize + lane
			r := &batchResults[lane]
			if r.err != nil {
				t.Errorf("game %d (seed pcg(%d, %d)): harness: %v",
					i, baseSeed, i, r.err)
				continue
			}
			for j, o := range r.obs {
				if o.readErr != nil {
					t.Errorf("game %d observer %d: unexpected read error: "+
						"%v (last phase %q, last seq %d)",
						i, j, o.readErr, o.lastPhase, o.lastSeq)
					continue
				}
				if o.snapshots == 0 {
					t.Errorf("game %d observer %d: received no snapshots", i, j)
				}
				totalDrops += o.gaps
			}
			if r.terminalLoss {
				lossGames++
				t.Logf("stress: game %d (seed pcg(%d, %d)): terminal snapshot loss",
					i, baseSeed, i)
			}
		}
		t.Logf("stress: batch %d/%d done; running totals: terminal loss %d, "+
			"intermediate drops %d",
			batch+1, stressTotalGames/stressBatchSize, lossGames, totalDrops)
	}

	t.Logf("stress: terminal snapshot loss %d/%d (%.1f%%), intermediate drops %d, race=%t",
		lossGames, stressTotalGames,
		float64(lossGames)/float64(stressTotalGames)*100,
		totalDrops, raceEnabled)

	checkGoroutineCensus(t, baseline)
}

// checkGoroutineCensus polls until the goroutine count returns to within
// stressGoroutineSlack of baseline, failing the test on timeout.
func checkGoroutineCensus(t *testing.T, baseline int) {
	t.Helper()
	deadline := time.Now().Add(stressCensusWait)
	for {
		got := runtime.NumGoroutine()
		if got <= baseline+stressGoroutineSlack {
			t.Logf("stress: goroutine census %d (baseline %d + slack %d)",
				got, baseline, stressGoroutineSlack)
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("goroutine census got %d, want <= %d (baseline %d + slack %d)",
				got, baseline+stressGoroutineSlack, baseline, stressGoroutineSlack)
			return
		}
		time.Sleep(stressCensusPoll)
	}
}
