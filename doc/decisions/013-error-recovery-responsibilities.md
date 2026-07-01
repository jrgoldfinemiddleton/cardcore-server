# ADR-013: Error Recovery Responsibilities

**Date:** 2026-07-01
**Status:** Accepted

## Context

This ADR supersedes [ADR-012](012-error-recovery-responsibilities.md).

ADR-012 assumed all server error responses were recoverable and that clients should retry. Implementation revealed two distinct classes: fatal errors (coding bugs that a well-structured client prevents entirely) and non-fatal errors (inherent timing races the client cannot prevent). Interactive and non-interactive clients need different treatment because only interactive clients can show messages and wait for human acknowledgment.

## Decision

**Server contract.** The server sends an `error` message for every rejected command and keeps the WebSocket connection open. The server never retries, auto-corrects, or closes on behalf of the client. `game_over` is terminal — the session has ended.

**Two classes of errors.** Client recovery behavior depends on whether the error is inherent to distributed systems (non-fatal) or indicates a coding bug (fatal).

- **Fatal errors** (`illegal_move`, `malformed_message`, and equivalent): the client must exit immediately. Structured clients prevent these via client-side validation before sending.
- **Non-fatal errors** (`stale_seq`, `out_of_turn`, `wrong_phase`, and equivalent): the game continues. Interactive clients show a dismissible message and resume on the next snapshot. Non-interactive clients silently resync.

**`game_over` is terminal.** The session has ended. The client displays final scores and exits cleanly. No retry — the session is finished.

**Structured client obligations.** Well-structured clients — those that validate commands client-side before sending and track `maxSeenSeq` — must never ask the user to retry after a fatal error and must always allow graceful continuation after a non-fatal one.

**Duplicate `action_id`.** The server returns the cached snapshot silently. The client processes it as any snapshot (ignoring it if `seq <= maxSeenSeq`). This is not an error condition.

## Consequences

(+) Client behavior is predictable across platforms: every error has a defined class and response. (+) Integration tests assert specific error codes and state transitions without guessing server behavior. (+) Non-fatal error messages explain what happened ("AI played for you") rather than blaming the user. (+) Fatal error messages explain that the user did nothing wrong ("Bug: server rejected a valid card"). (-) Interactive clients must implement a modal state machine. (-) Non-interactive clients need a separate error-handling path.
