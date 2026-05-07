# ADR-005: API Contract Strategy

**Date:** 2026-05-06
**Status:** Accepted

## Context
The server exposes a JSON-over-WebSocket protocol. Clients need to know the message shapes. Options range from a human-readable protocol doc to machine-readable specs (OpenAPI, JSON Schema) to code-generated SDKs.

## Decision
The API contract is defined as Go DTOs plus a human-readable protocol doc (`doc/api.md`). Generic envelope types (inbound message, snapshot base, error) live in `internal/api/`. Game-specific DTOs (snapshot fields, message payloads, phases) live in `internal/api/games/<game>/`. The protocol doc is split into a game-agnostic layer (session lifecycle, WebSocket mechanics, auth, error codes) and per-game protocol files (`doc/games/<game>/protocol.md`) that define message types, snapshot fields, and phases. A machine-readable spec (OpenAPI, JSON Schema) is introduced when a non-Go client demands it. This is a progression policy: start minimal, add formality when the cost of not having it exceeds the cost of maintaining it.

## Consequences
(+) Low overhead — Go structs are the source of truth, no spec drift. (+) The protocol doc is readable by any developer regardless of tooling. (+) Adding a machine-readable spec later is additive, not breaking. (-) Non-Go clients must hand-write DTOs from the protocol doc until a spec exists. (-) No automated contract testing across languages until then.
