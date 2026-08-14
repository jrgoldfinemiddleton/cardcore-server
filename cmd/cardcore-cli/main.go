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
	"syscall"
	"time"

	heartscli "github.com/jrgoldfinemiddleton/cardcore-server/cmd/cardcore-cli/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/flags"
)

const (
	aiTypeRandom  = "random"
	phaseGameOver = "game_over"
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
	// addr is the server base URL, scheme included (e.g., "http://127.0.0.1:8080").
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
	// deleteOnExit deletes the session after the game ends (player mode only;
	// observer mode never deletes).
	deleteOnExit bool
	// pacing is the delay between AI turns in milliseconds. It is applied
	// only when this command creates the session (auto-create and observe
	// modes); it is ignored when joining an existing session.
	pacing int
	// aiType is the AI player type.
	aiType string
	// exitDelay is the time to wait after game_over before exiting, in
	// milliseconds.
	exitDelay int
}

// main is the entry point for the cardcore client CLI.
func main() {
	signal.Ignore(syscall.SIGPIPE)

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)).With("component", "cli"))

	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
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
		flags.EnvOrDefault("CARDCORE_CLI_SCRIPT", ""),
		"path to the JSON script file of actions to execute "+
			"(required unless -observe) (env: CARDCORE_CLI_SCRIPT)")
	fs.StringVar(&cfg.addr, "addr",
		flags.EnvOrDefault("CARDCORE_CLI_ADDR", "http://127.0.0.1:8080"),
		"base URL of the cardcore server (env: CARDCORE_CLI_ADDR)")
	fs.StringVar(&cfg.game, "game",
		flags.EnvOrDefault("CARDCORE_CLI_GAME", heartsclient.GameName),
		"game identifier, e.g. hearts (env: CARDCORE_CLI_GAME)")
	fs.BoolVar(&cfg.observe, "observe",
		flags.BoolEnvOrDefault("CARDCORE_CLI_OBSERVE", false),
		"create an all-AI session and watch it play (receive-only) "+
			"(env: CARDCORE_CLI_OBSERVE)")
	fs.StringVar(&cfg.sessionID, "session-id",
		flags.EnvOrDefault("CARDCORE_CLI_SESSION_ID", ""),
		"join this existing session ID instead of creating a new one "+
			"(env: CARDCORE_CLI_SESSION_ID)")
	fs.StringVar(&cfg.token, "token",
		flags.EnvOrDefault("CARDCORE_CLI_TOKEN", ""),
		"bearer token for the seat being joined, issued at session "+
			"creation (env: CARDCORE_CLI_TOKEN)")
	fs.IntVar(&cfg.seat, "seat",
		flags.IntEnvOrDefault("CARDCORE_CLI_SEAT", 0),
		"seat index to join (0-based) (env: CARDCORE_CLI_SEAT)")
	fs.BoolVar(&cfg.deleteOnExit, "delete-on-exit",
		flags.BoolEnvOrDefault("CARDCORE_CLI_DELETE_ON_EXIT", false),
		"delete the session from the server when the game ends "+
			"(player mode only) (env: CARDCORE_CLI_DELETE_ON_EXIT)")
	fs.IntVar(&cfg.pacing, "pacing-delay-ms",
		flags.IntEnvOrDefault("CARDCORE_CLI_PACING_DELAY_MS", 500),
		"delay in milliseconds before each AI turn; applies only when "+
			"this command creates the session (env: CARDCORE_CLI_PACING_DELAY_MS)")
	fs.StringVar(&cfg.aiType, "ai-type",
		flags.EnvOrDefault("CARDCORE_CLI_AI_TYPE", aiTypeRandom),
		"AI implementation for bot seats and the human fallback: "+
			"random, heuristic, or pimc (env: CARDCORE_CLI_AI_TYPE)")
	fs.IntVar(&cfg.exitDelay, "exit-delay-ms",
		flags.IntEnvOrDefault("CARDCORE_CLI_EXIT_DELAY_MS", 1000),
		"time in milliseconds to wait after game_over before exiting "+
			"(env: CARDCORE_CLI_EXIT_DELAY_MS)")

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
		return nil, fmt.Errorf("-pacing-delay-ms must be >= 0")
	}
	if cfg.exitDelay < 0 {
		return nil, fmt.Errorf("-exit-delay-ms must be >= 0")
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
		sessionID, _, err = createSession(ctx, sc, cfg.game, cfg.aiType, true, cfg.pacing)
		if err != nil {
			return err
		}

	default:
		// Auto-create human player: 1 human + 3 AI, seat 0.
		var err error
		sessionID, token, err = createSession(ctx, sc, cfg.game, cfg.aiType, false, cfg.pacing)
		if err != nil {
			return err
		}
		mySeat = 0
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

	// Resolve the game-specific formatter (the builder is resolved below,
	// in player mode only).
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
// in compact notation. Unlike runPlayer it prints no separate scores line:
// the game_over snapshot's own formatted line already carries the scores.
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
					// ReadSnapshot already advanced maxSeenSeq to the error's
					// current_seq, so the stream is resynced; keep reading.
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
					// The server sent the latest snapshot immediately before
					// this error and ReadSnapshot advanced maxSeenSeq to the
					// error's current_seq, so the stream is already resynced.
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

// createSession creates and starts a session with the given game.
func createSession(
	ctx context.Context,
	sc *client.SessionClient,
	game, aiType string,
	observer bool,
	pacing int,
) (sessionID, token string, err error) {
	// pacing maps to the session's AI action delay. The deal display delay
	// is forced to 0: the CLI is scripted and has no interactive display
	// to pace.
	zero := 0
	switch game {
	case heartsclient.GameName:
		id, token, _, err := heartscli.CreateSession(
			ctx, sc, aiType, observer, &pacing, &zero,
		)
		return id, token, err
	default:
		return "", "", fmt.Errorf("unsupported game: %q", game)
	}
}

// newGameBuilder returns the command builder for the named game.
func newGameBuilder(game string) (GameBuilder, error) {
	switch game {
	case heartsclient.GameName:
		return heartscli.NewBuilder(), nil
	default:
		return nil, fmt.Errorf("unsupported game: %q", game)
	}
}

// newGameFormatter returns the snapshot formatter for the named game.
func newGameFormatter(game string) (GameFormatter, error) {
	switch game {
	case heartsclient.GameName:
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
