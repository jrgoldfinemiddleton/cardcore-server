package client

import (
	"encoding/json"
	"fmt"
)

// Command is the envelope for all messages sent from the client to the server.
type Command struct {
	Type     string          `json:"type"`
	ActionID string          `json:"action_id"`
	Seq      int             `json:"seq"`
	Payload  json.RawMessage `json:"payload"`
}

// ErrorMessage is sent by the server when a client command is rejected.
type ErrorMessage struct {
	Type       string `json:"type"`
	ErrorCode  string `json:"error_code"`
	Message    string `json:"message"`
	ActionID   string `json:"action_id,omitempty"`
	CurrentSeq int    `json:"current_seq"`
}

// Error returns the error code and message.
func (e *ErrorMessage) Error() string {
	return fmt.Sprintf("server error %s: %s", e.ErrorCode, e.Message)
}
