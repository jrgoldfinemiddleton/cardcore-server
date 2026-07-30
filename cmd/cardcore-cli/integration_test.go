package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	heartscli "github.com/jrgoldfinemiddleton/cardcore-server/cmd/cardcore-cli/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
	transporttestutil "github.com/jrgoldfinemiddleton/cardcore-server/internal/server/transport/testutil"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/testutil"
)

// TestIntegrationCreateSessionSmoke verifies that the CLI can create a
// Hearts session via the HTTP API without playing a full game.
func TestIntegrationCreateSessionSmoke(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	srv := transporttestutil.SetupTestServer(t)
	baseURL := "http://" + srv.Addr()

	cfg := testutil.HeartsConfigWithPacing(10)

	sc := &client.SessionClient{BaseURL: baseURL}
	id, seats, err := sc.CreateSession(ctx, cfg)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if id == "" {
		t.Fatal("create session returned empty id")
	}
	if len(seats) != 4 {
		t.Fatalf("create session returned %d seats, want 4", len(seats))
	}
	token := testutil.HumanToken(t, seats)
	if token == "" {
		t.Fatal("human seat returned empty token")
	}
}

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

	srv := transporttestutil.SetupTestServer(t)
	baseURL := "http://" + srv.Addr()

	// 10ms pacing keeps the sequential read loop from falling behind
	// the server's broadcast rate. With AIActionDelayMS: 0, the server
	// generates ~900 snapshots in ~100ms, faster than the player loop
	// can consume them. Under parallel load the 64-slot subscriber
	// buffer overflows and sendNonBlocking drops snapshots (including
	// game_over), causing flaky failures. 10ms is the same value used
	// by the transport-level full-game tests.
	cfg := testutil.HeartsConfigWithPacing(10)

	sc := &client.SessionClient{BaseURL: baseURL}
	id, seats, err := sc.CreateSession(ctx, cfg)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	token := testutil.HumanToken(t, seats)

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
		testutil.LogSnapshot(t, "player", res.data)

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
			testutil.LogCommand(t, "player", cmd)
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
			var tc struct {
				Phase       string `json:"phase"`
				TrickWinner int    `json:"trick_winner"`
				Trick       []struct {
					Seat int `json:"seat"`
					Card struct {
						Rank string `json:"rank"`
						Suit string `json:"suit"`
					} `json:"card"`
				} `json:"trick"`
			}
			if err := json.Unmarshal(res.data, &tc); err != nil {
				t.Fatalf("unmarshal trick_complete snapshot: %v", err)
			}
			if tc.TrickWinner < 0 {
				t.Errorf("trick_complete snapshot has trick_winner=%d, want >= 0", tc.TrickWinner)
			}
			if len(tc.Trick) != 4 {
				t.Errorf("trick_complete snapshot has %d trick entries, want 4", len(tc.Trick))
			}
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

	srv := transporttestutil.SetupTestServer(t)
	baseURL := "http://" + srv.Addr()

	// 5ms pacing prevents the observer channel from overflowing with
	// heuristic AI players. All-AI sessions with AIActionDelayMS: 0 generate
	// snapshots faster than the WebSocket writer can drain the 64-slot
	// subscriber buffer, causing sendNonBlocking to drop snapshots
	// (including game_over) and making the test flaky.
	cfg := testutil.HeartsAllAIConfigWithPacing(5)

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
		testutil.LogSnapshot(t, "observer", res.data)

		line := formatter.FormatSnapshot(res.data)
		if line == "" {
			t.Fatal("FormatSnapshot returned empty string")
		}

		var snap struct {
			Seq         int    `json:"seq"`
			Phase       string `json:"phase"`
			TrickWinner int    `json:"trick_winner"`
			Trick       []struct {
				Seat int `json:"seat"`
				Card struct {
					Rank string `json:"rank"`
					Suit string `json:"suit"`
				} `json:"card"`
			} `json:"trick"`
		}
		if err := json.Unmarshal(res.data, &snap); err != nil {
			t.Fatalf("unmarshal snapshot: %v", err)
		}
		if snap.Seq <= lastSeq {
			t.Fatalf("seq not monotonic: got %d, last %d", snap.Seq, lastSeq)
		}
		lastSeq = snap.Seq
		phases[snap.Phase] = true

		if snap.Phase == "trick_complete" {
			if snap.TrickWinner < 0 {
				t.Errorf("trick_complete snapshot has trick_winner=%d, want >= 0", snap.TrickWinner)
			}
			if len(snap.Trick) != 4 {
				t.Errorf("trick_complete snapshot has %d trick entries, want 4", len(snap.Trick))
			}
		}

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

// TestIntegrationRunObserverWithRealServer verifies that the CLI's run() path
// can observe an all-AI session from start to finish using a real server.
func TestIntegrationRunObserverWithRealServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	srv := transporttestutil.SetupTestServer(t)

	cfg := &cliConfig{
		observe:   true,
		addr:      "http://" + srv.Addr(),
		game:      "hearts",
		aiType:    "random",
		pacing:    10,
		exitDelay: 0,
	}
	if err := run(cfg); err != nil {
		t.Fatalf("run() got error %v, want nil", err)
	}
}
