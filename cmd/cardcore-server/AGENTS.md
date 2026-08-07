# AI Agent Guidance: Server Binary

## OVERVIEW
The `cardcore-server` binary entry point. Parses command-line flags, configures logging, builds the session registry, starts the HTTP/WebSocket server, and blocks on SIGINT/SIGTERM for graceful shutdown. Game-specific flags are registered by the game adapter via the session registry.

## STRUCTURE
```
cardcore-server/
├── main.go      # Entry point: parseFlags, run, parseLogLevel
├── doc.go       # Package documentation
└── main_test.go # Flag parsing and validation tests
```

## WHERE TO LOOK
| Task | File | Notes |
|------|------|-------|
| Change server flags | `main.go` | `parseFlags` registers generic flags; game-specific flags come from the registry |
| Change startup/shutdown | `main.go` | `run()` builds the manager, starts the server, waits for signals |
| Change log routing | `main.go` | `-log-file` opens a file; empty logs to stderr; `slog.SetDefault` is called once |
| Add a new game | `main.go` | Register the game's `GameConfig` in `run()` before `parseFlags` |
| Test flag parsing | `main_test.go` | Table-driven tests for defaults, env fallback, flag override, validation |

## CONVENTIONS
- Generic flags use `-ms` suffix for millisecond values; env vars use `_MS` suffix.
- Explicit flags take precedence over env vars, which take precedence over hardcoded defaults.
- `run()` returns an exit code: `0` success, `2` flag-parsing error, `1` runtime failure.
- The logger is tagged with `"component": "server"` via `slog.With`.
- Game-specific flags are registered by calling `registry.RegisterFlags(fs)` after the generic flags.

## ANTI-PATTERNS
- Never register game-specific flags directly in `parseFlags`; let the registry own them.
- Never call `os.Exit` from inside `run()`; return the exit code so deferred cleanup runs.
- Never import `internal/client/` from this binary; the server is client-agnostic.

## COMMANDS
```bash
# Run server tests
go test ./cmd/cardcore-server/...

# Build the binary
make build

# Run the server
go run ./cmd/cardcore-server

# Full gate
make check
```
