package transport

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
	heartsapi "github.com/jrgoldfinemiddleton/cardcore-server/internal/api/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// TestPauseResumeIntegration verifies that a single-human session can pause
// and resume, and that the turn deadline is recalculated on resume.
func TestPauseResumeIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	timeout := 5000
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Read the initial snapshot and wait for a human turn.
	var snap map[string]any
	for {
		snap = mustReadSnapshot(t, playerConn, ctx)
		phase, _ := snap["phase"].(string)
		turn, _ := snap["turn"].(float64)
		if (phase == "passing" || phase == "playing") && int(turn) == 0 {
			break
		}
		if phase == "game_over" {
			t.Fatal("game ended before human turn")
		}
	}

	seq, _ := snap["seq"].(float64)
	msg := api.InboundMessage{
		Type:     "pause",
		ActionID: "pause-1",
		Seq:      int(seq),
		Payload:  json.RawMessage("{}"),
	}
	if err := writeWSJSON(ctx, playerConn, msg); err != nil {
		t.Fatalf("write pause: %v", err)
	}

	// Verify the response snapshot is paused and the deadline is cleared.
	pauseSnap := mustReadSnapshot(t, playerConn, ctx)
	if pauseSnap["paused"] != true {
		t.Fatalf("expected paused=true after pause, got %v", pauseSnap["paused"])
	}
	if pauseSnap["turn_deadline_ms"] != float64(0) {
		t.Fatalf("expected turn_deadline_ms=0 after pause, got %v", pauseSnap["turn_deadline_ms"])
	}

	pauseSeq, _ := pauseSnap["seq"].(float64)
	resumeMsg := api.InboundMessage{
		Type:     "resume",
		ActionID: "resume-1",
		Seq:      int(pauseSeq),
		Payload:  json.RawMessage("{}"),
	}
	if err := writeWSJSON(ctx, playerConn, resumeMsg); err != nil {
		t.Fatalf("write resume: %v", err)
	}

	resumeSnap := mustReadSnapshot(t, playerConn, ctx)
	if resumeSnap["paused"] != false {
		t.Fatalf("expected paused=false after resume, got %v", resumeSnap["paused"])
	}
	if resumeSnap["turn_deadline_ms"] == float64(0) {
		t.Fatalf("expected turn_deadline_ms>0 after resume, got %v", resumeSnap["turn_deadline_ms"])
	}

	_ = playerConn.Close(websocket.StatusNormalClosure, "")
	_ = obsConn.Close(websocket.StatusNormalClosure, "")
	_ = mgr.Delete(id)
}

// TestPauseMultiHumanRejectedIntegration verifies that pause is rejected in a
// multi-human session.
func TestPauseMultiHumanRejectedIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	delay := 0
	cfg := session.Config{
		Game: "hearts",
		Seats: []session.SeatConfig{
			{Type: session.SeatHuman},
			{Type: session.SeatHuman},
			{Type: session.SeatAI, AIType: "random"},
			{Type: session.SeatAI, AIType: "random"},
		},
		AIActionDelayMS: &delay,
	}

	info, seats, err := mgr.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID

	var token string
	for _, s := range seats {
		if s.Type == session.SeatHuman && token == "" {
			token = s.Token
		}
	}
	if token == "" {
		t.Fatal("no human seat token found")
	}

	if err := mgr.Start(id); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	playerConn := mustDialPlayerWS(t, httpSrv.URL, id, token)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var snap map[string]any
	for {
		snap = mustReadSnapshot(t, playerConn, ctx)
		phase, _ := snap["phase"].(string)
		if phase == "passing" || phase == "playing" {
			break
		}
		if phase == "game_over" {
			t.Fatal("game ended before human turn")
		}
	}

	seq, _ := snap["seq"].(float64)
	msg := api.InboundMessage{
		Type:     "pause",
		ActionID: "pause-1",
		Seq:      int(seq),
		Payload:  json.RawMessage("{}"),
	}
	if err := writeWSJSON(ctx, playerConn, msg); err != nil {
		t.Fatalf("write pause: %v", err)
	}

	em := mustReadError(t, playerConn, ctx)
	if em.ErrorCode != api.ErrPauseNotAllowed {
		t.Fatalf("error code: got %q, want %q", em.ErrorCode, api.ErrPauseNotAllowed)
	}

	_ = playerConn.Close(websocket.StatusNormalClosure, "")
	_ = mgr.Delete(id)
}

