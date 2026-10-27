# Design: setup-04a-04-graph-roots-objectstore-component

Base `b3b9031`. Inventory: `inventory.md` (`INVENTORY PASS`, findings M1, L1-L4 folded in). Change 4, PR 5b of 9. Pin
files: `pin:storage/objectstore/component.go`, `register.go`, `stored_message.go`. Governing records: pin ADR-063 (in
`docs/adr/`), ADR-055, ADR-103, ADR-094, ADR-095; SemEngine's ADR-108 (shell) and ADR-113 (Start context), settled and
applied. Draft PR #437.

## D1. Unit of work

One pull request, one OpenSpec change: 1,497 pin lines carried (inventory 10), under the bound; no waiver (ruling
6386556850, item 1). About 600 pin lines do not enter (D3, D6, D8). Order: after #432 (merged). #436 (graph-ingest test
fields, #301) has no file overlap; either may merge first; if #436 lands a fault-injection helper first, D8 uses it.

## D2. Lifecycle on the shell (ADR-108, ADR-113; #196)

The Component holds a `component.Lifecycle` and answers `Initialize`, `Start`, `Stop`, `Health`, `DataFlow` through it,
replacing the pin's nine fields and 250 lines (S23, `pin:…/component.go:47-55`, `:432-682`). `start` opens the Store
(`NewStoreWithConfigAndMetrics` with `InstanceName "objectstore"` and the Component's logger) and binds each input:
core NATS through `natsclient.Client.Subscribe`, JetStream through `ConsumeStreamWithConfig` after `waitForStream`.
`cleanup` drains every bound handle under Stop's context and tolerates handles never acquired. The Store has no `Close`
(ruling 6446982969), so cleanup closes nothing else.

The Component creates no run context and keeps no cancel. Premise: it has no continuing work of its own; its handlers
run under contexts natsclient owns, not the setup context (`natsclient/stream.go:431-438`: "ctx is used for the setup
only and is not retained"; `natsclient/client.go:1047-1053`, `clientOwnedHandlerContext`). That removes G7 (the pin's
`runCtx` from `context.WithCancel(ctx)`, `pin:…/component.go:450`, which stopped both lanes; inventory probe 2) without
a new root. `Health` reports no stuck state; `DataFlow` counts each delivery (`CountInput`) and each refusal or failure
(`CountError`), replacing S24's zeros and S29's unread counters. Not carried: `IsStarted`, `messagesReceived`,
`messagesStored`, `enabled`, `startStoreError` (the shell returns the Store's fatal error as given, `errs.IsFatal`
holds).

## D3. What the write lane stores (ADR-063; N1)

Options. **A (recommended):** store only `ContentStorable` messages, through `StoreContent`; refuse every other message.
**B:** keep the pin's raw lane (store the wire bytes under a `DefaultKeyGenerator` key) and emit its reference as the
pin does; that reference opens to a JSON message, against ADR-063's read contract (`pin:docs/adr/063-…:278-285`), and
graph-embedding embeds the JSON (`pin:graph/embedding/worker.go:1069-1075`). **C:** keep the raw lane but emit no
reference for it: the stored bytes are then addressable by nobody (the `events` port, their only other announcement,
has no reader, D6). **D:** do nothing (port as is): B plus the S19-S29 findings.

Premises for A: no consumer publishes to `storage.objectstore.write` (non-test go/json/yaml of the four trees: empty,
inventory 4); the pin's own port description says the `stored` output is "StoredMessage output for ContentStorable
pattern" (`pin:…/config.go:93`); every `ContentStorable` is a `graph.Graphable` (`message/storable.go:66-74`,
`graph/graphable.go:54-60`: same two methods), so the pin's "ContentStorable not Graphable" skip
(`pin:…/component.go:776-781`) is unreachable and goes. A drops `Store.Store`, `storeWithKeySource`,
`DefaultKeyGenerator` and its test seam (S12, S13, N13) with the lane. Owner question 1.

Under A the lane decodes once with `message.NewDecoder(deps.PayloadRegistry)`; a message that does not decode, or whose
payload is not `ContentStorable`, is refused as invalid input naming the payload type; a payload `StoreContent` refuses
(entity ID, non-body content, ruling 6447702631) is refused with that error. Then, when a `stored` output is
configured, it publishes `StoredMessage{entity, triples, reference}` (JetStream output: `PublishToStream`; core:
`Publish`). A publish failure is transient (`pin:…/component.go:821-831`).

## D4. Refusals are loud on both lanes (ruling 6447702631; N11, S25, S26)

