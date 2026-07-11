package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	heartscli "github.com/jrgoldfinemiddleton/cardcore-server/cmd/cardcore-cli/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
)

const (
	aiTypeRandom   = "random"
	gameNameHearts = "hearts"
	phaseGameOver  = "game_over"
)

var errBrokenPipe = errors.New("broken pipe")

// GameFormatter formats snapshots into compact notation for a specific game.
type GameFormatter interface {
	FormatSnapshot(snapshot []byte) string
}

// cliConfig holds all command-line flag values after parsing and
// validation.
type cliConfig struct {
	// script is the path to the JSON script file.
	script string
	// addr is the server address (e.g., "127.0.0.1:8080").
	addr string
	// game selects which game to play.
	game string
	// observe creates a 4-AI session and connects as an observer.
	observe bool
	// sessionID is an existing session to join.
	sessionID string
	// token is the bearer token for the seat being joined.
	token string
	// seat is the seat index to join.
	seat int
	// deleteOnExit deletes the session after the game ends.
	deleteOnExit bool
	// pacing is the pacing delay in milliseconds.
	pacing int
	// aiType is the AI player type.
	aiType string
	// exitDelay is the duration to wait after game_over before closing.
	exitDelay int
}

// main is the entry point for the cardcore client CLI.
func main() {
	signal.Ignore(syscall.SIGPIPE)

	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if err := run(cfg); err != nil {
		if errors.Is(err, errBrokenPipe) {
			fmt.Fprintln(os.Stderr, "broken pipe")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// parseFlags parses and validates command-line flags.
func parseFlags(args []string) (*cliConfig, error) {
	cfg := &cliConfig{}

	fs := flag.NewFlagSet("cardcore-cli", flag.ContinueOnError)
	fs.StringVar(&cfg.script, "script",
		envOrDefault("CARDCORE_CLI_SCRIPT", ""),
		"path to JSON script file (env: CARDCORE_CLI_SCRIPT)")
	fs.StringVar(&cfg.addr, "addr",
		envOrDefault("CARDCORE_CLI_ADDR", "http://127.0.0.1:8080"),
		"server address (env: CARDCORE_CLI_ADDR)")
	fs.StringVar(&cfg.game, "game",
		envOrDefault("CARDCORE_CLI_GAME", gameNameHearts),
		"game to play (env: CARDCORE_CLI_GAME)")
	fs.BoolVar(&cfg.observe, "observe",
		boolEnvOrDefault("CARDCORE_CLI_OBSERVE", false),
		"create 4-AI session and observe (env: CARDCORE_CLI_OBSERVE)")
	fs.StringVar(&cfg.sessionID, "session-id",
		envOrDefault("CARDCORE_CLI_SESSION_ID", ""),
		"existing session ID to join (env: CARDCORE_CLI_SESSION_ID)")
	fs.StringVar(&cfg.token, "token",
		envOrDefault("CARDCORE_CLI_TOKEN", ""),
		"bearer token for the seat being joined (env: CARDCORE_CLI_TOKEN)")
	fs.IntVar(&cfg.seat, "seat",
		intEnvOrDefault("CARDCORE_CLI_SEAT", 0),
		"seat index to join (0-based) (env: CARDCORE_CLI_SEAT)")
	fs.BoolVar(&cfg.deleteOnExit, "delete-on-exit",
		boolEnvOrDefault("CARDCORE_CLI_DELETE_ON_EXIT", false),
		"delete session on exit (env: CARDCORE_CLI_DELETE_ON_EXIT)")
	fs.IntVar(&cfg.pacing, "pacing",
		intEnvOrDefault("CARDCORE_CLI_PACING_MS", 500),
		"pacing delay in milliseconds (env: CARDCORE_CLI_PACING_MS)")
	fs.StringVar(&cfg.aiType, "ai-type",
		envOrDefault("CARDCORE_CLI_AI_TYPE", aiTypeRandom),
		"AI player type (env: CARDCORE_CLI_AI_TYPE)")
	fs.IntVar(&cfg.exitDelay, "exit-delay",
		intEnvOrDefault("CARDCORE_CLI_EXIT_DELAY_MS", 1000),
		"exit delay in milliseconds (env: CARDCORE_CLI_EXIT_DELAY_MS)")

	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "Usage: %s [flags]\n\n", fs.Name())
		_, _ = fmt.Fprintln(fs.Output(), "Flags:")
		fs.PrintDefaults()
		_, _ = fmt.Fprintln(fs.Output(), "\nAll flags can also be set via the corresponding")
		_, _ = fmt.Fprintln(fs.Output(), "CARDCORE_CLI_* environment variable.")
		_, _ = fmt.Fprintln(fs.Output(), "Explicit flags take precedence over environment")
		_, _ = fmt.Fprintln(fs.Output(), "variables.")
	}

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if cfg.script == "" && !cfg.observe {
		return nil, fmt.Errorf("-script is required (or use -observe)")
	}
	if cfg.observe && cfg.sessionID != "" {
		return nil, fmt.Errorf("-observe and -session-id are mutually exclusive")
	}
	if cfg.sessionID != "" && cfg.token == "" {
		return nil, fmt.Errorf("-token is required when -session-id is set")
	}
	if cfg.sessionID == "" && cfg.token != "" {
		return nil, fmt.Errorf("-session-id is required when -token is set")
	}
	if cfg.seat < 0 {
		return nil, fmt.Errorf("-seat must be >= 0")
	}
	if cfg.pacing < 0 {
		return nil, fmt.Errorf("-pacing must be >= 0")
	}
	if cfg.exitDelay < 0 {
		return nil, fmt.Errorf("-exit-delay must be >= 0")
	}

	return cfg, nil
}

// run executes the CLI based on the parsed configuration.
func run(cfg *cliConfig) error {
	ctx := context.Background()

	sc := &client.SessionClient{BaseURL: cfg.addr}

	var (
		sessionID string
		token     string
		mySeat    int
		created   bool
	)

	switch {
	case cfg.sessionID != "":
		// Join mode: connect to an existing session.
		sessionID = cfg.sessionID
		token = cfg.token
		mySeat = cfg.seat

	case cfg.observe:
		// Observer mode: create a 4-AI session.
		var err error
		sessionID, _, err = createObserverSession(ctx, sc, cfg.game, cfg.aiType, cfg.pacing)
		if err != nil {
			return err
		}
		created = true

	default:
		// Auto-create human player: 1 human + 3 AI, seat 0.
		var err error
		sessionID, token, err = createHumanSession(ctx, sc, cfg.game, cfg.aiType, cfg.pacing)
		if err != nil {
			return err
		}
		mySeat = 0
		created = true
	}

	if created {
		if err := sc.StartSession(ctx, sessionID); err != nil {
			return fmt.Errorf("start session: %w", err)
		}
	}

	// Connect to WebSocket.
	var wsPath string
	if cfg.observe {
		wsPath = "/ws/observe"
	} else {
		wsPath = "/ws"
	}
	url := client.WebSocketURL(cfg.addr, sessionID, wsPath)

	conn := &client.Conn{}
	if err := conn.Connect(ctx, url, token); err != nil {
		return fmt.Errorf("connect websocket: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Resolve game-specific formatter and builder.
	formatter, err := newGameFormatter(cfg.game)
	if err != nil {
		return err
	}

	// Game loop.
	if cfg.observe {
		return runObserver(ctx, conn, formatter, cfg.exitDelay)
	}

	builder, err := newGameBuilder(cfg.game)
	if err != nil {
		return err
	}

	// Player mode: read and execute the script.
	scriptData, err := os.ReadFile(cfg.script)
	if err != nil {
		return fmt.Errorf("read script: %w", err)
	}

	script, err := parseScript(scriptData)
	if err != nil {
		return fmt.Errorf("parse script: %w", err)
	}

	return runPlayer(
		ctx, conn, script, mySeat, cfg.deleteOnExit,
		sc, sessionID, builder, cfg.exitDelay,
	)
}

// runObserver reads snapshots until game_over and prints each snapshot
// in compact notation, followed by final scores.
func runObserver(
	ctx context.Context,
	conn *client.Conn,
	formatter GameFormatter,
	exitDelay int,
) error {
	for {
		snapshot, err := conn.ReadSnapshot(ctx)
		if err != nil {
			var serverErr *client.ErrorMessage
			if errors.As(err, &serverErr) {
				if serverErr.ErrorCode == client.ErrStaleSeq {
					continue
				}
				if serverErr.ErrorCode == client.ErrGameOver {
					slog.Warn("game over",
						"error_code", serverErr.ErrorCode,
						"message", serverErr.Message)
					time.Sleep(time.Duration(exitDelay) * time.Millisecond)
					return nil
				}
				slog.Error("server error",
					"error_code", serverErr.ErrorCode,
					"message", serverErr.Message)
				return fmt.Errorf("server error %s: %s", serverErr.ErrorCode, serverErr.Message)
			}
			return fmt.Errorf("read snapshot: %w", err)
		}

		line := formatter.FormatSnapshot(snapshot)
		if _, err := fmt.Println(line); err != nil {
			if errors.Is(err, syscall.EPIPE) {
				return errBrokenPipe
			}
			return fmt.Errorf("write stdout: %w", err)
		}

		var env struct {
			Phase string `json:"phase"`
		}
		if err := json.Unmarshal(snapshot, &env); err != nil {
			return fmt.Errorf("unmarshal snapshot: %w", err)
		}

		if env.Phase == phaseGameOver {
			time.Sleep(time.Duration(exitDelay) * time.Millisecond)
			return nil
		}
	}
}

// runPlayer drives the scripted player loop.
func runPlayer(
	ctx context.Context,
	conn *client.Conn,
	script Script,
	mySeat int,
	deleteOnExit bool,
	sc *client.SessionClient,
	sessionID string,
	builder GameBuilder,
	exitDelay int,
) error {
	executor := NewScriptExecutor(script, mySeat, builder)

	for {
		snapshot, err := conn.ReadSnapshot(ctx)
		if err != nil {
			var serverErr *client.ErrorMessage
			if errors.As(err, &serverErr) {
				if serverErr.ErrorCode == client.ErrStaleSeq {
					continue
				}
				if serverErr.ErrorCode == client.ErrGameOver {
					slog.Warn("game over",
						"error_code", serverErr.ErrorCode,
						"message", serverErr.Message)
					if err := printFinalScores(nil); err != nil {
						return err
					}
					if deleteOnExit {
						deleteSession(context.Background(), sc, sessionID)
					}
					time.Sleep(time.Duration(exitDelay) * time.Millisecond)
					return nil
				}
				slog.Error("server error",
					"error_code", serverErr.ErrorCode,
					"message", serverErr.Message)
				return fmt.Errorf("server error %s: %s", serverErr.ErrorCode, serverErr.Message)
			}
			return fmt.Errorf("read snapshot: %w", err)
		}

		cmd, gameOver, err := executor.Step(snapshot)
		if err != nil {
			return fmt.Errorf("script step: %w", err)
		}
		if gameOver {
			if err := printFinalScores(snapshot); err != nil {
				return err
			}
			if deleteOnExit {
				deleteSession(context.Background(), sc, sessionID)
			}
			time.Sleep(time.Duration(exitDelay) * time.Millisecond)
			return nil
		}
		if cmd.Type != "" {
			if err := conn.SendCommand(ctx, cmd); err != nil {
				return fmt.Errorf("send command: %w", err)
			}
		}
	}
}

// createHumanSession creates a session with one human seat and the rest
// AI seats, returning the session ID and the human seat token.
func createHumanSession(
	ctx context.Context,
	sc *client.SessionClient,
	game, aiType string,
	pacing int,
) (string, string, error) {
	switch game {
	case gameNameHearts:
		return heartscli.CreateHumanSession(ctx, sc, aiType, pacing)
	default:
		return "", "", fmt.Errorf("unsupported game: %q", game)
	}
}

// createObserverSession creates an all-AI session for observation.
func createObserverSession(
	ctx context.Context,
	sc *client.SessionClient,
	game, aiType string,
	pacing int,
) (string, []client.SeatInfo, error) {
	switch game {
	case gameNameHearts:
		return heartscli.CreateObserverSession(ctx, sc, aiType, pacing)
	default:
		return "", nil, fmt.Errorf("unsupported game: %q", game)
	}
}

// newGameBuilder returns the command builder for the named game.
func newGameBuilder(game string) (GameBuilder, error) {
	switch game {
	case gameNameHearts:
		return heartscli.NewBuilder(), nil
	default:
		return nil, fmt.Errorf("unsupported game: %q", game)
	}
}

// newGameFormatter returns the snapshot formatter for the named game.
func newGameFormatter(game string) (GameFormatter, error) {
	switch game {
	case gameNameHearts:
		return heartscli.NewFormatter(), nil
	default:
		return nil, fmt.Errorf("unsupported game: %q", game)
	}
}

// deleteSession attempts to delete the session, logging any error.
func deleteSession(ctx context.Context, sc *client.SessionClient, sessionID string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sc.DeleteSession(ctx, sessionID); err != nil {
		fmt.Fprintf(os.Stderr, "warning: delete session: %v\n", err)
	}
}

// boolEnvOrDefault returns true if the environment variable is set to
// "true", "1", "yes", or "on" (case-insensitive); otherwise it returns
// defaultValue.
func boolEnvOrDefault(envVar string, defaultValue bool) bool {
	if v := os.Getenv(envVar); v != "" {
		switch strings.ToLower(v) {
		case "true", "1", "yes", "on":
			return true
		default:
			return false
		}
	}
	return defaultValue
}

// envOrDefault returns the environment variable value if set and non-empty,
// otherwise the default.
func envOrDefault(envVar, defaultValue string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return defaultValue
}

// intEnvOrDefault returns the environment variable value parsed as an int if
// set and valid (>= 0), otherwise the default.
func intEnvOrDefault(envVar string, defaultValue int) int {
	if v := os.Getenv(envVar); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d >= 0 {
			return d
		}
	}
	return defaultValue
}
