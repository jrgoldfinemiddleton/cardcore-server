package session

import (
	"flag"
	"math/rand/v2"
	"testing"
	"time"
)

// testGameConfig is a minimal GameConfig implementation that always
// creates the same game type. It is used by test helpers to build a
// registry without pulling in a real game engine.
type testGameConfig struct {
	name           string
	newGame        func() Game
	validateConfig func(Config) error
}

// Name returns the configured name so the registry can look up this
// config by the same name used in session Config.Game.
func (c *testGameConfig) Name() string { return c.name }

// RegisterFlags is a no-op; test configs are created programmatically and
// take no flags.
func (c *testGameConfig) RegisterFlags(*flag.FlagSet) {}

// Validate is a no-op; test configs have no flag values to check.
func (c *testGameConfig) Validate() error { return nil }

// NewGame calls the test's newGame closure, which returns a mock Game —
// no real engine is created, keeping session-layer tests fast and
// isolated.
func (c *testGameConfig) NewGame(_ Config, _ *rand.Rand) (Game, error) {
	return c.newGame(), nil
}

// ValidateConfig calls the test's validateConfig closure when one is set
// and otherwise accepts any configuration, so tests that do not exercise
// game-specific validation need no extra wiring.
func (c *testGameConfig) ValidateConfig(cfg Config) error {
	if c.validateConfig != nil {
		return c.validateConfig(cfg)
	}
	return nil
}

// mockGameRegistry returns a registry containing a mock game registered
// under the Hearts game name.
func mockGameRegistry() *Registry {
	r := NewRegistry()
	r.Register(&testGameConfig{name: "hearts", newGame: func() Game { return &mockGame{} }})
	return r
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
		AIActionDelayMS: &delay,
	}
}

// stepFinishedGameRegistry returns a registry containing a game that
// immediately finishes on any action.
func stepFinishedGameRegistry() *Registry {
	r := NewRegistry()
	r.Register(&testGameConfig{name: "hearts", newGame: func() Game { return &stepFinishedGame{} }})
	return r
}

// unmarshalableGameRegistry returns a registry containing a game whose
// snapshots cannot be marshaled to JSON.
func unmarshalableGameRegistry() *Registry {
	r := NewRegistry()
	r.Register(&testGameConfig{
		name:    "hearts",
		newGame: func() Game { return &unmarshalableGame{} },
	})
	return r
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
