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
	"syscall"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
	heartssession "github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/transport"
)

// main is the entry point for the cardcore-server binary. It creates a
// session manager, starts the HTTP/WebSocket server, and blocks on
// SIGINT or SIGTERM to trigger graceful shutdown.
func main() {
	lvl := new(slog.LevelVar)
	lvl.Set(logLevel())
	opts := &slog.HandlerOptions{Level: lvl}
	logger := slog.New(slog.NewTextHandler(os.Stderr, opts))
	slog.SetDefault(logger)

	aiActionDelay := flag.Int("ai-action-delay",
		intEnvOrDefault("CARDCORE_AI_ACTION_DELAY_MS", 1000),
		"AI action delay in milliseconds")
	dealDisplayDelay := flag.Int("deal-display-delay",
		intEnvOrDefault("CARDCORE_DEAL_DISPLAY_DELAY_MS", 1500),
		"deal display delay in milliseconds")
	turnTimeout := flag.Int("turn-timeout",
		intEnvOrDefault("CARDCORE_TURN_TIMEOUT_MS", 30000),
		"human turn timeout in milliseconds")
	heartsTrickDisplayDelay := flag.Int("hearts-trick-display-delay",
		intEnvOrDefault("CARDCORE_SERVER_HEARTS_TRICK_DISPLAY_DELAY_MS", 3000),
		"Hearts trick display delay in milliseconds")
	heartsRoundDisplayDelay := flag.Int("hearts-round-display-delay",
		intEnvOrDefault("CARDCORE_SERVER_HEARTS_ROUND_DISPLAY_DELAY_MS", 5000),
		"Hearts round display delay in milliseconds")
	flag.Parse()

	mgr := session.NewManager(func(cfg session.Config) (session.Game, error) {
		switch cfg.Game {
		case "hearts":
			return heartssession.NewAdapter(
				cfg.Seats, newRNG(),
				intPtrOrDefault(cfg.DealDisplayDelayMS, *dealDisplayDelay),
				*heartsTrickDisplayDelay,
				*heartsRoundDisplayDelay,
			)
		default:
			return nil, fmt.Errorf("%w: unknown game: %s", session.ErrInvalidConfig, cfg.Game)
		}
	}, session.DefaultDelays{
		AIActionDelayMS:    *aiActionDelay,
		DealDisplayDelayMS: *dealDisplayDelay,
		TurnTimeoutMS:      *turnTimeout,
	})

	srv := transport.NewServer(transport.Config{
		Manager: mgr,
		Addr:    listenAddr(),
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

	timeout := shutdownTimeout()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	slog.Info("shutting down")
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("shutdown complete")
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

// shutdownTimeout returns the duration to wait for graceful shutdown.
// It defaults to 10 seconds and can be overridden via the
// CARDCORE_SHUTDOWN_TIMEOUT environment variable (in seconds).
func shutdownTimeout() time.Duration {
	if v := os.Getenv("CARDCORE_SHUTDOWN_TIMEOUT"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 {
			return time.Duration(d) * time.Second
		}
	}
	return 10 * time.Second
}

// logLevel returns the slog.Level from the CARDCORE_LOG_LEVEL environment
// variable. Supported values: debug, info (default), warn, error.
func logLevel() slog.Level {
	switch os.Getenv("CARDCORE_LOG_LEVEL") {
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

// listenAddr returns the TCP address to listen on from the CARDCORE_ADDR
// environment variable. It defaults to "127.0.0.1:8080".
func listenAddr() string {
	if v := os.Getenv("CARDCORE_ADDR"); v != "" {
		return v
	}
	return "127.0.0.1:8080"
}
