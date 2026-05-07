# System Architecture

## Package Structure

The project uses a `cmd/` + `internal/` layout: `cmd/` holds thin entry points for the server and TUI binaries, while `internal/api/` provides shared wire DTOs and `internal/server/` splits server logic into transport, session, and view layers.

```
github.com/jrgoldfinemiddleton/cardcore-server
├── cmd/
│   ├── server/              ← entry point: parse flags, wire deps, start HTTP listener
│   └── tui/                 ← entry point: parse flags, connect WS, run Bubble Tea
├── internal/
│   ├── api/                 ← wire DTOs shared by server and TUI (JSON structs)
│   └── server/
│       ├── transport/       ← HTTP handlers, WebSocket upgrade, routing, message parsing
│       ├── session/         ← session lifecycle, game goroutine, token management, seq
│       └── view/            ← engine state → seat-filtered snapshot DTOs
```

## Data Flow

```
┌───────────┐          HTTP/WS          ┌────────────┐
│    TUI    │◄─────────────────────────►│   Server   │
│  (client) │       JSON messages       │            │
└───────────┘                           └─────┬──────┘
                                              │
                                    ┌─────────┼─────────┐
                                    │         │         │
                              transport/  session/    view/
                                    │         │         │
                                    │    ┌────┴────┐    │
                                    │    │cardcore │    │
                                    │    │ engine  │    │
                                    │    └─────────┘    │
                                    └───────────────────┘
```

1. **TUI** sends player commands (`play_card`, `pass_cards`) as JSON over WebSocket.
2. **Transport** accepts HTTP requests, upgrades WebSocket connections, parses inbound messages, and routes them to the appropriate session.
3. **Session** owns the game goroutine. It validates commands, applies them to the cardcore engine, runs AI turns, increments the seq counter, and broadcasts state changes.
4. **View** takes raw engine state and a seat index, produces a filtered snapshot DTO (hides opponents' hands, computes legal actions).
5. **Transport** serializes the snapshot and sends it to connected clients.

## Session Lifecycle

```
draft ──► active ──► finished
  │          │           │
  └──────────┴───────────┴──► expired (via DELETE or process exit)
```

- **Draft**: session created, config mutable, game not started.
- **Active**: game in progress, commands accepted.
- **Finished**: terminal game state reached (final snapshot sent).
- **Expired**: session torn down (DELETE endpoint or server shutdown).

## Concurrency Model

Each session runs in its own goroutine:

```
[transport goroutine]  ──── command channel ────►  [session goroutine]
                       ◄── snapshot broadcast ───
```

- Transport handlers are stateless — they parse, validate envelope structure, and enqueue.
- The session goroutine is the sole writer of game state. No mutexes needed.
- AI turns execute synchronously within the session goroutine, gated by a minimum delay.
- Observer connections subscribe to the snapshot broadcast; they never enqueue commands.

## Network Model

Localhost TCP on `127.0.0.1:0` — the OS picks a free port at startup. No unix sockets (not portable across OSes). The server prints the bound address on startup so the TUI can connect.

## Dependency Policy

External dependencies require explicit approval. The approved list lives in `doc/dependencies.md`. The `cardcore` engine is the only domain dependency; the Charm stack provides TUI rendering; `coder/websocket` provides WebSocket transport. All other functionality uses the Go standard library.

Dev tools (golangci-lint, pkgsite) are managed via the `tool` directive in `go.mod` and compiled automatically on first use.
