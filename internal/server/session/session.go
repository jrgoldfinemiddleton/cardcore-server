package session

// State represents a session's position in its lifecycle.
type State string

// Session lifecycle states.
const (
	Draft    State = "draft"
	Active   State = "active"
	Finished State = "finished"
	Expired  State = "expired"
)

// SeatConfig describes a single seat's setup at session creation time.
type SeatConfig struct {
	// Type is "human" or "ai".
	Type string `json:"type"`
	// AIType is the AI implementation name (e.g., "random", "heuristic").
	// Only meaningful when Type is "ai".
	AIType string `json:"ai_type,omitempty"`
}

// Config holds the parameters for creating a new session.
type Config struct {
	// Game is the game identifier (e.g., "hearts").
	Game string `json:"game"`
	// Seats defines each seat's configuration.
	Seats []SeatConfig `json:"seats"`
	// AIDelayMS is the minimum delay in milliseconds between AI moves.
	// Defaults to 500 if not set.
	AIDelayMS int `json:"ai_delay_ms,omitempty"`
}

// PatchConfig holds optional fields for updating a session in draft state.
// Pointer fields distinguish "not provided" from zero values.
type PatchConfig struct {
	// Seats replaces the seat configuration when non-nil.
	Seats []SeatConfig `json:"seats,omitempty"`
	// AIDelayMS updates the AI move delay when non-nil.
	AIDelayMS *int `json:"ai_delay_ms,omitempty"`
}

// SeatInfo is returned from session creation with the seat's token.
// Token is only present for human seats.
type SeatInfo struct {
	// Index is the 0-based seat position.
	Index int `json:"index"`
	// Type is "human" or "ai".
	Type string `json:"type"`
	// Token is the bearer token for WebSocket authentication.
	// Empty for AI seats.
	Token string `json:"token,omitempty"`
}

// SeatDetail describes a seat in session info responses.
// Unlike SeatInfo, it does not include the token.
type SeatDetail struct {
	// Index is the 0-based seat position.
	Index int `json:"index"`
	// Type is "human" or "ai".
	Type string `json:"type"`
	// AIType is the AI implementation name. Empty for human seats.
	AIType string `json:"ai_type,omitempty"`
}

// SessionSummary is the abbreviated form returned by list operations.
type SessionSummary struct {
	// SessionID is the opaque session identifier.
	SessionID string `json:"session_id"`
	// Game is the game identifier.
	Game string `json:"game"`
	// State is the current lifecycle state.
	State State `json:"state"`
	// SeatCount is the total number of seats.
	SeatCount int `json:"seat_count"`
	// HumanCount is the number of human seats.
	HumanCount int `json:"human_count"`
}

// SessionInfo is the full session detail returned by get and update
// operations.
type SessionInfo struct {
	// SessionID is the opaque session identifier.
	SessionID string `json:"session_id"`
	// Game is the game identifier.
	Game string `json:"game"`
	// State is the current lifecycle state.
	State State `json:"state"`
	// Seats describes each seat's configuration.
	Seats []SeatDetail `json:"seats"`
	// AIDelayMS is the configured AI move delay in milliseconds.
	AIDelayMS int `json:"ai_delay_ms"`
}
