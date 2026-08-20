# Releasing cardcore-server

This document is the authoritative release process for maintainers. Keep it in sync with `scripts/release.sh`,
`.goreleaser.yaml`, and `.github/workflows/release.yml`.

## Principles

- **Semantic versioning**, pre-1.0. Releases v1.0.0 and higher are never
  tagged; the release script rejects them.
- **Tags are permanent.** The Go module proxy caches a version on first fetch,
  the repository's tag ruleset restricts `refs/tags/v*` to administrators,
  and GitHub release immutability is enabled. Never re-tag, never reuse a
  version number, never delete a tag.
- **Fix forward.** If a release has a problem, land the fix on `main` and tag
  the next patch version. Do not attempt to repair a published release.
- **Releases are never cut by hand.** `scripts/release.sh` is the single
  entry point for tagging; it rehearses the full release before creating
  anything. `scripts/prepare-release.sh` is the single entry point for the
  changelog-preparation PR; it guarantees the heading format and PR body the
  release tooling depends on.

## Release artifacts

Every release publishes seven assets, built by GoReleaser (declared as a
`go.mod` tool; see `doc/dependencies.md`):

| Asset | Contents |
|---|---|
| `cardcore-server_X.Y.Z_linux_amd64.tar.gz` | all three binaries + LICENSE, README.md, CHANGELOG.md |
| `cardcore-server_X.Y.Z_linux_arm64.tar.gz` | same |
| `cardcore-server_X.Y.Z_darwin_amd64.tar.gz` | same |
| `cardcore-server_X.Y.Z_darwin_arm64.tar.gz` | same |
| `cardcore-server_X.Y.Z_windows_amd64.zip` | same (`.exe` binaries) |
| `cardcore-server_X.Y.Z_windows_arm64.zip` | same |
| `checksums.txt` | SHA-256 sums for the six archives |

Builds are `CGO_ENABLED=0` with `-trimpath` and commit-derived timestamps, so
re-running GoReleaser on the same commit reproduces the same binaries. Each
binary reports its build information via `-version` (injected through
`-ldflags -X`; local builds report `dev`).

Release notes are the hand-curated `## [X.Y.Z]` section of `CHANGELOG.md`;
GoReleaser's commit-derived changelog generation is disabled.

## Release procedure

### 1. Prepare the changelog (its own PR)

1. Ensure everything for the release is merged to `main`.
2. Run `scripts/prepare-release.sh vX.Y.Z` (`--dry-run` rehearses without
   creating anything). The script verifies the repository state, moves the
   `[Unreleased]` items into a dated `## [X.Y.Z] - YYYY-MM-DD` section
   (leaving `[Unreleased]` empty), and opens the PR titled
   `docs(changelog): prepare vX.Y.Z release`. Never hand-edit the changelog
   for release prep or open this PR manually.
3. Merge the PR. Prepare and tag on the same day: `release.sh` warns when
   the changelog heading date is not the tag date.
4. Optional: run the benchmark spot-check against the previous tag
   (`make bench` + `go tool benchstat`) and note any >2x regressions in the
   release notes.

### 2. Cut the release

```bash
scripts/release.sh vX.Y.Z            # or: --dry-run to rehearse first
```

The script verifies, in order:

1. The version is strict semver, pre-1.0, and greater than every existing tag.
2. The tree is on `main`, clean, and exactly at `origin/main`.
3. `CHANGELOG.md` has a dated, non-empty `[X.Y.Z]` section and an empty
   `[Unreleased]` (using the same extraction as the release workflow).
4. `make check` and `go tool goreleaser check` pass.
5. A full GoReleaser snapshot rehearsal succeeds: six archives plus
   `checksums.txt`, and the host binary's `-version` output proves the
   ldflags wiring.

Only then does it create the annotated tag (`git tag -a vX.Y.Z -m "Release
vX.Y.Z"`) and push it. The interactive confirmation gates can be bypassed
with `--yes`; `--skip-snapshot` skips phase 5 (discouraged).

### 3. The release workflow

The pushed tag triggers `.github/workflows/release.yml`, which:

1. Verifies the tag points at a commit on `main`.
2. Validates the tag is proper semver (no leading zeros).
3. Runs `make check`.
4. Extracts the `[X.Y.Z]` changelog section to a `release-notes.md` in the
   runner's temp directory — never inside the checkout, where the untracked
   file would fail GoReleaser's dirty-git-state validation.
5. Runs `go tool goreleaser release --clean --release-notes=<temp file>`,
   which builds all 18 binaries (3 commands × 3 OSes × 2 architectures),
   creates the six archives and `checksums.txt`, creates the GitHub Release
   titled `vX.Y.Z` with the changelog notes, and uploads the assets.

### 4. Post-release verification

The script prints this checklist; do every step.

1. The release workflow run is green.
2. The GitHub Release has exactly seven assets.
3. Download one archive, verify it
   (`sha256sum -c checksums.txt --ignore-missing`, or
   `shasum -a 256` on macOS), extract it, and confirm
   `./cardcore-server -version` reports the tagged version.
4. The Go module proxy serves the version (the script already curls
   `https://proxy.golang.org/github.com/jrgoldfinemiddleton/cardcore-server/@v/vX.Y.Z.info`).
5. Visit `https://pkg.go.dev/github.com/jrgoldfinemiddleton/cardcore-server@vX.Y.Z`
   to trigger documentation indexing.

## Local validation

```bash
make snapshot     # full GoReleaser rehearsal into dist/, publishes nothing
make clean        # removes bin/ and dist/
```

`go tool goreleaser check` validates `.goreleaser.yaml` without building.

## Recovery

**The release workflow failed after the tag was pushed.** The tag is
permanent; assume the module proxy has already cached it. Fix the cause on
`main` (workflow, config, or changelog), then tag the next patch version.
Do not delete or move the failed tag.

**A published asset is wrong.** GitHub release immutability means assets
cannot be replaced. Publish a patch release with corrected artifacts.

**Security fix.** Land the fix, then immediately release a new patch
version. Never silently patch.

## Deferred distribution extras

These are intentionally not part of the release pipeline. Add them only when
the trigger condition is met; each is a `.goreleaser.yaml` extension plus a
note in this document and `CHANGELOG.md`.

| Extra | Trigger to add it |
|---|---|
| `.deb` / `.rpm` packages (embedded nfpm) | Debian/Ubuntu or RHEL-family users ask for them |
| cosign keyless signing of `checksums.txt` | Supply-chain verification demand; add `id-token: write` to the release workflow |
| SLSA build provenance (`actions/attest-build-provenance`) | Same; first-party action, one step |
| Homebrew tap / Scoop / winget manifests | Users request package-manager installs; tap needs a separate repository |
| macOS notarization | macOS downloads become common; requires Apple Developer Program and a `macos` runner job |
| Windows code signing (Azure Trusted Signing) | SmartScreen warnings demonstrably block adoption |
| SBOM (Syft via `go.mod` tool) | Enterprise or compliance demand |
