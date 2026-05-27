package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// stubGame is a minimal Game implementation for transport tests.
type stubGame struct{}

// unmarshalableStubGame returns snapshots that json.Marshal cannot
// serialize, forcing the session goroutine to drop them rather than
// sending nil/empty frames to WebSocket subscribers.
type unmarshalableStubGame struct{}

// TestNewServerRegistersAllRoutes verifies that all 8 routes are registered
// in the mux and return a non-404-not-found response (even if the handler
// itself returns 404 for missing resources).
func TestNewServerRegistersAllRoutes(t *testing.T) {
	srv, _ := setupTestServer(t)

	routes := []struct {
		method string
		path   string
		body   string
	}{
		{"POST", "/sessions", `{"game":"hearts","seats":[{"type":"ai","ai_type":"random"}]}`},
		{"GET", "/sessions", ""},
		{"GET", "/sessions/test-id", ""},
		{"PATCH", "/sessions/test-id", `{"pacing_delay_ms":250}`},
		{"POST", "/sessions/test-id/start", ""},
		{"DELETE", "/sessions/test-id", ""},
	}

	for _, rt := range routes {
		var body *bytes.Reader
		if rt.body != "" {
			body = bytes.NewReader([]byte(rt.body))
		} else {
			body = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(rt.method, rt.path, body)
		if rt.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		// A route that is not registered returns "404 page not found\n"
		// from the default mux handler.
		if rec.Code == http.StatusNotFound &&
			strings.TrimSpace(rec.Body.String()) == "404 page not found" {
			t.Errorf("route %s %s not registered", rt.method, rt.path)
		}
	}
}

// TestServerStartStop verifies that the server can start on a random
// port and shut down gracefully.
func TestServerStartStop(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Start the server in a goroutine.
	done := make(chan error, 1)
	go func() {
		done <- srv.Start()
	}()

	// Wait for the server to be listening.
	time.Sleep(50 * time.Millisecond)

	addr := srv.Addr()
	if addr == "" {
		t.Fatal("server has no address after start")
	}

	// Verify we can connect.
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	_ = resp.Body.Close()

	// Root is not registered, so we expect 404.
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	// Graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Stop(ctx); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}

	// Wait for Start to return (should be nil or ErrServerClosed).
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Start() returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return after Stop()")
	}
}

// TestPanicRecovery verifies that the recovery middleware catches
// panics and returns 500 without crashing the server.
func TestPanicRecovery(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("intentional panic for test")
	})
	wrapped := recoveryMiddleware(panicHandler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// TestRequestLogMiddleware verifies that the request logging middleware
// passes through to the next handler.
func TestRequestLogMiddleware(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := requestLogMiddleware(okHandler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestResponseWriterCapturesStatus verifies that the responseWriter
// wrapper correctly captures the status code.
func TestResponseWriterCapturesStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusNotFound)
	if rw.statusCode != http.StatusNotFound {
		t.Errorf("got statusCode %d, want %d", rw.statusCode, http.StatusNotFound)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("recorder got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHttpStatus maps session errors to the correct HTTP status codes.
func TestHttpStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"invalid config", session.ErrInvalidConfig, http.StatusBadRequest},
		{"not found", session.ErrNotFound, http.StatusNotFound},
		{"not draft", session.ErrNotDraft, http.StatusConflict},
		{"not active", session.ErrNotActive, http.StatusConflict},
		{"unexpected", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := httpStatus(tc.err)
			if got != tc.want {
				t.Errorf("got status %d, want %d", got, tc.want)
			}
		})
	}
}

// TestWriteError verifies writeError produces correct JSON and status.
func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "something went wrong")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var body errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error != "something went wrong" {
		t.Errorf("got error %q, want %q", body.Error, "something went wrong")
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("got content-type %q, want %q", ct, "application/json")
	}
}

