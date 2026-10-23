# Design: natsclient-handler-root

Base `c499381` (code = `05d6ec6`). Retires #311 and #317. Inventory: `inventory.md`. `Subscribe`'s defect of the same
shape is #378, not fixed here.

## Context

`SubscribeForRequests` parents every request handler on the context given to the call
(`natsclient/request.go:327-329`, `:342`, `:351`). Owner ruling #9 comment 5994720412 item 2 says a setup context must
not parent handlers, for the consume functions "and any other function with the same shape";
`ConsumeStreamWithConfig` already follows it (`stream.go:445`, `:455-461`; `owned_delivery.go:124-170`). Under ADR-113
graph-ingest therefore registers its request handlers under its run context (`processor/graph-ingest/component.go:1024`,
`:1027`), so its Start context no longer bounds those registrations, and `start` re-reads the Start context afterwards
(`:1030-1035`). Separately, `Connect` refuses with "already connected" (`client.go:467`) on a client whose connection
nats.go has closed after a finite `WithMaxReconnects` ran out, while `Status` reports Disconnected (`:1815-1819`); and
ADR-112 cites `client.go:355-356` for the reconnect defaults, now at `:308-309`.

## Options

0. Do nothing. Every adopter of `SubscribeForRequests` must pass its run context and then re-check its Start context
   itself. That is graph-ingest now, and agentic-loop, agentic-tools and graph-query once ported; all three pass a run
   context at the pin. The only place an adopter learns this is a doc, and passing the Start context silently kills
   every later handler.
1. **Give `SubscribeForRequests` the consume split (recommended).** No new surface; the signature is unchanged.
2. Option 1 plus a `SubscribeForRequestsContexts` taking a caller handler parent, mirroring
   `ConsumeStreamWithConfigContexts`. Rejected: no caller needs it. graph-ingest drains its subscriptions before it
   cancels its run context (`component.go:1139-1147`), so the run context never ends one of its handlers today.

## Decisions

**D1. `SubscribeForRequests`' context bounds setup only.** It bounds the subscribe and the PING/PONG wait, as now
(`client.go:1060-1150`; the round trip at `:1114-1116`), and is not retained. Each handler invocation's context
descends from a context the client owns for that subscription, made with `clientOwnedHandlerContext`
(`stream.go:460-461`, the consumers' constructor, reused; no second root constructor). The client cancels it when
`Close` begins, including after the subscription's delivery has ended while a handler still runs. It also cancels it
once delivery has ended and the last handler invocation has returned, as `ownConsumer` does
(`owned_delivery.go:160-165`). That second cancel cannot be seen through a handler: each invocation's context already
ends when the handler returns (`request.go:351-352`). So it has no test and no mutant; it is stated so that the root
does not outlive its subscription. `Close` still joins every invocation. Each invocation keeps its per-request timeout
(`request.go:351`) and the trace from the request's headers (`:343-345`), and carries no value of the call's context.

Where the cancel lives and how the watcher (`client.go:1112`, `:1156`) sees `Close` is the developer's choice. The
binding constraint: no `context.Context` is stored on `Client`, and only the root's cancel is kept (ADR-113; developer
contract, Context ownership).

`Subscribe` also goes through `subscribeOwned`, and its handler parent is unchanged: still the call's context
(`client.go:1035-1038`). That is #378. The change to `SubscribeForRequests` must not alter `Subscribe`'s behaviour.

**D2. graph-ingest passes its Start context.** `setupQueryHandlers(ctx)` and `setupMutationHandlers(ctx)` at
`component.go:1024` and `:1027`. The registration then fails on an ended Start context itself (`client.go:1070-1073`
pre-check; `:1140-1141` after the round trip). The `ctx.Err()` block at `:1030-1035` stays. It still covers a Start
context that ends after the last registration succeeded but before `start` returns. Examples: during
`setupMutationHandlers`' log line (`mutation_runtime.go:33`), or at the moment the last PONG arrives. Nothing else reads
the context then: `subscribeOwned` returns on success without reading it (`client.go:1121`), and `Guard.Start` commits
without reading it (`internal/lifecycleguard/guard.go:86`). Under ADR-113 a context that ends before `Start` returns
fails `Start`.

