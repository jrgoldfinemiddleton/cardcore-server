//go:build stress

package transport

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
	heartssession "github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/testutil"
)

// stressHeartsConfig implements session.GameConfig for one stress-test
// iteration. Each instance is registered under a per-iteration name and
// used for exactly one game, so its RNG is never shared across concurrent
// sessions (a rand.Rand is not safe for concurrent use), while every game
// stays exactly reproducible from its logged seed.
type stressHeartsConfig struct {
	// name is the per-iteration registered game name.
	name string
	// rng is the per-game deterministic RNG.
	rng *rand.Rand
}

// Name returns the per-iteration game name.
func (c *stressHeartsConfig) Name() string { return c.name }

// RegisterFlags is a no-op; the stress config takes no flags.
func (c *stressHeartsConfig) RegisterFlags(*flag.FlagSet) {}

// Validate is a no-op; the stress config has no flag values to check.
func (c *stressHeartsConfig) Validate() error { return nil }

// NewGame creates a real Hearts adapter with the per-game RNG and the
// startup-grace deal display delay; trick and round display delays stay
// zero so the zero-pacing stress condition is preserved.
func (c *stressHeartsConfig) NewGame(
	cfg session.Config, _ *rand.Rand,
) (session.Game, error) {
	return heartssession.NewGameAdapter(cfg.Seats, c.rng, stressDealDelayMS, 0, 0)
}

// ValidateConfig delegates to the Hearts config validator so stress games
// enforce the same seat-validation rules as production games without
// duplicating them.
func (c *stressHeartsConfig) ValidateConfig(cfg session.Config) error {
	return heartssession.NewGameConfig().ValidateConfig(cfg)
}

// setupStressServer creates a Server whose registry holds one Hearts game
// config per iteration plus one for the warmup game, each with its own
// deterministic RNG derived from pcg(baseSeed, iteration).
func setupStressServer(t *testing.T, baseSeed uint64) (*Server, *session.Manager) {
	t.Helper()
	registry := session.NewRegistry()
	for i := -1; i < stressTotalGames; i++ {
		registry.Register(&stressHeartsConfig{
			name: stressGameName(i),
			rng:  rand.New(rand.NewPCG(baseSeed, stressSeed2(i))),
		})
	}
	mgr := session.NewManager(registry, session.DefaultServerDelays)
	return NewServer(Config{Manager: mgr}), mgr
}

// playStressGame runs one zero-pacing all-AI Hearts game to completion and
// returns the measured outcome. It never calls t.Fatal: it runs on a
// worker goroutine, so harness failures travel through gameResult.err and
// are asserted by the main test goroutine.
func playStressGame(httpSrvURL string, mgr *session.Manager, iteration int) gameResult {
	var res gameResult

	cfg := testutil.HeartsAllAISessionConfigWithPacing(0)
	cfg.Game = stressGameName(iteration)
	info, _, err := mgr.Create(cfg)
	if err != nil {
		res.err = fmt.Errorf("Create: %w", err)
		return res
	}
	id := info.SessionID
	if err := mgr.Start(id); err != nil {
		_ = mgr.Delete(id)
		res.err = fmt.Errorf("Start: %w", err)
		return res
	}

	ctx, cancel := context.WithTimeout(context.Background(), stressGameWatchdog)
	defer cancel()

	conns := make([]*websocket.Conn, 0, stressObserversPerGame)
	for j := range stressObserversPerGame {
		conn, err := dialObserverWS(httpSrvURL, id)
		if err != nil {
			res.err = fmt.Errorf("dial observer %d: %w", j, err)
			// Close already-dialed connections so error paths do not
			// leak server-side connection goroutines into the census.
			for _, c := range conns {
				_ = c.Close(websocket.StatusNormalClosure, "")
			}
			_ = mgr.Delete(id)
			return res
		}
		conns = append(conns, conn)
	}

	// One ordered result channel per game: each drain goroutine does
	// minimal work (read; decode phase and seq only) and sends exactly one
	// final result when its stream ends, so the drain is never the
	// bottleneck and the measurement reflects the server, not the harness.
	out := make(chan observerResult, stressObserversPerGame)
	for _, conn := range conns {
		go drainObserver(ctx, conn, out)
	}
	res.obs = make([]observerResult, 0, stressObserversPerGame)
	for range conns {
		res.obs = append(res.obs, <-out)
	}

	// Game completion is proven server-side via the session state; the
	// observer stream is only the measurement instrument. onDone
	// transitions state asynchronously, so poll briefly.
	if err := waitForFinishedState(mgr, id); err != nil {
		res.err = err
	}
	_ = mgr.Delete(id)

	// Terminal loss: an observer's connection closed while its last
	// observed phase was not game_over. Streams with read errors are
	// harness failures (reported via res.obs), not loss data.
	for _, o := range res.obs {
		if o.readErr == nil && o.lastPhase != "game_over" {
			res.terminalLoss = true
		}
	}
	return res
}