// TestHandleCreateSession verifies POST /sessions handler.
func TestHandleCreateSession(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantErr    string
	}{
		{
			name:       "valid all-ai",
			body:       `{"game":"hearts","seats":[{"type":"ai","ai_type":"random"}]}`,
			wantStatus: http.StatusCreated,
		},
		{
			name: "valid with human",
			body: `{"game":"hearts","seats":[` +
				`{"type":"human"},{"type":"ai","ai_type":"random"}]}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing game",
			body:       `{"seats":[{"type":"ai","ai_type":"random"}]}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "game is required",
		},
		{
			name:       "empty seats",
			body:       `{"game":"hearts","seats":[]}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "at least one seat",
		},
		{
			name:       "ai without ai_type",
			body:       `{"game":"hearts","seats":[{"type":"ai"}]}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "ai_type is required",
		},
		{
			name:       "invalid seat type",
			body:       `{"game":"hearts","seats":[{"type":"robot"}]}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "type must be",
		},
		{
			name:       "empty json object",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "game is required",
		},
		{
			name:       "invalid json",
			body:       `{bad json`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "invalid request body",
		},
		{
			name:       "empty body",
			body:       "",
			wantStatus: http.StatusBadRequest,
			wantErr:    "invalid request body",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := setupTestServer(t)
			var body *bytes.Reader
			if tc.body != "" {
				body = bytes.NewReader([]byte(tc.body))
			} else {
				body = bytes.NewReader(nil)
			}
			req := httptest.NewRequest(http.MethodPost, "/sessions", body)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			srv.mux.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantErr != "" {
				var er errorResponse
				if err := json.NewDecoder(rec.Body).Decode(&er); err != nil {
					t.Fatalf("decode error: %v", err)
				}
				if !strings.Contains(er.Error, tc.wantErr) {
					t.Errorf("got error %q, want containing %q", er.Error, tc.wantErr)
				}
			} else {
				var resp createResponse
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("decode success response: %v", err)
				}
				if resp.SessionID == "" {
					t.Error("got empty session_id")
				}
				// Count human seats and verify tokens.
				var humanCount int
				for _, s := range resp.Seats {
					if s.Type == session.SeatHuman {
						humanCount++
						if s.Token == "" {
							t.Errorf("human seat %d has no token", s.Index)
						}
					}
				}
				// Verify response has correct number of human seats.
				var expectedHumans int
				if tc.name == "valid with human" {
					expectedHumans = 1
				}
				if humanCount != expectedHumans {
					t.Errorf("got %d human seats, want %d", humanCount, expectedHumans)
				}
				// Verify response has correct number of seats.
				var expectedSeats int
				if tc.name == "valid with human" {
					expectedSeats = 2
				} else {
					expectedSeats = 1
				}
				if len(resp.Seats) != expectedSeats {
					t.Errorf("got %d seats, want %d", len(resp.Seats), expectedSeats)
				}
			}
		})
	}
}

// TestHandleListSessions verifies GET /sessions handler.
func TestHandleListSessions(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		srv, _ := setupTestServer(t)
		req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
		}
		var lr listResponse
		if err := json.NewDecoder(rec.Body).Decode(&lr); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(lr.Sessions) != 0 {
			t.Errorf("got %d sessions, want 0", len(lr.Sessions))
		}
	})

	t.Run("with session", func(t *testing.T) {
		srv, mgr := setupTestServer(t)
		_, _, err := mgr.Create(validConfig())
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
		}
		var lr listResponse
		if err := json.NewDecoder(rec.Body).Decode(&lr); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(lr.Sessions) != 1 {
			t.Errorf("got %d sessions, want 1", len(lr.Sessions))
		}
	})

	t.Run("expired excluded", func(t *testing.T) {
		srv, mgr := setupTestServer(t)
		info, _, err := mgr.Create(validConfig())
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := mgr.Delete(info.SessionID); err != nil {
			t.Fatalf("delete: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		var lr listResponse
		if err := json.NewDecoder(rec.Body).Decode(&lr); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(lr.Sessions) != 0 {
			t.Errorf("got %d sessions, want 0", len(lr.Sessions))
		}
	})
}

// TestHandleGetSession verifies GET /sessions/{id} handler.
func TestHandleGetSession(t *testing.T) {
	t.Run("existing", func(t *testing.T) {
		srv, mgr := setupTestServer(t)
		info, _, err := mgr.Create(validConfig())
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/sessions/"+info.SessionID, nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
		}
		var got session.SessionInfo
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.SessionID != info.SessionID {
			t.Errorf("got session_id %q, want %q", got.SessionID, info.SessionID)
		}
		if got.Game != "hearts" {
			t.Errorf("got game %q, want %q", got.Game, "hearts")
		}
		if got.State != session.Draft {
			t.Errorf("got state %q, want %q", got.State, session.Draft)
		}
		if got.PacingDelayMS != 500 {
			t.Errorf("got pacing_delay_ms %d, want 500", got.PacingDelayMS)
		}
		if len(got.Seats) != 1 {
			t.Errorf("got %d seats, want 1", len(got.Seats))
		}
	})

	t.Run("not found", func(t *testing.T) {
		srv, _ := setupTestServer(t)
		req := httptest.NewRequest(http.MethodGet, "/sessions/nope", nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}

// TestHandlePatchSession verifies PATCH /sessions/{id} handler.
func TestHandlePatchSession(t *testing.T) {
	t.Run("update pacing", func(t *testing.T) {
		srv, mgr := setupTestServer(t)
		info, _, err := mgr.Create(validConfig())
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		body := []byte(`{"pacing_delay_ms":250}`)
		req := httptest.NewRequest(http.MethodPatch,
			"/sessions/"+info.SessionID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
		}
		var got session.SessionInfo
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.PacingDelayMS != 250 {
			t.Errorf("got pacing_delay_ms %d, want 250", got.PacingDelayMS)
		}
	})

	t.Run("update seats returns tokens", func(t *testing.T) {
		srv, mgr := setupTestServer(t)
		info, _, err := mgr.Create(validConfig())
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		body := []byte(`{"seats":[{"type":"human"},{"type":"ai","ai_type":"random"}]}`)
		req := httptest.NewRequest(http.MethodPatch,
			"/sessions/"+info.SessionID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
		}
		var got patchResponse
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.SessionID != info.SessionID {
			t.Errorf("got session_id %q, want %q", got.SessionID, info.SessionID)
		}
		if len(got.SeatTokens) != 2 {
			t.Fatalf("got %d seat_tokens, want 2", len(got.SeatTokens))
		}
		var humanFound bool
		for _, s := range got.SeatTokens {
			if s.Type == session.SeatHuman {
				humanFound = true
				if s.Token == "" {
					t.Errorf("human seat %d has no token", s.Index)
				}
			}
		}
		if !humanFound {
			t.Error("no human seat found in response")
		}
	})

	t.Run("not found", func(t *testing.T) {
		srv, _ := setupTestServer(t)
		body := []byte(`{"pacing_delay_ms":250}`)
		req := httptest.NewRequest(http.MethodPatch, "/sessions/nope", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("not draft", func(t *testing.T) {
		srv, mgr := setupTestServer(t)
		info, _, err := mgr.Create(validConfig())
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := mgr.Start(info.SessionID); err != nil {
			t.Fatalf("start: %v", err)
		}

		body := []byte(`{"pacing_delay_ms":250}`)
		req := httptest.NewRequest(http.MethodPatch,
			"/sessions/"+info.SessionID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusConflict)
		}
	})

	t.Run("empty body", func(t *testing.T) {
		srv, mgr := setupTestServer(t)
		info, _, err := mgr.Create(validConfig())
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		body := []byte(`{}`)
		req := httptest.NewRequest(http.MethodPatch,
			"/sessions/"+info.SessionID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
		}
		var got session.SessionInfo
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.SessionID != info.SessionID {
			t.Errorf("got session_id %q, want %q", got.SessionID, info.SessionID)
		}
		if got.State != session.Draft {
			t.Errorf("got state %q, want %q", got.State, session.Draft)
		}
		if got.PacingDelayMS != 500 {
			t.Errorf("got pacing_delay_ms %d, want 500", got.PacingDelayMS)
		}
	})

	t.Run("invalid seats", func(t *testing.T) {
		srv, mgr := setupTestServer(t)
		info, _, err := mgr.Create(validConfig())
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		body := []byte(`{"seats":[{"type":"robot"}]}`)
		req := httptest.NewRequest(http.MethodPatch,
			"/sessions/"+info.SessionID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		srv, mgr := setupTestServer(t)
		info, _, err := mgr.Create(validConfig())
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		body := []byte(`{bad`)
		req := httptest.NewRequest(http.MethodPatch,
			"/sessions/"+info.SessionID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}

// TestHandleStartSession verifies POST /sessions/{id}/start handler.
func TestHandleStartSession(t *testing.T) {
	t.Run("draft session", func(t *testing.T) {
		srv, mgr := setupTestServer(t)
		info, _, err := mgr.Create(validConfig())
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/sessions/"+info.SessionID+"/start", nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
		}
		var got startResponse
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.SessionID != info.SessionID {
			t.Errorf("got session_id %q, want %q", got.SessionID, info.SessionID)
		}
		if got.State != string(session.Active) {
			t.Errorf("got state %q, want %q", got.State, session.Active)
		}
	})

	t.Run("not found", func(t *testing.T) {
		srv, _ := setupTestServer(t)
		req := httptest.NewRequest(http.MethodPost, "/sessions/nope/start", nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("already active", func(t *testing.T) {
		srv, mgr := setupTestServer(t)
		info, _, err := mgr.Create(validConfig())
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := mgr.Start(info.SessionID); err != nil {
			t.Fatalf("start: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/sessions/"+info.SessionID+"/start", nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusConflict)
		}
	})

	t.Run("unknown game", func(t *testing.T) {
		rejectFactory := func(cfg session.Config) (session.Game, error) {
			switch cfg.Game {
			case "hearts":
				return stubGame{}, nil
			default:
				return nil, fmt.Errorf("%w: unknown game: %s", session.ErrInvalidConfig, cfg.Game)
			}
		}
		mgr := session.NewManager(rejectFactory)
		srv := NewServer(Config{Manager: mgr, Addr: ":0"})

		cfg := session.Config{
			Game:  "poker",
			Seats: []session.SeatConfig{{Type: session.SeatAI, AIType: "random"}},
		}
		info, _, err := mgr.Create(cfg)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/sessions/"+info.SessionID+"/start", nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}

// TestHandleDeleteSession verifies DELETE /sessions/{id} handler.
func TestHandleDeleteSession(t *testing.T) {
	t.Run("draft session", func(t *testing.T) {
		srv, mgr := setupTestServer(t)
		info, _, err := mgr.Create(validConfig())
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		req := httptest.NewRequest(http.MethodDelete, "/sessions/"+info.SessionID, nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusNoContent)
		}
	})

	t.Run("active session", func(t *testing.T) {
		srv, mgr := setupTestServer(t)
		info, _, err := mgr.Create(validConfig())
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := mgr.Start(info.SessionID); err != nil {
			t.Fatalf("start: %v", err)
		}

		req := httptest.NewRequest(http.MethodDelete, "/sessions/"+info.SessionID, nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusNoContent)
		}
	})

	t.Run("not found", func(t *testing.T) {
		srv, _ := setupTestServer(t)
		req := httptest.NewRequest(http.MethodDelete, "/sessions/nope", nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("already expired", func(t *testing.T) {
		srv, mgr := setupTestServer(t)
		info, _, err := mgr.Create(validConfig())
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := mgr.Delete(info.SessionID); err != nil {
			t.Fatalf("delete: %v", err)
		}

		req := httptest.NewRequest(http.MethodDelete, "/sessions/"+info.SessionID, nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}

// TestParseBearerToken verifies authorization header parsing.
func TestParseBearerToken(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		want    string
		wantErr bool
	}{
		{"missing header", "", "", true},
		{"malformed no prefix", "token123", "", true},
		{"malformed basic auth", "Basic token123", "", true},
		{"valid token", "Bearer valid-token-123", "valid-token-123", false},
		{"valid with spaces in token", "Bearer token with spaces", "token with spaces", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			got, err := parseBearerToken(req)
			if (err != nil) != tc.wantErr {
				t.Errorf("got error %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("got token %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPlayerWSUpgradeValidIntegration verifies that a player with a valid token
// can upgrade to a WebSocket and receives an initial snapshot.
func TestPlayerWSUpgradeValidIntegration(t *testing.T) {
	srv, id, token := setupTestServerWithSession(t)

	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	wsURL := fmt.Sprintf("ws%s/sessions/%s/ws", strings.TrimPrefix(ts.URL, "http"), id)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)
	conn, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatalf("dial failed: %v (status=%d)", err, resp.StatusCode)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
	}

	// Read initial snapshot.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	typ, b, err := conn.Read(ctx2)
	if err != nil {
		t.Fatalf("read initial snapshot: %v", err)
	}
	if typ != websocket.MessageText {
		t.Fatalf("got message type %d, want text", typ)
	}

	var snap map[string]any
	if err := json.Unmarshal(b, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap["type"] != "snapshot" {
		t.Errorf("got type %q, want %q", snap["type"], "snapshot")
	}
}

// TestPlayerWSUpgradeMissingTokenIntegration verifies that a player WS upgrade
// without an Authorization header returns 401.
func TestPlayerWSUpgradeMissingTokenIntegration(t *testing.T) {
	srv, id, _ := setupTestServerWithSession(t)

	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	wsURL := fmt.Sprintf("ws%s/sessions/%s/ws", strings.TrimPrefix(ts.URL, "http"), id)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
	if resp == nil {
		t.Fatal("expected HTTP response, got nil")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	var body errorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error == "" {
		t.Error("expected non-empty error message")
	}
}

// TestPlayerWSUpgradeInvalidTokenIntegration verifies that a player WS upgrade
// with a malformed or non-existent token returns 401.
func TestPlayerWSUpgradeInvalidTokenIntegration(t *testing.T) {
	srv, id, _ := setupTestServerWithSession(t)

	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	wsURL := fmt.Sprintf("ws%s/sessions/%s/ws", strings.TrimPrefix(ts.URL, "http"), id)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	headers := http.Header{}
	headers.Set("Authorization", "Bearer invalid-token")
	conn, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: headers})
	if err == nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
	if resp == nil {
		t.Fatal("expected HTTP response, got nil")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

// TestPlayerWSUpgradeSessionNotActiveIntegration verifies that a player WS
// upgrade for a session in draft state returns 409.
func TestPlayerWSUpgradeSessionNotActiveIntegration(t *testing.T) {
	mgr := mockManager()
	srv := NewServer(Config{Manager: mgr, Addr: ":0"})

	cfg := session.Config{
		Game: "hearts",
		Seats: []session.SeatConfig{
			{Type: session.SeatHuman},
		},
	}
	info, seats, err := mgr.Create(cfg)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	var token string
	for _, s := range seats {
		if s.Type == session.SeatHuman {
			token = s.Token
			break
		}
	}
	if token == "" {
		t.Fatal("no human seat token generated")
	}

	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	wsURL := fmt.Sprintf("ws%s/sessions/%s/ws", strings.TrimPrefix(ts.URL, "http"), info.SessionID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)
	conn, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: headers})
	if err == nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
	if resp == nil {
		t.Fatal("expected HTTP response, got nil")
	}
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

// TestPlayerWSUpgradeWrongSessionIntegration verifies that a token belonging to
// a different session returns 401.
func TestPlayerWSUpgradeWrongSessionIntegration(t *testing.T) {
	mgr := mockManager()
	srv := NewServer(Config{Manager: mgr, Addr: ":0"})

	// Create session A with a human seat.
	cfgA := session.Config{
		Game: "hearts",
		Seats: []session.SeatConfig{
			{Type: session.SeatHuman},
			{Type: session.SeatAI, AIType: "random"},
		},
	}
	infoA, seatsA, err := mgr.Create(cfgA)
	if err != nil {
		t.Fatalf("create session A: %v", err)
	}
	if err := mgr.Start(infoA.SessionID); err != nil {
		t.Fatalf("start session A: %v", err)
	}

	// Create session B (also started).
	cfgB := session.Config{
		Game: "hearts",
		Seats: []session.SeatConfig{
			{Type: session.SeatHuman},
			{Type: session.SeatAI, AIType: "random"},
		},
	}
	infoB, _, err := mgr.Create(cfgB)
	if err != nil {
		t.Fatalf("create session B: %v", err)
	}
	if err := mgr.Start(infoB.SessionID); err != nil {
		t.Fatalf("start session B: %v", err)
	}

	var tokenA string
	for _, s := range seatsA {
		if s.Type == session.SeatHuman {
			tokenA = s.Token
			break
		}
	}
	if tokenA == "" {
		t.Fatal("no human seat token for session A")
	}

	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	// Try to use session A's token with session B's ID.
	wsURL := fmt.Sprintf("ws%s/sessions/%s/ws", strings.TrimPrefix(ts.URL, "http"), infoB.SessionID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+tokenA)
	conn, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: headers})
	if err == nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
	if resp == nil {
		t.Fatal("expected HTTP response, got nil")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

// TestObserverWSUpgradeValidIntegration verifies that an observer can upgrade to
// a WebSocket for an active session and receives an initial snapshot.
func TestObserverWSUpgradeValidIntegration(t *testing.T) {
	srv, id, _ := setupTestServerWithSession(t)

	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	wsURL := fmt.Sprintf("ws%s/sessions/%s/ws/observe", strings.TrimPrefix(ts.URL, "http"), id)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v (status=%d)", err, resp.StatusCode)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
	}

	// Read initial snapshot.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	typ, b, err := conn.Read(ctx2)
	if err != nil {
		t.Fatalf("read initial snapshot: %v", err)
	}
	if typ != websocket.MessageText {
		t.Fatalf("got message type %d, want text", typ)
	}

	var snap map[string]any
	if err := json.Unmarshal(b, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap["type"] != "snapshot" {
		t.Errorf("got type %q, want %q", snap["type"], "snapshot")
	}
}

// TestObserverWSUpgradeSessionNotFoundIntegration verifies that an observer WS
// upgrade for a non-existent session returns 404.
func TestObserverWSUpgradeSessionNotFoundIntegration(t *testing.T) {
	srv, _ := setupTestServer(t)

	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	wsURL := fmt.Sprintf(
		"ws%s/sessions/nonexistent-id/ws/observe",
		strings.TrimPrefix(ts.URL, "http"),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
	if resp == nil {
		t.Fatal("expected HTTP response, got nil")
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// TestObserverWSUpgradeSessionNotActiveIntegration verifies that an observer WS
// upgrade for a session in draft state returns 409.
func TestObserverWSUpgradeSessionNotActiveIntegration(t *testing.T) {
	mgr := mockManager()
	srv := NewServer(Config{Manager: mgr, Addr: ":0"})

	cfg := session.Config{
		Game: "hearts",
		Seats: []session.SeatConfig{
			{Type: session.SeatAI, AIType: "random"},
		},
	}
	info, _, err := mgr.Create(cfg)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	wsURL := fmt.Sprintf(
		"ws%s/sessions/%s/ws/observe",
		strings.TrimPrefix(ts.URL, "http"),
		info.SessionID,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
	if resp == nil {
		t.Fatal("expected HTTP response, got nil")
	}
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

// TestPlayerWSNoEmptyFrameOnMarshalFailureIntegration verifies that when the
// session goroutine produces an unmarshalable snapshot, the player
// WebSocket connection does not receive an empty or nil text frame.
func TestPlayerWSNoEmptyFrameOnMarshalFailureIntegration(t *testing.T) {
	srv, id, token := setupTestServerWithUnmarshalableSession(t)

	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	wsURL := fmt.Sprintf("ws%s/sessions/%s/ws", strings.TrimPrefix(ts.URL, "http"), id)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)
	conn, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatalf("dial failed: %v (status=%d)", err, resp.StatusCode)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	// Try to read a message — should time out because the session
	// goroutine drops unmarshalable snapshots and sends nothing.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel2()
	_, _, err = conn.Read(ctx2)
	if err == nil {
		t.Fatal("expected timeout (no message), but got a message — empty/nil frame was sent")
	}
}

// TestObserverWSNoEmptyFrameOnMarshalFailureIntegration verifies that when the
// session goroutine produces an unmarshalable snapshot, the observer
// WebSocket connection does not receive an empty or nil text frame.
func TestObserverWSNoEmptyFrameOnMarshalFailureIntegration(t *testing.T) {
	srv, id, _ := setupTestServerWithUnmarshalableSession(t)

	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	wsURL := fmt.Sprintf("ws%s/sessions/%s/ws/observe", strings.TrimPrefix(ts.URL, "http"), id)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v (status=%d)", err, resp.StatusCode)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	// Try to read a message — should time out because the session
	// goroutine drops unmarshalable snapshots and sends nothing.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel2()
	_, _, err = conn.Read(ctx2)
	if err == nil {
		t.Fatal("expected timeout (no message), but got a message — empty/nil frame was sent")
	}
}

// TestServerShutdownPropagatesGoingAwayIntegration verifies that Shutdown sends
// StatusGoingAway to active WebSocket connections and deletes active
// sessions.
func TestServerShutdownPropagatesGoingAwayIntegration(t *testing.T) {
	srv, sessionID, token := setupTestServerWithSession(t)

	done := make(chan error, 1)
	go func() {
		done <- srv.Start()
	}()

	time.Sleep(50 * time.Millisecond)

	addr := srv.Addr()
	if addr == "" {
		t.Fatal("server has no address after start")
	}

	ctx := context.Background()
	wsURL := "ws://" + addr + "/sessions/" + sessionID + "/ws"

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}},
	})
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()

	shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Start() returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return after Shutdown()")
	}

	readCtx, cancelRead := context.WithTimeout(ctx, 2*time.Second)
	defer cancelRead()

	var readErr error
	for {
		_, _, readErr = conn.Read(readCtx)
		if readErr != nil {
			break
		}
	}

	status := websocket.CloseStatus(readErr)
	if status != websocket.StatusGoingAway {
		t.Errorf("got close status %d, want %d (GoingAway)", status, websocket.StatusGoingAway)
	}

	_, err = srv.mgr.Get(sessionID)
	if !errors.Is(err, session.ErrNotFound) {
		t.Errorf("got error %v, want ErrNotFound", err)
	}
}

// mockManager returns a session manager with a stubGame factory.
func mockManager() *session.Manager {
	return session.NewManager(func(_ session.Config) (session.Game, error) {
		return stubGame{}, nil
	})
}

// validConfig returns a minimal valid session.Config for tests.
func validConfig() session.Config {
	return session.Config{
		Game:  "hearts",
		Seats: []session.SeatConfig{{Type: session.SeatAI, AIType: "random"}},
	}
}

// setupTestServer creates a Server with a mock manager for handler tests.
func setupTestServer(t *testing.T) (*Server, *session.Manager) {
	t.Helper()
	mgr := mockManager()
	srv := NewServer(Config{Manager: mgr, Addr: ":0"})
	return srv, mgr
}

// HandleAction implements session.Game.
func (stubGame) HandleAction(int, *api.InboundMessage) (session.StepResult, *session.CommandError) {
	return session.StepResult{}, nil
}

// AIPlay implements session.Game.
func (stubGame) AIPlay(int) (session.StepResult, error) {
	return session.StepResult{}, nil
}

// Resume implements session.Game.
func (stubGame) Resume() (session.StepResult, error) {
	return session.StepResult{}, nil
}

// Turn implements session.Game.
func (stubGame) Turn() int { return 0 }

// PlayerSnapshot implements session.Game.
func (stubGame) PlayerSnapshot(int, int) any {
	return map[string]any{"type": "snapshot", "seq": 0}
}

// ObserverSnapshot implements session.Game.
func (stubGame) ObserverSnapshot(int) any {
	return map[string]any{"type": "snapshot", "seq": 0}
}

// HandleAction implements session.Game for unmarshalableStubGame.
func (unmarshalableStubGame) HandleAction(
	int, *api.InboundMessage,
) (session.StepResult, *session.CommandError) {
	return session.StepResult{}, nil
}

// AIPlay implements session.Game for unmarshalableStubGame.
func (unmarshalableStubGame) AIPlay(int) (session.StepResult, error) {
	return session.StepResult{}, nil
}

// Resume implements session.Game for unmarshalableStubGame.
func (unmarshalableStubGame) Resume() (session.StepResult, error) {
	return session.StepResult{}, nil
}

// Turn implements session.Game for unmarshalableStubGame.
func (unmarshalableStubGame) Turn() int { return 0 }

// PlayerSnapshot implements session.Game for unmarshalableStubGame.
func (unmarshalableStubGame) PlayerSnapshot(int, int) any {
	return struct{ Ch chan int }{Ch: make(chan int)}
}

// ObserverSnapshot implements session.Game for unmarshalableStubGame.
func (unmarshalableStubGame) ObserverSnapshot(int) any {
	return struct{ Ch chan int }{Ch: make(chan int)}
}

// setupTestServerWithSession creates a server and an active session with
// 1 human + 3 AI seats. It returns the server, session ID, and the
// human seat's bearer token.
func setupTestServerWithSession(t *testing.T) (*Server, string, string) {
	t.Helper()
	mgr := mockManager()
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
	if token == "" {
		t.Fatal("no human seat token generated")
	}

	return srv, info.SessionID, token
}

// setupTestServerWithUnmarshalableSession creates a server and an active
// session whose snapshots cannot be marshaled to JSON. It returns the
// server, session ID, and human seat token.
func setupTestServerWithUnmarshalableSession(t *testing.T) (
	*Server, string, string,
) {
	t.Helper()
	mgr := session.NewManager(func(_ session.Config) (session.Game, error) {
		return unmarshalableStubGame{}, nil
	})
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
	if token == "" {
		t.Fatal("no human seat token generated")
	}

	return srv, info.SessionID, token
}
