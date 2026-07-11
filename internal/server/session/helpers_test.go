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

// timeoutGame is a mock Game that stays on a specific seat's turn for
// testing turn timeout behavior. After AIPlay, the turn advances to the
// next seat (modulo seatCount) to prevent infinite timeout loops.
type timeoutGame struct {
	turnSeat  int
	seatCount int
}

// aiPlayFinishedGame is a mock Game where AIPlay returns StepFinished.
type aiPlayFinishedGame struct {
	turnSeat int
}

// invalidTurnGame is a mock Game where Turn returns an invalid seat.
type invalidTurnGame struct{}

// aiPlayPauseGame is a mock Game where the first turn is seat 0 (human),
// AIPlay returns StepPause on the first call then StepFinished, and
// Resume advances the turn to seat 1 so resumePauses chains through.
type aiPlayPauseGame struct {
	callCount int
	turnSeat  int
}

// delayGame is a mock Game that returns a fixed non-zero DisplayDelay for
// verifying the goroutine sleep paths.
type delayGame struct {
	delay int
}

// seqSnapshotGame is a mock Game that returns snapshots embedding the
// seq value so tests can verify the sequence number in the wire format.
type seqSnapshotGame struct{}

// HandleAction implements Game.HandleAction for seqSnapshotGame.
func (seqSnapshotGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{}, nil
}

// AIPlay implements Game.AIPlay for seqSnapshotGame.
func (seqSnapshotGame) AIPlay(int) (StepResult, error) {
	return StepResult{}, nil
}

