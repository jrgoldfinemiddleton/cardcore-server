# ADR-010: Development Order

**Date:** 2026-05-06
**Status:** Accepted

## Context
The server is to support multiple modes: single-player (human + AI), all-AI demo, and networked multiplayer. Building everything simultaneously risks never shipping. We need to sequence development so that each phase validates the architecture before the next phase depends on it.

## Decision
Single-player (one human + AI opponents) and all-AI demo mode ship first. These modes validate the full architecture — session lifecycle, WebSocket protocol, snapshot generation, AI integration, and the TUI client — without the complexity of multiple human connections, latency compensation, or network authentication. Multiplayer extends the same session model once the foundation is proven.

## Consequences
(+) Early validation — architectural flaws surface with minimal code written. (+) All-AI demo mode enables automated stress testing from day one. (+) The TUI is usable immediately for development and testing. (-) Multiplayer is deferred (intentionally — it adds complexity that shouldn't pollute the initial architecture). (-) Some design choices (e.g., single seq counter, no reconnect) may need revision for multiplayer.
