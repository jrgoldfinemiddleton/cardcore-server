# cardcore-server

A WebSocket game server and Bubble Tea TUI client for [Cardcore](https://github.com/jrgoldfinemiddleton/cardcore).

[![CI](https://github.com/jrgoldfinemiddleton/cardcore-server/actions/workflows/main.yml/badge.svg)](https://github.com/jrgoldfinemiddleton/cardcore-server/actions/workflows/main.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## About

Cardcore-server provides a localhost WebSocket server that hosts card games powered by the [cardcore](https://github.com/jrgoldfinemiddleton/cardcore) engine, plus a terminal UI client built with [Bubble Tea](https://charm.land/bubbletea/). The server exposes a JSON-over-WebSocket protocol for real-time gameplay; see [`doc/api.md`](doc/api.md) for the full specification.

## Project Layout

```text
cardcore-server/
├── cmd/
│   ├── server/          # Game server binary
│   └── tui/             # Bubble Tea TUI client binary
├── internal/
│   └── server/
│       ├── transport/   # HTTP/WebSocket plumbing
│       ├── session/     # Session lifecycle and game goroutine
│       └── view/        # Seat-filtered snapshot generation
├── doc/
│   ├── api.md           # Protocol specification
│   ├── dependencies.md  # Approved external dependencies
│   └── decisions/       # Architecture Decision Records
├── CONTRIBUTING.md
├── SECURITY.md
├── Makefile
├── .golangci.yml
└── README.md
```

## Requirements

Go 1.25.9+

## Getting Started

```bash
make check
```

This runs formatting, vetting, linting, and tests. Dev tools like [golangci-lint](https://golangci-lint.run/) are declared in `go.mod` via the Go 1.25 `tool` directive and are compiled automatically on first use.

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
| `make help` | Show available targets |

## License

MIT
