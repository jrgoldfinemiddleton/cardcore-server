# Contributing to cardcore-server

## Prerequisites

[Go](https://go.dev/) 1.25.9+. Dev tools like [golangci-lint](https://golangci-lint.run/) are managed via the `tool` directive in `go.mod` and compiled automatically on first use.

## Development Workflow

1. Fork and clone the repository.
2. Create a topic branch from `main`.
3. Make your changes. Add or update tests as needed.
4. Run `make check` — must pass clean.
5. Commit using [Conventional Commits](#commit-messages) format.
6. Open a pull request against `main`.

All pull requests (PRs) are squash-merged, so feel free to commit frequently on your branch.

## Commit Messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/). PR titles must follow one of these formats:

```
<type>: <description>
<type>(<scope>): <description>
```

**Allowed types:**

| Type | Purpose |
|---|---|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `test` | Adding or updating tests |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `chore` | Maintenance (CI, build, tooling) |

An optional `!` after the type/scope indicates a breaking change: `feat(session)!: change snapshot format`.

**Note on versioning:** The project is pre-v1.0.0. Breaking changes may occur in any release.

## Guidelines

- **Tests are required.** Every code change should include corresponding tests.
- **Run `make check`** before pushing. It runs formatting, vetting, linting, and tests.
- **Update the changelog.** Add a note under the `## [Unreleased]` section in `CHANGELOG.md` for user-facing changes.
- **Naming.** `cardcore-server` (lowercase, hyphenated) is the Go module name. In prose, use `Cardcore` for the overall project, `Cardcore Server` for the formal project name (titles, first introductions), and `the Cardcore server` in descriptive prose. `Cardcore TUI` for the terminal client.
- **External dependencies.** Approved dependencies are listed in [`doc/dependencies.md`](doc/dependencies.md). New dependencies require discussion and explicit approval before introduction.

## Testing

### Test layers

| Layer | Package(s) | What it tests |
|-------|-----------|---------------|
| Unit (api) | `internal/api/`, `internal/api/games/<game>/` | Wire-format DTO serialization round-trips, conversion function correctness (engine ↔ wire mapping). |
| Unit (transport) | `internal/server/transport/` | HTTP handler routing, middleware, request parsing, response shapes. No game logic. Uses `httptest.NewRecorder` — no real WebSocket connections. |
| Unit (session) | `internal/server/session/` | Session goroutine lifecycle, command enqueue/dequeue, seq incrementing, token validation, AI turn triggering. |
| Unit (view) | `internal/server/view/<game>/` | Snapshot projection correctness: given engine state + seat, assert correct masking (no other hands visible, correct `legal_actions`, correct scores). |
| Integration | `internal/server/` or root | Real server on `:0`, real WebSocket client, play through a full game. WebSocket upgrade, message framing, close frames, and concurrent clients use `httptest.Server` + `websocket.Dial`. Exercises the same code path as production. |
| Protocol conformance | `internal/server/` or root | Table-driven: "send this message, expect this response shape." Validates wire format against `doc/api.md`. |
| Game protocol | `internal/server/` or root | Game-specific message handling: do commands produce correct snapshots? Do game-specific error cases fire correctly? Full-game integration through all phases. Validates behavior against `doc/games/<game>/protocol.md`. |
| TUI model | `cmd/tui/` | Bubble Tea model tests: send messages, assert on model state without rendering. Visual testing is manual. |
| Stress | `internal/server/` or root | Full games with all-AI sessions across many iterations. Surfaces protocol issues, state machine edge cases, and resource leaks at volume. |

### Test helpers convention

Shared test fixtures (mock implementations, setup helpers) live in `*_helpers_test.go` files within the package. Examples: `internal/server/session/helpers_test.go` contains `mockGame`, `mockGameFactory`, `mustCreateAndStart`, `validHeartsCfg`. This mirrors the `cardcore` engine's `helpers_test.go` / `bench_helpers_test.go` pattern.

### Benchmarks

Benchmark targets:

- Snapshot serialization throughput (JSON encoding of seat-filtered state)
- Session command throughput (commands/sec through the full pipeline)
- AI turn latency end-to-end (engine call + snapshot generation + broadcast)

Benchmark conventions follow `cardcore`'s:

- Use stdlib `testing.B` only (no third-party benchmark frameworks).
- Share deterministic fixtures via `*_helpers_test.go` builders.
- Place `Benchmark*` functions after `Test*` in the file.

When changing performance-sensitive code, run benchmarks before and after and include the comparison in your PR description:

```bash
git stash
make bench 2>&1 | tee /tmp/bench-old.txt
git stash pop
make bench 2>&1 | tee /tmp/bench-new.txt
go tool benchstat /tmp/bench-old.txt /tmp/bench-new.txt
```

## Code Conventions

### Doc comments

Every exported function, method, type, and constant must have a doc comment. The comment must begin with the symbol name:

```go
// HandleConnect processes a new WebSocket connection.
func HandleConnect(w http.ResponseWriter, r *http.Request) {
```

When a doc comment references an exported identifier from a **different package** in this module, use a doc link: `[package.Type]` for same-module imports, `["import/path".Identifier]` for cross-module or stdlib when it clarifies. Do not link same-package identifiers or obvious stdlib types (`error`, `context.Context`). Doc links are optional for local/internal references when the surrounding code makes the relationship obvious.

### Function ordering

#### Declarations before functions

All type, const, and var declarations must appear before any function or method declarations.

#### Production files

1. Constructor functions (`New*`)
2. Exported methods — grouped by receiver type
3. Exported package-level functions
4. Unexported methods — grouped by receiver type
5. Unexported package-level functions

Methods on the same receiver must be contiguous.

#### Test files

1. Unit tests (`func Test*`)
2. Integration tests (`func Test*Integration`, `func Test*FullGame*`)
3. Test helpers and setup functions (at the bottom)

### Import grouping

Imports are grouped by `gci` (enforced by `make lint`):

1. Standard library
2. Third-party packages
3. Local packages (`github.com/jrgoldfinemiddleton/cardcore-server/...`)

### Before every commit

Run `make check`. Fix any failures — do not suppress with `//nolint`.

## Reporting Bugs

Use the [bug report template](https://github.com/jrgoldfinemiddleton/cardcore-server/issues/new?template=bug_report.yml) on GitHub.

## Suggesting Features

Open a [GitHub Discussion](https://github.com/jrgoldfinemiddleton/cardcore-server/discussions) to propose and discuss feature ideas before opening a PR.
