package api

import (
	"encoding/json"
	"errors"
)

// Error code constants for client command rejection.
const (
	ErrStaleSeq         = "stale_seq"
	ErrOutOfTurn        = "out_of_turn"
	ErrIllegalMove      = "illegal_move"
	ErrWrongPhase       = "wrong_phase"
	ErrGameOver         = "game_over"
	ErrMalformedMessage = "malformed_message"
	ErrInternal         = "internal_error"
	// ErrPauseNotAllowed indicates that a pause/resume operation is not allowed
	// in the current game state.
	ErrPauseNotAllowed = "pause_not_allowed"
)

// InboundMessage is the common envelope for all client-to-server messages.
type InboundMessage struct {
	Type     string          `json:"type"`
	ActionID string          `json:"action_id"`
	Seq      int             `json:"seq"`
	Payload  json.RawMessage `json:"payload"`
}

// ErrorMessage is sent when a client command is rejected.
type ErrorMessage struct {
	Type       string `json:"type"`
	ErrorCode  string `json:"error_code"`
	Message    string `json:"message"`
	ActionID   string `json:"action_id,omitempty"`
	CurrentSeq int    `json:"current_seq"`
}

// ValidateInboundMessage checks that the required envelope fields are
// present. It returns a descriptive error for the first validation failure
// encountered.
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
	if msg.Seq < 0 {
		return errors.New("negative seq")
	}
	if len(msg.Payload) == 0 {
		return errors.New("missing payload")
	}
	return nil
}
