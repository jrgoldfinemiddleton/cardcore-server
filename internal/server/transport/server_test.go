package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// stubGame is a minimal Game implementation for transport tests.
type stubGame struct{}

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
func (stubGame) PlayerSnapshot(int, int) any { return nil }

// ObserverSnapshot implements session.Game.
func (stubGame) ObserverSnapshot(int) any { return nil }