JetStream input: the pin's settlement table stays (`pin:…/component.go:1022-1033`: nil→Ack, ended context→NAK 5 s,
invalid→Term, other→NAK 30 s); every non-Ack also logs at Error and calls `CountError`. Core-NATS input: no settlement
exists, so a refused or failed write is logged at Error and counted, and the spec states the lane is at-most-once. The
default input stays core NATS (`pin:…/config.go:78`; both consumers use default ports). Alternatives: refuse core-NATS
inputs at construction (JetStream only), which changes every consumer's default config; or a default JetStream input,
whose stream `config` derives from the subject (`config/streams.go:58-65`). Owner question 2. Adopting
`natsclient.SettleDeliveryWithRetry` (N5) is optional and not done: the table is tested and the delays differ.

## D5. Start context (ADR-113), proved by behaviour (N12)

The suite (`lifecycletest.Run`) runs with an `Observe` listing bound handles, and `TestStartContextEndKeepsBothLanesWriting`
publishes on a core-NATS and a JetStream input after Start's context ended and requires both bodies stored: the pin's
field-level suite passed while both lanes were dead (inventory probe 2).

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
registry's JSON fallback covers it). `storage.KeyGenerator` and `storage.MetadataExtractor` (`storage/storage.go:102-131`)
and their `storage/doc.go` examples leave `storage`: with D3's A and S20 nothing can read them (N7). Docs say only what
ships (N2): no cache, no versions, no events, no pluggable keys.

## D7. Instance name (ADR-063 open question; S27)

Options. **A (recommended, do nothing):** keep the one name `"objectstore"`, stamped in every reference and registered
by `ProvidedStores`. The factory receives no instance name (`component/registry.go:21`); semsource depends on the
literal (`semsource:graph/bodystore.go:14`); a second Component in one process is refused at Start by
`registerProvidedStores` (`service/component_manager.go:486`), a boot error, never a silent mismatch. **B:** a
`storage_instance` config key defaulting to `"objectstore"`: small and additive, but no consumer runs two Components,
so it is surface without a reader. **C:** pass the instance name to every factory: changes `component.Factory` and all
factories. Owner question 3 (it closes ADR-063's open question; the port note records the answer).

## D8. Tests without back doors (S22; #195; same class as #301/#436)

No func fields on `Component`. Integration tests use `natsfixture` on the package's shared server (#195): failure is
produced by the real server (a missing input stream with a short Start context; a `stored` JetStream output whose
stream is deleted, for the publish-failure NAK; a payload `StoreContent` refuses). The settlement table and config
resolution are unit tests over pure functions. The pin's `lifecycle_owner_test.go` (S22 user) and the tests needing
`payloadbuiltins` or `examples/processors/document` are rewritten on a local `ContentStorable` test type through
`payloadfixture`; dropped tests are named in `notes/2.1.md`. Pin `time.Sleep`s (inventory probe 4) do not port.

## D9. Gates and records

- #422: `storage/objectstore/component.go` joins `policyEntryPointAllowlist`
  (`internal/harness/contract/consumerpolicy_test.go:40-42`) (N10).
- Ledger: `storage/objectstore` contract gains the Component's adaptations and proving tests; `storage` row notes the
  two interfaces removed; ADR-063 port note records D7's answer.
- `docs/repository-map.md` row updated; `storage/objectstore` stays in `cover-check.sh` at 80%.
- Generated checks: the lane's law is a table (message kind × lane → outcome), each cell an example; no interacting
  history, so examples suffice (`docs/testing.md:417-430`).

## D10. Issues

| Issue | Disposition |
| --- | --- |
| #33 | retired, `Closes #33`: `stored_message.go`'s doc names `RegisterPayloads`' caller as the composition root (task 3.1) |
| #195 | advanced (Component's sites); stays open for other packages |
| #196 | advanced (shell adopted, D2); stays open for other packages |
| #280 | untouched (item 3 is `pkg/lifecycle`) |
| #281 | untouched (defect in `component/schema_tags.go`) |

Filed by this design: none.

## Owner questions

1. Should the Component store only `ContentStorable` bodies and refuse other messages (recommended, D3 A), or keep the
   pin's whole-message store whose reference opens to JSON?
2. On a core-NATS input, is "refuse loudly" an Error log plus a counted error with the lane declared at-most-once
   (recommended, D4), or must inputs be JetStream so a refusal is terminated?
3. Keep one store name, `"objectstore"`, per process, a second Component refused at boot (recommended, D7 A), closing
   ADR-063's multi-instance question?
