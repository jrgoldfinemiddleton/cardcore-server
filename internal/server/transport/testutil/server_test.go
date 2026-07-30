package testutil

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// TestSetupTestServerStartsOnFreePort verifies that SetupTestServer starts a
// real HTTP server on a free port and returns a usable base URL.
func TestSetupTestServerStartsOnFreePort(t *testing.T) {
	srv := SetupTestServer(t)
	if srv == nil {
		t.Fatal("SetupTestServer() returned nil")
	}

	addr := srv.Addr()
	if addr == "" {
		t.Fatal("server has no address after SetupTestServer")
	}

	resp, err := http.Get("http://" + addr + "/sessions")
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// TestSetupTestServerShutsDownCleanly verifies that SetupTestServer registers
// a cleanup that shuts the server down without error.
func TestSetupTestServerShutsDownCleanly(t *testing.T) {
	srv := SetupTestServer(t)
	addr := srv.Addr()
	if addr == "" {
		t.Fatal("server has no address after SetupTestServer")
	}

	done := make(chan error, 1)
	go func() {
		done <- srv.Start()
	}()

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/sessions")
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	_ = resp.Body.Close()

	// Manually invoke Shutdown to verify the cleanup path works.
	// t.Cleanup will run again at test end but Shutdown is idempotent
	// on an already-shut-down server.
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	select {
	case <-shutdownDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not return within 5s")
	}

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Start() returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return after Shutdown()")
	}
}
