package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestConnMaxSeenSeq verifies that ReadSnapshot filters stale
// snapshots and accepts fresh ones, using realistic seq values (the
// server contract guarantees seq >= 1).
func TestConnMaxSeenSeq(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	url := mustStartWSServer(t, func(conn *websocket.Conn) {
		_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"snapshot","seq":1}`))
		_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"snapshot","seq":1}`))
		_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"snapshot","seq":3}`))
		_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"snapshot","seq":2}`))
		_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"snapshot","seq":5}`))
	})

	conn := &Conn{}
	if err := conn.Connect(ctx, url, "token"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	data, err := conn.ReadSnapshot(ctx)
	if err != nil {
		t.Fatalf("read first snapshot: %v", err)
	}
	if got := string(data); got != `{"type":"snapshot","seq":1}` {
		t.Errorf("got %s, want {\"type\":\"snapshot\",\"seq\":1}", got)
	}

	data, err = conn.ReadSnapshot(ctx)
	if err != nil {
		t.Fatalf("read second snapshot: %v", err)
	}
	if got := string(data); got != `{"type":"snapshot","seq":3}` {
		t.Errorf("got %s, want {\"type\":\"snapshot\",\"seq\":3}", got)
	}

	data, err = conn.ReadSnapshot(ctx)
	if err != nil {
		t.Fatalf("read third snapshot: %v", err)
	}
	if got := string(data); got != `{"type":"snapshot","seq":5}` {
		t.Errorf("got %s, want {\"type\":\"snapshot\",\"seq\":5}", got)
	}

	if got := conn.MaxSeenSeq(); got != 5 {
		t.Errorf("got maxSeenSeq %d, want 5", got)
	}
}

// TestConnErrorClassification verifies that error messages are decoded
// and returned as *ErrorMessage with correct classification.
func TestConnErrorClassification(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	url := mustStartWSServer(t, func(conn *websocket.Conn) {
		em := ErrorMessage{
			Type:      "error",
			ErrorCode: ErrIllegalMove,
			Message:   "must follow suit",
			ActionID:  "act-123",
		}
		data, _ := json.Marshal(em)
		_ = conn.Write(ctx, websocket.MessageText, data)
	})

	conn := &Conn{}
	if err := conn.Connect(ctx, url, "token"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_, err := conn.ReadSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var em *ErrorMessage
	if !errors.As(err, &em) {
		t.Fatalf("expected *ErrorMessage, got %T", err)
	}
	if em.ErrorCode != ErrIllegalMove {
		t.Errorf("got error code %s, want %s", em.ErrorCode, ErrIllegalMove)
	}
	if em.Message != "must follow suit" {
		t.Errorf("got message %s, want must follow suit", em.Message)
	}
}

// TestConnSendCommand verifies that SendCommand marshals the message
// and writes it to the WebSocket, using maxSeenSeq as the seq field.
func TestConnSendCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	gotCh := make(chan Command, 1)
	url := mustStartWSServer(t, func(conn *websocket.Conn) {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Logf("read: %v", err)
			return
		}
		var msg Command
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Logf("unmarshal: %v", err)
			return
		}
		gotCh <- msg
	})

	conn := &Conn{}
	if err := conn.Connect(ctx, url, "token"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	conn.mu.Lock()
	conn.maxSeenSeq = 7
	conn.mu.Unlock()

	msg := Command{
		Type:     "play_card",
		ActionID: "act-456",
		Payload:  []byte(`{"card":{"rank":"ace","suit":"spades"}}`),
	}
	if err := conn.SendCommand(ctx, msg); err != nil {
		t.Fatalf("send command: %v", err)
	}

	select {
	case gotMsg := <-gotCh:
		if gotMsg.Type != "play_card" {
			t.Errorf("got type %s, want play_card", gotMsg.Type)
		}
		if gotMsg.ActionID != "act-456" {
			t.Errorf("got action_id %s, want act-456", gotMsg.ActionID)
		}
		if gotMsg.Seq != 7 {
			t.Errorf("got seq %d, want 7", gotMsg.Seq)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for command")
	}
}

// TestConnClose verifies that Close closes the WebSocket connection.
func TestConnClose(t *testing.T) {
	ctx := t.Context()
	url := mustStartWSServer(t, func(conn *websocket.Conn) {
		_, _, _ = conn.Read(ctx)
	})

	conn := &Conn{}
	if err := conn.Connect(ctx, url, "token"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

// TestConnCloseCodeClassification verifies that ReadSnapshot returns a
// ConnectionClosedError with the correct close code when the server
// closes the connection.
func TestConnCloseCodeClassification(t *testing.T) {
	tests := []struct {
		name     string
		code     websocket.StatusCode
		wantCode int
	}{
		{"normal closure", websocket.StatusNormalClosure, 1000},
		{"going away", websocket.StatusGoingAway, 1001},
		{"message too big", websocket.StatusMessageTooBig, 1009},
		{"internal error", websocket.StatusInternalError, 1011},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			url := mustStartWSServer(t, func(conn *websocket.Conn) {
				_ = conn.Close(tt.code, "test reason")
			})

			conn := &Conn{}
			if err := conn.Connect(ctx, url, "token"); err != nil {
				t.Fatalf("connect: %v", err)
			}
			defer func() { _ = conn.Close() }()

			_, err := conn.ReadSnapshot(ctx)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var ce *ConnectionClosedError
			if !errors.As(err, &ce) {
				t.Fatalf("expected *ConnectionClosedError, got %T", err)
			}
			if ce.Code != tt.wantCode {
				t.Errorf("got code %d, want %d", ce.Code, tt.wantCode)
			}
		})
	}
}

// TestConnConnectionOpenAfterError verifies that the WebSocket
// connection remains open after receiving an error message, allowing
// subsequent snapshots to be read.
func TestConnConnectionOpenAfterError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	url := mustStartWSServer(t, func(conn *websocket.Conn) {
		em := ErrorMessage{
			Type:       "error",
			ErrorCode:  ErrOutOfTurn,
			Message:    "not your turn",
			CurrentSeq: 5,
		}
		data, _ := json.Marshal(em)
		_ = conn.Write(ctx, websocket.MessageText, data)
		_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"snapshot","seq":7}`))
	})

	conn := &Conn{}
	if err := conn.Connect(ctx, url, "token"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_, err := conn.ReadSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var em *ErrorMessage
	if !errors.As(err, &em) {
		t.Fatalf("expected *ErrorMessage, got %T", err)
	}

	data, err := conn.ReadSnapshot(ctx)
	if err != nil {
		t.Fatalf("read snapshot after error: %v", err)
	}
	if got := string(data); got != `{"type":"snapshot","seq":7}` {
		t.Errorf("got %s, want {\"type\":\"snapshot\",\"seq\":7}", got)
	}
}

