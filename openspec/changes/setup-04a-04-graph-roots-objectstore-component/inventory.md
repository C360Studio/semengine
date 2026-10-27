# Inventory: setup-04a-04-graph-roots-objectstore-component

base: `b3b903172ea50a481398d11bccb4e00a032f5e42` (origin/main after PR #432; the claim commit
`3fba298edb50bba6a67a77312bd4ec155a10dc1a` on `claude/port-objectstore-component` adds no file). Pin paths are SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, written `pin:<path>:<line>`, read from a fresh extract of the pin tarball at
`/tmp/change4-5b-stage/pin/C360Studio-semstreams-8b99efe` (local only); `component.go`, `register.go`,
`stored_message.go`, `config.go`, `store.go`, `doc.go` and `README.md` were checked byte-equal (`cmp`) against
`gh api repos/C360Studio/semstreams/contents/storage/objectstore/<f>?ref=8b99efe9`. Consumer cites are at default-branch
heads of 2026-10-27 from `gh api repos/C360Studio/<repo>/tarball/<sha>` (`docs/inventory-scope.md` rule 4): semsource
`e4febc0d2e23bf33811edc20b9a4159cf5275dc4`, semconnect `d0d06e00bf05a545f30ceea798db1c2b1ee47d4f`, semteams
`27bf6ecbd2fbd9e79fdd1f772f58bb3f062dd524` (moved since the Store inventory's `8e215f59`), semboids
`8c03cc53836ced93a5df7064473c63ff144e64f1`; written `<repo>:<path>:<line>`.

Question: what the objectstore Component half (`pin:storage/objectstore/component.go`, `register.go`,
`stored_message.go`, plus the Store symbols the Store change deferred) is, what it needs, who reads it, and what is
open against it. Repositories read: this tree, the pin, semsource, semconnect, semteams, semboids (symbol level).

Status: round 1 `INVENTORY PASS` (independent review, sha `a5ce9505…`); findings M1, L1-L4 folded in.

Defining artifact: `openspec/changes/archive/2026-10-27-setup-04a-04-graph-roots-objectstore-store/` (proposal "Deferred
to pull request 5b", design D1, D3, D6, D8, D13, inventory sections 7 and 10, `notes/2.1.md:90-99`). Its inventory's
shape sweep (S19-S32, G7, G8) covered the Component's files; this inventory re-reads them and adds findings N1-N12.

## Briefing hypotheses checked

| Hypothesis | Result |
| --- | --- |
| 5b carries `component.go`, `register.go`, `stored_message.go`, about 1,500 pin lines | holds: 1,242 + 33 + 222 = **1,497** (`wc -l`, section 10). The deferred Store symbols add 208 pin lines the Store change already counted |
| 5b carries `Store.Store`, `DefaultKeyGenerator`, `Config.Ports` (merge note #9 comment 6452913874) | holds, but incomplete: the archived proposal's list also has `storeWithKeySource`, `Config.KeyGenerator`, `Config.MetadataExtractor`, `DefaultConfig`, `StoredMessage`, `Register`, `RegisterPayloads` (`proposal.md:31-32`); none is in the tree (section 1) |
| 5b closes #33; last site `stored_message.go:90` | holds: `pin:storage/objectstore/stored_message.go:90` — `// supplied registry. Called from payloadbuiltins.Register at process`; no non-test file in the tree names `payloadbuiltins` (`git grep -n payloadbuiltins -- '*.go'`: test files and the contract guard only) |
| 5b adopts `component.Lifecycle` for #196 | the binding basis is wider than #196: tree ADR-108 names `storage/objectstore/component.go:49` among change 4's shell adopters (`docs/adr/108-processor-shell.md:172`); `pin:…/component.go:49` — `lifecycleUsed  bool` |
| 5b moves the Component's two test-client sites to the fixture for #195 | holds: `pin:…/component_instance_integration_test.go:35`, `:67` — `tc := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())` |
| Owner rulings on PR #432 bind 5b: data cache dropped, `Close` not carried, non-body content refused | holds (comments 6446982969, 6447702631 read in full). 6446982969: "the Component (PR 5b) does not reinstate it"; 6447702631: "The Component (PR 5b) refuses such messages loudly". No other owner-ruling comment on #432 (all 18 comments read; the rest are reviews, answers, the probe and the acceptance 6448182297) |
| Archived design D13: "#36, #216 stay: the Component's write consumer (5b) names both" | **false for #216**: closed 2026-10-27 by PR #431 (`8927d82`, graph-ingest only). #36 is open but does not name objectstore (searched; section 3) |
| Archived design header: "ADR-108, ADR-113 and ADR-095 govern the Component"; Store inventory section 6 calls them pin records | ADR-108 and ADR-113 are **SemEngine's own** records, not the pin's (`ls pin/docs/adr`: 85 files ending at 107); ADR-095 is a pin record already in `docs/adr/` |
| Tier-0 only; nothing with an external provider | holds: non-test imports of the three files are stdlib, `component`, `graph`, `internal/lifecyclecleanup`, `message`, `metric`, `natsclient`, `pkg/errs`, `storage`, `payloadregistry`, `pkg/types`, `nats.go`, `nats.go/jetstream` (`pin:…/component.go:5-29`, `stored_message.go:3-12`, `register.go:3`) |

## Pin probe

go1.26.4 darwin/arm64, Docker 29.9.2, on the pin copy. Probe files (not part of the pin) written into the copy:
`internal/harness/lifecycletest/lifecycletest.go` copied from this tree at base, and
`storage/objectstore/zz_lifecycle_probe_integration_test.go` (package `objectstore`, `//go:build integration`, a pin
`natsclient.NewTestClient` per test; `Observe` reports `cancel`, `store`, `writeSubs`, `writeConsumers`, `started`).

1. **Lifecycle suite against the Component** (`go test -tags integration -count=1 -p 1 -run
   TestPinProbeObjectStoreLifecycleSuite -v ./storage/objectstore/`): **PASS, all eight checks**, twice: default ports
   (core-NATS `write` input) and a JetStream `write` input on stream `PROBE_WRITES`. Must-fail owner: a Component with a
   nil `NATSClient` (Start refuses before acquiring, `pin:…/component.go:445-447`). Promise: none (one-shot).
2. **G7 behaviour probe** (`TestPinProbeStartContextEndJetStreamDelivery`, same file, one run): JetStream input; one
   message published and stored (`List` = 1); then the Start context cancelled; a second message published; after 5 s
   `List` is still **1**. Logged: `stored 5s after Start ctx cancelled and a second publish: 1`. The write lane stops
   storing when the Start context ends while the Component still holds every handle and `started` is true. The suite
   in probe 1 passed because its `Observe` is field-level; `CheckStartContextEndStopsNothing` judges only what `Observe`
   lists (`internal/harness/lifecycletest/lifecycletest.go:189-215`). Mechanism at the pin: `ConsumeStreamWithConfig`
   passes its one context as both setup and handler context (`pin:natsclient/stream.go:432-433` —
   `ctx, ctx, "ConsumeStreamWithConfig", owner, cfg, handler,`), so every later handler gets an ended context, and
   `classifyWriteDisposition` NAKs any failure under an ended context (`pin:…/component.go:1026-1027` — `if ctxErr != nil
   {` `return dispositionNakShutdown`).
   **Default core-NATS lane, same probe shape** (`TestPinProbeStartContextEndCoreDelivery`, default ports, one run):
   stored before cancel 1; 5 s after the Start context was cancelled and a second publish, still **1**, with the log line
   `ERROR Failed to process write message subject=storage.objectstore.write error="Store.Store: store message failed:
   context canceled"`. Mechanism: the Component subscribes with the run context (`pin:…/component.go:547` —
   `writeSub, subscribeErr := subscribe(runCtx, writeSubject, c.handleWriteRequest)`), the pin client derives each
   handler context from it (`pin:natsclient/client.go:835-836` — `msgCtx, cancel := context.WithTimeout(ctx,
   30*time.Second)`), and a failure is logged and dropped (`pin:…/component.go:700-704`): on this lane the message is
   lost, not redelivered.
3. **Unit tests**: `go test -race -count=1 -p 1 -cpu 1 -cover ./storage/objectstore`: ok 1.323 s, coverage 43.3%.
   `go test -count=5 -p 1 -cpu 1 -shuffle=on -v ./storage/objectstore`: ok 0.270 s, `-test.shuffle 1793134254180032000`,
   no FAIL. 38 top-level tests pass per run (`-count=1 -v`, `--- PASS` count). The integration files (six) were not run
   beyond the probes.
4. **Test-text rules** over `pin:storage/objectstore/*_test.go` (14 files, 69 `func Test`): `time.Sleep` 6, all in
   Component tests: `component_ack_integration_test.go:424` — `time.Sleep(250 * time.Millisecond)`, `:492`, `:539`,
   `:605` (100 ms), `:622` (250 ms); `component_write_key_integration_test.go:130` — `time.Sleep(100 * time.Millisecond)`.
   `Skip`/`Skipf`/`SkipNow`: 0. Build tags other than `integration`: 0. `t.Parallel` in an integration-tagged file: 0
   (29 in unit files: `component_ack_test.go` 2, `config_ports_test.go` 10, `entity_id_contract_test.go` 2,
   `retention_test.go` 12, `stored_message_contract_test.go` 3). Test clients outside the fixture: the two #195 sites
   above and `store_integration_test.go:37` (the Store's, already replaced in the tree). Pin test imports this tree does
   not have: `payloadbuiltins.NewTestRegistry` (`component_ack_integration_test.go:450`, `:575`;
   `component_write_key_integration_test.go:99`), `examples/processors/document` (`component_ack_integration_test.go:18`),
   `types` (`:24`), testify in 11 of 14 files. Back-door use: `lifecycle_owner_test.go` sets the S22 func fields 18 times
   (six tests, `:97-356`).

## 1. What exists (the claimed gap)

`storage/objectstore` in the tree (base) has the Store half only: `config.go`, `content.go`, `doc.go`, `metrics.go`,
`retention.go`, `store.go`, `README.md`, tests. Searched the deferred names in the worktree
(`git grep -n -e 'func (s \*Store) Store(' -e DefaultKeyGenerator -e storeWithKeySource -e 'Ports ' -e DefaultConfig
-e NewComponent -e StoredMessage -e 'func Register' -- storage/objectstore`): empty. `Config` is `{BucketName,
InstanceName, Logger}` (`storage/objectstore/config.go:9-22`); `Store` holds `instanceName`, `objects`, `metrics`
(`storage/objectstore/store.go:28-32`) — no key generator, no metadata extractor, no decoder.

What the Component needs, present in the tree: `component.Lifecycle` (`component/lifecycle_type.go:27`, `Start(ctx,
start, cleanup)` `:63`, `Stop` `:89`, `Health(stuck...)` `:100`, `DataFlow` `:128`, `CountInput` `:147`, `CountError`
`:157`); `component.Registry.Register(Registration)` (`component/registry.go:127`; the pin calls `RegisterWithConfig`
with `RegistrationConfig`, `pin:…/register.go:22`); `component.Factory` carries no instance name
(`component/registry.go:21` — `type Factory func(rawConfig json.RawMessage, deps Dependencies) (Discoverable, error)`),
`PortDeclarer` does (`:30`); `component.GetConsumerConfig` (`component/port_jetstream.go:114`); `component.PortConfigFrom`
(`component/ports.go:168`); `component.GenerateConfigSchema` (`component/schema_tags.go:365`);
`component.StoreProvider` (`component/dependencies.go:26`) read by `service/component_manager.go:459`
(`provided := provider.ProvidedStores()`, refusing an instance another component holds, `:486`);
`component.StoreProvidePort` (`component/port_provide.go:13`, non-exclusive, `:22-24`); `natsclient.Client.Subscribe`
(`natsclient/client.go:1046`), `Publish` (`:1212`), `PublishToStream` (`:1293`), `GetConnection` (`:347`),
`ConsumeStreamWithConfig` (`natsclient/stream.go:439`), `ConsumeStreamWithConfigContexts` (`:730`),
`PortConsumerContext` (`:76`), `StreamConsumerConfig` (`:19`); `natsclient.SettleDelivery`/`SettleDeliveryWithRetry`
(`natsclient/delivery_settlement.go:278`, `:290`); `pkg/lifecyclecleanup.RollbackFailedStart`
(`pkg/lifecyclecleanup/lifecyclecleanup.go:36`; pin path `internal/lifecyclecleanup`); `message.NewDecoder`
(`message/decoder.go:33`); `payloadregistry.Registration` with `Builder` (`payloadregistry/registry.go:53`, `:48`);
`storage.KeyGenerator`, `storage.MetadataExtractor` (`storage/storage.go:113`, `:127`).

Gate the Component meets in the tree: the consumer-creation allowlist (#422) lists only
`processor/graph-ingest/consume.go` (`internal/harness/contract/consumerpolicy_test.go:40-42`); a Component calling
`ConsumeStreamWithConfig` or `GetConsumerConfig` from another file fails it.

Docs in the tree that already name 5b symbols: `docs/contract.md:167`, `:176` (`objectstore.Register`), `:186`
(`objectstore.RegisterPayloads`); `storage/doc.go:116`, `:174` (the Component "is not in this repository");
`docs/adr/063-store-substrate-and-resolver.md:25-27` (multi-instance naming "decided in
setup-04a-04-graph-roots-objectstore-component"); `docs/adr/108-processor-shell.md:81`, `:172`.

## 2. Every current spelling of the facts modelled

| Fact | Pin (carried by 5b) | Tree |
| --- | --- | --- |
| Instance name stamped as `StorageInstance` | `pin:…/component.go:148` — `const constructedInstanceName = "objectstore"`; `:743` (raw-lane ref), `:488` (threaded into the Store), `:403-404` (store-provide port) | `storage/objectstore/config.go:13-17` (`InstanceName`, defaults to bucket); consumers: `semsource:graph/bodystore.go:14` — `BodyStoreInstance = "objectstore"` ("must also match run.go's \"objectstore\" storage component name"), `semconnect:gateway/cs-api/graph_mutations.go:23` |
| Where a `StorageReference` points | ContentStorable lane: `StoreContent` (tree: raw body); raw lane: `pin:…/component.go:742-747` — the whole wire message, `ContentType: "application/json"` | `storage/objectstore/content.go:19` (`text/plain; charset=utf-8`), ADR-063 port note `docs/adr/063-…:15-18` (opening a reference returns the raw body) |
| Object key for a raw write | `pin:…/store.go:770-799` `DefaultKeyGenerator.GenerateKey` (`type/YYYY/MM/DD/HH/id_uuid`) | `storage/objectstore/content.go:115-119` `contentKey` (content lane, `content/…/<entity>_<uuid>`) |
| Delivery settlement (ack/nak/term) | `pin:…/component.go:984-1033` `classifyWriteDisposition`, `:1070-1108` `settleJetStreamWrite` (30 s / 5 s NAK delays) | `natsclient/delivery_settlement.go:278`, `:290`; graph-ingest's hand settlement `processor/graph-ingest/consume.go:164-205` and `keyed_ingest.go:228` |
| Waiting for an input stream | `pin:…/component.go:947-979` `waitForStream` (30 tries, `time.After`) | `processor/graph-ingest/consume.go:252` `waitForStream` |
| Component lifecycle state | `pin:…/component.go:47-55` (nine fields), `:432-682` | `component/lifecycle_type.go:27`; adopted by graph-index (`processor/graph-index/component.go`) |
| Health and data flow | `pin:…/component.go:1209-1242` (`ErrorCount: 0`, `Uptime: time.Since(lastAct)`, zero rates) | `component/lifecycle_type.go:100`, `:128` |
| JetStream consumer name for a port | `pin:…/component.go:332-337` `legacyObjectStoreConsumerName`, `:303-306` hashed name on collision, `:339-350` `validateObjectStoreConsumerName` | graph-ingest's consumer naming in `processor/graph-ingest/consume.go` (not compared further) |
| Strict config decode | `pin:…/component.go:173` — `if err := json.Unmarshal(rawConfig, &userConfig); err != nil {` (lenient) | `processor/graph-index/component.go:103-115` (`DisallowUnknownFields` plus trailing-data check); `processor/graph-ingest/component.go:246` |
| StoredMessage decode | `pin:…/stored_message.go:14-87` `buildStoredMessage` (map `Builder`) and `:177-197` `UnmarshalJSON` | `payloadregistry/registry.go:44-48` (Builder optional; JSON fallback) |

## 3. Adjacent claims

### Open pull requests

`gh pr list --state open --json number,title,isDraft,headRefName,changedFiles,files` (2026-10-27): #437 (this claim,
draft, 0 files) and #436 (`claude/fix-301-graph-ingest-test-seams`, draft, 0 files; head `f168d6f`, one claim commit;
`Closes #301`). No file or capability overlap today. Behavioural overlap: #436 retires graph-ingest's four test-only NATS
func fields (`consumeStream`, `waitForStreamInput`, `watchEntityStates`, `subscribeRequests`); the pin Component carries
the same shape with two of the same names (S22: `consumeStream`, `waitForStreamInput`). Whatever #436 settles as the
way tests make NATS fail without production fields is a precedent for 5b's S22 disposition. Both will edit
`docs/admission-ledger.yaml` (different rows). Merge order is the design's to state.

### Open issues naming a carried package

Queries (each `gh issue list --repo C360Studio/semengine --state open --search <q> --limit 200`): `objectstore`,
`"object store"`, `ObjectStore`, `StoredMessage`, `stored_message`, `StoreContent`, `storeregistry`, `ContentStorable`,
`StorageReference`, `StorageInstance`, `"instance name"`, `StoreProvider`, `DeclarePorts`, `waitForStream`,
`ConsumeStreamWithConfig`, `MaxDeliver`, `"consumer name"`, `payloadbuiltins`, `RegisterPayloads`, `KeyGenerator`,
`MetadataExtractor`, `DefaultKeyGenerator`, `"storage.objectstore"`, `Lifecycle`, `lifecycleguard`, `ErrorCount`,
`DataFlow`, `GenerateConfigSchema`, `"unknown key"`, `"content store"`, `ADR-063`, `ADR-055`, `ADR-103`, `ADR-108`,
`ADR-095`, `ADR-113`, `settlement`, `NakWithDelay`, `"core NATS"`, `natsfixture`, `"test client"`, `PublishToStream`,
`legacy`, `"store-provide"`, `Graphable`, `storage`, `cache`, `retention`, `binary`, `fusion`, `embedding`, `component`,
`"write lane"`, `consumer`. 51 distinct open issues; the hits for `objectstore`/`ObjectStore`/`"object store"`/
`StoredMessage`/`"storage.objectstore"` are #9, #33, #195, #196, #280, #281. Each read in full with its comments.

| Issue | Names | Gist | State of the work |
| --- | --- | --- | --- |
| #33 | `storage/objectstore/stored_message.go:90` (body; drift note 6386547712; comment 6451627146) | doc comment names `payloadbuiltins.Register` | every other site done; only this one left |
| #195 | "objectstore 2" (epic #9 table), comment 6451626523 "The objectstore Component's two test sites arrive with … (5b)" | each package's test client through `natsfixture.Open` | two Component sites (pin probe 4) |
| #196 | `storage/objectstore/component.go:49` (body), comment 6451628188 | lifecycle state through `internal/lifecycleguard` / `component.Lifecycle` | Component not yet ported |
| #280 | item 1 `storage/doc.go:82`, `:114-115`, `:206`, `storage/storage.go:40` | item 1 done by #432 (comment 6451625983); item 3 is `pkg/lifecycle` (`manager.go:325`) | nothing of objectstore left; stays for item 3 |
| #281 | objectstore's `DataCache` `category:performance` tag | `GenerateConfigSchema` drops a refused-tag field silently | the field is gone (ruling); the defect is in `component/schema_tags.go:404-409`, not objectstore |
| #9 | epic | change 4 order and rulings | — |

Adjacent, not naming the package: #36 (`internal/maxdelivery`, the parked-after-MaxDeliver observer; the pin
Component's comment names "the MaxDeliver parking follow-up", `pin:…/component.go:1053-1057`, and its write consumer's
default MaxDeliver is 3); #109 (processor shell adoption); #301 (graph-ingest's test-only NATS fields, PR
\#436); #68 (metric `sync.Once` workarounds; the Component registers no collector of its own, `grep -n metric pin:…/component.go`
shows only the registry passed to the Store, `:492`). Names the word only: #7, #10, #11, #15, #16, #20, #24, #26,
\#27, #29, #31, #32, #34, #35, #60, #80, #81, #94, #95, #96, #103, #105, #106, #107, #108, #110, #111, #114, #129,
\#134, #141, #155, #161, #198, #284, #300, #350, #361, #406, #415, #428. Closed since the Store inventory: #216 (PR #431).

### Specs and records

- `openspec/specs/object-store/spec.md` (the Store); `component-lifecycle` (`:12`, `:30`, `:48`, `:61`, `:90`: the
  shell); `lifecycle-suite` (`:57` "Start context ending stops nothing", `:70` "Owners are judged only by what Observe
  reports", `:176` "Observe adapter contract", `:214` "Each ported component proves its failed-cleanup branch");
  `component-registration` (`:18` "Each component package registers itself", `:78` "Unknown configuration keys are
  refused", `:88`, `:113`); `harness-boundaries` (`:33` "Production packages import one component family at most",
  `:65` "No retained context", the #422 consumer-creation rule); `metric-registry`; `health-status` (`:48`).
- Developer and reviewer contracts § Storage and retention carry the content-store bullet (`.agents/contracts/
  semengine-developer.md:271`, `:288`; reviewer `:291`); `.agents/rules-index.md:42`.
- SemStreams guidance that returns with the Component (not yet read at the pin; for the design): the pin's developer
  and reviewer contract sections on payload registry, state ownership and component wiring, NATS RPC, and the
  `new-payload` and `kv-or-stream` skills (already in `.agents/skills/` here).

### Admission-ledger rows

`storage/objectstore` (`docs/admission-ledger.yaml:3541-3588`, `disposition: adapt`, contract "the Store half (this
change); the Component arrives in setup-04a-04-graph-roots-objectstore-component", `known_risks` naming the deferred
symbols and S27). `docs/adr/063-…` row (`:3590-3602`, "multi-instance naming is left to the Component"). `storage`
(`:1995`). Not ported, by ruling: `componentregistry`, `payloadbuiltins` (`docs/tier1-cross-check.md:95`, 03B Q1 #3).
Rows the Component's port would touch: `storage/objectstore`, and the contract test's allowlist
(`internal/harness/contract`) if a file is added to it.

## 4. Who reads each 5b symbol (surface audit input)

Pin-wide non-test readers outside the package (`grep -rn --include='*.go'`, excluding `_test.go` and
`storage/objectstore/`), and consumers at symbol level (the four trees, non-test unless marked).

| Symbol | Pin readers outside the package | Consumer readers | In-package production reader |
| --- | --- | --- | --- |
| `Register` | `pin:componentregistry/register.go:129` (aggregator, not ported) | `semteams:internal/runtimecatalog/components.go:57`; semsource through the pin aggregator `semsource:cmd/semsource/run.go:295` and its `"objectstore"` component `:917-922` (`{"bucket_name": "CONTENT"}`) | — |
| `RegisterPayloads`, `StoredMessage` | `pin:payloadbuiltins/register.go:47` (aggregator); graph-ingest reads any `message.Storable` (`pin:processor/graph-ingest/component.go:1828`; tree `processor/graph-ingest/ingest.go:224-226`) | `semteams:internal/runtimecatalog/payloads.go:32`; `NewStoredMessage` in `semteams:internal/runtimecatalog/payloads_test.go:73` (test) ; semsource via `payloadbuiltins` `run.go:311` | `publishStoredMessage` `:803` |
| `NewComponent`, `DeclarePorts` | none (docs only: `pin:storage/doc.go:115`, `…/doc.go:142`) | none by name; reached through `Register` | `Register` `:24-25` |
| `Component` methods (`Start`, `Stop`, `Health`, `DataFlow`, `ProvidedStores`, ports, `Meta`, `ConfigSchema`, `Initialize`) | interfaces read by the component manager (tree: `service/component_manager.go:396`, `:459`) | through `Register` | — |
| `Component.IsStarted` | none | none | none |
| `Event` (type) | none | none (`grep -rn 'objectstore.Event'`: empty) | `publishEvent` `:836` |
| `"events"` port, subject `storage.objectstore.events` | none (`grep -rn 'storage\.objectstore\.'` outside the package: empty) | none (`storage.objectstore` in non-test go/json/yaml of the four trees: empty) | `publishEvent` |
| `"stored"` port, subject `storage.objectstore.stored`, type `storage.stored.v1` | none by subject; graph-ingest reads `Storable` on whatever subject a flow wires | none configured (both consumers set only `bucket_name`) | `publishStoredMessage` |
| `"write"` input, subject `storage.objectstore.write` | none publish to it | none publish to it (same search); semsource's comment: "the producer + Lens.Hydrate→StorageRef wiring lands once the upstream body/key convention is settled" (`semsource:cmd/semsource/run.go:911-916`) | — |
| `DefaultConfig` | none (doc `…/doc.go:102`) | none | `resolveConfig` `:170` |
| `Config.Ports` | — | none set it (`semteams:configs/flow-bootstrap.json`, `e2e-flow-bootstrap.json`: `{"bucket_name": "AGENT_CONTENT"}`; semsource `{"bucket_name": "CONTENT"}`) | `resolveObjectStorePorts` `:353` |
| `Config.KeyGenerator`, `Config.MetadataExtractor` | none; both `json:"-"` (`pin:…/config.go:44`, `:50`) so the Component, built from JSON, can never set them (S20) | none | the Store constructor `pin:…/store.go:161-167` |
| `storage.KeyGenerator`, `storage.MetadataExtractor` (in tree, `storage/storage.go:113`, `:127`) | implementers: only `DefaultKeyGenerator`; readers: only the Store's fields | none | after the Store change, nothing in the tree reads them (`git grep -n -e KeyGenerator -e MetadataExtractor -- '*.go'`: `storage/doc.go` comments and the declarations) |
| `Store.Store` | none | none | none; the Component calls the unexported `storeWithKeySource` (`:1158`) |
| `DefaultKeyGenerator` (exported type) | none | none | the Store constructor default (`pin:…/store.go:164`) |

Who would read `semsource:graph/bodystore.go:14`'s constant: the Component's stamped instance must equal
`"objectstore"` for semsource's resolver to find the store the Component registers; semsource's own standalone Stores
also stamp `"objectstore"` on bucket `CONTENT` (`semsource:processor/ast-source/bodystore.go:56-59`).

## 5. The problem shape

A port-fed write lane: subscribe or consume on configured inputs, decode once, store (content lane or raw lane), emit a
reference on an output port, settle the delivery; plus "a component owns a store and offers it to a registry". Closest
instances in the tree: graph-ingest's JetStream lane (`processor/graph-ingest/consume.go:64-205`: wait for stream,
policy entry point, decode once, poison → Term, settle) with `natsclient.SettleDelivery` (`keyed_ingest.go:228`);
the shell `component.Lifecycle` as graph-index adopts it; `service/component_manager.go:450-490`
`registerProvidedStores` for the store offer. Whether 5b adopts these is the design's.

## 6. Decision records for the carried files

Pin ADRs fetched in the copy (`docs/adr`, 85 files). `grep -l -i -E 'objectstore|object store|StoredMessage|
storage\.stored|StoreProvider|store-provide|ProvidedStores|constructedInstanceName|storage\.objectstore'`: 18 files
(028, 032, 033, 045, 049, 051, 055, 056, 059, 062, 063, 068, 072, 073, 075, 077, 094, 103); `grep -l -E
'NakWithDelay|MaxDeliver|settle'`: 15 more candidates, none naming objectstore. Read for what each decides about the
Component:

| ADR | Decides for the Component | In `docs/adr/` |
| --- | --- | --- |
| 063 | `StorageInstance` is the producing component's instance name (`pin:docs/adr/063-…:89-92`); the Component registers its live store (`:76`); a ref opens to the raw body (`:278-285`); multi-instance naming open (`:349-354`, `:412-413`); "on Stop, the objectstore Component `Close()`s its store" (`:157-158`, void here by ruling) | yes (port note `:13-27`) |
| 055 | `objectstore.StoredMessage` is a known multi-Subject Graphable; ingest regroups by Subject (`:410-418`) | no; the tree's graph-ingest regroups (`processor/graph-ingest/ingest.go:774-776`) |
| 103 | `StoredMessage`'s registration is the pattern for a payload with a decode path (`:80`) | no |
| 094 | Start-context descent for continuing work (`:66`); "Stream, KV, and ObjectStore roots returned by Client/framework constructors retire before release" (`:147`) | yes |
| 095 | one Start per instance; a failed Start's cleanup retained for the next Stop (the pin Component's `cleanupPending`, `pin:…/component.go:52`, `:611-625`) | yes |
| 062, 068, 073 | the Store's (Store inventory section 6) | 062/068/073 no |
| 028, 032, 033, 045, 049, 051, 056, 059, 072, 075, 077 | name ObjectStore in passing; decide nothing the three files implement | 077 yes |

SemEngine's own records binding the Component: ADR-108 (processor shell; `docs/adr/108-processor-shell.md:81`, `:172`
name objectstore's `component.go:49` as a change 4 adopter); ADR-113 (`docs/adr/113-start-context-bounds-start.md:59-64`:
"Ending `ctx` after `Start` returned stops nothing … The *run context* … does not end when the Start context ends";
`:71-74`: the suite check is the enforcement); ADR-109 (readiness: a process is ready when every component is healthy);
ADR-112 (natsclient promises). ADR-098 (observability splits by subject) is in the tree; it was not found to name the
Component.

## 7. Shape sweep

Pin copy: `component.go`, `register.go`, `stored_message.go`, and the deferred Store symbols.

`R` = candidate required work (basis named), `O` = optional under Bounded port scope. S19-S32, G7, G8 are the Store
inventory's ids, re-read here; N1-N12 are new.

| # | Shape | `pin:storage/objectstore/<path>:<line>` — text | R/O | Basis |
| --- | --- | --- | --- | --- |
| S19 | 4 config phantom | `component.go:184` — `if userConfig.DataCache.Enabled \|\| userConfig.DataCache.MaxSize > 0 {` | R | ruling 6446982969 removes the field; strict decode then refuses `data_cache` |
| S20 | 4 config phantom | `component.go:188-193` copy `KeyGenerator`/`MetadataExtractor` from JSON; `config.go:44` — ``KeyGenerator storage.KeyGenerator `json:"-" schema:"-"` ``, `:50` | R | surface audit (b): parsed, read by no behaviour |
| S21 | 4 unknown keys accepted | `component.go:173` — `if err := json.Unmarshal(rawConfig, &userConfig); err != nil {` | R | `component-registration` spec `:78` |
| S22 | 4 back doors | `component.go:76-81` — `newStore`, `closeStore`, `subscribeCore`, `waitForStreamInput`, `consumeStream`, `waitConsumerClosed` func fields (used only by `lifecycle_owner_test.go`, 18 sites) | R | back-door rule (#186, #190); `closeStore` also names the dropped `Close` |
| S23 | 2 copied shell | `component.go:47-55` — `lifecycleMu sync.Mutex` … `started bool`; `:432-682` own Start/Stop | R | ADR-108 (`docs/adr/108-…:172`); #196 |
| S24 | 4 advertised absent | `component.go:1222` — `ErrorCount: 0, // Would need error tracking`; `:1224` — `Uptime: time.Since(lastAct),`; `:1237-1239` zero rates | R | fail-closed/declared (`AGENTS.md` § 1); shell `component-lifecycle` `:48`, `:61` |
| S25 | 4 silent skip | `component.go:727` — `c.logger.Debug("Message not a BaseMessage, skipping StoredMessage emit",`; `:736` (not Graphable), `:778` (ContentStorable not Graphable): no count | R | "any skip, drop, or degrade is declared" |
| S26 | 4 drop | `component.go:700-704` — core-NATS write failure `c.logger.Error("Failed to process write message",` and dropped, no count | R | same; and ruling 6447702631 "refuses such messages loudly" on this lane too |
| S27 | 4 constant identity | `component.go:148` — `const constructedInstanceName = "objectstore"` | R (decision) | ADR-063 open question (`:412-413`); tree port note `docs/adr/063-…:25-27` assigns it here |
| S28 | 1 second spelling | `component.go:957-972` — `maxRetries := 30` … `case <-time.After(retryInterval):` beside `processor/graph-ingest/consume.go:252` | O | — |
| S29 | 4 exported/written, no reader | `component.go:685` — `func (c *Component) IsStarted() bool {`; `:84-85` — `messagesReceived uint64`, `messagesStored   uint64` (written `:697`, `:1059`, `:1128`, `:1163`, never read); `:108` — `type Event struct {` | R | surface audit (a) |
| S30 | 1 second spelling | `component.go:332` — `func legacyObjectStoreConsumerName(instanceName, subject string) string {` beside `:303-306` hashed name | O | — |
| S31 | 1 second spelling | `stored_message.go:14` — `func buildStoredMessage(fields map[string]any) (any, error) {` beside `:177` `UnmarshalJSON`; drops triple fields it does not name (e.g. `:42-46` timestamp parsed RFC3339 only, error ignored) | O | — |
| S32 | 4 stale doc | `stored_message.go:90-91` — `// supplied registry. Called from payloadbuiltins.Register at process` | R | #33 |
| S12 | 4 test seam in production | `store.go:742-746` — `now func() time.Time` ("Unexported test seam") in `DefaultKeyGenerator` | R | back-door rule (#186) |
| S13 | 4 exported, no reader | `store.go:220` — `func (s *Store) Store(ctx context.Context, msg any) (string, error) {`; `:741` — `type DefaultKeyGenerator struct {` | R | surface audit (a) |
| G7 | context | `component.go:450` — `runCtx, cancel := context.WithCancel(ctx)` (run context is Start's child) | R | ADR-113 Decision; **measured on both lanes**: pin probe 2 (JetStream: every later delivery NAKed; core NATS: every later write logged and lost, paired with N11; the suite passes field-level) |
| G8 | read contract | `component.go:1123` — `storageRef, err := c.store.StoreContent(ctx, cs)` | resolved by the Store change for this lane | tree `StoreContent` (D4 of the archived design) |
| N1 | read contract (G8's raw-lane twin) | `component.go:742-747` — `storageRef := &message.StorageReference{` … `ContentType:     "application/json",` `Size: int64(len(data))`: the raw lane's reference opens to the whole wire message (a `BaseMessage` JSON envelope), which graph-embedding reads as body text (`pin:graph/embedding/worker.go:1069-1075`, a Debug line when the bytes start with `{`) | R | ADR-063 raw-body read contract (`pin:docs/adr/063-…:278-285`), kept by the Store change (archived design D14) |
| N2 | 4 advertised absent | `register.go:10-12` — `//   - LRU/TTL/Hybrid caching support`, `Pluggable key generation`, `Pluggable metadata extraction`; `:8` and `component.go:1187` — "immutable message storage" (Put replaces, Store probe P4); `component.go:1-2` "time-bucketed keys and caching". The pin's Component docs say the same: `README.md:3` ("immutable message storage with time-bucketed keys and caching"), `:15-35` (config example with `events` "(stored, retrieved)" and `data_cache`), `:46-48` (`data_cache.*` keys), `:82-96` (write input "publishes storage event"; `events` port), `:112` ("immutable versioned object"); `doc.go:2`, `:8`, `:11`, `:26-29` ("events" port "(stored, retrieved, deleted)"), `:104-106`, `:126-136` (`EnableCache`). The tree's `README.md` and `doc.go` were rewritten by the Store change and carry none of these (`storage/objectstore/README.md:8`, `:64-67`); they stay dropped, and the Component's own text is new | R | S1/S2 class; ruling 6446982969 |
| N3 | 4 port with no reader | `config.go:85` — `Name: "events", Config: component.NATSPort{Subject: "storage.objectstore.events"}, Required: false,` and `component.go:836-855` `publishEvent`; no reader pin-wide or in a consumer (section 4) | R (surface audit disposition) | surface audit (a), (c): `Event.Type` comment `component.go:109` — `// "stored", "retrieved"`, only `stored` is published; `Metadata` never set |
| N4 | 1 second spelling | `component.go:849` — `if err := c.natsClient.GetConnection().Publish(eventSubject, data); err != nil {` (raw connection, no context) beside `:828` `c.natsClient.Publish(ctx, storedSubject, msgData)` | O | — |
| N5 | 1 second spelling | `component.go:1022-1033` `classifyWriteDisposition` and `:1070-1108` `settleJetStreamWrite` (hand settlement, own NAK delays `:985-986`) beside `natsclient.SettleDeliveryWithRetry` (`natsclient/delivery_settlement.go:290`) | O | the architect contract's adoption-sweep paragraph names hand settlement as the worked case; no binding requirement found |
| N6 | 4 dead field | `component.go:44` — `enabled      bool` (written `:217`, never read) | O | — |
| N7 | 4 surface the tree already carries with no reader | tree `storage/storage.go:113` — `type KeyGenerator interface {`, `:127` — `type MetadataExtractor interface {`: after the Store change nothing reads them; their only pin reader is the Store's construction default and `storeWithKeySource` (`pin:…/store.go:161-167`, `:246`, `:268-269`) | R (disposition) | surface audit (a); package is `storage`, not `objectstore` |
| N8 | 4 no-op | `component.go:135-138` — `func (c *Component) Initialize() error {` `return nil` | — | shell `component/lifecycle_type.go:46` owns `Initialize` |
| N9 | context retention check | `component.go:53-54` — `startDone chan struct{}`, `cancel context.CancelFunc`; no `context.Context` field; no `context.Background`/`TODO`/`WithoutCancel` in the three files (`grep -n -E 'context\.(Background\|TODO\|WithoutCancel\|WithCancel\|WithTimeout)\|ctx\.Value\|WithValue'`: only `:450`) | — | ADR-113 allows a private cancel |
| N10 | consumer gate | `component.go:931` — `consume := c.natsClient.ConsumeStreamWithConfig`; `:901` — `consumerCfg, consumerErr := component.GetConsumerConfig(port)` | R | `harness-boundaries` #422 allowlist (`internal/harness/contract/consumerpolicy_test.go:40-42`) |
| N11 | default lane durability | `config.go:78` — default `write` input is core NATS (`component.NATSPort{Subject: "storage.objectstore.write"}`), so a failed store or a refused `StoreContent` is logged and lost (`component.go:691-705`); both consumers use the default ports | R (declare) | "any skip, drop, or degrade is declared"; ruling 6447702631 (refuse loudly); pin ADR-070 states the same principle for work dispatch: "Core-NATS is for loss-tolerant fan-out" (`pin:docs/adr/070-gated-dag-durable-dispatch.md:43-44`, not in `docs/adr/`) |
| N13 | 1 second spelling (unchecked key part) | `store.go:776-786` — `msgType = m.Type().String()` … `identifier = m.ID()`: on the raw lane the key is built from the decoded message's type and wire `id`, which nothing validates (`message/base_message.go:104-106`, tree; the decoder sets it from the wire), beside the content lane's validated entity ID (`storage/objectstore/content.go:42-44`, tree) | O | uniqueness is the UUID nonce (`store.go:795`), so an odd `id` changes the key's readability, not whether two writes collide; what characters an ObjectStore name accepts is not probed (#170: not promised) |
| N12 | lifecycle-suite adapter | pin probe 2: an `Observe` that lists fields passes `StartContextEndStopsNothing` while both lanes are dead | R (test) | `lifecycle-suite` spec `:70`, `:176` (owners judged only by what `Observe` reports); ADR-113 `:71-74` |

Shape 5 (classification by error text): none. `grep -n -E 'strings\.(Contains|HasPrefix)\(.*[Ee]rr|\.Error\(\)'` over
the three files finds only log arguments (`component.go:703`, `:728`, `:845`, `:852`, `:1075`, `:1087`, `:1091`,
`:1097`, `:1100`, `:1105`); classification uses `errs.IsFatal` (`:425`), `errs.IsInvalid` (`:1029`), `errors.Is`
(`:1084`).

No further finding, read in full: `register.go` beyond N2; `stored_message.go:105-222` (accessors, `Validate` checks
entity ID and non-nil ref, `MarshalJSON` map); `planObjectStoreInputBindings` and `resolveObjectStorePorts`
(`component.go:234-411`: duplicate and collision refusals, removed `api` input refused, one subject per port).

Behaviour of the ContentStorable lane under the PR #432 ruling, from the code: a refused `StoreContent` returns an
invalid-input error (tree `storage/objectstore/content.go:72-109`); at the pin that maps to `dispositionTerm` on a
JetStream lane (`component.go:1029-1030`, an Error log, `:1095`) and to an Error log and drop on the core-NATS lane
(`:700-704`). No count either way (S26, N11).

## 8. External-provider code

None (section "Briefing hypotheses checked"). `grep -n -i -E 'net/http|http\.|s3|aws|minio|gcs|azure|llm|openai|
provider'` over the three files: `component.StoreProvider` and the `provider` port variable (`component.go:118`,
`:402-409`) only.

## 9. Collision table: the Component (a durable write lane and a store owner)

| Dimension | Evidence |
| --- | --- |
| Semantic class | accept messages on configured ports, persist them as objects, emit a `StorageReference` per stored message; own one Store and offer it as `StorageInstance` |
| Owners | the Component (5b); the Store (`storage/objectstore/store.go`, tree) for persistence; consumer-owned standalone Stores on the same bucket (`semsource:processor/ast-source/bodystore.go:56-59`, `processor/doc-source/component.go:206-209`, `processor/supersession/versiondiff_serve.go:40-47`, `processor/code-context/component.go:234-241`); graph-ingest for the graph write that a `StoredMessage` drives |
| Catalogs | `composition/catalog.go:41` (`"objectstore": "Object Store"`); `component.Registry` registration; `storage/storeregistry` at runtime |
| Status | `Health`/`DataFlow` (pin: S24); no status key |
| Lifecycle | `Start` creates or opens the bucket and binds inputs; `Stop` drains; one-shot (ADR-095); no replay of its own; JetStream redelivery bounded by MaxDeliver 3 (`component.go:1053-1057`) |
| Ownership | one store per Component; `registerProvidedStores` refuses a second claim on one instance name at Start (`service/component_manager.go:486`, doc `:442-449`); store-provide port is non-exclusive at admission (`component/port_provide.go:22-24`); with S27 two Components in one process both claim `"objectstore"` |
| Readers | `storage/storeregistry` resolvers (graph-embedding `Open`, fusion `Get`, change 5); graph-ingest reads `StoredMessage` as `message.Storable` (`processor/graph-ingest/ingest.go:224-226`) |
| Writers | publishers on the `write` input (none in the four consumers); the Component's own `stored` and `events` publishes |
| Recovery | JetStream lane: NAK/Term per `classifyWriteDisposition`; core-NATS lane: none (N11); nothing rebuilds objects |

## 10. Size against the one-package rule

Non-test pin lines carried: `component.go` 1,242, `stored_message.go` 222, `register.go` 33 = **1,497** (`wc -l` on the
pin copy). Deferred Store symbols, already counted in the Store change's 1,845: `store.go:212-296` (`Store`,
`storeWithKeySource`, 85), `:734-799` (`DefaultKeyGenerator`, 66), `config.go:22-23`, `:40-50`, `:69-112` (`Ports`,
`KeyGenerator`, `MetadataExtractor`, `DefaultConfig`, 57) = 208. Under the bound of about 3,000 (#159); the owner's
ruling says no waiver is needed (#9 comment 6386556850, item 1). Pin test lines for the Component: 2,025
(`component_ack_integration_test.go` 624, `lifecycle_owner_test.go` 356, `config_ports_test.go` 283,
`component_ack_test.go` 280, `component_write_key_integration_test.go` 157, `component_write_key_test.go` 148,
`stored_message_contract_test.go` 112, `component_instance_integration_test.go` 103, `lifecycle_integration_test.go`
65); `entity_id_contract_test.go` (96) tests `StoreContent`.

## 11. Library behaviour the port relies on (#170, ADR-112)

Measured here: pin probes 1 and 2 (one run each). Relied on by the pin's text and not probed here: nats.go
`NakWithDelay` delays redelivery (`component.go:981-986`); a `MaxDeliver` of 3 parks a message un-acked
(`:1053-1057`); `ConsumeContext.Drain` then `Closed()` joins delivery (`:631-652`). The tree's natsclient states the
last (`natsclient/stream.go:716-722`); the first two are not promised by a probe in this inventory. Measured: an ended
handler context reaches every later handler on both lanes (probe 2; pin `natsclient/stream.go:432-433`,
`natsclient/client.go:835-836`).

## 12. Adopter seam inventory

Surfaces reached from outside: the `objectstore` component factory and its JSON config (`bucket_name`, `ports`), the
`write`/`events`/`stored` ports and their default subjects, `RegisterPayloads`/`StoredMessage` (`storage.stored.v1`),
the `StorageInstance` the Component stamps and registers.

1. **What must an adopter know?** (a) The Component stamps and registers `StorageInstance = "objectstore"` whatever its
   instance key (S27); semsource relies on that literal (`semsource:graph/bodystore.go:14`) and two Components in one
   process collide at Start. (b) The default `write` input is core NATS: a failed or refused write is logged and lost
   (N11). (c) A ContentStorable message carrying non-body content is refused (ruling 6447702631); on the default lane
   that is a log line. (d) The raw lane's reference opens to the whole JSON message, not a body (N1). (e) Ending the
   context given to `Start` silently stops both write lanes: JetStream deliveries are NAKed, core-NATS writes are logged
   and lost (G7 with N11, probe 2). (f) `data_cache`, `KeyGenerator` and
   `MetadataExtractor` do nothing (S19, S20).
2. **If they do nothing:** both consumers configure only `bucket_name` and use default ports; no consumer publishes to
   `storage.objectstore.write` today, so the live path is the store-provide registration (`ProvidedStores`) only. A
   producer that starts publishing gets (b), (c) and (d) silently.
3. **Where they find out:** (a) boot error from `registerProvidedStores` for two Components, nowhere for a name mismatch
   (content-unresolved); (b) log line; (c) log line (core NATS) or Term plus log (JetStream); (d) a Debug line in
   graph-embedding; (e) an Error log per message on both lanes, never Health (it stays healthy); (f) nowhere at the pin
   (strict decode would make `data_cache` a boot error).
4. **What should they have to know:** none of (a)-(f) is a fact an adopter can act on better than the framework; each
   is a gap the design names.

## Intent check

This inventory moves no boundary: no capability in `AGENTS.md` § What this is for is admitted, deferred or excluded by
it. Rulings already in force for this package: tier 0 only (#9 comment 6386603659); the split (#9 comment 6386556850,
item 1); order (#9 comment 6386856657, item 4); data cache and `Close` dropped (PR #432 comment 6446982969); non-body
content refused (PR #432 comment 6447702631).

## Open evidence questions

- E1. Not measured: whether redelivery of the NAKed JetStream deliveries stops after MaxDeliver 3 and the message
  parks (the pin's comment says so, `pin:…/component.go:1053-1057`); probe 2 waited 5 s, one run.
- E2. Whether any deployment publishes to `storage.objectstore.write` outside the four consumers is out of scope.
- E3. The pin's developer and reviewer contract sections that govern this Component (payload registry, component
  wiring) were not read in this pass; the design reads them (Extraction slices, "Guidance returns with the package").
