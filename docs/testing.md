# Testing in SemEngine

This page is for a developer about to write or review a test here. It covers what the test has to tell apart, which
level to run it at, how to show it can fail, and what to record in the pull request. Most examples point at the
test harness under `internal/harness/`; the packages ported from SemStreams carry their own tests.

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
| Unit, repeated | `task test:repeat` | A test that depends on run order, on state left by another test, or on timing. Runs the unit tests five times at one CPU in shuffled order, without `-race`. |
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
- `pindiff` is not a test helper: it is the program behind `task ledger:check` and `task ledger:diff`, described
  under "Structural guards" below.

The requirements behind each package are in `openspec/specs/nats-fixture/`, `lifecycle-suite/`,
`integration-test-runner/` and `harness-boundaries/`.

### Structural guards

`internal/harness/contract/` holds tests that scan the whole repository for rules such as: no production import of
test helpers, no struct holding a `context.Context`, one pinned NATS image, no fixed broker addresses in tests, no
broad Docker cleanup, and a well-formed admission ledger (`docs/admission-ledger.yaml`, the list of packages ported
from SemStreams). Four more guards are shell scripts: `task cleanup-roots:check` and `task cover:check`, which
`task verify` runs as their own steps; the fixed-port guard `scripts/lint-test-ports.sh`, which `task lint` runs; and
`scripts/merge-check.sh`, which CI's `merge-check` job runs and `task verify` does not, because it reads GitHub
state that changes from one run to the next.

