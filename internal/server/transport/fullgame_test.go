package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jrgoldfinemiddleton/cardcore/games/hearts"

	heartsapi "github.com/jrgoldfinemiddleton/cardcore-server/internal/api/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/testutil"
)

// TestAllAIObserverSeesFourCardTrick verifies that when a trick completes,
// the observer sees a trick_complete snapshot with all four cards and then
// the next playing snapshot with a fresh one-card trick.
func TestAllAIObserverSeesFourCardTrick(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	// Slow the AI down so the observer can connect after the session starts
	// but before the first trick completes.
	cfg := testutil.HeartsAllAISessionConfigWithPacing(200)

	info, _, err := mgr.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID

	if err := mgr.Start(id); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	obsConn := mustDialObserverWS(t, httpSrv.URL, id)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var (
		gotTrickCompleteFour bool
		gotNextPlaying       bool
	)

	for range 5000 {
		snap := mustReadSnapshot(t, obsConn, ctx)
		phase, _ := snap["phase"].(string)
		trickLen := trickLengthFromSnap(t, snap)

		switch phase {
		case "playing":
			if gotTrickCompleteFour && trickLen == 1 {
				gotNextPlaying = true
			}
		case "trick_complete":
			if trickLen == hearts.NumPlayers {
				gotTrickCompleteFour = true
			}
		}

		if gotNextPlaying {
			break
		}
		if phase == "game_over" {
			break
		}
	}

	if !gotTrickCompleteFour {
		t.Error("never saw trick_complete snapshot with four-card trick")
	}
	if !gotNextPlaying {
		t.Error("never saw next playing snapshot after trick resolution")
	}
}

// TestAllAIFullGameIntegration verifies that a 4-AI Hearts game completes
// via WebSocket and an observer receives all snapshots showing phase
// progression.
func TestAllAIFullGameIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	info, _, err := mgr.Create(testutil.HeartsAllAISessionConfigWithPacing(10))
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

	info, seats, err := mgr.Create(testutil.HeartsSessionConfigWithPacing(10))
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID

	token := testutil.HumanSessionToken(t, seats)

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
			snap, err := readTestSnapshot(t, ctx, obsConn)
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
	cfg := testutil.HeartsSessionConfigWithPacing(0)
	cfg.TurnTimeoutMS = &timeout

	info, seats, err := mgr.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID

	token := testutil.HumanSessionToken(t, seats)

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
	cfg := testutil.HeartsSessionConfigWithPacing(0)
	cfg.TurnTimeoutMS = &timeout

	info, seats, err := mgr.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID

	token := testutil.HumanSessionToken(t, seats)

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

	info, seats, err := mgr.Create(testutil.HeartsFourHumanSessionConfig())
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

