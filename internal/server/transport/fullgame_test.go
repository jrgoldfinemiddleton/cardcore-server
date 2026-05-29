package transport

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
	heartsapi "github.com/jrgoldfinemiddleton/cardcore-server/internal/api/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
	heartssession "github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session/games/hearts"
)

// TestAllAIFullGameIntegration verifies that a 4-AI Hearts game completes
// via WebSocket and an observer receives all snapshots showing phase
// progression.
func TestAllAIFullGameIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	info, _, err := mgr.Create(allAIHeartsConfig())
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID
	if err := mgr.Start(id); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	obsConn := mustDialObserverWS(t, httpSrv.URL, id)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Read snapshots until the game ends.
	snaps := readSnapshotsUntil(t, obsConn, ctx, "game_over")

	if len(snaps) == 0 {
		t.Fatal("received no snapshots")
	}

	// Verify seq is strictly monotonically increasing.
	// The session goroutine is single-threaded; all snapshots for a
	// single subscriber flow through one FIFO channel. This test only
	// exercises the broadcast path (no stale_seq / duplicate action_id
	// multiplexing), so out-of-order delivery is impossible here.
	maxSeq := -1
	for _, snap := range snaps {
		if snap.Seq <= maxSeq {
			t.Fatalf("seq not strictly monotonic: got %d after max %d", snap.Seq, maxSeq)
		}
		maxSeq = snap.Seq
	}

	// Verify phase progression: must see playing, trick_complete, round_complete, game_over.
	phases := make(map[string]bool)
	for _, snap := range snaps {
		phases[snap.Phase] = true
	}
	for _, required := range []string{"playing", "trick_complete", "round_complete", "game_over"} {
		if !phases[required] {
			t.Fatalf("did not observe required phase %q, got phases: %v", required, phases)
		}
	}
}

// TestHumanAIFullGameIntegration verifies that a human player can send
// valid commands through WebSocket and the game completes correctly with
// 1 human + 3 AI seats.
func TestHumanAIFullGameIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	info, seats, err := mgr.Create(humanAIHeartsConfig())
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID

	var token string
	for _, s := range seats {
		if s.Type == session.SeatHuman {
			token = s.Token
			break
		}
	}
	if token == "" {
		t.Fatal("no human seat token found")
	}

	if err := mgr.Start(id); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	playerConn := mustDialPlayerWS(t, httpSrv.URL, id, token)
	obsConn := mustDialObserverWS(t, httpSrv.URL, id)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	actionCount := 0
	maxSeq := -1

	// Start a goroutine to drain all observer snapshots.
	var obsSnaps []testSnapshot
	var obsDone = make(chan struct{})
	go func() {
		defer close(obsDone)
		for {
			snap, err := readTestSnapshot(ctx, obsConn)
			if err != nil {
				return
			}
			obsSnaps = append(obsSnaps, snap)
			if snap.Phase == "game_over" {
				return
			}
		}
	}()

	// Process player snapshots starting with the initial subscription snapshot.
	var snap map[string]any
	for {
		if snap == nil {
			snap = mustReadSnapshot(t, playerConn, ctx)
		}
		seq, ok := snap["seq"].(float64)
		if !ok {
			t.Fatalf("snapshot missing seq: %v", snap)
		}
		if int(seq) <= maxSeq {
			t.Fatalf("seq not strictly monotonic: got %d after max %d", int(seq), maxSeq)
		}
		maxSeq = int(seq)

		phase, _ := snap["phase"].(string)
		if phase == "game_over" {
			break
		}

		turn, _ := snap["turn"].(float64)
		if int(turn) == 0 && (phase == "passing" || phase == "playing") {
			legalActionsRaw, ok := snap["legal_actions"]
			if !ok {
				t.Fatalf("snapshot missing legal_actions when human turn: %v", snap)
			}
			legalActions := extractCards(t, legalActionsRaw)
			if len(legalActions) == 0 {
				t.Fatalf("no legal actions available for human player")
			}

			actionID := "human-action-" + string(rune('a'+actionCount))
			actionCount++

			switch phase {
			case "passing":
				if len(legalActions) < 3 {
					t.Fatalf(
						"expected at least 3 legal actions for passing, got %d",
						len(legalActions),
					)
				}
				sendPassCards(t, playerConn, actionID, maxSeq, legalActions[:3])
			case "playing":
				sendPlayCard(t, playerConn, actionID, maxSeq, legalActions[0])
			}

			respSnap := mustReadSnapshot(t, playerConn, ctx)
			respSeq, ok := respSnap["seq"].(float64)
			if !ok {
				t.Fatalf("response snapshot missing seq: %v", respSnap)
			}
			if int(respSeq) <= maxSeq {
				t.Fatalf(
					"response seq not strictly monotonic: got %d after max %d",
					int(respSeq), maxSeq,
				)
			}
			maxSeq = int(respSeq)

			if msgType, ok := respSnap["type"].(string); ok && msgType == "error" {
				t.Fatalf("received error response for command: %v", respSnap)
			}
		}
		snap = mustReadSnapshot(t, playerConn, ctx)
	}

	// Wait for the observer goroutine to finish.
	<-obsDone
	if len(obsSnaps) == 0 {
		t.Fatal("observer received no snapshots")
	}
}

