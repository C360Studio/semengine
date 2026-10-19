# Design: start-context

ADR-113 decides this (`docs/adr/113-start-context-bounds-start.md`; #282 comments 6249934379 and 6250135029). This
design adds no decision of its own. It states each observable behaviour and the test that proves it. Line numbers are
at the base commit (`inventory.md`).

## D1. Check `StartContextEndStopsNothing`

Replaces `CheckControlledStopUnderLiveStartAuthority`, in the same slot of `Run`. Steps:

1. Derive `startCtx` from the check's context; `Start(startCtx)` must return nil.
2. `before := o.Observe()`; cancel `startCtx`.
3. Re-read `Observe` until the settle window (D3) has passed. Fail if any item in `before.Unresolved` is no longer
   listed, naming the missing items ("after the Start context ended the owner stopped holding …").
4. `finish` (Stop under fresh finite authority) must return nil, and `Observe` must then list nothing.

Only removals fail: an owner may add work while it runs. Proof: failpoint `workDiesWithStartContext` (D4) trips it.
`stopReturnsNilWithWorkerRunning` stays mapped here (step 4).

## D2. Check `StopReturnsItsEndedContext`

Replaces `CheckAbortStopPreservesCause`. Start under a live context (deferred cancel only). Stop with a context
already cancelled with a cause must return promptly, with an error matching that context's error or cause. A nil
return, or a return past the bound, fails as today. The Start context is not cancelled first. "A later Stop tries
again" is held by Run's terminal `finish` and by `TestAbortStopThenFinishJoinsWorker` (D4); adding it to this check
would make `stopReturnsNilWithWorkerRunning` trip two checks. Proof: `abortStopDropsCause` and
`stopIgnoresCallerDeadline` (unchanged rows, renamed target).

## D3. The settle window

This change's one real-clock interval, as `docs/testing.md:573` allows: 100 ms, a named constant beside `grace`,
waited on with a timer, never `time.Sleep`.

- Its expiry can never fail a correct owner: a correct owner keeps everything for any window length.
- Inside the sensitivity matrix (synctest) the window is exact, because the fake clock advances only once the dying
  worker has exited.
- For real owners it only gives already-runnable goroutines a chance to run. Detection there is best-effort.
- Cost: about 0.1 s per `Run`, about 2.5 s across `test:unit`, `test:repeat` and `test:integration`.
- Rejected: reading `Observe` once with no wait. That detects nothing, even in the matrix, because the worker may not
  have run yet.
- Rejected: a new `Observation` field. That is new mechanism, and it tells the suite no more than an adapter that
  reports whether work is running.

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
- `TestAbortStopThenFinishJoinsWorker` (`:453-490`): the abort Stop no longer signals the clean worker, so
  `<-o.workerExit.Entered()` (`:467`) would deadlock the bubble. Start `finish` first, then wait for `Entered`, then
  `synctest.Wait`; the rest is unchanged. The exclusion at `:456` names the new check.

Proving tests: `TestEachFailpointTripsExactlyItsCheck`, `TestChecksPassAgainstCleanDouble`,
`TestFailpointTableComplete`, `TestRunOverCleanDouble`, `TestAbortStopThenFinishJoinsWorker`,
`TestFinalizeReportsWorkerHeldAfterSignal`.

## D5. `metric.Server`: stop handing the Start context to requests

Measured: no handler reads the request context (inventory). Options:

- (a) Recommended: delete `BaseContext` (`handler.go:145`). Requests are served under `net/http`'s documented default
  base. Stop's `Shutdown` and `Close` already end serving. No cancel to keep, and `metric` no longer holds any context
  in a closure.
- (b) A run context created in `start`, captured by `BaseContext`, its cancel kept privately and called where Stop
  clears the handles (`:279-281`). This adds a field, a closure that holds a context, and a cancel on every failed
  Start return (`:152`, `:164`), for a context nothing reads.

Behaviour either way: ending the Start context after Start returned stops nothing, and requests keep being served.
Proof: new `TestServerServesAfterStartContextEnds`. `Start(startCtx)`, cancel, GET `/metrics` answers 200, Stop
returns nil, nothing held. It passes before and after the change; it is a regression guard, with sensitivity shown
by M5. The suite's D1 also covers the serve goroutine.

Docs: `handler.go:54-55` becomes "ctx bounds Start only; ending it after Start returned stops nothing. Requests are
served until Stop." `metric/README.md:209` becomes "One-shot synchronous bind; ctx bounds Start only".

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
- Reviewer contract `:240-241`: "Verify startup work runs under the context given to `Start`, continuing work under
  the one run context `Start` creates and `Stop` ends, and every task joins `Stop` (ADR-113); continuing work that
  ends when the Start context ends is `BLOCKING`. A `Run` owner's work derives from `Run`."
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
  context, ended in `Stop`, ADR-113) or nil defaults". Adds 58 bytes; `AGENTS.md` is 21,934 bytes, leaving 2,066; row
  counts unchanged.
