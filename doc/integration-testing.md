# Full-Game Integration Testing Principles

This note collects reusable guidance for full-game integration tests. It is meant to be principle-driven, not a catalog of one-off field names from a particular test.

## 1. Test the production path, not a shortcut

A full-game integration test should exercise the same boundary real clients use:

- real server on an ephemeral port (`:0`)
- real HTTP/WebSocket upgrade
- real protocol messages
- real session goroutine
- real game adapter
- real client read/write behavior

No in-process shortcuts, mocked transports, or helper APIs that bypass protocol behavior. The point is to prove the transport/session/client boundary works under real message flow. See [ADR-004](decisions/004-strict-transport-boundary.md) for the policy this enforces.

**Isolate integration tests in their own file.** Full-game tests import packages unit tests do not need (server constructors, game adapters, transport internals). Keeping them in `integration_test.go` or `fullgame_test.go` makes the dependency boundary explicit and prevents unit-test files from accumulating server-internal imports.

**Always call `t.Parallel()`.** Every full-game test should call `t.Parallel()` so the Go test runner can execute multiple games concurrently. Each test spins up its own server on `:0`, so parallel execution has no port conflicts.

## 2. Start from the smallest working full-game path, then add complexity

Do not start with the most complex scenario.

Recommended build-up:

1. Server starts and accepts a session.
2. One connection can receive snapshots.
3. A full all-AI/observer game reaches terminal state.
4. A human-player game reaches terminal state.
5. A player + observer game reaches terminal state.
6. Error handling works without closing the connection.
7. Stress/concurrency variants only after the above are reliable.

When a complex full-game test fails, reduce it back toward the last known working shape. Do not keep layering fixes onto the complex version first.

## 3. Make game randomness deterministic

Full-game tests still need realistic gameplay, but failures must be reproducible.

Use deterministic RNG seeded from the test name. That gives each test a stable game sequence while avoiding every test playing the exact same game.

The principle is not “use FNV specifically”; the principle is:

> Full-game tests should use deterministic-but-distinct seeds so a failing hand/order/round can be reproduced exactly.

## 4. Use pacing deliberately

The server can produce snapshots faster than clients can consume them. Its subscriber buffer is finite. Once it fills, snapshots are dropped permanently.

So pacing is not cosmetic. It is part of test correctness.

**Default to a small nonzero pacing delay for all full-game tests.** A value of 10ms is appropriate for this project and matches the transport-level full-game tests. Zero pacing is only safe when the test has proven backpressure that keeps the producer from outrunning the consumer under parallel and race-detector load. Even human-player tests can fail with `AIActionDelayMS: 0` if the client read loop cannot keep up.

General rule:

> Prefer realistic, small pacing over “fastest possible” pacing. Fast tests that drop terminal snapshots are worse than slower tests that prove the intended behavior.

## 5. Give full-game tests race-detector-safe deadlines

A test that passes normally can timeout under `-race` because the race detector slows goroutine scheduling, JSON work, and WebSocket IO.

Use deadlines sized for `make race`, not just normal `go test`.

Principle:

> Full-game test timeouts should fail true hangs, not punish expected race-detector slowdown.

For this project, 120s matches the server-side full-game tests and is appropriate for full lifecycle tests.

## 6. Own the drain path intentionally

Any connection receiving high-throughput snapshots must be drained continuously. A goroutine reader is only useful if its output is also drained. Otherwise the goroutine blocks on send, stops reading the WebSocket, the server writer backs up, the server subscriber buffer fills, and snapshots get dropped.

**The main goroutine must own the drain loop for any high-throughput stream.** If the main goroutine is busy with other work (command construction, JSON parsing, assertions), the stream that only reads must not be left undrained. Common pattern: run the active participant (player) in a goroutine and let the main goroutine drain the passive participant (observer). Use a buffered error channel so the goroutine can send its final error and exit without blocking.

Causal chain:

> main drains consumer channel → channel does not fill → goroutine keeps reading WebSocket → server writer keeps draining subscriber channel → snapshots are not dropped → terminal state is observed.

## 7. Use one ordered result channel per reader

If a goroutine reads snapshots, send both data and errors through one ordered result channel:

```go
type result struct {
    data json.RawMessage
    err  error
}
```

Do not split data and errors into separate channels. Separate channels can reorder observation: the test may select an EOF/error before processing the already-buffered final snapshot.

Principle:

> Preserve wire-read order all the way to the assertion loop.

## 8. Assert the journey, not just the destination

A full-game test that only checks `game_over` is too weak. It can miss bugs where intermediate lifecycle behavior is skipped, malformed, duplicated, or delivered out of order.

Assert:

- sequence numbers strictly increase for each subscriber
- required phase progression occurred
- human players were actually prompted to act
- commands were accepted without protocol errors
- observers saw terminal state, not just “some snapshots”
- error paths return protocol errors without killing the connection

Terminal state is necessary, but not sufficient.

