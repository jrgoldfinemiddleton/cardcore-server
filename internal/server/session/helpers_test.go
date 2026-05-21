package session

import (
	"testing"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
)

// mockGame is a minimal Game implementation for testing Manager
// lifecycle and session goroutine behavior without a real engine.
type mockGame struct{}

// stepFinishedGame is a mock Game that always returns StepFinished.
type stepFinishedGame struct{}

// unmarshalableGame is a mock Game whose snapshots contain types that
// json.Marshal cannot serialize (e.g., channels).
type unmarshalableGame struct{}

// playerSnapshotUnmarshalableGame is a mock Game whose player snapshots
// fail to marshal but observer snapshots succeed.
type playerSnapshotUnmarshalableGame struct{}

// HandleAction implements Game.HandleAction for mockGame.
func (m *mockGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{}, nil
}

// AIPlay implements Game.AIPlay for mockGame.
func (m *mockGame) AIPlay(int) (StepResult, error) {
	return StepResult{}, nil
}

// Resume implements Game.Resume for mockGame.
func (m *mockGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn implements Game.Turn for mockGame.
func (m *mockGame) Turn() int {
	return 0
}

// PlayerSnapshot implements Game.PlayerSnapshot for mockGame.
func (m *mockGame) PlayerSnapshot(int, int) any {
	return nil
}

// ObserverSnapshot implements Game.ObserverSnapshot for mockGame.
func (m *mockGame) ObserverSnapshot(int) any {
	return nil
}

// HandleAction implements Game.HandleAction for stepFinishedGame.
func (s *stepFinishedGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{Outcome: StepFinished}, nil
}

// AIPlay implements Game.AIPlay for stepFinishedGame.
func (s *stepFinishedGame) AIPlay(int) (StepResult, error) {
	return StepResult{}, nil
}

// Resume implements Game.Resume for stepFinishedGame.
func (s *stepFinishedGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn implements Game.Turn for stepFinishedGame.
func (s *stepFinishedGame) Turn() int {
	return 0
}

// PlayerSnapshot implements Game.PlayerSnapshot for stepFinishedGame.
func (s *stepFinishedGame) PlayerSnapshot(int, int) any {
	return nil
}

// ObserverSnapshot implements Game.ObserverSnapshot for stepFinishedGame.
func (s *stepFinishedGame) ObserverSnapshot(int) any {
	return nil
}

// HandleAction implements Game.HandleAction for unmarshalableGame.
func (u *unmarshalableGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{}, nil
}

// AIPlay implements Game.AIPlay for unmarshalableGame.
func (u *unmarshalableGame) AIPlay(int) (StepResult, error) {
	return StepResult{}, nil
}

// Resume implements Game.Resume for unmarshalableGame.
func (u *unmarshalableGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn implements Game.Turn for unmarshalableGame.
func (u *unmarshalableGame) Turn() int {
	return 0
}

// PlayerSnapshot implements Game.PlayerSnapshot for unmarshalableGame.
func (u *unmarshalableGame) PlayerSnapshot(int, int) any {
	return struct{ Ch chan int }{Ch: make(chan int)}
}

// ObserverSnapshot implements Game.ObserverSnapshot for unmarshalableGame.
func (u *unmarshalableGame) ObserverSnapshot(int) any {
	return struct{ Ch chan int }{Ch: make(chan int)}
}

// HandleAction implements Game.HandleAction for playerSnapshotUnmarshalableGame.
func (p *playerSnapshotUnmarshalableGame) HandleAction(
	int, *api.InboundMessage,
) (StepResult, *CommandError) {
	return StepResult{}, nil
}

// AIPlay implements Game.AIPlay for playerSnapshotUnmarshalableGame.
func (p *playerSnapshotUnmarshalableGame) AIPlay(int) (StepResult, error) {
	return StepResult{}, nil
}

// Resume implements Game.Resume for playerSnapshotUnmarshalableGame.
func (p *playerSnapshotUnmarshalableGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn implements Game.Turn for playerSnapshotUnmarshalableGame.
func (p *playerSnapshotUnmarshalableGame) Turn() int {
	return 0
}

// PlayerSnapshot implements Game.PlayerSnapshot for playerSnapshotUnmarshalableGame.
func (p *playerSnapshotUnmarshalableGame) PlayerSnapshot(int, int) any {
	return struct{ Ch chan int }{Ch: make(chan int)}
}

// ObserverSnapshot implements Game.ObserverSnapshot for playerSnapshotUnmarshalableGame.
func (p *playerSnapshotUnmarshalableGame) ObserverSnapshot(int) any {
	return map[string]any{"type": "snapshot"}
}

// mockGameFactory returns a game factory for tests that don't need a
// real game engine.
func mockGameFactory() func(Config) (Game, error) {
	return func(Config) (Game, error) {
		return &mockGame{}, nil
	}
}

// mustCreateAndStart creates a session with cfg and transitions it to
// active, failing the test on any error. It returns the session ID.
func mustCreateAndStart(t *testing.T, m *Manager, cfg Config) string {
	t.Helper()
	info, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID
	if err := m.Start(id); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	return id
}

// validHeartsCfg returns a realistic 4-seat Hearts config for tests.
func validHeartsCfg() Config {
	delay := 0
	return Config{
		Game: "hearts",
		Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		},
		PacingDelayMS: &delay,
	}
}

// stepFinishedGameFactory returns a game factory that creates games which
// immediately finish on any action.
func stepFinishedGameFactory() func(Config) (Game, error) {
	return func(Config) (Game, error) {
		return &stepFinishedGame{}, nil
	}
}

// unmarshalableGameFactory returns a game factory that creates games whose
// snapshots cannot be marshaled to JSON.
func unmarshalableGameFactory() func(Config) (Game, error) {
	return func(Config) (Game, error) {
		return &unmarshalableGame{}, nil
	}
}

// waitForFinished polls Get until the session reaches Finished state or
// the timeout expires.
func waitForFinished(t *testing.T, m *Manager, id string) {
	t.Helper()
	for range 100 {
		info, err := m.Get(id)
		if err != nil {
			t.Fatalf("Get() error: %v", err)
		}
		if info.State == Finished {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("session did not reach finished state")
}
