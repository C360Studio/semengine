# Design: lifecycleguard-tidy

Depends on `INVENTORY PASS` for `inventory.md`; pins are at the base commit. Behaviour, not mechanism: the developer
chooses locks and field layout, settled by tests under `-race`.

## D1. One refusal shape for a nil or ended context

- One unexported helper (suggested name `refuseContext(ctx, method)`) is the first call in both `Start` and `Stop`,
  before any state is read:
  - nil context: `errs.WrapInvalid(errs.ErrInvalidData, "lifecycleguard", method, "nil context")`;
  - ended context: `errs.WrapInvalid(ctx.Err(), "lifecycleguard", method, "context already ended")`;
  - otherwise nil.
  - `method` ("Start" or "Stop") only names the method in the message. It switches no behaviour.
- Observable result:
  - both methods return an error classified invalid (`errs.IsInvalid`);
  - a nil context's error matches `errs.ErrInvalidData`;
  - an ended context's error matches that context's error under `errors.Is`, which `CheckAbortStopPreservesCause`
    (`lifecycletest.go:220`), `CheckPreCancelledStartRefused` (`:157`, `:162`) and `TestComponent_Stop_Timeout`
    (`component_integration_test.go:120`) need, and which ADR-113's "returns that error" keeps for PR 2;
  - nothing is acquired, run or used up, as today.
- Start changes one token (`ErrInvalidConfig` becomes `ErrInvalidData`) and its nil text. Stop's ended context gets
  Start's existing wrapper.
- The wait path stays bare (`guard.go:108-109`). That is Stop's bound running out while it waits for a running
  `Start`, not a refusal of the input it was given. `metric.Server` makes the same split (`metric/handler.go:73-74`
  refuses at entry; `:248-249` reports the bound bare).
- Sentinel options, with the tree's count (inventory section 2):
  - (a) `ErrInvalidData`, chosen. The guard's own `Stop` uses it already, and so does `metric.Server`, the other
    lifecycle owner under the suite, in both `Start` and `Stop`. That makes 5 sites, and it matches its class:
    `IsInvalid` lists it.
  - (b) `ErrInvalidConfig`: one site in the tree, and its sentinel's own class (`IsFatal`, `pkg/errs/errs.go:186`)
    disagrees with the invalid wrapper.
  - (c) `errors.New("nil context")` wrapped invalid, natsclient's shape (13 sites): better words, but it changes both
    methods and moves away from the nearest owner.
  - (d) A plain `errors.New`, as `graph/readiness` chose because it has no classified vocabulary. The guard already
    classifies its other refusals (`:51` fatal, `:114` transient).
