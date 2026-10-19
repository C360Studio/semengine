# Design: start-context

ADR-113 decides this (`docs/adr/113-start-context-bounds-start.md`; #282 comments 6249934379 and 6250135029). This
design adds no decision of its own. It states each observable behaviour and the test that proves it. Line numbers are
at the base commit (`inventory.md`).

## D1. Check `StartContextEndStopsNothing`

Replaces `CheckControlledStopUnderLiveStartAuthority`, in the same slot of `Run`. Steps:

1. Derive `startCtx` from the check's context. `Start(startCtx)` must return nil.
2. Take `before := o.Observe()`, then cancel `startCtx`.
3. Wait on a timer for the settle window (D3), then read `Observe` once. Fail if any item in `before.Unresolved` is
   no longer listed, and name the missing items ("after the Start context ended the owner stopped holding …").
4. `finish` (Stop under fresh finite authority) must return nil, and `Observe` must then list nothing.

Only removals fail; an owner may add work while it runs. Proof: failpoint `workDiesWithStartContext` (D4) trips this
check. `stopReturnsNilWithWorkerRunning` stays mapped here and fails at step 4.

## D2. Check `StopReturnsItsEndedContext`

Replaces `CheckAbortStopPreservesCause`.

1. Start under a live context; its cancel runs only when the check returns.
2. Call Stop with a context already cancelled with a cause. It must return promptly, with an error matching that
   context's error or cause. A nil return, or a return past the bound, fails as today.

The check does not cancel the Start context first.

For the double, `TestAbortStopThenFinishJoinsWorker` (D4) proves "a later Stop tries again": after that Stop, a later
Stop joins the worker and leaves nothing held. For real owners, `Run`'s terminal Stop checks only the returned error
(`lifecycletest.go:120-122`, `:128-132`). The suite does not check that a later Stop leaves nothing held; each owner's
own tests carry that.

Proof: `abortStopDropsCause` and `stopIgnoresCallerDeadline` trip this check (rows unchanged, check name updated).

## D3. The settle window

The window is 100 ms, a named constant beside `grace`. Step 3 of D1 blocks on its timer and then reads `Observe` once.

- Inside the failpoint matrix (synctest) the result is exact: the fake clock fires the timer only once the dying
  worker has exited.
- For a real owner outside a bubble, no explicit synchronization exists for a black-box owner. The window only lets
  goroutines that are already runnable run, so detection is best-effort.
- Why it cannot fail a correct owner today: it does not, for any `Observe` item that lasts until Stop. Some items end
  on their own: metric's `admitted request` (`metric/lifecycle_test.go:50-52`) and natsclient's `message handler` and
  `connection-loss timer armed` (`natsclient/client_lifecycle_test.go:73-81`). Those items appear only under traffic
  or connection loss, and `Run` drives neither.
- Cost: about 0.1 s per `Run`, about 2.5 s across `test:unit`, `test:repeat` and `test:integration`.

`docs/testing.md:573` does not allow this wait: no interval defines "ending ctx stops nothing". The window is a new
exception to the test rules, so it is owner question 2.

Rejected alternatives:

- Reading `Observe` once with no wait. That is insensitive even in the matrix.
- A new `Observation` field. That is new mechanism, and it shows the suite nothing an adapter that reports whether
  work is running does not.

## D4. The sensitivity double (`refowner_test.go`)

- New failpoint `workDiesWithStartContext`; table row
  `{workDiesWithStartContext, "StartContextEndStopsNothing", Promise{}}`. The double builds its run context from
  Start's context (today's `:110`) only under this failpoint.
- The clean double adopts the contract: its worker survives the Start context ending. Any construction of the run
  context passes; the simplest Go is one line.
