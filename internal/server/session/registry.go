package session

import (
	"flag"
	"fmt"
	"math/rand/v2"
)

// GameConfig is implemented by every game adapter that can be registered
// with the server. It exposes the game name, its own command-line flags,
// runtime validation of those flags, creation of a Game instance, and
// validation of an incoming session configuration.
type GameConfig interface {
	// Name returns the canonical game name (e.g., "hearts").
	Name() string

	// RegisterFlags adds game-specific flags to the provided flag set.
	// The flag names should be prefixed with the game name so they are
	// self-documenting in the server help output.
	RegisterFlags(*flag.FlagSet)

	// Validate checks the parsed flag values for correctness. It is called
	// after the flag set has been parsed.
	Validate() error

	// NewGame creates a new Game instance for an active session.
	NewGame(cfg Config, rng *rand.Rand) (Game, error)

	// ValidateConfig checks game-specific constraints on a session
	// configuration before the session is created.
	ValidateConfig(cfg Config) error
}

// Registry holds GameConfig implementations keyed by game name.
type Registry struct {
	factories map[string]GameConfig
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]GameConfig)}
}

// Register adds a GameConfig to the registry. It panics if a game with
// the same name has already been registered.
func (r *Registry) Register(f GameConfig) {
	if _, ok := r.factories[f.Name()]; ok {
		panic(fmt.Sprintf("game %q already registered", f.Name()))
	}
	r.factories[f.Name()] = f
}

// ValidateConfig validates cfg against the registered game for cfg.Game.
func (r *Registry) ValidateConfig(cfg Config) error {
	f, ok := r.factories[cfg.Game]
	if !ok {
		return fmt.Errorf("%w: unknown game: %s", ErrInvalidConfig, cfg.Game)
	}
	return f.ValidateConfig(cfg)
}

// NewGame creates a Game instance for cfg using the registered game factory.
func (r *Registry) NewGame(cfg Config, rng *rand.Rand) (Game, error) {
	f, ok := r.factories[cfg.Game]
	if !ok {
		return nil, fmt.Errorf("%w: unknown game: %s", ErrInvalidConfig, cfg.Game)
	}
	return f.NewGame(cfg, rng)
}

// RegisterFlags calls RegisterFlags on every registered game.
func (r *Registry) RegisterFlags(fs *flag.FlagSet) {
	for _, f := range r.factories {
		f.RegisterFlags(fs)
	}
}

// Validate calls Validate on every registered game.
func (r *Registry) Validate() error {
	for _, f := range r.factories {
		if err := f.Validate(); err != nil {
			return fmt.Errorf("%s: %w", f.Name(), err)
		}
	}
	return nil
}

// Names returns the sorted list of registered game names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}
