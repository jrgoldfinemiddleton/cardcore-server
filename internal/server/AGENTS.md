# AI Agent Guidance: Server Package

## OVERVIEW
The `internal/server/` tree hosts the game server. It is split into three focused sub-packages, each with its own AGENTS.md:

- `session/` — session lifecycle, goroutine management, command-channel protocol, and game adapter registry.
- `transport/` — HTTP/WebSocket server, route handlers, and the strict transport boundary.
- `view/` — seat-filtered snapshot generation from game state.

The `internal/server/` directory itself contains no Go files; it is a pure namespace. All server-side code lives in one of the three sub-packages above.

## STRUCTURE
```
server/
├── session/   # Session lifecycle and game goroutine (see session/AGENTS.md)
├── transport/ # HTTP/WebSocket plumbing (see transport/AGENTS.md)
└── view/      # Seat-filtered snapshot generation (see view/AGENTS.md)
```

## WHERE TO LOOK
| Task | Sub-package | Notes |
|------|--------------|-------|
| Change session lifecycle or game loop | `session/` | `Manager` is mutex-protected; `session.run()` is the sole goroutine per session |
| Change HTTP routes or WebSocket protocol | `transport/` | All integration tests use a real server on `:0` |
| Change snapshot generation | `view/` | `View` interface; each game implements `PlayerSnapshot` and `ObserverSnapshot` |
| Add a new game | `session/games/<game>/` + `view/games/<game>/` + `api/games/<game>/` | Wire the factory in `cmd/cardcore-server/main.go` |

## CONVENTIONS
- Each sub-package owns one concern; do not cross sub-package boundaries except through the documented interfaces (`session.Game`, `view.View`, `session.Manager`).
- Game-specific code lives under `games/<game>/` in each sub-package, mirroring the same nesting across layers.
- The server never imports `internal/client/`; the client mirrors wire types independently.

## ANTI-PATTERNS
- Never add Go files directly under `internal/server/`; place them in the appropriate sub-package.
- Never import `internal/client/` from any server sub-package.
- Never bypass the `session.Manager` to mutate session state from transport handlers.
