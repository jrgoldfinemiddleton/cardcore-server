package session

import (
	"bytes"
	"log/slog"
	"testing"
)

// TestSendNonBlockingDropsOnFullChannel verifies that sendNonBlocking does
// not block when the subscriber channel is full and instead drops the
// snapshot and logs a warning.
func TestSendNonBlockingDropsOnFullChannel(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	slog.SetDefault(slog.New(handler))

	g := &mockGame{}
	cfg := Config{Seats: []SeatConfig{{Type: SeatHuman}}}
	s := newSession("test-drop", g, cfg, DefaultDelays{}, nil)
	defer func() {
		close(s.cancel)
		<-s.done
	}()

	ch := make(chan SubscriberMessage, 1)
	ch <- SubscriberMessage{Data: []byte("first")}

	s.sendNonBlocking(ch, []byte("dropped"))

	if len(ch) != 1 {
		t.Fatalf("want channel depth 1, got %d", len(ch))
	}
	got := <-ch
	if string(got.Data) != "first" {
		t.Fatalf("got %q, want %q", string(got.Data), "first")
	}

	logs := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("subscriber channel full, snapshot dropped")) {
		t.Errorf("expected drop warning in logs, got:\n%s", logs)
	}
}
