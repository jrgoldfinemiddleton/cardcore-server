# AI Agent Guidance: Server Architecture Decisions

## Scope

This directory contains Architecture Decision Records (ADRs) — durable design policies that govern the cardcore-server project. ADRs state laws, not status reports.

## How to Use

1. **Before making any architectural change**, read the relevant ADR(s)
2. **If no ADR covers your situation**, propose one before implementing
3. **If an ADR is wrong**, write a new ADR that supersedes it — never edit substantive content of an existing ADR after initial commit

## Conventions

- Sequential numbering (`001-`, `002-`, ...)
- `Status` field is the only mutable part after initial commit
- Status values: `Proposed`, `Accepted`, `Superseded`, `Deprecated`
- Self-contained: a reader should never need to consult a superseded ADR

## Key ADRs

| ADR | Topic | Critical Rule |
|-----|-------|---------------|
| 001 | ADR process | How we write and maintain ADRs |
| 004 | Strict transport boundary | No in-process shortcuts — always HTTP/WebSocket |
| 006 | Session ownership | One goroutine per session, no locks on game state |
| 007 | State sync model | Full snapshots, no incremental diffs |
| 008 | Authentication | Capability-based: session ID + per-seat bearer tokens |
| 009 | Dependency policy | Stdlib-first; external deps require explicit approval |
| 010 | Development order | Hearts-first, then generic abstractions when second game demands |

## Anti-Patterns

- **Never cite AGENTS.md in an ADR** — ADRs are the authority, not derived from guidance
- **Never write "X is deferred"** — Frame as "X is introduced when Y demands it"
- **Never put mutable lists (approved deps, etc.) in ADRs** — Use living documents referenced by ADRs