// TestPauseAlreadyPausedRejectedIntegration verifies that a second pause
// command is rejected when the game is already paused.
func TestPauseAlreadyPausedRejectedIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	timeout := 5000
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var snap map[string]any
	for {
		snap = mustReadSnapshot(t, playerConn, ctx)
		phase, _ := snap["phase"].(string)
		turn, _ := snap["turn"].(float64)
		if (phase == "passing" || phase == "playing") && int(turn) == 0 {
			break
		}
		if phase == "game_over" {
			t.Fatal("game ended before human turn")
		}
	}

	seq, _ := snap["seq"].(float64)
	pauseMsg := api.InboundMessage{
		Type:     "pause",
		ActionID: "pause-1",
		Seq:      int(seq),
		Payload:  json.RawMessage("{}"),
	}
	if err := writeWSJSON(ctx, playerConn, pauseMsg); err != nil {
		t.Fatalf("write pause: %v", err)
	}
	_ = mustReadSnapshot(t, playerConn, ctx)

	pauseSeq := int(seq) + 1
	secondPauseMsg := api.InboundMessage{
		Type:     "pause",
		ActionID: "pause-2",
		Seq:      pauseSeq,
		Payload:  json.RawMessage("{}"),
	}
	if err := writeWSJSON(ctx, playerConn, secondPauseMsg); err != nil {
		t.Fatalf("write second pause: %v", err)
	}

	em := mustReadError(t, playerConn, ctx)
	if em.ErrorCode != api.ErrPauseNotAllowed {
		t.Fatalf("error code: got %q, want %q", em.ErrorCode, api.ErrPauseNotAllowed)
	}

	_ = playerConn.Close(websocket.StatusNormalClosure, "")
	_ = mgr.Delete(id)
}

// TestResumeNotPausedRejectedIntegration verifies that resume is rejected
// when the game is not paused.
func TestResumeNotPausedRejectedIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	timeout := 5000
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var snap map[string]any
	for {
		snap = mustReadSnapshot(t, playerConn, ctx)
		phase, _ := snap["phase"].(string)
		turn, _ := snap["turn"].(float64)
		if (phase == "passing" || phase == "playing") && int(turn) == 0 {
			break
		}
		if phase == "game_over" {
			t.Fatal("game ended before human turn")
		}
	}

	seq, _ := snap["seq"].(float64)
	msg := api.InboundMessage{
		Type:     "resume",
		ActionID: "resume-1",
		Seq:      int(seq),
		Payload:  json.RawMessage("{}"),
	}
	if err := writeWSJSON(ctx, playerConn, msg); err != nil {
		t.Fatalf("write resume: %v", err)
	}

	em := mustReadError(t, playerConn, ctx)
	if em.ErrorCode != api.ErrPauseNotAllowed {
		t.Fatalf("error code: got %q, want %q", em.ErrorCode, api.ErrPauseNotAllowed)
	}

	_ = playerConn.Close(websocket.StatusNormalClosure, "")
	_ = mgr.Delete(id)
}

// TestDisconnectWhilePausedAutoUnpauseIntegration verifies that a paused game
// auto-unpauses when the human disconnects, the turn timeout fires, and the AI
// plays.
func TestDisconnectWhilePausedAutoUnpauseIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	timeout := 200
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var snap map[string]any
	for {
		snap = mustReadSnapshot(t, playerConn, ctx)
		phase, _ := snap["phase"].(string)
		turn, _ := snap["turn"].(float64)
		if (phase == "passing" || phase == "playing") && int(turn) == 0 {
			break
		}
		if phase == "game_over" {
			t.Fatal("game ended before human turn")
		}
	}

	seq, _ := snap["seq"].(float64)
	pauseMsg := api.InboundMessage{
		Type:     "pause",
		ActionID: "pause-1",
		Seq:      int(seq),
		Payload:  json.RawMessage("{}"),
	}
	if err := writeWSJSON(ctx, playerConn, pauseMsg); err != nil {
		t.Fatalf("write pause: %v", err)
	}
	pauseSnap := mustReadSnapshot(t, playerConn, ctx)
	if pauseSnap["paused"] != true {
		t.Fatalf("expected paused=true after pause, got %v", pauseSnap["paused"])
	}

	// Close the player connection while paused.
	_ = playerConn.Close(websocket.StatusNormalClosure, "")

	// Wait longer than the timeout for the AI fallback to fire.
	time.Sleep(400 * time.Millisecond)

	// The observer should eventually see a non-paused snapshot.
	var sawUnpaused bool
	for range 100 {
		snap := mustReadSnapshot(t, obsConn, ctx)
		if paused, ok := snap["paused"].(bool); ok && !paused {
			sawUnpaused = true
			break
		}
	}
	if !sawUnpaused {
		t.Fatal("observer never saw unpaused snapshot after disconnect")
	}

	_ = obsConn.Close(websocket.StatusNormalClosure, "")
	_ = mgr.Delete(id)
}

