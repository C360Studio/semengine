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
- `natsfixture.Restart(ctx)` stops and starts the same container, so a file-backed stream keeps its messages and a
  memory-backed one does not. Docker may map a new host port, so `URL()` and `JetStream()` are valid only until the
  next `Restart`. Stop whatever you built on the old URL (a client, a consumer) before calling `Restart`, and start
  it again from the new `URL()` afterwards. `Restart` first ends every consumer the fixture's `Consume` created and
  waits for its handlers, so never call it from inside a `Consume` handler. Handles taken from the old connection
  are dead afterwards.
- `natsfixture.FaultKV` wraps a real key-value bucket so a write fails on demand. `FailBefore(op, err)` returns
  `err` without calling the server, so nothing is written. `FailAfter(op, err)` makes the real write and then returns
  `err`, the case where the server applied the write but the caller saw an error. `op` is one of `KVPut` (which
  covers `PutString`), `KVCreate`, `KVUpdate` or `KVDelete`; every other method reaches the bucket unchanged. A
  fault stays set until you pass a nil error, and `Calls()` reports how many real calls each write made.
- `prochost` runs a helper process for tests that kill, pause or signal a process. A helper is a function in the
  test binary: register each one with `prochost.Helper(name, fn)` inside a `TestHelperProcess` function, and
  `prochost.Start(t, name)` re-runs the test binary with a marker that selects it. In a normal test run every
  `Helper` call returns at once. The child runs in its own process group, its output goes to files, and the test's
  cleanup kills and reaps it. A helper that must stay up waits for SIGTERM (`signal.Notify`, then receive), never on
  a bare `select {}`: Go's deadlock detector kills a process whose goroutines are all blocked.
- `probe` lets a test observe what a component did instead of guessing from timing. `Callback` exposes entered and
  joined channels and a `Release` method for a blocked callback; `ObservedContext` signals when code first checks
  `ctx.Done()`; `Await` polls under your context and, on timeout, reports the last value and last error it saw.
- `lifecycletest` is a minimum set of checks for anything with `Start(ctx)` and `Stop(ctx)`: nil contexts refused,
  cancelled start refused, stop before start safe, repeated stop is a no-op, a failed start holds nothing, and so on.
  `Run(t, factory, mustFail, promise)` runs them as subtests; "Services and the lifecycle suite" below says how to
  use it. Passing these checks does not prove a component drains and joins its own workers; that still needs
  focused tests.
- `pindiff` is not a test helper: it is the program behind `task ledger:check` and `task ledger:diff`, described
  under "Structural guards" below.

The requirements behind each package are in `openspec/specs/nats-fixture/`, `lifecycle-suite/`,
`integration-test-runner/`, `process-host/` and `harness-boundaries/`.

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

When the wrong change is to a Go source file that is not a test file, `task mutate:check` runs these steps for you,
edits nothing in the tree, and names the outcome by the rules under "Outcomes" below. For any other file, run the
steps by hand.

### Run it with `task mutate:check`

Write the wrong change into a copy of the file kept outside the repository. Then name the package, the test, the
file, the copy, and the assertion you expect the wrong change to make fail. In this example the wrong change stops
`probe.Await` from keeping the latest value it observed, which `TestAwaitClearsEarlierObservationError` checks:

```bash
sed 's/last, lastErr = value, err/lastErr = err/' internal/harness/probe/await.go >"${TMPDIR:-/tmp}/await.go"
task mutate:check -- -pkg ./internal/harness/probe -test TestAwaitClearsEarlierObservationError \
  -file internal/harness/probe/await.go -mutant "${TMPDIR:-/tmp}/await.go" -expect-text 'want final observation'
```

It ends with `verdict: detection` and exits zero. The flags:

- `-pkg`: one package, as `go test` takes it. A pattern with `...` is refused.
- `-test`: one top-level test, or `Name/Sub` for one of its subtests. Each part is matched exactly.
- `-file`: the file the wrong change is made to. `-mutant`: the copy with the wrong change in it.
- `-expect`: the location Go prints at the start of the expected failure line, written `file.go:N`, such as
  `probe_test.go:217`. `-expect-text`: a fixed string that line contains. Give at least one; each can be repeated.
