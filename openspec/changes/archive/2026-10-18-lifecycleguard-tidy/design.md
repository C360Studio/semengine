# Design: lifecycleguard-tidy

Depends on `INVENTORY PASS` for `inventory.md`; pins are at the base commit. Round 2: revised after `INVENTORY FAIL`
and `CHANGES REQUESTED` at `9c772d5`. Behaviour, not mechanism: the developer chooses locks and field layout, settled
by tests under `-race`.

## D1. One nil-context refusal; the ended context kept apart on purpose

Where the shape came from: #153 item 6 (a style-bar review of PR #93) named both spellings and ended with "Plain shape:
one refusal helper, and the cleanup given once"; #196's comment of 2026-10-16 and #282's point 5 carry those words.
Neither weighed how the tree's stop methods treat an ended context (inventory section 2). In this design the helper
covers the nil context only. Start and Stop answer an ended context differently, and that is deliberate.

- **Recommended, (b).**
  - One unexported helper (suggested name `nilContextError(method)`, mirroring natsclient's `nilContextError`,
    `natsclient/request.go:20-24`) returns `errs.WrapInvalid(errs.ErrInvalidData, "lifecycleguard", method, "nil
    context")`. `Start` and `Stop` each call it from their first check. `method` ("Start" or "Stop") only names the
    method in the message and switches no behaviour. Start changes one token (`ErrInvalidConfig` becomes
    `ErrInvalidData`) and its text; Stop's error is unchanged.
  - `Start` keeps refusing an ended context as invalid (`guard.go:45-46`). A Start given an ended context cannot begin.
    `errors.Is` on the context's error holds, which `CheckPreCancelledStartRefused` (`lifecycletest.go:157`, `:162`) and
    `TestComponent_RespectsContext_Timeout` (`component_test.go:937`) need, and which ADR-113's "Start returns that
    error" (`:60`) keeps for PR 2. `metric.Server.start` does the same (`metric/handler.go:73-74`).
  - `Stop` keeps returning an ended context's error bare (`guard.go:89-90`). That is the same error it returns when its
    bound runs out during its wait (`:108-109`) or inside graph-ingest's cleanup (`component.go:1136-1137`, `:1143`),
    and what every stop method in the tree returns (inventory section 2): `metric.Server.Stop`
    (`metric/handler.go:248-249`), `natsclient.Client.Close` (`client.go:688-689`, `:704-705`), `KeyedPool.Shutdown`
    (`keyed_pool.go:304-307`) and `CoalescingSet.Shutdown` (`coalescing_set.go:213-214`). The owner's ruling treats
    those cases as one event: "if `ctx` ends first it returns that error and a later `Stop` tries again" (#282 comment
    6249934379 point 2; ADR-113 `:61-62`; ADR-113 `:54-55` holds Stop and Shutdown to that one promise).
  - The ended-context handling is today's code. This change documents it (D5) and pins it with a test.
- **(a), the alternative.** One helper also refuses an ended context in both methods, wrapped as invalid. It costs
  three things:
  - graph-ingest's `Stop` would be classified invalid when its context ended before the call, but transient when the
    context ends during the wait or inside `release`. Those are two classes for the one event the ruling names.
  - The guard's `Stop` would be the only stop method in the tree that turns an ended context into a refusal class.
  - Nobody reads the class today (inventory section 3), so the cost falls on the contract's coherence, not on a caller.
- **(c)** `WrapTransient` in both: Start would leave `metric.Server.start`'s shape for no reader. **(d)** Do nothing: the
  nil context keeps two sentinels and two texts.