// TestObserverReceivesPausedSnapshotIntegration verifies that an observer
// connected to a single-human session receives paused snapshots.
func TestObserverReceivesPausedSnapshotIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	timeout := 5000
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var snap map[string]any
	for {
		snap = mustReadSnapshot(t, playerConn, ctx)
		phase, _ := snap["phase"].(string)
		turn, _ := snap["turn"].(float64)
		if (phase == "passing" || phase == "playing") && int(turn) == 0 {
			break
		}
		if phase == "game_over" {
			t.Fatal("game ended before human turn")
		}
	}

	seq, _ := snap["seq"].(float64)
	pauseMsg := api.InboundMessage{
		Type:     "pause",
		ActionID: "pause-1",
		Seq:      int(seq),
		Payload:  json.RawMessage("{}"),
	}
	if err := writeWSJSON(ctx, playerConn, pauseMsg); err != nil {
		t.Fatalf("write pause: %v", err)
	}

	_ = mustReadSnapshot(t, playerConn, ctx)

	var sawPaused bool
	for range 100 {
		snap := mustReadSnapshot(t, obsConn, ctx)
		if paused, ok := snap["paused"].(bool); ok && paused {
			sawPaused = true
			break
		}
	}
	if !sawPaused {
		t.Fatal("observer never saw paused snapshot")
	}

	_ = playerConn.Close(websocket.StatusNormalClosure, "")
	_ = obsConn.Close(websocket.StatusNormalClosure, "")
	_ = mgr.Delete(id)
}

// TestMultiHumanUnchangedIntegration verifies that turn timeout still fires
// and AI plays for a human seat in a multi-human session.
func TestMultiHumanUnchangedIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	timeout := 200
	delay := 0
	cfg := session.Config{
		Game: "hearts",
		Seats: []session.SeatConfig{
			{Type: session.SeatHuman},
			{Type: session.SeatHuman},
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
		if s.Type == session.SeatHuman && token == "" {
			token = s.Token
		}
	}
	if token == "" {
		t.Fatal("no human seat token found")
	}

	if err := mgr.Start(id); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	playerConn := mustDialPlayerWS(t, httpSrv.URL, id, token)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	snap := mustReadSnapshot(t, playerConn, ctx)
	initialSeq, ok := snap["seq"].(float64)
	if !ok {
		t.Fatalf("snapshot missing seq: %v", snap)
	}

	time.Sleep(400 * time.Millisecond)

	snap = mustReadSnapshot(t, playerConn, ctx)
	timeoutSeq, ok := snap["seq"].(float64)
	if !ok {
		t.Fatalf("post-timeout snapshot missing seq: %v", snap)
	}
	if int(timeoutSeq) <= int(initialSeq) {
		t.Fatalf("seq did not advance after timeout: got %d, want > %d",
			int(timeoutSeq), int(initialSeq))
	}

	_ = playerConn.Close(websocket.StatusNormalClosure, "")
	_ = mgr.Delete(id)
}

