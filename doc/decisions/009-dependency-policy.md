# ADR-009: Dependency Policy

**Date:** 2026-05-06
**Status:** Accepted

## Context
The Cardcore engine uses zero external dependencies (stdlib only). The server and TUI have different constraints: WebSocket handling and terminal rendering benefit from battle-tested libraries. We need a policy that balances "minimal dependencies" against "don't reinvent the wheel."

## Decision
External dependencies require explicit justification and approval before introduction. The approved dependency list is maintained in a living document (`doc/dependencies.md`), not in this ADR. This ADR states the policy; the list evolves independently.

**Approval criteria:** the library must solve a problem that would require substantial effort to implement correctly with stdlib alone, must have a permissive license (MIT, ISC, BSD, Apache-2.0), and must be actively maintained.

## Consequences
(+) Dependencies are intentional, not accidental — every one is a conscious decision. (+) The living document can be updated without writing a new ADR. (+) Contributors know where to look for what's allowed. (-) Adds friction to adopting new libraries (intentional friction). (-) Requires periodic review of the approved list for staleness.
