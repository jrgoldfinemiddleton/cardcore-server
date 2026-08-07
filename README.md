# cardcore-server

A WebSocket game server and Bubble Tea TUI client for [Cardcore](https://github.com/jrgoldfinemiddleton/cardcore).

[![CI](https://github.com/jrgoldfinemiddleton/cardcore-server/actions/workflows/main.yml/badge.svg)](https://github.com/jrgoldfinemiddleton/cardcore-server/actions/workflows/main.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/jrgoldfinemiddleton/cardcore-server.svg)](https://pkg.go.dev/github.com/jrgoldfinemiddleton/cardcore-server)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## About

Cardcore Server hosts card games on a localhost WebSocket server, with a terminal UI client for interactive play. The server exposes a JSON-over-WebSocket protocol documented in [`doc/api.md`](doc/api.md).

Hearts is the first implemented game. Other games will follow the same vertical-slice structure.

## Quickstart

Install [Go](https://go.dev/) 1.25.12+, then build the binaries:

```bash
git clone https://github.com/jrgoldfinemiddleton/cardcore-server.git
cd cardcore-server
make build
```

In one terminal, start the server:

```bash
./bin/cardcore-server
```

In another terminal, start the TUI and select **Start Game** from the menu:

```bash
./bin/cardcore-tui
```

The default menu starts a 1-human + 3-AI Hearts game. No flags are required.

## Commands

### `cardcore-server`

The game server. It listens on `127.0.0.1:8080` by default, manages game sessions over HTTP/WebSocket, and runs the game goroutine. Press `Ctrl+C` for graceful shutdown.

### `cardcore-tui`

The interactive terminal client. When run without game-related flags, it shows a menu to choose the server, AI difficulty, observer mode, and theme.

Menu navigation: `↑` / `↓` to move, `Enter` to change a setting or start the game, `Esc` to exit.

During a Hearts game (the only implemented game so far): `←` / `→` to move through your hand, `Space` to select or deselect a card, `Enter` to confirm, and `Esc` to initiate quitting.

### `cardcore-cli`

A non-interactive scripted client. Point it at a JSON script to drive a game automatically and print compact snapshots to stdout. Use `-observe` to watch an all-AI game without sending commands.

## TUI terminal requirements

The TUI requires a terminal emulator that supports ANSI escape sequences and 24-bit true color. All modern terminals (xterm, iTerm2, Windows Terminal, Ghostty, etc.) meet this.

For tmux, set `TERM=screen-256color` or `tmux-256color`. Focus reporting is not enabled. See the [termenv terminal feature support matrix](https://github.com/muesli/termenv#terminal-feature-support) for details.

## Configuration

Each command prints its full flag and environment-variable reference:

```bash
./bin/cardcore-server -h
./bin/cardcore-tui -h
./bin/cardcore-cli -h
```

All flags have a matching `CARDCORE_*` environment variable. Explicit flags take precedence.

### Notable flags

- `-log-level` (server): minimum severity to log (`debug`, `info`, `warn`, `error`). Default is `info`.
- `-shutdown-timeout-secs` (server): graceful shutdown timeout in seconds.
- `-pacing-ms` / `-exit-delay-ms` (CLI): delays between snapshots and before exiting, in milliseconds.
- `-debug` (TUI): writes debug logs to `tui.log` in the **current working directory**.
- `-observe` (TUI/CLI): watch the full game state without taking a seat. All hands are visible, and you receive every snapshot, but you cannot send moves.

## Development

For build, test, lint, and contribution workflow, see [`CONTRIBUTING.md`](CONTRIBUTING.md) or run:

```bash
make help
```

## License

MIT
