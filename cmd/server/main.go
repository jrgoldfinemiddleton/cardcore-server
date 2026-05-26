package main

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math/rand/v2"
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

	mgr := session.NewManager(func(cfg session.Config) (session.Game, error) {
		switch cfg.Game {
		case "hearts":
			return heartssession.NewAdapter(cfg.Seats, newRNG())
		default:
			return nil, fmt.Errorf("%w: unknown game: %s", session.ErrInvalidConfig, cfg.Game)
		}
	})

	srv := transport.NewServer(transport.Config{
		Manager: mgr,
	})

	go func() {
		if err := srv.Start(); err != nil {
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
