# Design Principles and Philosophy

## Overview
Cardcore Server is a WebSocket game server and Bubble Tea TUI client for the [cardcore](https://github.com/jrgoldfinemiddleton/cardcore) engine. It translates engine state into a real-time multiplayer protocol, proving the API-first architecture defined in cardcore [ADR-004](https://github.com/jrgoldfinemiddleton/cardcore/blob/main/doc/decisions/004-api-architecture.md).

## Suckless Code Design
The code follows the [suckless philosophy](https://suckless.org/philosophy/): small, readable, and composable. External dependencies are permitted but tightly controlled — each must be explicitly approved and listed in `doc/dependencies.md`. The approved set is intentionally minimal: a WebSocket library, the Charm TUI stack, and the `cardcore` engine itself.

The project infrastructure — documentation, CI, convention enforcement, and contributor tooling — deliberately goes beyond what a pure suckless project would include. Cardcore is designed to be approachable by contributors who are new to Go, which requires guardrails and guidance that suckless projects targeting experienced users typically omit.

## Strict Transport Boundary
All client-server communication uses HTTP and WebSocket, even when server and client run in the same process on localhost. There are no in-process shortcuts. The TUI always exercises the real network path. This ensures a single code path and minimizes "works locally but breaks over network" bugs.

## Full-State Snapshots
The server sends a complete seat-filtered snapshot after every state change. No incremental diffs, no patch sequences. Snapshots are idempotent — a lost or duplicate snapshot causes no harm. This is viable because card game state is small (a few KB per snapshot) and eliminates an entire class of synchronization bugs.

## Capability-Based Authentication
Authorization is possession-based: opaque session IDs identify games, per-seat bearer tokens authorize play. No user accounts, no passwords, no session cookies. This model is simple for localhost and extends naturally to networked multiplayer.

## Session-Per-Goroutine Concurrency
Each game session owns a single goroutine that serializes all engine mutations. Transport handlers enqueue commands; the session goroutine processes them in order. AI turns run inside the session goroutine's control flow. No concurrent engine access, no locks on game state.

## Error Handling
The server follows `cardcore`'s error-handling convention: functions return errors for conditions the caller cannot prevent; precondition violations trigger panics. WebSocket command errors are typed events sent over the connection — the connection stays open. Close frames are reserved for unrecoverable protocol violations.

## Logging
Structured logging via `log/slog` (stdlib). Per-component prefixes (`server`, `tui`). The server logs to stderr or a file via `--log-file`; the TUI logs to a file via `tea.LogToFile()` since stdout is the terminal UI.

## Testing
Multiple layers: unit tests per internal package, integration tests that spin up a real server on a random port and connect real WebSocket clients, and protocol conformance tests that validate the API contract from `doc/api.md`. The strict transport boundary pays off here — integration tests exercise the same code path as production.

Stress testing runs full games with 4 AI players across many iterations to surface protocol correctness issues, state machine edge cases, and resource leaks that unit tests miss.
