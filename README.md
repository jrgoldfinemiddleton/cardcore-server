# cardcore-server

A WebSocket game server and Bubble Tea TUI client for [Cardcore](https://github.com/jrgoldfinemiddleton/cardcore).

[![CI](https://github.com/jrgoldfinemiddleton/cardcore-server/actions/workflows/main.yml/badge.svg)](https://github.com/jrgoldfinemiddleton/cardcore-server/actions/workflows/main.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/jrgoldfinemiddleton/cardcore-server.svg)](https://pkg.go.dev/github.com/jrgoldfinemiddleton/cardcore-server)
[![Go Report Card](https://goreportcard.com/badge/github.com/jrgoldfinemiddleton/cardcore-server)](https://goreportcard.com/report/github.com/jrgoldfinemiddleton/cardcore-server)
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

### Development (go run)

```bash
go run ./cmd/cardcore-server
go run ./cmd/cardcore-tui
go run ./cmd/cardcore-cli -script script.json
```

### Production (make build)

```bash
make build
```

This compiles all binaries to `bin/`:

```bash
./bin/cardcore-server
./bin/cardcore-tui
./bin/cardcore-cli -script script.json
```

### Server

```bash
go run ./cmd/cardcore-server
# or after make build:
# ./bin/cardcore-server
```

The server listens on `127.0.0.1:8080` by default. It hosts WebSocket game sessions and serves the HTTP API documented in [`doc/api.md`](doc/api.md). Press `Ctrl+C` for graceful shutdown.

Environment variables:

- `CARDCORE_LOG_LEVEL` — set to `debug` for verbose per-component logging (`info` is default).

### TUI Client

```bash
go run ./cmd/cardcore-tui
# or after make build:
# ./bin/cardcore-tui
```

Starts an interactive Bubble Tea session. By default it auto-creates a 1-human+3-AI Hearts game, connects as seat 0, and begins play immediately.

Key commands during a game:

- `←` / `→` — navigate cards in your hand
- `Space` — select/deselect a card
- `Enter` — confirm selection (pass 3 cards or play 1 card)
- `Esc` — initiate quit, then `Enter` to confirm

Flags:

- `-server <host:port>` — server base URL (default `http://127.0.0.1:8080`)
- `-observe` — connect as an observer to an existing session (requires `-session`)
- `-session <session-id>` — session ID to join (required for observer mode and when joining as a player)
- `-token <bearer-token>` — bearer token for the seat being joined
- `-seat <index>` — seat index to join (default `0`)
- `-game <game>` — game to play (default `hearts`)
- `-debug` — enable debug logging to `tui.log`

### CLI Client

```bash
go run ./cmd/cardcore-cli -script script.json
# or after make build:
# ./bin/cardcore-cli -script script.json
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

Flags and environment variables:

| Flag | Env Var | Default | Description |
|---|---|---|---|
| `-script` | — | *(required)* | Path to JSON script file |
| `-addr` | `CARDCORE_ADDR` | `http://127.0.0.1:8080` | Server address |
| `-game` | `CARDCORE_GAME` | `hearts` | Game to play |
| `-ai-type` | `CARDCORE_AI_TYPE` | `random` | AI player type (`random` or `pimc`) |
| `-pacing` | `CARDCORE_PACING_MS` | `500` | Pacing delay between snapshots (ms) |
| `-exit-delay` | `CARDCORE_EXIT_DELAY_MS` | `1000` | Wait after `game_over` before exiting (ms) |
| `-observe` | — | `false` | Create 4-AI session and observe |
| `-session-id` | — | — | Join an existing session |
| `-token` | — | — | Bearer token for the seat being joined |
| `-seat` | — | `0` | Seat index to join |
| `-delete-on-exit` | — | `false` | Delete session after game ends |

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