- `.agents/rules-index.md:17`: rule column as above; where column adds "ADR-113"; enforcement column becomes
  "`TestNoRetainedContext` (`task test:unit`) for struct fields; `lifecycletest`'s `StartContextEndStopsNothing` for
  owners whose tests run the suite; invented roots and nil defaults are review only".
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

- graph-ingest's request subscriptions take the run context, because natsclient parents their handlers on it. A Start
  context that ends during their setup wait does not cut it short; natsclient caps that wait at
  `DefaultRequestTimeout` (5 s) per subscription (`request.go:293-296`). Filed as inventory issue 1.
- No test can see which context builds the entity cache. Its expiry sweep has no observable outside `internal/cache`
  without a hook, and `Get` checks expiry itself (`cache.go:131`) while Hybrid's LRU bounds size. The PR records it as
  an uncovered case.

## D12. #301 (the four test-only fields)

This change does not remove the reasons #301 gives, which are untouched by moving work between contexts:
`initStorage` still replaces the entity bucket before the sweep, so `watchEntityStates` is still needed, and
`*natsclient.Client` still has no seam for `subscribeRequests` and `consumeStream`. The suite adapter itself uses
`consumeStream` (`lifecycle_adapter_test.go:36-48`). #301 stays open, with this answer posted there. This PR adds no
field.

## Mutants

`task mutate:check`; the integration ones with `GOFLAGS=-tags=integration`, or by hand per `docs/testing.md:230`
while #129 is open.

- M1: D1 without the `startCtx` cancel. Detected by `TestEachFailpointTripsExactlyItsCheck/workDiesWithStartContext`
  ("did not detect").
- M2: D1 with the settle-window comparison removed. Same detector.
- M3: the clean double builds its run context from `ctx`. Detected by `TestChecksPassAgainstCleanDouble`.
- M4: D2 accepting a nil Stop. Detected by `TestEachFailpointTripsExactlyItsCheck/abortStopDropsCause`.
- M5: metric's Start closes the server when `ctx` ends (`go func() { <-ctx.Done(); httpServer.Close() }()`).
  Detected by `TestServerServesAfterStartContextEnds` and `TestServerLifecycleSuite/StartContextEndStopsNothing`.
- M6: graph-ingest's `runCtx` from `ctx` (today's `:974`). Detected by
  `TestGraphIngestLifecycleSuite/StartContextEndStopsNothing` and `TestComponent_StartContextEndStopsNothing`.
- M7: graph-ingest's `setupSubscriptions(runCtx, …)`. Detected by `TestGraphIngest_StartBoundedByItsContext`.
- M8: graph-ingest's adapter back to `statusDone != nil`. M6's suite subtest then passes: this is the adapter's
  sensitivity, recorded as a survivor if so, which is expected.

## Merge order against PR 3 (#291)

No PR exists for #291. This PR merges first. PR 3 branches from `main` afterwards and edits the connect block
(`component.go:981-994`), which this PR only re-points from `runCtx` to `ctx`. #291's plan note already assumes that
split ("waits under the Start context"), so the two do not collide. Change 3 needs nothing from these adopters. Open
point for change 3 only: the manager hands each `Start` a startup-bounded context and keeps no per-component cancel
(ADR-113).

## Owner questions

1. Metric (D5): does ADR-113's "Start creates the one run context" bind an owner whose continuing work reads no
   context? Recommended reading: no, so option (a) deletes `BaseContext`. If yes, option (b).

Otherwise none. The #282 point 4 premise correction (inventory, "`lifecycletest.Run` callers") is a record, not a
question.
