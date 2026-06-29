# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/).

## [Unreleased]

### Added

- CLI compact snapshot notation with Unicode suit symbols (e.g., `seq=5 phase=passing turn=0 hand=[2♣ 3♣ 4♣]`)
- CLI environment variables: `CARDCORE_AI_TYPE` (`random`/`heuristic`/`pimc`), `CARDCORE_PACING_MS`, `CARDCORE_EXIT_DELAY_MS`
- Server environment variable: `CARDCORE_ADDR`
- TUI end-to-end integration test (`TestIntegrationTUIClientFullGame`)
- TUI phase transition views (`trick_complete`, `round_complete`, `game_over`): completed trick display with winner derived from `snap.Turn`, round score overlay with data-mismatch guards, and game-over screen with "Press Enter to exit" prompt
- `heartstui` package with pure card rendering, phase views, observer mode, and command builders; card symbols, styled rendering with cursor/selected/dimmed states, passing/playing views, and seat-prefixed action IDs to prevent cross-client collisions
- Game-agnostic TUI model with `gameClient` interface and `-game` flag for client selection; delegates all game-specific logic to adapters so the model stays protocol-only
- TUI binary `cmd/cardcore-tui/` with Bubble Tea v2: terminal UI client with WebSocket bridge, game-agnostic model state machine, error handling with flash timers, lipgloss-based layout rendering, and 14 unit tests covering state machine, WebSocket message dispatch, and error flash behavior
- CLI binary `cmd/cardcore-cli/` with phase-matched script execution (`first_n`, `first_legal`, `by_index` selectors), deterministic action IDs, and three modes: auto-create human player, auto-create observer (`--observe`), and join existing session (`--session-id` + `--token` + `--seat`)
- Client-side full game integration tests: `TestIntegrationFullLifecycle` (human player lifecycle), `TestIntegrationObserverFullGame` (observer reads until `game_over`), `TestIntegrationPlayerAndObserver` (concurrent player and observer with goroutine separation to prevent backpressure), and `TestIntegrationErrorResponse` (wrong-phase command returns `ErrorMessage` with connection left open)
- Debug logging across session, transport, and Hearts adapter with per-component `slog.With` prefixes. Test output defaults to WARN level; set `TEST_LOG_LEVEL=debug` to reveal all logs while debugging
- Full game integration tests via WebSocket: `TestAllAIFullGameIntegration` (4-AI Hearts, observer verifies phase progression and seq monotonicity) and `TestHumanAIFullGameIntegration` (1 human + 3 AI, human sends pass/play commands, game completes)
- Generic WebSocket test helpers: `testSnapshot` for fast phase/seq observation, `mustReadTestSnapshot`, `readTestSnapshot` (goroutine-safe), `writeWSJSON`, `readSnapshotsUntil`
- Subscription logging: structured `slog.Info` calls for player/observer subscribe and unsubscribe events
- Graceful shutdown: `Server.Shutdown(ctx)` sends `websocket.StatusGoingAway` to all active WebSocket connections, deletes all active sessions, and shuts down the HTTP server. `cmd/cardcore-server/main.go` binary handles SIGINT/SIGTERM with a configurable timeout via `CARDCORE_SHUTDOWN_TIMEOUT` (default 10s)
- Server binary `cmd/cardcore-server/main.go` with game adapter dispatch: currently supports `hearts`; new games can be added by extending the factory switch in `main.go`
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

### Changed

- Bumped `cardcore` engine dependency to v0.5.0: the engine now requires an explicit `*rand.Rand` for `hearts.New()` and `Deck.Shuffle()`, making seeded games fully deterministic. The server's `NewAdapter` passes its existing RNG through to the engine

### Fixed

- Server shutdown logging: `http.ErrServerClosed` from `srv.Serve(ln)` during graceful shutdown is a normal return path, not an error. The server now treats it as success instead of logging at ERROR level
- Transport write-abort logging: `net.ErrClosed` (client closed TCP) and `context.Canceled` (transport teardown) are expected races during normal client disconnection. Reclassified from ERROR to WARN
- Session cleanup logging: `session.ErrNotActive` on unsubscribe after `game_over` is expected because the session goroutine has already exited and cleaned up. Reclassified from ERROR to DEBUG
- TUI silent failure on invalid snapshot data: `RenderRoundCompleteView`, `RenderObserverView`, and `submitPlay` now return explicit "ERROR" or "Invalid state" messages instead of silently displaying incorrect zeros or no-ops
- TUI game-client decode errors silently dropped: the `gameClient` interface's `HandleSnapshot` returned nothing, so JSON unmarshal failures in the Hearts adapter were invisible to the model. Added `LastError() string` to the interface; the model now queries it after each `HandleSnapshot` call and flashes the error to the user. The Hearts adapter sets `lastErr` on envelope, player, and observer decode failures, and clears it on success
- Flaky `TestServerShutdownPropagatesGoingAwayIntegration` (`got close status -1, want 1001`), and the same race on the observer path now covered by `TestServerShutdownPropagatesGoingAwayToObserverIntegration`: shutdown could deliver an abrupt close instead of `StatusGoingAway` under CPU contention, in two ways. (1) For already-connected clients, `Server.Shutdown` deletes active sessions, which closes each subscriber channel and made the connection's writer goroutine cancel the shared read context; that cancellation tripped `coder/websocket`'s read-timeout hook, which closes the socket directly — bypassing the close-frame path and racing the `GoingAway` frame still being written. The player and observer writers now leave the read goroutine parked during shutdown instead of cancelling it, leaving `Shutdown`'s `conn.Close(StatusGoingAway, …)` as the sole connection-teardown path. (2) A connection whose upgrade completed after `Shutdown`'s close sweep was missed entirely; the player and observer handlers now send `GoingAway` themselves when the connection registers after shutdown has begun, so every accepted connection observes `1001`
- Flaky client integration tests: full-game tests capped snapshot read loops at 1000 iterations, but game length is nondeterministic run-to-run (623–899+ snapshots observed with an identical seed), so long games exceeded the cap and the test reported `never saw game_over phase`. Raised caps to 5000 (the 120s context remains the real timeout) and added last-seen `seq` logging on failure paths for diagnosability
- Server initial snapshot seq: `session.seq` started at 0, causing the first snapshot sent to new WebSocket subscribers to have `seq=0`, which the client discarded as stale (since `maxSeenSeq` initializes to 0 and the filter is `seq <= maxSeenSeq`). Fixed by initializing `seq` to 1 in `newSession()` so the initial snapshot is always accepted by fresh clients
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
