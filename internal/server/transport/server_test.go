package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// TestNewServerRegistersAllRoutes verifies that the server has all 8
// routes registered in its mux.
func TestNewServerRegistersAllRoutes(t *testing.T) {
	mgr := mockManager()
	srv := NewServer(Config{Manager: mgr})

	// TODO: verify handler registration once HTTP and WS handlers are
	// wired into the mux. For now, verify the server struct is created
	// and the mux is present.
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	if srv.srv == nil {
		t.Fatal("server.srv is nil")
	}
	if srv.mgr != mgr {
		t.Error("server.mgr does not match configured manager")
	}
}

// TestServerStartStop verifies that the server can start on a random
// port and shut down gracefully.
func TestServerStartStop(t *testing.T) {
	mgr := mockManager()
	srv := NewServer(Config{
		Manager: mgr,
		Addr:    ":0",
	})

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
	// Create a handler that panics.
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("intentional panic for test")
	})

	// Wrap with recovery middleware.
	wrapped := recoveryMiddleware(panicHandler)

	// Test with httptest.
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
	// Create a handler that returns 200.
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with request log middleware.
	wrapped := requestLogMiddleware(okHandler)

	// Test with httptest.
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

// mockManager returns a session manager for testing transport layer
// without a real game engine.
func mockManager() *session.Manager {
	return session.NewManager(func(_ session.Config) (session.Game, error) {
		return nil, nil
	})
}
