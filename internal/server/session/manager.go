package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
)

const defaultPacingDelayMS = 500

// Sentinel errors returned by Manager methods.
var (
	ErrNotFound  = errors.New("session not found")
	ErrNotDraft  = errors.New("session is not in draft state")
	ErrNotActive = errors.New("session not active")
	ErrNotReady  = errors.New("session start not implemented")
)

// entry holds the internal state of a session within the Manager.
type entry struct {
	// state is the current session lifecycle state.
	state State
	// config is the session configuration (immutable after start).
	config Config
	// seats holds seat info with tokens. Replaced by Update when the seat
	// configuration changes.
	seats []SeatInfo
	// sess is the running session goroutine, nil until Start.
	sess *session
}

// tokenInfo holds the session and seat associated with a bearer token.
type tokenInfo struct {
	sessionID string
	seat      int
}

// Manager is a thread-safe registry of game sessions.
type Manager struct {
	// mu protects sessions and tokenIndex maps.
	mu sync.RWMutex
	// sessions maps session ID to entry.
	sessions map[string]*entry
	// tokenIndex maps bearer token to session and seat for WebSocket
	// authentication. Populated on Create/Update, cleaned on Delete.
	tokenIndex map[string]tokenInfo
	// factory creates Game adapters from a Config.
	factory func(Config) (Game, error)
}

// NewManager creates an empty session manager. The factory creates a
// Game adapter from a Config.
func NewManager(factory func(Config) (Game, error)) *Manager {
	return &Manager{
		sessions:   make(map[string]*entry),
		tokenIndex: make(map[string]tokenInfo),
		factory:    factory,
	}
}

// Create validates cfg, generates a session ID and per-seat tokens, and
// stores the session in draft state. The returned *SessionInfo contains
// the session state, the []SeatInfo contains freshly minted bearer
// tokens for human seats (empty for AI seats), and error is non-nil on
// validation or token-generation failure.
func (m *Manager) Create(cfg Config) (*SessionInfo, []SeatInfo, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, nil, err
	}

	id, err := generateSessionID()
	if err != nil {
		return nil, nil, fmt.Errorf("generating session ID: %w", err)
	}

	seats, err := buildSeatInfo(cfg.Seats)
	if err != nil {
		return nil, nil, err
	}

	m.mu.Lock()
	m.sessions[id] = &entry{
		state:  Draft,
		config: cfg,
		seats:  seats,
	}
	for i, s := range seats {
		if s.Token != "" {
			m.tokenIndex[s.Token] = tokenInfo{sessionID: id, seat: i}
		}
	}
	info := m.sessions[id].info(id)
	m.mu.Unlock()

	return info, seats, nil
}

// Get returns the full SessionInfo for id. The returned error is
// ErrNotFound if the session does not exist or has expired.
func (m *Manager) Get(id string) (*SessionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return nil, ErrNotFound
	}
	return e.info(id), nil
}

// List returns a summary of every session that is not expired. The
// slice is newly allocated on each call; callers may modify it.
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

// Update applies patch to the session identified by id. Only the seat
// configuration and AI delay may be changed, and only while the session
// is in draft state. When patch.Seats is non-nil the returned []SeatInfo
// contains freshly minted bearer tokens for human seats; otherwise it is
// nil. The returned *SessionInfo never contains tokens. Returns
// ErrNotFound (missing/expired) or ErrNotDraft (already started).
func (m *Manager) Update(
	id string, patch PatchConfig,
) (*SessionInfo, []SeatInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return nil, nil, ErrNotFound
	}
	if e.state != Draft {
		return nil, nil, ErrNotDraft
	}

	if patch.Seats != nil {
		cfg := Config{
			Game:          e.config.Game,
			Seats:         patch.Seats,
			PacingDelayMS: e.config.PacingDelayMS,
		}
		if err := validateConfig(cfg); err != nil {
			return nil, nil, err
		}
		for _, s := range e.seats {
			if s.Token != "" {
				delete(m.tokenIndex, s.Token)
			}
		}
		e.config.Seats = patch.Seats

		seats, err := buildSeatInfo(patch.Seats)
		if err != nil {
			return nil, nil, err
		}
		e.seats = seats
		for i, s := range seats {
			if s.Token != "" {
				m.tokenIndex[s.Token] = tokenInfo{sessionID: id, seat: i}
			}
		}
		return e.info(id), seats, nil
	}

	if patch.PacingDelayMS != nil {
		e.config.PacingDelayMS = patch.PacingDelayMS
	}

	return e.info(id), nil, nil
}

// Start transitions the session from draft to active. It creates the
// game adapter via the factory, spawns the session goroutine, and sets
// state to Active. Returns ErrNotFound (missing/expired), ErrNotDraft
// (not in draft), or a game-specific error if the adapter rejects the
// config.
func (m *Manager) Start(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return ErrNotFound
	}
	if e.state != Draft {
		return ErrNotDraft
	}

	game, err := m.factory(e.config)
	if err != nil {
		return fmt.Errorf("creating game: %w", err)
	}

	sessionID := id
	onDone := func(finalState State) {
		m.mu.Lock()
		// Only transition from Active. Delete may have already set
		// Expired by the time the goroutine's exit callback fires.
		if entry, ok := m.sessions[sessionID]; ok && entry.state == Active {
			entry.state = finalState
		}
		m.mu.Unlock()
	}

	e.sess = newSession(id, game, e.config, onDone)
	e.state = Active
	return nil
}

