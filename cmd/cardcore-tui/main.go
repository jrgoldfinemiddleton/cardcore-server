package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	heartstui "github.com/jrgoldfinemiddleton/cardcore-server/cmd/cardcore-tui/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
)

// tuiConfig holds all command-line flag values after parsing.
//
// Using a dedicated struct (rather than bare variables) makes it easy to
// pass configuration around and keeps the flag surface area explicit.
type tuiConfig struct {
	// server is the base URL of the cardcore server (e.g., "http://localhost:8080").
	server string
	// game selects which game client to render (e.g., "hearts").
	game string
	// session is the session ID to join. Required for observer mode and when joining as a player.
	session string
	// token is the bearer token for the seat being joined. Required when joining a session.
	token string
	// seat is the player's seat index. The valid range depends on the game.
	seat int
	// observer enables receive-only mode where all hands are visible.
	observer bool
	// debug enables logging to a file (tui.log) for troubleshooting.
	debug bool
}

// main is the entry point for the cardcore TUI client.
//
// The initialization sequence is critical:
//
//  1. Parse flags.
//  2. Create HTTP client (SessionClient) and WebSocket connection (Conn).
//  3. Connect the WebSocket before starting the TUI program.
//  4. Create the Bubble Tea model with the connection.
//  5. Create the program. The model must use a pointer so that setting
//     m.program after tea.NewProgram works (see below).
//  6. Start the WebSocket reader goroutine.
//  7. Run the program. This blocks until the user exits.
//
// Why connect before starting the program? Bubble Tea's event loop runs
// in a single goroutine. If we tried to connect inside the model's Init()
// or Update(), any connection error would require complex error handling
// in the TUI state machine. By connecting first, we can fail fast with a
// clear error message on the terminal.
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
//
// Validation rules:
//
//	-observer requires -session (you can't observe without a session).
//	-seat must be >= 0 (the server validates the upper bound).
//	-token requires -session (a token without a session is meaningless).
func parseFlags() (*tuiConfig, error) {
	cfg := &tuiConfig{}

	flag.StringVar(&cfg.server, "server", "http://localhost:8080", "server base URL")
	flag.StringVar(&cfg.game, "game", "hearts", "game to play")
	flag.StringVar(&cfg.session, "session", "", "session ID to join")
	flag.StringVar(&cfg.token, "token", "", "seat bearer token")
	flag.IntVar(&cfg.seat, "seat", 0, "seat index (game-dependent)")
	flag.BoolVar(&cfg.observer, "observe", false, "observer mode (receive-only)")
	flag.BoolVar(&cfg.debug, "debug", false, "enable debug logging")
	flag.Parse()

	if cfg.observer && cfg.session == "" {
		return nil, fmt.Errorf("-session is required with -observe")
	}
	if cfg.token != "" && cfg.session == "" {
		return nil, fmt.Errorf("-session is required when -token is set")
	}
	if cfg.seat < 0 {
		return nil, fmt.Errorf("-seat must be >= 0")
	}

	return cfg, nil
}

// run executes the TUI lifecycle.
//
// Step 1: Create WebSocket connection.
//
//	Conn wraps the WebSocket and handles message framing, maxSeenSeq
//	filtering (per ADR-011), and command sending.
//
// Step 2: Connect.
//
//	The connection is established before the TUI program starts. This
//	allows us to fail fast on connection errors with a clean terminal
//	message instead of a complex TUI error state.
//
// Step 3: Create model with pointer receiver.
//
//	The model must be a pointer (*model) so that m.program = p works.
//	See the detailed comment below for why this matters.
//
// Step 4: Create program.
//
//	tea.NewProgram takes tea.Model (an interface). When passed a *model,
//	it stores the pointer internally. Any later modification to the
//	underlying struct (like setting m.program) is visible to the program.
//
// Step 5: Set model.program.
//
//	Now that the program exists, we set the model's program reference.
//	The WebSocket reader goroutine will use this to call program.Send().
//
// Step 6: Start WebSocket reader.
//
//	The goroutine reads snapshots from the WebSocket and injects them
//	into the model via program.Send().
//
// Step 7: Run.
//
//	p.Run() blocks until the user exits (ctrl+c, game over, etc.).
//	Cleanup is handled by defer on the connection.
func run(cfg *tuiConfig) error {
	// Step 1: Create WebSocket connection.
	//
	// Conn is created but not connected yet. The Connect method establishes
	// the WebSocket handshake and returns the initial snapshot.
	conn := &client.Conn{}

	// Step 2: Connect.
	//
	// Construct the WebSocket URL from the base URL, session ID, and path.
	// The wsURL helper converts http:// to ws:// and appends the session path.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wsPath string
	if cfg.observer {
		wsPath = "/ws/observe"
	} else {
		wsPath = "/ws"
	}
	url := wsURL(cfg.server, cfg.session, wsPath)

	if err := conn.Connect(ctx, url, cfg.token); err != nil {
		return fmt.Errorf("websocket connect: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Step 3: Create model with pointer receiver.
	//
	// CRITICAL: The model must be a pointer (*model).
	//
	// Bubble Tea's tea.NewProgram takes a tea.Model interface. When you pass
	// a value (not a pointer), Go copies the value into the interface. The
	// program stores its own copy. If you later set m.program = p, you're
	// modifying the LOCAL variable, not the copy inside the program. The
	// WebSocket goroutine will call program.Send() on a nil pointer, causing
	// a panic.
	//
	// When you pass a pointer (*model), the program stores the pointer
	// (interface{} holds the pointer value). The program and your local
	// variable both point to the same struct. Setting m.program = p modifies
	// the shared struct, so the goroutine sees the correct program reference.
	game, err := newGameClient(cfg.game, cfg.seat, cfg.observer)
	if err != nil {
		return err
	}
	m := &model{
		conn:  conn,
		game:  game,
		phase: "connecting",
	}

	// Step 4: Create program.
	p := tea.NewProgram(m)

	// Step 5: Set model.program.
	//
	// Now that p exists, store the reference in the model. Because m is a
	// pointer, this modification is visible to the program.
	m.program = p

	// Step 6: Start WebSocket reader goroutine.
	//
	// The goroutine reads from the WebSocket and sends messages into the
	// model via program.Send(). This is safe because Program.Send() is
	// thread-safe — it can be called from any goroutine.
	go startWSReader(ctx, conn, p)

	// Step 7: Run.
	//
	// p.Run() blocks until the user exits. The model's Update() handles
	// all messages (snapshots, errors, keypresses, timers).
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	return nil
}

// newGameClient constructs the game-specific client for the named game. It is
// the single composition point where concrete games are wired into the
// game-agnostic model; add new games by extending the switch.
func newGameClient(game string, seat int, observer bool) (gameClient, error) {
	switch game {
	case "hearts":
		return heartstui.NewClient(seat, observer), nil
	default:
		return nil, fmt.Errorf("unsupported game: %q", game)
	}
}

// wsURL converts an HTTP base URL to a WebSocket URL for a session.
//
// It replaces http:// with ws:// and https:// with wss://, then appends
// the session path.
func wsURL(baseURL, sessionID, path string) string {
	u := strings.TrimSuffix(baseURL, "/")
	u = strings.Replace(u, "http://", "ws://", 1)
	u = strings.Replace(u, "https://", "wss://", 1)
	return fmt.Sprintf("%s/sessions/%s%s", u, sessionID, path)
}
