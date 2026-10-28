# Design: setup-04a-04-graph-roots-objectstore-component

Base `b3b9031`. Inventory: `inventory.md` (`INVENTORY PASS`, findings M1, L1-L4 folded in). Change 4, PR 5b of 9. Pin
files: `pin:storage/objectstore/component.go`, `register.go`, `stored_message.go`. Governing records: pin ADR-063 (in
`docs/adr/`), ADR-055, ADR-103, ADR-094, ADR-095; SemEngine's ADR-108 (shell) and ADR-113 (Start context), settled and
applied. Draft PR #437.

## D1. Unit of work

One pull request, one OpenSpec change: 1,497 pin lines carried (inventory 10), under the bound; no waiver (ruling
6386556850, item 1). About 600 pin lines do not enter (D3, D6, D8). Order: after #432 (merged). #436 (graph-ingest test
fields, #301) touches other code; both PRs edit `docs/admission-ledger.yaml` (different rows), so either order is fine;
if #436 lands a fault-injection helper first, D8 uses it.

## D2. Lifecycle on the shell (ADR-108, ADR-113; #196)

The Component holds a `component.Lifecycle` and answers `Initialize`, `Start`, `Stop`, `Health`, `DataFlow` through it,
replacing the pin's nine fields and 250 lines (S23, `pin:…/component.go:47-55`, `:432-682`). `start` opens the Store
(`NewStoreWithConfigAndMetrics` with `InstanceName "objectstore"` and the Component's logger) and binds each input:
core NATS through `natsclient.Client.Subscribe`, JetStream through `ConsumeStreamWithConfig` after `waitForStream`.
`cleanup` drains every bound handle under Stop's context and tolerates handles never acquired. The Store has no `Close`
(ruling 6446982969), so cleanup closes nothing else.

Handlers run under contexts natsclient owns, not the setup context (`natsclient/stream.go:431-438`: "ctx is used for
the setup only and is not retained"; `natsclient/client.go:1047-1053`, `clientOwnedHandlerContext`). That alone
removes G7 (the pin's `runCtx` from `context.WithCancel(ctx)`, `pin:…/component.go:450`, which stopped both lanes;
inventory probe 2).

**Lost input (#216's ruling; mirrors graph-ingest).** The owner ruled that a restart is the recovery: the component
goes unhealthy, `/readyz` fails, the supervisor restarts (#216 comment 6443457897); the accepted Store design routed
the ruling here (archived `…objectstore-store/design.md:311`). A deleted consumer ends the consume loop (`Closed()`
fires) when the server's deletion notice answers a pull waiting at the server (nats.go v1.54
`jetstream/pull.go:740`, read; observed once, not promised, `notes/2.2.md` item 9: deleted before the first pull, the loop
kept running
30 s later, and the lookup alone reports the loss), but a broker restart that loses a memory-backed stream does not:
nats.go re-pulls forever (archived
`fix-216-consumer-rebind/inventory.md:34`, `:99`, `:124`). graph-ingest catches both with its status tick
(`processor/graph-ingest/readiness.go:150-163`, every `readiness.DefaultHeartbeat`, 5 s,
`graph/readiness/watcher.go:71`), which for each bound JetStream consumer (a) records the binding lost, for good, when
its `Closed()` fired while the shell runs (`readiness.go:52-90`, read loop-end first, running second), and (b) looks the
consumer up by name, a failed lookup being a stuck state until a lookup succeeds (`:200-226`, `:289-301`). The
Component mirrors exactly those two observations and `Health`'s use of them (`processor/graph-ingest/component.go:398-401`:
`lifecycle.Health(…, fault, c.lostBindingFault())`). It does not mirror the readiness envelope the same tick publishes:
the Component is no index producer and nothing reads a readiness key from it (ADR-109). The stuck text names the input
and, for a lost binding, says a process restart is the recovery. Core-NATS subscriptions have no consumer to lose and
are not observed; a Component with no JetStream input runs no tick.
Clearing the stuck state after a later lookup succeeds is stated, not tested, as in graph-ingest
(`processor/graph-ingest/health_integration_test.go:55-58` asserts unhealthy only): a mutant that never clears fails
closed into the ruled restart, and one that always clears fails the memory-loss test.

The tick is continuing work, so the Component has a run context, as graph-ingest does (`component.go:448`:
`context.WithCancel(context.WithoutCancel(ctx))`): `start` creates it, keeps only its cancel, and starts the tick under
it as the last step; `cleanup` drains the inputs, then cancels and waits for the tick under Stop's context
(`component.go:600-610` is the order). This is ADR-113's one permitted root (`docs/adr/113-start-context-bounds-start.md:62-64`);
the `background-work` rule's join is `Stop`'s. Proof: the suite's `Observe` lists the tick until it has returned, so
`requireNothingRetained` after `Stop` proves the join, and `TestStartContextEndKeepsBothLanesWriting` proves the tick
and lanes outlive the Start context. `Health` reads stored observations and never waits on NATS.

`DataFlow` counts each delivery
(`CountInput`) and each refusal or failure (`CountError`), replacing S24's zeros and S29's unread counters. Not carried:
`IsStarted`, `messagesReceived`, `messagesStored`, `enabled`, `startStoreError` (the shell returns the Store's fatal
error as given, `errs.IsFatal` holds).

## D3. What the write lane stores (ADR-063; N1)

Options. **A (recommended):** store only `ContentStorable` messages, through `StoreContent`; refuse every other message.
**B:** keep the pin's raw lane (store the wire bytes under a `DefaultKeyGenerator` key) and emit its reference as the
pin does; that reference opens to a JSON message, against ADR-063's read contract (`pin:docs/adr/063-…:278-285`), and
graph-embedding embeds the JSON (`pin:graph/embedding/worker.go:1069-1075`). **C:** keep the raw lane but emit no
reference for it: the stored bytes are then addressable by nobody (the `events` port, their only other announcement,
has no reader, D6). **D:** do nothing (port as is): B plus the S19-S29 findings.

Premises for A: no consumer publishes to `storage.objectstore.write` (non-test go/json/yaml of the four trees: empty,
inventory 4); the pin's own port description says the `stored` output is "StoredMessage output for ContentStorable
pattern" (`pin:…/config.go:93`); every `ContentStorable` is a `graph.Graphable` (`message/content_storable.go:47-48`
embeds `Storable`; `message/storable.go:66-74`, `graph/graphable.go:54-60`: same two methods), so the pin's
"ContentStorable not Graphable" skip (`pin:…/component.go:776-781`) is unreachable and goes. A drops `Store.Store`,
`storeWithKeySource`, `DefaultKeyGenerator` and its test seam (S12, S13, N13) with the lane. Owner ruling 1: A.

Under A the lane decodes once with `message.NewDecoder(deps.PayloadRegistry)`; a message that does not decode, or whose
payload is not `ContentStorable`, is refused as invalid input naming the payload type; a payload `StoreContent` refuses
(entity ID, non-body content, ruling 6447702631) is refused with that error. Then it publishes `StoredMessage{entity,
triples, reference}` (JetStream output: `PublishToStream`; core: `Publish`). A publish failure is transient
(`pin:…/component.go:821-831`). Because a stored body is useful only through its reference, a configuration with an
input and no `stored` output is refused at construction naming the missing output: otherwise every body would be written
and announced to nobody, the orphan option C has. A Component with no inputs (offering its Store only) needs no `stored`
output; the defaults have both `write` and `stored`, which is what both consumers run (inventory 4).

## D4. Refusals are loud on both lanes (ruling 6447702631; N11, S25, S26)

JetStream input: the pin's settlement table stays (`pin:…/component.go:1022-1033`: nil→Ack, ended context→NAK 5 s,
invalid→Term, other→NAK 30 s); every non-Ack also logs at Error and calls `CountError`. Core-NATS input: no settlement
exists, so a refused or failed write is logged at Error and counted, and the spec states the lane is at-most-once. The
default input stays core NATS (`pin:…/config.go:78`; both consumers use default ports). Alternatives: refuse core-NATS
inputs at construction (JetStream only), which changes every consumer's default config; or a default JetStream input,
whose stream `config` derives from the subject (`config/streams.go:58-65`). Owner ruling 2: as recommended. Adopting
`natsclient.SettleDeliveryWithRetry` (N5) is optional and not done: the table is tested and the delays differ.

