# AI Agent Guidance: View Layer

## OVERVIEW
Defines the interface between the session layer and game-specific snapshot generators. Each game implements a concrete view that produces player-filtered and observer snapshots from the current game state. Game adapters call their view package's concrete functions directly when broadcasting state; conformance to the `View` interface is enforced structurally (compile-time checks), not through interface-typed calls.

## STRUCTURE
```
view/
├── view.go           # View interface: PlayerSnapshot, ObserverSnapshot
└── games/
    └── hearts/       # Hearts-specific snapshot generation
        ├── hearts.go     # ViewState, PlayerView, ObserverView
        └── bench_test.go # Snapshot serialization benchmarks
```

## WHERE TO LOOK
| Task | File | Notes |
|------|------|-------|
| Change the View interface | `view.go` | Impacts every `games/<game>/` implementation |
| Add a new game's view | `games/<game>/` | Implement `view.View`; `Clone()` engine state before masking |
| Change Hearts snapshot shape | `games/hearts/hearts.go` | `PlayerView` filters per-seat; `ObserverView` shows all hands |
| Verify interface conformance | `view_test.go` | Compile-time `var _ view.View = ...` plus runtime guard |

## CONVENTIONS
- `ViewState` (or equivalent) wraps the engine game with server-synthesized flags (e.g., `TrickComplete`, `RoundComplete`, `DealPending`, `TurnDeadline`, `Paused`).
- Always `Clone()` the engine game before reading fields for a snapshot; the original must never be mutated.
- Player snapshots hide other seats' hands; observer snapshots show all hands.
- The view package has no I/O and no goroutines; it is pure data transformation.

## ANTI-PATTERNS
- Never mutate the engine game state from a view function; clone first.
- Never add WebSocket or HTTP concerns to this package; transport lives in `internal/server/transport/`.
- Never put game-agnostic snapshot logic in `games/<game>/`; keep it in `view.go` or a shared helper.
