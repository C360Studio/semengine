# Design: natsclient-subscribe-handler-root

Base `4675084` (code = `46d3cd5`). Retires #378. Inventory: `inventory.md` (INVENTORY PASS). The mirror is
`openspec/changes/archive/2026-10-23-natsclient-handler-root/` (the change for #311).

## Context

`Subscribe` passes no handler cancel to `subscribeOwned` (`client.go:1041`). Each message's context is
`context.WithTimeout(ctx, 30*time.Second)` on the call's own `ctx` (`:1044`): it ends with the call's context, `Close`
never ends it, and it inherits the call's context values.

The message logger works around this by subscribing under a run context that keeps the Start context's values without
its cancellation (`service/message_logger.go:276`, `:288`). The cost: its Start context does not bound the subscribes,
and its `start` returns nil without a final look at that context (`:292-305`).

The owner's fix shape (#378, 2026-10-23): "pass a `clientOwnedHandlerContext` cancel from `Subscribe` too, and change
the message logger's caller."

## Options

0. **Do nothing.** Every `Subscribe` adopter must keep the four facts of the inventory's adopter seam, and the logger's
   ADR-113 gap stays open.
1. **The split; the logger subscribes at Start under its Start context and checks it after the last subscribe
   (recommended).** It is graph-ingest's shape (`processor/graph-ingest/component.go:1025`, `:1028`, `:1031`).
   - Gain: a Start deadline cuts a subscribe whose answer has not come back.
   - Cost: a residual Option 2 does not have. A subscribe the Start context cuts after its SUB was sent is unsubscribed
     (`client.go:1153`). Its handler context is now the client's and still live, and the subscription is in no logger
     map. A message delivered between that SUB and its UNSUB can be stored after `Start` failed and rolled back. Only
     the client's `Close` joins that handler, not the logger's `Stop`.
2. **The split; the logger keeps the run context for its Start subscribes, plus the same final check.**
   - Gain: no Start subscribe is ever cut, so the residual above cannot happen.
   - Cost: a Start deadline cannot cut a subscribe waiting on a slow server. `Start` runs past its deadline by up to
     `DefaultRequestTimeout` (5 s, `request.go:18`) for the subscribe in progress, plus connection waits that are not
     promised. That is the cost #282 named ("A caller cannot bound startup").

No test here tells Option 1 from Option 2. Both properties need a server that withholds its PONG, which the logger's
shared-server tests cannot produce (M5). The choice therefore rests on two grounds:

- ADR-113's Decision: `Start` "does its startup work under `ctx`" (`docs/adr/113-start-context-bounds-start.md:59`).
- The owner's fix shape: "change the message logger's caller".

On those grounds Option 1 is recommended, and its residual is declared (Residual risk, first item). Option 1 with
graph-ingest's entry check (a handler refused until start completes, `component.go:1069-1071`) would close that window.
It is rejected: the logger stores messages delivered during `Start` today, and the check would drop them. A
failed-Start flag instead would be new state guarding a diagnostic entry in a logger that cannot run again.

## Decisions

**D1. `Subscribe`'s context bounds setup only.**

- `Subscribe` makes a context the client owns with `clientOwnedHandlerContext` (`stream.go:461`; no second constructor)
  and passes its cancel to `subscribeOwned`.
- Each handler invocation's context descends from that owned context. It keeps the 30-second per-message timeout and the
  trace from the message's headers (`client.go:1047-1049`), and carries no value of the call's context. This is the
  `SubscribeForRequests` wording (`transport-client/spec.md:180-181`); the inventory's §3a(c) shows the logger loses
  nothing by it.
- The client cancels the owned context when `Close` begins, also after the subscription's delivery has ended while a
  handler still runs, and once the last handler has returned. The existing watcher does this (`client.go:1181-1193`).
- No context is stored on `Client`. Only the cancel is kept (ADR-113; developer contract, Context ownership).

**D2. The logger subscribes at Start under its Start context, with a final check.**

- `start` passes `ctx` to the subscribes in its loop (`:288`).
- `start` checks `ctx.Err()` once more after the loop, before it starts the retry and before the "MessageLogger
  started" record (`:300`). A failed `Start` therefore never logs "started". ADR-113 (`:59-60`): if ctx ends before
  `Start` returns, `Start` returns its error and holds nothing.
- The guard's rollback runs `cleanup` under a fresh 5-second context without the Start context's cancellation
  (`pkg/lifecyclecleanup/lifecyclecleanup.go:36`). It joins the retry, drains, and clears the pending set (`:542`).
- A subscribe that the Start context cuts returns a transient error (`client.go:1086-1089`, `:1160-1161`). The subject
  is recorded as pending and logged once as "retrying", and the loop's check or the final check then fails `Start`.
- A context that ends while the "started" record is being written does not fail `Start`. No check can close the last
  instant before `Start` returns; this one comes after the last subscribe, the last step that does startup work.
- No ADR-113 gap is left unfixed, so nothing is filed.

**D3. A message whose handler starts during the client's `Close` is dropped, and the spec declares it.**

- Once `Close` begins, the handler's context has ended. The logger's entry check (`:651`) returns without storing the
  message. A handler that passed that check before `Close` began still stores its message.
- How many messages nats.go delivers during `Close`'s drain is not promised (ADR-112), so no test asserts a count.
- A composition root that stops the logger before closing its client loses nothing: `Stop` drains every subscription
  first (`:522`).
- Why accept rather than change the entry check: the check is the logger's existing contract for an ended context. The
  same rule already holds for `SubscribeForRequests` and the consumers.

**D4. Keep `runCtx` and `runCancel`, with new comments.**

- Under ADR-113 the subscribe retry is continuing work, and it needs a context that is not a new root. The run context is
  that context; dropping it would need `context.Background` in a goroutine, which is banned.
- The name stays, because "run context" is ADR-113's term.
- The comments at `:205` and `:274-275` change to "the run context bounds the retry's subscribes".
- Moving the cancel ahead of the retry join (`:519`) would let `Stop` cut a retry's subscribe that is waiting for the
  server. No logger test can observe that (it needs a server that withholds its PONG), so it is not done.

**D5. The two logger tests' sensitivity.**

- `TestMessageLoggerKeepsCapturingAfterStartContextEnds` (`service/message_logger_subscription_integration_test.go:165`)
  now guards D1 across the seam. With D2 the subscription is made under the cancelled Start context, so M1 drops the
  message. Its comment is rewritten to say so.
- `TestMessageLoggerStopDrainsInFlightMessage` (`:192`) loses the run-context mutant, which is now equivalent: the
  cancel ends no handler. Its comment names M6 instead. Message 2 reaching the handler during the drain stays measured,
  not promised (archived logger design, D2 drain probe).

**D6. Stale text and the task that fixes each.** Lines are at `46d3cd5`; task 2.1 re-measures them after the merge.

| Text | Task |
| --- | --- |
| `natsclient/client.go:997`, `:1036`; `stream.go:455-460` | 2.2 |
| `service/message_logger.go:205`, `:250-256` (`Start` doc), `:274-275`; test comments at `message_logger_http_test.go:209`, `message_logger_subscription_integration_test.go:159-164`, `:184-191`, `:324` (cite functions, not `client.go` lines) | 2.3 |
| `natsclient/README.md:58-60`; `service/README.md:438-441`; `docs/admission-ledger.yaml:1013-1017`, `:1128-1131`; ADR-113 `:47` (the cite corrected to `stream.go:455-460`; no Status line, since the record adds no decision) | 3.1 |
| `transport-client` and `service-lifecycle` specs | 3.2 (sync) |
| Archived logger design `:85-87`, `:481`, `:484` | not edited (history) |

## Observable behaviour and proving tests

Each test fails on the base. T1-T5 call existing API and compile first. Tests and implementation land in one commit per
task, and the base failure is recorded in the task's notes (the #380 lesson).

| # | Behaviour | Test | Base failure |
| --- | --- | --- | --- |
| T1 | A message arriving after `Subscribe`'s setup context was cancelled gets a live handler context | `TestSubscribeHandlerContextOutlivesSetupContext` (natsclient, embedded server) | handler sees `context.Canceled` |
| T2 | `Close` beginning while a `Subscribe` handler runs ends its context; `Close` returns nil only after it returns | `TestSubscribeHandlerContextEndsWhenCloseBegins` | context stays live |
| T3 | As T2, after `Unsubscribe` while the handler still runs | `TestSubscribeHandlerContextEndsWhenCloseBeginsAfterUnsubscribe` | context stays live |
| T4 | A setup context carrying a trace: a message without trace headers gives a handler context with no trace; a message with headers gives the headers' trace | `TestSubscribeHandlerContextCarriesOnlyTheMessageTrace` | first handler sees the setup trace |
| T5 | With one subject, the logger's Start context, cancelled while that subject's "Subscribed to subject" record (Debug) is held, fails `Start` with `context.Canceled`; no "MessageLogger started" record is written; the suite adapter's held list is empty; a later `Stop` returns nil | `TestMessageLoggerStartFailsWhenItsContextEndsAfterSubscribing` (service, integration; the existing `holdingHandler`, a production logger dependency, enabled from Debug) | `Start` returns nil |

- T4 publishes both messages on the raw connection (`GetConnection()`): the header-free one with `Publish`, the other
  with `PublishMsg` after `InjectTrace`. It cannot use `Client.Publish`, which adds a new trace when its context has
  none (`client.go:1216-1217`).
- T1-T4 reuse `request_handler_root_test.go`'s helpers. They use a real connection, so they do not run inside `synctest`
  (#156).
- The two retained logger tests are covered in D5.

## Mutants

Task 2.4, run with `task mutate:check`.

- M1: `Subscribe`'s callback built on the call's context again. Expected detectors: T1, and
  `TestMessageLoggerKeepsCapturingAfterStartContextEnds`.
- M2: `Subscribe` passes a nil cancel. Expected detectors: T2, T3.
- M3: the owned context is built as `context.WithoutCancel(ctx)`. Expected detector: T4.
- M4: the logger's final check deleted. Expected detector: T5.
- M5: the logger's Start subscribes go back to `runCtx` (Option 2). Expected result: survivor, declared. Telling the
  two apart needs a withheld PONG, which the shared-server logger tests cannot produce; reviewed only.
- M6: `drainSubscriptions` unsubscribes instead of draining. Expected detector:
  `TestMessageLoggerStopDrainsInFlightMessage`.

## Spec delta

- `specs/transport-client/spec.md`:
  - MODIFIED "A setup context does not parent its handlers": `Subscribe` is added, the exception goes, and T4 becomes a
    new scenario.
  - MODIFIED "Subscribe returns after the server has read the SUB": one clause.
- `specs/service-lifecycle/spec.md`: MODIFIED "The message logger's Start bounds Start and Stop is the only stop": D2,
  D3, the cut-subscribe residual, and the T5 scenario. That `Start`'s subscribes run under its context is listed as
  reviewed only (M5).

## Residual risk

Declared under #185.

- **Cut-subscribe residual (Option 1).** A message delivered to a Start subscribe that the Start context cut, between
  its SUB and its UNSUB, can be stored by a handler still running after `Start` failed and rolled back. The client's
  `Close` joins that handler; the logger's `Stop` does not. MEDIUM at most: it needs that delivery inside the window of
  a `Start` that is already failing, and the logger is then terminal.
- A caller that cancelled its subscribe context to stop running handlers no longer can. `Subscription.Drain(ctx)`
  returning nil, or `Close`, is the proof that handlers have returned. The only production caller drains (`:522`).
- A message without trace headers no longer inherits a trace from the subscribe context. The logger loses nothing
  (inventory §3a(c)).
- A logger message whose handler starts during the client's `Close` is dropped (D3).
- A subscribe cut by the Start context logs one "retrying" error before `Start` fails (D2).
- A context that ends while the "started" record is written does not fail `Start` (D2).
- M5 is undetected (Mutants).

## Merge order

- #376 merged on 2026-10-23 as `a1c5f21` and touched none of this change's code files. Task 2.1 merges `origin/main`
  into this branch, with no rebase, and re-measures the ledger, README and `service-lifecycle` cites. The ledger lines
  move down by one; the README logger text moves to about `:441-444`.
- #381 shares no file with this change apart from a possible `docs/repository-map.md` row at archive. Either may land
  first, and the later one keeps both rows.
