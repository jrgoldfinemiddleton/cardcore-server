# ADR-004: Strict Transport Boundary

**Date:** 2026-05-06
**Status:** Accepted

## Context
When server and client run in the same process (localhost single-player), it is tempting to bypass the network layer and call server functions directly. This creates two code paths: one for local play and one for networked play. Bugs that appear only over the network are invisible during local development.

## Decision
All client-server communication uses HTTP and WebSocket, even when server and client run on the same machine. The reference client always exercises the real API. There are no in-process shortcuts, no shared memory channels, no function-call bypasses.

## Consequences
(+) A single code path — what works locally works over a network. (+) Integration tests are production-realistic by default. (+) The protocol spec (`doc/api.md`) is the sole contract; no hidden coupling. (+) Clients are structurally prevented from corrupting game state — the process boundary enforces what the engine's precondition panics defend against. (-) Adds serialization and network overhead for local play (negligible for card game payloads). (-) Startup requires binding a TCP port even for single-player.
