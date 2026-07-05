package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"sync"
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

	// Verify the initial snapshot has seq == 1.
	if snaps[0].Seq != 1 {
		t.Fatalf("initial snapshot seq: got %d, want 1", snaps[0].Seq)
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

// TestHumanTurnTimeoutIntegration verifies that when a human player does
// not act within the turn timeout, the session auto-plays an AI move and
// the game advances.
func TestHumanTurnTimeoutIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	timeout := 50
	delay := 0
	cfg := session.Config{
		Game: "hearts",
		Seats: []session.SeatConfig{
			{Type: session.SeatHuman},
			{Type: session.SeatAI, AIType: "random"},
			{Type: session.SeatAI, AIType: "random"},
			{Type: session.SeatAI, AIType: "random"},
		},
		AIActionDelayMS: &delay,
		TurnTimeoutMS:   &timeout,
	}

	info, seats, err := mgr.Create(cfg)
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

	// Connect human player but do not send any commands.
	playerConn := mustDialPlayerWS(t, httpSrv.URL, id, token)
	obsConn := mustDialObserverWS(t, httpSrv.URL, id)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Read initial snapshot.
	snap := mustReadSnapshot(t, playerConn, ctx)
	initialSeq, ok := snap["seq"].(float64)
	if !ok {
		t.Fatalf("snapshot missing seq: %v", snap)
	}

	// Wait longer than the timeout for the AI fallback to fire.
	time.Sleep(150 * time.Millisecond)

	// Read the post-timeout snapshot.
	snap = mustReadSnapshot(t, playerConn, ctx)
	timeoutSeq, ok := snap["seq"].(float64)
	if !ok {
		t.Fatalf("post-timeout snapshot missing seq: %v", snap)
	}

	if int(timeoutSeq) <= int(initialSeq) {
		t.Fatalf(
			"seq did not advance after timeout: got %d, want > %d",
			int(timeoutSeq), int(initialSeq),
		)
	}

	// Clean up: close connections and delete session.
	_ = playerConn.Close(websocket.StatusNormalClosure, "")
	_ = obsConn.Close(websocket.StatusNormalClosure, "")
	_ = mgr.Delete(id)
}

// TestPassPhaseTimeoutIntegration verifies that when a human player does
// not send pass_cards during the pass phase, the turn timeout fires and
// the AI fallback selects 3 cards to pass.
func TestPassPhaseTimeoutIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	timeout := 100
	delay := 0
	cfg := session.Config{
		Game: "hearts",
		Seats: []session.SeatConfig{
			{Type: session.SeatHuman},
			{Type: session.SeatAI, AIType: "random"},
			{Type: session.SeatAI, AIType: "random"},
			{Type: session.SeatAI, AIType: "random"},
		},
		AIActionDelayMS: &delay,
		TurnTimeoutMS:   &timeout,
	}

	info, seats, err := mgr.Create(cfg)
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	snap := mustReadSnapshot(t, playerConn, ctx)
	phase, _ := snap["phase"].(string)
	if phase != "passing" {
		t.Fatalf("expected passing phase, got %q", phase)
	}
	initialSeq, ok := snap["seq"].(float64)
	if !ok {
		t.Fatalf("snapshot missing seq: %v", snap)
	}

	time.Sleep(200 * time.Millisecond)

	snap = mustReadSnapshot(t, playerConn, ctx)
	passSeq, ok := snap["seq"].(float64)
	if !ok {
		t.Fatalf("post-timeout snapshot missing seq: %v", snap)
	}

	if int(passSeq) <= int(initialSeq) {
		t.Fatalf(
			"seq did not advance after pass timeout: got %d, want > %d",
			int(passSeq), int(initialSeq),
		)
	}

	_ = playerConn.Close(websocket.StatusNormalClosure, "")
	_ = obsConn.Close(websocket.StatusNormalClosure, "")
	_ = mgr.Delete(id)
}