// hashTestName returns a deterministic uint64 seed derived from the test name.
func hashTestName(name string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return h.Sum64()
}

// heartsFactory returns a session factory that creates real Hearts game
// adapters using a deterministic RNG seeded from t.Name().
func heartsFactory(t *testing.T) func(session.Config) (session.Game, error) {
	t.Helper()
	seed := hashTestName(t.Name())
	rng := rand.New(rand.NewPCG(seed, seed+1))
	return func(cfg session.Config) (session.Game, error) {
		return heartssession.NewAdapter(cfg.Seats, rng)
	}
}

// allAIHeartsConfig returns a 4-seat all-AI Hearts config with a small
// pacing delay so the observer can connect before the game completes.
func allAIHeartsConfig() session.Config {
	delay := 10
	return session.Config{
		Game: "hearts",
		Seats: []session.SeatConfig{
			{Type: session.SeatAI, AIType: "random"},
			{Type: session.SeatAI, AIType: "random"},
			{Type: session.SeatAI, AIType: "random"},
			{Type: session.SeatAI, AIType: "random"},
		},
		PacingDelayMS: &delay,
	}
}

// setupHeartsServer creates a Server with a real Hearts game factory.
func setupHeartsServer(t *testing.T) (*Server, *session.Manager) {
	t.Helper()
	factory := heartsFactory(t)
	mgr := session.NewManager(factory)
	cfg := Config{Manager: mgr}
	srv := NewServer(cfg)
	return srv, mgr
}

// humanAIHeartsConfig returns a 1-human + 3-AI Hearts config with a small
// pacing delay so the human player can react before the game completes.
// The human is at seat 0.
func humanAIHeartsConfig() session.Config {
	delay := 10
	return session.Config{
		Game: "hearts",
		Seats: []session.SeatConfig{
			{Type: session.SeatHuman},
			{Type: session.SeatAI, AIType: "random"},
			{Type: session.SeatAI, AIType: "random"},
			{Type: session.SeatAI, AIType: "random"},
		},
		PacingDelayMS: &delay,
	}
}

// sendPlayCard sends a play_card command over the player WebSocket.
func sendPlayCard(t *testing.T, conn *websocket.Conn, actionID string, seq int,
	card heartsapi.Card) {
	t.Helper()
	payload, err := json.Marshal(heartsapi.PlayCardPayload{Card: card})
	if err != nil {
		t.Fatalf("marshal play_card payload: %v", err)
	}
	msg := api.InboundMessage{
		Type:     "play_card",
		ActionID: actionID,
		Seq:      seq,
		Payload:  payload,
	}
	if err := writeWSJSON(context.Background(), conn, msg); err != nil {
		t.Fatalf("write play_card: %v", err)
	}
}

// sendPassCards sends a pass_cards command over the player WebSocket.
func sendPassCards(t *testing.T, conn *websocket.Conn, actionID string, seq int,
	cards []heartsapi.Card) {
	t.Helper()
	payload, err := json.Marshal(heartsapi.PassCardsPayload{Cards: cards})
	if err != nil {
		t.Fatalf("marshal pass_cards payload: %v", err)
	}
	msg := api.InboundMessage{
		Type:     "pass_cards",
		ActionID: actionID,
		Seq:      seq,
		Payload:  payload,
	}
	if err := writeWSJSON(context.Background(), conn, msg); err != nil {
		t.Fatalf("write pass_cards: %v", err)
	}
}

// extractCards extracts a slice of heartsapi.Card from a []any of map[string]any
// (as returned by json.Unmarshal into map[string]any for legal_actions).
func extractCards(t *testing.T, raw any) []heartsapi.Card {
	t.Helper()
	arr, ok := raw.([]any)
	if !ok {
		t.Fatalf("legal_actions is not an array, got %T", raw)
	}
	cards := make([]heartsapi.Card, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("legal_actions item is not a map, got %T", item)
		}
		rank, _ := m["rank"].(string)
		suit, _ := m["suit"].(string)
		cards = append(cards, heartsapi.Card{Rank: rank, Suit: suit})
	}
	return cards
}