// TestObserverSnapshotParityIntegration verifies that the observer
// stream is a consistent view of the same game the seated player sees:
// observer snapshots form a strictly increasing subsequence of the
// player's (delivery is lossy under burst, so drops are tolerated),
// snapshots sharing a seq agree on every shared field, and the streams
// diverge exactly where seat filtering requires it — the player sees
// only their own hand, while the observer sees all four hands and the
// trick history.
func TestObserverSnapshotParityIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	info, seats, err := mgr.Create(testutil.HeartsSessionConfigWithPacing(10))
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID
	token := testutil.HumanSessionToken(t, seats)

	if err := mgr.Start(id); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	playerConn := mustDialPlayerWS(t, httpSrv.URL, id, token)
	obsConn := mustDialObserverWS(t, httpSrv.URL, id)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// A goroutine drains the observer stream until game_over; the main
	// goroutine drives the player, keeping both streams drained.
	var obsSnaps []map[string]any
	obsDone := make(chan struct{})
	go func() {
		defer close(obsDone)
		for {
			snap, err := readSnapshot(t, obsConn, ctx)
			if err != nil {
				return
			}
			obsSnaps = append(obsSnaps, snap)
			if phase, _ := snap["phase"].(string); phase == "game_over" {
				return
			}
		}
	}()

	var playerSnaps []map[string]any
	actionCount := 0
	maxSeq := -1

	var snap map[string]any
	for {
		if snap == nil {
			snap = mustReadSnapshot(t, playerConn, ctx)
		}
		playerSnaps = append(playerSnaps, snap)

		seq := snapSeq(t, snap)
		if seq <= maxSeq {
			t.Fatalf("seq not strictly monotonic: got %d after max %d", seq, maxSeq)
		}
		maxSeq = seq

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

			actionID := fmt.Sprintf("parity-action-%d", actionCount)
			actionCount++

			switch phase {
			case "passing":
				if len(legalActions) < 3 {
					t.Fatalf("expected at least 3 legal actions, got %d", len(legalActions))
				}
				sendPassCards(t, playerConn, actionID, maxSeq, legalActions[:3])
			case "playing":
				sendPlayCard(t, playerConn, actionID, maxSeq, legalActions[0])
			}

			respSnap := mustReadSnapshot(t, playerConn, ctx)
			playerSnaps = append(playerSnaps, respSnap)
			respSeq := snapSeq(t, respSnap)
			if respSeq <= maxSeq {
				t.Fatalf("response seq not monotonic: got %d after max %d", respSeq, maxSeq)
			}
			maxSeq = respSeq

			if msgType, ok := respSnap["type"].(string); ok && msgType == "error" {
				t.Fatalf("received error response for command: %v", respSnap)
			}
		}
		snap = mustReadSnapshot(t, playerConn, ctx)
	}

	<-obsDone

	if len(obsSnaps) == 0 {
		t.Fatal("observer received no snapshots")
	}

	// Both streams must terminate at the same game_over snapshot. An
	// observer that stops earlier means the connection closed or the
	// terminal snapshot was dropped — a failure, not an EOF.
	lastObs := obsSnaps[len(obsSnaps)-1]
	if phase, _ := lastObs["phase"].(string); phase != "game_over" {
		t.Fatalf("observer stream ended at phase %q, want game_over", phase)
	}
	lastPlayer := playerSnaps[len(playerSnaps)-1]
	if got, want := snapSeq(t, lastObs), snapSeq(t, lastPlayer); got != want {
		t.Fatalf("terminal seq: observer got %d, player got %d, want equal", got, want)
	}

	// The observer stream must be strictly increasing and a subsequence
	// of the player stream.
	playerBySeq := make(map[int]map[string]any, len(playerSnaps))
	for _, ps := range playerSnaps {
		playerBySeq[snapSeq(t, ps)] = ps
	}

	sharedFields := []string{
		"type", "phase", "round_number", "trick_number", "pass_direction",
		"turn", "trick_winner", "hearts_broken", "hand_counts", "trick",
		"scores", "round_points", "turn_deadline_ms", "paused",
	}

	maxObs := -1
	for _, os := range obsSnaps {
		seq := snapSeq(t, os)
		if seq <= maxObs {
			t.Fatalf("observer seq not strictly monotonic: got %d after max %d", seq, maxObs)
		}
		maxObs = seq

		ps, ok := playerBySeq[seq]
		if !ok {
			t.Errorf("observer saw seq %d that the player never saw", seq)
			continue
		}
		for _, field := range sharedFields {
			if !reflect.DeepEqual(os[field], ps[field]) {
				t.Errorf("seq %d field %q: observer got %v, player got %v",
					seq, field, os[field], ps[field])
			}
		}
	}

	// Seat filtering must diverge exactly at the hidden-information
	// boundary: hands for the observer, own hand for the player.
	for _, ps := range playerSnaps {
		if _, ok := ps["hand"]; !ok {
			t.Errorf("player snapshot seq %d missing hand", snapSeq(t, ps))
		}
		if _, ok := ps["hands"]; ok {
			t.Errorf("player snapshot seq %d unexpectedly contains hands", snapSeq(t, ps))
		}
		if _, ok := ps["trick_history"]; ok {
			t.Errorf("player snapshot seq %d unexpectedly contains trick_history", snapSeq(t, ps))
		}
	}
	for _, os := range obsSnaps {
		hands, ok := os["hands"].([]any)
		if !ok || len(hands) != hearts.NumPlayers {
			t.Errorf("observer snapshot seq %d: got hands %v, want %d-element array",
				snapSeq(t, os), os["hands"], hearts.NumPlayers)
		}
		if _, ok := os["hand"]; ok {
			t.Errorf("observer snapshot seq %d unexpectedly contains hand", snapSeq(t, os))
		}
	}
}

// trickLengthFromSnap returns the number of entries in the snapshot's trick
// array. It returns 0 if the field is missing or not an array.
func trickLengthFromSnap(t *testing.T, snap map[string]any) int {
	t.Helper()
	raw, ok := snap["trick"].([]any)
	if !ok {
		return 0
	}
	return len(raw)
}

// setupHeartsServer creates a Server with a real Hearts game registry.
func setupHeartsServer(t *testing.T) (*Server, *session.Manager) {
	t.Helper()
	registry := testutil.HeartsRegistry(t)
	mgr := session.NewManager(registry, session.DefaultServerDelays)
	cfg := Config{Manager: mgr}
	srv := NewServer(cfg)
	return srv, mgr
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

// extractCards extracts a slice of [heartsapi.Card] from the untyped JSON
// value produced by json.Unmarshal for the legal_actions field.
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

// snapSeq extracts the seq field from an untyped snapshot.
func snapSeq(t *testing.T, snap map[string]any) int {
	t.Helper()
	seq, ok := snap["seq"].(float64)
	if !ok {
		t.Fatalf("snapshot missing seq: %v", snap)
	}
	return int(seq)
}