Delivery on a JetStream input is at-least-once. A delivery is acked only after the body is stored and its reference
published; a failed publish NAKs it, and the redelivery stores the body again under a new key, because `StoreContent`'s
key carries a per-call nonce (`storage/objectstore/content.go:115-119`). So a redelivery after a failed reference
publish leaves one extra, unreferenced object; the spec says so. The pin's comment that a ContentStorable redelivery
"overwrites rather than duplicates" (`pin:…/component.go:1046-1051`) is false here and is not carried. A delivery NAKed
`MaxDeliver` times (3 by default, `component/port_jetstream.go:112`, `:130`) is not delivered again and this Component
reports nothing about it; the spec says so and #36 owns making such a message visible. That the server stops
redelivering at `MaxDeliver` is the pin's statement, not measured here (inventory E1).

## D5. Start context (ADR-113), proved by behaviour (N12)

The suite (`lifecycletest.Run`) runs with an `Observe` listing bound handles, and
`TestStartContextEndKeepsBothLanesWriting` publishes on a core-NATS and a JetStream input after Start's context ended
and requires both bodies stored: the pin's field-level suite passed while both lanes were dead (inventory probe 2).

## D6. Surface (surface audit; S19-S21, S29, N2, N3, N7)

Exported after this change: `Component` (its interface methods and `ProvidedStores`), `Register`, `RegisterPayloads`,
`StoredMessage`, `NewStoredMessage`. Readers: semteams `runtimecatalog/components.go:57`, `payloads.go:32`; semsource
through its aggregator (`cmd/semsource/run.go:295`, `:311`, `:917-922`); graph-ingest reads `StoredMessage` as
`message.Storable` (`processor/graph-ingest/ingest.go:224-226`). The factory and `DeclarePorts` are unexported, reached
through `Register` (`component/registry.go:127`).

