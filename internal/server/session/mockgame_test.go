package session

import (
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

// deadlineBroadcastGame is a mock Game for verifying that the session
// broadcasts a snapshot after setting the turn deadline. The snapshot
// includes the deadline so tests can verify the client receives it.
type deadlineBroadcastGame struct {
	deadline time.Time
}

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

// HandleAction accepts every action and returns StepContinue; the seq test
// exercises snapshot numbering, not action handling.
func (seqSnapshotGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{}, nil
}

// AIPlay returns StepContinue without simulating a move; the seq test runs
// a single human seat, so no AI turn is driven.
func (seqSnapshotGame) AIPlay(int) (StepResult, error) {
	return StepResult{}, nil
}

// Resume returns StepContinue; seqSnapshotGame never enters a pausable
// state.
func (seqSnapshotGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn always returns seat 0; the seq test uses a single-seat game, so turn
// order never matters.
func (seqSnapshotGame) Turn() int { return 0 }

// PlayerSnapshot returns a map embedding the seq value so the test can
// unmarshal the wire snapshot and verify the session stamps the sequence
// number onto it.
func (seqSnapshotGame) PlayerSnapshot(_, seq int) any {
	return map[string]any{"seq": seq}
}

// ObserverSnapshot returns a map embedding the seq value so observer
// snapshots carry the same verifiable sequence number as player snapshots.
func (seqSnapshotGame) ObserverSnapshot(seq int) any {
	return map[string]any{"seq": seq}
}

// DisplayDelay returns zero; the seq test does not exercise UX pacing.
func (seqSnapshotGame) DisplayDelay() int { return 0 }

// SetTurnDeadline is a no-op; seqSnapshotGame does not track turn
// deadlines.
func (seqSnapshotGame) SetTurnDeadline(time.Time) {}

// SetPaused is a no-op; seqSnapshotGame does not track pause state.
func (seqSnapshotGame) SetPaused(bool) {}

// HandleAction accepts the human play, returns StepContinue, and moves the
// turn to seat 1 so driveTurns schedules the AI seat next.
func (a *aiPlayPauseGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	a.turnSeat = 1
	return StepResult{Outcome: StepContinue}, nil
}

// AIPlay returns StepPause on the first call and StepFinished on subsequent
// calls, simulating a game that pauses once then ends.
func (a *aiPlayPauseGame) AIPlay(int) (StepResult, error) {
	a.callCount++
	if a.callCount == 1 {
		return StepResult{Outcome: StepPause}, nil
	}
	return StepResult{Outcome: StepFinished}, nil
}

// Resume returns StepContinue so resumePauses chains back into turn driving
// after the single pause.
func (a *aiPlayPauseGame) Resume() (StepResult, error) {
	return StepResult{Outcome: StepContinue}, nil
}

// Turn returns the turnSeat field, which HandleAction sets to the AI seat
// so the pause sequence plays out on the AI turn.
func (a *aiPlayPauseGame) Turn() int {
	return a.turnSeat
}

// PlayerSnapshot returns nil, which marshals to null; the test asserts on
// seq counts and session finish, not snapshot content.
func (a *aiPlayPauseGame) PlayerSnapshot(int, int) any {
	return nil
}

// ObserverSnapshot returns nil, which marshals to null; the pause test
// checks subscriber close codes rather than observer snapshot content.
func (a *aiPlayPauseGame) ObserverSnapshot(int) any {
	return nil
}

// DisplayDelay returns zero so the test's pacing comes only from the
// session's configured AI action delay.
func (a *aiPlayPauseGame) DisplayDelay() int { return 0 }

// SetTurnDeadline is a no-op; aiPlayPauseGame does not track turn
// deadlines.
func (a *aiPlayPauseGame) SetTurnDeadline(time.Time) {}

// SetPaused is a no-op; aiPlayPauseGame does not track external pause
// state.
func (a *aiPlayPauseGame) SetPaused(bool) {}

// HandleAction accepts every action and returns StepContinue; the delay
// test exercises only the initial display-delay sleep.
func (d *delayGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{}, nil
}

// AIPlay returns StepFinished so the session exits immediately after the
// initial display delay elapses.
func (d *delayGame) AIPlay(int) (StepResult, error) {
	return StepResult{Outcome: StepFinished}, nil
}

// Resume returns StepContinue; delayGame never enters a pausable state.
func (d *delayGame) Resume() (StepResult, error) {
	return StepResult{Outcome: StepContinue}, nil
}

// Turn always returns seat 0; the delay test configures a single seat.
func (d *delayGame) Turn() int { return 0 }

// PlayerSnapshot returns nil, which marshals to null; the delay test
// measures elapsed time, not snapshot content.
func (d *delayGame) PlayerSnapshot(int, int) any { return nil }

// ObserverSnapshot returns nil, which marshals to null; the delay test has
// no observer subscribers.
func (d *delayGame) ObserverSnapshot(int) any { return nil }

// DisplayDelay returns the configured delay so the test can verify the
// goroutine sleeps before advancing past the initial state.
func (d *delayGame) DisplayDelay() int { return d.delay }

// SetTurnDeadline is a no-op; delayGame does not track turn deadlines.
func (d *delayGame) SetTurnDeadline(time.Time) {}

// SetPaused is a no-op; delayGame does not track pause state.
func (d *delayGame) SetPaused(bool) {}

// HandleAction accepts every action and returns StepContinue; mockGame
// only satisfies the Game interface for session lifecycle tests and does
// not model play.
func (m *mockGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{}, nil
}

// AIPlay returns StepContinue without simulating a move; tests using
// mockGame do not exercise AI turns.
func (m *mockGame) AIPlay(int) (StepResult, error) {
	return StepResult{}, nil
}

// Resume returns StepContinue; mockGame never enters a pausable state.
func (m *mockGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn always returns seat 0; tests using mockGame configure seat 0 as the
// only relevant seat.
func (m *mockGame) Turn() int {
	return 0
}

// PlayerSnapshot returns nil, which marshals to null; tests using mockGame
// assert on session state rather than snapshot content.
func (m *mockGame) PlayerSnapshot(int, int) any {
	return nil
}

// ObserverSnapshot returns nil, which marshals to null; tests using
// mockGame do not inspect observer snapshot content.
func (m *mockGame) ObserverSnapshot(int) any {
	return nil
}

// DisplayDelay returns zero so tests are not slowed by UX pacing.
func (m *mockGame) DisplayDelay() int { return 0 }

// SetTurnDeadline is a no-op; mockGame does not track turn deadlines.
func (m *mockGame) SetTurnDeadline(time.Time) {}

// SetPaused is a no-op; mockGame does not track pause state.
func (m *mockGame) SetPaused(bool) {}

// HandleAction accepts the action and returns StepFinished so a single
// play drives the session to game over.
func (s *stepFinishedGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{Outcome: StepFinished}, nil
}

// AIPlay returns StepContinue; the tests end the game through HandleAction
// on a human seat, so AIPlay is never exercised.
func (s *stepFinishedGame) AIPlay(int) (StepResult, error) {
	return StepResult{}, nil
}

// Resume returns StepContinue; stepFinishedGame never enters a pausable
// state.
func (s *stepFinishedGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn always returns seat 0; the game ends on the first action, so turn
// order is irrelevant.
func (s *stepFinishedGame) Turn() int {
	return 0
}

// PlayerSnapshot returns nil, which marshals to null; the tests assert on
// channel closure and goroutine exit, not snapshot content.
func (s *stepFinishedGame) PlayerSnapshot(int, int) any {
	return nil
}

// ObserverSnapshot returns nil, which marshals to null; the tests check
// that observer channels close rather than what they receive.
func (s *stepFinishedGame) ObserverSnapshot(int) any {
	return nil
}

// DisplayDelay returns zero; the game-over tests do not exercise UX pacing.
func (s *stepFinishedGame) DisplayDelay() int { return 0 }

// SetTurnDeadline is a no-op; stepFinishedGame does not track turn
// deadlines.
func (s *stepFinishedGame) SetTurnDeadline(time.Time) {}

// SetPaused is a no-op; stepFinishedGame does not track pause state.
func (s *stepFinishedGame) SetPaused(bool) {}

// HandleAction accepts every action and returns StepContinue; the marshal
// failure under test happens at snapshot time, not action time.
func (u *unmarshalableGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{}, nil
}

// AIPlay returns StepContinue; the session terminates on the initial
// snapshot broadcast before any AI turn is driven.
func (u *unmarshalableGame) AIPlay(int) (StepResult, error) {
	return StepResult{}, nil
}

// Resume returns StepContinue; unmarshalableGame never enters a pausable
// state.
func (u *unmarshalableGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn always returns seat 0; the test configures a single human seat.
func (u *unmarshalableGame) Turn() int {
	return 0
}

// PlayerSnapshot returns a struct containing a channel, which json.Marshal
// cannot serialize, so every player snapshot triggers a marshal failure.
func (u *unmarshalableGame) PlayerSnapshot(int, int) any {
	return struct{ Ch chan int }{Ch: make(chan int)}
}

// ObserverSnapshot returns a struct containing a channel, which
// json.Marshal cannot serialize, so every observer snapshot triggers a
// marshal failure.
func (u *unmarshalableGame) ObserverSnapshot(int) any {
	return struct{ Ch chan int }{Ch: make(chan int)}
}

// DisplayDelay returns zero; the marshal-failure test does not exercise UX
// pacing.
func (u *unmarshalableGame) DisplayDelay() int { return 0 }

// SetTurnDeadline is a no-op; unmarshalableGame does not track turn
// deadlines.
func (u *unmarshalableGame) SetTurnDeadline(time.Time) {}

// SetPaused is a no-op; unmarshalableGame does not track pause state.
func (u *unmarshalableGame) SetPaused(bool) {}

// HandleAction accepts every action and returns StepContinue; the tests
// exercise snapshot marshaling, not action handling.
func (p *playerSnapshotUnmarshalableGame) HandleAction(
	int, *api.InboundMessage,
) (StepResult, *CommandError) {
	return StepResult{}, nil
}

// AIPlay returns StepContinue; the tests drive the session through
// subscription and stale-seq commands rather than AI turns.
func (p *playerSnapshotUnmarshalableGame) AIPlay(int) (StepResult, error) {
	return StepResult{}, nil
}

// Resume returns StepContinue; playerSnapshotUnmarshalableGame never enters
// a pausable state.
func (p *playerSnapshotUnmarshalableGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn always returns seat 0; the tests configure a single human seat.
func (p *playerSnapshotUnmarshalableGame) Turn() int {
	return 0
}

// PlayerSnapshot returns a struct containing a channel so player snapshots
// fail to marshal, isolating the player-marshal failure path while observer
// snapshots still succeed.
func (p *playerSnapshotUnmarshalableGame) PlayerSnapshot(int, int) any {
	return struct{ Ch chan int }{Ch: make(chan int)}
}

// ObserverSnapshot returns a marshalable map so observer subscribers
// receive a normal snapshot while the player marshal failure is exercised.
func (p *playerSnapshotUnmarshalableGame) ObserverSnapshot(int) any {
	return map[string]any{"type": "snapshot"}
}

// DisplayDelay returns zero; the marshal-failure tests do not exercise UX
// pacing.
func (p *playerSnapshotUnmarshalableGame) DisplayDelay() int { return 0 }

// SetTurnDeadline is a no-op; playerSnapshotUnmarshalableGame does not
// track turn deadlines.
func (p *playerSnapshotUnmarshalableGame) SetTurnDeadline(time.Time) {}

// SetPaused is a no-op; playerSnapshotUnmarshalableGame does not track
// pause state.
func (p *playerSnapshotUnmarshalableGame) SetPaused(bool) {}

// HandleAction accepts every action and returns StepContinue without
// advancing the turn; timeout tests drive the game through AI timeout plays
// rather than human actions.
func (g *timeoutGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{Outcome: StepContinue}, nil
}

// AIPlay advances turnSeat to the next seat modulo seatCount and returns
// StepContinue, simulating the AI auto-play a turn timeout triggers without
// looping on one seat forever.
func (g *timeoutGame) AIPlay(int) (StepResult, error) {
	g.turnSeat = (g.turnSeat + 1) % g.seatCount
	return StepResult{Outcome: StepContinue}, nil
}

// Resume returns StepContinue; timeoutGame never enters a pausable state.
func (g *timeoutGame) Resume() (StepResult, error) {
	return StepResult{Outcome: StepContinue}, nil
}

// Turn returns the turnSeat field so the session schedules a timeout for
// whichever seat the game is currently on.
func (g *timeoutGame) Turn() int {
	return g.turnSeat
}

// PlayerSnapshot returns nil, which marshals to null; the timeout tests
// assert on seq advancement, not snapshot content.
func (g *timeoutGame) PlayerSnapshot(int, int) any {
	return nil
}

// ObserverSnapshot returns nil, which marshals to null; the timeout tests
// have no observer subscribers.
func (g *timeoutGame) ObserverSnapshot(int) any {
	return nil
}

// DisplayDelay returns zero; timeout pacing comes from the session's
// turn-timeout configuration, not the game.
func (g *timeoutGame) DisplayDelay() int { return 0 }

// SetTurnDeadline is a no-op; timeoutGame lets the session own deadline
// scheduling and does not track it.
func (g *timeoutGame) SetTurnDeadline(time.Time) {}

// SetPaused is a no-op; timeoutGame does not track pause state.
func (g *timeoutGame) SetPaused(bool) {}

// HandleAction accepts the human play, moves the turn to seat 1, and
// returns StepContinue so driveTurns schedules the AI seat whose play ends
// the game.
func (a *aiPlayFinishedGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	a.turnSeat = 1
	return StepResult{Outcome: StepContinue}, nil
}

// AIPlay returns StepFinished so the session terminates as soon as the AI
// seat's turn is driven.
func (a *aiPlayFinishedGame) AIPlay(int) (StepResult, error) {
	return StepResult{Outcome: StepFinished}, nil
}

// Resume returns StepContinue; aiPlayFinishedGame never enters a pausable
// state.
func (a *aiPlayFinishedGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn returns the turnSeat field, which HandleAction sets to the AI seat.
func (a *aiPlayFinishedGame) Turn() int {
	return a.turnSeat
}

// PlayerSnapshot returns nil, which marshals to null; the test asserts on
// session termination and channel closure, not snapshot content.
func (a *aiPlayFinishedGame) PlayerSnapshot(int, int) any {
	return nil
}

// ObserverSnapshot returns nil, which marshals to null; the test checks
// observer channel closure rather than snapshot content.
func (a *aiPlayFinishedGame) ObserverSnapshot(int) any {
	return nil
}

// DisplayDelay returns zero; the test sets the session's AI action delay
// instead of using game pacing.
func (a *aiPlayFinishedGame) DisplayDelay() int { return 0 }

// SetTurnDeadline is a no-op; aiPlayFinishedGame does not track turn
// deadlines.
func (a *aiPlayFinishedGame) SetTurnDeadline(time.Time) {}

// SetPaused is a no-op; aiPlayFinishedGame does not track pause state.
func (a *aiPlayFinishedGame) SetPaused(bool) {}

// HandleAction accepts every action and returns StepContinue; the test
// terminates on the invalid turn before any action is processed.
func (i *invalidTurnGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{Outcome: StepContinue}, nil
}

// AIPlay returns StepContinue; the session terminates on the invalid Turn
// result before any AI play.
func (i *invalidTurnGame) AIPlay(int) (StepResult, error) {
	return StepResult{}, nil
}

// Resume returns StepContinue; invalidTurnGame never enters a pausable
// state.
func (i *invalidTurnGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn always returns -1, an invalid seat, so driveTurns treats it as a
// fatal error and terminates the session.
func (i *invalidTurnGame) Turn() int {
	return -1
}

// PlayerSnapshot returns nil, which marshals to null; the test asserts on
// session termination, not snapshot content.
func (i *invalidTurnGame) PlayerSnapshot(int, int) any {
	return nil
}

// ObserverSnapshot returns nil, which marshals to null; the test has no
// observer subscribers.
func (i *invalidTurnGame) ObserverSnapshot(int) any {
	return nil
}

// DisplayDelay returns zero; the invalid-turn test does not exercise UX
// pacing.
func (i *invalidTurnGame) DisplayDelay() int { return 0 }

// SetTurnDeadline is a no-op; invalidTurnGame does not track turn
// deadlines.
func (i *invalidTurnGame) SetTurnDeadline(time.Time) {}

// SetPaused is a no-op; invalidTurnGame does not track pause state.
func (i *invalidTurnGame) SetPaused(bool) {}

// HandleAction accepts the human play and returns StepContinue, leaving
// the turn on seat 0 so the session sets and broadcasts a fresh deadline
// after the play.
func (d *deadlineBroadcastGame) HandleAction(int, *api.InboundMessage) (StepResult, *CommandError) {
	return StepResult{}, nil
}

// AIPlay returns StepContinue; the test uses a single human seat, so no AI
// turn is driven.
func (d *deadlineBroadcastGame) AIPlay(int) (StepResult, error) {
	return StepResult{}, nil
}

// Resume returns StepContinue; deadlineBroadcastGame never enters a
// pausable state.
func (d *deadlineBroadcastGame) Resume() (StepResult, error) {
	return StepResult{}, nil
}

// Turn always returns seat 0 so every turn belongs to the human seat and
// carries a deadline.
func (d *deadlineBroadcastGame) Turn() int { return 0 }

// PlayerSnapshot returns a map carrying seq, seat, turn, and the stored
// deadline as turn_deadline_ms so the test can verify the client receives
// the deadline after a play.
func (d *deadlineBroadcastGame) PlayerSnapshot(seat, seq int) any {
	return map[string]any{
		"seq":              seq,
		"seat":             seat,
		"turn":             0,
		"turn_deadline_ms": d.deadline.UnixMilli(),
	}
}

// ObserverSnapshot returns a map carrying seq, turn, and the stored
// deadline as turn_deadline_ms so observer snapshots expose the same
// deadline.
func (d *deadlineBroadcastGame) ObserverSnapshot(seq int) any {
	return map[string]any{
		"seq":              seq,
		"turn":             0,
		"turn_deadline_ms": d.deadline.UnixMilli(),
	}
}

// DisplayDelay returns zero; the deadline test does not exercise UX pacing.
func (d *deadlineBroadcastGame) DisplayDelay() int { return 0 }

// SetTurnDeadline stores the deadline in the deadline field so the next
// snapshot broadcasts it as turn_deadline_ms.
func (d *deadlineBroadcastGame) SetTurnDeadline(deadline time.Time) {
	d.deadline = deadline
}

// SetPaused is a no-op; deadlineBroadcastGame does not track pause state.
func (d *deadlineBroadcastGame) SetPaused(bool) {}