- Sentinel options for the nil context, with the tree's count (inventory section 2):
  - `ErrInvalidData`, chosen. The guard's own `Stop` already uses it, and so does `metric.Server` in both `start` and
    `Stop` (`metric/handler.go:71`, `:196`): 5 sites. Its class agrees with the wrapper, since `IsInvalid` lists it.
  - `ErrInvalidConfig`: one site, and its sentinel's own class (`IsFatal`, `pkg/errs/errs.go:186`) disagrees with the
    invalid wrapper.
  - `errors.New("nil context")` wrapped invalid, natsclient's shape (13 sites): better words, but it changes both
    methods and moves away from the nearest owner.
  - A plain `errors.New`, as `graph/readiness` chose because it has no classified vocabulary. The guard already
    classifies its other refusals (`:51` fatal, `:114` transient).
- Test `TestGuardRefusesNilAndEndedContexts`. Rows use `t.Errorf`, so every failing row prints its line.
  - `Start(nil)` on a fresh guard: an error with `errs.IsInvalid` and `errors.Is(err, errs.ErrInvalidData)`; `start`
    not run; a following live Start is admitted. Failure line: "Start(nil) = %v, want an invalid refusal matching
    errs.ErrInvalidData".
  - `Stop(nil)` on a started guard: the same checks; the cleanup is not run, and a following live Stop runs it once.
    Failure line: "Stop(nil) = %v, want an invalid refusal matching errs.ErrInvalidData".
  - The two nil messages are equal apart from the method name. Failure line: "Start(nil) and Stop(nil) messages %q and
    %q differ beyond the method name".
  - `Start` with a cancelled and an expired context, on a fresh guard: `errs.IsInvalid`, `errors.Is(err, ctx.Err())`,
    `start` not run, guard still unused. Failure line: "Start(%s) = %v, want an invalid refusal matching %v".
  - `Stop` with a cancelled and an expired context, on a started guard: `errors.Is(err, ctx.Err())` and
    `errs.IsTransient`, the class of the context's own error; cleanup not run; a following live Stop runs it once.
    Failure line: "Stop(%s) = %v, want the context's own error %v".
  - On the base, the `Start(nil)` row fails (it matches `ErrInvalidConfig`), and so does the nil message check
    ("context cannot be nil" against "nil context", and the sentinels' texts). The ended rows pass on the base; their
    sensitivity is M4 (D4). Under option (a), the ended `Stop` rows would assert `errs.IsInvalid` instead, and an
    ended-message check would also fail on the base.

## D2. The cleanup is given once, to Start

- Chosen, (a): `Start(ctx, start, cleanup)` keeps its signature and records `cleanup` when it admits the Start;
  `Stop(ctx)` runs the recorded cleanup. An admitted Start is one that passed the nil-context, ended-context and
  second-Start refusals.
- Observable result:
  - a failed Start's rollback and every later Stop run the same function;
  - a refused Start records nothing and leaves the first Start's cleanup in place;
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
  be nil. Today a nil cleanup panics in `Stop` and then leaves every later `Stop` refused as concurrent (inventory 1e,
  filed as #309). After this change the same mistake panics in the first `Stop` with the same result, and a failed
  Start reports a nil rollback through the rollback helper (`lifecyclecleanup.go:48-49`). Nothing goes unnoticed.
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
  - New: a failed Start whose rollback succeeds now prints it.
  - Log options:
    - in `release`, chosen;
    - drop the log;
    - log in `Component.Stop` after the guard's `Stop` returns nil, rejected: that prints on every no-op `Stop`, before
      Start or repeated, because the guard returns nil there too.
    - `git grep -n "stopped gracefully"` finds `component.go:1078` and, as history rather than a reader,
      `openspec/changes/archive/2026-10-12-setup-04a-02-ingest-kernel/notes/4.4.md:205`. No test or doc reads it.
- Tests that pass a different cleanup today:
  - `test_owner_support_test.go:93-95` (`leaveGuardCleanupPending`) and `lifecycle_owner_test.go:136-138`: the cleanup
    returns the rollback failure on its first call (`errRollback`, or `component.release(expired)`) and runs
    `c.release(ctx)` with the context it is given after that. Their assertions and test names do not change, so the
    `lifecycle-suite` scenario (`spec.md:137-142`) and the ledger's `proving_tests` still hold.
  - `guard_test.go`: `owner.Stop` becomes `o.g.Stop(ctx)` (`:39-41`).
  - `TestGuardStopWaitsForAnInFlightStart` gives its counting cleanup to `Start`.
  - `TestGuardRollsBackAPanickingStart` asserts the recorded cleanup ran once in total.
  - `TestGuardStopBeforeStartRefusesLaterStart` calls `Stop(ctx)`.
- Test `TestGuardStopRunsTheCleanupStartWasGiven`:
  - `Start` with cleanup A is admitted. A second `Start` with cleanup B is refused. Failure line: "second Start = %v,
    want ErrAlreadyStarted".
  - `Stop` returns nil. B's count is checked first, and both counts use `t.Errorf`, so B's line prints whatever else
    fails. A `t.Fatalf` on A's line first would end a mutant run with no expected line, which `mutate:check` reads as
    inconclusive (`internal/harness/mutcheck/classify.go:196-197`). Failure lines: "Stop ran the refused Start's
    cleanup %d time(s), want 0"; "Stop ran the admitted Start's cleanup %d time(s), want 1".
  - A repeated `Stop` returns nil and runs nothing. Failure line: "repeated Stop = %v with calls A %d, B %d, want nil,
    1, 0".
  - It cannot compile on the base, because `Stop` takes a cleanup there. Its proof that it can fail is M2 (D4).

## D3. The concurrent-Stop refusal is tested

- `TestGuardRefusesAConcurrentStop`, in `synctest` like `TestGuardStopWaitsForAnInFlightStart`:
  1. A cleanup, on its first call, signals that it has started and blocks until the test releases it. Calls are counted.
  2. Stop A runs in a goroutine; the test waits for the signal.
  3. Stop B runs in a goroutine; after `synctest.Wait()`, B must already have returned an error with `errs.IsTransient`
     true, and the cleanup has run once.
  4. The test releases A, which returns nil. A third Stop returns nil, and the cleanup has still run once.
- Failure lines: "Stop while another Stop runs cleanup = %v, want a transient refusal"; "second Stop did not return
  while the first ran cleanup"; "cleanup ran %d time(s), want 1".
- On the base the test calls the two-argument `Stop`, and the blocking cleanup is handed to Stop A, since there the
  cleanup given to `Start` runs only in a rollback. Task 2.3 rewrites it so the cleanup is given to `Start`.
- No sleeps, and no private state. `metric`'s equivalent test polls `server.stopping` and the mutex
  (`metric/handler_test.go:368-379`); this one uses the cleanup's own signal.
- It passes on the base: it pins behaviour that already exists. Its proof that it can fail is M3 (D4).

## D4. The suite still passes, unchanged, and every new test can fail

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
- Mutants for `task mutate:check -- -pkg ./internal/lifecycleguard -file internal/lifecycleguard/guard.go -mutant <copy>
  -test <test> -expect-text <text>`. Each copy is kept outside the repository, and each needs `verdict: detection`:

| Mutant | Wrong change to `guard.go` | `-test` | `-expect-text` |
| --- | --- | --- | --- |
| M1 | the nil-context helper wraps `errs.ErrInvalidConfig` | `TestGuardRefusesNilAndEndedContexts` | `want an invalid refusal matching errs.ErrInvalidData` |
| M2 | the cleanup is recorded before the second-Start check | `TestGuardStopRunsTheCleanupStartWasGiven` | `ran the refused Start's cleanup` |
| M3 | the concurrent-Stop refusal returns nil | `TestGuardRefusesAConcurrentStop` | `want a transient refusal` |
| M4 | Stop's ended-context check wraps the error invalid (option D1 (a)) | `TestGuardRefusesNilAndEndedContexts` | `want the context's own error` |

## D5. Doc comments

- The package comment keeps the three behaviours that `.agents/rules-index.md:28` cites: one Start, a Stop that is safe
  at any time, and a failed cleanup kept for the next Stop. It adds, in plain words:
  - the owner gives its cleanup once, to `Start`, and a failed Start's rollback and every `Stop` run that one function;
  - `Start` and `Stop` refuse a nil context the same way;
  - `Start` refuses an ended context, while `Stop` returns the context's own error, as it does whenever its context ends
    first;
- `Start`'s comment adds that it records the cleanup and that a refused Start records nothing.
- `Stop`'s comment says it runs the cleanup `Start` recorded, what it returns for a nil and an ended context, and that
  a cleanup that panics leaves every later `Stop` refused (after a `Stop`) or waiting (after a rollback): the open
  defect #309, which no caller triggers today.
- `Component.Stop`'s and `release`'s comments say where the log is printed (D2).
- graph-ingest's `Start` comment (`component.go:951-956`) stays as it is; PR 2 rewrites it.

## D6. Spec, ledger and rules: none changed

- No spec states the guard's refusal classes or how the cleanup is passed (inventory section 3); `skip_specs: true`
  stands.
- Where each guard invariant is written:
  - lifecycle-suite "Portable floor", through the suite run;
  - "Failed start whose own cleanup fails";
  - the package and method docs, for what no spec states:
    - the retry after a failed Stop, which D13 left there (archive `design.md:757`, "except that retry; no delta");
    - the concurrent-Stop refusal, stated today only in Stop's doc (`guard.go:83-84`);
    - this change's one cleanup (D2), and what a panicking cleanup leaves behind, as #309 describes (D7).
- A requirement now would sit on the same spec that PR 2 rewrites.
- Ledger: the guard has no row (own code), and graph-ingest's item (6) stays true.
- `AGENTS.md:82` and `rules-index.md:28` name the package, not its signature, so their text stands.

## D7. A panicking cleanup: filed as #309, not fixed here (inventory 1e)

- No real-world trigger today: graph-ingest's `release` has no panic path, every cleanup in the tree is a method value,
  and no caller recovers a panic (inventory 1e). Under the severity rule (#185) that makes it MEDIUM at most, and the
  reviewer contract's answer to such a finding is a doc sentence or a declared degrade, never new mechanism to make a
  promise true. So it is filed as #309 (`status:blocked` on this PR, which edits the same lines), and `Stop`'s doc
  comment states the behaviour and cites it (D5).
- Options:
  - (a) fix both paths here, mirroring the panicking-`start` handling (`guard.go:57-75`): the cleanup stays pending,
    the panic continues to the caller, a later `Stop` runs it again; on the rollback path `Start`'s bookkeeping still
    finishes. Two tests (`TestGuardStopSurvivesAPanickingCleanup`; `TestGuardStartSurvivesAPanickingRollback`, in
    `synctest`) and two mutants. Rejected here under #185; the shape is recorded on #309 for whoever fixes it.
  - (b) the `Stop` path only, with the rollback path filed: the same defect split in two, rejected for the same reason.
  - (c) file both and state the behaviour, chosen.
- The owner can pull (a) into this change by saying so on #308; its tests and mutants then join tasks 2.1 to 2.3.

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
  one line. D1 (b) leaves `Stop`'s ended-context error as PR 2's new checks will read it: the context's own error.

Owner questions:

1. Keep the stop log in `release`, where it also prints after a failed `Start` whose rollback succeeds, or drop it?
   (Logging in `Component.Stop` is rejected in D2.)
2. For an ended context, take (b), which is recommended: `Start` refuses it as invalid and `Stop` returns the context's
   own error, as every stop method does? Or take (a): one helper refuses it as invalid in both, at the cost of two
   classes for one event in `Stop`?

Owner answers (2026-10-18, PR #308 comment): (1) keep the log in `release`; (2) option (b).

(The question whether PR 2 retires #282 is settled: #282 was reopened on 2026-10-18 after #304's body closed it by
accident, and PR 2 retires it as the order on #282 says.)
