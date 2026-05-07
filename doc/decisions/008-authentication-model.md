# ADR-008: Authentication Model

**Date:** 2026-05-06
**Status:** Accepted

## Context
Players need to prove they are authorized to act for a specific seat in a specific session. The system must distinguish between seats (e.g., four in standard Hearts) without requiring user accounts or passwords (overkill for localhost play, and the engine has no concept of user identity).

## Decision
Capability-based authentication: opaque session IDs identify games, per-seat bearer tokens authorize play. Tokens are generated with `crypto/rand` (32 bytes, hex-encoded) at session creation and delivered in the `POST /sessions` response. Possession of the token is the authorization — no further identity verification. Tokens are immutable once issued.

## Consequences
(+) Simple — no user accounts, no password storage, no session cookies. (+) Extends naturally to multiplayer: each human player receives their seat token out-of-band. (+) Session ID in URLs does not grant seat access (defense in depth). (-) Token leakage grants full seat control (acceptable for localhost; multiplayer adds TLS).