// drainObserver reads the observer WebSocket until it closes, doing the
// minimal work required for measurement: unmarshal only phase and seq,
// track the last observed values, and count seq gaps. The first observed
// snapshot only establishes the seq baseline; its lead-in is not counted
// as a gap because a late subscription is indistinguishable from a dropped
// early snapshot. Exactly one result is sent on out when the stream ends.
func drainObserver(ctx context.Context, conn *websocket.Conn, out chan<- observerResult) {
	var res observerResult
	for {
		typ, b, err := conn.Read(ctx)
		if err != nil {
			// A normal closure (or EOF) after game end is the expected
			// stream end — and, when the terminal snapshot was dropped,
			// the loss event itself. Anything else is a harness failure.
			if !isNormalWSClose(err) {
				res.readErr = err
			}
			break
		}
		if typ != websocket.MessageText {
			res.readErr = fmt.Errorf("got message type %d, want text", typ)
			break
		}
		var snap testSnapshot
		if err := json.Unmarshal(b, &snap); err != nil {
			res.readErr = fmt.Errorf("unmarshal snapshot: %w", err)
			break
		}
		if res.snapshots > 0 && snap.Seq > res.lastSeq+1 {
			res.gaps += snap.Seq - res.lastSeq - 1
		}
		if snap.Seq > res.lastSeq {
			res.lastSeq = snap.Seq
		}
		res.lastPhase = snap.Phase
		res.snapshots++
	}
	out <- res
}

// dialObserverWS dials the observer WebSocket endpoint and returns an
// error instead of failing the test. It mirrors mustDialObserverWS, which
// cannot be used here: its t.Fatalf would run on a worker goroutine,
// aborting the goroutine and silently losing the game result.
func dialObserverWS(httpSrvURL, id string) (*websocket.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(httpSrvURL, "http") +
		"/sessions/" + id + "/ws/observe"
	conn, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("got status %d, want %d",
			resp.StatusCode, http.StatusSwitchingProtocols)
	}
	return conn, nil
}

// waitForFinishedState polls the manager until the session reaches
// Finished or the deadline expires. The session's onDone callback
// transitions state asynchronously, so a short poll is required even after
// all observer streams have closed.
func waitForFinishedState(mgr *session.Manager, id string) error {
	deadline := time.Now().Add(stressFinishedWait)
	for {
		info, err := mgr.Get(id)
		if err != nil {
			return fmt.Errorf("Get: %w", err)
		}
		if info.State == session.Finished {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session state got %q, want %q",
				info.State, session.Finished)
		}
		time.Sleep(stressFinishedPoll)
	}
}

// isNormalWSClose reports whether err is the expected end of an observer
// stream: a normal WebSocket closure or EOF after the game has ended.
func isNormalWSClose(err error) bool {
	return errors.Is(err, io.EOF) ||
		websocket.CloseStatus(err) == websocket.StatusNormalClosure
}

// stressGameName returns the registered game name for an iteration;
// negative iterations name the warmup game.
func stressGameName(iteration int) string {
	if iteration < 0 {
		return "hearts-stress-warmup"
	}
	return fmt.Sprintf("hearts-stress-%03d", iteration)
}

// stressSeed2 returns the second PCG seed component for an iteration.
func stressSeed2(iteration int) uint64 {
	if iteration < 0 {
		return stressWarmupSeed2
	}
	return uint64(iteration)
}