Its comment's premise ("register under the run context") no longer holds, so the comment becomes: "A Start context
that ends after the last registration succeeded, while setup is still running, fails Start here (ADR-113: if ctx ends
before Start returns, Start returns its error and holds nothing). The guard's rollback then runs release." The
`errors.Join(err, ctx.Err())` at `:1025` and `:1028` stays (#315; the failing fake in
`TestGraphIngest_StartReportsItsContextWhenHandlerSetupFails` returns `ErrNotConnected` without the context).

A new regression, T6, guards the kept check. It builds the component with `deps.Logger` set to a `slog.Handler` that
cancels the Start context when it handles the record "mutation handlers registered". That record is logged after the
last registration succeeded and before setup returns. T6 requires that `Start` wraps `context.Canceled`, holds nothing,
and that a later `Stop` returns nil. It uses the production logger dependency (`component.go:678`) and needs no new
field.

Doc and test-comment edits:

- `Start` doc (`component.go:958-963`): "request handlers … run under a run context" becomes "request handlers run
  under a context natsclient owns; release drains their subscriptions, which waits for running handlers".
- `TestGraphIngest_StartFailsWhenItsContextEndsDuringHandlerSetup` (`lifecycle_integration_test.go:438-444`): "the
  registration itself does not see the ending; start's check of its context before the status loop does" becomes "the
  registration itself sees the ending".
- `TestGraphIngest_StartReportsItsContextWhenHandlerSetupFails` (`lifecycle_integration_test.go:476-483`): "T-G4 lets
  the registration succeed, so it reaches start's final ctx.Err check; here the registration fails and start returns
  early" becomes "T-G4's registration fails on the ended Start context itself; here the fake fails the registration
  with an error of its own".

T-G4 becomes table-driven, with two rows that mirror `:497-500`:

- `query`: cancel at call 1.
- `mutation`: cancel at call `len(querySubjects)+1`.

