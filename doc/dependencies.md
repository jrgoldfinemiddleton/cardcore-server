# Approved Dependencies

This document lists the external dependencies approved for use in `cardcore-server`.
New dependencies require discussion and explicit approval before introduction
(see [ADR-009](decisions/009-dependency-policy.md)).

## Runtime

| Module | Purpose | License |
|---|---|---|
| `github.com/jrgoldfinemiddleton/cardcore` | Card game engine | MIT |
| `github.com/coder/websocket` | WebSocket server (context-aware, net/http native) | ISC |
| `charm.land/bubbletea/v2` | Terminal UI framework | MIT |
| `charm.land/bubbles/v2` | Reusable Bubble Tea components | MIT |
| `charm.land/lipgloss/v2` | Terminal styling | MIT |
| `github.com/charmbracelet/x/ansi` | ANSI string utilities (used by lipgloss v2) | MIT |

## Dev Tools

| Module | Purpose | License |
|---|---|---|
| `github.com/golangci/golangci-lint` | Linter aggregator (via `go tool`) | GPL-3.0 |
