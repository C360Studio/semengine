# Inventory: natsclient-handler-root

Base `c499381` (code identical to `origin/main` `05d6ec6` plus the claim commit). Every line reference is at that base.

## 1. Claimed gap

`natsclient/request.go:327-329` passes the call's context into the handler builder
(`c.subscribeOwned(ctx, "SubscribeForRequests", subject, func(conn *nats.Conn) nats.MsgHandler { return
c.requestCallback(ctx, conn, subject, handler) })`). The handler context starts as `msgCtx := ctx` (`:342`) and is
wrapped with `context.WithTimeout(msgCtx, c.requestHandlerTimeout)` (`:351`). Every handler descends from the subscribe
call's context. Confirmed.

## 2. Every place that decides a handler's parent context

- `ConsumeStreamWithConfig`: client-owned root, `stream.go:445`; constructor `clientOwnedHandlerContext`,
  `stream.go:460-461`.
- `ConsumeInternalStreamWithConfig`: the same, `stream.go:564`.
- `ConsumeStreamWithConfigContexts`: the caller's `handlerCtx` (`transport-client` spec `:178`).
- `ownConsumer` cancels that root when `Close` begins, also after delivery has ended (`owned_delivery.go:124-170`).
- The metrics poller has its own root, `client.go:608`.
- `Subscribe` has the same shape: `client.go:1035-1038` wraps the call's own context in
  `context.WithTimeout(ctx, 30*time.Second)`. Its one production caller is `service/message_logger.go:437`, which passes
  `runCtx`. The owner ruling (#9 comment 5994720412, item 2) covers "any other function with the same shape"; this
  change does not fix `Subscribe`; it is filed as #378 (bug, status:blocked). Both go through `subscribeOwned`
  (`client.go:1060`).

## 3. Adjacent claims

- Spec: `openspec/specs/transport-client/spec.md:172-190`, "A consumer's setup context does not parent its handlers";
  `:266-270` in "Subscribe returns after the server has read the SUB" ("its handlers' context descends from the call's
  own context").
- ADRs: ADR-112 `:203` cites `client.go:355-356`; the reconnect defaults are at `client.go:308-309`. ADR-113 is
  settled and applied.
- Open PRs:
  - #376 (service PR 5, draft): three files under
    `openspec/changes/setup-04a-03-boot-service-storage-observability/`. Its pin sources
    (`service/storage_observability*.go` at `8b99efe9`) call neither `Subscribe` nor `SubscribeForRequests`. No overlap.
  - #374 (status tracker, draft, 15 files): no natsclient, graph-ingest or transport-client file. No overlap.
  - #358 (Dependabot prometheus bump): no overlap.
  - Merge order: any; the later PR merges `origin/main`. #374 edits `docs/repository-map.md` and `AGENTS.md`; if this
    change's archive adds a `docs/repository-map.md` row, the two overlap on that file only.
- Open issues naming natsclient:
  - #311 and #317 are retired by this change.
  - #83 stays (non-goal; not about handler contexts).
  - #301 stays: T-G4 and the failed-start tests still use graph-ingest's test-only `subscribeRequests` field
    (`component.go:425-431`). This change adds a second T-G4 row (cancelling at the first mutation registration)
    that reads the same field; it adds no field.
  - #378 (`Subscribe` parents handlers on its call context) stays: filed from this inventory; not fixed here.
  - #195, #141, #68, #162, #24, #300, #134, #216, #196 and #281 name natsclient symbols this change does not touch;
    they stay. #314, #280, #60 and #103 use the word, not the package.

## 4. Who it is for

No new exported symbol; the signature is unchanged. Only the handler context's lifetime changes.

## 5. Problem shape

Setup context versus handler root. The closest existing instance is `ConsumeStreamWithConfig` with `ownConsumer`; this
change adopts it.

## 6. Pin ADRs

The change carries no package from the pin.

## Callers of `SubscribeForRequests`

Measured with `gopls references request.go:318:18`, with and without `-tags=integration`.

- The only production caller is `processor/graph-ingest/component.go:1064` (`subscribeForRequests`), reached from
  `query.go:50` and `mutation_runtime.go:25`. Both get `runCtx` (`component.go:1024`, `:1027`), built at `:982` as
  `context.WithCancel(context.WithoutCancel(ctx))`.
- The check at `component.go:1030-1035` (`if err := ctx.Err(); err != nil`) exists only because the request handlers
  register under the run context.
- `cleanup` drains the subscriptions (`:1139-1143`) before `c.cancel()` (`:1145-1147`), so the run context never
  cancels a request handler during Stop today.
- Test callers: natsclient (about 25 calls), `pkg/lifecycle/manager_integration_test.go:103,157,180,205`, graph-ingest
  `failed_start_callbacks_integration_test.go:158` and `lifecycle_integration_test.go:459,520`. None depends on the
  handler descending from the subscribe context.
- No production code reads a context value from a handler's context except the trace (`TraceContextFromContext`).
- At the pin, the future adopters `processor/agentic-loop/component.go:636`, `processor/agentic-tools/component.go:262`
  and `processor/graph-query/component.go:41` (an interface) pass `runCtx` or an interface context.

## #317 facts

- The `Connect` doc (`client.go:444-445`) says it refuses "a client that is already connected"; the refusal at `:467`
  says "already connected" with `errs.ErrAlreadyStarted`.
- `:547` is a losing concurrent `Connect`; the winner's connection is live, so its text is accurate.
- When nats.go closes the connection, `handleClosed` (`:1815-1819`) writes `StatusDisconnected` and leaves `m.conn`
  installed; `WaitForConnection` (`:378-393`) then waits until its context ends.
- Production never sets a finite limit: `maxReconnects: -1` at `:308`.
- graph-ingest tolerates `ErrAlreadyStarted` at `component.go:991`, so the sentinel stays.
- nats.go v1.54.0 closes the connection when no servers are left (`nats.go:3591-3597`); read in the code, measured only
  by T5 in the design.

## Adopter seam

Today an adopter of `SubscribeForRequests` must pass its run context and re-check its Start context; passing the Start
context instead gives every later handler an ended context, reported nowhere but the doc. After the change the adopter
needs the one fact consumers already need: Stop drains or unsubscribes the subscription, which joins the handler.

## Library-behaviour statements

New: "nats.go closes the connection after a finite MaxReconnects runs out", backed by the extended
`TestIntegration_ReconnectionIsRedial` at `WithMaxReconnects(0)`; above 0 it is read, not measured. Every other
statement (round-trip bounds, held writes P13 and P17, the closed handler firing after the delivery goroutine exits) is
cited unchanged from existing text.
