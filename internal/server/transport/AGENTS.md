# AI Agent Guidance: Transport Layer

## OVERVIEW
HTTP/WebSocket server and route handlers. This package owns the strict transport boundary between clients and the session layer.

## STRUCTURE
```
transport/
├── server.go              # Server struct, route registration, Start/Shutdown
├── config.go              # Server config (Manager reference, timeouts, limits)
├── http_sessions.go       # REST endpoints for session lifecycle
├── http_errors.go         # HTTP error response helpers
├── ws_player.go           # Player WebSocket connection (read/write goroutines)
├── ws_observer.go         # Observer WebSocket connection (write goroutine)
├── ws_helpers.go          # Shared WS utilities (guarded, readJSON, writeBytes, parseBearerToken)
├── helpers_test.go        # Test server and WebSocket dial helpers
├── fullgame_test.go       # Full-game integration tests (real server + WebSocket)
├── stress_test.go         # Stress tests (gated behind the stress build tag)
└── stress_harness_test.go # Stress test harness
```

## WHERE TO LOOK
| Task | File | Notes |
|------|------|-------|
| Add/change HTTP route | `server.go` | Register in `NewServer`; mux is wired there |
| Change session CRUD responses | `http_sessions.go` | Maps `Manager` errors to HTTP status via `httpStatus()` |
| Change player WS protocol | `ws_player.go` | Reads commands, validates tokens, sends snapshots |
| Change observer WS protocol | `ws_observer.go` | Subscribe-only stream; receives snapshots |
| Add WS shared helper | `ws_helpers.go` | `parseBearerToken`, `readWSJSON`, `writeWSBytes` |
| Write transport test | `server_test.go` | Table-driven protocol conformance |
| Write full-game integration test | `fullgame_test.go` | Real server + real WebSocket |
| Test WS panic recovery | `ws_panic_test.go` | Unit tests for the `guarded` wrapper in `ws_helpers.go` |

## CONVENTIONS
- Integration tests must start a real server on `:0` and connect via WebSocket; no mocked transport.
- Protocol conformance tests are table-driven: one row per message type × expected response.
- `Server` holds a `*session.Manager`; all session state changes go through `Manager`.
- WebSocket handlers authenticate via `Authorization: Bearer <seat-token>`.

## ANTI-PATTERNS
- Never hold `Server.mu` while doing I/O; it protects the listener/connection set.
- Never return `Manager` sentinel errors directly to clients; map them via `httpStatus()`/`writeError()`.
