# PROJECT KNOWLEDGE BASE

## OVERVIEW
`cardcore-server` is a WebSocket game server and Bubble Tea TUI client for the `cardcore` engine. It hosts card games over a JSON-over-WebSocket protocol. Module: `github.com/jrgoldfinemiddleton/cardcore-server`. Go 1.25.9.

## STRUCTURE
```
cardcore-server/
├── cmd/
│   ├── cardcore-server/     # game server binary entry point
│   ├── cardcore-tui/        # Bubble Tea TUI client (game-agnostic shell + hearts/)
│   └── cardcore-cli/        # scripted non-TTY client (game-agnostic shell + hearts/)
├── internal/
│   ├── api/                 # wire-format envelopes and error codes
│   ├── client/              # shared HTTP/WebSocket client engine
│   └── server/
│       ├── transport/       # HTTP/WebSocket server and route handlers
│       ├── session/         # session lifecycle and game goroutine
│       └── view/            # seat-filtered snapshot generation
├── doc/
│   ├── api.md               # protocol spec
│   ├── decisions/           # ADRs (architecture decision records)
│   └── dependencies.md      # approved external dependencies
├── .github/workflows/       # CI/CD (first-party actions only)
├── scripts/                 # label sync/apply and repo configuration
├── Makefile                 # build/test/lint targets
└── .golangci.yml            # lint config
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Add a game | `internal/server/session/games/<game>/`, `internal/api/games/<game>/`, `internal/server/view/<game>/`, `internal/client/<game>/`, `cmd/cardcore-tui/<game>/`, `cmd/cardcore-cli/<game>/` | Follow the Hearts vertical slice; wire the factory in `cmd/cardcore-server/main.go` |
| Change HTTP/WS routes or handlers | `internal/server/transport/` | `Server` registers routes; `http_sessions.go` for REST, `ws_player.go`/`ws_observer.go` for WebSockets |
| Change session lifecycle | `internal/server/session/` | `Manager` is the mutex-protected registry; `session.run()` is the single goroutine |
| Change protocol messages | `internal/api/api.go` | `InboundMessage`, `ErrorMessage`, and error codes are shared across server and clients |
| Change client engine | `internal/client/` | `SessionClient` (HTTP), `Conn` (WebSocket), `messages.go` (Command envelope) |
| Change TUI rendering | `cmd/cardcore-tui/hearts/` | Pure render functions; `Client` holds cursor/selection state |
| Change CLI formatting | `cmd/cardcore-cli/hearts/` | `Formatter` (compact output), `Builder` (script actions), `session.go` (create helpers) |
| Read architecture policy | `doc/decisions/` | ADRs-004, 006, 007, 008 are the critical ones |

## CODE MAP
| Symbol | Type | Location | Refs | Role |
|--------|------|----------|------|------|
| `Manager` | Struct | `internal/server/session/manager.go:53` | 21 | Thread-safe session registry |
| `Server` | Struct | `internal/server/transport/server.go:22` | 24 | HTTP/WebSocket server |
| `InboundMessage` | Struct | `internal/api/api.go:20` | 60+ | Client-to-server message envelope |
| `ErrorMessage` | Struct | `internal/api/api.go:28` | 15 | Server-to-client error envelope |
| `Game` | Interface | `internal/server/session/game.go` | 10+ | Bridge between session goroutine and game implementations |
| `SessionClient` | Struct | `internal/client/http.go` | 20+ | HTTP client for session lifecycle |
| `Conn` | Struct | `internal/client/ws.go` | 15+ | WebSocket client connection |
| `Command` | Struct | `internal/client/messages.go` | 15+ | Client command envelope |
| `DefaultDelays` | Struct | `internal/server/session/manager.go:68` | 10+ | Server-wide timing defaults |

## CONVENTIONS
- `make check` is the local gate before any change; CI also runs `make race`.
- Run `go vet ./...`, `go test ./...`, and `golangci-lint` via `go tool` (declared in `go.mod`).
- Add or update tests for every code change; integration tests use a real server on `:0` and a real WebSocket.
- Exported symbols must have doc comments starting with the symbol name; declarations must precede functions.
- Test helpers must call `t.Helper()`; expected values are named `want`, actual values `got`; failure messages use `"got X, want Y"`.
- All state lives in structs passed explicitly; no global variables.
- Use `log/slog` with a `"component"` key for per-component log prefixes.
- PR descriptions follow the template in `.github/PULL_REQUEST_TEMPLATE.md` and use functional/component breakdowns, not per-file lists.
- PR titles must follow Conventional Commits with a space after the colon: `^(feat|fix|docs|test|refactor|chore)(\(.+\))?!?:[[:space:]].+`.

## ANTI-PATTERNS (THIS PROJECT)
- Never add a dependency not listed in `doc/dependencies.md` without explicit approval.
- Never use third-party GitHub Actions; only `actions/*` are allowed.
- Never commit with failing tests or lint errors.
- Never edit the substantive content of an ADR after its initial commit; write a new ADR instead (Status is the only mutable field).
- Never use `//nolint` to silence lint errors; fix the code. `convention_test.go` enforces this.
- Never tag a v1.0.0 or higher release.
- Never write multi-line commit messages; use a one-line subject and put detail in the PR description.
- Never cite `AGENTS.md` as the source of a rule from any other file in the repo.
- Never manually apply `scope:*` labels to PRs; `scripts/apply-labels.sh` computes them from changed paths.

## UNIQUE STYLES
- **Vertical slice per game**: Hearts-specific code is split across `internal/api/games/hearts/`, `internal/server/session/games/hearts/`, `internal/server/view/hearts/`, `internal/client/hearts/`, `cmd/cardcore-tui/hearts/`, and `cmd/cardcore-cli/hearts/`. Each layer owns its game-specific concerns.
- **Strict transport boundary**: All integration tests use a real HTTP/WebSocket server; no in-process shortcuts (ADR-004).
- **One goroutine per session**: `session.run()` is the sole goroutine that touches game state; the `Manager` is mutex-protected, not a goroutine (ADR-006).
- **Full snapshots only**: Every state change broadcasts a complete snapshot; no incremental diffs (ADR-007).
- **Capability-based auth**: Seat tokens are bearer credentials surfaced only on session creation/update; `Get`/`List` never return tokens (ADR-008).
- **AST convention enforcement**: `convention_test.go` walks the module to enforce function ordering, doc comments, no `//nolint`, and `doc.go` presence.

## COMMANDS
```bash
make check        # fmt + vet + lint + test (local gate)
make race         # run all tests with -race
make build        # compile binaries to bin/
make lint-extra   # extra-strict lint config
go test ./...     # run all tests
go test -race ./...   # run tests with race detector
go run ./cmd/cardcore-server
go run ./cmd/cardcore-tui
go run ./cmd/cardcore-cli -script script.json
```

## NOTES
- `make check` does not run the race detector; `make race` does.
- Dev tools (`golangci-lint`, `pkgsite`) are declared via Go 1.25's `tool` directive in `go.mod`.
- Stress tests are planned but not yet implemented; when added they must be gated by a build tag so they do not run during `make check`.
- The `internal/client` package mirrors some server DTOs intentionally; client types have JSON tags and decouple the client from server internals.

## Maintainer Runbook
If `doc/maintainer-runbook.md` exists locally, read it for release procedures, PR review workflow, repository settings reference, and recovery steps.

## Implementation Plans
If `doc/implementation-plans.md` exists locally, read it for details pertaining to ongoing implementations, including plans, guidelines, architecture details.

## Future Considerations
If `doc/future-considerations.md` exists locally, read it for a list of proposed features and improvements, including triggers for when they may be relevant for further consideration or implementation.
