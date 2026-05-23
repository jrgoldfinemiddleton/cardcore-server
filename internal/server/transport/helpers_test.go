package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
)

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
