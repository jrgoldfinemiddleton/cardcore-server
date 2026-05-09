package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
)

const defaultAIDelayMS = 500

// Sentinel errors returned by Manager methods.
var (
	ErrNotFound = errors.New("session not found")
	ErrNotDraft = errors.New("session is not in draft state")
	ErrNotReady = errors.New("session start not implemented")
)

// entry holds the internal state of a session within the Manager.
type entry struct {
	state  State
	config Config
	seats  []SeatInfo
}

// Manager is a thread-safe registry of game sessions.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*entry
}

// NewManager creates an empty session manager.
func NewManager() *Manager {
	return &Manager{sessions: make(map[string]*entry)}
}

// Create validates the config, generates a session ID and seat tokens,
// and stores the session in draft state.
func (m *Manager) Create(cfg Config) (string, []SeatInfo, error) {
	if err := validateConfig(cfg); err != nil {
		return "", nil, err
	}

	id, err := generateSessionID()
	if err != nil {
		return "", nil, fmt.Errorf("generating session ID: %w", err)
	}

	seats, err := buildSeatInfo(cfg.Seats)
	if err != nil {
		return "", nil, err
	}

	m.mu.Lock()
	m.sessions[id] = &entry{
		state:  Draft,
		config: cfg,
		seats:  seats,
	}
	m.mu.Unlock()

	return id, seats, nil
}

// Get returns full session details. Returns ErrNotFound if the session
// does not exist or has expired.
func (m *Manager) Get(id string) (*SessionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return nil, ErrNotFound
	}
	return e.info(id), nil
}

// List returns summaries of all non-expired sessions.
func (m *Manager) List() []SessionSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []SessionSummary
	for id, e := range m.sessions {
		if e.state == Expired {
			continue
		}
		humans := 0
		for _, s := range e.config.Seats {
			if s.Type == SeatHuman {
				humans++
			}
		}
		out = append(out, SessionSummary{
			SessionID:  id,
			Game:       e.config.Game,
			State:      e.state,
			SeatCount:  len(e.config.Seats),
			HumanCount: humans,
		})
	}
	return out
}

// Update modifies a session's configuration. Only valid in draft state.
// Returns ErrNotFound if missing/expired, ErrNotDraft if not in draft.
func (m *Manager) Update(
	id string, patch PatchConfig,
) (*SessionInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return nil, ErrNotFound
	}
	if e.state != Draft {
		return nil, ErrNotDraft
	}

	if patch.Seats != nil {
		cfg := Config{
			Game:      e.config.Game,
			Seats:     patch.Seats,
			AIDelayMS: e.config.AIDelayMS,
		}
		if err := validateConfig(cfg); err != nil {
			return nil, err
		}
		e.config.Seats = patch.Seats

		seats, err := buildSeatInfo(patch.Seats)
		if err != nil {
			return nil, err
		}
		e.seats = seats
	}

	if patch.AIDelayMS != nil {
		e.config.AIDelayMS = patch.AIDelayMS
	}

	return e.info(id), nil
}

// Start transitions a session from draft to active. Not yet
// implemented — returns ErrNotReady.
func (m *Manager) Start(id string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return ErrNotFound
	}
	if e.state != Draft {
		return ErrNotDraft
	}
	return ErrNotReady
}

// Delete transitions a session to expired from any other state.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return ErrNotFound
	}
	e.state = Expired
	return nil
}

// info builds a SessionInfo from an entry. Caller must hold at least a
// read lock.
func (e *entry) info(id string) *SessionInfo {
	details := make([]SeatDetail, len(e.config.Seats))
	for i, sc := range e.config.Seats {
		details[i] = SeatDetail{
			Index:  i,
			Type:   sc.Type,
			AIType: sc.AIType,
		}
	}
	return &SessionInfo{
		SessionID: id,
		Game:      e.config.Game,
		State:     e.state,
		Seats:     details,
		AIDelayMS: e.delay(),
	}
}

// delay returns the resolved AI delay in milliseconds, applying the
// default when the config value is nil.
func (e *entry) delay() int {
	if e.config.AIDelayMS != nil {
		return *e.config.AIDelayMS
	}
	return defaultAIDelayMS
}

// generateSessionID returns a 32-character hex string from 16 random
// bytes.
func generateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// generateSeatToken returns a 64-character hex string from 32 random
// bytes.
func generateSeatToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// buildSeatInfo creates SeatInfo entries with tokens for human seats.
func buildSeatInfo(configs []SeatConfig) ([]SeatInfo, error) {
	seats := make([]SeatInfo, len(configs))
	for i, sc := range configs {
		seats[i] = SeatInfo{Index: i, Type: sc.Type}
		if sc.Type == SeatHuman {
			token, err := generateSeatToken()
			if err != nil {
				return nil, fmt.Errorf(
					"generating token for seat %d: %w", i, err,
				)
			}
			seats[i].Token = token
		}
	}
	return seats, nil
}

// validateConfig checks game-agnostic constraints on a session config.
func validateConfig(cfg Config) error {
	if cfg.Game == "" {
		return errors.New("game is required")
	}
	if len(cfg.Seats) == 0 {
		return errors.New("at least one seat is required")
	}
	for i, s := range cfg.Seats {
		if s.Type != SeatHuman && s.Type != SeatAI {
			return fmt.Errorf(
				"seat %d: type must be \"human\" or \"ai\"", i,
			)
		}
		if s.Type == SeatAI && s.AIType == "" {
			return fmt.Errorf(
				"seat %d: ai_type is required for AI seats", i,
			)
		}
	}
	return nil
}
