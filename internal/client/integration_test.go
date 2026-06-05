package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
	heartssession "github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/transport"
)

// TestIntegrationFullLifecycle connects a human player and plays a full game,
// verifying snapshot delivery, seq monotonicity, and phase progression.
func TestIntegrationFullLifecycle(t *testing.T) {
	t.Parallel()
	// 120s timeout accommodates the race detector, which slows
	// goroutine scheduling and JSON unmarshaling by ~10x. The
	// transport-level full-game tests use the same timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	srv := setupTestServer(t)
	baseURL := "http://" + srv.Addr()

	// 10ms pacing keeps the sequential read loop from falling behind
	// the server's broadcast rate. With PacingDelayMS: 0, the server
	// generates ~900 snapshots in ~100ms, faster than the player loop
	// can consume them. Under parallel load the 64-slot subscriber
	// buffer overflows and sendNonBlocking drops snapshots (including
	// game_over), causing flaky failures. 10ms is the same value used
	// by the transport-level full-game tests.
	delay := 10
	cfg := client.Config{
		Game: "hearts",
		Seats: []client.SeatConfig{
			{Type: "human"},
			{Type: "ai", AIType: "random"},
			{Type: "ai", AIType: "random"},
			{Type: "ai", AIType: "random"},
		},
		PacingDelayMS: &delay,
	}

	sc := &client.SessionClient{BaseURL: baseURL}
	id, seats, err := sc.CreateSession(ctx, cfg)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	var token string
	for _, s := range seats {
		if s.Type == "human" {
			token = s.Token
			break
		}
	}
	if token == "" {
		t.Fatal("no human seat token found")
	}

	if err := sc.StartSession(ctx, id); err != nil {
		t.Fatalf("start session: %v", err)
	}

	wsURL := "ws://" + srv.Addr() + "/sessions/" + id + "/ws"
	conn := &client.Conn{}
	if err := conn.Connect(ctx, wsURL, token); err != nil {
		t.Fatalf("connect websocket: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// A goroutine reader with a buffered channel prevents the server's
	// 64-slot subscriber buffer from overflowing. Under race-detector
	// load the sequential read loop in earlier versions could not keep
	// up with the server's snapshot production rate, causing
	// sendNonBlocking to drop snapshots (including game_over) and making
	// the test flaky in CI.
	const bufSize = 256
	type result struct {
		data json.RawMessage
		err  error
	}
	resCh := make(chan result, bufSize)
	go func() {
		for {
			data, err := conn.ReadSnapshot(ctx)
			resCh <- result{data: data, err: err}
			if err != nil {
				return
			}
		}
	}()

	// maxRounds is a safety limit. A full Hearts game generates ~900
	// snapshots; 1000 prevents an infinite loop if the game never reaches
	// game_over (e.g., due to a client command that is silently ignored).
	var (
		lastSeq   int
		gotPass   bool
		gotPlay   bool
		gotOver   bool
		maxRounds = 1000
	)
	for range maxRounds {
		res := <-resCh
		if res.err != nil {
			// gotOver is set on the same iteration that calls
			// break — there is no subsequent iteration where it could
			// be true while res.err != nil.
			t.Fatalf("read snapshot: %v", res.err)
		}

		var snap struct {
			Type  string `json:"type"`
			Seq   int    `json:"seq"`
			Phase string `json:"phase"`
			Turn  int    `json:"turn"`
		}
		if err := json.Unmarshal(res.data, &snap); err != nil {
			t.Fatalf("unmarshal snapshot: %v", err)
		}

		if snap.Seq <= lastSeq {
			t.Fatalf("seq not monotonic: got %d, last %d", snap.Seq, lastSeq)
		}
		lastSeq = snap.Seq

		if snap.Phase == "game_over" {
			gotOver = true
			break
		}
		if snap.Phase == "passing" && snap.Turn == 0 {
			gotPass = true
			var handSnap struct {
				Hand []heartsclient.Card `json:"hand"`
			}
			if err := json.Unmarshal(res.data, &handSnap); err != nil {
				t.Fatalf("unmarshal hand: %v", err)
			}
			if len(handSnap.Hand) < 3 {
				t.Fatalf("hand too small to pass: got %d, want >= 3", len(handSnap.Hand))
			}
			// Cards must come from the actual hand. Arbitrary cards like
			// "two of clubs" may not be held by this seat, and the engine
			// rejects them with "player N does not have X".
			actionID := fmt.Sprintf("pass-%d", snap.Seq)
			cmd, err := heartsclient.NewPassCardsMessage(
				actionID, snap.Seq, handSnap.Hand[:3],
			)
			if err != nil {
				t.Fatalf("build pass command: %v", err)
			}
			if err := conn.SendCommand(ctx, cmd); err != nil {
				t.Fatalf("send pass command: %v", err)
			}
			continue
		}
		if snap.Phase == "playing" && snap.Turn == 0 {
			gotPlay = true
			var actionSnap struct {
				LegalActions []heartsclient.Card `json:"legal_actions"`
			}
			if err := json.Unmarshal(res.data, &actionSnap); err != nil {
				t.Fatalf("unmarshal legal actions: %v", err)
			}
			if len(actionSnap.LegalActions) == 0 {
				t.Fatal("no legal actions but it's our turn")
			}
			// Use legal_actions, not hand[0]. The server validates every
			// play against game rules (follow suit, lead restrictions, etc.).
			// legal_actions[0] is guaranteed acceptable; hand[0] often is not.
			actionID := fmt.Sprintf("play-%d", snap.Seq)
			cmd, err := heartsclient.NewPlayCardMessage(
				actionID, snap.Seq, actionSnap.LegalActions[0],
			)
			if err != nil {
				t.Fatalf("build play command: %v", err)
			}
			if err := conn.SendCommand(ctx, cmd); err != nil {
				t.Fatalf("send play command: %v", err)
			}
			continue
		}
	}

	// A human player should always see all three phases. gotPass and
	// gotPlay verify the human was actually asked to act; gotOver
	// verifies the game reached completion.
	if !gotPass {
		t.Error("never saw a passing phase with our turn")
	}
	if !gotPlay {
		t.Error("never saw a playing phase with our turn")
	}
	if !gotOver {
		t.Error("never saw game_over phase")
	}
}

// TestIntegrationObserverFullGame connects as an observer and reads snapshots
// until game_over, verifying seq monotonicity.
func TestIntegrationObserverFullGame(t *testing.T) {
	t.Parallel()
	// 120s timeout accommodates the race detector; see
	// TestIntegrationFullLifecycle for the rationale.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	srv := setupTestServer(t)
	baseURL := "http://" + srv.Addr()

	// A small pacing delay (10ms) prevents the server's 64-slot observer
	// buffer from overflowing. With PacingDelayMS: 0, the server generates
	// ~900 snapshots in ~100ms, faster than the WebSocket writer can drain
	// the buffer, causing sendNonBlocking to drop snapshots (including
	// game_over). 10ms still completes in ~9s but gives the writer time
	// to keep up. This is a test-specific accommodation, not a prod config.
	delay := 10
	cfg := client.Config{
		Game: "hearts",
		Seats: []client.SeatConfig{
			{Type: "ai", AIType: "random"},
			{Type: "ai", AIType: "random"},
			{Type: "ai", AIType: "random"},
			{Type: "ai", AIType: "random"},
		},
		PacingDelayMS: &delay,
	}

	sc := &client.SessionClient{BaseURL: baseURL}
	id, _, err := sc.CreateSession(ctx, cfg)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := sc.StartSession(ctx, id); err != nil {
		t.Fatalf("start session: %v", err)
	}

	// /ws/observe is the observer endpoint; it does not require
	// authentication. /ws is the player endpoint and returns 401
	// without a valid Bearer token.
	wsURL := "ws://" + srv.Addr() + "/sessions/" + id + "/ws/observe"
	conn := &client.Conn{}
	if err := conn.Connect(ctx, wsURL, ""); err != nil {
		t.Fatalf("connect observer websocket: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// With PacingDelayMS: 0, the server generates snapshots faster than a
	// synchronous read loop can consume them. The server's subscriber
	// channel has a 64-slot buffer; once full, sendNonBlocking drops
	// snapshots and the observer may miss game_over or see stale seqs.
	// A goroutine reader with a buffered channel decouples consumption
	// from the server's broadcast rate. A single result channel (instead
	// of separate data/error channels) preserves ordering so the test loop
	// always sees snapshots in the order they were read from the wire.
	//
	// bufSize is 256 (4x the server's 64-slot buffer) so the goroutine
	// can absorb bursts without stalling even if the test loop is briefly
	// blocked on assertions.
	const bufSize = 256
	type result struct {
		data json.RawMessage
		err  error
	}
	resCh := make(chan result, bufSize)
	go func() {
		for {
			data, err := conn.ReadSnapshot(ctx)
			resCh <- result{data: data, err: err}
			if err != nil {
				return
			}
		}
	}()

	phases := make(map[string]bool)
	var (
		lastSeq int
		gotOver bool
	)
	for range 1000 {
		res := <-resCh
		if res.err != nil {
			// We do not check gotOver here because it is set on the
			// same iteration that breaks the loop — gotOver can never
			// be true while the loop is still running. Any error
			// received before the loop exits is a real failure.
			t.Fatalf("read snapshot: %v", res.err)
		}

		var snap struct {
			Type  string `json:"type"`
			Seq   int    `json:"seq"`
			Phase string `json:"phase"`
		}
		if err := json.Unmarshal(res.data, &snap); err != nil {
			t.Fatalf("unmarshal snapshot: %v", err)
		}
		if snap.Seq <= lastSeq {
			t.Fatalf("seq not monotonic: got %d, last %d", snap.Seq, lastSeq)
		}
		lastSeq = snap.Seq
		phases[snap.Phase] = true
		if snap.Phase == "game_over" {
			gotOver = true
			break
		}
	}

	if !gotOver {
		t.Error("never saw game_over phase")
	}
	for _, required := range []string{"playing", "trick_complete", "round_complete", "game_over"} {
		if !phases[required] {
			t.Errorf("did not observe required phase %q, got phases: %v", required, phases)
		}
	}
}

// TestIntegrationPlayerAndObserver runs a full game with a human player
// and a concurrent observer, verifying that both see game_over and that
// the observer witnesses all required phases.
func TestIntegrationPlayerAndObserver(t *testing.T) {
	t.Parallel()
	// 120s timeout accommodates the race detector; see
	// TestIntegrationFullLifecycle for the rationale.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	srv := setupTestServer(t)
	baseURL := "http://" + srv.Addr()

	// 10ms pacing is sufficient when the main goroutine continuously
	// drains the observer; the goroutine never stalls on a full channel.
	delay := 10
	cfg := client.Config{
		Game: "hearts",
		Seats: []client.SeatConfig{
			{Type: "human"},
			{Type: "ai", AIType: "random"},
			{Type: "ai", AIType: "random"},
			{Type: "ai", AIType: "random"},
		},
		PacingDelayMS: &delay,
	}

	sc := &client.SessionClient{BaseURL: baseURL}
	id, seats, err := sc.CreateSession(ctx, cfg)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	var token string
	for _, s := range seats {
		if s.Type == "human" {
			token = s.Token
			break
		}
	}
	if token == "" {
		t.Fatal("no human seat token found")
	}

	if err := sc.StartSession(ctx, id); err != nil {
		t.Fatalf("start session: %v", err)
	}

	// Connect player and observer.
	playerWS := "ws://" + srv.Addr() + "/sessions/" + id + "/ws"
	playerConn := &client.Conn{}
	if err := playerConn.Connect(ctx, playerWS, token); err != nil {
		t.Fatalf("connect player websocket: %v", err)
	}
	defer func() { _ = playerConn.Close() }()

	obsWS := "ws://" + srv.Addr() + "/sessions/" + id + "/ws/observe"
	obsConn := &client.Conn{}
	if err := obsConn.Connect(ctx, obsWS, ""); err != nil {
		t.Fatalf("connect observer websocket: %v", err)
	}
	defer func() { _ = obsConn.Close() }()

	// Start an observer reader goroutine that forwards snapshots to obsCh.
	// The main goroutine must drain obsCh in a tight loop and do nothing
	// else. If the main goroutine instead ran the player loop, it would
	// block on ReadSnapshot for the entire ~100ms game duration whenever
	// it was not the human's turn, leaving obsCh completely undrained.
	// A full game produces ~900 snapshots; obsCh has capacity 256, so
	// it would fill and the observer goroutine would stall on send. That
	// stops the WebSocket read, the server's observer subCh fills, and
	// sendNonBlocking drops snapshots (including game_over). This
	// arrangement was learned through two failed structures: a
	// mutex+slice race, and a reversed goroutine assignment (player in
	// main, observer in goroutine) that produced "subscriber channel
	// full, snapshot dropped" warnings and caused the test to deadlock
	// or time out.
	type result struct {
		data json.RawMessage
		err  error
	}
	obsCh := make(chan result, 256)
	go func() {
		for {
			data, err := obsConn.ReadSnapshot(ctx)
			obsCh <- result{data: data, err: err}
			if err != nil {
				return
			}
		}
	}()

	// Player goroutine: reads snapshots and sends commands. Errors are
	// sent back to the main goroutine so it owns all assertions.
	//
	// playerErrCh has capacity 1 so the goroutine can send its final
	// error and exit without blocking, even if the main goroutine is
	// still draining the observer. A zero-capacity channel would stall
	// the goroutine until the main goroutine is ready to receive.
	playerDone := make(chan struct{})
	playerErrCh := make(chan error, 1)
	var (
		playerGotPass bool
		playerGotPlay bool
		playerGotOver bool
	)
	go func() {
		defer close(playerDone)
		var playerLastSeq int
		for range 1000 {
			data, err := playerConn.ReadSnapshot(ctx)
			if err != nil {
				playerErrCh <- fmt.Errorf("player read snapshot: %w", err)
				return
			}

			var snap struct {
				Type  string `json:"type"`
				Seq   int    `json:"seq"`
				Phase string `json:"phase"`
				Turn  int    `json:"turn"`
			}
			if err := json.Unmarshal(data, &snap); err != nil {
				playerErrCh <- fmt.Errorf("player unmarshal snapshot: %w", err)
				return
			}

			if snap.Seq <= playerLastSeq {
				playerErrCh <- fmt.Errorf(
					"player seq not monotonic: got %d, last %d",
					snap.Seq, playerLastSeq,
				)
				return
			}
			playerLastSeq = snap.Seq

			if snap.Phase == "game_over" {
				playerGotOver = true
				return
			}
			if snap.Phase == "passing" && snap.Turn == 0 {
				playerGotPass = true
				var handSnap struct {
					Hand []heartsclient.Card `json:"hand"`
				}
				if err := json.Unmarshal(data, &handSnap); err != nil {
					playerErrCh <- fmt.Errorf("player unmarshal hand: %w", err)
					return
				}
				if len(handSnap.Hand) < 3 {
					playerErrCh <- fmt.Errorf(
						"player hand too small to pass: got %d, want >= 3",
						len(handSnap.Hand),
					)
					return
				}
				actionID := fmt.Sprintf("pass-%d", snap.Seq)
				cmd, err := heartsclient.NewPassCardsMessage(actionID, snap.Seq, handSnap.Hand[:3])
				if err != nil {
					playerErrCh <- fmt.Errorf("player build pass command: %w", err)
					return
				}
				if err := playerConn.SendCommand(ctx, cmd); err != nil {
					playerErrCh <- fmt.Errorf("player send pass command: %w", err)
					return
				}
				continue
			}
			if snap.Phase == "playing" && snap.Turn == 0 {
				playerGotPlay = true
				var actionSnap struct {
					LegalActions []heartsclient.Card `json:"legal_actions"`
				}
				if err := json.Unmarshal(data, &actionSnap); err != nil {
					playerErrCh <- fmt.Errorf("player unmarshal legal actions: %w", err)
					return
				}
				if len(actionSnap.LegalActions) == 0 {
					playerErrCh <- fmt.Errorf("player no legal actions but it's our turn")
					return
				}
				actionID := fmt.Sprintf("play-%d", snap.Seq)
				cmd, err := heartsclient.NewPlayCardMessage(
					actionID, snap.Seq, actionSnap.LegalActions[0],
				)
				if err != nil {
					playerErrCh <- fmt.Errorf("player build play command: %w", err)
					return
				}
				if err := playerConn.SendCommand(ctx, cmd); err != nil {
					playerErrCh <- fmt.Errorf("player send play command: %w", err)
					return
				}
				continue
			}
		}
		playerErrCh <- fmt.Errorf("player loop exceeded 1000 iterations without game_over")
	}()

	// Main goroutine: drain observer until game_over.
	obsPhases := make(map[string]bool)
	var (
		obsLastSeq int
		obsGotOver bool
	)
	for range 2000 {
		res := <-obsCh
		if res.err != nil {
			// obsGotOver is set on the same iteration that calls
			// break — there is no subsequent iteration where it could
			// be true while res.err != nil.
			t.Fatalf("observer read snapshot: %v", res.err)
		}

		var snap struct {
			Type  string `json:"type"`
			Seq   int    `json:"seq"`
			Phase string `json:"phase"`
		}
		if err := json.Unmarshal(res.data, &snap); err != nil {
			t.Fatalf("observer unmarshal snapshot: %v", err)
		}
		if snap.Seq <= obsLastSeq {
			t.Fatalf("observer seq not monotonic: got %d, last %d", snap.Seq, obsLastSeq)
		}
		obsLastSeq = snap.Seq
		obsPhases[snap.Phase] = true
		if snap.Phase == "game_over" {
			obsGotOver = true
			break
		}
	}

	// Wait for player goroutine to finish. The goroutine closes
	// playerDone on exit, so this select blocks until it returns.
	select {
	case <-playerDone:
	case <-ctx.Done():
		t.Fatal("player goroutine did not finish before test timeout")
	}
	// playerErrCh has capacity 1, so the goroutine may have sent an
	// error before closing playerDone. We use select+default to avoid
	// blocking if the goroutine exited cleanly without sending an error.
	select {
	case perr := <-playerErrCh:
		if perr != nil {
			t.Fatal(perr)
		}
	default:
	}

	if !playerGotPass {
		t.Error("player never saw passing phase with our turn")
	}
	if !playerGotPlay {
		t.Error("player never saw playing phase with our turn")
	}
	if !playerGotOver {
		t.Error("player never saw game_over phase")
	}
	if !obsGotOver {
		t.Error("observer never saw game_over phase")
	}
	for _, required := range []string{
		"playing", "trick_complete", "round_complete", "game_over",
	} {
		if !obsPhases[required] {
			t.Errorf(
				"observer did not observe required phase %q, got phases: %v",
				required, obsPhases,
			)
		}
	}
}

// TestIntegrationErrorResponse verifies that sending a command in the
// wrong phase returns an ErrorMessage and leaves the connection open.
func TestIntegrationErrorResponse(t *testing.T) {
	t.Parallel()
	// 120s timeout accommodates the race detector; see
	// TestIntegrationFullLifecycle for the rationale.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	srv := setupTestServer(t)
	baseURL := "http://" + srv.Addr()

	zero := 0
	cfg := client.Config{
		Game: "hearts",
		Seats: []client.SeatConfig{
			{Type: "human"},
			{Type: "ai", AIType: "random"},
			{Type: "ai", AIType: "random"},
			{Type: "ai", AIType: "random"},
		},
		PacingDelayMS: &zero,
	}

	sc := &client.SessionClient{BaseURL: baseURL}
	id, seats, err := sc.CreateSession(ctx, cfg)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	var token string
	for _, s := range seats {
		if s.Type == "human" {
			token = s.Token
			break
		}
	}
	if token == "" {
		t.Fatal("no human seat token found")
	}

	if err := sc.StartSession(ctx, id); err != nil {
		t.Fatalf("start session: %v", err)
	}

	wsURL := "ws://" + srv.Addr() + "/sessions/" + id + "/ws"
	conn := &client.Conn{}
	if err := conn.Connect(ctx, wsURL, token); err != nil {
		t.Fatalf("connect websocket: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Read the initial snapshot. Hearts always starts in passing phase.
	data, err := conn.ReadSnapshot(ctx)
	if err != nil {
		t.Fatalf("read initial snapshot: %v", err)
	}
	var initial struct {
		Phase string `json:"phase"`
		Turn  int    `json:"turn"`
	}
	if err := json.Unmarshal(data, &initial); err != nil {
		t.Fatalf("unmarshal initial snapshot: %v", err)
	}
	if initial.Phase != "passing" {
		t.Fatalf("expected initial phase %q, got %q", "passing", initial.Phase)
	}

	// Send a play_card command during passing phase. This must fail with
	// ErrWrongPhase because the command type does not match the current phase.
	// seq=0 is deliberate: the server rejects wrong-phase commands before
	// checking seq staleness, so the client seq doesn't matter.
	badCard := heartsclient.Card{Rank: "two", Suit: "clubs"}
	badCmd, err := heartsclient.NewPlayCardMessage("bad-play-1", 0, badCard)
	if err != nil {
		t.Fatalf("build bad command: %v", err)
	}
	if err := conn.SendCommand(ctx, badCmd); err != nil {
		t.Fatalf("send bad command: %v", err)
	}

	// The server responds with an error message on the WebSocket.
	_, err = conn.ReadSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error response for bad command, got nil")
	}
	var em *client.ErrorMessage
	if !errors.As(err, &em) {
		t.Fatalf("expected *ErrorMessage, got %T: %v", err, err)
	}
	if em.ErrorCode != client.ErrWrongPhase {
		t.Errorf("got error code %q, want %q", em.ErrorCode, client.ErrWrongPhase)
	}

	// Receiving an ErrorMessage (not ConnectionClosedError) proves the
	// server kept the WebSocket open after the rejected command.
	// We call Close explicitly here (rather than relying on the defer)
	// to make the test intent explicit: the connection must still be
	// functional, and a clean close confirms it.
	if cerr := conn.Close(); cerr != nil {
		t.Fatalf("close connection after error: %v", cerr)
	}
}

// setupTestServer creates a real server with a Hearts game factory,
// starts it on an ephemeral port, and registers cleanup.
//
// We use a real transport.Server (not httptest) because integration
// tests must exercise the full HTTP/WebSocket stack including the
// upgrade handshake, per ADR-004's strict transport boundary.
func setupTestServer(t *testing.T) *transport.Server {
	t.Helper()
	factory := heartsFactory(t)
	mgr := session.NewManager(factory)
	srv := transport.NewServer(transport.Config{Manager: mgr})
	go func() {
		_ = srv.Start() // Server exited (usually on Shutdown).
	}()
	for i := 0; i < 100 && srv.Addr() == ""; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.Addr() == "" {
		t.Fatal("server did not start listening")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return srv
}

// heartsFactory returns a session game factory that creates real Hearts
// adapters with a deterministic RNG seeded from the test name.
//
// A deterministic seed makes failures reproducible across runs. The
// seed is derived from the test name so different tests get different
// game sequences while the same test always produces the same sequence.
func heartsFactory(t *testing.T) func(session.Config) (session.Game, error) {
	t.Helper()
	seed := hashTestName(t.Name())
	rng := rand.New(rand.NewPCG(seed, seed+1))
	return func(cfg session.Config) (session.Game, error) {
		return heartssession.NewAdapter(cfg.Seats, rng)
	}
}

// hashTestName converts a test name string into a deterministic uint64
// seed for the RNG.
func hashTestName(name string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return h.Sum64()
}
