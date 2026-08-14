# Design Principles and Philosophy

## Overview
Cardcore Server is a WebSocket game server and Bubble Tea TUI client for the [cardcore](https://github.com/jrgoldfinemiddleton/cardcore) engine. It translates engine state into a real-time multiplayer protocol, proving the API-first architecture defined in cardcore [ADR-004](https://github.com/jrgoldfinemiddleton/cardcore/blob/main/doc/decisions/004-api-architecture.md).

## Suckless Code Design
The code follows the [suckless philosophy](https://suckless.org/philosophy/): small, readable, and composable. External dependencies are permitted but tightly controlled — each must be explicitly approved and listed in `doc/dependencies.md`. The approved set is intentionally minimal: a WebSocket library, the Charm TUI stack, and the `cardcore` engine itself.

The project infrastructure — documentation, CI, convention enforcement, and contributor tooling — deliberately goes beyond what a pure suckless project would include. Cardcore is designed to be approachable by contributors who are new to Go, which requires guardrails and guidance that suckless projects targeting experienced users typically omit.

## Strict Transport Boundary
All client-server communication uses HTTP and WebSocket, even when server and client run on the same machine. There are no in-process shortcuts. The TUI always exercises the real network path. This ensures a single code path and minimizes "works locally but breaks over network" bugs.

## Full-State Snapshots
The server sends a complete seat-filtered snapshot after every state change. No incremental diffs, no patch sequences. Snapshots are idempotent — a lost or duplicate snapshot causes no harm. This is viable because card game state is small (a few KB per snapshot) and eliminates an entire class of synchronization bugs.

## Capability-Based Authentication
Authorization is possession-based: opaque session IDs identify games, per-seat bearer tokens authorize play. No user accounts, no passwords, no session cookies. This model is simple for localhost and extends naturally to networked multiplayer.

## Session-Per-Goroutine Concurrency
Each game session owns a single goroutine that serializes all engine mutations. Transport handlers enqueue commands; the session goroutine processes them in order. AI turns run inside the session goroutine's control flow. No concurrent engine access, no locks on game state.

## Client Design
Clients treat the documented protocol — not server implementation behavior — as the contract ([ADR-005](decisions/005-api-contract-strategy.md)). A game-agnostic client engine owns session lifecycle, WebSocket connections, protocol-envelope decoding, sequence deduplication, and error classification. Game-specific adapters interpret snapshots, expose legal actions, construct commands, and supply rendering and phase information. New games add adapters; new frontends reuse the engine behind platform-specific presentation.

Client core state is platform-neutral: sessions, snapshots, and commands — never terminals, colors, or keybindings. The TUI renders that state for humans; the CLI renders it for machines. Human UX and test UX are different products: the TUI optimizes readability and interaction, the CLI optimizes determinism and scriptability. They share the engine, not presentation code.

Deterministic automation is a first-class product feature. The CLI uses explicit, stable scripted input and compact, parseable output; reproducibility outranks presentation. This makes full-game client/server testing possible without a display.

Client development proceeds in complete, tested vertical slices: validate one usable path before broadening scope ([ADR-010](decisions/010-development-order.md)).

The UI is reactive, not authoritative ([ADR-007](decisions/007-state-sync-model.md), [ADR-011](decisions/011-client-snapshot-consumption.md)). At every decision point, the protocol must provide the complete set of legal actions, and clients must disable every choice outside that set. This client-side constraint is a UX obligation, not an authority boundary: the server still validates every command. After sending a command, the client waits for the next accepted snapshot or error rather than mutating game state optimistically. Clients map errors to deliberate UX states ([ADR-013](decisions/013-error-recovery-responsibilities.md)). A single component owns each connection from opening through shutdown and cancellation, and WebSocket close codes map to explicit user-visible states.

Client work will reveal missing protocol affordances. Capture those as deliberate protocol changes — never private hacks against undocumented fields — and add protocol conformance tests whenever behavior becomes client-visible ([ADR-004](decisions/004-strict-transport-boundary.md)).

## Error Handling
The server follows `cardcore`'s error-handling convention: functions return errors for conditions the caller cannot prevent; precondition violations trigger panics. WebSocket command errors are typed events sent over the connection — the connection stays open. Close frames are reserved for unrecoverable protocol violations.

## Logging
Structured logging via `log/slog` (stdlib). Per-component prefixes (`server`, `tui`). The server logs to stderr by default and to a file via the `-log-file` flag; the TUI logs to a file via `tea.LogToFile()` since stdout is the terminal UI.

## Testing
Multiple layers: unit tests per internal package, integration tests that spin up a real server on a random port and connect real WebSocket clients, and protocol conformance tests that validate the API contract from `doc/api.md`. The strict transport boundary pays off here — integration tests exercise the same code path as production.

Stress testing runs full games with 4 AI players across many iterations to surface protocol correctness issues, state machine edge cases, and resource leaks that unit tests miss.
