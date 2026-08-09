# AI Agent Guidance: CLI Client

## OVERVIEW
Non-interactive scripted client. The top-level package parses flags and runs either a player loop (scripted actions) or an observer loop (read-only). Game-specific formatting and command building lives in `games/hearts/`.

## STRUCTURE
```
cardcore-cli/
├── main.go              # Entry point, parseFlags, run modes
├── script.go            # ScriptExecutor: evaluates snapshots against JSON script
├── integration_test.go  # Full-game scripted tests
├── testdata/            # JSON script fixtures for integration tests
└── games/
    └── hearts/
        ├── session.go       # CreateHumanSession, CreateObserverSession
        ├── format.go        # Formatter: compact one-line snapshot output
        └── script.go        # Builder: BuildCommand for pass_cards/play_card
```

## WHERE TO LOOK
| Task | File | Notes |
|------|------|-------|
| Change CLI modes | `main.go` | `runPlayer()`, `runObserver()`, `createHumanSession()` |
| Change script execution | `script.go` | `ScriptExecutor` maps phase to action |
| Change Hearts output format | `games/hearts/format.go` | `Formatter.FormatSnapshot` |
| Change Hearts script selectors | `games/hearts/script.go` | `first_n`, `first_legal`, `by_index` |
| Add Hearts session helper | `games/hearts/session.go` | HTTP create + start helpers |

## CONVENTIONS
- Output is compact one-line text (e.g., `seq=5 phase=playing ...`).
- Unicode suit symbols are used in output: `♣`, `♦`, `♥`, `♠`.
- Scripts are JSON arrays of `{phase, action, selector, selector_args}`.
- `runObserver()` is read-only; `runPlayer()` drives actions from the script.

## ANTI-PATTERNS
- Never make `games/hearts/script.go` depend on terminal or UI code.
- Never put game-agnostic CLI logic in `games/hearts/`.
- Never hardcode game-specific behavior in `main.go` beyond wiring the Hearts builder/formatter.
