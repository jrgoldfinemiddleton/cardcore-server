package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/flags"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
	heartssession "github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/transport"
)

// serverConfig holds all generic command-line flag values after parsing.
// Game-specific flags are owned by the registered game adapters.
type serverConfig struct {
	addr             string
	logLevel         string
	logFile          string
	shutdownTimeout  int
	aiActionDelay    int
	dealDisplayDelay int
	turnTimeout      int
}

// main is the entry point for the cardcore-server binary. It delegates to
// run() and translates the returned exit code via os.Exit so that all
// cleanup paths (including deferred cancel()) execute before termination.
func main() {
	os.Exit(run())
}

// run creates a session manager, starts the HTTP/WebSocket server, and blocks
// on SIGINT or SIGTERM to trigger graceful shutdown. It returns a process exit
// code: 0 for success, 2 for flag-parsing errors, and 1 for runtime failures.
func run() int {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	return runWithArgsAndSignal(os.Args[1:], sigCh)
}

// runWithArgsAndSignal executes the server lifecycle with the provided
// command-line arguments and signal channel. It is the testable core of run().
func runWithArgsAndSignal(args []string, sigCh <-chan os.Signal) int {
	registry := session.NewRegistry()
	registry.Register(heartssession.NewGameConfig())

	cfg, err := parseFlags(args, registry)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	lvl := new(slog.LevelVar)
	lvl.Set(parseLogLevel(cfg.logLevel))
	opts := &slog.HandlerOptions{Level: lvl}

	var logWriter io.Writer = os.Stderr
	if cfg.logFile != "" {
		f, err := os.OpenFile(cfg.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open log file: %v\n", err)
			return 1
		}
		defer func() { _ = f.Close() }()
		logWriter = f
	}

	logger := slog.New(slog.NewTextHandler(logWriter, opts)).With("component", "server")
	slog.SetDefault(logger)

	mgr := session.NewManager(registry, session.DefaultDelays{
		AIActionDelayMS:    cfg.aiActionDelay,
		DealDisplayDelayMS: cfg.dealDisplayDelay,
		TurnTimeoutMS:      cfg.turnTimeout,
	})

	srv := transport.NewServer(transport.Config{
		Manager: mgr,
		Addr:    cfg.addr,
	})

	startErr := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server start", "error", err)
			startErr <- err
		}
	}()

	select {
	case <-sigCh:
	case err := <-startErr:
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	timeout := time.Duration(cfg.shutdownTimeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	slog.Info("shutting down")
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown", "error", err)
		return 1
	}
	slog.Info("shutdown complete")
	return 0
}

// parseFlags parses command-line flags and returns a populated
// serverConfig. All flags have corresponding CARDCORE_SERVER_* env-var
// fallbacks; explicit flags take precedence over env vars, which take
// precedence over hardcoded defaults. Game-specific flags are registered
// by the registry.
func parseFlags(args []string, registry *session.Registry) (*serverConfig, error) {
	cfg := &serverConfig{}

	fs := flag.NewFlagSet("cardcore-server", flag.ContinueOnError)
	fs.StringVar(&cfg.addr, "addr",
		flags.EnvOrDefault("CARDCORE_SERVER_ADDR", "127.0.0.1:8080"),
		"listen address (env: CARDCORE_SERVER_ADDR)")
	fs.StringVar(&cfg.logLevel, "log-level",
		flags.EnvOrDefault("CARDCORE_SERVER_LOG_LEVEL", "info"),
		"log level: debug, info, warn, error (env: CARDCORE_SERVER_LOG_LEVEL)")
	fs.StringVar(&cfg.logFile, "log-file",
		flags.EnvOrDefault("CARDCORE_SERVER_LOG_FILE", ""),
		"log file path (empty logs to stderr) (env: CARDCORE_SERVER_LOG_FILE)")
	fs.IntVar(&cfg.shutdownTimeout, "shutdown-timeout",
		flags.IntEnvOrDefault("CARDCORE_SERVER_SHUTDOWN_TIMEOUT", 10),
		"graceful shutdown timeout in seconds (env: CARDCORE_SERVER_SHUTDOWN_TIMEOUT)")
	fs.IntVar(&cfg.aiActionDelay, "ai-action-delay-ms",
		flags.IntEnvOrDefault("CARDCORE_SERVER_AI_ACTION_DELAY_MS", 1000),
		"AI action delay in milliseconds (env: CARDCORE_SERVER_AI_ACTION_DELAY_MS)")
	fs.IntVar(&cfg.dealDisplayDelay, "deal-display-delay-ms",
		flags.IntEnvOrDefault("CARDCORE_SERVER_DEAL_DISPLAY_DELAY_MS", 1500),
		"deal display delay in milliseconds (env: CARDCORE_SERVER_DEAL_DISPLAY_DELAY_MS)")
	fs.IntVar(&cfg.turnTimeout, "turn-timeout-ms",
		flags.IntEnvOrDefault("CARDCORE_SERVER_TURN_TIMEOUT_MS", 30000),
		"human turn timeout in milliseconds (env: CARDCORE_SERVER_TURN_TIMEOUT_MS)")

	registry.RegisterFlags(fs)

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
	if cfg.aiActionDelay < 0 || cfg.dealDisplayDelay < 0 || cfg.turnTimeout < 0 {
		return nil, fmt.Errorf("delay values must be >= 0")
	}
	if err := registry.Validate(); err != nil {
		return nil, err
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
