package transport

import (
	"context"
	"encoding/json"
	"flag"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// errorGameConfig is a minimal [session.GameConfig] that creates errorGame.
type errorGameConfig struct{}

// errorGame is a stub Game implementation that rejects every action with
// an illegal_move error.
type errorGame struct{}

// TestPlayerWSSendsCommandIntegration verifies that a player can send a command via
// WebSocket and receive a snapshot response.
func TestPlayerWSSendsCommandIntegration(t *testing.T) {
	srv, id, token := setupTestServerWithSession(t)
	httpSrv := mustStartTestServer(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn := mustDialPlayerWS(t, httpSrv.URL, id, token)
	_ = mustReadWSMessage(t, conn, ctx) // consume initial snapshot

	cmd := api.InboundMessage{
		Type:     "play_card",
		ActionID: "test-action-1",
		Seq:      1,
		Payload:  json.RawMessage(`{"card":"2c"}`),
	}
	if err := writeWSJSON(ctx, conn, cmd); err != nil {
		t.Fatalf("write command: %v", err)
	}

	snap := mustReadSnapshot(t, conn, ctx)
	if snap["type"] != "snapshot" {
		t.Errorf("got type %q, want %q", snap["type"], "snapshot")
	}
}

// TestPlayerWSMalformedMessageIntegration verifies that a message missing required
// fields produces a malformed_message error.
func TestPlayerWSMalformedMessageIntegration(t *testing.T) {
	srv, id, token := setupTestServerWithSession(t)
	httpSrv := mustStartTestServer(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn := mustDialPlayerWS(t, httpSrv.URL, id, token)
	_ = mustReadWSMessage(t, conn, ctx) // consume initial snapshot

	cmd := api.InboundMessage{
		ActionID: "test-action-2",
		Seq:      1,
	}
	if err := writeWSJSON(ctx, conn, cmd); err != nil {
		t.Fatalf("write command: %v", err)
	}

	em := mustReadError(t, conn, ctx)
	if em.Type != "error" {
		t.Errorf("got type %q, want %q", em.Type, "error")
	}
	if em.ErrorCode != api.ErrMalformedMessage {
		t.Errorf("got error_code %q, want %q", em.ErrorCode, api.ErrMalformedMessage)
	}
}

// TestPlayerWSKickedOnSecondConnectionIntegration verifies that a second connection
// with the same seat token causes the first connection to stop receiving
// snapshots.
func TestPlayerWSKickedOnSecondConnectionIntegration(t *testing.T) {
	srv, id, token := setupTestServerWithSession(t)
	httpSrv := mustStartTestServer(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn1 := mustDialPlayerWS(t, httpSrv.URL, id, token)
	_ = mustReadWSMessage(t, conn1, ctx) // consume conn1 initial snapshot

	conn2 := mustDialPlayerWS(t, httpSrv.URL, id, token)
	_ = mustReadWSMessage(t, conn2, ctx) // consume conn2 initial snapshot

	// conn1 should now be closed (or stop receiving messages).
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer shortCancel()
	_, _, err := conn1.Read(shortCtx)
	if err == nil {
		t.Fatal("conn1 still receiving after kick")
	}
}

// TestPlayerWSReceivesGameErrorIntegration verifies that a game error is returned
// as an error message on the WebSocket.
func TestPlayerWSReceivesGameErrorIntegration(t *testing.T) {
	r := session.NewRegistry()
	r.Register(&errorGameConfig{})
	mgr := session.NewManager(r, session.DefaultServerDelays)
	srv := NewServer(Config{Manager: mgr, Addr: ":0"})

	cfg := session.Config{
		Game: "hearts",
		Seats: []session.SeatConfig{
			{Type: session.SeatHuman},
			{Type: session.SeatAI, AIType: "random"},
			{Type: session.SeatAI, AIType: "random"},
			{Type: session.SeatAI, AIType: "random"},
		},
	}
	info, seats, err := mgr.Create(cfg)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := mgr.Start(info.SessionID); err != nil {
		t.Fatalf("start session: %v", err)
	}

	var token string
	for _, s := range seats {
		if s.Type == session.SeatHuman {
			token = s.Token
			break
		}
	}

	httpSrv := mustStartTestServer(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn := mustDialPlayerWS(t, httpSrv.URL, info.SessionID, token)
	_ = mustReadWSMessage(t, conn, ctx) // consume initial snapshot

	cmd := api.InboundMessage{
		Type:     "play_card",
		ActionID: "test-action-3",
		Seq:      1,
		Payload:  json.RawMessage(`{"card":"2c"}`),
	}
	if err := writeWSJSON(ctx, conn, cmd); err != nil {
		t.Fatalf("write command: %v", err)
	}

	em := mustReadError(t, conn, ctx)
	if em.Type != "error" {
		t.Errorf("got type %q, want %q", em.Type, "error")
	}
	if em.ErrorCode != api.ErrIllegalMove {
		t.Errorf("got error_code %q, want %q", em.ErrorCode, api.ErrIllegalMove)
	}
}

// TestPlayerWSCleanupOnDisconnectIntegration verifies that the player's subscription
// is removed when the WebSocket connection is closed by the client.
func TestPlayerWSCleanupOnDisconnectIntegration(t *testing.T) {
	srv, id, token := setupTestServerWithSession(t)
	httpSrv := mustStartTestServer(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn := mustDialPlayerWS(t, httpSrv.URL, id, token)
	_ = mustReadWSMessage(t, conn, ctx) // consume initial snapshot

	// Close from client side.
	if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Allow goroutines to exit.
	time.Sleep(100 * time.Millisecond)

	// A new subscription to the same seat should succeed,
	// confirming the old one was cleaned up.
	ch, err := srv.mgr.SubscribePlayer(id, 0)
	if err != nil {
		t.Fatalf("resubscribe after disconnect: %v", err)
	}
	if ch == nil {
		t.Fatal("resubscribe returned nil channel")
	}
	if err := srv.mgr.UnsubscribePlayer(id, 0); err != nil {
		t.Fatalf("cleanup test subscription: %v", err)
	}
}

// Name returns "hearts" so the registry resolves the test session config
// to the error game config.
func (errorGameConfig) Name() string { return "hearts" }

// RegisterFlags is a no-op; the error game config takes no flags.
func (errorGameConfig) RegisterFlags(*flag.FlagSet) {}

// Validate is a no-op; the error game config has no flag values to check.
func (errorGameConfig) Validate() error { return nil }

// NewGame returns an errorGame so the test can verify that a rejected
// action surfaces as an error message on the player's WebSocket.
func (errorGameConfig) NewGame(_ session.Config, _ *rand.Rand) (session.Game, error) {
	return errorGame{}, nil
}

// ValidateConfig is a no-op; the error game config accepts any seat
// configuration.
func (errorGameConfig) ValidateConfig(_ session.Config) error { return nil }

// HandleAction rejects every action with an illegal_move CommandError so
// the test can verify that a game rejection is delivered to the player as
// an error message.
func (errorGame) HandleAction(
	int, *api.InboundMessage,
) (session.StepResult, *session.CommandError) {
	return session.StepResult{}, &session.CommandError{
		Code:    api.ErrIllegalMove,
		Message: "test error",
	}
}

// AIPlay reports StepContinue without doing anything; errorGame has no AI
// logic, and the test drives the game only through the rejected command.
func (errorGame) AIPlay(int) (session.StepResult, error) {
	return session.StepResult{}, nil
}

// Resume reports StepContinue without changing state; errorGame never
// returns StepPause, so Resume only satisfies the interface.
func (errorGame) Resume() (session.StepResult, error) {
	return session.StepResult{}, nil
}

// Turn always returns 0; errorGame does not model turn order.
func (errorGame) Turn() int { return 0 }

// PlayerSnapshot returns the same minimal map for every seat, carrying only
// type and seq; the test needs a deliverable initial snapshot before its
// command is rejected, nothing more.
func (errorGame) PlayerSnapshot(_, seq int) any {
	return map[string]any{"type": "snapshot", "seq": seq}
}

// ObserverSnapshot returns a minimal map carrying only type and seq;
// errorGame has no real state to expose.
func (errorGame) ObserverSnapshot(seq int) any {
	return map[string]any{"type": "snapshot", "seq": seq}
}

// DisplayDelay returns 0; errorGame has no pacing states, and zero delay
// keeps the test fast.
func (errorGame) DisplayDelay() int { return 0 }

// SetPaused is a no-op; errorGame does not track pause state.
func (errorGame) SetPaused(bool) {}

// SetTurnDeadline is a no-op; errorGame does not track turn deadlines.
func (errorGame) SetTurnDeadline(time.Time) {}
