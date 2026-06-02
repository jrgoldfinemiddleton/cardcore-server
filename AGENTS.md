# AI Agent Guidance (AGENTS.md)

## 1. Project Summary
Cardcore Server is a WebSocket game server and Bubble Tea TUI client for the [cardcore](https://github.com/jrgoldfinemiddleton/cardcore) engine. The server hosts card game sessions over a JSON-over-WebSocket protocol; the TUI connects as a player client. External dependencies are permitted (see `doc/dependencies.md` for the approved list).

Module: `github.com/jrgoldfinemiddleton/cardcore-server`

## 2. Codebase Map
```
cardcore-server/
├── cmd/
│   ├── server/          # Game server binary
│   ├── tui/             # Bubble Tea TUI client binary
│   └── client/          # Non-TTY CLI client binary
├── internal/
│   ├── api/
│   │   ├── api.go        # Game-agnostic message envelopes and error codes
│   │   └── games/
│   │       └── <game>/   # Wire-format DTOs and conversion functions
│   ├── client/          # Shared protocol-agnostic client engine
│   │   └── hearts/      # Hearts-specific adapter and DTOs
│   └── server/
│       ├── transport/   # HTTP/WebSocket plumbing
│       ├── session/     # Session lifecycle and game goroutine
│       └── view/
│           └── <game>/  # Seat-filtered snapshot generation
├── doc/
│   ├── api.md           # Protocol specification
│   ├── dependencies.md  # Approved external dependencies
│   └── decisions/       # ADRs — read these before making architectural changes
├── .github/
│   ├── PULL_REQUEST_TEMPLATE.md
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.yml
│   │   └── config.yml   # Redirects features/questions to Discussions
│   └── workflows/
│       ├── pr.yml             # PR validation: title check, make check
│       ├── pr-changelog.yml   # PR events: changelog update reminder
│       ├── main.yml           # Push to main: make check
│       ├── release.yml        # Tag push: validate, test, create GitHub Release
│       ├── labels-sync.yml    # Push to main: provision repository label set
│       └── labels-apply.yml   # PR events: auto-apply scope/state labels
├── scripts/
│   ├── sync-labels.sh   # Source of truth for the repository label set
│   └── apply-labels.sh  # Compute and apply labels for a PR
├── CONTRIBUTING.md      # Contribution guidelines
├── SECURITY.md          # Vulnerability reporting
├── Makefile             # Build/test/lint targets
├── .golangci.yml        # Linter config
└── README.md            # Project overview
```

## 3. Always Do
- Run `make check` before considering any change complete
- Add or update tests whenever you add or change code
- Write Go doc comments on all exported symbols
- Read the relevant ADRs in `doc/decisions/` before making architectural decisions
- Read `doc/api.md` before modifying protocol behavior
- Follow existing naming conventions: exported types are PascalCase, unexported are camelCase
- Keep the Go version in `go.mod` aligned with the minimum version stated in `README.md`
- Read `CONTRIBUTING.md` for general project conventions before making changes
- Within any file, all type/var/const declarations must precede all function declarations
- In tests, name expected-value variables `want` (and corresponding actual-value variables `got`)
- Test failure messages use `"got X, want Y"` form — no colon after `got`
- Match test layer to the package: transport tests use `httptest`, session tests use test channels, view tests assert snapshot correctness, integration tests use real server + WebSocket
- Integration tests must spin up a real server on `:0` and connect via WebSocket — no mocking the transport boundary
- Protocol conformance tests are table-driven: one row per message type × expected response
- **PR Details section format**: Use functional/component breakdown (e.g., "Reader goroutine", "Writer goroutine", "Deferred bug fixes") rather than per-file bullet points. Only mention files when the change is specifically about that file's structure or when the file name itself conveys important information
- **PR descriptions**: Follow `.github/PULL_REQUEST_TEMPLATE.md`. Omit checklist items that do not apply (e.g., skip `CHANGELOG.md` `[Unreleased]` section updated (if user-facing change) if the change is not user-facing). Keep the exact text of retained items.

## 4. Never Do
- Never add dependencies not listed in `doc/dependencies.md` without explicit approval
- Never use third-party GitHub Actions — first-party (`actions/*`) are acceptable
- Never commit with failing tests or lint errors
- Never edit the substantive content of an ADR file after its initial commit — write a new one instead (Status line is the exception)
- Never use `//nolint` directives to silence lint errors — fix the code instead
- Never tag a v1.0.0 or higher release
- Never write multi-line commit messages — use a one-line subject only and put all detail in the PR description in accordance with `.github/PULL_REQUEST_TEMPLATE.md`, excluding checkbox items that are not relevant to the PR
- Never cite `AGENTS.md` as the source of a rule from any other file in the repo
- Never manually apply `scope:*` labels to PRs — they are computed automatically from changed paths by `scripts/apply-labels.sh`. Edit the script's path rules if a label is wrong.

## 5. Development Workflow
1. Make a change
2. Run `make check` and `make race` — must pass clean
3. If lint errors appear, fix the code (do not suppress with `//nolint`)
4. Commit only when all checks pass
5. Write commit messages following [Conventional Commits](https://www.conventionalcommits.org/)
   - Format: `<type>(<scope>): <description>`
   - Types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`
   - Example: `feat(session): implement WebSocket player connection`

## 6. Key Conventions
- **Error handling**: functions return `error` as the last return value; callers must check it
- **No global state**: all state is in structs passed explicitly
- **Logging**: use `log/slog` with per-component prefixes
- **Testing**: use standard `testing` package; test files are `*_test.go` in the same package
- **Test layers**: unit (per package), integration (real server + WS), protocol conformance (table-driven), game protocol (game-specific commands + snapshots), TUI model, stress (all-AI at volume)
- **Stress tests**: run full games with all-AI sessions; gated behind a build tag so they don't run during `make check`
- **Benchmarks**: stdlib `testing.B` only; share fixtures via `*_helpers_test.go`; place `Benchmark*` after `Test*`
- **Formatting**: `gofmt` is enforced by `make check`
- **Function ordering**: follow the conventions in [CONTRIBUTING.md](CONTRIBUTING.md#code-conventions)
- **Import grouping**: stdlib, then third-party, then local (enforced by `gci` via `make lint`)

## 7. Architecture Decisions
Read `doc/decisions/` for the rationale behind key choices. Important ADRs:
- ADR-003: Repo scope — what lives here vs separate repos
- ADR-004: Strict transport boundary — no in-process shortcuts
- ADR-006: Session ownership — one goroutine per session, no locks
- ADR-007: State sync — full snapshots, no incremental diffs
- ADR-008: Authentication — capability-based seat tokens

## 8. When to Check In With the Human
- Before making any architectural change not covered by an ADR
- Before adding any dependency not listed in `doc/dependencies.md`
- Before writing or modifying any file, propose the change and wait for explicit approval
- Before installing any dev tool
- Before running ANY git or GitHub write/delete command.  Previous authorization does not constitute current approval.  Authorization must be explicit.

## 9. Maintainer Runbook
If `doc/maintainer-runbook.md` exists locally, read it for release procedures, PR review workflow, repository settings reference, and recovery steps.
