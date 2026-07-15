package heartstui

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
	heartssession "github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/transport"
)

// TestIntegrationTUIClientFullGame connects a TUI client to a real server and
// plays a full 1-human+3-AI Hearts game, verifying that the Client renders all
// phases correctly and produces valid commands.
func TestIntegrationTUIClientFullGame(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	srv := setupTestServer(t)
	baseURL := "http://" + srv.Addr()

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

	c := NewClient(0, false, NewDarkTheme())

	var (
		gotPass             bool
		gotPlay             bool
		gotTrickComplete    bool
		gotRoundComplete    bool
		gotOver             bool
		verifiedTrickWinner bool
	)

outer:
	for range 5000 {
		res := <-resCh
		if res.err != nil {
			t.Fatalf("read snapshot: %v", res.err)
		}

		c.HandleSnapshot(res.data)

		rendered := c.Render()
		if rendered == "" {
			t.Fatal("Render() returned empty string")
		}

		switch c.phase {
		case heartsclient.PhasePassing:
			gotPass = true
			if c.playerSnap.Turn == 0 {
				cmd, send, _ := c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
				if !send {
					// Need to select 3 cards first.
					for i := 0; i < 3; i++ {
						c.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace})
						c.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
					}
					cmd, send, _ = c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
					if !send {
						t.Fatal("failed to send pass command after selecting 3 cards")
					}
				}
				if err := conn.SendCommand(ctx, cmd); err != nil {
					t.Fatalf("send pass command: %v", err)
				}
			}
		case heartsclient.PhasePlaying:
			gotPlay = true
			if c.playerSnap.Turn == 0 {
				// Select the first legal card directly instead of brute-forcing.
				for i, card := range c.playerSnap.Hand {
					if slices.Contains(c.playerSnap.LegalActions, card) {
						c.cursor = i
						break
					}
				}
				cmd, send, _ := c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
				if send {
					if err := conn.SendCommand(ctx, cmd); err != nil {
						t.Fatalf("send play command: %v", err)
					}
				}
			}
		case heartsclient.PhaseTrickComplete:
			gotTrickComplete = true
			if !verifiedTrickWinner {
				verifiedTrickWinner = true
				if len(c.playerSnap.Trick) != 4 {
					t.Errorf("trick_complete trick entries: got %d, want 4",
						len(c.playerSnap.Trick))
				}
				if c.playerSnap.TrickWinner < 0 {
					t.Errorf("trick_complete TrickWinner: got %d, want >= 0",
						c.playerSnap.TrickWinner)
				}
				want := fmt.Sprintf("Seat %d won", c.playerSnap.TrickWinner)
				if !strings.Contains(rendered, want) {
					t.Errorf("trick_complete render: got %q, want %q", rendered, want)
				}
			}
		case heartsclient.PhaseRoundComplete:
			gotRoundComplete = true
		case heartsclient.PhaseGameOver:
			gotOver = true
			break outer
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

// TestIntegrationTUIClientAutoCreateSession verifies that the Hearts session
// helper creates and starts a session, and that the returned token connects to
// the first snapshot.
func TestIntegrationTUIClientAutoCreateSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	srv := setupTestServer(t)
	baseURL := "http://" + srv.Addr()

	delay := 10
	sc := &client.SessionClient{BaseURL: baseURL}
	id, token, seat, err := CreateSession(
		ctx, sc, "random", false, &delay, &delay,
	)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if id == "" {
		t.Fatal("CreateSession returned empty session ID")
	}
	if token == "" {
		t.Fatal("CreateSession returned empty token")
	}
	if seat != 0 {
		t.Errorf("seat got %d, want 0", seat)
	}

	wsURL := "ws://" + srv.Addr() + "/sessions/" + id + "/ws"
	conn := &client.Conn{}
	if err := conn.Connect(ctx, wsURL, token); err != nil {
		t.Fatalf("connect websocket: %v", err)
	}
	defer func() { _ = conn.Close() }()

	data, err := conn.ReadSnapshot(ctx)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("first snapshot was empty")
	}

	c := NewClient(0, false, NewDarkTheme())
	c.HandleSnapshot(data)
	if c.phase == "" {
		t.Error("snapshot did not produce a phase")
	}
}

// TestTUITimeoutAutoPlayIntegration verifies the server auto-plays a human
// turn when the client does not act within the configured turn timeout.
func TestTUITimeoutAutoPlayIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	srv := setupTestServer(t)
	baseURL := "http://" + srv.Addr()

	timeout := 500
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
		TurnTimeoutMS:   &timeout,
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

	c := NewClient(0, false, NewDarkTheme())
	gotTimeoutAutoPlay := false

	for range 5000 {
		data, err := conn.ReadSnapshot(ctx)
		if err != nil {
			t.Fatalf("read snapshot: %v", err)
		}

		c.HandleSnapshot(data)

		if c.phase == heartsclient.PhasePassing && c.playerSnap.Turn == 0 {
			// Wait for timeout without sending a command.
			gotTimeoutAutoPlay = true
		}
		if c.phase == heartsclient.PhasePlaying && c.playerSnap.Turn == 0 {
			gotTimeoutAutoPlay = true
		}
		if c.phase == heartsclient.PhaseGameOver {
			break
		}
	}

	if !gotTimeoutAutoPlay {
		t.Error("never saw a human turn that could time out")
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
