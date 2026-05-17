package session

import (
	"testing"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
)

// mockGame is a minimal Game implementation for testing Manager
// lifecycle and session goroutine behavior without a real engine.
type mockGame struct{}

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
		AIDelayMS: &delay,
	}
}
