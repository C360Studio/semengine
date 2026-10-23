# Design: natsclient-handler-root

Base `c499381` (code = `05d6ec6`). Retires #311 and #317. Inventory: `inventory.md`.

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

0. Do nothing. Every adopter of `SubscribeForRequests` (graph-ingest now; agentic-loop, agentic-tools and graph-query
   when ported, all passing a run context at the pin) must pass its run context and re-check its Start context, a fact
   found only in a doc; passing the Start context silently kills every later handler.
1. **Give `SubscribeForRequests` the consume split (recommended).** No new surface; the signature is unchanged.
2. Option 1 plus a `SubscribeForRequestsContexts` taking a caller handler parent, mirroring
   `ConsumeStreamWithConfigContexts`. Rejected: no caller needs it. graph-ingest drains its subscriptions before it
   cancels its run context (`component.go:1139-1147`), so the run context never ends one of its handlers today.

## Decisions

**D1. `SubscribeForRequests`' context bounds setup only.** It bounds the subscribe and the PING/PONG wait, as now
(`client.go:1060-1150`; the round trip at `:1114-1116`), and is not retained. Each handler invocation's context
descends from a context the client owns for that subscription, not from the call's context. The client cancels it when
`Close` begins, including when the subscription's delivery has already ended while a handler still runs. `Close` still
joins every invocation. Each invocation keeps its per-request timeout (`request.go:351`) and the trace from the
request's headers (`:343-345`); it carries no value of the call's context. Mechanism (where the cancel lives, how the
watcher at `client.go:1112`/`:1156` sees Close) is the developer's; it mirrors `ownConsumer`. Binding: no
`context.Context` is stored on `Client`. The root is created in the call, like `clientOwnedHandlerContext`, and only its
cancel is kept (ADR-113; developer contract, Context ownership).

**D2. graph-ingest passes its Start context.** `setupQueryHandlers(ctx)` and `setupMutationHandlers(ctx)` at
`component.go:1024` and `:1027`. The registration then fails on an ended Start context itself (`client.go:1073-1076`
pre-check; `:1137-1138` after the round trip). The `ctx.Err()` block at `:1030-1035` is deleted: its stated premise
("register under the run context") no longer holds. The `errors.Join(err, ctx.Err())` at `:1025` and `:1028` stays
(#315; the failing fake in `TestGraphIngest_StartReportsItsContextWhenHandlerSetupFails` returns `ErrNotConnected`
without the context). The `Start` doc (`:958-963`) changes "request handlers … run under a run context" to "request
handlers run under a context natsclient owns; release drains their subscriptions". The doc comment of
`TestGraphIngest_StartFailsWhenItsContextEndsDuringHandlerSetup` (`lifecycle_integration_test.go:438-444`) changes to
"the registration itself sees the ending"; its body is unchanged.

