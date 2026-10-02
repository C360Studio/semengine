# Testing in SemEngine

This page is for a developer about to write or review a test here. It covers three questions: what the test has to
tell apart, which level to run it at, and how to show it can fail. Today the only Go code in the repository is the
test harness under `internal/harness/`, so the examples point at it.

## Start with the wrong behavior

Before writing a test, write down two things in one sentence: the correct behavior, and one specific wrong behavior
a plausible implementation could have instead. The test is useful only if it passes for the first and fails for the
second. A test that would still pass against the wrong behavior proves nothing, however many assertions it has.

Then decide what the test observes. Choose a result a caller can see (a return value, a typed error, stored state, a
message on the wire), including side effects that must *not* happen. A successful return can sit next to a bad write.

Suppose a function must accept names up to 256 bytes. The wrong behavior worth catching is an off-by-one at
the limit. Inputs of 255, 256 and 257 bytes tell the two apart; a random length between 1 and 10,000 almost never
lands on the limit.

Cover both directions where they apply. A rejection test can pass against code that rejects everything, and a
round-trip test can pass against a parser that accepts invalid input consistently.

## Expected values come from somewhere else

The expected value in an assertion must come from a source independent of the code under test: the requirement, an
external specification, or a small model written in the test. If the test computes its expectation with the same
algorithm as the implementation, it repeats the implementation's mistakes and cannot fail.

`FuzzCheckName` in `internal/harness/natsfixture/names_test.go` shows the pattern. Its oracle (the independent source
of the expected answer) is a regular expression and a list of forbidden substrings declared in the test file, and it
fails whenever `CheckName` disagrees with them.

## Pick the lowest level that can show it

Use the cheapest level that can still expose the wrong behavior. Do not move a test up a level just because the
production code touches NATS, and do not replace a real-broker check with a fake that cannot reproduce the behavior
being protected.

| Level | Command | What it can show |
|---|---|---|
| Unit | `task test:unit` | One function or in-process behavior. Runs every package with `-race`; no Docker, no network. |
| Integration | `task test:integration` | Behavior that depends on a real NATS server: JetStream delivery, key-value buckets, consumers. |
| Structural guards | `task test:unit`, `task verify` | Repository-wide rules, checked by tests in `internal/harness/contract/`. |

Consumer qualification runs live in the consumer repositories, not here.

### Unit tests

Unit tests live in ordinary `*_test.go` files and must not start containers or bind fixed ports. During iteration,
run one package directly:

```bash
go test -race -count=1 -run '^TestAwait' ./internal/harness/probe/
```

### Integration tests

Integration tests live in `*_integration_test.go` files that start with `//go:build integration`, so plain
`go test` does not build them. Run them only through the runner:

```bash
task test:integration -- ./internal/harness/natsfixture/
```

The runner (`scripts/test-integration.sh`) takes a host-wide lock so two runs never share a Docker daemon at the same
time, runs `go test -race -failfast -tags=integration -count=1 -p 2 -timeout 10m`, forwards interrupts to every test
process, and after the run removes any container left behind by that run, by ID. If you call `natsfixture.Start`
outside the runner it refuses with `ErrNotAdmitted` before touching Docker.

### Helper packages

The packages under `internal/harness/` are test-only; a contract test refuses any import of them from production code.

- `natsfixture` gives one test one disposable NATS server with JetStream in a container. `New(t)` registers cleanup;
  `Start(ctx)` returns only once JetStream answers. `Name(base)` produces run-unique resource names, and the fixture
  creates streams, buckets and consumers for you and records each one. `Stop` returns nil only after it has seen every
  owned resource, the connection and the container gone.
- `probe` lets a test observe what a component did instead of guessing from timing. `Callback` exposes entered and
  joined channels and a `Release` method for a blocked callback; `ObservedContext` signals when code first checks
  `ctx.Done()`; `Await` polls under your context and, on timeout, reports the last value and last error it saw.
- `lifecycletest` is a minimum set of checks for anything with `Start(ctx)` and `Stop(ctx)`: nil contexts refused,
  cancelled start refused, stop before start safe, repeated stop is a no-op, and so on. `Run(t, factory, promise)`
  runs them as subtests. Passing these checks does not prove a component drains and joins its own workers; that still
  needs focused tests.

The requirements behind each package are in `openspec/specs/nats-fixture/`, `lifecycle-suite/`,
`integration-test-runner/` and `harness-boundaries/`.

### Structural guards

`internal/harness/contract/` holds tests that scan the whole repository for rules such as: no production import of
test helpers, no struct holding a `context.Context`, one pinned NATS image, no fixed broker addresses in tests, no
broad Docker cleanup, and a well-formed admission ledger (`docs/admission-ledger.yaml`, the list of packages ported
from SemStreams). Four more guards are shell scripts: `task cleanup-roots:check` and `task cover:check`, which
`task verify` runs as their own steps; the fixed-port guard `scripts/lint-test-ports.sh`, which `task lint` runs; and
`scripts/merge-check.sh`, which CI's `merge-check` job runs and `task verify` does not, because it reads GitHub.

## Show that the test can fail

A green test tells you nothing until you have seen it go red for the right reason. The procedure:

1. Run the selected test on the unmodified code. It passes, and it actually ran (use `-v` and check the name).
2. Make one deliberate, plausible wrong change to the implementation, the one the test is meant to catch. Leave the
   test, its inputs and its expectations untouched. The change must compile and the test must reach it.
3. Run the test again. The intended assertion fails, for the reason you expected. A timeout, a build error, or a
   different assertion failing does not count.
