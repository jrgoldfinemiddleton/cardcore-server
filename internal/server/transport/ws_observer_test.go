package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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

// TestObserverWSSurvivesAbruptClientDisconnectIntegration reproduces the
// benign teardown race where a client closes its TCP connection while the
// server is mid-write. The server must survive and remain healthy.
func TestObserverWSSurvivesAbruptClientDisconnectIntegration(t *testing.T) {
	srv, id, token := setupTestServerWithSession(t)
	httpSrv := mustStartTestServer(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	obsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") +
		"/sessions/" + id + "/ws/observe"
	obsConn, resp, err := websocket.Dial(ctx, obsURL, nil)
	if err != nil {
		t.Fatalf("dial observer: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
	}

	_ = mustReadSnapshot(t, obsConn, ctx)

	// Simulate abrupt client disconnect (process kill, ctrl-c, network drop).
	_ = obsConn.CloseNow()
	time.Sleep(50 * time.Millisecond)

	// Trigger a snapshot broadcast that will hit the dead connection.
	playerURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") +
		"/sessions/" + id + "/ws"
	playerConn, _, err := websocket.Dial(ctx, playerURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}},
	})
	if err != nil {
		t.Fatalf("dial player: %v", err)
	}
	defer func() { _ = playerConn.Close(websocket.StatusNormalClosure, "") }()

	_ = mustReadSnapshot(t, playerConn, ctx)

	cmd := map[string]any{
		"type":      "play_card",
		"action_id": "test-disconnect-action",
		"seq":       1,
		"payload": map[string]any{
			"card": map[string]any{"rank": "two", "suit": "clubs"},
		},
	}
	cmdBytes, _ := json.Marshal(cmd)
	if err := playerConn.Write(ctx, websocket.MessageText, cmdBytes); err != nil {
		t.Fatalf("write command: %v", err)
	}
	_ = mustReadWSMessage(t, playerConn, ctx)

	time.Sleep(100 * time.Millisecond)

	// Prove the session survived: a new observer connects successfully.
	obsConn2, resp2, err := websocket.Dial(ctx, obsURL, nil)
	if err != nil {
		t.Fatalf("dial second observer: %v", err)
	}
	if resp2.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("got status %d, want %d", resp2.StatusCode, http.StatusSwitchingProtocols)
	}
	defer func() { _ = obsConn2.Close(websocket.StatusNormalClosure, "") }()

	snap := mustReadSnapshot(t, obsConn2, ctx)
	if snap["type"] != "snapshot" {
		t.Errorf("got type %q, want %q", snap["type"], "snapshot")
	}
}