- Options for the ended context:
  - (a) wrap invalid, chosen (Start's shape today and `metric.Server.start`'s);
  - (b) bare `ctx.Err()` in both (Stop's shape today; classified transient);
  - (c) `WrapTransient`, as `natsclient.Connect` does (`client.go:469-470`).
  - Nobody reads the class (inventory section 3), so this is a choice for consistency. Nothing depends on the old
    classes.
- Do nothing: the two spellings stay, and every adopter #196 lists copies them.

## D2. The cleanup is given once, to Start

- Chosen, (a): `Start(ctx, start, cleanup)` keeps its signature and records `cleanup` when it admits the Start;
  `Stop(ctx)` runs the recorded cleanup.
- Observable result:
  - a failed Start's rollback and every later Stop run the same function;
  - a refused Start (nil or ended context, or a second Start) records nothing and leaves the first Start's cleanup in
    place;
  - Stop before any Start runs nothing, returns nil and uses up the guard, as today;
  - the zero value of `Guard` still works (`component.go:446`).
- Invariant: whenever Stop runs a cleanup, an admitted Start recorded it. The guard becomes "used" only through an
  admitted Start, which records the cleanup, or through a Stop before Start, which also marks it finished. No state is
  pending with nothing recorded.
- Options:
  - (a) chosen. graph-ingest's `Start` line (`:967`) does not change. It mirrors how graph-ingest's `start` records the
    handles `release` reads (`component.go:451-454`).
  - (b) A constructor, `New(cleanup)`, or an exported `Cleanup` field. The zero value stops working. A `Guard` holds a
    mutex, so `New` must return a pointer and the field changes type. An exported field is "set once" only by
    convention. The cleanup would sit apart from the `start` it undoes.
  - (c) Keep two parameters and document "pass the same one". The defect stays, and nothing enforces the doc.
  - (d) `start` returns its own cleanup. A panicking `start` returns nothing, so the rollback would have nothing to run.
  - (e) Do nothing.
- Not added: a check that `start` or `cleanup` is non-nil. The one production caller passes method values, which cannot
  be nil. Today a nil cleanup panics in `Stop`; after this change the same mistake panics in the first `Stop` and is
  reported by the rollback helper (`lifecyclecleanup.go:48-49`). Nothing goes unnoticed.
- graph-ingest:
  - `component.go:967` is unchanged.
  - `:1073-1081` becomes `return c.lifecycle.Stop(ctx)`, and its doc comment (`:1069-1072`) says Stop runs `release`,
    the cleanup `Start` gave the guard.
  - The "component stopped gracefully" log moves from Stop's closure (`:1078`) to the end of `release`, after
    `clearLifecycleHandles()` (`:1094`). It prints once for each cleanup that succeeds, whether a Stop's or a failed
    Start's rollback.
  - A repeated `Stop` prints nothing: the guard runs no cleanup after one has succeeded
    (`TestGuardFailedStopLeavesCleanupForNextStop` `:177-180`; `CheckRepeatedStopIsNoOp` in
    `TestGraphIngestLifecycleSuite`). A Stop before Start, or a Stop whose release fails, prints nothing, as today.
  - New: a failed Start whose rollback succeeds now prints it. The alternative is to drop the log; no test or doc reads
    it (`git grep -n "stopped gracefully"`: `component.go:1078` only).
- Tests that pass a different cleanup today:
  - `test_owner_support_test.go:93-95` (`leaveGuardCleanupPending`) and `lifecycle_owner_test.go:136-138`: the cleanup
    returns the rollback failure on its first call (`errRollback`, or `component.release(expired)`) and runs
    `c.release(ctx)` with the context it is given after that. Their assertions and test names do not change, so the
    `lifecycle-suite` scenario (`spec.md:137-142`) and the ledger's `proving_tests` still hold.
  - `guard_test.go`: `owner.Stop` becomes `o.g.Stop(ctx)` (`:39-41`).
  - `TestGuardStopWaitsForAnInFlightStart` gives its counting cleanup to `Start`.
  - `TestGuardRollsBackAPanickingStart` asserts the recorded cleanup ran once in total.
  - `TestGuardStopBeforeStartRefusesLaterStart` calls `Stop(ctx)`.
- New test `TestGuardStopRunsTheCleanupStartWasGiven`:
  - `Start` with cleanup A is admitted; a second `Start` with cleanup B is refused with `ErrAlreadyStarted`.
  - `Stop` runs A once and B never; a repeated `Stop` runs nothing.
  - The oracle is a call counter per function.
  - It cannot compile on the base, because `Stop` takes a cleanup there. Its proof that it can fail is mutant M2.

## D3. The concurrent-Stop refusal is tested

- `TestGuardRefusesAConcurrentStop`, in `synctest` like `TestGuardStopWaitsForAnInFlightStart`:
  1. The guard is started with a cleanup that, on its first call, signals that it has started and blocks until the
     test releases it. Calls are counted.
  2. Stop A runs in a goroutine; the test waits for the signal.
  3. Stop B runs in a goroutine; after `synctest.Wait()`, B must already have returned an error with `errs.IsTransient`
     true, and the cleanup has run once.
  4. The test releases A, which returns nil. A third Stop returns nil, and the cleanup has still run once.
- Failure lines:
  - "Stop while another Stop runs cleanup = %v, want a transient refusal";
  - "second Stop did not return while the first ran cleanup".
- No sleeps, and no private state. `metric`'s equivalent test polls `server.stopping` and the mutex
  (`metric/handler_test.go:368-379`); this one uses the cleanup's own signal.
- It passes on the base: it pins behaviour that already exists. It is shown able to fail by mutant M3.
- Mutants for `task mutate:check`, each a copy of `internal/lifecycleguard/guard.go` kept outside the repository, each
  needing `verdict: detection`:
  - M1: in the helper, the ended branch returns the context's error bare. `-test TestGuardRefusesNilAndEndedContextsAlike
    -expect-text 'want an invalid refusal'`.
  - M2: the cleanup is recorded before the second-Start check. `-test TestGuardStopRunsTheCleanupStartWasGiven
    -expect-text "ran the refused Start's cleanup"`.
  - M3: the concurrent-Stop refusal returns nil. `-test TestGuardRefusesAConcurrentStop -expect-text 'want a
    transient refusal'`.
- D1's test `TestGuardRefusesNilAndEndedContextsAlike`:
  - Rows: Start against a fresh guard and Stop against a started guard, each with a nil, a cancelled and an expired
    context.
  - Each row requires `errs.IsInvalid`; `errors.Is(err, errs.ErrInvalidData)` for nil and `errors.Is(err, ctx.Err())`
    for an ended context; start and cleanup not run. After a refused Start, a live Start is admitted; after a refused
    Stop, a live Stop runs the cleanup once.
  - For each context, Start's and Stop's messages are equal apart from the method name.
  - On the base, the Start-nil row and both ended Stop rows fail, and the notes name them.

## D4. The suite still passes, unchanged

- `TestGuardOwnerPassesTheLifecycleSuite` (`guard_test.go:105-110`) and graph-ingest's `TestGraphIngestLifecycleSuite`
  (`lifecycle_integration_test.go:306`) keep their checks; only `owner.Stop`'s one line changes. The guard's behaviour
  stays as today's suite reads it:
  - one `Start`;
  - `Stop` safe before, during and after `Start`;
  - a failed cleanup pending for the next `Stop`;
  - rollback through `pkg/lifecyclecleanup`.
- Commands:
  - `go test -race -count=1 ./internal/lifecycleguard/ ./internal/harness/lifecycletest/ ./processor/graph-ingest/`
  - `task test:integration -- ./processor/graph-ingest/`
  - `task test:repeat -- ./internal/lifecycleguard/ ./processor/graph-ingest/`
  - `task verify` on a clean tree.

## D5. Doc comments

- The package comment keeps the three behaviours that `.agents/rules-index.md:28` cites: one Start, a Stop that is safe
  at any time, and a failed cleanup kept for the next Stop. It adds two things in plain words:
  - the owner gives its cleanup once, to `Start`, and a failed Start's rollback and every `Stop` run that one function;
  - `Start` and `Stop` refuse a nil or ended context the same way.
- `Start`'s comment adds that it records the cleanup and that a refused Start records nothing.
- `Stop`'s comment says it runs the cleanup `Start` recorded, and that it refuses a nil or ended context as `Start`
  does.
- `Component.Stop`'s and `release`'s comments say where the log is printed (D2).
- graph-ingest's `Start` comment (`component.go:951-956`) stays as it is; PR 2 rewrites it.

## D6. Spec, ledger and rules: none changed

- No spec states the guard's refusal classes or how the cleanup is passed (inventory section 3); `skip_specs: true`
  stands.
- Where each guard invariant is written:
  - lifecycle-suite "Portable floor", through the suite run;
  - "Failed start whose own cleanup fails";
  - the package doc for three behaviours D13 left there: the retry after a failed Stop, the concurrent-Stop refusal,
    and the one cleanup.
- A requirement now would sit on the same spec that PR 2 rewrites.
- Ledger: the guard has no row (own code), and graph-ingest's item (6) stays true.
- `AGENTS.md:82` and `rules-index.md:28` name the package, not its signature, so their text stands.

## Merge order against PR 2

- PR 1 merges first. PR 2 is unclaimed, so it branches from `main` with the one-argument `Stop`.
- PR 1 edits:
  - `guard.go`;
  - in `guard_test.go`, `:39-41`, the tests at `:183-268`, and the new tests;
  - in `component.go`, `:1069-1096`;
  - `test_owner_support_test.go:85-99`;
  - `lifecycle_owner_test.go:133-140`.
- PR 2 edits (ADR-113, Consequences; #282 point 4):
  - `lifecycletest.go:187-225` and the suite's test double;
  - `component/lifecycle.go:43-62`;
  - the `lifecycle-suite` spec;
  - `metric/handler.go`;
  - graph-ingest's `start` (`component.go:973-1033`) and its `Start` comment (`:951-956`);
  - the guard's test owner (`guard_test.go:19-99` apart from `:39-41`).
- No line is shared. If PR 2 is claimed before PR 1 merges, it merges `origin/main` afterwards and takes `owner.Stop`'s
  one line.

Owner question: keep the stop log in `release`, where it also prints after a failed `Start` whose rollback succeeds, or
drop it? (The question whether PR 2 retires #282 is settled: #282 was reopened on 2026-10-18 after #304's body closed it
by accident, and PR 2 retires it as the order on #282 says.)
