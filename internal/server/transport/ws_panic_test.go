package transport

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestGuardedRecoversPanic verifies that guarded recovers a panic in
// the wrapped function, logs it with the goroutine name, and returns
// normally instead of crashing the process.
func TestGuardedRecoversPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := testBufferedLogger(&buf)

	done := make(chan struct{})
	go func() {
		defer close(done)
		guarded(logger, "test-goroutine", func() {
			panic("injected fault")
		})()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("guarded goroutine did not return after panic")
	}

	if got, want := buf.String(), "connection goroutine panic"; !strings.Contains(got, want) {
		t.Errorf("got log %q, want substring %q", got, want)
	}
	if got, want := buf.String(), "test-goroutine"; !strings.Contains(got, want) {
		t.Errorf("got log %q, want goroutine name %q", got, want)
	}
	if got, want := buf.String(), "injected fault"; !strings.Contains(got, want) {
		t.Errorf("got log %q, want panic value %q", got, want)
	}
}

// TestGuardedPanicAllowsPeerCleanup verifies the production recovery
// semantics: when one guarded goroutine panics, its peer still
// completes, wg.Wait returns, and the statements after it — the
// unsubscribe and close in playerConn.run and observerConn.run — get
// to execute. The pre-fix topology could provide none of this.
func TestGuardedPanicAllowsPeerCleanup(t *testing.T) {
	var buf bytes.Buffer
	logger := testBufferedLogger(&buf)

	var wg sync.WaitGroup

	wg.Go(guarded(logger, "writer", func() {
		panic("injected writer fault")
	}))

	peerDone := make(chan struct{})
	wg.Go(guarded(logger, "reader", func() {
		time.Sleep(50 * time.Millisecond)
		close(peerDone)
	}))

	waited := make(chan struct{})
	go func() {
		wg.Wait()
		close(waited)
	}()

	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		t.Fatal("wg.Wait did not return after guarded goroutine panic")
	}

	select {
	case <-peerDone:
	default:
		t.Error("peer goroutine did not complete")
	}
	if got, want := buf.String(), "connection goroutine panic"; !strings.Contains(got, want) {
		t.Errorf("got log %q, want substring %q", got, want)
	}
}

// testBufferedLogger returns a logger writing to buf for assertions.
func testBufferedLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil))
}
