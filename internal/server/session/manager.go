package session

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	mathrand "math/rand/v2"
	"sync"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
)

// DefaultServerDelays provides the built-in defaults for server-wide
// timing values.
var DefaultServerDelays = DefaultDelays{
	AIActionDelayMS:    1000,
	DealDisplayDelayMS: 1500,
	TurnTimeoutMS:      30000,
}

// Sentinel errors returned by Manager methods.
var (
	// ErrNotFound is returned when a session ID does not resolve to a
	// live session, or a bearer token does not resolve to a seat.
	ErrNotFound = errors.New("session not found")
	// ErrNotDraft is returned when an operation requires a session in
	// draft state but the session has already been started.
	ErrNotDraft = errors.New("session is not in draft state")
	// ErrNotActive is returned when an operation requires an active
	// session but the session is still in draft state or has finished.
	ErrNotActive = errors.New("session not active")
	// ErrInvalidConfig is returned when a session configuration fails
	// validation; the wrapped message describes the specific violation.
	ErrInvalidConfig = errors.New("invalid session configuration")
)

// entry holds the internal state of a session within the Manager.
type entry struct {
	// state is the current session lifecycle state.
	state State
	// config is the session configuration (immutable after start).
	config Config
	// seats holds seat info with tokens. Replaced by Update when the seat
	// configuration changes.
	seats []Seat
	// sess is the running session goroutine, nil until Start.
	sess *session
	// defaults holds the server-wide defaults at creation time.
	defaults DefaultDelays
}

// tokenInfo holds the session and seat associated with a bearer token.
type tokenInfo struct {
	sessionID string
	seat      int
}

// Manager is a thread-safe registry of game sessions. Transport handlers
// call Manager methods concurrently — one goroutine per HTTP request or
// WebSocket connection — so the sessions and tokenIndex maps are guarded
// by mu. Game state, by contrast, is owned by the single session
// goroutine and needs no lock (see ADR-006).
type Manager struct {
	// mu protects sessions and tokenIndex maps.
	mu sync.RWMutex
	// sessions maps session ID to entry.
	sessions map[string]*entry
	// tokenIndex maps bearer token to session and seat for WebSocket
	// authentication. Populated on Create/Update, cleaned on Delete.
	tokenIndex map[string]tokenInfo
	// registry creates Game adapters from a Config and validates
	// game-specific configuration.
	registry *Registry
	// defaultDelays holds the server-wide default delay values.
	defaultDelays DefaultDelays
}

// DefaultDelays holds server-wide default timing values.
type DefaultDelays struct {
	// AIActionDelayMS is the default pacing delay in milliseconds applied
	// before each AI turn, applied when a session's Config.AIActionDelayMS
	// is nil.
	AIActionDelayMS int
	// DealDisplayDelayMS is the default delay in milliseconds for showing
	// a fresh deal before play advances, applied when a session's
	// Config.DealDisplayDelayMS is nil.
	DealDisplayDelayMS int
	// TurnTimeoutMS is the default maximum time in milliseconds to wait
	// for a human player to act before auto-playing an AI move on their
	// behalf, applied when a session's Config.TurnTimeoutMS is nil.
	// 0 disables the timeout.
	TurnTimeoutMS int
}

// NewManager creates an empty session manager. The registry creates and
// validates Game adapters from a Config. Defaults are applied when config
// fields are nil.
func NewManager(registry *Registry, defaults DefaultDelays) *Manager {
	return &Manager{
		sessions:      make(map[string]*entry),
		tokenIndex:    make(map[string]tokenInfo),
		registry:      registry,
		defaultDelays: defaults,
	}
}

// Create validates cfg, generates a session ID and per-seat tokens, and
// stores the session in draft state. The returned *Info contains
// the session state, the []Seat contains freshly minted bearer
// tokens for human seats (empty for AI seats), and error is non-nil on
// validation or token-generation failure.
func (m *Manager) Create(cfg Config) (*Info, []Seat, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, nil, err
	}
	if err := m.registry.ValidateConfig(cfg); err != nil {
		return nil, nil, err
	}

	id, err := generateSessionID()
	if err != nil {
		return nil, nil, fmt.Errorf("generating session ID: %w", err)
	}

	seats, err := buildSeats(cfg.Seats)
	if err != nil {
		return nil, nil, err
	}

	m.mu.Lock()
	m.sessions[id] = &entry{
		state:    Draft,
		config:   cfg,
		seats:    seats,
		defaults: m.defaultDelays,
	}
	for i, s := range seats {
		if s.Token != "" {
			m.tokenIndex[s.Token] = tokenInfo{sessionID: id, seat: i}
		}
	}
	info := m.sessions[id].info(id)
	m.mu.Unlock()

	slog.With("component", "session_manager").Info("session created",
		"session_id", id,
		"game", cfg.Game,
		"seats", len(cfg.Seats),
	)

	return info, seats, nil
}