// TestConnMalformedSnapshot verifies that ReadSnapshot returns an error
// when the server sends invalid JSON.
func TestConnMalformedSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	url := mustStartWSServer(t, func(conn *websocket.Conn) {
		_ = conn.Write(ctx, websocket.MessageText, []byte(`{invalid json`))
	})

	conn := &Conn{}
	if err := conn.Connect(ctx, url, "token"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_, err := conn.ReadSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("got error %q, want containing 'unmarshal'", err.Error())
	}
}

// TestConnReadSnapshotContextCancel verifies that ReadSnapshot respects
// context cancellation.
func TestConnReadSnapshotContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	url := mustStartWSServer(t, func(conn *websocket.Conn) {
		<-ctx.Done()
	})

	conn := &Conn{}
	if err := conn.Connect(ctx, url, "token"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	readCtx, readCancel := context.WithCancel(ctx)
	readCancel()

	_, err := conn.ReadSnapshot(readCtx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got error %v, want context.Canceled", err)
	}
}

// TestConnSkipsBinaryMessages verifies that ReadSnapshot skips binary
// messages and continues reading for text snapshots.
func TestConnSkipsBinaryMessages(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	url := mustStartWSServer(t, func(conn *websocket.Conn) {
		_ = conn.Write(ctx, websocket.MessageBinary, []byte(`binary data`))
		_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"snapshot","seq":1}`))
	})

	conn := &Conn{}
	if err := conn.Connect(ctx, url, "token"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	data, err := conn.ReadSnapshot(ctx)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if got := string(data); got != `{"type":"snapshot","seq":1}` {
		t.Errorf("got %s, want {\"type\":\"snapshot\",\"seq\":1}", got)
	}
}

// TestConnErrorUpdatesMaxSeenSeq verifies that error messages update
// maxSeenSeq from their current_seq field.
func TestConnErrorUpdatesMaxSeenSeq(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	url := mustStartWSServer(t, func(conn *websocket.Conn) {
		_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"snapshot","seq":3}`))
		em := ErrorMessage{
			Type:       "error",
			ErrorCode:  ErrOutOfTurn,
			Message:    "not your turn",
			CurrentSeq: 5,
		}
		data, _ := json.Marshal(em)
		_ = conn.Write(ctx, websocket.MessageText, data)
		em2 := ErrorMessage{
			Type:       "error",
			ErrorCode:  ErrOutOfTurn,
			Message:    "not your turn",
			CurrentSeq: 2,
		}
		data2, _ := json.Marshal(em2)
		_ = conn.Write(ctx, websocket.MessageText, data2)
	})

	conn := &Conn{}
	if err := conn.Connect(ctx, url, "token"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_, err := conn.ReadSnapshot(ctx)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	_, err = conn.ReadSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := conn.MaxSeenSeq(); got != 5 {
		t.Errorf("after current_seq=5 error, got maxSeenSeq %d, want 5", got)
	}

	_, err = conn.ReadSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := conn.MaxSeenSeq(); got != 5 {
		t.Errorf("after current_seq=2 error, got maxSeenSeq %d, want 5", got)
	}
}

// mustStartWSServer starts an httptest.Server with a WebSocket handler
// that calls fn for each connection.
func mustStartWSServer(t *testing.T, fn func(*websocket.Conn)) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Logf("accept: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		fn(conn)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
}
