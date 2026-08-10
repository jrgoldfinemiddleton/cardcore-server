# AI Agent Guidance: Client Engine

## OVERVIEW
Shared protocol-agnostic client engine: HTTP session lifecycle, WebSocket connection, command envelopes, and error classification. Used by both the TUI and CLI binaries. The only `internal/` package the `cmd/` binaries may share is `internal/flags/`; all other client DTOs are mirrored in this package to keep the client decoupled from server internals.

## STRUCTURE
```
client/
├── http.go              # SessionClient: Create/Start/Delete/Get sessions
├── ws.go                # Conn: WebSocket connect, read, send, close
├── url.go               # WebSocket URL construction from HTTP base URL
├── messages.go          # Command and ErrorMessage envelopes
├── errors.go            # Error code constants and recovery classification
├── bench_test.go        # BenchmarkSessionCommandRoundTrip (real WebSocket latency)
└── integration_test.go  # End-to-end tests with real server

client/games/hearts/
├── dto.go               # Hearts-specific DTOs
├── phases.go            # Hearts phase constants
├── symbols.go           # GameName constant, rank/suit display symbols
└── commands.go          # Command envelope builders
```

## WHERE TO LOOK
| Task | File | Notes |
|------|------|-------|
| Add HTTP session endpoint | `http.go` | `SessionClient` methods mirror REST routes |
| Change WebSocket behavior | `ws.go` | `Conn` deduplicates snapshots by `maxSeenSeq` |
| Build WebSocket URL | `url.go` | Convert HTTP/HTTPS base URL to WS/WSS session URL |
| Change command envelope | `messages.go` | Used by `client/games/hearts` command builders |
| Add client error handling | `errors.go` | `ClassifyError` maps server codes to recovery actions |
| Change shared flags/env helpers | `internal/flags/` | Used by `cmd/` binaries |
| Write client integration test | `integration_test.go` | Real server + WebSocket |

## CONVENTIONS
- Client DTOs mirror server types but are separate structs with JSON tags; do not import server internals.
- `Conn.ReadSnapshot` blocks until a snapshot with a higher `seq` arrives.
- `Conn.SendCommand` sets `seq` to the current `maxSeenSeq`, overwriting any caller-provided value.
- Tests use `setupTestServer()` that starts a real server on `:0`.

## ANTI-PATTERNS
- Never import `internal/server/*` from `internal/client`; keep the client decoupled.
- Never leak WebSocket close details into higher layers; wrap them in `ConnectionClosedError`.