## 9. Source commands only from server-authoritative snapshot data

Tests should not invent “reasonable” game commands. They should use data the server just gave them.

Principle:

> A full-game protocol test should prove client/server interaction, not hard-code assumptions about game legality.

Examples:

- Pass cards actually in the player’s current hand.
- Play from currently legal actions, not from the first card in hand.
- Include current seq/action identifiers in the shape expected by the client API.

## 10. Treat EOF/close as meaningful, not harmless

A closed WebSocket is only expected after the test has already observed the terminal state it was waiting for.

Do not accept “EOF after some snapshots” as success. That hides the most important failure mode: missing `game_over` because the consumer fell behind and the server dropped the final snapshot.

Principle:

> Connection close is acceptable only after the test has already observed the expected terminal condition.

## 11. Make failures specific

Use independent flags/assertions for the important milestones instead of one generic “game did not complete.”

Good failures say:

- never saw passing turn
- never saw playable turn
- observer missed `game_over`
- required phase was absent
- seq stopped increasing
- server returned wrong error code
- connection closed before terminal state

This makes debugging a failed full-game test take minutes instead of a timeout cycle plus log archaeology.

## 12. Debug strategically; do not wait for timeouts

Timeouts are expensive and low-information. If a full-game test appears stuck, stop trying random tweaks and inspect the pipeline.

Recommended debug order:

1. **Check latest observed phase/turn/seq**  
   Determines whether the game is advancing, waiting on a human, or stuck in a pause/timeout path.

2. **Check whether commands are being sent when the human is turn owner**  
   If the game waits for a human and the test never sends a valid command, it will hang forever.

3. **Check server rejection logs**  
   `wrong_phase`, `illegal_move`, `out_of_turn`, or `stale_seq` usually explain why progress stopped.

4. **Check subscriber-buffer warnings**  
   `subscriber channel full, snapshot dropped` means the consumer is too slow. Fix drain architecture or pacing; do not merely increase loop limits.

5. **Check whether terminal state was observed before close**  
   EOF before terminal state is a failure. EOF after terminal state is normal.

6. **Run the failing test alone**  
   Distinguish intrinsic test-design bugs from parallel-load/race-detector sensitivity.

7. **Compare against the closest working full-game test**  
   Duplicate the proven shape first, confirm it still works, then add one variable at a time.

8. **Escalate early**  
   If two fixes fail, consult a reviewer/critic before trying a third. Concurrency failures are usually structural, not solved by arbitrary sleeps or buffer tweaks.

**Key log lines to grep for when a full-game test hangs:**

| Log line | What it tells you |
|---|---|
| `phase` and `turn` on every snapshot | Confirms the game is advancing through states |
| `action rejected` with `error_code` | Reveals why the server rejected a client command |
| `broadcastSnapshot` with `seq` | Confirms the server is generating and sending snapshots |
| `driveTurns done` with `status` | Reveals whether the session goroutine exited cleanly or fatally |
| `subscriber channel full, snapshot dropped` | Consumer is too slow; fix drain architecture or pacing |
| `turn timeout scheduled` / `turn timeout, playing AI move` | Confirms the turn loop is running and AI is acting |

## 13. Assessment checklist for a new full-game test

Before calling a full-game test “good,” verify:

- It uses the real server and real WebSocket path.
- It can run with `t.Parallel()`.
- It uses deterministic game setup.
- It has a race-detector-safe deadline.
- Its pacing matches the consumer pattern.
- Every high-throughput stream is continuously drained.
- Goroutine communication preserves ordering.
- The main goroutine owns assertions.
- It asserts seq monotonicity.
- It asserts phase progression.
- It proves all participating roles saw what they are supposed to see.
- It includes at least one negative/error-path test where relevant.
- It passes alone, in the package suite, under `make check`, and under `make race`.
- Its comments explain non-obvious concurrency, pacing, and protocol assumptions.

## 14. Clean up and set up logging before you need it

Every resource created in a test must be released so a panicking or failing test does not leave goroutines or listeners behind:

- `defer cancel()` for contexts
- `defer conn.Close()` for WebSockets
- `t.Cleanup(func() { _ = srv.Shutdown(ctx) })` for servers

**Debug logging is a prerequisite.** Before writing any integration test, ensure the package has a `logger_test.go` with `TestMain` that reads `TEST_LOG_LEVEL` and configures `slog`. Without it, hangs are nearly impossible to diagnose.

## 15. Anti-patterns to avoid

- "It passed once" as proof.
- Letting full-game tests timeout repeatedly while guessing.
- Increasing timeout to hide a hang.
- Increasing buffer size without understanding who drains it.
- Accepting EOF before terminal state.
- Checking only `game_over`.
- Using arbitrary game commands.
- Splitting data/errors across channels.
- Copying shared observer state before the observer has finished.
- Starting with player + observer + error handling all at once instead of layering from a working baseline.