- The worker still selects on `runCtx.Done()` and `stopCh`. `o.cancel` is the run context's cancel.
- `finalize` and `finalizeVerdict` need no code change. The cancel they call (`:264`, `:271-273`) is the double's own
  run-context cancel, so an unsignalled worker is still ended and joined. Only their comments change ("through its
  run context").
- `startFailsButHolds` (`:84-92`): comment only; `finalize` ends it the same way.
- Existing failpoints: `abortStopDropsCause` now means "Stop given an ended context returns nil" (before: "after Start
  authority was cancelled"); the code path is unchanged (`:150-155`). `stopIgnoresCallerDeadline`: unchanged code; it
  trips D2.
- `TestAbortStopThenFinishJoinsWorker` (`:453-490`): the abort Stop no longer signals the clean double's worker.
  1. Start `finish` in a goroutine.
  2. Select on `o.workerExit.Entered()` against `finished`. If `finished` wins while `Observe` still lists `worker`,
     end the worker first (the double's cancel, then join, as `finalizeVerdict` does at `:265-266`) and then fail:
     a #40-shaped regression (an ended-context Stop that marks the owner stopped without signalling or joining) is
     reported as a failure, not as a bubble deadlock.
  3. Then `synctest.Wait` and the rest, unchanged.

  The exclusion at `:456` names the new check.

Proving tests: `TestEachFailpointTripsExactlyItsCheck`, `TestChecksPassAgainstCleanDouble`,
`TestFailpointTableComplete`, `TestRunOverCleanDouble`, `TestAbortStopThenFinishJoinsWorker`,
`TestFinalizeReportsWorkerHeldAfterSignal`.

## D5. `metric.Server`: stop handing the Start context to requests

Option (a) is the reading ADR-113 gives: its Decision is stated as behaviour (`:59`); no constructor is named
(`:71-75`); the run-context sentence (`:76-78`) bounds the exception to the root ban and is not a duty to create a
context for work that reads none.

Measured: no handler reads the request context (inventory).

The change: delete `BaseContext` (`handler.go:145`). Requests are then served under `net/http`'s documented default
base context. Stop's `Shutdown` and `Close` already end serving. No cancel is kept, and no context is captured in a
closure. Option (b), a run context with a private cancel, was not taken: it would add a field and a closure holding a
context that nothing reads.

The adapter (`metric/lifecycle_test.go:47-49`) lists "serve goroutine" only while `s.serveDone != nil &&
len(s.serveDone) == 0`. `Serve`'s goroutine sends its one result before it closes the channel, and Stop clears the
field only after it has received that result. This follows the "Observe adapter contract" (goroutines are listed
while they run), as D6 does for the status loop.

Behaviour: ending the Start context after Start returned stops nothing, and requests keep being served.

Proofs:

- `TestServerServesAfterStartContextEnds`: `Start(startCtx)`, cancel, GET `/metrics` returns 200, Stop returns nil,
  nothing is held. It passes before and after the change.
- `TestServerLifecycleSuite/StartContextEndStopsNothing`. With the adapter change it detects M5.

Test edit: delete `metric/handler_test.go:167` (`require.Same(t, t.Context(), server.server.BaseContext(…))`, which
pins the old rule). The new test covers its behaviour.

Docs:

- `handler.go:54-55` becomes "ctx bounds Start only; ending it after Start returned stops nothing. Requests are
  served until Stop."
- `metric/README.md:209` becomes "// One-shot synchronous bind; ctx bounds Start only".
- `runtimeCtx` becomes `ctx` at `metric/README.md:71-72`, `:554-555`, `:565`, `:763-764` and `metric/doc.go:373-374`.
- `metric/README.md:490`: the example's `go g.processingLoop(ctx)` runs under a run context the example's `Start`
  creates and its `Stop()` cancels first (a `cancel context.CancelFunc` field on the example struct). Issue 3 covers
  the rest of that example.

## D6. graph-ingest `start`: startup under the Start context, the rest under run

Sketch (`component.go:973-1033`):

```go
runCtx, cancel := c.newRunContext()                   // the one run context; release ends it
poolCtx, poolCancel := context.WithCancel(runCtx)      // unchanged
submitCtx, submitCancel := context.WithCancel(runCtx)  // unchanged
Connect(ctx); WaitForConnection(ctx)                   // :982-990; error checks read ctx.Err()
initStorage(ctx, runCtx)                               // buckets under ctx; the entity cache's expiry sweep under runCtx
startEntityStateGuard(ctx, …); createStatusBucket(ctx)
buildIngestPool(poolCtx); setupSubscriptions(ctx, submitCtx)   // setup under ctx; closures submit on submitCtx
setupQueryHandlers(runCtx); setupMutationHandlers(runCtx)      // the context parents handlers (request.go:322-329)
startStatusMetricsLoop(runCtx)
```

`c.newRunContext()` stands for whatever one line builds the run context; the contract names no constructor.

- `release` and `cleanup` are unchanged, keeping the cancel order (`:1099-1143`): drain consumers, submit cancel, pool
  `Shutdown`, pool cancel, subscription drains, run cancel, status join, cache `Close`.
- A failed Start: the guard's rollback runs `release`, which ends the run context.
- Adapter (`lifecycle_adapter_test.go:89`): list "status loop" while `statusDone` is set and not closed, that is,
  while the goroutine runs, as the "Observe adapter contract" says of goroutines. `StartedOwnerReportsItsHandles` is
  unchanged.

Proofs:

- (T-G1) `TestGraphIngestLifecycleSuite/StartContextEndStopsNothing`. Fails first on today's code: the status loop
  exits.
- (T-G2) `TestComponent_StartContextEndStopsNothing` replaces `TestComponent_RespectsContext_Cancellation`
  (`component_integration_test.go:131-158`, which asserts the old rule). `Start(startCtx)`; cancel; publish one entity
  on the input stream; `probe.Await` (bounded by the test context) until `ENTITY_STATES` holds it; one entity query
  answers without error; Stop returns nil; `statusDone` is closed. Fails first: lanes negatively acknowledge every
  input (`keyed_ingest.go:219-226`).
- (T-G3) `TestGraphIngest_StartBoundedByItsContext`. No input stream exists; Start under a 1 s deadline returns an
  error matching `context.DeadlineExceeded` well inside a 10 s failure bound, and holds nothing. This guards the setup
  half (`waitForStream` retries about 50 s under any other context).
- Existing: `TestGraphIngest_FailedBootSweepFailsStart` (`cancelDuringSweep`, `lifecycle_integration_test.go:184-240`)
  holds the sweep under the Start context.

After `setupMutationHandlers` (`:1017-1019`) and before `startStatusMetricsLoop`, `start` adds one check, the same
shape as `:983-985`:

```go
if err := ctx.Err(); err != nil {
    return errs.Wrap(err, "Component", "Start", "context ended during startup")
}
```

A Start context that ends while a run-context step runs (the request subscriptions' PONG wait) then fails Start; the
guard's rollback runs `release`, which ends the run context. So "if ctx ends first, Start returns that error and holds
nothing" holds. The guard itself commits without reading the context (`internal/lifecycleguard/guard.go:86-90`), so
`start` is the only place this can be seen.

- (T-G4) `TestGraphIngest_StartFailsWhenItsContextEndsDuringHandlerSetup` (integration). Through the existing
  `subscribeRequests` field (`component.go:425-427`, waived under #301), the test's subscribe function cancels
  `startCtx` and then delegates to the client. Start must return an error matching `context.Canceled`; the adapter
  must list nothing; Stop must return nil. This adds one test reader of a #301 field, recorded on #301.

Also reword `processor/graph-ingest/test_owner_support_test.go:38`: "The fixture keeps its Start context live until
Stop; ADR-113 no longer needs this, and it does no harm."

Comments:

- `:951-956`, Start, gains: "ctx bounds Start only. Connecting, bucket setup, the ENTITY_STATES sweep and consumer
  setup run under it, and a ctx that ends first fails Start. Subscriptions, request handlers, the ingest pool, the
  entity cache's expiry sweep and the status loop run under a run context start creates and release ends, so ending
  ctx after Start returned stops nothing (ADR-113)."
- `:547-548` becomes "children of the run context Start creates".
- `keyed_ingest.go:161` becomes "The pool context, a child of the run context Start creates, …"; `:204` "the pool
  context"; `:220-221` "refused only once the lane context has ended, at Stop, which ends it after the pool's Shutdown
  returns or runs out of time."

Tests that relied on the Start context ending graph-ingest's work: T-G2 replaces the only one found. `task verify`'s
integration run is the gate for any other.

## D7. The guard's test owner and natsclient: no code change

`guard_test.go:52-55` waits only on `stop` and is expected to pass D1 as written. natsclient's `Connect` context is
dial-scoped (ADR-112), with the poller and handlers under client-owned roots. Right after `Connect` its `Observe`
lists only the long-lived kinds (health monitor, metrics poller), so D1's removal test cannot trip falsely. The
developer confirms both by the first `task test:unit` (`TestGuardOwnerPassesTheLifecycleSuite`,
`TestClientLifecycleSuite`) and records the run in `notes/3.3.md`.

## D8. The `LifecycleComponent` comment (`component/lifecycle.go:49-53`)

Replaced by:

```go
//   - Start(ctx) does its startup work under ctx and returns once the
//     component is ready; if ctx ends first, Start returns that error and
//     holds nothing. Ending ctx after Start returned stops nothing: the
//     component runs its continuing work under a context of its own, which
//     Stop ends (ADR-113).
//   - Stop(ctx) is the only stop. It drains, joins and cleans up within ctx's
//     bound and returns nil only once everything is released; if ctx ends
//     first, it returns that context's error and a later Stop tries again.
```

## D9. Rule texts (exact replacements)

- Developer contract `:215-218`: "Pass context as the first argument, and pass the exact received or derived context
  directly into goroutines, callbacks, and helpers. A lifecycle owner's `Start` does its startup work under the
  context it is given; ending that context after `Start` returns stops nothing. For its continuing work `Start`
  creates one run context, keeps only a private `context.CancelFunc` (synchronized to match the start/stop contract),
  and ends it inside `Stop`; every spawned task joins `Stop` (ADR-113). A `Run` owner's work derives from `Run`'s
  context."
- Developer contract `:219-221`: "Create production root contexts only at the process composition boundary, with one
  exception: a lifecycle owner's `Start` creates the one run context for its continuing work, keeps it only as a
  cancel, and ends it inside `Stop` (ADR-113). Otherwise constructors, factories, callbacks, watchers, and goroutines
  must not invent roots with `context.Background`, `context.TODO`, or `context.WithoutCancel`." The rest of the bullet
  is unchanged.
- Developer contract `:232`: "Do not use `context.WithoutCancel(parent)` directly or create an unbounded descendant,
  except as that one run context."
- Developer contract `:235-237`: "`Stop(ctx)` is the only stop: it returns nil only once everything is released, and
  if `ctx` ends first it returns that context's error and a later `Stop` tries again. The callee must not replace
  Stop's context with its own timeout. Timeout or cancellation does not establish completed callback or worker
  joins."
- Reviewer contract `:237-241`, the whole bullet: "Require context as the first argument; verify the exact received
  or derived context passes directly into goroutines, callbacks, and helpers. A lifecycle owner may retain only a
  private `context.CancelFunc`, with synchronization proven against its start/stop contract; it may not retain the
  context itself. Verify startup work runs under the context given to `Start`, continuing work under the one run
  context `Start` creates and `Stop` ends, and every task joins `Stop` (ADR-113); continuing work that ends when the
  Start context ends is `BLOCKING`. A `Run` owner's work derives from `Run`."
- Reviewer contract `:242`: "Root creation outside the process composition boundary is `BLOCKING`, except a lifecycle
  owner's one run context: created in `Start`, kept only as a cancel, ended inside `Stop` (ADR-113)."
- Reviewer contract `:255`: "Direct use or any unbounded descendant of `context.WithoutCancel` is `BLOCKING`, except
  as that one run context."
- Architect contract `:401-404`: "Contexts enter operations as the first argument. A lifecycle owner's `Start` does
  startup work under the context it is given and creates the one run context for its continuing work, which `Stop`
  ends (ADR-113); a `Run` owner's work derives from `Run`. The design passes the exact received or derived context
  directly to goroutines, callbacks, and helpers, and every spawned task joins `Stop`."
- Architect contract `:405`: "**Root creation stays at composition**, except a lifecycle owner's one run context,
  created in `Start`, kept only as a cancel, and ended in `Stop` (ADR-113)."
- `AGENTS.md:71`: "Production structs never retain `context.Context`; never invent roots (except a `Start`'s one run
  context, ended in `Stop`, ADR-113) or nil defaults". Adds 63 bytes; `AGENTS.md` becomes 21,997 of 24,000; row counts
  unchanged.
- `.agents/rules-index.md:17`: rule column as above; where column adds "ADR-113"; enforcement column becomes
  "`TestNoRetainedContext` (`task test:unit`) for struct fields; invented roots and nil defaults are review only; the
  exception's bound (continuing work outlives the Start context) is checked by `lifecycletest`'s
  `StartContextEndStopsNothing` for owners whose tests run the suite".
- `docs/setup-plan.md:291` adds ", except a lifecycle owner's one run context: created in `Start`, kept only as a
  cancel, ended in `Stop` (ADR-113)". `:293-294` becomes "The context given to `Start` bounds only `Start`; ending it
  later stops nothing. `Stop(ctx)` is the only stop: it uses the caller's exact finite context, returns nil only once
  everything is released, and returns that context's error if it ends first, so a later `Stop` tries again (ADR-113).
  The callee must not replace Stop authority with its own timeout." The rest is unchanged.
- `docs/admission-ledger.yaml:176-177`: "portable lifecycle floor semantics (ending the Start context after Start
  stops nothing and Stop then releases everything; a Stop whose context has ended reports it; repeated Stop no-op;
  nil and pre-cancelled contexts refused; Stop before Start safe; ADR-113 replaces the pin's controlled/abort pair)".
- `pkg/errs/shutdown.go:17`: "// PhaseJoinRuntime identifies waiting for an owner's continuing work to finish."
- `docs/adr/108-processor-shell.md`: a dated note under Status, the pattern ADR-066 `:22` uses: "**Decision 3's
  'controlled and abort Stop' superseded by ADR-113 (2026-10-18).** The context given to `Start` bounds only `Start`;
  `Stop(ctx)` is the only stop, and a `Stop` whose context ends first returns that error so a later `Stop` tries
  again."

## D10. The spec delta

`specs/lifecycle-suite/spec.md` in this change (replacing the skeleton's `skip_specs: true`) modifies "Portable
floor" (the two changed checks and the settle window, with a new scenario "Owner whose work ends with its Start
context") and "Complete sensitivity matrix" (an unsignalled worker is ended by cancelling the double's own run
context).

## D11. Declared limits (#185: MEDIUM at most, no new mechanism)

- Request subscriptions: graph-ingest's request subscriptions take the run context, because natsclient parents their
  handlers on it (`natsclient/request.go:322-329`). A Start context that ends during their setup does not cut it
  short. natsclient caps the PONG wait at `DefaultRequestTimeout`, 5 s per subscription, but a held connection write
  before that wait can last a minute or more (`request.go:294-300`; ADR-112, probe P17). D6's check, and the joined
  error on a failed registration (#315), then fail Start with its context's error, so the cost is a late return,
  not a broken contract. Issue 1 tracks the natsclient
  shape.
- Entity cache: no test can see which context builds it. Its expiry sweep has no observable outside `internal/cache`
  without a hook. `Get` checks expiry itself (`cache.go:133`), and Hybrid's LRU bounds size. The PR records this as an
  uncovered case.

## D12. #301 (the four test-only fields)

This change does not remove the reasons #301 gives, which are untouched by moving work between contexts:
`initStorage` still replaces the entity bucket before the sweep, so `watchEntityStates` is still needed, and
`*natsclient.Client` still has no seam for `subscribeRequests` and `consumeStream`. The suite adapter itself uses
`consumeStream` (`lifecycle_adapter_test.go:36-48`). #301 stays open, with this answer posted there. This PR adds no
field.

## Generated checks: examples suffice

This change reorders a short, fixed history: Start returns, the Start context ends, Stop runs with an ended context, and
a later Stop runs. Every interleaving that matters is named and forced, not sampled. The failpoint matrix runs each
check against one defect at a time inside a synctest bubble, where the fake clock settles every goroutine before an
assertion, so each ordering is deterministic. `TestAbortStopThenFinishJoinsWorker` forces the one race the history has
(a Stop returning before its worker exits) with an explicit probe instead of the scheduler. There is no input space to
generate over: the owner takes no data, only the four calls in that order. The adopters' interacting input is which
startup step the Start context ends in. That is enumerated at the three places the step's context changes: the boot
sweep (`cancelDuringSweep`), consumer setup (T-G3) and handler setup (T-G4); every other startup step (connect, buckets,
the status bucket) shares the same context-error path. The history after Start (cancel, ingest, query, Stop) is fixed
and is T-G2. So no generator is added (`docs/testing.md`, "Decide whether generated checks are needed").

## Mutants

`task mutate:check`; the integration ones with `GOFLAGS=-tags=integration`, or by hand per `docs/testing.md:230`
while #129 is open.

- M1: D1 without the `startCtx` cancel. Detected by `TestEachFailpointTripsExactlyItsCheck/workDiesWithStartContext`
  ("did not detect").
- M2: D1 with the settle-window comparison removed. Same detector.
- M3: the clean double builds its run context from `ctx`. Detected by `TestChecksPassAgainstCleanDouble`.
- M4: D2 accepting a nil Stop. Detected by `TestEachFailpointTripsExactlyItsCheck/abortStopDropsCause`.
- M5: metric's Start closes the server when `ctx` ends (`go func() { <-ctx.Done(); httpServer.Close() }()`).
  Detected by `TestServerServesAfterStartContextEnds` (the GET fails) and
  `TestServerLifecycleSuite/StartContextEndStopsNothing` (with the adapter's liveness reading, D5).
- M6: graph-ingest's `runCtx` from `ctx` (today's `:974`). Detected by
  `TestGraphIngestLifecycleSuite/StartContextEndStopsNothing` and `TestComponent_StartContextEndStopsNothing`.
- M7: graph-ingest's `setupSubscriptions(runCtx, …)`. Detected by `TestGraphIngest_StartBoundedByItsContext`.
- M9: D6's `ctx.Err()` check deleted. Detected by T-G4.

The adapter's liveness reading (D6) is test code, so it is not a mutant: T-G1's note records that it fails first on
today's code (the status loop exits), shown with that reading.

## Merge order against PR 3 (#291)

No PR exists for #291. This PR merges first. PR 3 branches from `main` afterwards and edits the connect block
(`component.go:981-994`), which this PR only re-points from `runCtx` to `ctx`. #291's plan note already assumes that
split ("waits under the Start context"), so the two do not collide. Change 3 needs nothing from these adopters. Open
point for change 3 only: the manager hands each `Start` a startup-bounded context and keeps no per-component cancel
(ADR-113).

## Owner questions

1. Confirmation only: metric takes option (a) as ADR-113's reading (D5 cites).
2. The settle window (D3): accept a no-false-failure observation window and amend `docs/testing.md:573` to name it,
   or drop it (D1 keeps steps 1, 2 and 4; the `workDiesWithStartContext` failpoint and its scenario go).

Owner answers (2026-10-18, Enjoy conversation; recorded on PR #310): (1) confirmed, option (a); (2) keep the 100 ms
observation window and name the exception in `docs/testing.md`. Task 4.2 adds to `docs/testing.md:573`'s list of
allowed real-clock intervals: "an observation window whose expiry cannot fail a correct implementation, where no
signal exists (the lifecycle suite's Start-context check, `internal/harness/lifecycletest`)".

The #282 point 4 premise correction (inventory, "`lifecycletest.Run` callers") is a record, not a question.
