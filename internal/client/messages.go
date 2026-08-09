package client

import (
	"encoding/json"
	"fmt"
)

// Command is the envelope for all messages sent from the client to the server.
type Command struct {
	// Type is the message type identifier; game-specific types are defined by
	// the game.
	Type string `json:"type"`
	// ActionID is the client-generated unique ID used for idempotency; it is
	// opaque to the server.
	ActionID string `json:"action_id"`
	// Seq is the last snapshot seq value the client received.
	Seq int `json:"seq"`
	// Payload is the type-specific command payload defined by the game.
	Payload json.RawMessage `json:"payload"`
}

// ErrorMessage is sent by the server when a client command is rejected.
type ErrorMessage struct {
	// Type is the message type identifier, always "error".
	Type string `json:"type"`
	// ErrorCode is the machine-readable error code.
	ErrorCode string `json:"error_code"`
	// Message is the human-readable explanation, suitable for display to the
	// user.
	Message string `json:"message"`
	// ActionID is the action_id from the rejected command; it is omitted only
	// when JSON parsing failed and the field could not be extracted.
	ActionID string `json:"action_id,omitempty"`
	// CurrentSeq is the server's current seq value.
	CurrentSeq int `json:"current_seq"`
}

// Error returns the error code and message.
func (e *ErrorMessage) Error() string {
	return fmt.Sprintf("server error %s: %s", e.ErrorCode, e.Message)
}
