# cardcore-server

A WebSocket game server and Bubble Tea TUI client for [Cardcore](https://github.com/jrgoldfinemiddleton/cardcore).

[![CI](https://github.com/jrgoldfinemiddleton/cardcore-server/actions/workflows/main.yml/badge.svg)](https://github.com/jrgoldfinemiddleton/cardcore-server/actions/workflows/main.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/jrgoldfinemiddleton/cardcore-server.svg)](https://pkg.go.dev/github.com/jrgoldfinemiddleton/cardcore-server)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## About

Cardcore Server hosts card games on a localhost WebSocket server, with a terminal UI client for interactive play. The server exposes a JSON-over-WebSocket protocol documented in [`doc/api.md`](doc/api.md).

Hearts is the first implemented game. Other games will follow the same vertical-slice structure.

## Installation

### Prebuilt binaries (recommended)

Download the archive for your platform from the
[latest release](https://github.com/jrgoldfinemiddleton/cardcore-server/releases/latest).
Every archive contains all three commands plus the license and changelog:

| Platform | Archive |
|---|---|
| Linux (amd64) | `cardcore-server_X.Y.Z_linux_amd64.tar.gz` |
| Linux (arm64) | `cardcore-server_X.Y.Z_linux_arm64.tar.gz` |
| macOS (Intel) | `cardcore-server_X.Y.Z_darwin_amd64.tar.gz` |
| macOS (Apple Silicon) | `cardcore-server_X.Y.Z_darwin_arm64.tar.gz` |
| Windows (amd64) | `cardcore-server_X.Y.Z_windows_amd64.zip` |
| Windows (arm64) | `cardcore-server_X.Y.Z_windows_arm64.zip` |

Verify, extract, and install (shown for Linux amd64; substitute your archive
from the table above):

```bash
VERSION=0.1.0  # the release to install, without the leading "v"

curl -LO "https://github.com/jrgoldfinemiddleton/cardcore-server/releases/download/v${VERSION}/cardcore-server_${VERSION}_linux_amd64.tar.gz"
curl -LO "https://github.com/jrgoldfinemiddleton/cardcore-server/releases/download/v${VERSION}/checksums.txt"
sha256sum -c checksums.txt --ignore-missing   # macOS: shasum -a 256 -c checksums.txt --ignore-missing
tar -xzf "cardcore-server_${VERSION}_linux_amd64.tar.gz"
sudo mv cardcore-server cardcore-tui cardcore-cli /usr/local/bin/
```

The archives already carry executable permissions, so no `chmod` is needed.

> **macOS:** archives downloaded with `curl` do not trigger Gatekeeper. If
> you download through a browser instead, remove the quarantine attribute
> after extracting: `xattr -d com.apple.quarantine cardcore-*`.
>
> **Windows:** extract the `.zip` and run the `.exe` files. SmartScreen may
> warn about the unsigned binaries; choose **More info → Run anyway**.

### Install with Go

Requires [Go](https://go.dev/) 1.26.6 or newer:

```bash
go install github.com/jrgoldfinemiddleton/cardcore-server/cmd/cardcore-server@latest
go install github.com/jrgoldfinemiddleton/cardcore-server/cmd/cardcore-tui@latest
go install github.com/jrgoldfinemiddleton/cardcore-server/cmd/cardcore-cli@latest
```

### Build from source

Requires [Go](https://go.dev/) 1.26.6 or newer:

```bash
git clone https://github.com/jrgoldfinemiddleton/cardcore-server.git
cd cardcore-server
make build    # binaries land in bin/
```

## Quickstart

In one terminal, start the server (use `./bin/cardcore-server` when building
from source):

```bash
cardcore-server
```

In another terminal, start the TUI and select **Start Game** from the menu:

```bash
cardcore-tui
```

The default menu starts a 1-human + 3-AI Hearts game. No flags are required.

Every command reports its build information:

```bash
cardcore-server -version
```

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
cardcore-server -h
cardcore-tui -h
cardcore-cli -h
```

All flags have a matching `CARDCORE_*` environment variable. Explicit flags take precedence.

### Notable flags

- `-log-level` (server): minimum severity to log (`debug`, `info`, `warn`, `error`). Default is `info`.
- `-shutdown-timeout-secs` (server): graceful shutdown timeout in seconds.
- `-pacing-delay-ms` / `-exit-delay-ms` (CLI): pacing delay before each AI turn, and linger time after `game_over` before exiting, in milliseconds.
- `-debug` (TUI): writes debug logs to `tui.log` in the **current working directory**.
- `-observe` (TUI/CLI): watch the full game state without taking a seat. All hands are visible, and you receive every snapshot, but you cannot send moves.

## Development

For build, test, lint, and contribution workflow, see [`CONTRIBUTING.md`](CONTRIBUTING.md) or run:

```bash
make help
```

## License

MIT
