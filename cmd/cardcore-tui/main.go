package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	heartstui "github.com/jrgoldfinemiddleton/cardcore-server/cmd/cardcore-tui/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/cmd/cardcore-tui/menu"
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
	// aiType selects the AI player type for auto-created sessions.
	aiType string
	// theme selects the color palette: "dark" or "light".
	theme string
	// menuSkipped is true when the user explicitly set a game-related flag,
	// bypassing the interactive menu.
	menuSkipped bool
}

const (
	themeDark  = "dark"
	themeLight = "light"
)

// main is the entry point for the cardcore TUI client.
//
// The initialization sequence is critical:
//
//  1. Parse flags.
//  2. Run the menu to resolve server, game, AI, observer, theme, and debug.
//  3. Create HTTP client (SessionClient) and WebSocket connection (Conn).
//  4. Connect the WebSocket before starting the TUI program.
//  5. Create the Bubble Tea model with the connection.
//  6. Create the program. The model must use a pointer so that setting
//     m.program after tea.NewProgram works (see below).
//  7. Start the WebSocket reader goroutine.
//  8. Run the program. This blocks until the user exits.
//
// Why connect before starting the program? Bubble Tea's event loop runs
// in a single goroutine. If we tried to connect inside the model's Init()
// or Update(), any connection error would require complex error handling
// in the TUI state machine. By connecting first, we can fail fast with a
// clear error message on the terminal.
func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	resolvedCfg, err := runMenu(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if resolvedCfg == nil {
		return
	}

	if err := runGame(resolvedCfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// parseFlags parses and validates command-line flags.
//
// Validation rules:
//
//	-observe and -session are mutually exclusive.
//	-session requires -token (a session without a token is meaningless).
//	-token requires -session (a token without a session is meaningless).
//	-seat must be >= 0 (the server validates the upper bound).
//
// The -ai-type value is passed through to the game-specific session helper,
// which validates it against the supported types for the selected game. This
// keeps the top-level TUI binary game-agnostic.
//
// When neither -session nor -observe is provided, the TUI auto-creates a
// session: 1 human + 3 AI for player mode, or 4 AI for observer mode.
func parseFlags(args []string) (*tuiConfig, error) {
	cfg := &tuiConfig{}

	fs := flag.NewFlagSet("cardcore-tui", flag.ContinueOnError)
	fs.StringVar(&cfg.server, "server",
		envOrDefault("CARDCORE_TUI_SERVER", "http://localhost:8080"),
		"server base URL (env: CARDCORE_TUI_SERVER)")
	fs.StringVar(&cfg.game, "game",
		envOrDefault("CARDCORE_TUI_GAME", "hearts"),
		"game to play (env: CARDCORE_TUI_GAME)")
	fs.StringVar(&cfg.session, "session",
		envOrDefault("CARDCORE_TUI_SESSION", ""),
		"session ID to join (env: CARDCORE_TUI_SESSION)")
	fs.StringVar(&cfg.token, "token",
		envOrDefault("CARDCORE_TUI_TOKEN", ""),
		"seat bearer token (env: CARDCORE_TUI_TOKEN)")
	fs.IntVar(&cfg.seat, "seat",
		intEnvOrDefault("CARDCORE_TUI_SEAT", 0),
		"seat index (game-dependent) (env: CARDCORE_TUI_SEAT)")
	fs.BoolVar(&cfg.observer, "observe",
		boolEnvOrDefault("CARDCORE_TUI_OBSERVE", false),
		"observer mode (receive-only) (env: CARDCORE_TUI_OBSERVE)")
	fs.BoolVar(&cfg.debug, "debug",
		boolEnvOrDefault("CARDCORE_TUI_DEBUG", false),
		"enable debug logging (env: CARDCORE_TUI_DEBUG)")
	fs.StringVar(&cfg.aiType, "ai-type",
		envOrDefault("CARDCORE_TUI_AI_TYPE", "random"),
		"AI player type (env: CARDCORE_TUI_AI_TYPE)")
	fs.StringVar(&cfg.theme, "theme",
		envOrDefault("CARDCORE_TUI_THEME", themeDark),
		"color theme: dark or light (env: CARDCORE_TUI_THEME)")

	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "Usage: %s [flags]\n\n", fs.Name())
		_, _ = fmt.Fprintln(fs.Output(), "Flags:")
		fs.PrintDefaults()
		_, _ = fmt.Fprintln(fs.Output(), "\nAll flags can also be set via the corresponding")
		_, _ = fmt.Fprintln(fs.Output(), "CARDCORE_TUI_* environment variable.")
		_, _ = fmt.Fprintln(fs.Output(), "Explicit flags take precedence over environment")
		_, _ = fmt.Fprintln(fs.Output(), "variables.")
	}

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "ai-type", "session", "token", "observe":
			cfg.menuSkipped = true
		}
	})

	if cfg.observer && cfg.session != "" {
		return nil, fmt.Errorf("-observe and -session are mutually exclusive")
	}
	if cfg.session != "" && cfg.token == "" {
		return nil, fmt.Errorf("-token is required when -session is set")
	}
	if cfg.token != "" && cfg.session == "" {
		return nil, fmt.Errorf("-session is required when -token is set")
	}
	if cfg.seat < 0 {
		return nil, fmt.Errorf("-seat must be >= 0")
	}
	if cfg.theme != themeDark && cfg.theme != themeLight {
		return nil, fmt.Errorf("-theme must be 'dark' or 'light'")
	}

	return cfg, nil
}

