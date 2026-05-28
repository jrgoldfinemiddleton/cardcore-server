# ADR-012: Error Recovery Responsibilities

**Date:** 2026-05-27
**Status:** Accepted

## Context

The server rejects client commands for multiple reasons, such as stale sequence, wrong turn, illegal move, wrong phase, finished game, or malformed message. Ambiguity about who owns recovery leads to fragile client implementations that retry blindly, ignore errors, or disconnect unnecessarily.

## Decision

The server sends an `error` message for every recoverable rejection, then keeps the WebSocket connection open. The server never retries, auto-corrects, or closes the connection on behalf of the client. The client owns all recovery decisions for recoverable errors. `game_over` is terminal — the session has ended.

Commands rejected with `stale_seq` are never cached for `action_id` idempotency. The client may safely retry the same `action_id` with a corrected `seq`.

| Error code | Server behavior | Client responsibility |
|---|---|---|
| `stale_seq` | Returns fresh snapshot + error. Connection open. | Update `maxSeenSeq` from the snapshot, retry command with corrected `seq`. Same `action_id` is safe — stale_seq commands are not cached. |
| `out_of_turn` | Returns error with current `seq`. Connection open. | Wait for snapshot indicating it is this seat's turn; retry only after `turn` matches seat. |
| `illegal_move` | Returns error with game engine explanation. Connection open. | Display message to user; retry with a different payload. |
| `wrong_phase` | Returns error with current `seq`. Connection open. (Game-specific — the session delegates phase validation to the game adapter.) | Wait for snapshot indicating correct phase; retry only after phase change. |
| `game_over` | Returns error for commands in queue when the session ends. Connection closes with `1000 Normal Closure` after final snapshot. | Disable input; render final state from last snapshot; handle close gracefully. No retry — the session is finished. |
| `malformed_message` | Returns error, omits `action_id` if unparseable. Connection open. | Fix message format; never retry identical malformed payload. |

Duplicate `action_id` is not an error — the server returns the cached snapshot silently. The client processes it as any snapshot and stops retrying.

## Consequences

(+) Client logic is predictable: every rejection is recoverable (except terminal `game_over`), every error has a clear next step. (+) Integration tests can assert specific error codes and client state transitions without guessing server behavior. (+) Connection stability: a single illegal move never severs the WebSocket. (-) Client must implement distinct handlers for each error code — more state machine complexity than a generic "something went wrong" path. (-) `game_over` arrives only for commands already in flight when the session ends; a well-behaved client that parsed the final snapshot's terminal state should not be sending commands at that point.
