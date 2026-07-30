package heartssession

import (
	"flag"
	"fmt"
	"math/rand/v2"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/flags"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// GameName is the canonical Hearts game name used by the server-side
// adapter.
const GameName = "hearts"

// GameConfig implements [session.GameConfig] for Hearts. It owns the
// Hearts-specific command-line flags and creates GameAdapter instances.
type GameConfig struct {
	trickDisplayDelayMS int
	roundDisplayDelayMS int
}

// NewGameConfig creates a Hearts GameConfig with flag defaults.
func NewGameConfig() *GameConfig {
	return &GameConfig{
		trickDisplayDelayMS: 3000,
		roundDisplayDelayMS: 5000,
	}
}

// Name returns the canonical Hearts game name.
func (c *GameConfig) Name() string { return GameName }

// RegisterFlags adds Hearts-specific display delay flags to the server
// flag set.
func (c *GameConfig) RegisterFlags(fs *flag.FlagSet) {
	fs.IntVar(&c.trickDisplayDelayMS, "hearts-trick-display-delay-ms",
		flags.IntEnvOrDefault("CARDCORE_SERVER_HEARTS_TRICK_DISPLAY_DELAY_MS", 3000),
		"Hearts trick delay in ms (env: CARDCORE_SERVER_HEARTS_TRICK_DISPLAY_DELAY_MS)")
	fs.IntVar(&c.roundDisplayDelayMS, "hearts-round-display-delay-ms",
		flags.IntEnvOrDefault("CARDCORE_SERVER_HEARTS_ROUND_DISPLAY_DELAY_MS", 5000),
		"Hearts round delay in ms (env: CARDCORE_SERVER_HEARTS_ROUND_DISPLAY_DELAY_MS)")
}

// Validate checks that the parsed Hearts flag values are non-negative.
func (c *GameConfig) Validate() error {
	if c.trickDisplayDelayMS < 0 || c.roundDisplayDelayMS < 0 {
		return fmt.Errorf(" Hearts delay values must be >= 0")
	}
	return nil
}

// NewGame creates a new Hearts GameAdapter.
func (c *GameConfig) NewGame(
	cfg session.Config, rng *rand.Rand,
) (session.Game, error) {
	return NewGameAdapter(
		cfg.Seats, rng,
		*cfg.DealDisplayDelayMS,
		c.trickDisplayDelayMS,
		c.roundDisplayDelayMS,
	)
}

// ValidateConfig checks game-specific constraints for a Hearts session.
func (c *GameConfig) ValidateConfig(cfg session.Config) error {
	return validateConfig(cfg)
}