Configuration is an unexported `componentConfig{BucketName, Ports}` decoded strictly, as graph-index does
(`processor/graph-index/component.go:103-115`; `component-registration` spec "Unknown configuration keys are refused"):
`data_cache` is refused naming it (S19, S21). Defaults: bucket `MESSAGES`; inputs `write` (core NATS,
`storage.objectstore.write`); outputs `stored` (`storage.objectstore.stored`, `storage.stored.v1`) plus the derived
`store-provide`. An output other than `stored` is refused naming it, since nothing would publish on it. Not carried, no
reader at the pin or in a consumer (inventory 4): `events` port, `Event`, `publishEvent` (N3, N4); `Config.Ports`,
`KeyGenerator`, `MetadataExtractor`, `DefaultConfig` (S20); `IsStarted` (S29); `buildStoredMessage` (S31: its only
caller is `payloadregistry.Registry.Build`, which has no production caller, `git grep -n '\.Build('`: empty; the
registry's JSON fallback covers it). `storage.KeyGenerator` and `storage.MetadataExtractor`
(`storage/storage.go:102-131`) and their `storage/doc.go` examples leave `storage`: with D3's A and S20 nothing can read
them (N7). Docs say only what ships (N2): no cache, no versions, no events, no pluggable keys.

## D7. Instance name (ADR-063 open question; S27)

Options. **A (recommended, do nothing):** keep the one name `"objectstore"`, stamped in every reference and registered
by `ProvidedStores`. The factory receives no instance name (`component/registry.go:21`); semsource depends on the
literal (`semsource:graph/bodystore.go:14`), semconnect names it (`semconnect:gateway/cs-api/graph_mutations.go:23`),
and semteams maps the instance key to its bucket by name (`semteams:cmd/semteams/main.go:516-535`); a second Component
in one process is refused at Start by `registerProvidedStores` (`service/component_manager.go:486`), a boot error, never
a silent mismatch. **B:** a `storage_instance` config key defaulting to `"objectstore"`: small and additive, but no
consumer runs two Components, so it is surface without a reader. **C:** pass the instance name to every factory: changes
`component.Factory` and all factories. Owner ruling 3: A (it closes ADR-063's open question; the port note records the
answer).

## D8. Tests without back doors (S22; #195; same class as #301/#436)

No func fields on `Component`. Integration tests use `natsfixture` on the package's shared server (#195): failure is
produced by the real server (a missing input stream with a short Start context; a `stored` JetStream output whose
stream is deleted, for the publish-failure NAK; a payload `StoreContent` refuses). The settlement table and config
resolution are unit tests over pure functions. The pin's `lifecycle_owner_test.go` (S22 user) and the tests needing
`payloadbuiltins` or `examples/processors/document` are rewritten on a local `ContentStorable` test type through
`payloadfixture`; dropped tests are named in `notes/2.1.md`. Pin `time.Sleep`s (inventory probe 4) do not port.

The lifecycle suite does not judge a failed `Start` whose own cleanup also fails (`openspec/specs/lifecycle-suite/spec.md:214`
leaves it to a component-owned test). The trigger is real: an input that is still finishing a write can outlast
rollback's five-second budget (`pkg/lifecyclecleanup/lifecyclecleanup.go:29`), and `Subscription.Drain` returns the
context error before the native close and allows a later join (`natsclient/client.go:962`).
`TestFailedCleanupRetainsInputsForLaterStop` proves it as graph-ingest's
`TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop` does (`processor/graph-ingest/lifecycle_owner_test.go:121`):
the test calls `lifecycle.Start` with a start function that fails after binding and a rollback that releases under an
already-ended context. It asserts that `Start` returns both errors, that `Observe` still lists the inputs, and that a
later `Stop` with a fresh finite context returns nil and leaves nothing. It needs no production hook.

## D9. Gates and records

- #422: `storage/objectstore/component.go` joins `policyEntryPointAllowlist`
  (`internal/harness/contract/consumerpolicy_test.go:40-42`) (N10).
- Ledger: `storage/objectstore` contract gains the Component's adaptations and proving tests; `storage` row notes the
  two interfaces removed; ADR-063 port note records D7's answer.
- `docs/repository-map.md` row updated; `storage/objectstore` stays in `cover-check.sh` at 80%.
- Generated checks (`docs/testing.md:417-430`): retry and redelivery make a history, so the choice is recorded. Each
  delivery's outcome is a function of (lane, message kind, store result, publish result), the table in D3-D4; the only
  state one delivery leaves for the next is the objects stored and the delivery count. Examples suffice: the table is
  `TestWriteLaneRefusals` and `TestClassifyWriteDisposition` cell by cell, and the one cross-delivery history,
  publish fails then succeeds on redelivery, is `TestStoredEmitFailureNaksThenRedeliveryStores` (asserts one acked
  delivery, two objects, exactly one referenced). No generated history is used: there is no second interacting input
  and no ordering the examples leave out.
- Guidance (inventory E3): the pin's developer and reviewer sections that govern this Component, § Payload registry
  (`pin:.agents/contracts/semstreams-developer.md:234-241`) and § State ownership and component wiring (`:243-251`),
  are already carried (`.agents/contracts/semengine-developer.md:293-303`, `:305`; reviewer `:306-315`); the storage
  bullet came with the Store (`:287-291`). No new text carries. `StoredMessage`'s explicit `MarshalJSON` and
  `UnmarshalJSON` (`pin:…/stored_message.go:163-197`) stay instead of the alias form the payload rule names: its
  fields are unexported, and an alias needs exported fields, which would make a received `StoredMessage` mutable.

## D11. Waiting for one consumer's handlers (`natsclient`; owner ruling on #443)

Owner ruling, 2026-10-28 (PR #437 comment 6467701324, verbatim on #443): add a small exported `natsclient` method
that waits, bounded by the caller's context, until one consumer's handler invocations have all returned; it exposes the
completion `natsclient` already owns, keeps the exact native handle (#9 comment 5980296112), never closes the shared
client, and replaces the Component's `handlerJoin`. One-package exception for this method only (#159).

**Inventory.** Each consumer the client starts is tracked by a `consumerOwnership`
(`natsclient/owned_delivery.go:109-113`): `ownConsumer` (`:124-168`) admits a client-owned goroutine before native
`Consume`, which waits for the handle's `Closed()` or Close beginning, records the end (`ownedDelivery.end`, `:74-84`),
waits for `ownedDelivery.done` (closed once the end is recorded and no invocation runs, `:30-32`, `:87-89`), then
releases the claim and closes `released`; a delivery after the end is refused, logged and counted (`:41-48`, `:98-104`;
`transport-client` spec "A delivery after the recorded end is refused, and counted", `:146`). `commitConsumer`
(`:181-199`) returns the native handle unchanged. All three consumer entry points reach it:
`ConsumeStreamWithConfig` and `ConsumeStreamWithConfigContexts` through `consumePortStreamWithConfigContexts`
(`natsclient/stream.go:678`, `:713`), `ConsumeInternalStreamWithConfig` directly (`:567`, `:599`). Nothing keeps an
ownership after setup returns: no table maps a handle to it (`git grep -n -e 'ownConsumer(' -e 'commitConsumer('
-- natsclient`: those three sites only), so it cannot be looked up by handle today. At nats.go v1.54.0 the handle is
a `*pullSubscription` (`jetstream/pull.go:228`, returned at `:425`), a pointer; that is the library's current shape,
not its promise. Records the method joins: `transport-client` spec (`:146`, `:205`, `:235`, `:533`); ADR-112 § 4
"Ownership" (`docs/adr/112-natsclient-contract.md:192-197`: "`Close` stops it and joins its handlers").

**Contract.** `func (c *Client) WaitConsumerHandlers(ctx context.Context, handle jetstream.ConsumeContext) error`:

- It waits; it does not stop delivery. Ending delivery stays the caller's (`Drain`/`Stop` on its own handle) or
  `Close`'s. It returns nil once the consumer's delivery has ended, every handler invocation of that consumer has
  returned, **and the consumer's durable claim is released**: it waits on `consumerOwnership.released`
  (`owned_delivery.go:112`; its deferred close, `:135`, runs after `release`, `:164`), not on `ownedDelivery.done`.
  `done` closes before `release` frees the claim and forgets the policy and metrics (`natsclient/stream.go:678-684`), so
  a nil on `done` could let a caller that starts the same durable next meet its own claim. From then on no handler of it
  starts, and a later delivery is refused, logged and counted as today.
- A nil `ctx` is refused as invalid input (spec `:235`). If `ctx` ends first it returns an error matching `ctx.Err()`
  and changes nothing; a later call waits again.
- A `handle` this client did not return from one of the three entry points (nil, another client's, a value nats.go
  never made) is refused at once as invalid input, never waited on and never a panic. A failed setup returns no handle,
  so it has nothing to wait on.
- A handle stays known to its client for the client's life, so a call after its handlers finished, or after `Close`
  returned nil (which joins them, spec `:533`), returns nil at once. Dropping the record at release would make a
  finished handle look foreign. This differs from the client's stated rule for durable claims, which are kept
  "without retaining lifecycle handles" (`natsclient/client.go:73-74`): the record holds a handle only as a lookup key
  for its finished ownership, with no lifecycle authority (nothing stops, drains or closes through it). Its cost is one
  small record per consumer a client ever starts. That matters only for a caller that creates consumers per request or
  rebinds in a loop; there is no such production caller: components start their consumers once per `Start`, and an
  instance starts once (ADR-095), and #216's ruling made restart, not rebinding, the recovery.
- It never closes, drains or reconnects the client and never wraps or replaces the handle. No other `natsclient`
  change.

Its promise rests on the client's own count, not on nats.go: `Closed()` can close while a handler still runs
(`owned_delivery.go:14-18`, citing `jetstream/pull.go:822-837`, and the Component's
`TestStopJoinsHandlerAfterConnectionLoss`, the probe at this version). How long delivery takes to end after a
`Drain` is not promised (#170).

**The Component.** `cleanup` drains each JetStream input, then calls `WaitConsumerHandlers(ctx, handle)` under Stop's
context for each; a context that ends first keeps the input for a later `Stop` (lifecycle-suite, retained handles).
`handlerJoin`, its field and the Component's own late-delivery refusal go (`storage/objectstore/component.go:136-196`,
`:109`, `:423`, `:513-522`, `:603`). `TestStopJoinsHandlerAfterConnectionLoss` stays unchanged and must still pass;
`TestHandlerJoinWaitsForRunningHandlersAndRefusesLaterOnes` is dropped with the type, its behaviour now proven in
`natsclient`.

**Proving tests (natsclient, failing first: the method does not exist):**
`TestWaitConsumerHandlersWaitsForRunningHandler` (a handler held mid-invocation; the handle stopped; the wait does not
return until the handler is released, then nil, and a consumer with the same durable then starts on the same client: the
claim is released), `TestWaitConsumerHandlersAfterConnectionLoss` (broker gone for good while a handler runs: `Closed()`
has fired and the wait still blocks until the handler returns), `TestWaitConsumerHandlersContextEndsFirst` (handle not
stopped, a handler held; the wait's short context ends: error matches `ctx.Err()`; then the handler is released, one
more message is published and the handler must receive it, which proves the wait stopped nothing; then the handle is
stopped and a call with a fresh context returns nil), `TestWaitConsumerHandlersRefusesUnknownHandle` (nil, a foreign
client's handle, a fake non-comparable value: invalid input, no wait, no panic), `TestWaitConsumerHandlersAfterClose`
(nil at once), and `TestWaitConsumerHandlersRefusesNilContext`; the late-delivery refusal stays proven by the existing
`:146` scenario's test. Shown able to fail: each runs red against a stub that returns nil at once, by an assertion and
not only a compile error (the blocking, ctx-error, invalid-input and claim assertions), except `…AfterClose`, shown red
against a stub that forgets a handle at release; and `…ContextEndsFirst` also runs red against a version that stops the
handle before waiting (the extra message is not received). Recorded in `notes/4.1.md`.

**Adopters (reusable primitive).** The objectstore Component (this change); graph-ingest's `Stop` (#441, after this
merges: `processor/graph-ingest` waits on `Closed()` today, the same gap). Recorded on #441; no other caller drains a
consumer and waits for its handlers (`git grep -n 'Closed()' -- '*.go' ':!*_test.go' ':!natsclient'`, to be re-run in
task 4.1).

## D10. Issues

| Issue | Disposition |
| --- | --- |
| #33 | retired, `Closes #33`: `stored_message.go`'s doc names `RegisterPayloads`' caller as the composition root (task 3.1) |
| #195 | advanced (Component's sites); stays open for other packages |
| #196 | advanced (shell adopted, D2); stays open for other packages |
| #280 | untouched (item 3 is `pkg/lifecycle`) |
| #281 | untouched (defect in `component/schema_tags.go`) |
| #216 | closed; its ruling applied: input binding lost is a stuck state (D2), task 2.2 |
| #443 | retired, `Closes #443`: D11, tasks 4.1-4.3 |
| #438 | retired, `Closes #438`: fixed at `8af0674` (`ack_wait` forwarded, `consumer_name` refused) |
| #439 | retired, `Closes #439`: fixed at `8af0674` (Stop joins running handlers after connection loss); D11 moves the join into `natsclient` |
| #441 | stays: graph-ingest adopts D11's method after this merges |
| #36 | stays: a delivery parked after `MaxDeliver` is declared in the spec, not reported (D4); making it visible is #36's re-homed observer, a later change |

Filed by this design: none.

## Owner rulings

The owner answered the three design questions on 2026-10-27 (PR #437 comment 6454878443):

1. Bodies only: the Component stores only `ContentStorable` bodies and refuses other messages (D3 A).
2. Logged and counted is enough: on the default input, which keeps nothing, a refused message is logged, counted, then
   gone (D4).
3. One store name, `"objectstore"`, per process, a second Component refused at boot (D7 A); this closes ADR-063's
   multi-instance question.
