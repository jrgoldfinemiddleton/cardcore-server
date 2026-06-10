package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
)

const aiTypeRandom = "random"

// cliConfig holds all command-line flag values after parsing and
// validation.
type cliConfig struct {
	// script is the path to the JSON script file.
	script string
	// baseURL is the server base URL (e.g., "http://localhost:8080").
	baseURL string
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
}

// main is the entry point for the cardcore client CLI.
func main() {
	cfg, err := parseFlags()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// parseFlags parses and validates command-line flags.
func parseFlags() (*cliConfig, error) {
	cfg := &cliConfig{}

	flag.StringVar(&cfg.script, "script", "", "path to JSON script file (required)")
	flag.StringVar(&cfg.baseURL, "base-url", "http://localhost:8080", "server base URL")
	flag.BoolVar(&cfg.observe, "observe", false, "create 4-AI session and observe")
	flag.StringVar(&cfg.sessionID, "session-id", "", "existing session ID to join")
	flag.StringVar(&cfg.token, "token", "", "bearer token for the seat being joined")
	flag.IntVar(&cfg.seat, "seat", 0, "seat index to join (0-based)")
	flag.BoolVar(&cfg.deleteOnExit, "delete-on-exit", false, "delete session on exit")
	flag.Parse()

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

	return cfg, nil
}

// run executes the CLI based on the parsed configuration.
func run(cfg *cliConfig) error {
	ctx := context.Background()

	sc := &client.SessionClient{BaseURL: cfg.baseURL}

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
		sessionID, _, err = createObserverSession(ctx, sc)
		if err != nil {
			return err
		}
		created = true

	default:
		// Auto-create human player: 1 human + 3 AI, seat 0.
		var err error
		sessionID, token, err = createHumanSession(ctx, sc)
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
	url := wsURL(cfg.baseURL, sessionID, wsPath)

	conn := &client.Conn{}
	if err := conn.Connect(ctx, url, token); err != nil {
		return fmt.Errorf("connect websocket: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Game loop.
	if cfg.observe {
		return runObserver(ctx, conn)
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

	return runPlayer(ctx, conn, script, mySeat, cfg.deleteOnExit, sc, sessionID)
}

// runObserver reads snapshots until game_over and prints final scores.
func runObserver(ctx context.Context, conn *client.Conn) error {
	for {
		snapshot, err := conn.ReadSnapshot(ctx)
		if err != nil {
			return fmt.Errorf("read snapshot: %w", err)
		}

		var env struct {
			Phase string `json:"phase"`
		}
		if err := json.Unmarshal(snapshot, &env); err != nil {
			return fmt.Errorf("unmarshal snapshot: %w", err)
		}

		if env.Phase == "game_over" {
			printFinalScores(snapshot)
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
) error {
	executor := NewScriptExecutor(script, mySeat)

	for {
		snapshot, err := conn.ReadSnapshot(ctx)
		if err != nil {
			return fmt.Errorf("read snapshot: %w", err)
		}

		cmd, gameOver, err := executor.Step(snapshot)
		if err != nil {
			return fmt.Errorf("script step: %w", err)
		}
		if gameOver {
			printFinalScores(snapshot)
			if deleteOnExit {
				deleteSession(context.Background(), sc, sessionID)
			}
			return nil
		}
		if cmd.Type != "" {
			if err := conn.SendCommand(ctx, cmd); err != nil {
				return fmt.Errorf("send command: %w", err)
			}
		}
	}
}

// createHumanSession creates a session with one human seat and three
// AI seats, returning the session ID and the human seat token.
// The human seat is always index 0.
func createHumanSession(
	ctx context.Context,
	sc *client.SessionClient,
) (string, string, error) {
	pacing := 500 // default pacing for human play
	cfg := client.Config{
		Game: "hearts",
		Seats: []client.SeatConfig{
			{Type: "human"},
			{Type: "ai", AIType: aiTypeRandom},
			{Type: "ai", AIType: aiTypeRandom},
			{Type: "ai", AIType: aiTypeRandom},
		},
		PacingDelayMS: &pacing,
	}

	id, seats, err := sc.CreateSession(ctx, cfg)
	if err != nil {
		return "", "", fmt.Errorf("create session: %w", err)
	}

	const humanSeat = 0
	token := seats[humanSeat].Token
	if token == "" {
		return "", "", fmt.Errorf("no human seat token in create response")
	}

	return id, token, nil
}

// createObserverSession creates a 4-AI session for observation.
func createObserverSession(
	ctx context.Context,
	sc *client.SessionClient,
) (string, []client.SeatInfo, error) {
	pacing := 500
	cfg := client.Config{
		Game: "hearts",
		Seats: []client.SeatConfig{
			{Type: "ai", AIType: aiTypeRandom},
			{Type: "ai", AIType: aiTypeRandom},
			{Type: "ai", AIType: aiTypeRandom},
			{Type: "ai", AIType: aiTypeRandom},
		},
		PacingDelayMS: &pacing,
	}

	return sc.CreateSession(ctx, cfg)
}

// deleteSession attempts to delete the session, logging any error.
func deleteSession(ctx context.Context, sc *client.SessionClient, sessionID string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sc.DeleteSession(ctx, sessionID); err != nil {
		fmt.Fprintf(os.Stderr, "warning: delete session: %v\n", err)
	}
}
