package main

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
	heartssession "github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/transport"
)

// serverConfig holds all command-line flag values after parsing.
type serverConfig struct {
	addr                    string
	logLevel                string
	shutdownTimeout         int
	aiActionDelay           int
	dealDisplayDelay        int
	turnTimeout             int
	heartsTrickDisplayDelay int
	heartsRoundDisplayDelay int
}

// main is the entry point for the cardcore-server binary. It creates a
// session manager, starts the HTTP/WebSocket server, and blocks on
// SIGINT or SIGTERM to trigger graceful shutdown.
func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	lvl := new(slog.LevelVar)
	lvl.Set(parseLogLevel(cfg.logLevel))
	opts := &slog.HandlerOptions{Level: lvl}
	logger := slog.New(slog.NewTextHandler(os.Stderr, opts))
	slog.SetDefault(logger)

	mgr := session.NewManager(func(sessionCfg session.Config) (session.Game, error) {
		switch sessionCfg.Game {
		case "hearts":
			return heartssession.NewAdapter(
				sessionCfg.Seats, newRNG(),
				intPtrOrDefault(sessionCfg.DealDisplayDelayMS, cfg.dealDisplayDelay),
				cfg.heartsTrickDisplayDelay,
				cfg.heartsRoundDisplayDelay,
			)
		default:
			return nil, fmt.Errorf("%w: unknown game: %s",
				session.ErrInvalidConfig, sessionCfg.Game)
		}
	}, session.DefaultDelays{
		AIActionDelayMS:    cfg.aiActionDelay,
		DealDisplayDelayMS: cfg.dealDisplayDelay,
		TurnTimeoutMS:      cfg.turnTimeout,
	})

	srv := transport.NewServer(transport.Config{
		Manager: mgr,
		Addr:    cfg.addr,
	})

	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server start", "error", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	timeout := time.Duration(cfg.shutdownTimeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	slog.Info("shutting down")
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("shutdown complete")
}

// parseFlags parses command-line flags and returns a populated
// serverConfig. All flags have corresponding CARDCORE_SERVER_* env-var
// fallbacks; explicit flags take precedence over env vars, which take
// precedence over hardcoded defaults.
func parseFlags(args []string) (*serverConfig, error) {
	cfg := &serverConfig{}

	fs := flag.NewFlagSet("cardcore-server", flag.ContinueOnError)
	fs.StringVar(&cfg.addr, "addr",
		envOrDefault("CARDCORE_SERVER_ADDR", "127.0.0.1:8080"),
		"listen address (env: CARDCORE_SERVER_ADDR)")
	fs.StringVar(&cfg.logLevel, "log-level",
		envOrDefault("CARDCORE_SERVER_LOG_LEVEL", "info"),
		"log level: debug, info, warn, error (env: CARDCORE_SERVER_LOG_LEVEL)")
	fs.IntVar(&cfg.shutdownTimeout, "shutdown-timeout",
		intEnvOrDefault("CARDCORE_SERVER_SHUTDOWN_TIMEOUT", 10),
		"graceful shutdown timeout in seconds (env: CARDCORE_SERVER_SHUTDOWN_TIMEOUT)")
	fs.IntVar(&cfg.aiActionDelay, "ai-action-delay",
		intEnvOrDefault("CARDCORE_SERVER_AI_ACTION_DELAY_MS", 1000),
		"AI action delay in milliseconds (env: CARDCORE_SERVER_AI_ACTION_DELAY_MS)")
	fs.IntVar(&cfg.dealDisplayDelay, "deal-display-delay",
		intEnvOrDefault("CARDCORE_SERVER_DEAL_DISPLAY_DELAY_MS", 1500),
		"deal display delay in milliseconds (env: CARDCORE_SERVER_DEAL_DISPLAY_DELAY_MS)")
	fs.IntVar(&cfg.turnTimeout, "turn-timeout",
		intEnvOrDefault("CARDCORE_SERVER_TURN_TIMEOUT_MS", 30000),
		"human turn timeout in milliseconds (env: CARDCORE_SERVER_TURN_TIMEOUT_MS)")
	fs.IntVar(&cfg.heartsTrickDisplayDelay, "hearts-trick-display-delay",
		intEnvOrDefault("CARDCORE_SERVER_HEARTS_TRICK_DISPLAY_DELAY_MS", 3000),
		"Hearts trick delay in ms (env: CARDCORE_SERVER_HEARTS_TRICK_DISPLAY_DELAY_MS)")
	fs.IntVar(&cfg.heartsRoundDisplayDelay, "hearts-round-display-delay",
		intEnvOrDefault("CARDCORE_SERVER_HEARTS_ROUND_DISPLAY_DELAY_MS", 5000),
		"Hearts round delay in ms (env: CARDCORE_SERVER_HEARTS_ROUND_DISPLAY_DELAY_MS)")

	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "Usage: %s [flags]\n\n", fs.Name())
		_, _ = fmt.Fprintln(fs.Output(), "Flags:")
		fs.PrintDefaults()
		_, _ = fmt.Fprintln(fs.Output(), "\nAll flags can also be set via the corresponding")
		_, _ = fmt.Fprintln(fs.Output(), "CARDCORE_SERVER_* environment variable.")
		_, _ = fmt.Fprintln(fs.Output(), "Explicit flags take precedence over environment")
		_, _ = fmt.Fprintln(fs.Output(), "variables.")
	}

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if cfg.shutdownTimeout <= 0 {
		return nil, fmt.Errorf("-shutdown-timeout must be > 0")
	}
	if cfg.aiActionDelay < 0 || cfg.dealDisplayDelay < 0 || cfg.turnTimeout < 0 ||
		cfg.heartsTrickDisplayDelay < 0 || cfg.heartsRoundDisplayDelay < 0 {
		return nil, fmt.Errorf("delay values must be >= 0")
	}

	return cfg, nil
}

// parseLogLevel returns the slog.Level for the given string.
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// envOrDefault returns the environment variable value if set and non-empty,
// otherwise the default.
func envOrDefault(envVar string, defaultValue string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return defaultValue
}

// intEnvOrDefault returns the environment variable value parsed as an
// int if set and valid (>= 0), otherwise the default.
func intEnvOrDefault(envVar string, defaultValue int) int {
	if v := os.Getenv(envVar); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d >= 0 {
			return d
		}
	}
	return defaultValue
}

// intPtrOrDefault returns the value pointed to by p, or defaultValue
// if p is nil.
func intPtrOrDefault(p *int, defaultValue int) int {
	if p != nil {
		return *p
	}
	return defaultValue
}

// newRNG returns a math/rand/v2.Rand seeded from crypto/rand. If
// crypto/rand fails, it falls back to a time-based seed.
func newRNG() *rand.Rand {
	var seed [16]byte
	if _, err := crand.Read(seed[:]); err != nil {
		return rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))
	}
	s1 := binary.LittleEndian.Uint64(seed[:8])
	s2 := binary.LittleEndian.Uint64(seed[8:])
	return rand.New(rand.NewPCG(s1, s2))
}