Each row asserts that `Start` wraps `context.Canceled`, holds nothing, and makes no registration after the one during
which its context ended (the fake's call count equals the cancelling call). The count assertion is the only part that
catches a loop reverted to `runCtx`. If the query loop is reverted, the first mutation registration fails on the ended
context. If the mutation loop is reverted, the kept final check fails `Start`. Either way `Start` still wraps
`context.Canceled`. Both rows read #301's `subscribeRequests` field, as T-G4 does today; this change adds no
field.

**D3. `Connect` names the closed case (#317; MEDIUM under #185; no mechanism).** The doc at `client.go:444-445` says
it refuses a client that already has a connection installed: one that is connected, one nats.go is reconnecting, or
one nats.go has closed because a finite `WithMaxReconnects` ran out (Status then reports Disconnected). A Client
connects once; recovery is a new Client (`integration_test.go:46-52`). The refusal text at `:467` becomes
`"connection already installed; a Client connects once"`. The sentinel stays `errs.ErrAlreadyStarted` (graph-ingest
depends on it, `component.go:991`). `:547`, a losing concurrent `Connect`, is unchanged: the winner's connection is
live.

**D4. Docs.**

- `request.go:278-316`: add the D1 paragraph, with the ruling's cite.
- `request.go:323`: the comment no longer says each request derives from ctx.
- README "Consumer contexts and handler panics" becomes "Handler contexts and handler panics", with one sentence for
  `SubscribeForRequests` and a note that `Subscribe` does not yet follow it (#378).
- README `:38-39`: one sentence for the finite-reconnect case.
- ADR-112 Status gains an amendment line (natsclient-handler-root, #317) covering two edits:
  - §5's cite is corrected to `client.go:308-309` at the archive commit, re-measured then.
  - §5 gains one bullet: "With a finite `WithMaxReconnects`, nats.go closes the connection once its attempts run out.
    The client keeps it installed and reports Disconnected, and `Connect` refuses with `errs.ErrAlreadyStarted`;
    recovery is a new Client. Measured at 0 by `TestIntegration_ReconnectionIsRedial`; above 0 not promised (read at
    `nats.go:3591-3597`, v1.54.0)."

## Observable behaviour and proving tests

Task 2.1; each new test fails on the base, except T6, a regression guard whose sensitivity is M7.

| # | Behaviour | Test | Base failure |
| --- | --- | --- | --- |
| T1 | A request arriving after the setup context was cancelled gets a live handler context | `TestRequestHandlerContextOutlivesSetupContext` (natsclient, embedded server) | handler sees `context.Canceled` |
| T2 | `Close` beginning while a handler runs ends its context; `Close` returns nil after it returns | `TestRequestHandlerContextEndsWhenCloseBegins` | context stays live until the test's bound |
| T3 | Same, after `Unsubscribe` while the handler still runs | `TestRequestHandlerContextEndsWhenCloseBeginsAfterUnsubscribe` | as T2 |
| T4 | A Start context ending during a query or a mutation registration fails graph-ingest `Start` at that registration and holds nothing | T-G4, two rows (D2) | both rows fail on the base through the call count (every registration runs under `runCtx`) |
| T5 | With `WithMaxReconnects(0)`, after the broker restarts: once `GetConnection().IsClosed()` (awaited with `probe.Await`, not read once), `Status` is Disconnected and `Connect` matches `errs.ErrAlreadyStarted` with the D3 text | extend `TestIntegration_ReconnectionIsRedial` | text says "already connected" |
| T6 | A Start context that ends after the last request registration succeeded, before setup returns, fails graph-ingest `Start`, which holds nothing; a later `Stop` returns nil | `TestGraphIngest_StartFailsWhenItsContextEndsAfterHandlerSetup` (integration; `deps.Logger` handler cancels on "mutation handlers registered") | passes on the base; guards M7 |

T1-T3 use a real connection, so they do not run inside `synctest` (#156). T5 is also the pinned-version probe for
"nats.go closes the connection after its reconnects run out".

## Mutants

Task 2.3.

- M1: `requestCallback` gets the call's context again. Expected detector: T1.
- M2: the handler root is not cancelled when `Close` begins. Expected detectors: T2, T3.
- M3: the root is cancelled on `Close` only while delivery has not ended. Expected detector: T3.
- M4: `:467` says "already connected" again. Expected detector: T5.
- M5: `ErrAlreadyStarted` is dropped from `:467`. Expected detectors: T5 and `client_close_final_test.go:702`.
- M6q: only `setupQueryHandlers` gets `runCtx` again. Expected detector: T4 `query` row (call count).
- M6m: only `setupMutationHandlers` gets `runCtx` again. Expected detector: T4 `mutation` row.
- M7: graph-ingest's final `ctx.Err()` check (`component.go:1030-1035`) is deleted. Expected detector: T6.

## Spec delta

`specs/transport-client/spec.md`:

- RENAMED: "A consumer's setup context does not parent its handlers" becomes "A setup context does not parent its
  handlers".
- MODIFIED, under the new name: the requirement now covers `SubscribeForRequests`, says `Subscribe` does not yet follow
  it (#378), and gains one scenario.
- MODIFIED: "Subscribe returns after the server has read the SUB", one clause at `:268-269`.

`.openspec.yaml` drops `skip_specs`. The proposal's Capabilities section names `transport-client`.

## Residual risk

Declared under #185.

- A caller that cancelled its subscribe context to stop in-flight handlers can no longer do so. A running handler's
  context now ends only when `Close` begins or its per-request timeout passes (`request.go:351`).
  `Subscription.Drain(ctx)` returning nil is what proves every callback has returned (`client.go:965`). `Unsubscribe`
  stops delivery but does not wait for a running handler (`client.go:950-955`; T3 relies on that), so a caller must not
  release what its handler uses on `Unsubscribe` alone. graph-ingest already drains, and does so before it cancels its
  run context (`component.go:1139-1147`). The pin adopters change when they are ported.
- A handler running after a failed registration keeps a live context until `Close` begins or its request timeout
  passes. graph-ingest refuses such a handler at entry (`component.go:1059-1074`).
- A request without trace headers no longer inherits a trace from the subscribe context.
- With a finite `WithMaxReconnects`, graph-ingest's `Start` on a closed connection waits in `WaitForConnection` until
  its context ends (`component.go:991-1001`). No production code sets a finite limit.
- `Subscribe` keeps the old shape until #378.

## Merge order

PR #376 touches no natsclient, graph-ingest or transport-client file. #374 edits `docs/repository-map.md` and `AGENTS.md`.
If this change's archive (task 3.2) adds a `docs/repository-map.md` row for the archived change, it overlaps #374 on
that file. Either may land first, and the later one merges `origin/main` and keeps both rows. This change does not
edit `AGENTS.md`.