// Get returns the full Info for id. The returned error is
// ErrNotFound if the session does not exist or has expired.
func (m *Manager) Get(id string) (*Info, error) {
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
func (m *Manager) List() []Summary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]Summary, 0)
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
		out = append(out, Summary{
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
// configuration and the delay/timeout values may be changed, and only
// while the session is in draft state; all provided fields are applied
// together. When patch.Seats is non-nil, all former seat tokens are
// invalidated and the returned []Seat contains freshly minted bearer
// tokens for every human seat, including unchanged ones; otherwise it is
// nil. The returned *Info never contains tokens. Returns
// ErrNotFound (missing/expired), ErrNotDraft (already started), or
// ErrInvalidConfig if the resulting configuration fails game-agnostic or
// game-specific validation.
func (m *Manager) Update(
	id string, patch PatchConfig,
) (*Info, []Seat, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return nil, nil, ErrNotFound
	}
	if e.state != Draft {
		return nil, nil, ErrNotDraft
	}

	var seats []Seat
	if patch.Seats != nil {
		cfg := Config{
			Game:               e.config.Game,
			Seats:              patch.Seats,
			AIActionDelayMS:    e.config.AIActionDelayMS,
			DealDisplayDelayMS: e.config.DealDisplayDelayMS,
			TurnTimeoutMS:      e.config.TurnTimeoutMS,
		}
		if err := validateConfig(cfg); err != nil {
			return nil, nil, err
		}
		if err := m.registry.ValidateConfig(cfg); err != nil {
			return nil, nil, err
		}
		// Build the replacement seats before mutating the entry so a
		// token-generation failure leaves the session untouched.
		newSeats, err := buildSeats(patch.Seats)
		if err != nil {
			return nil, nil, err
		}
		for _, s := range e.seats {
			if s.Token != "" {
				delete(m.tokenIndex, s.Token)
			}
		}
		e.config.Seats = patch.Seats
		e.seats = newSeats
		for i, s := range newSeats {
			if s.Token != "" {
				m.tokenIndex[s.Token] = tokenInfo{sessionID: id, seat: i}
			}
		}
		seats = newSeats
	}

	if patch.AIActionDelayMS != nil {
		e.config.AIActionDelayMS = patch.AIActionDelayMS
	}
	if patch.DealDisplayDelayMS != nil {
		e.config.DealDisplayDelayMS = patch.DealDisplayDelayMS
	}
	if patch.TurnTimeoutMS != nil {
		e.config.TurnTimeoutMS = patch.TurnTimeoutMS
	}

	return e.info(id), seats, nil
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

	resolvedCfg := resolveConfig(e.config, e.defaults)
	game, err := m.registry.NewGame(resolvedCfg, newRNG())
	if err != nil {
		return fmt.Errorf("creating game: %w", err)
	}

	sessionID := id
	// onDone transitions the session from Active to Finished when the
	// goroutine exits. It runs asynchronously to avoid deadlocking with
	// Manager methods that hold RLock while blocking on session
	// goroutine responses.
	onDone := func(finalState State) {
		go func() {
			m.mu.Lock()
			// Only transition from Active. Delete may have already set
			// Expired by the time the goroutine's exit callback fires.
			if entry, ok := m.sessions[sessionID]; ok && entry.state == Active {
				entry.state = finalState
			}
			m.mu.Unlock()
		}()
	}

	// aiActionDelay and turnTimeout using server-wide defaults.
	e.sess = newSession(id, game, e.config, e.defaults, onDone)
	e.state = Active

	slog.With("component", "session_manager").Info("session started",
		"session_id", id,
	)

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

	slog.With("component", "session_manager").Info("session deleted",
		"session_id", id,
	)

	return nil
}

// LookupToken resolves a bearer token to its session and seat index.
// Returns ErrNotFound if the token is invalid or the session has expired.
func (m *Manager) LookupToken(token string) (string, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ti, ok := m.tokenIndex[token]
	if !ok {
		slog.With("component", "session_manager").Warn("bearer token lookup failed")
		return "", 0, ErrNotFound
	}
	return ti.sessionID, ti.seat, nil
}

// SubscribePlayer sends a subscribe command to the session goroutine and
// returns a new buffered channel that receives snapshot updates for seat.
// If seat already has an active subscriber, the goroutine closes the
// previous channel and replaces it with the new one. Returns ErrNotFound
// (missing/expired) or ErrNotActive if the session is not active.
func (m *Manager) SubscribePlayer(id string, seat int) (chan SubscriberMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return nil, ErrNotFound
	}
	if e.state != Active {
		return nil, ErrNotActive
	}

	ch := make(chan SubscriberMessage, subChanSize)

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

// SubscribeObserver sends a subscribe command to the session goroutine
// and returns a new buffered channel that receives every broadcast
// snapshot for the session. Observers do not replace each other; multiple
// observer channels may be active concurrently. Returns ErrNotFound
// (missing/expired) or ErrNotActive if the session is not active.
func (m *Manager) SubscribeObserver(id string) (chan SubscriberMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.sessions[id]
	if !ok || e.state == Expired {
		return nil, ErrNotFound
	}
	if e.state != Active {
		return nil, ErrNotActive
	}

	ch := make(chan SubscriberMessage, subChanSize)

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
func (m *Manager) UnsubscribeObserver(id string, ch chan SubscriberMessage) error {
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

// info builds an Info from an entry. Caller must hold at least a
// read lock.
func (e *entry) info(id string) *Info {
	details := make([]SeatDetail, len(e.config.Seats))
	for i, sc := range e.config.Seats {
		details[i] = SeatDetail{
			Index:  i,
			Type:   sc.Type,
			AIType: sc.AIType,
		}
	}
	return &Info{
		SessionID:          id,
		Game:               e.config.Game,
		State:              e.state,
		Seats:              details,
		AIActionDelayMS:    e.aiActionDelay(),
		DealDisplayDelayMS: e.dealDisplayDelay(),
		TurnTimeoutMS:      e.turnTimeout(),
	}
}

// aiActionDelay returns the resolved AI action delay in milliseconds,
// applying the default when the config value is nil.
func (e *entry) aiActionDelay() int {
	if e.config.AIActionDelayMS != nil {
		return *e.config.AIActionDelayMS
	}
	return e.defaults.AIActionDelayMS
}

// dealDisplayDelay returns the resolved deal display delay in milliseconds,
// applying the default when the config value is nil.
func (e *entry) dealDisplayDelay() int {
	if e.config.DealDisplayDelayMS != nil {
		return *e.config.DealDisplayDelayMS
	}
	return e.defaults.DealDisplayDelayMS
}

// turnTimeout returns the resolved turn timeout in milliseconds,
// applying the default when the config value is nil.
func (e *entry) turnTimeout() int {
	if e.config.TurnTimeoutMS != nil {
		return *e.config.TurnTimeoutMS
	}
	return e.defaults.TurnTimeoutMS
}

// resolveConfig returns a copy of cfg with nil override pointers replaced
// by the corresponding server-wide defaults. Game adapters receive a fully
// resolved config so they do not need to know the default values.
func resolveConfig(cfg Config, defaults DefaultDelays) Config {
	resolved := cfg
	if resolved.AIActionDelayMS == nil {
		v := defaults.AIActionDelayMS
		resolved.AIActionDelayMS = &v
	}
	if resolved.DealDisplayDelayMS == nil {
		v := defaults.DealDisplayDelayMS
		resolved.DealDisplayDelayMS = &v
	}
	if resolved.TurnTimeoutMS == nil {
		v := defaults.TurnTimeoutMS
		resolved.TurnTimeoutMS = &v
	}
	return resolved
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

// buildSeats creates Seat entries with tokens for human seats.
func buildSeats(configs []SeatConfig) ([]Seat, error) {
	seats := make([]Seat, len(configs))
	for i, sc := range configs {
		seats[i] = Seat{Index: i, Type: sc.Type}
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
		return fmt.Errorf("%w: game is required", ErrInvalidConfig)
	}
	if len(cfg.Seats) == 0 {
		return fmt.Errorf("%w: at least one seat is required", ErrInvalidConfig)
	}
	for i, s := range cfg.Seats {
		if s.Type != SeatHuman && s.Type != SeatAI {
			return fmt.Errorf(
				"%w: seat %d: type must be \"human\" or \"ai\"",
				ErrInvalidConfig, i,
			)
		}
		if s.Type == SeatAI && s.AIType == "" {
			return fmt.Errorf(
				"%w: seat %d: ai_type is required for AI seats",
				ErrInvalidConfig, i,
			)
		}
	}
	return nil
}

// newRNG returns a math/rand/v2.Rand seeded from crypto/rand. If
// crypto/rand fails, it falls back to a time-based seed.
func newRNG() *mathrand.Rand {
	var seed [16]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return mathrand.New(mathrand.NewPCG(uint64(time.Now().UnixNano()), 0))
	}
	s1 := binary.LittleEndian.Uint64(seed[:8])
	s2 := binary.LittleEndian.Uint64(seed[8:])
	return mathrand.New(mathrand.NewPCG(s1, s2))
}
