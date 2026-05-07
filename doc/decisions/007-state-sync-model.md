# ADR-007: State Sync Model

**Date:** 2026-05-06
**Status:** Accepted

## Context
After each game state change, connected clients need to know the new state. Options include incremental diffs (send only what changed), event sourcing (send the action that caused the change), or full snapshots (send the complete current state).

## Decision
The server sends a full seat-filtered snapshot after every state change. No incremental diffs, no patch sequences. Each snapshot contains the complete game state visible to that seat. Snapshots are idempotent — a lost, duplicated, or reordered snapshot causes no harm because the client always replaces its state wholesale.

## Consequences
(+) Eliminates synchronization bugs — client state cannot diverge from server state. (+) Reconnect is trivial: send the latest snapshot. (+) Client implementation is simple: parse and render, no merge logic. (-) Higher bandwidth per message than diffs (acceptable — card game state is a few KB). (-) Observers with all hands visible get slightly larger payloads (still small).