// Resume implements Game.Resume for seqSnapshotGame.
func (seqSnapshotGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn implements Game.Turn for seqSnapshotGame.
func (seqSnapshotGame) Turn() int { return 0 }

// PlayerSnapshot implements Game.PlayerSnapshot for seqSnapshotGame.
func (seqSnapshotGame) PlayerSnapshot(seat, seq int) any {
	return map[string]any{"seq": seq}
}

// ObserverSnapshot implements Game.ObserverSnapshot for seqSnapshotGame.
func (seqSnapshotGame) ObserverSnapshot(seq int) any {
	return map[string]any{"seq": seq}
}

// DisplayDelay implements Game.DisplayDelay for seqSnapshotGame.
func (seqSnapshotGame) DisplayDelay() int { return 0 }

// SetTurnDeadline implements Game.SetTurnDeadline for seqSnapshotGame.
func (seqSnapshotGame) SetTurnDeadline(time.Time) {}

// TurnDeadline implements Game.TurnDeadline for seqSnapshotGame.
func (seqSnapshotGame) TurnDeadline() time.Time { return time.Time{} }

// HandleAction implements Game.HandleAction for aiPlayPauseGame.
func (a *aiPlayPauseGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	a.turnSeat = 1
	return StepResult{Outcome: StepContinue}, nil
}

// AIPlay implements Game.AIPlay for aiPlayPauseGame.
func (a *aiPlayPauseGame) AIPlay(int) (StepResult, error) {
	a.callCount++
	if a.callCount == 1 {
		return StepResult{Outcome: StepPause}, nil
	}
	return StepResult{Outcome: StepFinished}, nil
}

// Resume implements Game.Resume for aiPlayPauseGame.
func (a *aiPlayPauseGame) Resume() (StepResult, error) {
	return StepResult{Outcome: StepContinue}, nil
}

// Turn implements Game.Turn for aiPlayPauseGame.
func (a *aiPlayPauseGame) Turn() int {
	return a.turnSeat
}

// PlayerSnapshot implements Game.PlayerSnapshot for aiPlayPauseGame.
func (a *aiPlayPauseGame) PlayerSnapshot(int, int) any {
	return nil
}

// ObserverSnapshot implements Game.ObserverSnapshot for aiPlayPauseGame.
func (a *aiPlayPauseGame) ObserverSnapshot(int) any {
	return nil
}

// DisplayDelay implements Game.DisplayDelay for aiPlayPauseGame.
func (a *aiPlayPauseGame) DisplayDelay() int { return 0 }

// SetTurnDeadline implements Game.SetTurnDeadline for aiPlayPauseGame.
func (a *aiPlayPauseGame) SetTurnDeadline(time.Time) {}

// TurnDeadline implements Game.TurnDeadline for aiPlayPauseGame.
func (a *aiPlayPauseGame) TurnDeadline() time.Time { return time.Time{} }

// HandleAction implements Game.HandleAction for delayGame.
func (d *delayGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{}, nil
}

// AIPlay implements Game.AIPlay for delayGame.
func (d *delayGame) AIPlay(int) (StepResult, error) {
	return StepResult{Outcome: StepFinished}, nil
}

// Resume implements Game.Resume for delayGame.
func (d *delayGame) Resume() (StepResult, error) {
	return StepResult{Outcome: StepContinue}, nil
}

// Turn implements Game.Turn for delayGame.
func (d *delayGame) Turn() int { return 0 }

// PlayerSnapshot implements Game.PlayerSnapshot for delayGame.
func (d *delayGame) PlayerSnapshot(int, int) any { return nil }

// ObserverSnapshot implements Game.ObserverSnapshot for delayGame.
func (d *delayGame) ObserverSnapshot(int) any { return nil }

// DisplayDelay implements Game.DisplayDelay for delayGame.
func (d *delayGame) DisplayDelay() int { return d.delay }

// SetTurnDeadline implements Game.SetTurnDeadline for delayGame.
func (d *delayGame) SetTurnDeadline(time.Time) {}

// TurnDeadline implements Game.TurnDeadline for delayGame.
func (d *delayGame) TurnDeadline() time.Time { return time.Time{} }

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

// DisplayDelay implements Game.DisplayDelay for mockGame.
func (m *mockGame) DisplayDelay() int { return 0 }

// SetTurnDeadline implements Game.SetTurnDeadline for mockGame.
func (m *mockGame) SetTurnDeadline(time.Time) {}

// TurnDeadline implements Game.TurnDeadline for mockGame.
func (m *mockGame) TurnDeadline() time.Time { return time.Time{} }

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

// DisplayDelay implements Game.DisplayDelay for stepFinishedGame.
func (s *stepFinishedGame) DisplayDelay() int { return 0 }

// SetTurnDeadline implements Game.SetTurnDeadline for stepFinishedGame.
func (s *stepFinishedGame) SetTurnDeadline(time.Time) {}

// TurnDeadline implements Game.TurnDeadline for stepFinishedGame.
func (s *stepFinishedGame) TurnDeadline() time.Time { return time.Time{} }

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

// DisplayDelay implements Game.DisplayDelay for unmarshalableGame.
func (u *unmarshalableGame) DisplayDelay() int { return 0 }

// SetTurnDeadline implements Game.SetTurnDeadline for unmarshalableGame.
func (u *unmarshalableGame) SetTurnDeadline(time.Time) {}

// TurnDeadline implements Game.TurnDeadline for unmarshalableGame.
func (u *unmarshalableGame) TurnDeadline() time.Time { return time.Time{} }

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

// DisplayDelay implements Game.DisplayDelay for playerSnapshotUnmarshalableGame.
func (p *playerSnapshotUnmarshalableGame) DisplayDelay() int { return 0 }

// SetTurnDeadline implements Game.SetTurnDeadline for playerSnapshotUnmarshalableGame.
func (p *playerSnapshotUnmarshalableGame) SetTurnDeadline(time.Time) {}

// TurnDeadline implements Game.TurnDeadline for playerSnapshotUnmarshalableGame.
func (p *playerSnapshotUnmarshalableGame) TurnDeadline() time.Time { return time.Time{} }

// HandleAction implements Game.HandleAction for timeoutGame.
func (g *timeoutGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{Outcome: StepContinue}, nil
}

// AIPlay implements Game.AIPlay for timeoutGame.
func (g *timeoutGame) AIPlay(int) (StepResult, error) {
	g.turnSeat = (g.turnSeat + 1) % g.seatCount
	return StepResult{Outcome: StepContinue}, nil
}

// Resume implements Game.Resume for timeoutGame.
func (g *timeoutGame) Resume() (StepResult, error) {
	return StepResult{Outcome: StepContinue}, nil
}

// Turn implements Game.Turn for timeoutGame.
func (g *timeoutGame) Turn() int {
	return g.turnSeat
}

// PlayerSnapshot implements Game.PlayerSnapshot for timeoutGame.
func (g *timeoutGame) PlayerSnapshot(int, int) any {
	return nil
}

// ObserverSnapshot implements Game.ObserverSnapshot for timeoutGame.
func (g *timeoutGame) ObserverSnapshot(int) any {
	return nil
}

// DisplayDelay implements Game.DisplayDelay for timeoutGame.
func (g *timeoutGame) DisplayDelay() int { return 0 }

// SetTurnDeadline implements Game.SetTurnDeadline for timeoutGame.
func (g *timeoutGame) SetTurnDeadline(time.Time) {}

// TurnDeadline implements Game.TurnDeadline for timeoutGame.
func (g *timeoutGame) TurnDeadline() time.Time { return time.Time{} }

// HandleAction implements Game.HandleAction for aiPlayFinishedGame.
func (a *aiPlayFinishedGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	a.turnSeat = 1
	return StepResult{Outcome: StepContinue}, nil
}

// AIPlay implements Game.AIPlay for aiPlayFinishedGame.
func (a *aiPlayFinishedGame) AIPlay(int) (StepResult, error) {
	return StepResult{Outcome: StepFinished}, nil
}

// Resume implements Game.Resume for aiPlayFinishedGame.
func (a *aiPlayFinishedGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn implements Game.Turn for aiPlayFinishedGame.
func (a *aiPlayFinishedGame) Turn() int {
	return a.turnSeat
}

// PlayerSnapshot implements Game.PlayerSnapshot for aiPlayFinishedGame.
func (a *aiPlayFinishedGame) PlayerSnapshot(int, int) any {
	return nil
}

// ObserverSnapshot implements Game.ObserverSnapshot for aiPlayFinishedGame.
func (a *aiPlayFinishedGame) ObserverSnapshot(int) any {
	return nil
}

// DisplayDelay implements Game.DisplayDelay for aiPlayFinishedGame.
func (a *aiPlayFinishedGame) DisplayDelay() int { return 0 }

// SetTurnDeadline implements Game.SetTurnDeadline for aiPlayFinishedGame.
func (a *aiPlayFinishedGame) SetTurnDeadline(time.Time) {}

// TurnDeadline implements Game.TurnDeadline for aiPlayFinishedGame.
func (a *aiPlayFinishedGame) TurnDeadline() time.Time { return time.Time{} }

// HandleAction implements Game.HandleAction for invalidTurnGame.
func (i *invalidTurnGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{Outcome: StepContinue}, nil
}

// AIPlay implements Game.AIPlay for invalidTurnGame.
func (i *invalidTurnGame) AIPlay(int) (StepResult, error) {
	return StepResult{}, nil
}

// Resume implements Game.Resume for invalidTurnGame.
func (i *invalidTurnGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn implements Game.Turn for invalidTurnGame.
func (i *invalidTurnGame) Turn() int {
	return -1
}

// PlayerSnapshot implements Game.PlayerSnapshot for invalidTurnGame.
func (i *invalidTurnGame) PlayerSnapshot(int, int) any {
	return nil
}

// ObserverSnapshot implements Game.ObserverSnapshot for invalidTurnGame.
func (i *invalidTurnGame) ObserverSnapshot(int) any {
	return nil
}

// DisplayDelay implements Game.DisplayDelay for invalidTurnGame.
func (i *invalidTurnGame) DisplayDelay() int { return 0 }

// SetTurnDeadline implements Game.SetTurnDeadline for invalidTurnGame.
func (i *invalidTurnGame) SetTurnDeadline(time.Time) {}

// TurnDeadline implements Game.TurnDeadline for invalidTurnGame.
func (i *invalidTurnGame) TurnDeadline() time.Time { return time.Time{} }

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
		AIActionDelayMS: &delay,
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
