package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
)

// testSnapshot is a minimal snapshot struct for fast unmarshal in tests.
type testSnapshot struct {
	Phase string `json:"phase"`
	Seq   int    `json:"seq"`
}

// mustStartTestServer starts an httptest.Server for the given Server and
// registers cleanup.
func mustStartTestServer(t *testing.T, srv *Server) *httptest.Server {
	t.Helper()
	httpSrv := httptest.NewServer(srv.mux)
	t.Cleanup(httpSrv.Close)
	return httpSrv
}

// mustDialPlayerWS dials the player WebSocket endpoint and verifies a 101
// Switching Protocols response. It registers connection cleanup.
func mustDialPlayerWS(t *testing.T, httpSrvURL, id, token string) *websocket.Conn {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(httpSrvURL, "http") +
		"/sessions/" + id + "/ws"
	conn, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
	}
	return conn
}

// mustDialObserverWS dials the observer WebSocket endpoint and verifies a
// 101 Switching Protocols response. It registers connection cleanup.
func mustDialObserverWS(t *testing.T, httpSrvURL, id string) *websocket.Conn {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(httpSrvURL, "http") +
		"/sessions/" + id + "/ws/observe"
	conn, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
	}
	return conn
}

// mustReadWSMessage reads a text message from the WebSocket connection.
func mustReadWSMessage(t *testing.T, conn *websocket.Conn, ctx context.Context) []byte {
	t.Helper()
	typ, b, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if typ != websocket.MessageText {
		t.Fatalf("got message type %d, want text", typ)
	}
	return b
}

// mustReadSnapshot reads a WebSocket message and unmarshals it as a snapshot.
func mustReadSnapshot(t *testing.T, conn *websocket.Conn, ctx context.Context) map[string]any {
	t.Helper()
	b := mustReadWSMessage(t, conn, ctx)
	var snap map[string]any
	if err := json.Unmarshal(b, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	return snap
}

// mustReadTestSnapshot reads a WebSocket message and unmarshals only phase
// and seq for fast observation tests.
func mustReadTestSnapshot(t *testing.T, conn *websocket.Conn, ctx context.Context) testSnapshot {
	t.Helper()
	b := mustReadWSMessage(t, conn, ctx)
	var snap testSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	return snap
}

// readTestSnapshot reads a WebSocket message and unmarshals only phase
// and seq. It returns an error instead of failing the test so it can be used
// in goroutines.
func readTestSnapshot(ctx context.Context, conn *websocket.Conn) (testSnapshot, error) {
	typ, b, err := conn.Read(ctx)
	if err != nil {
		return testSnapshot{}, err
	}
	if typ != websocket.MessageText {
		return testSnapshot{}, fmt.Errorf("got message type %d, want text", typ)
	}
	var snap testSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return testSnapshot{}, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return snap, nil
}

// mustReadError reads a WebSocket message and unmarshals it as an ErrorMessage.
func mustReadError(t *testing.T, conn *websocket.Conn, ctx context.Context) *api.ErrorMessage {
	t.Helper()
	b := mustReadWSMessage(t, conn, ctx)
	var em api.ErrorMessage
	if err := json.Unmarshal(b, &em); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	return &em
}

// writeWSJSON marshals v as JSON and writes it as a text message on ws.
// It uses a 30-second timeout context.
func writeWSJSON(ctx context.Context, ws *websocket.Conn, v any) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return ws.Write(ctx, websocket.MessageText, b)
}

// readSnapshotsUntil reads testSnapshot messages from the observer connection
// until a snapshot with the target phase is received.
func readSnapshotsUntil(t *testing.T, conn *websocket.Conn, ctx context.Context,
	targetPhase string) []testSnapshot {
	t.Helper()
	var snaps []testSnapshot
	for {
		snap := mustReadTestSnapshot(t, conn, ctx)
		snaps = append(snaps, snap)
		if snap.Phase == targetPhase {
			return snaps
		}
	}
}
