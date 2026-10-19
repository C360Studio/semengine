# Inventory: start-context

base: `acfa927b8bd2869e72ff16dbbe343c4c26680675` (origin/main `1dc132b` plus the claim skeleton; draft PR #310)

Status: `INVENTORY PASS` (semengine-reviewer, 2026-10-18, round 1 at `092c5e8`; cite drift fixed in round 2).
Every cite is at the base commit.

## Question

What must change on this tree so that the context given to `Start` bounds only `Start`, and `Stop(ctx)` is the only
stop (ADR-113), for every owner whose test runs `lifecycletest`, for the suite itself, and for every text that states
the old rule?

## Scope and non-goals

In: the lifecycle suite (2 of 8 checks), the sensitivity double, the adopters `metric.Server` and graph-ingest's
`start`, the guard's test owner and natsclient (confirm only), the `LifecycleComponent` comment, the `lifecycle-suite`
delta, and the rule texts ADR-113 lists.

Out: #291 (PR 3), #309, natsclient code, `internal/lifecycleguard/guard.go`, the component manager (change 3).

## Repositories read

This tree only. No sister repository is needed: `lifecycletest` is under `internal/`, so Go refuses any import of it
from outside this module. No consumer in `docs/inventory-scope.md` imports semengine's `metric` or `component`; the
table lists semvocab and semsparql as importing `vocabulary` and `graph` only. No production caller of
`metric.NewServer` exists in this tree (`git grep -n "NewServer(" -- '*.go' ':!*_test.go'` finds only its
definition, `metric/handler.go:37`, and comment lines in `metric/doc.go`). Pin: no package is carried, so no pin
probes are owed. ADR-113's pin cites stand as written.

## 1. Claimed gap

No check ends the Start context and then requires the owner to keep running.

- `internal/harness/lifecycletest/lifecycletest.go:187`: `CheckControlledStopUnderLiveStartAuthority` never cancels
  the Start context (`:188-189`).
- `:206`: `CheckAbortStopPreservesCause` cancels it (`:212`) and then reads only Stop's error.

Confirmed.

## 2. Every spelling of "what the Start context owns"

- `component/lifecycle.go:49-53`: "Stop is controlled while the Start context is still live … Ending the Start
  context first makes Stop an abort".
- `internal/harness/lifecycletest/lifecycletest.go:100-101` (Run's list), `:183-224` (the two checks).
- `internal/harness/lifecycletest/refowner_test.go:43` "worker goroutine derived from Start authority"; `:86`, `:110`
  `runCtx, cancel := context.WithCancel(ctx)`; `:243`, `:264`, `:273` (finalize ends workers "through Start
  authority"); `:303-305` (table rows); `:456` (exclusion keyed to the controlled check); `:467` (waits for the worker
  to be entered before finish).
- `openspec/specs/lifecycle-suite/spec.md:19-20` "controlled Stop completing under live Start authority … abort Stop
  preserving the caller's context cause"; `:87-88` "A worker that no Stop has signalled SHALL be ended through its
  Start context".
- `metric/handler.go:54-55` "ctx is the exact base context for served requests"; `:145`
  `BaseContext: func(net.Listener) context.Context { return ctx },`.
- `metric/README.md:209` "One-shot synchronous bind; context owns served requests".
- `metric/handler_test.go:167` `require.Same(t, t.Context(), server.server.BaseContext(server.listener))`, a test
  that pins the Start context as the request base; under D5 it would call a nil function.
- `metric/README.md:71`, `:554`, `:565`, `:763` and `metric/doc.go:373-374` name the Start context
  `runtimeCtx := context.Background()`; `metric/README.md:490` has `go g.processingLoop(ctx)`.
- `processor/graph-ingest/component.go:974` `runCtx, cancel := context.WithCancel(ctx)`; `:547-548` "submission
  contexts are lexical children of Start"; `:951-956` the Start comment; `:973` the start comment.
- `processor/graph-ingest/keyed_ingest.go:161` "Start-derived pool context"; `:204` "the pool context Start derives";
  `:220-221` "when Start's context ends, or at Stop".
- `processor/graph-ingest/component_integration_test.go:131-158`: `TestComponent_RespectsContext_Cancellation`
  asserts the old rule ("the status loop Start runs under its context returns once that context is cancelled").
- `processor/graph-ingest/test_owner_support_test.go:38` "Accepted Start authority stays live through concrete Stop";
  `test_owner_test.go:22-50` cites a spec `test-cleanup-policy` that does not exist under `openspec/specs/`
  (issue 2 below).
- `pkg/errs/shutdown.go:17` "waiting for Start-owned runtime completion".
- `docs/setup-plan.md:291` "do not invent background roots"; `:293-294` "keeping Start authority live … abort is
  distinct".
- `docs/admission-ledger.yaml:176-177` (the `lifecycletest` row's `consumer_purpose`).
- `docs/adr/108-processor-shell.md:75` "controlled and abort Stop".
- `.agents/contracts/semengine-developer.md:215-218`, `:219-221`, `:232-233`, `:235-237`.
- `.agents/contracts/semengine-reviewer.md:237-241`, `:242-245`, `:255`.
- `.agents/contracts/semengine-architect.md:401-404`, `:405-409`.
- `AGENTS.md:71` row "never invent roots or nil defaults"; `.agents/rules-index.md:17`, its row ("invented roots and
  nil defaults are review only").

Searched: `git grep -n -i "start authority|Start-owned|Start context|abort is distinct|invent roots|invent background
roots|Start's context ends|derives from .Start"`. No adapter under `.claude/` or another agent directory repeats the
contract text.

## 3. Adjacent claims

- Open PRs: only #310 (this claim; three files under `openspec/changes/start-context/`). No open PR for #291.
- Specs: `lifecycle-suite` (modified here). `harness-boundaries` `:144-149` ("Bounded cleanup roots") bans roots only
  in `*_test.go` Stop, Close and Terminate calls, so the production-root exception is contract and review text only.
  `background-work` (`:20-22`) is unchanged.
- ADRs: 113 (decides this change), 108 decision 3 and 3(e), 112 (natsclient's `Connect`).
- The guard commits a `Start` without reading the context (`internal/lifecycleguard/guard.go:86-90`), so `start` is
  the only place a Start context that ended during a run-context step can be seen (design D6).
- Open issues (searches: lifecycletest, lifecycleguard, graph-ingest, metric, "start context", lifecycle):
  - #282: retired by this PR (`Closes #282`).
  - #196: stays open. It is the adoption tracker for each ported owner (its last comment); the PR body says
    `Addresses #196`.
  - #291: stays open. It is PR 3, which edits the connect block this PR re-points (`component.go:981-994`).
  - #309: stays open. A panicking cleanup in the guard; `guard.go` is unchanged here.
  - #301: stays open, with the answer to its plan note recorded (design D12).
  - #306: stays open. The cache handle is cleared while readers run (`cleanup`, `clearLifecycleHandles`); the cleanup
    order is unchanged here.
  - #129: stays open. `mutate:check` on integration tests; it decides how M7 and M8 are run (design, Mutants).
  - Issues that name the word but not the touched code: #300 #216 #270 #141 #195 #106 #107 #110 #111 #15 #16 #20 #134
    #198 #68 #284 #36 #29 #33 #80 #81 #86 #280 #109 #94 #95 #96; epics #7 #9 #24 #84.
- Open `class:flake` issues: none.

## 4. Who it is for

New exported symbols: `lifecycletest.CheckStartContextEndStopsNothing` and `CheckStopReturnsItsEndedContext`, which
replace `CheckControlledStopUnderLiveStartAuthority` and `CheckAbortStopPreservesCause`. The package is test-only and
internal. Readers: `Run` (`lifecycletest.go:97-106`) and the `checks` list (`refowner_test.go:204-215`). No other
reference to either old name exists outside the package (`git grep` finds only ADR-113 `:97`). No new production
export.

## 5. Problem shape

An owner creates its own run context, keeps only the cancel, and ends it in its stop. Closest instances, both
already adopted:

- `natsclient/client.go:607` `pollCtx, cancel := context.WithCancel(context.Background())` (doc `:601-604`; `Close`
  cancels and joins).
- `natsclient/stream.go:460-461` `clientOwnedHandlerContext` (doc `:455-459`).

Counter-instance: `natsclient/request.go:322-329`, where `SubscribeForRequests` parents every handler on the caller's
subscribe context (issue 1 below; design D11).

## 6. Pin ADRs

No package is carried. ADR-113 records which pin ADR-094 and ADR-095 sentences are superseded or kept for this
module.

## `lifecycletest.Run` callers and what each `Observe` reads

- `internal/lifecycleguard/guard_test.go:110`: owner `:25-99`; its worker (`:52-55`) waits only on `stop`; `Observe`
  reads whether the worker is running (`:92-96`). It already survives the Start context ending.
- `metric/lifecycle_test.go:76`: `Observe` (`:37-53`) lists the listener, the server, the serve goroutine and admitted
  requests. None of these ends when the Start context ends.
- `natsclient/client_lifecycle_test.go:792`: `Observe` (`:49-82`) lists kinds of running work (`client.go:189-198`);
  the poller and handlers run under client-owned roots.
- `processor/graph-ingest/lifecycle_integration_test.go:345`: `Observe` (`lifecycle_adapter_test.go:61-98`) lists
  handle fields that are not nil, not whether work is still running; the status loop is listed by
  `c.statusDone != nil` (`:89`).
- `internal/harness/lifecycletest/refowner_test.go:530-538` (`TestRunOverCleanDouble`, two Runs).

So #282 point 4's premise, that the new check fails for `metric` and graph-ingest until they adopt, does not hold on
this tree as measured: it would fail for neither. Design D6 changes graph-ingest's adapter to report whether the
status loop is running, which is what makes the check fail on today's code; `metric` gets its own test (D5). A
correction comment goes on #282, like 6250135029.

## metric: who reads the request context

Handlers on the mux (`metric/handler.go:113-139`): promhttp's `HandlerFor`, `/health`, `/`. None reads
`r.Context()`. promhttp v1.24.1 reads `req.Context()` only in `option.go:88` and `:105` (instrumentation options this
server does not use). `admittedRequests.wrap` (`:332-343`) does not read it either.

## graph-ingest: which context each start step takes

Startup I/O:

- `Connect` and `WaitForConnection` `:981-994`.
- `initStorage` `:996`: buckets `:1166`, `:1193`; the entity cache at `:1173` is built with
  `cache.NewFromConfig(ctx, …)`, and its expiry sweep goroutine runs until that context ends (`internal/cache/cache.go:126`,
  `:401`). If `initStorage` ran entirely under the Start context, the sweep would die when that context ends, so
  `initStorage` takes both contexts (design D6).
- `startEntityStateGuard` `:999`: its watcher stops before return (#282 comment 6250135029).
- `createStatusBucket` `:1005`.
- Inside `setupSubscriptions` `:1011`: `waitForStream` `:1455`, `ValidateHeartbeatDeliveryPolicy` `:1508`,
  `consumeStream` `:1522` (a setup-only context; `natsclient/stream.go:431-438`) and `recordBindBacklog` `:1609`.

Continuing work:

- the pool `:1008` (`poolCtx`);
- the consumer closure `:1590` (`submitCtx`);
- query and mutation subscriptions `:1014`, `:1017` (the context parents their handlers, `request.go:322-329`);
- the status loop `:1021` (`readiness.go:72-86`).

Ending the run context today: the lane context ends, so every input is negatively acknowledged and redelivered
without being applied (`keyed_ingest.go:219-226`); the status loop returns; request handlers get an ended context.

ADR-113 says natsclient "has this shape already". That holds for `Connect`, the metrics poller and consumer handlers,
not for `SubscribeForRequests` (`natsclient/request.go:322-329`): its handlers' contexts come from the context passed
to the subscribe call. graph-ingest's request handlers must therefore get the run context, and their setup wait is
then capped at `DefaultRequestTimeout` (5 s, `request.go:18`) instead of by the Start context (design D11; issue 1).

## Size

About 120 non-test lines changed (suite about 45, metric about 5, graph-ingest about 30, docs and contracts about 40)
and about 150 test lines.

## Adopter seam (component authors outside this module)

1. What they must know: Start's context bounds only Start; continuing work runs under their own run context, which
   Stop ends.
2. If they do nothing: work tied to the Start context dies once a host ends that context after Start. That is silent.
3. Where they find out: the doc comment (`component/lifecycle.go`). ADR-108 3(e) and ADR-113 already record this as
   a gap.
4. What they should have to know: nothing more. The gap stands because the suite is internal; no new finding.

## Issues filed (status:blocked on this branch, no closing keyword)

1. #311. natsclient: `SubscribeForRequests` uses one context for its setup wait and as every handler's parent. #9 comment
   5994720412 item 2 ruled that the context that bounds setup must not parent handlers, and that this applies to "any
   other function with the same shape". `ConsumeStreamWithConfig` follows the ruling (`natsclient/stream.go:431-438`,
   `:455-461`); `SubscribeForRequests` does not (`natsclient/request.go:322-329`). A caller whose handlers outlive
   `Start` (ADR-113) must therefore pass its run context, and the setup wait is then capped only by
   `DefaultRequestTimeout` (5 s). graph-ingest's query and mutation subscriptions are the case today. MEDIUM.
2. #312. graph-ingest tests cite a spec that does not exist: `processor/graph-ingest/test_owner_test.go:22`, `:57`, `:76`,
   `:105` and `test_owner_child_test.go:52` cite "spec: test-cleanup-policy / Lexical ownership of lifecycle test
   fixtures", and there is no `openspec/specs/test-cleanup-policy/`. The test owner's keeping its Start context live
   until Stop (`test_owner_support_test.go:38`) is harmless under ADR-113 and is reworded in this PR (design D6), not
   filed.
3. #313. `metric/README.md`'s `GPSService` example takes no context in `Stop` (`:496`) and sleeps (`:483`). The parts of
   that example that state the retired rule (`:490`; the `runtimeCtx` name at `:71`, `:554`, `:565`, `:763` and
   `doc.go:373`) are fixed in this PR (design D5); the signature and the sleep are filed.
4. #314. `docs/admission-ledger.yaml:50` (the natsfixture row) lists "lifecycletest.Run over the fixture adapter" under
   `proving_tests`, and no such call exists (`git grep -n lifecycletest -- internal/harness/natsfixture` is empty).