4. Put the original code back and run the test again. It passes.

Keep a copy of every file before you change it (`cp`) and restore from that copy. Do not use `git checkout --`,
`git restore`, `git stash` or `git reset --hard`; they can discard other work in the tree. Record the change you made,
the commands, and the output of all three runs in the pull request.

Do this when the change:

- enforces a rule whose violation would silently lose or corrupt data, allow something forbidden, or leave work
  running after shutdown;
- emits a signal (a log line and a metric) for a skip, drop or degraded path: remove the emit, and the test must
  fail;
- fixes a bug that the existing tests let through; or
- has a specific wrong behavior that someone suspects the tests would miss.

If none applies, a one-line reason in the pull request is enough. If you cannot complete the experiment, say why and
what risk remains; the reviewer accepts or rejects that.

The repository's own checks follow the same pattern, built into the tests:

- The Go guards in `internal/harness/contract/` have paired `...Sensitivity` tests (for example
  `TestNoFixedAddressesInTests` and `TestNoFixedAddressesInTestsSensitivity`). The sensitivity test plants the
  violation in a temporary tree and requires the guard to name the planted file and what it violates, so a guard that
  fires for the wrong reason, or matches nothing, fails. Two script guards are covered the same way by
  `TestCleanupRootsCheckSensitivity` and `TestCoverCheckSensitivity`; the fixed-port script has its own fixture test,
  `scripts/lint-test-ports_fixture_test.sh`; and `TestMergeCheckKnownFlake` runs the merge check against canned
  GitHub answers.
- `TestEachFailpointTripsExactlyItsCheck` in `internal/harness/lifecycletest/` runs every lifecycle check against a
  reference component with one defect switched on at a time, and requires exactly the matching check to fail.

## Fuzz targets and property-based tests

Use a native Go fuzz target (`func FuzzX(f *testing.F)`) for code that parses, decodes or validates outside bytes or
strings. Seed it with examples of every input class it must accept and every class it must reject, and assert a rule
(it never panics, it round-trips, it agrees with an independent oracle), not a table of expected outputs.
`go test` and `task test:unit` replay only the seeds. To explore new inputs, run the fuzzer yourself:

```bash
go test -run '^$' -fuzz '^FuzzCheckName$' -fuzztime 30s ./internal/harness/natsfixture/
```

A property-based test generates structured inputs or sequences of operations and checks a rule that holds for all of
them. It is the right tool for an input format with many interacting valid and invalid cases, a transformation with a
stated law (round-trip, idempotence, normalization), or behavior that depends on the order of operations. The rule
must come from the requirement, not from reading the implementation. No property-testing library is in `go.mod`
today; adding one is a dependency change of its own.

For both tools, the generator must be able to reach the boundary the rule is about. A wide random range that only
occasionally lands on a limit catches an off-by-one by luck. Being reachable is not the same as being exercised in a
given run, so keep explicit examples for any boundary you claim the test covers.

## Concurrency and cleanup

- Wait on a signal, never on `time.Sleep`. Prefer a channel, callback or `sync.WaitGroup`; next, an injected clock;
  last, bounded polling with `probe.Await`. A longer sleep does not make an observation causal.
- Run with `-race`. `task test:unit` and `task test:integration` both do.
- Pass contexts in. Derive test contexts from `t.Context()` and narrow them with `context.WithTimeout`. Helpers take
  the caller's context rather than making their own.
- Bound cleanup. The test's context is already cancelled when `t.Cleanup` functions run, so cleanup needs its own
  context, and it must have a deadline: `context.WithTimeout(context.Background(), budget)`. `task cleanup-roots:check`
  fails any test that calls `Stop`, `Close` or `Terminate` with a bare `context.Background()` or `context.TODO()`.
- Use ephemeral ports. Listen on `127.0.0.1:0` and read back the port; reach the NATS fixture through `f.URL()`.
  `task lint` and a contract test refuse fixed `net.Listen` ports and fixed broker addresses in tests.
- Keep tests independent. A test must not depend on another test's stream, bucket, file or goroutine, and must be
  safe to run after a failure. Do not use `t.Parallel()` in tests that change process-wide state.

## Budgets and failure evidence

What the repository enforces today:

- `task cover:check` fails below 80% statement coverage on `natsfixture` (from the last integration run) and on
  `lifecycletest` and `probe` (from the unit run).
- The integration runner caps a run at a 10-minute `go test` timeout and records per-package wall time.
- One NATS fixture is one container. The fixture's cleanup gets 60 seconds.
- `task verify` prints the wall time of each step.

When a test waits on asynchronous or external state and fails, its message should name the condition, the elapsed
time, the attempts, the last value seen and the last error. `probe.Await` reports all of those except the condition;
name the condition yourself when you report its error. The NATS fixture writes a record of each start phase, the
container logs on failure, and what it still held after `Stop` to the run's evidence directory (`.evidence/` by
default). A test that uses randomness prints its seed.

## What a reviewer will ask

- What wrong behavior does this test catch, and could an implementation pass it while still being wrong?
- Where does the expected value come from, and is it independent of the code under test?
- Is this the lowest level that can show the behavior?
- If the change met one of the criteria above, where are the baseline, wrong-change, and restored runs?
- Does the generator or seed corpus reach the boundary the rule is about?
- Does anything wait on a sleep, use an unbounded cleanup context, or bind a fixed port?
- What does the test not cover?

---

Adapted from SemStreams `docs/contributing/01-testing.md` and `docs/contributing/09-property-testing.md` at commit
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`.
