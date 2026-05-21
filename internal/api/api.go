package api

import "encoding/json"

// Error code constants for client command rejection.
const (
	ErrStaleSeq         = "stale_seq"
	ErrOutOfTurn        = "out_of_turn"
	ErrIllegalMove      = "illegal_move"
	ErrWrongPhase       = "wrong_phase"
	ErrGameOver         = "game_over"
	ErrMalformedMessage = "malformed_message"
	ErrInternal         = "internal_error"
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
