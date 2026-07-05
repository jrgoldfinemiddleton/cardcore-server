package main

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"math/rand/v2"
	"testing"
	"time"

	heartscli "github.com/jrgoldfinemiddleton/cardcore-server/cmd/cardcore-cli/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
	heartssession "github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/transport"
)

// TestIntegrationScriptFullGame connects a scripted CLI player to a real
// server and plays a full 1-human+3-AI Hearts game, verifying compact
// notation output and script-driven command construction.
func TestIntegrationScriptFullGame(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	srv := setupTestServer(t)
	baseURL := "http://" + srv.Addr()

	// 10ms pacing keeps the sequential read loop from falling behind
	// the server's broadcast rate. With AIActionDelayMS: 0, the server
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
		AIActionDelayMS: &delay,
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

	script := Script{
		"passing": {
			Phase:        "passing",
			Action:       "pass_cards",
			Selector:     "first_n",
			SelectorArgs: []byte(`{"count": 3}`),
		},
		"playing": {
			Phase:    "playing",
			Action:   "play_card",
			Selector: "first_legal",
		},
	}

	executor := NewScriptExecutor(script, 0, heartscli.NewBuilder())
	formatter := heartscli.NewFormatter()

	var (
		gotPass          bool
		gotPlay          bool
		gotTrickComplete bool
		gotRoundComplete bool
		gotOver          bool
	)

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

outer:
	for range 5000 {
		res := <-resCh
		if res.err != nil {
			t.Fatalf("read snapshot: %v", res.err)
		}

		line := formatter.FormatSnapshot(res.data)
		if line == "" {
			t.Fatal("FormatSnapshot returned empty string")
		}

		cmd, done, err := executor.Step(res.data)
		if err != nil {
			t.Fatalf("script step: %v", err)
		}
		if done {
			gotOver = true
			break outer
		}
		if cmd.Type != "" {
			if err := conn.SendCommand(ctx, cmd); err != nil {
				t.Fatalf("send command: %v", err)
			}
		}

		var env struct {
			Phase string `json:"phase"`
		}
		if err := json.Unmarshal(res.data, &env); err != nil {
			t.Fatalf("unmarshal phase: %v", err)
		}

		switch env.Phase {
		case "passing":
			gotPass = true
		case "playing":
			gotPlay = true
		case "trick_complete":
			gotTrickComplete = true
		case "round_complete":
			gotRoundComplete = true
		}
	}

	if !gotPass {
		t.Error("never saw passing phase")
	}
	if !gotPlay {
		t.Error("never saw playing phase")
	}
	if !gotTrickComplete {
		t.Error("never saw trick_complete phase")
	}
	if !gotRoundComplete {
		t.Error("never saw round_complete phase")
	}
	if !gotOver {
		t.Error("never saw game_over phase")
	}
}

// TestIntegrationObserverFullGame connects as an observer to an all-AI
// session with minimal pacing, verifying compact notation output for
// observer snapshots and that all required phases are observed.
func TestIntegrationObserverFullGame(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	srv := setupTestServer(t)
	baseURL := "http://" + srv.Addr()

	// 5ms pacing prevents the observer channel from overflowing with
	// heuristic AI players. All-AI sessions with AIActionDelayMS: 0 generate
	// snapshots faster than the WebSocket writer can drain the 64-slot
	// subscriber buffer, causing sendNonBlocking to drop snapshots
	// (including game_over) and making the test flaky.
	delay := 5
	cfg := client.Config{
		Game: "hearts",
		Seats: []client.SeatConfig{
			{Type: "ai", AIType: "random"},
			{Type: "ai", AIType: "random"},
			{Type: "ai", AIType: "random"},
			{Type: "ai", AIType: "random"},
		},
		AIActionDelayMS: &delay,
	}

	sc := &client.SessionClient{BaseURL: baseURL}
	id, _, err := sc.CreateSession(ctx, cfg)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := sc.StartSession(ctx, id); err != nil {
		t.Fatalf("start session: %v", err)
	}

	wsURL := "ws://" + srv.Addr() + "/sessions/" + id + "/ws/observe"
	conn := &client.Conn{}
	if err := conn.Connect(ctx, wsURL, ""); err != nil {
		t.Fatalf("connect observer websocket: %v", err)
	}
	defer func() { _ = conn.Close() }()

	formatter := heartscli.NewFormatter()

	phases := make(map[string]bool)
	var (
		lastSeq int
		gotOver bool
	)

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

	for range 5000 {
		res := <-resCh
		if res.err != nil {
			t.Fatalf("read snapshot: %v", res.err)
		}

		line := formatter.FormatSnapshot(res.data)
		if line == "" {
			t.Fatal("FormatSnapshot returned empty string")
		}

		var snap struct {
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
		t.Logf("last seq=%d", lastSeq)
		t.Error("never saw game_over phase")
	}
	for _, required := range []string{"playing", "trick_complete", "round_complete", "game_over"} {
		if !phases[required] {
			t.Errorf("did not observe required phase %q, got phases: %v", required, phases)
		}
	}
}

// setupTestServer creates a real server with a Hearts game factory,
// starts it on an ephemeral port, and registers cleanup.
func setupTestServer(t *testing.T) *transport.Server {
	t.Helper()
	factory := heartsFactory(t)
	mgr := session.NewManager(factory, session.DefaultServerDelays)
	srv := transport.NewServer(transport.Config{Manager: mgr})
	go func() {
		_ = srv.Start()
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
func heartsFactory(t *testing.T) func(session.Config) (session.Game, error) {
	t.Helper()
	seed := hashTestName(t.Name())
	rng := rand.New(rand.NewPCG(seed, seed+1))
	return func(cfg session.Config) (session.Game, error) {
		return heartssession.NewAdapter(cfg.Seats, rng, 0, 0, 0)
	}
}

// hashTestName converts a test name string into a deterministic uint64
// seed for the RNG.
func hashTestName(name string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return h.Sum64()
}
