# Contributing to cardcore-server

## Prerequisites

[Go](https://go.dev/) 1.25.12+. Dev tools like [golangci-lint](https://golangci-lint.run/) are managed via the `tool` directive in `go.mod` and compiled automatically on first use.

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
- **Log output during tests.** By default only WARN and ERROR `log/slog` output is printed to stderr. To reveal all structured logs while debugging a failing test, set `TEST_LOG_LEVEL=debug`:

  ```bash
  TEST_LOG_LEVEL=debug go test ./internal/server/session/...
  TEST_LOG_LEVEL=debug make race
  ```

- **Update the changelog.** Add a note under the `## [Unreleased]` section in `CHANGELOG.md` for user-facing changes.
- **Naming.** `cardcore-server` (lowercase, hyphenated) is the Go module name. In prose, use `Cardcore` for the overall project, `Cardcore Server` for the formal project name (titles, first introductions), and `the Cardcore server` in descriptive prose. `Cardcore TUI` for the terminal client.
- **External dependencies.** Approved dependencies are listed in [`doc/dependencies.md`](doc/dependencies.md). New dependencies require discussion and explicit approval before introduction.

## Testing

For guidance on designing, assessing, and debugging full-game integration tests, see [doc/integration-testing.md](doc/integration-testing.md).

### Test layers

| Layer | Package(s) | What it tests |
|-------|-----------|---------------|
| Unit (api) | `internal/api/`, `internal/api/games/<game>/` | Wire-format DTO serialization round-trips, conversion function correctness (engine ↔ wire mapping). |
| Unit (client engine) | `internal/client/`, `internal/client/games/<game>/` | Client DTO JSON round-trips, error classification, HTTP session lifecycle (`httptest`), WebSocket message filtering (`maxSeenSeq`), command builder correctness. |
| Unit (transport) | `internal/server/transport/` | HTTP handler routing, middleware, request parsing, response shapes. No game logic. Uses `httptest.NewRecorder` — no real WebSocket connections. |
| Unit (session) | `internal/server/session/` | Session goroutine lifecycle, command enqueue/dequeue, seq incrementing, token validation, AI turn triggering. |
| Unit (game adapter) | `internal/server/session/games/<game>/` | Game-specific adapter logic: config validation, engine integration, command dispatch. |
| Unit (view) | `internal/server/view/games/<game>/` | Snapshot projection correctness: given engine state + seat, assert correct masking (no other hands visible, correct `legal_actions`, correct scores). |
| Integration | `internal/server/transport/`, `internal/client/`, `cmd/cardcore-cli/`, `cmd/cardcore-tui/games/<game>/` | Real server on `:0`, real WebSocket client, play through a full game. WebSocket upgrade, message framing, close frames, and concurrent clients use `httptest.Server` + `websocket.Dial`. Exercises the same code path as production. |
| Protocol conformance | `internal/server/transport/` | Table-driven: "send this message, expect this response shape." Validates wire format against `doc/api.md`. |
| Game protocol | `internal/server/transport/`, `internal/server/session/games/<game>/` | Game-specific message handling: do commands produce correct snapshots? Do game-specific error cases fire correctly? Full-game integration through all phases. Validates behavior against `doc/games/<game>/protocol.md`. |
| TUI model | `cmd/cardcore-tui/`, `cmd/cardcore-tui/games/<game>/` | Bubble Tea model tests: send messages, assert on model state without rendering. Visual testing is manual. |
| Stress | `internal/server/transport/` or root | Full games with all-AI sessions across many iterations. Surfaces protocol issues, state machine edge cases, and resource leaks at volume. |

### Test helpers convention

Shared test fixtures (mock implementations, setup helpers) live in `*_helpers_test.go` files within the package. Examples: `internal/server/session/helpers_test.go` contains `mockGame`, `mockGameRegistry`, `mustCreateAndStart`, `validHeartsCfg`. This mirrors the `cardcore` engine's `helpers_test.go` / `bench_helpers_test.go` pattern.

Fixtures that are reused across multiple packages live in standalone packages:

- `internal/testutil/` — cross-package helpers such as deterministic Hearts fixtures and session/client config builders.
- `internal/server/transport/testutil/` — real server setup for transport and integration tests.

### Bubble Tea test safety

Bubble Tea v2 tests that run a `tea.Program` are prone to data races and hangs because the program runs its own event loop while the test goroutine inspects model state. Follow these rules whenever a `cmd/cardcore-tui/` test starts a program or accesses model state concurrently:

- **Treat every helper that reads model state as concurrent.** Methods like `waitForMsg` or `getState` that read model fields from outside the event loop must guard access with `sync.Mutex`.
- **Synchronize mutable slices and maps shared with the event loop.** Guard both the event-loop mutation and every test-side read with the same mutex. Operations such as `append` and `len` can race because appending may reallocate a slice's backing array.
- **Run `make race` before pushing.** `make check` does not run the race detector.
- **Always disable interactive input with `tea.WithInput(&bytes.Buffer{})`.** `tea.WithInput(nil)` attempts to open `/dev/tty` and blocks forever.
- **Always bound the program with `tea.WithContext(ctx)`** using a short timeout (e.g., `context.WithTimeout(ctx, 3*time.Second)`) so tests exit even when something goes wrong.
- **Let the event loop start before sending.** After `go p.Run()`, wait briefly (e.g., 50ms) before launching goroutines that call `p.Send()` — `Send` blocks until the loop is reading.
- **Poll model state with a deadline; never busy-wait.** Tests may need to wait for the event loop to process messages — for example, until `m.msgs` contains an expected entry. A tight loop such as `for len(m.msgs) == 0 {}` does not deliberately yield and can starve the event-loop goroutine that must update the state, especially on single-core or CPU-constrained systems. It also hangs forever if the message never arrives. Read the state under its mutex, release the lock, sleep briefly between checks, and fail with a useful message when an overall deadline expires.
- **Shut the program down with a deadline.** Call `p.Quit()` and wait for `p.Run()` to return with a timeout so a failed test cannot leak a running program.

Production models need no explicit synchronization: the framework calls `Update` sequentially on the event-loop goroutine, and `p.Send()` is thread-safe. Mutexes are only for test code that reads model state from outside the loop.

### Benchmarks

Benchmark targets:

- Snapshot serialization throughput (JSON encoding of seat-filtered state)
- Session command throughput (commands/sec through the full pipeline)
- AI turn latency end-to-end (engine call + snapshot generation + broadcast)

Benchmark conventions follow `cardcore`'s:

- Use stdlib `testing.B` only (no third-party benchmark frameworks).
- Share deterministic fixtures via `*_helpers_test.go` builders.
- Place `Benchmark*` functions after `Test*` in the file.

Benchmarks are not yet implemented. When the benchmark suite is added, this section will be updated with the comparison workflow.

## Code Conventions

### Doc comments

Every exported function, method, type, and constant must have a doc comment. The comment must begin with the symbol name:

```go
// HandleConnect processes a new WebSocket connection.
func HandleConnect(w http.ResponseWriter, r *http.Request) {
```

Every constant in a grouped const declaration must have its own doc comment, and every field of an exported struct must have a doc comment beginning with the field name:

```go
const (
	// PhasePlaying indicates that trick-taking is in progress.
	PhasePlaying = "playing"
	// PhasePaused indicates the Hearts game is paused.
	PhasePaused = "paused"
)

type Card struct {
	// Rank is the rank name, for example "ace".
	Rank string `json:"rank"`
}
```

`convention_test.go` enforces these rules: `TestDocComments` covers functions and methods, and `TestConstAndFieldDocComments` covers constants and exported struct fields.

Use links in comments to help readers navigate to referenced resources. Square brackets are reserved for Go doc links; other targets use a `See` line.

- **Go symbols** — use standard Go doc links: `[package.Symbol]` for a symbol in another package of this module (e.g., `[session.Game]`), and `[package.Symbol.Method]` with the full chain inside the brackets for methods and fields (e.g., `[view.View.PlayerSnapshot]`). Do not link same-package identifiers; name them plainly instead. For standard-library or external-module symbols, use the full import path (e.g., `[log/slog.Default]`); the short form (e.g., `[slog.Default]`) is allowed only in files that already import the package; in a package doc comment (`doc.go`) the short form is allowed when any file in the package imports it. Do not link obvious stdlib types (`error`, `context.Context`).
- **ADRs** — reference from code comments as `See ADR-NNN (doc/decisions/NNN-slug.md).` Go doc links cannot target Markdown files, so do not bracket ADR references. Place the reference in the package `doc.go` or near the code that embodies the decision.
- **Repository docs** — reference from code comments as `See doc/<path>.md[#anchor].` (e.g., `See doc/api.md#websocket-inbound-messages.`). In Markdown files, use normal Markdown links with relative paths for in-repo targets and full URLs only for cross-repo targets.
- **External resources** — put a bare URL on a `See` line (e.g., `See RFC 6455 §7.4: https://datatracker.ietf.org/doc/html/rfc6455#section-7.4.`).

`convention_test.go` (`TestDocLinks`) verifies that bracketed doc links resolve and that `ADR-NNN` and `doc/....md` references in comments point at existing files.

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

1. Interface checks (`var _ T = (*Impl)(nil)`)
2. Unit tests (`func Test*`)
3. Integration tests (`func Test*Integration`, `func Test*FullGame*`)
4. Benchmarks (`func Benchmark*`)
5. Fuzz tests (`func Fuzz*`)
6. Examples (`func Example*`)
7. Test helpers and setup functions (at the bottom)

See `convention_test.go` for the canonical ordering logic.

### Import grouping

Imports are grouped by `gci` (enforced by `make lint`):

1. Standard library
2. Third-party packages
3. Local packages (`github.com/jrgoldfinemiddleton/cardcore-server/...`)

### Project conventions

In addition to the rules above, contributors should follow these conventions:

- **Test assertions:** name expected values `want` and actual values `got`.
- **Test helpers:** call `t.Helper()` at the start of every test helper function.
- **Logging:** use `log/slog` and include a `"component"` key for per-component log prefixes.
- **Global state:** keep all state in structs passed explicitly; avoid global variables.
- **Commit messages:** use a single-line subject; put detail in the PR description.
- **Doc comments:** end with a period.
- **Line length:** keep lines within 100 characters.
- **Function length:** keep functions under roughly 100 lines / 60 statements.
- **Package docs:** every Go package must contain a `doc.go`.
- **Agent guidance files:** nested `AGENTS.md` files are committed. When you change a package's structure or conventions, update its `AGENTS.md` in the same PR; `convention_test.go` verifies that paths referenced in those files exist.

The project validates these expectations through `convention_test.go`, `.golangci.yml`, and `.golangci-extra.yml`. Run `make check` and `make lint-extra` to exercise them.

### Before every commit

Run `make check`. Fix any failures — do not suppress with `//nolint`.

## Reporting Bugs

Use the [bug report template](https://github.com/jrgoldfinemiddleton/cardcore-server/issues/new?template=bug_report.yml) on GitHub.

## Suggesting Features

Open a [GitHub Discussion](https://github.com/jrgoldfinemiddleton/cardcore-server/discussions) to propose and discuss feature ideas before opening a PR.