`task ledger:check` also runs `internal/harness/pindiff`, which compares every `carry` row of the ledger (a package
ported unchanged) with the pin (SemStreams at the row's `source_sha`) and fails on any difference;
`docs/provenance.md` rule 5 says what may differ and what to do when it fails. It fetches the pin. With no `carry`
row it fetches nothing; with one or more, as today, `task verify` and CI make one unauthenticated fetch from
`github.com` per distinct `source_sha` (about 3.6 s for the whole `task ledger:check` in the one measured run). A
fetch that does not answer (two minutes for all the fetches of a run) fails the check with a message that says no
entry was checked. That is a red run to re-run, not a known flake (`.agents/protocol.md`, "Known flakes"). When a
fetch is cut off, by that bound or by an interrupt, the program kills the processes it started for the fetch, git's
transport helper included, so none is left running when it returns.

## Show that the test can fail

A green test tells you nothing until you have seen it go red for the right reason. A mutation check makes one
deliberate wrong change to the code and confirms the test fails because of it. The procedure:

1. Run the selected test on the unmodified code. It passes, and it actually ran (use `-v` and check the name).
2. Make one deliberate, plausible wrong change to the implementation, the one the test is meant to catch. Leave the
   test, its inputs and its expectations untouched. The change must compile and the test must reach it.
3. Run the test again. The intended assertion fails, for the reason you expected. A timeout, a build error, or a
   different assertion failing does not count.
4. Put the original code back and run the test again. It passes.

Keep a copy of every file before you change it (`cp`) and restore from that copy. Do not use `git checkout --`,
`git restore`, `git stash` or `git reset --hard`; they can discard other work in the tree. Record the change you made,
the commands, and the output of all three runs in the pull request.

Only step 3 going red as expected counts as a detection. Record any other outcome under its own name:

- **Survivor:** the wrong change compiled and the test ran, but the test still passed. Look for the missing input,
  assertion or scope, and report the survivor until it is resolved. If you change the test as a result, start again
  from step 1 with the new test.
- **Inconclusive:** the run ended without the intended assertion failing: an error, a timeout of the whole run, a
  different or unrelated failure, or a `-run` pattern that did not select the test. A failing exit code alone is not a
  detection. Fix the cause and run the check again; never skip or weaken a test to get past it.

If the test generates its inputs, replay the same input or seed against the wrong change and against the original
code. Two different random samples differ for reasons unrelated to the change; if the same input cannot be replayed,
the outcome is inconclusive.

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
  `scripts/lint-test-ports_fixture_test.sh`; and `TestMergeCheckKnownFlake` and `TestMergeCheckReview` run the merge
  check against canned GitHub answers.
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

In the pull request, report the two kinds of run separately: the seed replay that `task test:unit` did, and any
exploratory `-fuzz` run with its exact command, its `-fuzztime`, and what it found. A green `task test:unit` is not
evidence of fuzz exploration.

A property-based test generates many inputs or sequences of operations and checks a rule that must hold for all of
them. The rule must come from the requirement, not from reading the implementation. The property-testing library is
Rapid, `pgregory.net/rapid` (admitted by the owner on 2026-10-02; SemStreams uses `v1.3.0`). It is in `go.mod` at
`v1.3.0`; the first tests that use it are the properties in `pkg/types/entity_id_prop_test.go`. A native fuzz target
that checks a rule can serve as a property test.

For both tools, the generator must be able to reach the boundary the rule is about. A wide random range that only
occasionally lands on a limit catches an off-by-one by luck. Being reachable is not the same as being exercised in a
given run, so keep explicit examples for any boundary you claim the test covers.

### Decide whether generated checks are needed

Before you write the tests, decide whether the change needs generated checks, and record the decision in the change's
design or the pull request. The decision is needed when the change touches any of:

- an input format with many interacting valid and invalid cases, or limits on size or count;
- a transformation with a stated law: round-trip, idempotence, normalization, every item lands in exactly one part;
- a history, where the outcome depends on the order of operations, on replacement or deletion, on retry or replay, or
  on shutdown.

Then either write a generated check for the rule, or name the example tests that cover it and explain why they cover
the combinations, repetitions and orderings that matter. "The existing tests pass" or a count of tests is not a
reason. There is no quota of property tests per pull request.

### Show that each assertion runs

Assertion activation is the condition under which an assertion actually executes. Check it separately from whether
the generator can reach an input. A loop over a result set that is always empty passes without checking anything; so
does an assertion behind a guard, an early return, or an operation that no generated sequence ever performs. For
every rule you claim, say what makes its assertion run and how the test gets there. If a case must run, set it up
deterministically (a fixed prefix of operations or a named example) rather than hoping a random run hits it. Do not
require a random run to reach a rare state a minimum number of times; that makes the test flaky.

### Check a history against a reference model

A reference model is a small, simplified version of the expected behavior that the test itself owns, such as a map
from each generated key to its latest value. For a history, update the model from the generated operations
according to the requirement, run the same operations against the real code, and compare what the real code shows
with the model, including effects that must not happen. Never fill the model by calling the production code, and do
not copy its decisions: the model is useful only if it can disagree with a plausible bug. A sequential model does not
test concurrent interleavings, process crashes, or broker recovery; those need their own tests at the right level.

### What a generated run leaves behind

A zero exit, or the number of checks you asked for, does not show that the checks ran. Run with `-count=1 -v` and
read the number of checks actually completed from the tool's output. Record:

- the seed of the run, so it can be repeated (a fixed seed repeats the same inputs only for the same test code and
  library version; it does not control scheduling, clocks or external state);
- the number of checks completed;
- for a failure, the failing input or operation sequence and the command that replays it. Keep an important failure
  as a named, deterministic test, because a change to the generator can change what an old seed produces.

### Running and replaying a Rapid test

Rapid's flags exist only in a package that imports Rapid, so name that package rather than `./...`.
For example, one of the `pkg/types` properties:

```bash
go test ./pkg/types -run '^TestPropEntityIDRoundTrip$' -count=1 -race -v -rapid.checks=100 -rapid.seed=1320
```

`-rapid.checks` sets how many cases to try (default 100) and `-rapid.seed` fixes the seed; `-rapid.seed=0` asks for a
fresh one. With `-v`, Rapid prints a summary of the checks it actually completed (`[rapid] OK, passed 100 tests`);
that line, not the flag, is the count to record.

`task test:unit` and `task test:repeat` pass no Rapid flags, so they run the defaults: 100 checks and a fresh seed on
every run. The five runs of `task test:repeat` therefore try different inputs, and a rare failing input can fail one
run and pass the next on the same tree. That is a failing input Rapid found, not noise: replay it with the seed it
printed before doing anything else.

A test that passes and fails on the same tree is also a known flake (`.agents/protocol.md`, "Known flakes"), and a
Rapid failure that comes and goes is treated as one. File it with the `class:flake` label and the seed Rapid printed;
merges stop until it is fixed. The fix replays that seed, corrects the code or the rule, and keeps the failing input
as a named, deterministic test.

When a check fails, Rapid prints the seed to replay it with and writes the shrunk failing case to a `.fail` file under
the package's `testdata/rapid/<TestName>/`. Replay with the printed seed, or with `-rapid.failfile=<path>`. If that
file is missing or no longer matches the generator, Rapid can log a diagnostic and fall through to fresh checks, so a
green run is not proof the failure was replayed; read the `-v` output. Pass `-rapid.nofailfile` during a mutation
check so the wrong change leaves no file behind.

A `.fail` file is untracked and is not covered by `.gitignore`. `task verify` reports untracked files but does not fail
on them, so check `git status` after a red run. Do not commit `.fail` files as they appear: record the seed and input
in the pull request, and keep a case that matters as a named, deterministic test.

Write generators that produce only valid inputs. Rapid's `*rapid.T` has `Skip`, `Skipf` and `SkipNow`, which discard
the current case, and `TestNoSkippedTests` matches any `.Skip(`, `.Skipf(` or `.SkipNow(` call in a test file,
including those.

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
- Check that a child process is alive before inspecting it. A test that looks at another process (with `ps`, a
  signal, or its output) first confirms the process is still running, so a child that died early fails with that
  cause named instead of a misleading error. In `internal/harness/prochost`, a helper process parked on a bare
  `select {}` was killed by Go's deadlock detector, so the test's `ps` call failed intermittently (CI runs
  37005148521, 37006036797 and 37013932497; fixed in 89395c2).

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

## What the pull request records

The pull request (or the change's design, linked from it) says what was expected, what was run, and what was not
covered. Fill in the first part before writing the tests and the rest after running them; leave anything not run
marked as not run.

```text
Decision:     generated check, examples enough, or not applicable; the reason says what could go wrong
Rule:         the requirement it checks, and where the expected result comes from
Inputs:       generated input or operation classes; fixed examples; what makes each assertion run
Run:          commit, exact command, seed, checks completed, result; seed replay and -fuzz exploration apart
Mutation:     the wrong change, and the baseline, wrong-change and restored runs with their outcome
Not covered:  rules not exercised, unresolved survivors, deferred checks and the reason for each
```

## What a reviewer will ask

- What wrong behavior does this test catch, and could an implementation pass it while still being wrong?
- Where does the expected value come from, and is it independent of the code under test?
- Is this the lowest level that can show the behavior?
- If the change met one of the criteria above, where are the baseline, wrong-change, and restored runs? Is any
  survivor or inconclusive run reported as such, not as a detection?
- Was the decision on generated checks recorded, with a reason that says what could go wrong and why these tests
  would catch it, not a test count?
- Does the generator or seed corpus reach the boundary the rule is about, and what makes each assertion run?
- Does a history test compare against a model the test owns, filled from the requirement and not from the code?
- Do the seed, the number of checks completed and a replay command appear, with seed replay and fuzz exploration
  reported apart?
- Does anything wait on a sleep, use an unbounded cleanup context, or bind a fixed port?
- What does the test not cover?

---

Adapted from SemStreams `docs/contributing/01-testing.md` and `docs/contributing/09-property-testing.md` at commit
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`.
