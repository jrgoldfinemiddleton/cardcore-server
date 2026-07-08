package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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
	// AIActionDelayMS is the delay in milliseconds between AI turns.
	// Zero means no delay. nil means use the server default.
	AIActionDelayMS *int `json:"ai_action_delay_ms,omitempty"`
	// DealDisplayDelayMS is how long to show the deal before
	// advancing. Applied after every Deal(). Zero means no delay.
	// nil means use the server default.
	DealDisplayDelayMS *int `json:"deal_display_delay_ms,omitempty"`
	// TurnTimeoutMS is the maximum time in milliseconds to wait for
	// a human player to act before auto-playing an AI move. Zero means
	// no timeout. nil means use the server default.
	TurnTimeoutMS *int `json:"turn_timeout_ms,omitempty"`
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

// SessionInfo is the full session detail returned by get and update
// operations.
type SessionInfo struct {
	// SessionID is the opaque session identifier.
	SessionID string `json:"session_id"`
	// Game is the game identifier.
	Game string `json:"game"`
	// State is the current lifecycle state (e.g., "draft", "active", "finished").
	State string `json:"state"`
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

// HTTPError is returned when the server responds with a non-success
// status code.
type HTTPError struct {
	// StatusCode is the HTTP status code from the response.
	StatusCode int
	// Message is the error message from the response body.
	Message string
}

// SessionClient wraps HTTP calls to the cardcore server for session
// lifecycle management.
type SessionClient struct {
	// BaseURL is the server base URL (e.g., "http://localhost:8080").
	BaseURL string
	// HTTPClient is the HTTP client used for requests. If nil,
	// [http.DefaultClient] is used.
	HTTPClient *http.Client
	// Logger is the structured logger for client operations. If nil,
	// [slog.Default] is used.
	Logger *slog.Logger
}

// createResponse is the JSON body for POST /sessions responses.
type createResponse struct {
	SessionID string     `json:"session_id"`
	Seats     []SeatInfo `json:"seats"`
}

// startResponse is the JSON body for POST /sessions/{id}/start responses.
type startResponse struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"`
}

// Error returns a string including the status code and message.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

// CreateSession creates a new session in draft state.
// On success it returns the session ID and seat info (including tokens
// for human seats).
func (c *SessionClient) CreateSession(ctx context.Context, cfg Config) (string, []SeatInfo, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		c.logger().Error("marshal create session config", "error", err)
		return "", nil, err
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/sessions", data)
	if err != nil {
		c.logger().Error("create session request", "error", err)
		return "", nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		he := readError(resp)
		c.logger().Warn("create session failed", "status_code", he.StatusCode, "error", he.Message)
		return "", nil, he
	}

	var cr createResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		c.logger().Error("decode create session response", "error", err)
		return "", nil, err
	}
	c.logger().Info("session created", "session_id", cr.SessionID)
	return cr.SessionID, cr.Seats, nil
}

// StartSession transitions a draft session to active.
func (c *SessionClient) StartSession(ctx context.Context, sessionID string) error {
	resp, err := c.doRequest(ctx, http.MethodPost, "/sessions/"+sessionID+"/start", nil)
	if err != nil {
		c.logger().Error("start session request", "session_id", sessionID, "error", err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		he := readError(resp)
		c.logger().Warn("start session failed",
			"session_id", sessionID,
			"status_code", he.StatusCode,
			"error", he.Message)
		return he
	}
	c.logger().Info("session started", "session_id", sessionID)
	return nil
}

// DeleteSession deletes a session.
func (c *SessionClient) DeleteSession(ctx context.Context, sessionID string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/sessions/"+sessionID, nil)
	if err != nil {
		c.logger().Error("delete session request", "session_id", sessionID, "error", err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		he := readError(resp)
		c.logger().Warn("delete session failed",
			"session_id", sessionID,
			"status_code", he.StatusCode,
			"error", he.Message)
		return he
	}
	c.logger().Info("session deleted", "session_id", sessionID)
	return nil
}

// GetSession fetches the current session info for the given session ID.
func (c *SessionClient) GetSession(ctx context.Context, sessionID string) (SessionInfo, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/sessions/"+sessionID, nil)
	if err != nil {
		c.logger().Error("get session request", "session_id", sessionID, "error", err)
		return SessionInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		he := readError(resp)
		c.logger().Warn("get session failed",
			"session_id", sessionID,
			"status_code", he.StatusCode,
			"error", he.Message)
		return SessionInfo{}, he
	}

	var info SessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		c.logger().Error("decode get session response", "session_id", sessionID, "error", err)
		return SessionInfo{}, err
	}
	c.logger().Info("session fetched", "session_id", sessionID, "state", info.State)
	return info, nil
}

// logger returns the configured logger or [slog.Default] if nil.
func (c *SessionClient) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// doRequest sends an HTTP request and returns the response. The caller
// is responsible for closing the response body.
func (c *SessionClient) doRequest(
	ctx context.Context,
	method, path string,
	body []byte,
) (*http.Response, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	c.logger().Debug("http response",
		"method", method,
		"path", path,
		"status_code", resp.StatusCode)
	return resp, nil
}

// readError decodes an error response body into an HTTPError.
func readError(resp *http.Response) *HTTPError {
	var er struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&er)
	return &HTTPError{
		StatusCode: resp.StatusCode,
		Message:    er.Error,
	}
}
