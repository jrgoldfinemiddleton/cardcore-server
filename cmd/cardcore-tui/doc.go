// Package main implements the cardcore TUI client.
//
// The TUI client is an interactive Bubble Tea v2 application that connects to
// a cardcore-server game session via WebSocket. It supports two modes:
//
//   - Human player: join a session as a specific seat and interact with
//     the game via keyboard (card selection, passing, playing).
//
//   - Observer: connect to a session in receive-only mode to watch all
//     hands and game state.
//
// Running the binary without explicit game-related flags opens an interactive
// menu. The menu lets you review or change the connection and game settings
// before starting:
//
//   - Game            Display-only game name (currently "hearts").
//   - Server          Server base URL. Press Enter to edit inline; Esc cancels
//     the edit, Enter confirms it.
//   - AI Difficulty   Difficulty shown as Easy, Medium, or Hard; these map to
//     the server AI types random, heuristic, and pimc.
//   - Observer        Toggle receive-only mode with all hands visible.
//   - Theme           Color theme: dark or light.
//   - Start Game      Start the game with the selected settings.
//
// Menu controls:
//
//   - Up/Down — move the cursor between items.
//   - Enter   — cycle the value of AI Difficulty, Observer, or Theme, or start
//     the game when Start Game is selected.
//   - Esc     — exit the menu without starting a game.
//
// Explicitly providing any of these flags skips the menu and starts the game
// directly:
//
//   - -session  Session ID to join.
//   - -token    Seat bearer token (required when -session is set).
//   - -observe  Observer mode.
//   - -ai-type  AI player type; when set explicitly, the menu is skipped.
//
// Usage:
//
//	go run ./cmd/cardcore-tui -server http://localhost:8080 -session <id> -token <token> -seat 0
//
// Flags:
//
//	-server   Server base URL (default: http://localhost:8080)
//	-session  Session ID to join (required for observer or join mode)
//	-token    Seat bearer token (required when joining)
//	-seat     Seat index (game-dependent, default: 0)
//	-observe  Observer mode: receive-only, all hands visible
//	-ai-type  AI player type for auto-created sessions: random, heuristic, or
//	          pimc; when set explicitly, skips the interactive menu
//	-theme    Color theme: dark or light (default: dark)
//	-debug    Enable debug logging to tui.log
//
// The TUI uses the Elm Architecture (Model-Update-View) via Bubble Tea:
//
//	Model:   Holds game state, UI state, and WebSocket connection.
//	Update:  Handles messages (snapshots, errors, keypresses, timers).
//	View:    Renders the complete terminal screen on every frame.
//
// Key design decisions:
//
//   - The WebSocket connection is established in main() before the Bubble Tea
//     program starts. This avoids a complex connecting state in the model.
//
//   - The model uses pointer receivers so it can store a reference to the
//     tea.Program for goroutine-safe message sending.
//
//   - A dedicated goroutine (startWSReader) reads from the WebSocket and
//     sends typed messages into the model via program.Send().
//
//   - All UI state mutations happen in Update() on the single program
//     goroutine. No locks are needed.
//
//   - The alternate screen buffer is enabled so the TUI does not scroll the
//     terminal history.
//
// Terminal Requirements:
//
// The TUI requires a terminal emulator with ANSI escape sequence support.
// All modern terminals (xterm, iTerm2, Windows Terminal, etc.) meet this.
//
// Specific requirements:
//
//   - Alternate screen buffer (smcup/rmcup): enabled via v.AltScreen.
//   - True color (24-bit): required for the lipgloss color scheme.
//   - Minimum width: 80 columns for the layout.
//
// For tmux users: set TERM=screen-256color or tmux-256color. Focus reporting
// is not enabled — the game continues regardless of terminal focus.
//
// See Bubble Tea's terminal docs: https://charm.land/bubbletea/docs/terminal
package main