- `-runs` (default 3), `-timeout` (default `2m`, the `go test -timeout` of each run) and `-seed` (default 1; see
  "Tests that generate their inputs").

The command runs `go test -race` on the named test `-runs` times on the unchanged code (the baseline runs), `-runs`
times with the wrong change (the mutant runs), and once more on the unchanged code (the after-run). If every mutant
run passes, it makes one more run to find out whether the test reached the wrong change at all (the reach run, under
"How reach is judged"). For a mutant run, Go compiles the copy in place of the file: the command passes Go's
`-overlay` build option, which maps a file's path to another file's content, through `GOFLAGS`. A program the test
itself builds with `go build` or `go run` gets the wrong change too, and no file in the repository is written. A run
that has not ended by twice `-timeout` is stopped, with the processes it started in its process group, and counts as
inconclusive. Ctrl-C or SIGTERM stops the run in progress, and the command exits non-zero with no verdict.

It prints a report to paste into the pull request. The report shows the commit and the Go version, the wrong change
as a diff, the expected locations and texts, the seed, each run's command line, `GOFLAGS`, result and failure lines,
the file's SHA-256 checksum before and after, and a fingerprint of the tree before the first run and after the last
(a short hash, from `scripts/tree-state.sh`, of the commit, the uncommitted changes and the untracked files git does
not ignore). Its last line is the verdict and the reason. The command exits zero only for detection. The log
directory the report names is on your machine only.

Before it starts any run, the command refuses, naming the reason, when:

- the file is a `_test.go` file (the experiment leaves the test unchanged), or is not Go source: a script, YAML, test
  data, or another file a test reads while it runs. An overlay replaces only what the compiler reads, so check those
  by hand;
- the copy is inside the repository, or is identical to the file;
- neither `-expect` nor `-expect-text` is given;
- `GOFLAGS`, including a value set with `go env -w`, already sets `-overlay` or a coverage flag (`-cover`, `-coverpkg`,
  `-covermode`, `-coverprofile`). With coverage on, Go compiles the file on disk and the wrong change would not run;
- `-seed` is 0, or an untracked file sits under the package's `testdata/rapid/` (see "Tests that generate their
  inputs").

The `mutation-check` spec lists every refusal. The command does not run integration tests: plain `go test` does not
build a file tagged `integration`, so the test is not found and the outcome is inconclusive.

The command applies the rules; it does not judge. Whether the wrong change is plausible, and whether the assertion you
named is the one meant to catch it, stay with you and the reviewer. Three baseline runs catch a test that fails often;
a rarer flaky test can pass all three.

### Run it by hand

Use this for a wrong change the command refuses. Keep a copy of every file before you change it (`cp`), and record
its SHA-256 checksum. Restore from that copy, and check that the checksum matches:

```bash
cp scripts/cover-check.sh "${TMPDIR:-/tmp}/cover-check.sh.bak" && shasum -a 256 scripts/cover-check.sh   # before
cp "${TMPDIR:-/tmp}/cover-check.sh.bak" scripts/cover-check.sh && shasum -a 256 scripts/cover-check.sh   # after
```

Do not use `git checkout --`, `git restore`, `git stash` or `git reset --hard`; they can discard other work in the
tree. Do not check the restore with `git diff` either: it shows nothing for an untracked file. Record the change you
made, the commands, and the output of all three runs in the pull request.

### Outcomes

Only step 3 going red as expected counts as a detection. Record any other outcome under its own name:

- **Detection:** every baseline run passed, every run with the wrong change failed with the expected assertion, and
  the after-run passed.
- **Survivor:** the wrong change compiled and the test ran, but the test still passed. Look for the missing input,
  assertion or scope, and report the survivor until it is resolved. If you change the test as a result, start again
  from step 1 with the new test.
- **Invalid:** the wrong change does not compile, or the test never reached it (see "How reach is judged"). Either
  way it says nothing about the test's assertions. Fix a wrong change that does not compile. For one the test never
  reached, look for a missing input, or choose a wrong change on the path the test is meant to cover; then check
  again.