// TestPlayWhilePausedRejectedIntegration verifies the paused guard end to
// end: a gameplay command sent while paused is rejected with game_paused
// without advancing seq; a stale command during the same pause still gets
// stale_seq with a fresh snapshot (seq validation precedes the guard);
// resume restores the turn deadline (the rejected commands must not have
// cleared waitingForHuman); and the same gameplay command is accepted
// after resume.
func TestPlayWhilePausedRejectedIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	timeout := 5000
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Wait for a human turn and capture the phase and legal actions.
	var snap map[string]any
	var phase string
	for {
		snap = mustReadSnapshot(t, playerConn, ctx)
		phase, _ = snap["phase"].(string)
		turn, _ := snap["turn"].(float64)
		if (phase == "passing" || phase == "playing") && int(turn) == 0 {
			break
		}
		if phase == "game_over" {
			t.Fatal("game ended before human turn")
		}
	}
	legalActions := extractCards(t, snap["legal_actions"])
	seq, _ := snap["seq"].(float64)

	pauseMsg := api.InboundMessage{
		Type:     "pause",
		ActionID: "pause-1",
		Seq:      int(seq),
		Payload:  json.RawMessage("{}"),
	}
	if err := writeWSJSON(ctx, playerConn, pauseMsg); err != nil {
		t.Fatalf("write pause: %v", err)
	}
	pauseSnap := mustReadSnapshot(t, playerConn, ctx)
	if pauseSnap["paused"] != true {
		t.Fatalf("expected paused=true after pause, got %v", pauseSnap["paused"])
	}
	pauseSeq, _ := pauseSnap["seq"].(float64)

	// A gameplay command while paused is rejected with game_paused, and the
	// rejection does not advance seq.
	sendGameplayCommand(t, playerConn, phase, "paused-cmd-1", int(pauseSeq), legalActions)
	em := mustReadError(t, playerConn, ctx)
	if em.ErrorCode != api.ErrGamePaused {
		t.Fatalf("error code: got %q, want %q", em.ErrorCode, api.ErrGamePaused)
	}
	if em.CurrentSeq != int(pauseSeq) {
		t.Fatalf("current_seq: got %d, want %d (rejection must not advance seq)",
			em.CurrentSeq, int(pauseSeq))
	}

	// A stale gameplay command during the same pause still gets stale_seq:
	// the fresh resync snapshot arrives first, then the error.
	sendGameplayCommand(t, playerConn, phase, "paused-cmd-stale", int(pauseSeq)-1, legalActions)
	staleSnap := mustReadSnapshot(t, playerConn, ctx)
	if staleSnap["paused"] != true {
		t.Fatalf("expected paused=true resync snapshot, got %v", staleSnap["paused"])
	}
	em = mustReadError(t, playerConn, ctx)
	if em.ErrorCode != api.ErrStaleSeq {
		t.Fatalf("error code: got %q, want %q", em.ErrorCode, api.ErrStaleSeq)
	}

	// Resume: the turn deadline must be restored, proving the rejected
	// commands did not clear waitingForHuman.
	resumeMsg := api.InboundMessage{
		Type:     "resume",
		ActionID: "resume-1",
		Seq:      int(pauseSeq),
		Payload:  json.RawMessage("{}"),
	}
	if err := writeWSJSON(ctx, playerConn, resumeMsg); err != nil {
		t.Fatalf("write resume: %v", err)
	}
	resumeSnap := mustReadSnapshot(t, playerConn, ctx)
	if resumeSnap["paused"] != false {
		t.Fatalf("expected paused=false after resume, got %v", resumeSnap["paused"])
	}
	if resumeSnap["turn_deadline_ms"] == float64(0) {
		t.Fatalf("expected turn_deadline_ms>0 after resume, got %v", resumeSnap["turn_deadline_ms"])
	}
	resumeSeq, _ := resumeSnap["seq"].(float64)

	// The same gameplay command is accepted after resume.
	sendGameplayCommand(t, playerConn, phase, "post-resume-cmd", int(resumeSeq), legalActions)
	acceptedSnap := mustReadSnapshot(t, playerConn, ctx)
	if msgType, ok := acceptedSnap["type"].(string); ok && msgType == "error" {
		t.Fatalf("got error for post-resume command: %v", acceptedSnap)
	}
	acceptedSeq, _ := acceptedSnap["seq"].(float64)
	if int(acceptedSeq) <= int(resumeSeq) {
		t.Fatalf("seq did not advance after accepted command: got %d, want > %d",
			int(acceptedSeq), int(resumeSeq))
	}

	_ = playerConn.Close(websocket.StatusNormalClosure, "")
	_ = mgr.Delete(id)
}

