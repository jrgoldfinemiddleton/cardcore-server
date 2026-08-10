# AI Agent Guidance: Hearts TUI Package

## OVERVIEW
`heartstui` is the Hearts-specific slice of the TUI client: snapshot decoding, key handling, command building, and all Hearts rendering. Everything except `Client` and `CreateSession` is a pure function of data plus UI state. The game-agnostic shell lives one level up and wires this package in via the `gameClient` interface (`newGameClient` in `cmd/cardcore-tui/main.go`).

## STRUCTURE
```
games/hearts/
├── client.go           # Client: stateful adapter (snapshot decode, key handling, action IDs)
├── views.go            # Pure render functions per phase (passing, playing, round/game over, paused, deal)
├── card.go             # CardLabel, RenderCard/RenderHand, CardState styling, pass-direction labels
├── observer.go         # Observer views: all hands visible, square layout, round summary
├── commands.go         # BuildPassCommand, BuildPlayCommand producing client.Command values
├── theme.go            # Hearts Theme: embeds the shell palette, adds WinnerBg
├── session.go          # CreateSession: auto-create and start a Hearts session over HTTP
└── integration_test.go # TUI full-game integration against a real server
```

## WHERE TO LOOK
| Task | File | Notes |
|------|------|-------|
| Change key handling or cursor | `client.go` | `handlePassingKey` / `handlePlayingKey`; cursor snaps to and skips illegal cards |
| Change a phase view | `views.go` | `RenderPassingView`, `RenderPlayingView`, `RenderRoundCompleteView`, `RenderGameOverView` |
| Change card rendering | `card.go` | `RenderCard` styles by `CardState` (cursor, selected, dimmed, winner) |
| Change observer rendering | `observer.go` | `RenderObserverView`, `RenderObserverRoundCompleteView` |
| Add or change a command | `commands.go` | Envelope from `internal/client`; payloads from `internal/client/games/hearts` |
| Change Hearts colors | `theme.go` | Only `WinnerBg` is Hearts-specific; shared colors live in `cmd/cardcore-tui/theme` |
| Change session auto-create | `session.go` | `validAITypes`: `random`, `heuristic`, `pimc` |

## CONVENTIONS
- Render functions are pure: take data + UI state + a `Theme`, return a string. No I/O, no goroutines, no Bubble Tea internals.
- `Client` is the only stateful type; it holds `cursor`, `selected`, `submitted`, and `lastErr`.
- Action IDs include the seat number to avoid collisions: `tui-<seat>-<counter>`.
- `Theme` embeds the shell palette, so render functions read shared fields (e.g., `theme.Background`) via promotion; add Hearts-specific colors here and shared colors to `cmd/cardcore-tui/theme`.
- Protocol details are documented in `doc/games/hearts/protocol.md`.

## ANTI-PATTERNS
- Never add I/O to render or key-handling code; the only I/O in this package is `session.go`'s `CreateSession`, and it goes through the shared client engine rather than direct network code.
- Never use Bubble Tea internals in render functions.
- Never add shared (non-Hearts-specific) colors to `theme.go` here; they belong to the shell palette.