- **Inconclusive:** the run ended without the intended assertion failing: an error, a timeout of the whole run, a run
  killed by a signal, a different or unrelated failure, or a `-run` pattern that did not select the test. Runs with the
  wrong change that disagree, a baseline or after-run that failed, and a change to the tree during the check also give
  inconclusive. A failing exit code alone is not a detection. Fix the cause and run the check again; never skip or
  weaken a test to get past it.

When the wrong change makes something never finish, a test can still detect it if the test waits with a time limit of
its own and fails with its own message when that limit runs out (for example, a `probe.Await` whose context has a
deadline): that failure line is an assertion like any other. The timeout of the whole `go test` run cannot detect it.
It shows only that the run did not finish, not which check caught the wrong change, so the outcome is inconclusive.
When you need a detection from such a test, give the test its own time limit.

A report from Go's race detector, the line `race detected during execution of test`, counts as the expected failure
only when you named it before running: with the command, `-expect-text 'race detected during execution of test'`.
Otherwise a run whose only failure is a race report is inconclusive. Name it when the wrong change removes a lock and
the race detector is the check meant to catch that. Once it is named, a race anywhere in the package that the wrong
change sets off counts as well; the report shows where the race was.

Go prints a `t.Log` line the same way as a failure line, so a log line at an expected location matches too. The
report prints every matched line in full: read them to confirm the failure is the one you meant. A panic or a further
failure after the expected line is listed in the report and does not change a detection.

Only lines printed by the named test and its subtests are matched. When `-test` names a subtest (`Name/Sub`) and the
code panics inside it, `go test -json`, which the command reads, files the panic text under the parent test, `Name`.
That text cannot match the subtest's `-expect-text`, so unless the subtest printed an expected line before the panic,
the outcome is inconclusive. An expected line printed before the panic still gives a detection, and the report lists
the panic either way. If the wrong change is likely to panic, name an assertion that fails before the panic, or give
`-test` the parent test, whose output includes the panic text.

A survivor is equivalent when the wrong change alters nothing the code promises, for example by swapping two
independent assignments. Equivalent is never an outcome: the outcome stays survivor. Whether a survivor is equivalent
is the reviewer's assessment, recorded in the pull request with the contract the code is held to, the inputs
considered, the reasoning, and who assessed it.

### How reach is judged

The test reaches a wrong change when it runs at least one of the changed lines. A wrong change the test never runs
cannot make it fail, so a passing run with the wrong change says nothing about the test until reach is shown. By
hand, show it yourself (step 2). The command shows it with the reach run: one more run of the unchanged code with
Go's coverage of the file's package, made only when every run with the wrong change passed.

The unchanged code and the wrong change run the same statements on the same inputs until a run first enters a changed
line. So if the unchanged code ran a changed line, the runs with the wrong change got there too. Go's coverage records
how many times each coverage block ran: a coverage block is a stretch of statements with no branch between them,
given by its first and last line and column. From the coverage of the unchanged code, the command reads:

- for lines the wrong change removes or replaces: whether a coverage block that overlaps them ran;
- for lines it only inserts: whether the coverage block holding the statement right after the insertion ran, taking
  that statement from the same `{}` block or `case` clause; at the end of the block or clause, the statement right
  before the insertion. A different branch of the same `if`, `switch` or `select` never decides.

Reached gives survivor; not reached gives invalid. When coverage cannot answer, the outcome is survivor and the report
says that reach could not be measured. That happens when:

- no coverage block overlaps the removed or replaced lines, as for a declaration or an import outside any function;
- the insertion lies outside any function body, as a new method or type does;
- the insertion goes into an empty `{}` block (a function body, a branch, a loop body) or an empty `case` or `default`
  clause, which has no statement to decide it;
- the insertion lies inside a statement written over several lines, or between a label and its statement;
- the insertion ends a block or clause whose last statement is labeled;
- no coverage block holds the statement that would decide.

