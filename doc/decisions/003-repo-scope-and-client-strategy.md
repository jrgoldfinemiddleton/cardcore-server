# ADR-003: Repo Scope and Client Strategy

**Date:** 2026-05-06
**Status:** Accepted

## Context
The Cardcore engine ([ADR-004](https://github.com/jrgoldfinemiddleton/cardcore/blob/main/doc/decisions/004-api-architecture.md)) mandates that all clients are thin and talk to a server over HTTP/WebSocket. We need to decide what lives alongside the Cardcore server in this repository versus in separate repositories, and which client serves as the reference implementation that proves the API is sufficient.

## Decision
This repository contains the game server and a reference terminal client built with Bubble Tea. Other clients (web, mobile, desktop) reside in separate repositories in their respective languages. The TUI serves as the reference client: it proves the API is sufficient for any client implementation by exercising every endpoint and message type.

## Consequences
(+) Server and reference client evolve together — API changes are validated immediately. (+) Contributors only need Go tooling to develop the full stack. (+) The TUI doubles as a developer tool for testing the server.
