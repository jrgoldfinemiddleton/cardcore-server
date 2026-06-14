package main

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"charm.land/bubbletea/v2"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
)

// testModel is a minimal Bubble Tea model for testing wsbridge.
type testModel struct {
	program *tea.Program
	mu      sync.Mutex
	msgs    []tea.Msg
}

// mockWSReader implements WSReader for testing.
type mockWSReader struct {
	msgs []json.RawMessage
	err  error
	idx  int
}

// TestStartWSReaderSnapshot sends a snapshot and verifies it is received.
func TestStartWSReaderSnapshot(t *testing.T) {
	snapshot := json.RawMessage(`{"type":"snapshot","seq":1,"phase":"passing"}`)
	mock := &mockWSReader{msgs: []json.RawMessage{snapshot}}

	var in bytes.Buffer
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	m := &testModel{}
	p := tea.NewProgram(m,
		tea.WithContext(ctx),
		tea.WithInput(&in),
		tea.WithoutRenderer(),
	)
	m.program = p

	done := make(chan struct{})
	go func() {
		_, _ = p.Run()
		close(done)
	}()
	defer func() {
		p.Quit()
		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatal("bubbletea test program did not exit in time")
		}
	}()

	// Give the program time to start its event loop.
	time.Sleep(50 * time.Millisecond)
	go startWSReader(context.Background(), mock, p)

	msg := m.waitForMsg(t)
	if _, ok := msg.(wsSnapshotMsg); !ok {
		t.Fatalf("got %T, want wsSnapshotMsg", msg)
	}
}

// TestStartWSReaderError sends an error message and verifies it is received.
func TestStartWSReaderError(t *testing.T) {
	errMsg := json.RawMessage(`{"type":"error","code":"out_of_turn","message":"Not your turn"}`)
	mock := &mockWSReader{msgs: []json.RawMessage{errMsg}}

	var in bytes.Buffer
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	m := &testModel{}
	p := tea.NewProgram(m,
		tea.WithContext(ctx),
		tea.WithInput(&in),
		tea.WithoutRenderer(),
	)
	m.program = p

	done := make(chan struct{})
	go func() {
		_, _ = p.Run()
		close(done)
	}()
	defer func() {
		p.Quit()
		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatal("bubbletea test program did not exit in time")
		}
	}()

	// Give the program time to start its event loop.
	time.Sleep(50 * time.Millisecond)
	go startWSReader(context.Background(), mock, p)

	msg := m.waitForMsg(t)
	em, ok := msg.(wsErrorMsg)
	if !ok {
		t.Fatalf("got %T, want wsErrorMsg", msg)
	}
	if em.code != "out_of_turn" {
		t.Errorf("error code = %q, want out_of_turn", em.code)
	}
}

// TestStartWSReaderClose verifies close code 1000 is sent on connection close.
func TestStartWSReaderClose(t *testing.T) {
	mock := &mockWSReader{err: &client.ConnectionClosedError{Code: 1000, Reason: "normal"}}

	var in bytes.Buffer
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	m := &testModel{}
	p := tea.NewProgram(m,
		tea.WithContext(ctx),
		tea.WithInput(&in),
		tea.WithoutRenderer(),
	)
	m.program = p

	done := make(chan struct{})
	go func() {
		_, _ = p.Run()
		close(done)
	}()
	defer func() {
		p.Quit()
		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatal("bubbletea test program did not exit in time")
		}
	}()

	// Give the program time to start its event loop.
	time.Sleep(50 * time.Millisecond)
	go startWSReader(context.Background(), mock, p)

	msg := m.waitForMsg(t)
	cm, ok := msg.(wsCloseMsg)
	if !ok {
		t.Fatalf("got %T, want wsCloseMsg", msg)
	}
	if cm.code != 1000 {
		t.Errorf("close code = %d, want 1000", cm.code)
	}
}

// Init implements tea.Model.
func (m *testModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model. It captures all messages for test inspection.
func (m *testModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.mu.Lock()
	m.msgs = append(m.msgs, msg)
	m.mu.Unlock()
	return m, nil
}

// View implements tea.Model.
func (m *testModel) View() tea.View {
	v := tea.NewView("test")
	v.AltScreen = true
	return v
}

// waitForMsg waits for the first non-system message from the goroutine.
func (m *testModel) waitForMsg(t *testing.T) tea.Msg {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		// Wait for at least one message.
		for {
			m.mu.Lock()
			length := len(m.msgs)
			m.mu.Unlock()
			if length > 0 {
				break
			}
			select {
			case <-deadline:
				t.Fatal("timeout waiting for message")
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}
		// Skip Bubble Tea system messages.
		m.mu.Lock()
		for len(m.msgs) > 0 {
			msg := m.msgs[0]
			m.msgs = m.msgs[1:]
			switch msg.(type) {
			case tea.ColorProfileMsg, tea.WindowSizeMsg, tea.EnvMsg:
				continue
			}
			m.mu.Unlock()
			return msg
		}
		m.mu.Unlock()
		// All messages were system messages, wait for more.
		select {
		case <-deadline:
			t.Fatal("timeout waiting for message")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// ReadSnapshot implements WSReader for testing.
func (m *mockWSReader) ReadSnapshot(ctx context.Context) (json.RawMessage, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.idx >= len(m.msgs) {
		return nil, &client.ConnectionClosedError{Code: 1000, Reason: "normal closure"}
	}
	msg := m.msgs[m.idx]
	m.idx++
	return msg, nil
}
