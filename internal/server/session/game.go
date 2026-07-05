package session

import "github.com/jrgoldfinemiddleton/cardcore-server/internal/api"

// StepOutcome describes what the session goroutine should do after a
// game mutation.
type StepOutcome uint8

const (
	// StepContinue means the game advanced normally. The session
	// should emit a snapshot and schedule the next turn.
	StepContinue StepOutcome = iota
	// StepPause means the game reached a state that should be shown
	// to players before advancing (e.g., a completed trick or round
	// summary). The session should emit a snapshot, wait for UX
	// pacing, then call Resume.
	StepPause
	// StepFinished means the game is over. The session should emit a
	// final snapshot and stop.
	StepFinished
)

// StepResult is returned by game-mutating methods to tell the session
// goroutine how to proceed.
type StepResult struct {
	// Outcome determines post-mutation behavior.
	Outcome StepOutcome
}

// CommandError is returned when a player action is rejected. Code is a
// machine-readable error code (e.g., [api.ErrIllegalMove]) and Message is
// a human-readable explanation.
type CommandError struct {
	// Code is the machine-readable error code for the wire protocol.
	Code string
	// Message is the human-readable explanation of the rejection.
	Message string
}

// Game is the interface the session goroutine uses to drive a card
// game. Implementations are game-specific; the session layer never
// inspects game-specific types or phase names.
//
// The session goroutine handles game-agnostic concerns (seq numbering,
// action_id idempotency, stale_seq detection, AI turn scheduling,
// subscriber management, JSON serialization). The Game implementation
// handles everything else: action validation, engine mutations, phase
// transitions, AI player logic, and snapshot generation.
//
// Mutating methods (HandleAction, AIPlay, Resume) return a StepResult
// that tells the session goroutine whether to continue, pause for UX
// pacing, or stop because the game is over. The session goroutine
// reacts to the StepOutcome without knowing what caused it.
type Game interface {
	// HandleAction processes an inbound player action. The
	// implementation validates turn order, phase, and legality. It
	// returns a non-nil CommandError if the action is rejected, or a
	// StepResult describing how the session should proceed.
	HandleAction(seat int, msg *api.InboundMessage) (StepResult, *CommandError)

	// AIPlay executes the AI player's move for the given seat. The
	// implementation selects and applies the appropriate action based
	// on the current game phase.
	//
	// Outcome may be StepContinue, StepPause, or StepFinished.
	// StepPause occurs when the AI's play completes a pausable game
	// state (e.g., a finished trick in Hearts). After StepPause, the
	// session waits for UX pacing and then calls Resume(), so the
	// game advances just as it does for human plays.
	AIPlay(seat int) (StepResult, error)

	// Resume advances the game past a pausable state. Only valid
	// after a StepPause outcome. Returns an error if the game is not
	// in a pausable state. The returned StepResult may indicate
	// another pause (chained pauses), normal continuation, or game
	// completion.
	Resume() (StepResult, error)

	// Turn returns the seat index whose turn it is. Only meaningful
	// when the last StepOutcome was StepContinue.
	Turn() int

	// PlayerSnapshot builds a seat-filtered snapshot for the given
	// player at the given sequence number. The session goroutine
	// calls json.Marshal on the returned value.
	PlayerSnapshot(seat int, seq int) any

	// ObserverSnapshot builds a full-information snapshot at the
	// given sequence number. The session goroutine calls
	// json.Marshal on the returned value.
	ObserverSnapshot(seq int) any

	// DisplayDelay returns the number of milliseconds to wait before
	// advancing past the current game state. Zero means advance
	// immediately. The session goroutine calls this after broadcasting
	// a snapshot in a pausable or initial state to give clients time
	// to render it.
	DisplayDelay() int
}

// Error implements the error interface.
func (e *CommandError) Error() string {
	return e.Message
}
