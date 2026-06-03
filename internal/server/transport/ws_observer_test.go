package transport

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestObserverWSReceivesInitialSnapshotIntegration verifies that an observer receives
// an initial snapshot immediately upon WebSocket connection.
func TestObserverWSReceivesInitialSnapshotIntegration(t *testing.T) {
	srv, id, _ := setupTestServerWithSession(t)
	httpSrv := mustStartTestServer(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn := mustDialObserverWS(t, httpSrv.URL, id)
	snap := mustReadSnapshot(t, conn, ctx)
	if snap["type"] != "snapshot" {
		t.Errorf("got type %q, want %q", snap["type"], "snapshot")
	}
}

// TestObserverWSReceivesBroadcastSnapshotsIntegration verifies that an observer
// receives snapshots broadcast after state-changing events.
func TestObserverWSReceivesBroadcastSnapshotsIntegration(t *testing.T) {
	srv, id, token := setupTestServerWithSession(t)
	httpSrv := mustStartTestServer(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect observer.
	obsConn := mustDialObserverWS(t, httpSrv.URL, id)
	_ = mustReadSnapshot(t, obsConn, ctx) // consume initial snapshot

	// Connect player and trigger a state change.
	playerConn := mustDialPlayerWS(t, httpSrv.URL, id, token)
	_ = mustReadSnapshot(t, playerConn, ctx) // consume player initial snapshot

	cmd := map[string]any{
		"type":      "play_card",
		"action_id": "test-action-1",
		"seq":       1,
		"payload": map[string]any{
			"card": map[string]any{"rank": "two", "suit": "clubs"},
		},
	}
	cmdBytes, _ := json.Marshal(cmd)
	if err := playerConn.Write(ctx, websocket.MessageText, cmdBytes); err != nil {
		t.Fatalf("write command: %v", err)
	}

	// Player must read its response so the session goroutine's broadcast
	// (a sequential send to all subscribers) does not stall waiting on this
	// player's subCh, which would prevent the observer from receiving the
	// updated snapshot.
	_ = mustReadWSMessage(t, playerConn, ctx)

	// Observer should receive the updated snapshot.
	snap := mustReadSnapshot(t, obsConn, ctx)
	if snap["type"] != "snapshot" {
		t.Errorf("got type %q, want %q", snap["type"], "snapshot")
	}
}

// TestObserverWSIgnoresInboundMessagesIntegration verifies that messages sent from
// the client to the observer WebSocket are ignored. The server responds
// with a close frame (policy violation for unexpected data), but the
// server process remains healthy and can accept new connections.
func TestObserverWSIgnoresInboundMessagesIntegration(t *testing.T) {
	srv, id, _ := setupTestServerWithSession(t)
	httpSrv := mustStartTestServer(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn := mustDialObserverWS(t, httpSrv.URL, id)
	_ = mustReadSnapshot(t, conn, ctx) // consume initial snapshot

	// Send a message from the client (observers should not process inbound).
	msg := []byte(`{"type":"junk","action_id":"abc","seq":0}`)
	if err := conn.Write(ctx, websocket.MessageText, msg); err != nil {
		t.Fatalf("write message: %v", err)
	}

	// Server will close the connection with a policy violation.
	// Wait for it and verify the server is still accepting connections.
	time.Sleep(100 * time.Millisecond)

	// Verify the server is still healthy by connecting a new observer.
	conn2 := mustDialObserverWS(t, httpSrv.URL, id)
	_ = mustReadSnapshot(t, conn2, ctx)
	if err := conn2.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatalf("close conn2: %v", err)
	}
}
