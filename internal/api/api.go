package api

import (
	"encoding/json"
	"errors"
	"fmt"
)

// MaxActionIDLength is the maximum allowed length for an action_id value.
const MaxActionIDLength = 256

// Error code constants for client command rejection.
const (
	// ErrStaleSeq indicates that the command's seq is behind the server's; the
	// server sends a fresh snapshot immediately before the error, and the
	// client should resync from it.
	ErrStaleSeq = "stale_seq"
	// ErrOutOfTurn indicates that a well-formed command was sent when it was not
	// the sender's turn.
	ErrOutOfTurn = "out_of_turn"
	// ErrIllegalMove indicates that the attempted move violates the game rules;
	// the message field carries the engine's explanation of the violation.
	ErrIllegalMove = "illegal_move"
	// ErrWrongPhase indicates that a well-formed command was sent during a game
	// phase that does not allow it.
	ErrWrongPhase = "wrong_phase"
	// ErrGameOver indicates that the session is in the finished state.
	ErrGameOver = "game_over"
	// ErrMalformedMessage indicates that the message was malformed: a missing
	// required field, a negative seq, an empty or over-length action_id, an
	// unknown message type, or an unparsable or otherwise invalid payload.
	// Envelope JSON that does not parse at all produces no error message;
	// the server closes the connection instead.
	ErrMalformedMessage = "malformed_message"
	// ErrInternal indicates that an unexpected internal server error occurred.
	ErrInternal = "internal_error"
	// ErrPauseNotAllowed indicates that a pause/resume operation is not allowed
	// in the current game state.
	ErrPauseNotAllowed = "pause_not_allowed"
	// ErrGamePaused indicates that a gameplay command was sent while the game
	// is paused. The command is not applied; the client should wait for the
	// resume snapshot instead of retrying.
	ErrGamePaused = "game_paused"
)

// InboundMessage is the common envelope for all client-to-server messages.
type InboundMessage struct {
	// Type is the message type identifier; game-specific types are defined
	// by each game's protocol.
	Type string `json:"type"`
	// ActionID is a client-generated unique ID for idempotency; it is
	// opaque to the server and must be a non-empty string of at most
	// MaxActionIDLength characters.
	ActionID string `json:"action_id"`
	// Seq is the last snapshot seq value the client received.
	Seq int `json:"seq"`
	// Payload is the type-specific payload defined by the game's protocol.
	Payload json.RawMessage `json:"payload"`
}

// ErrorMessage is sent when a client command is rejected.
type ErrorMessage struct {
	// Type is the message type identifier, always "error".
	Type string `json:"type"`
	// ErrorCode is the machine-readable error code.
	ErrorCode string `json:"error_code"`
	// Message is the human-readable explanation of the rejection, suitable
	// for display to the user.
	Message string `json:"message"`
	// ActionID is the action_id from the rejected command; it is omitted
	// when the server has none to echo — the rejected message itself lacked
	// an action_id, or the error was synthesized without one (game_over for
	// a command queued as the session ended).
	ActionID string `json:"action_id,omitempty"`
	// CurrentSeq is the server's current seq value.
	CurrentSeq int `json:"current_seq"`
}

// ValidateInboundMessage checks that the required envelope fields are
// present and well-formed. It returns a descriptive error for the first
// validation failure encountered.
func ValidateInboundMessage(msg *InboundMessage) error {
	if msg == nil {
		return errors.New("nil message")
	}
	if msg.Type == "" {
		return errors.New("missing message type")
	}
	if msg.ActionID == "" {
		return errors.New("missing action_id")
	}
	if len(msg.ActionID) > MaxActionIDLength {
		return fmt.Errorf("action_id exceeds %d characters", MaxActionIDLength)
	}
	if msg.Seq < 0 {
		return errors.New("negative seq")
	}
	if len(msg.Payload) == 0 {
		return errors.New("missing payload")
	}
	return nil
}
