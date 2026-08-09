# AI Agent Guidance: Session Layer

## Scope

The session package owns game session lifecycle, goroutine management, and the command-channel protocol between HTTP/WebSocket handlers and the game engine.

## Structure

```
session/
├── session.go         # Config, State, Seat, SeatDetail, Summary, Info DTOs
├── manager.go         # Mutex-protected session registry: Create, Get, List, Update, Start, Delete, Subscribe, SubmitAction
├── registry.go        # Game adapter registry: GameConfig interface, Registry (register, validate, build games)
├── goroutine.go       # session.run() event loop, turn driving, lifecycle
├── command.go         # playCmd, subscribeCmd, unsubscribeCmd, SubmitResult
├── commands.go        # Command dispatch and handlers: handlePlay, handlePauseCmd, handleResumeCmd
├── subscribers.go     # Player/observer subscriber management and WebSocket message sending
├── snapshot.go        # Snapshot generation, broadcast, and idempotent replay cache
├── game.go            # Game interface: HandleAction, AIPlay, Resume, Turn, Snapshot methods
├── helpers_test.go    # Shared test fixtures: testGameConfig, mock registries, validHeartsCfg
├── mockgame_test.go   # Mock Game implementations used across session tests
└── games/
    └── hearts/        # Hearts adapter: implements Game interface
        ├── hearts.go      # GameAdapter: HandleAction, AIPlay, Resume, Snapshot; seat/ai_type validation
        ├── config.go      # GameConfig: Hearts flags, NewGame, ValidateConfig wiring
        └── bench_test.go  # BenchmarkAIPlay (passing/playing phases, random/heuristic)
```

## Where to Look

| Task | File | Notes |
|------|------|-------|
| Add a Manager method | `manager.go` | Must acquire `mu` (RLock for reads, Lock for writes) |
| Change game registration or lookup | `registry.go` | `Registry.Register` panics on duplicate names; `ValidateConfig`/`NewGame` look up by `Config.Game` |
| Change command types | `command.go` | Add to sealed `command` interface; update `handleCommand` switch |
| Change command handling | `commands.go` | `handlePlay()`, `handlePauseCmd()`, `handleResumeCmd()`, `handleTurnTimeout()` |
| Change broadcast behavior | `snapshot.go` | `broadcastSnapshot()` or `sendNonBlocking()` |
| Change subscriber management | `subscribers.go` | `handleSubscribePlayer()`, `handleUnsubscribe()`, `closeSubscribers()` |
| Add session state logic | `goroutine.go` | `run()`, `driveTurns()`, `processTurns()` |
| Change the Game interface | `game.go` | Impacts all `internal/server/session/games/<game>/` |
| Add a new game | `games/<game>/` | Implement `session.Game`; wire factory in `cmd/cardcore-server` |

## Key Conventions

- **Manager is NOT a goroutine** — it's a mutex-protected struct. Only the `session` struct runs a goroutine.
- **Channel directions matter** — `s.cmds` receives from Manager, `c.resp` sends back to Manager.
- **Snapshot broadcast is non-blocking** — `sendNonBlocking` drops on full buffer. Clients resync via `stale_seq` on next action.
- **Seat tokens are bearer credentials** — `Seat.Token` for humans; empty for AI seats.
- **`Create` and `Update` both return `(*Info, []Seat, error)`** — tokens are surfaced only at mutation time, never in `Get`/`List`.

## Anti-Patterns

- **Never hold `mu` while blocking on a channel** — `SubmitAction` uses `select` with `default` to fast-fail on full `cmds`.
- **Never close `s.done` from outside the goroutine** — use `close(cancel)` to signal shutdown; goroutine closes `done` via `defer`.
- **Never mutate `entry.state` from the goroutine** — use `onDone` callback for race-free transitions.

## Commands

```bash
# Run session tests
go test ./internal/server/session/...

# Run with race detector
go test -race ./internal/server/session/...

# Full gate
make check
```
