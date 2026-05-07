# ADR-002: Documentation Structure

**Date:** 2026-05-06
**Status:** Accepted

## Context
The project needs documentation that serves multiple audiences: contributors, AI co-developers, and future maintainers. We want to avoid over-documenting early while still making key decisions legible. The Cardcore engine established a three-tier structure in [ADR-002](https://github.com/jrgoldfinemiddleton/cardcore/blob/main/doc/decisions/002-documentation-strategy.md); we adopt the same approach here.

## Decision
Three-tier doc structure: (1) `README.md` — project overview and quick start; (2) `doc/design.md` and `doc/architecture.md` — deeper design and system explanations; (3) `doc/decisions/*.md` — ADRs for significant choices. Go doc comments on all exported symbols provide API documentation via `pkgsite` or `go doc`. The protocol specification lives in `doc/api.md`.

## Consequences
(+) Each layer has a clear purpose. (+) README stays short. (+) ADRs are self-contained. (+) Consistent structure across Cardcore project repositories. (-) Requires writers to choose the right layer for each piece of information.