When the wrong change is in several places (the report's hunks), one place reached is enough for reached. If none is
reached and one cannot be measured, reach could not be measured.

The reach run shows one path through the unchanged code. A line that runs only on some schedules, or only in a child
process the test starts (which writes no coverage), can read as not reached, and the outcome is then invalid. A
coverage block counts as run once it is entered, not line by line, so a line after a call that panics, or right after
a `return`, can read as reached although it never ran, and the outcome is then survivor. The report lists the coverage
blocks that decided reach.

### Tests that generate their inputs

If the test generates its inputs, replay the same input or seed against the wrong change and against the original
code. Two different random samples differ for reasons unrelated to the change; if the same input cannot be replayed,
the outcome is inconclusive.

The command does this for Rapid. Every run gets `RAPID_SEED` set from `-seed` (default 1) and `RAPID_NOFAILFILE=true`,
so every run starts from the same seed and Rapid writes no failure file. The seed decides whether the generated inputs
hit what the wrong change breaks, so a survivor holds only for that seed, and its reason says so: choose a seed that
finds the wrong change. A seed of 0 is refused, because Rapid reads it as "choose a random seed". The command also
refuses to start while an untracked file sits under the package's `testdata/rapid/`: Rapid replays every file there on
every run, and a failure file left by an earlier run could turn a survivor into a detection. A committed file there is
a fixed input, replayed in every run alike. A native fuzz target needs nothing more: plain `go test` replays only its
seeds.

### When to run one

Do this when the change:

- enforces a rule whose violation would silently lose or corrupt data, allow something forbidden, or leave work
  running after shutdown;
- emits a signal (a log line and a metric) for a skip, drop or degraded path: remove the emit, and the test must
  fail;
- fixes a bug that the existing tests let through; or
- has a specific wrong behavior that someone suspects the tests would miss.

If none applies, a one-line reason in the pull request is enough. If you cannot complete the experiment, say why and
what risk remains; the reviewer accepts or rejects that.

### Built into the repository's own checks

The repository's own checks follow the same pattern, built into the tests:

- The Go guards in `internal/harness/contract/` have paired `...Sensitivity` tests (for example
  `TestNoFixedAddressesInTests` and `TestNoFixedAddressesInTestsSensitivity`; the deployment-authority checks
  `TestNoDeploymentAuthorityNames` and `TestNoDeploymentAuthorityNamesSensitivity`, and `TestNoSecondAuthorityField`
  and `TestNoSecondAuthorityFieldSensitivity`). The sensitivity test plants the violation in a temporary tree and
  requires the guard to name the planted file and what it violates, so a guard that fires for the wrong reason, or
  matches nothing, fails. Two script guards are covered the same way by
  `TestCleanupRootsCheckSensitivity` and `TestCoverCheckSensitivity`; the fixed-port script has its own fixture test,
  `scripts/lint-test-ports_fixture_test.sh`; `TestMergeCheckKnownFlake` and `TestMergeCheckReview` run the merge
  check against canned GitHub answers; and `TestSpecQueueHolds`, `TestSpecQueueFirstLineCaveats`,
  `TestSpecQueueCheck` and `TestSpecCheckWiring` run the OpenSpec queue script, `scripts/openspec-queue.sh`, against
  a stand-in `openspec` command and planted `tasks.md` files.
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

## Services and the lifecycle suite

A service here is a type the engine starts, supervises and stops, whose `Start` can fail: `metric.Server` and
`natsclient.Client` today. Every service runs the whole `lifecycletest` suite:

```go
lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})
```

- `factory` returns a fresh, unstarted owner each time it is called.
- `mustFail` is required. It returns an owner whose `Start` fails for a real reason, such as a port the test is
  already holding or a refused local address. The suite checks that the failed `Start` returned an error, that the
  owner then holds nothing, and that a following `Stop` returns nil and makes no call. `Run` fails before any check
  if `mustFail` is nil.
- `Promise{Restart: true}` says a stopped owner accepts a second full start and stop; without it, a second `Start`
  must be refused.

The suite judges completion through `Observe()`, which reports what the owner still holds. Production types do not
grow an `Observe` method. Instead, write an adapter in a `_test.go` file in the service's own package, where it can
read unexported fields; `metric/lifecycle_test.go` (`serverOwner`) and `natsclient/client_lifecycle_test.go` are the
two in the tree. Checklist for an adapter:

- `Unresolved` names every kind of thing the service holds while started, with a stable name each: connections,
  subscriptions, consumers, key-value watchers, listeners, goroutines, tickers and timers.
- `Calls` counts every cleanup or external call the service makes (a bind, a shutdown, a close), so the suite can
  show that a second `Stop` did nothing.
- The suite cannot notice a kind the adapter leaves out. A reviewer compares the adapter's list with every field
  the service retains.

## Background work

Background work is a goroutine that outlives the call that started it, in code that is not a service. It stops in
one of three ways, and never by waiting a fixed time:

| Shape | Use it when | Example in the tree |
|---|---|---|
| `Run(ctx) error`, preferred | The caller can run the loop on a goroutine it owns. `Run` returns when `ctx` ends, and its return is the join. | `internal/resource.Watcher.Run` |
| `Close() error`, or a stop function | The goroutine waits only on what the stop controls, such as its own ticker or done channel. `Close` cancels it and waits with no timeout. | the TTL and hybrid caches in `internal/cache` |
| `Shutdown(ctx) error` | The goroutine waits on something the stop does not control: a caller's callback, in-flight requests, network I/O. `Shutdown` waits within `ctx` and returns `ctx.Err()` if it ends first; the goroutine exits once what it waited on returns, and a later `Shutdown` returns nil. | `internal/cache.CoalescingSet.Shutdown` |

Two more rules apply to every shape:

- A nil context is refused at the call that receives it: with an error where the call returns one, otherwise with
  a panic at the call before any goroutine starts, never with a panic inside the goroutine.
- The code has a unit test that starts the work, uses it and stops it inside `synctest.Test`. The bubble returns
  only when every goroutine started in it has exited, so the test fails if the stop left one running. Examples:
  `TestWatcher_Run_ReturnsOnCancelAndLeavesNothing` (`internal/resource`), `TestTTLCacheCloseLeavesNothingRunning`
  and `TestCoalescingSetShutdownLeavesNothingRunning` (`internal/cache`).

The rule is the `background-work` capability in `openspec/specs/`.

## Porting a test from SemStreams

A ported `_test.go` file lands already meeting this page's rules: no sleeps, no skips, no hidden build tags, no fixed
addresses, and green under `task test:repeat`. Each repair is recorded on the package's ledger row, by line at the
SemStreams pin and in SemEngine. The repairs fall into five classes:

- **Sleep replaced by a signal.** Code whose timing comes from its own timers runs inside `synctest.Test`, where
  `<-time.After(d)` advances a fake clock and `synctest.Wait` settles the bubble before an assertion (`t.Parallel`
  cannot be called inside a bubble). A test against a real broker waits on a channel, a callback or `probe.Await`;
  a real-clock timer is allowed only as the failure bound of that wait, taken from the test's context or at least 10
  seconds. A real-clock interval that the behaviour itself is defined by (an ack wait, a TTL, a drain window) is
  allowed only when its expiry can never fail a correct implementation, and only where the change's design lists it.
- **Skip removed.** A test skipped because it needs a broker moves into an `//go:build integration` file instead.
- **Old build tag removed.** A `// +build` line is deleted; the `//go:build integration` line stays.
- **Repeat failure fixed.** A ported test that fails `task test:repeat` is fixed in the porting pull request, by
  removing its cause, and never filed as a flake.
- **Fixed address removed.** A test binds an ephemeral port or uses the fixture's `URL()`.

A test of a feature that the port removes is deleted with it, and the deletion is recorded on the row like a repair.

## Budgets and failure evidence

What the repository enforces today:

- `task cover:check` fails below 80% statement coverage on each package in its target list
  (`scripts/cover-check.sh`): `natsfixture` (from the last integration run); `lifecycletest`, `probe`, `message` and
  `payloadregistry` (from the unit run); and `natsclient` (the unit and integration runs merged).
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
