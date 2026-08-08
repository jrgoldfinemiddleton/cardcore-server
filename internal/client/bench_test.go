package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/transport"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/testutil"
)

// benchResult carries one Conn.ReadSnapshot result from the reader
// goroutine to the benchmark loop. A single channel (instead of separate
// data/error channels) preserves wire order.
type benchResult struct {
	// data is the raw JSON snapshot message.
	data json.RawMessage
	// err is the read error, if any.
	err error
}

// benchSnapshot is the subset of a Hearts player snapshot the benchmark
// needs to decide on and build its next command.
type benchSnapshot struct {
	// Seq is the monotonic state change counter.
	Seq int `json:"seq"`
	// Phase is the current game phase (e.g., "passing", "playing").
	Phase string `json:"phase"`
	// Turn is the seat index of the player who must act next.
	Turn int `json:"turn"`
	// Hand is the receiving player's current hand, sorted.
	Hand []heartsclient.Card `json:"hand"`
	// LegalActions is the cards the player may legally play or pass.
	LegalActions []heartsclient.Card `json:"legal_actions"`
}

// benchGame bundles one live session: the human seat's WebSocket
// connection, the buffered stream of snapshots read from it, and the
// human seat index.
type benchGame struct {
	// conn is the human seat's WebSocket connection.
	conn *client.Conn
	// resCh receives every snapshot read from conn by the reader goroutine.
	resCh chan benchResult
	// humanSeat is the index of the human seat in this session.
	humanSeat int
}

// BenchmarkSessionCommandRoundTrip measures the latency of one accepted
// game command round-trip as a client experiences it over the real
// transport (ADR-004): a real server on an ephemeral port, session setup
// over HTTP, and play over a real WebSocket through the shared client
// engine (SessionClient and Conn).
//
// One "op" is a single accepted command round-trip, defined as:
//
//  1. Wait (untimed) for a snapshot where it is the human seat's turn.
//  2. Build (untimed) a valid command from that snapshot's
//     server-authoritative data — pass_cards from the current hand during
//     the passing phase, play_card from legal_actions during the playing
//     phase — carrying the snapshot's seq and a unique action_id
//     (doc/integration-testing.md §9).
//  3. Send the command with Conn.SendCommand (timed).
//  4. Read until the first snapshot whose seq advanced beyond the seq the
//     command was sent against is observed (timed). Per ADR-011,
//     Conn.ReadSnapshot discards stale seqs, and the game is blocked on
//     the human seat when the command is sent, so no unrelated snapshot
//     can intervene: the first fresh read is exactly the post-command
//     state.
//
// Reported ns/op therefore covers the WebSocket write, server-side
// validation and state transition on the session goroutine, snapshot
// generation, and the broadcast read back to the client. Time spent in AI
// turns while waiting for the human to be prompted, and session
// re-creation after game_over, is excluded with b.StopTimer so only
// command round-trips are timed.
//
// The session is a 1-human + 3-AI Hearts game with zero AI pacing and a
// deterministic RNG seeded from the benchmark name. Whenever the game
// reaches game_over, a fresh session is created. The benchmark does not
// call b.ReportAllocs: across the WebSocket boundary, allocation counts
// are noise next to the transport latency signal (the alloc columns in
// make bench output come from its global -benchmem flag).
func BenchmarkSessionCommandRoundTrip(b *testing.B) {
	// Hang guard only: a full make bench run of this benchmark finishes in
	// seconds; the deadline stays generous to absorb -race slowdowns.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	srv := setupBenchServer(b)
	game := newBenchGame(b, ctx, srv)
	defer func() { _ = game.conn.Close() }()

	for b.Loop() {
		// Waiting for the human seat's turn (AI play) and session
		// re-creation are set up, not measurement: keep the timer stopped
		// for everything except the command round-trip itself. The timer
		// is always running again before the next b.Loop call, which
		// fails when called with the timer stopped.
		b.StopTimer()
		snap, gameOver := waitForHumanTurn(b, game)
		for gameOver {
			_ = game.conn.Close()
			game = newBenchGame(b, ctx, srv)
			snap, gameOver = waitForHumanTurn(b, game)
		}
		cmd := buildBenchCommand(b, snap)

		b.StartTimer()
		if err := game.conn.SendCommand(ctx, cmd); err != nil {
			b.Fatalf("send command: %v", err)
		}
		awaitPostCommand(b, game, snap.Seq)
	}
}

// setupBenchServer starts a real transport server on an ephemeral port
// with a deterministic Hearts registry, and registers cleanup. It is the
// bench-local equivalent of transporttestutil.SetupTestServer: that helper
// takes *testing.T because testutil.HeartsRegistry requires *testing.T,
// so generalizing it to testing.TB does not compile without touching
// non-test code.
func setupBenchServer(b *testing.B) *transport.Server {
	b.Helper()
	seed := testutil.HashTestName(b.Name())
	rng := rand.New(rand.NewPCG(seed, seed+1))
	registry := session.NewRegistry()
	registry.Register(&testutil.TestHeartsConfig{Rng: rng})
	mgr := session.NewManager(registry, session.DefaultServerDelays)
	srv := transport.NewServer(transport.Config{Manager: mgr})
	go func() {
		_ = srv.Start()
	}()
	for i := 0; i < 100 && srv.Addr() == ""; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.Addr() == "" {
		b.Fatal("server did not start listening")
	}
	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return srv
}

