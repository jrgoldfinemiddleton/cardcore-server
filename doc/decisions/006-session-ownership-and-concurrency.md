# ADR-006: Session Ownership and Concurrency

**Date:** 2026-05-06
**Status:** Accepted

## Context
A game session involves mutable state (the Cardcore engine), multiple connected clients (players and observers), and AI players that need to take turns. Concurrent access to mutable game state is a common source of race conditions and deadlocks.

## Decision
Each game session has one owner goroutine that serializes all engine mutations. Transport handlers enqueue commands via a channel and subscribe to snapshot broadcasts. AI turns run inside the session goroutine's control flow — they are synchronous calls, not separate goroutines. There are no mutexes on game state.

## Consequences
(+) No concurrent engine access — race conditions are structurally impossible. (+) The session goroutine is the single authority on turn order, seq numbering, and state transitions. (+) AI delay is trivially implemented as a sleep within the session goroutine. (-) A slow AI blocks other commands for that session (acceptable — AI delay is bounded and configurable). (-) Sessions are independent; no cross-session coordination (fine for card games).
