# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/).

## [Unreleased]

### Added

- Debug logging across session, transport, and Hearts adapter with per-component `slog.With` prefixes. Test output defaults to WARN level; set `TEST_LOG_LEVEL=debug` to reveal all logs while debugging
- Full game integration tests via WebSocket: `TestAllAIFullGameIntegration` (4-AI Hearts, observer verifies phase progression and seq monotonicity) and `TestHumanAIFullGameIntegration` (1 human + 3 AI, human sends pass/play commands, game completes)
- Generic WebSocket test helpers: `testSnapshot` for fast phase/seq observation, `mustReadTestSnapshot`, `readTestSnapshot` (goroutine-safe), `writeWSJSON`, `readSnapshotsUntil`
- Subscription logging: structured `slog.Info` calls for player/observer subscribe and unsubscribe events
- Graceful shutdown: `Server.Shutdown(ctx)` sends `websocket.StatusGoingAway` to all active WebSocket connections, deletes all active sessions, and shuts down the HTTP server. `cmd/server/main.go` binary handles SIGINT/SIGTERM with a configurable timeout via `CARDCORE_SHUTDOWN_TIMEOUT` (default 10s)
- Server binary `cmd/server/main.go` with game adapter dispatch: currently supports `hearts`; new games can be added by extending the factory switch in `main.go`
- Configurable human turn timeout with AI auto-play fallback: session config gains `turn_timeout_ms` (default 30s, `0` to disable); when a human player doesn't act in time, the session auto-plays an AI move and broadcasts the result while preserving the human seat configuration
- Observer WebSocket connection: receive-only writer goroutine with `CloseRead` for ping/pong/close frame handling, context-based coordination, and automatic cleanup on disconnect
- Player WebSocket reader/writer goroutines: full bidirectional message handling with context-based coordination, envelope validation, and game error propagation
- Session termination on snapshot marshal failure: when a game adapter produces unmarshalable state after a successful action, the session terminates cleanly with `internal_error` rather than continuing in an unplayable state
- WebSocket upgrade endpoints for player (`/sessions/{id}/ws`) and observer (`/sessions/{id}/ws/observe`) connections
- HTTP session handlers: `POST /sessions`, `GET /sessions`, `GET /sessions/{id}`, `PATCH /sessions/{id}`, `POST /sessions/{id}/start`, `DELETE /sessions/{id}`
- Bearer token authentication on player WebSocket upgrades
- WebSocket message size limit (64 KB default, configurable via `WSReadLimit`)
- Nil-safe snapshot handling throughout the session layer
- Marshal-failure defense: sessions skip empty snapshots rather than sending nil frames to WebSocket clients
- HTTP server bootstrap with panic recovery middleware, request logging middleware, and `Start`/`Stop`/`Addr()` lifecycle
- Session goroutine event loop with command dispatch, snapshot broadcast to subscribers, `action_id` idempotency, `stale_seq` detection, and interruptible `autoResume` pacing
- Game interface (`session.Game`) and Hearts adapter: turn validation, phase checking, AI delegation, trick-completion pausing, and wire-compatible `CommandError` codes
- Session Manager with CRUD operations: `Create`, `Get`, `List`, `Update`, `Delete` with race-safe `sync.RWMutex` and `ErrInvalidConfig` sentinel for validation failures
- Seat-filtered snapshot generation for Hearts: player view masks opponent hands, observer view is omniscient, phase priority (`TrickComplete` > `RoundComplete` > engine phase) with tested correctness across game states
- Wire-format DTOs and runtime dependencies: game-agnostic envelopes (`InboundMessage`, `ErrorMessage`), Hearts-specific wire types (`Card`, `TrickEntry`, `PlayerSnapshot`, `ObserverSnapshot`, `PlayCardPayload`, `PassCardsPayload`), and runtime dependency declarations

### Fixed

- Human turn timeout deadlock: `NewAdapter` only created AI players for `SeatAI` seats, so when a human timed out, `AIPlay` returned an error and the game deadlocked. Now creates a fallback "random" AI player for all human seats, used exclusively by `handleTurnTimeout` while preserving the seat's human classification for normal play
- Missing snapshot broadcast after `Resume()`: trick/round/game transitions following a `StepPause` were invisible to observers because `broadcastSnapshot()` was not called after `Resume()` advanced the game state
- `finishWithGrace` grace period: reduced from 60s to 100ms since the final snapshot is already broadcast before entering the grace period
- Turn advancement in Hearts passing phase: human `pass_cards` commands left `Turn` stale, causing the session to wait indefinitely on the wrong seat. Added `advanceTurn()` helper used by both human and AI pass paths
- Marshal failure handling in subscription handlers: `handleSubscribePlayer` and `handleSubscribeObserver` silently skipped nil snapshots instead of terminating the session, leaving subscribers with no baseline state and no error
- Stale seq marshal failure: when a stale seq command arrived but the snapshot failed to marshal, the session continued instead of terminating. Now consistently fatal like all other marshal failures
- WebSocket close code on marshal failure: session now sends `1011 Internal Error` directly through the subscriber channel instead of broadcasting an `internal_error` text message and closing with `1000 Normal Closure`. Transport goroutines call `conn.Close(1011, "snapshot marshal failure")` when they receive a `SubscriberMessage` with a non-zero `CloseCode`
- WebSocket close frame documentation updated to reflect actual behavior: `1000 Normal Closure` for all normal session ends, `1001 Going Away` for server shutdown, `1009 Message Too Big` for oversized messages, `1011 Internal Error` for unrecoverable server errors
- `StepPause` routing bug: after a human play or timeout AI move returned `StepPause`, the event loop could call `AIPlay()` before `Resume()` had advanced the game past the pause phase, producing an illegal extra AI move. Fixed by routing pause outcomes through the resume cycle before further turn processing
- Stuck-session bug: invalid game adapter state (out-of-range seat, stuck turn, or `Resume` failure) left the session alive but unplayable. Now treated as fatal errors that terminate the session cleanly with subscriber cleanup and Manager notification
- Action ID cache eviction: replaced arbitrary map-key eviction with LRU so that recently-used duplicate action IDs are protected from eviction during long games
- Duplicate snapshot broadcast in session event loop: `handlePlay()` and `handleTurnTimeout()` each broadcasted once, then called `handleStepResult()` which broadcasted again for `StepPause` and `StepFinished` outcomes. Eliminated `handleStepResult()` entirely and inlined outcome dispatch so each state mutation produces exactly one broadcast
- Session lifecycle bugs: goroutine leak on natural game completion, double-close panic when `Delete` races with `StepFinished`, callers blocking forever on dead sessions, and `Delete` idempotency
