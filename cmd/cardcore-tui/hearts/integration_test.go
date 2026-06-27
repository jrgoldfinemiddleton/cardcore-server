package heartstui

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"charm.land/bubbletea/v2"

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

	c := NewClient(0, false)

	var (
		gotPass          bool
		gotPlay          bool
		gotTrickComplete bool
		gotRoundComplete bool
		gotOver          bool
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

// setupTestServer creates a real server with a Hearts game factory,
// starts it on an ephemeral port, and registers cleanup.
func setupTestServer(t *testing.T) *transport.Server {
	t.Helper()
	factory := heartsFactory(t)
	mgr := session.NewManager(factory)
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