// newBenchGame creates and starts a fresh 1-human + 3-AI Hearts session
// with zero AI pacing, connects the human seat's WebSocket, and starts a
// reader goroutine that continuously drains the broadcast stream into a
// buffered channel. The buffer (256, 4x the server's 64-slot subscriber
// buffer) keeps the server's sendNonBlocking from ever dropping snapshots
// while the benchmark loop is busy.
func newBenchGame(b *testing.B, ctx context.Context, srv *transport.Server) benchGame {
	b.Helper()
	// Discard per-connection logs: closing a finished game's WebSocket logs
	// a WARN per session re-creation, which would flood the bench output.
	logger := slog.New(slog.DiscardHandler)
	sc := &client.SessionClient{BaseURL: "http://" + srv.Addr(), Logger: logger}
	id, seats, err := sc.CreateSession(ctx, testutil.HeartsConfigWithPacing(0))
	if err != nil {
		b.Fatalf("create session: %v", err)
	}

	humanSeat := -1
	token := ""
	for i, s := range seats {
		if s.Type == "human" {
			humanSeat = i
			token = s.Token
			break
		}
	}
	if token == "" {
		b.Fatal("no human seat token found")
	}

	if err := sc.StartSession(ctx, id); err != nil {
		b.Fatalf("start session: %v", err)
	}

	conn := &client.Conn{Logger: logger}
	wsURL := "ws://" + srv.Addr() + "/sessions/" + id + "/ws"
	if err := conn.Connect(ctx, wsURL, token); err != nil {
		b.Fatalf("connect websocket: %v", err)
	}

	const bufSize = 256
	resCh := make(chan benchResult, bufSize)
	go func() {
		for {
			data, err := conn.ReadSnapshot(ctx)
			resCh <- benchResult{data: data, err: err}
			if err != nil {
				return
			}
		}
	}()

	return benchGame{conn: conn, resCh: resCh, humanSeat: humanSeat}
}

// waitForHumanTurn drains the snapshot stream until the human seat must
// act, returning the snapshot to respond to. It reports gameOver=true when
// the game reaches the game_over phase. A server rejection surfaces here
// as a read error and fails the benchmark, so the rejection path can never
// be measured silently.
func waitForHumanTurn(b *testing.B, g benchGame) (benchSnapshot, bool) {
	b.Helper()
	for {
		res := <-g.resCh
		if res.err != nil {
			b.Fatalf("read snapshot: %v", res.err)
		}
		var snap benchSnapshot
		if err := json.Unmarshal(res.data, &snap); err != nil {
			b.Fatalf("unmarshal snapshot: %v", err)
		}
		if snap.Phase == heartsclient.PhaseGameOver {
			return snap, true
		}
		if snap.Turn != g.humanSeat {
			continue
		}
		if snap.Phase == heartsclient.PhasePassing || snap.Phase == heartsclient.PhasePlaying {
			return snap, false
		}
	}
}

// buildBenchCommand builds a valid command for an our-turn snapshot, using
// only server-authoritative data: pass_cards with the first three cards of
// the current hand during passing, play_card with the first legal action
// during playing. The action_id is unique within the session because seq
// is strictly increasing.
func buildBenchCommand(b *testing.B, snap benchSnapshot) client.Command {
	b.Helper()
	actionID := fmt.Sprintf("bench-%d", snap.Seq)
	switch snap.Phase {
	case heartsclient.PhasePassing:
		if len(snap.Hand) < 3 {
			b.Fatalf("hand too small to pass: got %d, want >= 3", len(snap.Hand))
		}
		cmd, err := heartsclient.NewPassCardsMessage(actionID, snap.Seq, snap.Hand[:3])
		if err != nil {
			b.Fatalf("build pass command: %v", err)
		}
		return cmd
	case heartsclient.PhasePlaying:
		if len(snap.LegalActions) == 0 {
			b.Fatal("no legal actions but it's our turn")
		}
		cmd, err := heartsclient.NewPlayCardMessage(actionID, snap.Seq, snap.LegalActions[0])
		if err != nil {
			b.Fatalf("build play command: %v", err)
		}
		return cmd
	default:
		b.Fatalf("unexpected our-turn phase %q", snap.Phase)
		return client.Command{}
	}
}

// awaitPostCommand reads until the first snapshot with seq greater than
// sendSeq — the state produced by our accepted command, which closes the
// round-trip. The reader goroutine has already discarded stale-seq frames
// per ADR-011, so in practice the first result received returns
// immediately; the seq check keeps the closing condition explicit.
func awaitPostCommand(b *testing.B, g benchGame, sendSeq int) {
	b.Helper()
	for {
		res := <-g.resCh
		if res.err != nil {
			b.Fatalf("read post-command snapshot: %v", res.err)
		}
		var snap benchSnapshot
		if err := json.Unmarshal(res.data, &snap); err != nil {
			b.Fatalf("unmarshal post-command snapshot: %v", err)
		}
		if snap.Seq > sendSeq {
			return
		}
	}
}