// TestFourHumansFullGameIntegration verifies that a session with 4 human
// seats completes correctly. Each human client independently detects
// their turn and sends commands.
func TestFourHumansFullGameIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	info, seats, err := mgr.Create(fourHumanHeartsConfig())
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID

	tokens := make([]string, 4)
	for i, s := range seats {
		if s.Type != session.SeatHuman {
			t.Fatalf("seat %d is not human", i)
		}
		tokens[i] = s.Token
	}

	if err := mgr.Start(id); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, 4)
	var cancelOnce sync.Once

	for seat := range 4 {
		wg.Go(func() {
			conn := mustDialPlayerWS(t, httpSrv.URL, id, tokens[seat])
			defer func() {
				_ = conn.Close(websocket.StatusNormalClosure, "")
			}()

			actionCount := 0
			maxSeq := -1

			for {
				snap, err := readSnapshot(t, conn, ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					errCh <- fmt.Errorf("read snapshot: %w", err)
					return
				}

				if msgType, ok := snap["type"].(string); ok && msgType == "error" {
					errCh <- fmt.Errorf(
						"seat %d received error: %v", seat, snap,
					)
					return
				}

				seq, ok := snap["seq"].(float64)
				if !ok {
					errCh <- fmt.Errorf(
						"snapshot missing seq: %v", snap,
					)
					return
				}
				if int(seq) <= maxSeq {
					errCh <- fmt.Errorf(
						"seq not strictly monotonic: got %d after max %d",
						int(seq), maxSeq,
					)
					return
				}
				maxSeq = int(seq)

				phase, _ := snap["phase"].(string)
				if phase == "game_over" {
					cancelOnce.Do(cancel)
					return
				}

				turn, _ := snap["turn"].(float64)
				if int(turn) == seat &&
					(phase == "passing" || phase == "playing") {
					legalActionsRaw, ok := snap["legal_actions"]
					if !ok {
						errCh <- fmt.Errorf(
							"snapshot missing legal_actions: %v", snap,
						)
						return
					}
					legalActions := extractCards(t, legalActionsRaw)
					if len(legalActions) == 0 {
						errCh <- fmt.Errorf(
							"no legal actions for seat %d", seat,
						)
						return
					}

					actionID := fmt.Sprintf(
						"seat%d-action-%c", seat, 'a'+actionCount,
					)
					actionCount++

					switch phase {
					case "passing":
						if len(legalActions) < 3 {
							errCh <- fmt.Errorf(
								"expected at least 3 legal actions, got %d",
								len(legalActions),
							)
							return
						}
						sendPassCards(
							t, conn, actionID, maxSeq, legalActions[:3],
						)
					case "playing":
						sendPlayCard(
							t, conn, actionID, maxSeq, legalActions[0],
						)
					}
				}
			}
		})
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("client goroutine error: %v", err)
		}
	}
}

// fourHumanHeartsConfig returns a 4-seat all-human Hearts config with
// zero pacing delay.
func fourHumanHeartsConfig() session.Config {
	delay := 0
	return session.Config{
		Game: "hearts",
		Seats: []session.SeatConfig{
			{Type: session.SeatHuman},
			{Type: session.SeatHuman},
			{Type: session.SeatHuman},
			{Type: session.SeatHuman},
		},
		AIActionDelayMS: &delay,
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
		return heartssession.NewAdapter(cfg.Seats, rng, 0, 0, 0)
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
		AIActionDelayMS: &delay,
	}
}

// setupHeartsServer creates a Server with a real Hearts game factory.
func setupHeartsServer(t *testing.T) (*Server, *session.Manager) {
	t.Helper()
	factory := heartsFactory(t)
	mgr := session.NewManager(factory, session.DefaultServerDelays)
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
		AIActionDelayMS: &delay,
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

// readSnapshot reads a single WebSocket message and unmarshals it as a
// snapshot. Unlike mustReadSnapshot it returns an error so callers can
// decide whether the error is fatal. Used by the 4-human goroutine so it
// can return quietly when the shared context is cancelled after another
// seat sees game_over.
func readSnapshot(t *testing.T, conn *websocket.Conn, ctx context.Context) (map[string]any, error) {
	t.Helper()
	typ, b, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	if typ != websocket.MessageText {
		return nil, fmt.Errorf("got message type %d, want text", typ)
	}
	var snap map[string]any
	if err := json.Unmarshal(b, &snap); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return snap, nil
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
