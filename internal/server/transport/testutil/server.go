package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/transport"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/testutil"
)

// SetupTestServer creates a real server with a Hearts game registry,
// starts it on an ephemeral port, and registers cleanup.
func SetupTestServer(t *testing.T) *transport.Server {
	t.Helper()
	registry := testutil.HeartsRegistry(t)
	mgr := session.NewManager(registry, session.DefaultServerDelays)
	srv := transport.NewServer(transport.Config{Manager: mgr})
	go func() {
		_ = srv.Start()
	}()
	for i := 0; i < 100 && srv.Addr() == ""; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.Addr() == "" {
		t.Fatal("server did not start listening")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return srv
}

// SetupTestServerWithManager creates a real server and its underlying session
// manager with a Hearts game registry, starts it on an ephemeral port, and
// registers cleanup.
func SetupTestServerWithManager(t *testing.T) (*transport.Server, *session.Manager) {
	t.Helper()
	registry := testutil.HeartsRegistry(t)
	mgr := session.NewManager(registry, session.DefaultServerDelays)
	srv := transport.NewServer(transport.Config{Manager: mgr})
	go func() {
		_ = srv.Start()
	}()
	for i := 0; i < 100 && srv.Addr() == ""; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.Addr() == "" {
		t.Fatal("server did not start listening")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return srv, mgr
}
