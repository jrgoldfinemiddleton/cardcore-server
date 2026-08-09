# AI Agent Guidance: TUI Client

## OVERVIEW
Bubble Tea TUI client binary. The top-level package is game-agnostic: it opens the WebSocket, starts the WS reader goroutine, and runs the Bubble Tea program. Game-specific rendering lives in `games/hearts/`.

## STRUCTURE
```
cardcore-tui/
├── main.go              # Entry point: parse flags, connect, run tea program
├── model.go             # tea.Model implementation: Update, View, Init
├── wsbridge.go          # Goroutine that reads WS messages and sends tea.Msg
├── layout.go            # Header/main/footer layout assembly
├── status.go            # Status bar and close/error messages
├── timeout.go           # Shared timer helpers for UI flashes
├── theme.go             # Boundary alias for the hearts theme type
├── menu/                # Pre-game menu wizard (server, AI difficulty, observer, theme)
│   ├── menu.go          # Menu Config and option definitions
│   └── model.go         # Menu tea.Model
└── games/
    └── hearts/          # Hearts-specific rendering and command building
        ├── client.go           # Stateful Client: snapshot decode, key handling
        ├── views.go            # Pure render functions per phase
        ├── commands.go         # BuildPassCommand, BuildPlayCommand
        ├── card.go             # Card symbol and lipgloss styling
        ├── observer.go         # Observer view rendering
        ├── theme.go            # Theme struct and constructors
        ├── session.go          # Auto-create Hearts session via HTTP
        └── integration_test.go # TUI full-game integration
```

## WHERE TO LOOK
| Task | File | Notes |
|------|------|-------|
| Change TUI lifecycle | `main.go` | `run()` creates `Conn`, `gameClient`, model, starts reader |
| Change global UI model | `model.go` | `model` handles tea.Msg dispatch and key events |
| Change WS-to-UI bridge | `wsbridge.go` | `startWSReader` sends typed messages to `m.program.Send()` |
| Change layout | `layout.go` | Assembles header, main content, footer |
| Change status bar | `status.go` | Error/close messages and status line |
| Change pre-game menu | `menu/menu.go`, `menu/model.go` | Menu wizard produces a `Config`; no I/O |
| Change Hearts rendering | `games/hearts/views.go`, `games/hearts/card.go` | Pure functions; no I/O |
| Change Hearts key handling | `games/hearts/client.go` | Cursor, selection, submitted flag |

## CONVENTIONS
- Render functions are pure: take data + UI state, return a string.
- `hearts.Client` is the only stateful game-specific type; it holds `cursor`, `selected`, and `submitted`.
- Use lipgloss for styling; colors are hardcoded hex values (e.g., hearts/diamonds red).
- Action IDs must include seat number to avoid collisions: `tui-<seat>-<counter>`.

## ANTI-PATTERNS
- Never put I/O or WebSocket logic in `games/hearts/`.
- Never introduce global state in the TUI model; pass the model explicitly.
- Never use Bubble Tea internals in `games/hearts/` render functions.
