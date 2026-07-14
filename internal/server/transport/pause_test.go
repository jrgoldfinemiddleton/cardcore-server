package transport

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
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