// runMenu starts the interactive menu when no explicit game-related flag was
// set. If the user pressed Esc, it returns nil, nil so the program exits
// silently. Otherwise it returns the resolved configuration (a copy of cfg
// updated with menu selections).
func runMenu(cfg *tuiConfig) (*tuiConfig, error) {
	if cfg.menuSkipped {
		return cfg, nil
	}

	theme := NewDarkTheme()
	if cfg.theme == themeLight {
		theme = NewLightTheme()
	}

	initial := menu.Config{
		Server:   cfg.server,
		Game:     cfg.game,
		AIType:   cfg.aiType,
		Observer: cfg.observer,
		Theme:    cfg.theme,
		Debug:    cfg.debug,
	}

	result, err := menu.Run(initial, theme)
	if err != nil {
		if errors.Is(err, menu.ErrCancelled) {
			return nil, nil
		}
		return nil, err
	}

	return &tuiConfig{
		server:      result.Server,
		game:        result.Game,
		session:     "",
		token:       "",
		seat:        0,
		observer:    result.Observer,
		debug:       result.Debug,
		aiType:      result.AIType,
		theme:       result.Theme,
		menuSkipped: cfg.menuSkipped,
	}, nil
}

// runGame executes the TUI lifecycle.
//
// Step 1: Create HTTP session client and connect context.
//
//	SessionClient is used for both auto-creating a session when none
//	was provided and for fetching the turn timeout config. The connect
//	context has a 10-second timeout so a hanging dial fails fast.
//
// Step 2: Auto-create session if needed.
//
//	When cfg.session is empty, create a Hearts session via the helper.
//	Player mode gets 1 human + 3 AI; observer mode gets 4 AI.
//
// Step 3: Create WebSocket connection.
//
//	Conn is created but not connected yet. The Connect method establishes
//	the WebSocket handshake and returns the initial snapshot.
//
// Step 4: Connect.
//
//	The connection is established before the TUI program starts. This
//	allows us to fail fast on connection errors with a clean terminal
//	message instead of a complex TUI error state.
//
// Step 5: Create model with pointer receiver.
//
//	The model must be a pointer (*model) so that m.program = p works.
//	See the detailed comment below for why this matters.
//
// Step 6: Create program.
//
//	tea.NewProgram takes tea.Model (an interface). When passed a *model,
//	it stores the pointer internally. Any later modification to the
//	underlying struct (like setting m.program) is visible to the program.
//
// Step 7: Set model.program.
//
//	Now that the program exists, we set the model's program reference.
//	The WebSocket reader goroutine will use this to call program.Send().
//
// Step 8: Start WebSocket reader.
//
//	The goroutine reads snapshots from the WebSocket and injects them
//	into the model via program.Send().
//
// Step 9: Run.
//
//	p.Run() blocks until the user exits (ctrl+c, game over, etc.).
//	Cleanup is handled by defer on the connection.
func runGame(cfg *tuiConfig) error {
	configureLogging(cfg.debug)

	// The connect context has a 10-second timeout so a hanging dial fails
	// fast. The read loop uses a separate long-lived context (readCtx) so
	// the WebSocket reader does not cancel after 10 seconds.
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer connectCancel()

	// SessionClient is used for auto-creating a session when none was
	// provided. The connect context has a 10-second timeout so a hanging
	// dial fails fast.
	sc := &client.SessionClient{BaseURL: cfg.server}

	// Auto-create a Hearts session when no session ID was provided.
	// Player mode gets 1 human + 3 AI; observer mode gets 4 AI. The helper
	// uses server delay defaults (nil overrides).
	if cfg.session == "" {
		sessionID, token, seat, err := heartstui.CreateSession(
			connectCtx, sc, cfg.aiType, cfg.observer, nil, nil,
		)
		if err != nil {
			return fmt.Errorf("auto-create session: %w", err)
		}
		cfg.session = sessionID
		cfg.token = token
		cfg.seat = seat
	}

	// Step 3: Create WebSocket connection.
	//
	// Conn is created but not connected yet. The Connect method establishes
	// the WebSocket handshake and returns the initial snapshot.
	conn := &client.Conn{}

	// Step 4: Connect.
	//
	// Construct the WebSocket URL from the base URL, session ID, and path.
	// The wsURL helper converts http:// to ws:// and appends the session path.
	var wsPath string
	if cfg.observer {
		wsPath = "/ws/observe"
	} else {
		wsPath = "/ws"
	}
	url := client.WebSocketURL(cfg.server, cfg.session, wsPath)

	if err := conn.Connect(connectCtx, url, cfg.token); err != nil {
		return fmt.Errorf("websocket connect: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// readCtx is the long-lived context for the WebSocket read loop. It
	// cancels when runGame() returns (program exit), stopping the reader.
	readCtx, readCancel := context.WithCancel(context.Background())
	defer readCancel()

	// Step 5: Create model with pointer receiver.
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
	// Construct the theme from the configured string. The value is
	// validated by parseFlags, so only "dark" or "light" reach here.
	theme := NewDarkTheme()
	if cfg.theme == themeLight {
		theme = NewLightTheme()
	}

	game, err := newGameClient(cfg.game, cfg.seat, cfg.observer, theme)
	if err != nil {
		return err
	}
	m := &model{
		conn:  conn,
		game:  game,
		phase: "connecting",
		theme: theme,
	}

	// Step 6: Create program.
	p := tea.NewProgram(m)

	// Step 7: Set model.program.
	//
	// Now that p exists, store the reference in the model. Because m is a
	// pointer, this modification is visible to the program.
	m.program = p

	// Step 8: Start WebSocket reader goroutine.
	//
	// The goroutine reads from the WebSocket and sends messages into the
	// model via program.Send(). This is safe because Program.Send() is
	// thread-safe — it can be called from any goroutine.
	go startWSReader(readCtx, conn, p)

	// Step 9: Run.
	//
	// p.Run() blocks until the user exits. The model's Update() handles
	// all messages (snapshots, errors, keypresses, timers).
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	return nil
}

// configureLogging sets up the default slog logger so server and wsbridge
// log output never corrupts the terminal.
//
// When debug is true, logs are written to tui.log in the working directory.
// When false, logs are discarded entirely. This must be called before any
// goroutine that uses slog (notably startWSReader and client.Conn) is started.
func configureLogging(debug bool) {
	var w io.Writer
	if debug {
		f, err := os.Create("tui.log")
		if err != nil {
			w = io.Discard
		} else {
			w = f
		}
	} else {
		w = io.Discard
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, nil)))
}

// newGameClient constructs the game-specific client for the named game. It is
// the single composition point where concrete games are wired into the
// game-agnostic model; add new games by extending the switch. The theme is
// passed through to the game client for use in rendering.
func newGameClient(game string, seat int, observer bool, theme Theme) (gameClient, error) {
	switch game {
	case "hearts":
		return heartstui.NewClient(seat, observer, theme), nil
	default:
		return nil, fmt.Errorf("unsupported game: %q", game)
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
