package testutil

import (
	"flag"
	"hash/fnv"
	"math/rand/v2"
	"testing"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
	heartssession "github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session/games/hearts"
)

// testGameName is the game name used by all Hearts test helpers.
const testGameName = "hearts"

// testAIType is the AI type used by all Hearts test helpers.
const testAIType = "random"

// TestHeartsConfig is a [session.GameConfig] for integration tests that
// creates real Hearts adapters with a deterministic RNG.
type TestHeartsConfig struct {
	// Rng is the deterministic random source passed to every Hearts
	// adapter this config creates, replacing the session-provided RNG so
	// tests are reproducible.
	Rng *rand.Rand
}

// Name returns "hearts" so the test config registers under the same name
// as the production Hearts config.
func (c *TestHeartsConfig) Name() string { return testGameName }

// RegisterFlags is a no-op; test configs are created programmatically and
// take no flags.
func (c *TestHeartsConfig) RegisterFlags(*flag.FlagSet) {}

// Validate is a no-op; test configs have no flag values to check since
// they are created programmatically.
func (c *TestHeartsConfig) Validate() error { return nil }

// NewGame creates a real Hearts adapter using the test config's
// deterministic RNG and zero display delays, so tests get reproducible
// games without pacing overhead.
func (c *TestHeartsConfig) NewGame(cfg session.Config, _ *rand.Rand) (session.Game, error) {
	return heartssession.NewGameAdapter(cfg.Seats, c.Rng, 0, 0, 0)
}

// ValidateConfig delegates to the production Hearts config validator so
// test sessions enforce the same seat-validation rules as production
// sessions.
func (c *TestHeartsConfig) ValidateConfig(cfg session.Config) error {
	return heartssession.NewGameConfig().ValidateConfig(cfg)
}

// HeartsRegistry returns a session registry containing a real Hearts game
// config with a deterministic RNG seeded from the test name.
func HeartsRegistry(t *testing.T) *session.Registry {
	t.Helper()
	seed := HashTestName(t.Name())
	rng := rand.New(rand.NewPCG(seed, seed+1))
	registry := session.NewRegistry()
	registry.Register(&TestHeartsConfig{Rng: rng})
	return registry
}

// HashTestName converts a test name string into a deterministic uint64
// seed for the RNG.
func HashTestName(name string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return h.Sum64()
}

// HumanToken extracts the bearer token for the first human seat from the
// seat list returned by a session creation.
func HumanToken(t *testing.T, seats []client.SeatInfo) string {
	t.Helper()
	for _, s := range seats {
		if s.Type == "human" {
			return s.Token
		}
	}
	t.Fatal("no human seat token found")
	return ""
}

// HumanSessionToken extracts the bearer token for the first human seat from
// the session seat list returned by [session.Manager.Create].
func HumanSessionToken(t *testing.T, seats []session.Seat) string {
	t.Helper()
	for _, s := range seats {
		if s.Type == session.SeatHuman {
			return s.Token
		}
	}
	t.Fatal("no human seat token found")
	return ""
}

// HeartsConfigWithPacing returns a 1-human+3-AI Hearts [client.Config] with
// the given AI pacing delay.
func HeartsConfigWithPacing(pacingMS int) client.Config {
	return client.Config{
		Game: testGameName,
		Seats: []client.SeatConfig{
			{Type: "human"},
			{Type: "ai", AIType: testAIType},
			{Type: "ai", AIType: testAIType},
			{Type: "ai", AIType: testAIType},
		},
		AIActionDelayMS: &pacingMS,
	}
}

// HeartsAllAIConfigWithPacing returns a 4-AI Hearts [client.Config] with the
// given AI pacing delay.
func HeartsAllAIConfigWithPacing(pacingMS int) client.Config {
	return client.Config{
		Game: testGameName,
		Seats: []client.SeatConfig{
			{Type: "ai", AIType: testAIType},
			{Type: "ai", AIType: testAIType},
			{Type: "ai", AIType: testAIType},
			{Type: "ai", AIType: testAIType},
		},
		AIActionDelayMS: &pacingMS,
	}
}

// HeartsSessionConfigWithPacing returns a 1-human+3-AI Hearts [session.Config]
// with the given AI pacing delay.
func HeartsSessionConfigWithPacing(pacingMS int) session.Config {
	return session.Config{
		Game: testGameName,
		Seats: []session.SeatConfig{
			{Type: session.SeatHuman},
			{Type: session.SeatAI, AIType: testAIType},
			{Type: session.SeatAI, AIType: testAIType},
			{Type: session.SeatAI, AIType: testAIType},
		},
		AIActionDelayMS: &pacingMS,
	}
}

// HeartsAllAISessionConfigWithPacing returns a 4-AI Hearts [session.Config] with
// the given AI pacing delay.
func HeartsAllAISessionConfigWithPacing(pacingMS int) session.Config {
	return session.Config{
		Game: testGameName,
		Seats: []session.SeatConfig{
			{Type: session.SeatAI, AIType: testAIType},
			{Type: session.SeatAI, AIType: testAIType},
			{Type: session.SeatAI, AIType: testAIType},
			{Type: session.SeatAI, AIType: testAIType},
		},
		AIActionDelayMS: &pacingMS,
	}
}

// HeartsFourHumanSessionConfig returns a 4-human Hearts [session.Config] with
// zero pacing delay.
func HeartsFourHumanSessionConfig() session.Config {
	delay := 0
	return session.Config{
		Game: testGameName,
		Seats: []session.SeatConfig{
			{Type: session.SeatHuman},
			{Type: session.SeatHuman},
			{Type: session.SeatHuman},
			{Type: session.SeatHuman},
		},
		AIActionDelayMS: &delay,
	}
}
