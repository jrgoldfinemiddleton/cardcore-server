# cardcore-server

A WebSocket game server and Bubble Tea TUI client for [Cardcore](https://github.com/jrgoldfinemiddleton/cardcore).

[![CI](https://github.com/jrgoldfinemiddleton/cardcore-server/actions/workflows/main.yml/badge.svg)](https://github.com/jrgoldfinemiddleton/cardcore-server/actions/workflows/main.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/jrgoldfinemiddleton/cardcore-server.svg)](https://pkg.go.dev/github.com/jrgoldfinemiddleton/cardcore-server)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## About

Cardcore Server provides a localhost WebSocket server that hosts card games powered by the [cardcore](https://github.com/jrgoldfinemiddleton/cardcore) engine, plus a terminal UI client built with [Bubble Tea](https://charm.land/bubbletea/). The server exposes a JSON-over-WebSocket protocol for real-time gameplay; see [`doc/api.md`](doc/api.md) for the full specification.

## Design Philosophy

- Minimal dependencies: stdlib-first, external deps require explicit approval.
- Strict transport boundary: all clients use the real API, no in-process shortcuts.
- Full-state snapshots: no incremental diffs, no sync bugs.
- [Suckless](https://suckless.org/philosophy/) code design: small, readable, and composable.
- Contributor-friendly: thorough docs, automated checks, and clear conventions lower the barrier to entry.

## Project Layout

```text
cardcore-server/
├── cmd/
│   ├── cardcore-server/ # Game server binary
│   ├── cardcore-tui/    # Bubble Tea TUI client binary
│   │   └── <game>/      # Game-specific rendering and command-builders
│   └── cardcore-cli/    # Non-TTY CLI client binary
│       └── <game>/      # Game-specific command builders and formatters
└── internal/
    ├── api/              # Shared protocol-agnostic client engine
    │   └── games/<game>/ # Game-specific wire-format types
    ├── client/           # Shared protocol-agnostic client engine
    │   └── <game>/       # Game-specific adapter and DTOs
    └── server/
        ├── session/          # Session lifecycle and game goroutine
        │   └── games/<game>/ # Game-specific adapter for session manager 
        ├── transport/        # HTTP/WebSocket plumbing
        └── view/
            └── <game>/       # Seat-filtered snapshot generation
```

## Requirements

Go 1.25.9+

## Getting Started

```bash
make check
```

This runs formatting, vetting, linting, and tests. Dev tools like [golangci-lint](https://golangci-lint.run/) are declared in `go.mod` via the Go 1.25 `tool` directive and are compiled automatically on first use.

### TUI Terminal Requirements

The TUI (`cmd/cardcore-tui`) requires a terminal emulator with ANSI escape sequence support. All modern terminals (xterm, iTerm2, Windows Terminal, etc.) meet this.

Specific requirements:

- **Alternate screen buffer** (smcup/rmcup): enabled so the TUI does not scroll the terminal history.
- **True color (24-bit)**: required for the lipgloss color scheme.
- **Minimum width**: 80 columns for the layout.

For tmux users: set `TERM=screen-256color` or `tmux-256color`. Focus reporting is not enabled — the game continues regardless of terminal focus.

See [Bubble Tea's terminal docs](https://charm.land/bubbletea/docs/terminal) for details.

## Usage

This project is intended for two different audiences. Use the **binary usage** instructions when running released or locally-built artifacts. Use the **development usage** instructions when iterating on the source code between releases or commits.

### End-user binary usage

Build the binaries once with `make build`, then run them from `bin/`:

```bash
make build
./bin/cardcore-server
./bin/cardcore-tui
./bin/cardcore-cli -script script.json
```

#### Server

```bash
./bin/cardcore-server
```

The server listens on `127.0.0.1:8080` by default. It hosts WebSocket game sessions and serves the HTTP API documented in [`doc/api.md`](doc/api.md). Press `Ctrl+C` for graceful shutdown.

| Flag | Env Var | Default | Description |
|---|---|---|---|
| `-addr` | `CARDCORE_SERVER_ADDR` | `127.0.0.1:8080` | Listen address |
| `-log-level` | `CARDCORE_SERVER_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `-shutdown-timeout` | `CARDCORE_SERVER_SHUTDOWN_TIMEOUT` | `10` | Graceful shutdown timeout in seconds |
| `-ai-action-delay` | `CARDCORE_SERVER_AI_ACTION_DELAY_MS` | `1000` | AI action delay in milliseconds |
| `-deal-display-delay` | `CARDCORE_SERVER_DEAL_DISPLAY_DELAY_MS` | `1500` | Deal display delay in milliseconds |
| `-turn-timeout` | `CARDCORE_SERVER_TURN_TIMEOUT_MS` | `30000` | Human turn timeout in milliseconds |
| `-hearts-trick-display-delay` | `CARDCORE_SERVER_HEARTS_TRICK_DISPLAY_DELAY_MS` | `3000` | Hearts trick display delay in milliseconds |
| `-hearts-round-display-delay` | `CARDCORE_SERVER_HEARTS_ROUND_DISPLAY_DELAY_MS` | `5000` | Hearts round display delay in milliseconds |

#### TUI Client

```bash
./bin/cardcore-tui
```

Starts an interactive Bubble Tea session. By default it auto-creates a 1-human+3-AI Hearts game, connects as seat 0, and begins play immediately.

Key commands during a game:

- `←` / `→` — navigate cards in your hand
- `Space` — select/deselect a card
- `Enter` — confirm selection (pass 3 cards or play 1 card)
- `Esc` — initiate quit, then `Enter` to confirm

| Flag | Env Var | Default | Description |
|---|---|---|---|
| `-server` | `CARDCORE_TUI_SERVER` | `http://127.0.0.1:8080` | Server base URL |
| `-game` | `CARDCORE_TUI_GAME` | `hearts` | Game to play |
| `-session` | `CARDCORE_TUI_SESSION` | — | Session ID to join |
| `-token` | `CARDCORE_TUI_TOKEN` | — | Bearer token for the seat being joined |
| `-seat` | `CARDCORE_TUI_SEAT` | `0` | Seat index to join |
| `-observe` | `CARDCORE_TUI_OBSERVE` | `false` | Observer mode (receive-only) |
| `-ai-type` | `CARDCORE_TUI_AI_TYPE` | `random` | AI type for auto-created sessions: `random`, `heuristic`, or `pimc` |
| `-debug` | `CARDCORE_TUI_DEBUG` | `false` | Enable debug logging to `tui.log` |

#### CLI Client

```bash
./bin/cardcore-cli -script script.json
```

Runs a non-interactive scripted game. The script is a JSON array of phase-action entries that drive command construction automatically.

Example `script.json` for Hearts:

```json
[
  {
    "phase": "passing",
    "action": "pass_cards",
    "selector": "first_n",
    "selector_args": {"count": 3}
  },
  {
    "phase": "playing",
    "action": "play_card",
    "selector": "first_legal"
  }
]
```

The CLI prints each snapshot in compact notation to stdout. Use `-observe` to watch an all-AI session without sending commands.

| Flag | Env Var | Default | Description |
|---|---|---|---|
| `-script` | `CARDCORE_CLI_SCRIPT` | — | Path to JSON script file |
| `-addr` | `CARDCORE_CLI_ADDR` | `http://127.0.0.1:8080` | Server address |
| `-game` | `CARDCORE_CLI_GAME` | `hearts` | Game to play |
| `-ai-type` | `CARDCORE_CLI_AI_TYPE` | `random` | AI player type (`random` or `pimc`) |
| `-pacing` | `CARDCORE_CLI_PACING_MS` | `500` | Pacing delay between snapshots (ms) |
| `-exit-delay` | `CARDCORE_CLI_EXIT_DELAY_MS` | `1000` | Wait after `game_over` before exiting (ms) |
| `-observe` | `CARDCORE_CLI_OBSERVE` | `false` | Create 4-AI session and observe |
| `-session-id` | `CARDCORE_CLI_SESSION_ID` | — | Join an existing session |
| `-token` | `CARDCORE_CLI_TOKEN` | — | Bearer token for the seat being joined |
| `-seat` | `CARDCORE_CLI_SEAT` | `0` | Seat index to join |
| `-delete-on-exit` | `CARDCORE_CLI_DELETE_ON_EXIT` | `false` | Delete session after game ends |

### Development usage

Use these commands when testing changes between releases or commits. They compile the source on every run, so they are slower than running pre-built binaries.

```bash
go run ./cmd/cardcore-server
go run ./cmd/cardcore-tui
go run ./cmd/cardcore-cli -script script.json
```

All flags and environment variables described in the binary usage sections above apply equally to `go run`; only the binary path changes.

## Makefile Targets

| Target | Description |
|---|---|
| `make test` | Run all tests |
| `make fmt` | Format code with [gofmt](https://pkg.go.dev/cmd/gofmt) |
| `make vet` | Run [go vet](https://pkg.go.dev/cmd/vet) |
| `make lint` | Run [golangci-lint](https://golangci-lint.run/) |
| `make lint-extra` | Run golangci-lint with the extra-strict config |
| `make build` | Compile all packages and binaries |
| `make doc` | Browse docs locally via [pkgsite](https://pkg.go.dev/golang.org/x/pkgsite) |
| `make check` | Run fmt, vet, lint, and test |
| `make clean` | Remove build output directory |
| `make help` | Show available targets |

## License

MIT
