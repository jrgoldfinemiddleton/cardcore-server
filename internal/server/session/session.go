package session

// State represents a session's position in its lifecycle.
type State string

// Session lifecycle states.
const (
	// Draft is the state of a session that has been created but not yet
	// started; its configuration may still be changed via Update.
	Draft State = "draft"
	// Active is the state of a session whose game is running; commands
	// are accepted and snapshots are emitted to subscribers.
	Active State = "active"
	// Finished is the state of a session whose game ended naturally.
	// It is read-only and never auto-expires.
	Finished State = "finished"
	// Expired is the state of a session that has been deleted; its ID is
	// no longer valid and it is omitted from Get and List results.
	Expired State = "expired"
)

// Seat type identifiers.
const (
	// SeatHuman identifies a seat played by a human client. Human seats
	// receive a bearer token on creation and the session waits for their
	// input, subject to the turn timeout.
	SeatHuman = "human"
	// SeatAI identifies a seat played by an AI. AI seats require an
	// ai_type in the seat configuration and receive no bearer token.
	SeatAI = "ai"
)

// SeatConfig describes a single seat's setup at session creation time.
type SeatConfig struct {
	// Type is "human" or "ai".
	Type string `json:"type"`
	// AIType is the AI implementation name (e.g., "random", "heuristic").
	// For AI seats it drives that seat's play. For human seats it is the
	// fallback AI used when the human turn times out or auto-play is
	// needed; empty means the game's default fallback ("random" for Hearts).
	AIType string `json:"ai_type,omitempty"`
}

// Config holds the parameters for creating a new session.
type Config struct {
	// Game selects which registered GameConfig validates and builds this
	// session: it is the lookup key in the server's game registry (e.g.,
	// "hearts"), not the cardcore engine's canonical game name. A registry
	// may hold multiple entries that build the same engine game with
	// different configurations, so the key need not match any engine-level
	// name.
	Game string `json:"game"`
	// Seats defines each seat's configuration.
	Seats []SeatConfig `json:"seats"`
	// AIActionDelayMS is the delay in milliseconds between AI turns.
	// Nil means use the default (1000ms). *0 means no delay.
	AIActionDelayMS *int `json:"ai_action_delay_ms,omitempty"`
	// DealDisplayDelayMS is how long to show the deal before
	// advancing. Applied after every Deal() — initial game start and
	// between rounds. Nil means use the default (1500ms). *0 means no delay.
	DealDisplayDelayMS *int `json:"deal_display_delay_ms,omitempty"`
	// TurnTimeoutMS is the maximum time in milliseconds to wait for
	// a human player to act before auto-playing an AI move. Nil means
	// use the default (30000ms = 30s). *0 means disabled (no timeout).
	TurnTimeoutMS *int `json:"turn_timeout_ms,omitempty"`
}

// PatchConfig holds optional fields for updating a session in draft state.
// Pointer fields distinguish "not provided" from zero values.
type PatchConfig struct {
	// Seats replaces the seat configuration when non-nil.
	Seats []SeatConfig `json:"seats,omitempty"`
	// AIActionDelayMS updates the AI action delay when non-nil.
	AIActionDelayMS *int `json:"ai_action_delay_ms,omitempty"`
	// DealDisplayDelayMS updates the deal display delay when non-nil.
	DealDisplayDelayMS *int `json:"deal_display_delay_ms,omitempty"`
	// TurnTimeoutMS updates the turn timeout when non-nil.
	TurnTimeoutMS *int `json:"turn_timeout_ms,omitempty"`
}

// Seat is returned from session creation and update with the seat's
// token. Token is only present for human seats.
type Seat struct {
	// Index is the 0-based seat position.
	Index int `json:"index"`
	// Type is "human" or "ai".
	Type string `json:"type"`
	// Token is the bearer token for WebSocket authentication.
	// Empty for AI seats.
	Token string `json:"token,omitempty"`
}

// SeatDetail describes a seat in session info responses.
// Unlike Seat, it does not include the token.
type SeatDetail struct {
	// Index is the 0-based seat position.
	Index int `json:"index"`
	// Type is "human" or "ai".
	Type string `json:"type"`
	// AIType is the AI implementation name. For human seats this is the
	// configured fallback AI type (empty when no override was requested).
	AIType string `json:"ai_type,omitempty"`
}

// Summary is the abbreviated form returned by list operations.
type Summary struct {
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

// Info is the full session detail returned by get and update
// operations.
type Info struct {
	// SessionID is the opaque session identifier.
	SessionID string `json:"session_id"`
	// Game is the game identifier.
	Game string `json:"game"`
	// State is the current lifecycle state.
	State State `json:"state"`
	// Seats describes each seat's configuration.
	Seats []SeatDetail `json:"seats"`
	// AIActionDelayMS is the configured AI action delay in milliseconds.
	AIActionDelayMS int `json:"ai_action_delay_ms"`
	// DealDisplayDelayMS is the configured deal display delay in milliseconds.
	DealDisplayDelayMS int `json:"deal_display_delay_ms"`
	// TurnTimeoutMS is the configured turn timeout in milliseconds.
	// 0 means disabled.
	TurnTimeoutMS int `json:"turn_timeout_ms"`
}