// Delete transitions the session to expired, shutting down the session
// goroutine if it is running. Returns ErrNotFound if the session does
// not exist or is already expired.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return ErrNotFound
	}
	// Only close cancel when the session is Active. A Finished
	// session's goroutine has already exited; double-closing panics.
	if e.state == Active {
		close(e.sess.cancel)
	}
	for _, s := range e.seats {
		if s.Token != "" {
			delete(m.tokenIndex, s.Token)
		}
	}
	e.state = Expired
	return nil
}

// LookupToken resolves a bearer token to its session and seat index.
// Returns ErrNotFound if the token is invalid or the session has expired.
func (m *Manager) LookupToken(token string) (string, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ti, ok := m.tokenIndex[token]
	if !ok {
		return "", 0, ErrNotFound
	}
	return ti.sessionID, ti.seat, nil
}

// SubscribePlayer opens a buffered channel that receives snapshot updates
// for seat. If seat already has an active subscriber, the previous
// channel is closed and replaced. Returns ErrNotFound (missing/expired)
// or ErrNotActive if the session is not active.
func (m *Manager) SubscribePlayer(id string, seat int) (chan []byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return nil, ErrNotFound
	}
	if e.state != Active {
		return nil, ErrNotActive
	}

	ch := make(chan []byte, subChanSize)

	// Delete may have closed the goroutine's cancel channel and still
	// hold the write lock, so e.state still reads Active even though
	// the goroutine has already exited and closed done. <-done is the
	// definitive signal that the goroutine is dead.
	select {
	case <-e.sess.done:
		return nil, ErrNotActive
	case e.sess.cmds <- subscribePlayerCmd{seat: seat, ch: ch}:
		return ch, nil
	default:
		return nil, errors.New("command queue full")
	}
}

// SubscribeObserver opens a buffered channel that receives every
// broadcast snapshot for the session. Observers do not replace each
// other; multiple observer channels may be active concurrently. Returns
// ErrNotFound (missing/expired) or ErrNotActive if the session is not
// active.
func (m *Manager) SubscribeObserver(id string) (chan []byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return nil, ErrNotFound
	}
	if e.state != Active {
		return nil, ErrNotActive
	}

	ch := make(chan []byte, subChanSize)

	// Delete may have closed the goroutine's cancel channel and still
	// hold the write lock, so e.state still reads Active even though
	// the goroutine has already exited and closed done. <-done is the
	// definitive signal that the goroutine is dead.
	select {
	case <-e.sess.done:
		return nil, ErrNotActive
	case e.sess.cmds <- subscribeObserverCmd{ch: ch}:
		return ch, nil
	default:
		return nil, errors.New("command queue full")
	}
}

// UnsubscribePlayer sends an unsubscribe command to the session
// goroutine for seat, causing the goroutine to close the player's
// snapshot channel. Returns ErrNotFound (missing/expired) or
// ErrNotActive if the session is not active.
func (m *Manager) UnsubscribePlayer(id string, seat int) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return ErrNotFound
	}
	if e.state != Active {
		return ErrNotActive
	}

	// Delete may have closed the goroutine's cancel channel and still
	// hold the write lock, so e.state still reads Active even though
	// the goroutine has already exited and closed done. <-done is the
	// definitive signal that the goroutine is dead.
	select {
	case <-e.sess.done:
		return ErrNotActive
	case e.sess.cmds <- unsubscribeCmd{seat: seat}:
		return nil
	default:
		return errors.New("command queue full")
	}
}

// UnsubscribeObserver sends an unsubscribe command for ch to the
// session goroutine, causing the goroutine to remove and close ch from
// the observer list. Returns ErrNotFound (missing/expired) or
// ErrNotActive if the session is not active.
func (m *Manager) UnsubscribeObserver(id string, ch chan []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return ErrNotFound
	}
	if e.state != Active {
		return ErrNotActive
	}

	// Delete may have closed the goroutine's cancel channel and still
	// hold the write lock, so e.state still reads Active even though
	// the goroutine has already exited and closed done. <-done is the
	// definitive signal that the goroutine is dead.
	select {
	case <-e.sess.done:
		return ErrNotActive
	case e.sess.cmds <- unsubscribeCmd{seat: -1, ch: ch}:
		return nil
	default:
		return errors.New("command queue full")
	}
}

// SubmitAction submits a player command from seat to the session goroutine
// and blocks until the goroutine processes it. The returned
// SubmitResult contains the resulting snapshot (on success) or a
// CommandError (on rejection), and the error value is non-nil only for
// transport-level failures (ErrNotFound, ErrNotActive).
func (m *Manager) SubmitAction(
	id string, seat int, msg *api.InboundMessage,
) (SubmitResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return SubmitResult{}, ErrNotFound
	}
	if e.state != Active {
		return SubmitResult{}, ErrNotActive
	}

	resp := make(chan SubmitResult, 1)
	cmd := playCmd{
		seat: seat,
		msg:  msg,
		resp: resp,
	}

	// The send is non-blocking (with a default case) because this method
	// holds an RLock; blocking on a full cmd channel would stall all other
	// read operations on the Manager. The receive is also guarded with
	// <-done so that the caller does not block forever if the goroutine
	// exits after the send to cmds succeeds but before it responds on resp.
	select {
	case e.sess.cmds <- cmd:
		select {
		case result := <-resp:
			return result, nil
		case <-e.sess.done:
			return SubmitResult{}, ErrNotActive
		}
	default:
		return SubmitResult{}, errors.New("command queue full")
	}
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
		SessionID:     id,
		Game:          e.config.Game,
		State:         e.state,
		Seats:         details,
		PacingDelayMS: e.delay(),
	}
}

// delay returns the resolved pacing delay in milliseconds, applying
// the default when the config value is nil.
func (e *entry) delay() int {
	if e.config.PacingDelayMS != nil {
		return *e.config.PacingDelayMS
	}
	return defaultPacingDelayMS
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
