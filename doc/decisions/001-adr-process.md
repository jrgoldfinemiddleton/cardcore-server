# ADR-001: Use Architecture Decision Records

**Date:** 2026-05-06
**Status:** Accepted

## Context
We want a lightweight, durable way to record significant architectural choices. ADRs are text files that live with the code, are immutable once committed, and are easy for contributors (human and AI) to read. The Cardcore engine established this practice in [ADR-001](https://github.com/jrgoldfinemiddleton/cardcore/blob/main/doc/decisions/001-adr-process.md); we adopt the same process here.

## Decision
We will use ADRs stored in `doc/decisions/` as Markdown files. Each ADR is numbered sequentially. Each ADR carries a Status field with one of: **Proposed**, **Accepted**, **Deprecated**, **Superseded**, or **Rejected**. ADRs are never edited after their initial commit, except for the Status field.

## Consequences
(+) Decisions are traceable and self-documenting. (+) AI co-developers can read ADRs for context. (+) Consistent process across Cardcore project repositories. (-) Requires discipline to write ADRs at decision time, not retroactively.