**D3. `Connect` names the closed case (#317; MEDIUM under #185; no mechanism).** The doc at `client.go:444-445` says
it refuses a client that already has a connection installed: one that is connected, one nats.go is reconnecting, or
one nats.go has closed because a finite `WithMaxReconnects` ran out (Status then reports Disconnected). A Client
connects once; recovery is a new Client (`integration_test.go:46-52`). The refusal text at `:467` becomes
`"connection already installed; a Client connects once"`. The sentinel stays `errs.ErrAlreadyStarted` (graph-ingest
depends on it, `component.go:991`). `:547`, a losing concurrent `Connect`, is unchanged: the winner's connection is
live.

**D4. Docs.**

- `request.go:278-316` gains the D1 paragraph with the ruling's cite; the comment at `:323` no longer says each request
  derives from ctx.
- README: "Consumer contexts and handler panics" becomes "Handler contexts and handler panics", with one sentence for
  `SubscribeForRequests`; `:38-39` gains one sentence for the finite-reconnect case.
- ADR-112 gets a Status amendment line (natsclient-handler-root, #317). §5's cite is corrected to `client.go:308-309`
  at the archive commit, re-measured then. §5 gains one bullet: "With a finite `WithMaxReconnects`, nats.go closes the
  connection once its attempts run out (`nats.go:3591-3597` at v1.54.0; measured at 0 by
  `TestIntegration_ReconnectionIsRedial`, read but not measured above 0). The client keeps it installed and reports
  Disconnected, and `Connect` refuses with `errs.ErrAlreadyStarted`. Recovery is a new Client."

## Observable behaviour and proving tests

Task 2.1; each fails on the base.

| # | Behaviour | Test | Base failure |
| --- | --- | --- | --- |
| T1 | A request arriving after the setup context was cancelled gets a live handler context | `TestRequestHandlerContextOutlivesSetupContext` (natsclient, embedded server) | handler sees `context.Canceled` |
| T2 | `Close` beginning while a handler runs ends its context; `Close` returns nil after it returns | `TestRequestHandlerContextEndsWhenCloseBegins` | context stays live until the bound |
| T3 | Same, after `Unsubscribe` while the handler still runs | `TestRequestHandlerContextEndsWhenCloseBeginsAfterUnsubscribe` | as T2 |
| T4 | A Start context ending during request registration fails graph-ingest `Start` and holds nothing | existing T-G4 | none (guards D2) |
| T5 | After the broker restarts with `WithMaxReconnects(0)`: `GetConnection().IsClosed()`, `Status` Disconnected, `Connect` matches `errs.ErrAlreadyStarted` with the D3 text | extend `TestIntegration_ReconnectionIsRedial` | text says "already connected" |

T1-T3 use a real connection, so they do not run inside `synctest` (#156).

## Mutants

Task 2.3.

- M1: `requestCallback` gets the call's context again. Expected detector: T1.
- M2: the handler root is not cancelled when `Close` begins. Expected detectors: T2, T3.
- M3: the root is cancelled on `Close` only while delivery has not ended. Expected detector: T3.
- M4: `:467` says "already connected" again. Expected detector: T5.
- M5: `ErrAlreadyStarted` is dropped from `:467`. Expected detectors: T5 and `client_lifecycle_test.go:319`.
- M6: graph-ingest passes `runCtx` to the registrations again (D2). Expected detector: T-G4.

## Spec delta

File: `openspec/changes/natsclient-handler-root/specs/transport-client/spec.md`.

- RENAMED: "A consumer's setup context does not parent its handlers" becomes "A setup context does not parent its
  handlers".
- MODIFIED "A setup context does not parent its handlers": `ConsumeStreamWithConfig`,
  `ConsumeInternalStreamWithConfig` and `SubscribeForRequests` SHALL use their context for setup only and SHALL NOT
  retain it; a write the server is not reading holds the setup as "A write the server is not reading holds the
  connection" states. Each handler's context SHALL descend from a context the client owns for that consumer or
  subscription, which the client SHALL cancel when `Close` begins, also when delivery has already ended; `Close` SHALL
  still join every handler invocation (owner ruling, #9 comment 5994720412 item 2). `ConsumeStreamWithConfigContexts`
  keeps the caller's `handlerCtx` as the handlers' parent. A `SubscribeForRequests` handler's context carries the trace
  from the request's headers when it has one and no value of the call's context. `Subscribe` is not covered by this
  requirement. Scenarios: a handler after the setup deadline; `Close` begins while a handler runs; `Close` begins after
  the subscription ended while a handler runs.
- MODIFIED "Subscribe returns after the server has read the SUB": the current text unchanged except the clause at
  `:268-269`, which becomes "a core subscription holds no claim, a `Subscribe` handler's context descends from the
  call's own context and a `SubscribeForRequests` handler's from a context the client owns ("A setup context does not
  parent its handlers"),".

If `RENAMED` trips `task spec:check`, the fallback is an ADDED requirement "A request subscription's setup context does
not parent its handlers". The proposal's Capabilities section names `transport-client`, and `skip_specs` is removed.

## Residual risk

Declared under #185.

- A caller that cancelled its subscribe context to stop in-flight handlers must now drain or unsubscribe. graph-ingest
  already drains first (`component.go:1139-1147`); the pin adopters change when they are ported.
- A handler running after a failed registration keeps a live context until `Close` begins or its request timeout ends.
  graph-ingest refuses such a handler at entry (`component.go:1059-1074`).
- A Start context ending at the moment the last PONG arrives may let `Start` commit: the round trip's select picks one
  at random, and neither ended first.
- A request without trace headers no longer inherits a trace from the subscribe context.
- With a finite `WithMaxReconnects`, graph-ingest's `Start` on a closed connection waits in `WaitForConnection` until
  its context ends (`component.go:991-1001`). No production code sets a finite limit.
- `Subscribe` keeps the old shape; filed as its own issue, linked from #377 without a closing keyword.

## Merge order

No open PR overlaps: #376 and #374 touch no natsclient, graph-ingest or transport-client file. Either order works; the
later one merges `origin/main`.