// TestDuplicateActionWhilePausedIntegration verifies that replaying an
// already-accepted action_id while paused returns the cached snapshot
// silently (ADR-013: a duplicate action_id is not an error condition);
// action_id dedup precedes the paused guard.
func TestDuplicateActionWhilePausedIntegration(t *testing.T) {
	t.Parallel()
	srv, mgr := setupHeartsServer(t)
	httpSrv := mustStartTestServer(t, srv)

	timeout := 5000
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Wait for the human passing turn and pass three cards so their
	// action_id has a cached snapshot to replay.
	var snap map[string]any
	for {
		snap = mustReadSnapshot(t, playerConn, ctx)
		phase, _ := snap["phase"].(string)
		turn, _ := snap["turn"].(float64)
		if phase == "passing" && int(turn) == 0 {
			break
		}
		if phase == "game_over" {
			t.Fatal("game ended before human passing turn")
		}
	}
	legalActions := extractCards(t, snap["legal_actions"])
	if len(legalActions) < 3 {
		t.Fatalf("expected at least 3 legal actions for passing, got %d", len(legalActions))
	}
	seq, _ := snap["seq"].(float64)
	sendPassCards(t, playerConn, "pass-dup", int(seq), legalActions[:3])
	acceptSnap := mustReadSnapshot(t, playerConn, ctx)
	if msgType, ok := acceptSnap["type"].(string); ok && msgType == "error" {
		t.Fatalf("got error for initial pass_cards: %v", acceptSnap)
	}
	acceptSeq, _ := acceptSnap["seq"].(float64)

	// Wait for the human's first playing turn, then pause.
	for {
		snap = mustReadSnapshot(t, playerConn, ctx)
		phase, _ := snap["phase"].(string)
		turn, _ := snap["turn"].(float64)
		if phase == "playing" && int(turn) == 0 {
			break
		}
		if phase == "game_over" {
			t.Fatal("game ended before human playing turn")
		}
	}
	seq, _ = snap["seq"].(float64)
	pauseMsg := api.InboundMessage{
		Type:     "pause",
		ActionID: "pause-1",
		Seq:      int(seq),
		Payload:  json.RawMessage("{}"),
	}
	if err := writeWSJSON(ctx, playerConn, pauseMsg); err != nil {
		t.Fatalf("write pause: %v", err)
	}
	pauseSnap := mustReadSnapshot(t, playerConn, ctx)
	if pauseSnap["paused"] != true {
		t.Fatalf("expected paused=true after pause, got %v", pauseSnap["paused"])
	}
	pauseSeq, _ := pauseSnap["seq"].(float64)

	// Replaying the accepted action_id while paused returns the cached
	// snapshot, not a game_paused error.
	sendPassCards(t, playerConn, "pass-dup", int(pauseSeq), legalActions[:3])
	dupSnap := mustReadSnapshot(t, playerConn, ctx)
	if msgType, ok := dupSnap["type"].(string); ok && msgType == "error" {
		t.Fatalf("got error for duplicate action_id during pause: %v", dupSnap)
	}
	dupSeq, _ := dupSnap["seq"].(float64)
	if int(dupSeq) != int(acceptSeq) {
		t.Fatalf("duplicate response seq: got %d, want cached snapshot seq %d",
			int(dupSeq), int(acceptSeq))
	}

	_ = playerConn.Close(websocket.StatusNormalClosure, "")
	_ = mgr.Delete(id)
}

// sendGameplayCommand sends the phase-appropriate gameplay command for the
// paused-guard tests: pass_cards with the first three legal actions during
// passing, play_card with the first legal action during playing.
func sendGameplayCommand(t *testing.T, conn *websocket.Conn, phase, actionID string, seq int,
	legalActions []heartsapi.Card) {
	t.Helper()
	switch phase {
	case "passing":
		if len(legalActions) < 3 {
			t.Fatalf("expected at least 3 legal actions for passing, got %d", len(legalActions))
		}
		sendPassCards(t, conn, actionID, seq, legalActions[:3])
	case "playing":
		if len(legalActions) == 0 {
			t.Fatal("no legal actions available for playing")
		}
		sendPlayCard(t, conn, actionID, seq, legalActions[0])
	default:
		t.Fatalf("unexpected phase %q", phase)
	}
}
