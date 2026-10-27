# Inventory: setup-04a-04-graph-roots-objectstore-store

base: `5d6042a84fe28b846fa3d4deab4c996ea121d3e6` (origin/main after PR #414; the claim commit `4102f34` on
`claude/port-objectstore` adds no file). Pin paths are SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`,
written `pin:<path>:<line>`, read from the pin tarball copy at `/tmp/pin-8b99efe9-gi/src` (local only); three files
(`store.go`, `component.go`, `retention.go`) were checked byte-equal against
`gh api repos/C360Studio/semstreams/contents/storage/objectstore/<f>?ref=8b99efe9`. Consumer cites are at the
default-branch heads of 2026-10-27, from `gh api repos/C360Studio/<repo>/tarball/<sha>` (`docs/inventory-scope.md`
rule 4): semsource `e4febc0d2e23bf33811edc20b9a4159cf5275dc4`, semconnect `d0d06e00bf05a545f30ceea798db1c2b1ee47d4f`,
semteams `8e215f59ae2d5de9bebdc9ceb89fe96f6828d810`, semboids `8c03cc53836ced93a5df7064473c63ff144e64f1`; written
`<repo>:<path>:<line>`.

Status: round 2. `INVENTORY PASS` (early Claude reviewer); design findings folded in. the design was drafted in the same
pass at the caller's direction, as on the
four earlier change 4 pull requests.

Kind of change: a port, split by the owner's ruling (#9 comment 6386556850, item 1: "objectstore (3,342) splits into
the Store, then the Component, and needs none"). This change carries the Store half; pull request 5b carries the
Component. The shape sweep below covers every non-test file of the package, both halves, so 5b starts from it.

Briefing hypotheses that did not hold:

- "Store, then Component may mean two PRs, or a waiver": the split into two pull requests is already ruled (above);
  neither half needs a waiver (section 10).
- "External-provider code (cloud object stores, HTTP clients, S3)": none (section 8).
- "`Config.DataCache` exposes `internal/cache`": true at the pin shape (`pin:storage/objectstore/config.go:38`), but
  no consumer sets it and no pin package outside objectstore reads it (section 4); it is the Component's default
  that turns it on (`pin:…/config.go:102-108`).
- "Who calls objectstore": the only production callers outside the package at the pin are `componentregistry`
  (`Register`), `payloadbuiltins` (`RegisterPayloads`) and the example `examples/processors/document`
  (`*objectstore.Store`, `StoreContent`); semsource is the one consumer that calls the Store (section 4).

## Pin probe

go1.26.10 darwin/arm64, on the pin copy, unit tests only (no build tag).

- `go test -race -count=1 -p 1 -cpu 1 -cover ./storage/objectstore`: ok, 1.35 s, coverage 43.3% (the Store's I/O is
  covered only by the integration tests, not run).
- `go test -count=5 -p 1 -cpu 1 -shuffle=on -v ./storage/objectstore`: ok, 0.29 s, `-test.shuffle 1793112890540235000`.
- The lifecycle suite is not run against the pin: it is this tree's harness and the pin's Component does not build
  against it. The Store is not a suite target after design D6 (it would own no goroutine); the Component is, in 5b.
- Integration tests (six files, `//go:build integration`) were not run at the pin (Docker cost).
- Test-text rules over `pin:storage/objectstore/*_test.go` (3,690 lines, 69 `func Test`): `time.Sleep` 6
  (`component_write_key_integration_test.go:130`; `component_ack_integration_test.go:424`, `:492`, `:539`, `:605`,
  `:622`, all Component tests, 5b); `t.Skip` 0; `t.Parallel` 29, none in an integration-tagged file
  (`component_ack_test.go`, `entity_id_contract_test.go`, `config_ports_test.go`, `retention_test.go`,
  `stored_message_contract_test.go`); build tags other than `integration`: none. Test clients outside the fixture:
  `component_instance_integration_test.go:35`, `:67` (`natsclient.NewTestClient`, 5b),
  `store_integration_test.go:37` (`natsclient.NewSharedTestClient`, this change; #195).

## 1. What exists (the claimed gap)

### The package is not in the tree

`git ls-files storage/objectstore`: empty. `grep -n "source_path: storage/objectstore" docs/admission-ledger.yaml`:
empty. Production references in the tree (`git grep -n -i objectstore -- '*.go' ':!*_test.go'`) are comments and
one refusal text: `natsclient/backing_stream_prefix.go:125-127` — `"…content stores are "+ "provisioned by the
content-store constructor (storage/objectstore.NewStoreWithConfig), and "+`, naming a symbol this change brings;
`natsclient/kv_retention.go:18`, `:42` (the KV guard "mirrors backingStreamRetention in
storage/objectstore/retention.go"); `composition/catalog.go:41` (`"objectstore": "Object Store"`).

### What the Store needs, and where it is in the tree

`natsclient/client.go:1236` — `func (m *Client) JetStream() (jetstream.JetStream, error) {`;
`natsclient/kv.go:179` `CheckNoLifecycleRetention`, `:169` `ErrGraphBucketRetention`;
`natsclient/backing_stream_prefix.go:28` — `ObjectStoreStreamPrefix = "OBJ_"`; `storage/storage.go:16`
`ErrObjectNotFound`, `:51` `Store`, `:94` `StreamableStore`; `message/content_storable.go:44` `ContentStorable`,
`:101` `BinaryContent`, `:134` `BinaryStorable`; `message/storable.go:15` `StorageReference`;
`pkg/types/entity_id.go:149` `ValidateEntityID`; `pkg/errs/errs.go:370`, `:379`, `:388`, `:176` (`WrapTransient`,
`WrapFatal`, `WrapInvalid`, `IsFatal`); `metric/registry.go:29` `RegisterOrGet`; `github.com/google/uuid v1.6.0`
(`go.mod:13`). The Store needs nothing absent. `internal/cache` exists (`internal/cache/config.go:107`
`NewFromConfig`); its ledger row moved it from `pkg/cache` (`docs/admission-ledger.yaml:812-816`).

The Component half (5b) also needs `component.Lifecycle` (`component/lifecycle_type.go`), the consumer policy entry
points and the #422 allowlist, `payloadregistry` (`payloadregistry/registry.go:48` `Builder`), `component.StoreProvider`
(`component/dependencies.go:26`) and `component.StoreProvidePort` (`component/port_provide.go:13`); all present.

## 2. Every current spelling of the facts modeled

| Fact | Spellings at the pin (carried) | Spellings in the tree |
| --- | --- | --- |
| A content store has no lifecycle eviction | `pin:storage/objectstore/retention.go:70-126` `reconcileNoLifecycleRetention`, `:33-47` `backingStreamRetention` | `natsclient/kv_retention.go:60-123` `ReconcileNoLifecycleRetention` (the KV twin, same steps, prefix `KV_`), `:22-36` `kvBackingStreamRetention`; shared predicate `natsclient/kv.go:179`; `natsclient/kvspec.go:88` `RetentionNoLifecycle` (strip) and `:104` `RetentionNoLifecycleStrict` (refuse) |
| An object is missing | `pin:…/store.go:322-324` (Get maps `jetstream.ErrObjectNotFound` to `storage.ErrObjectNotFound`); `:346-349` Open, `:368-371` Delete, `:409-412` GetMetadata, `:555-558` FetchContent, `:718-721` FetchBinary do not | `storage/storage.go:9-16` (`ErrObjectNotFound`, "deliberately NOT classified as transient") |
| Key uniqueness per write | `pin:…/store.go:788-796` (UUID nonce, `DefaultKeyGenerator`); `:671-681` and `:685-695` (`UnixNano` suffix, content and binary keys) | none |
| Bucket name default | `pin:…/store.go:96-99` (`"MESSAGES"`), `pin:…/config.go:101` (`DefaultConfig`) | none |
| Instance name | `pin:…/store.go:105-108` (defaults to bucket), `pin:…/component.go:148` (`constructedInstanceName = "objectstore"`), `:743` (StoredMessage stamp) | `storage/storeregistry` keys on it; consumers hard-code `"objectstore"`: `semsource:graph/bodystore.go:14`, `semconnect:gateway/cs-api/graph_mutations.go:23` |
| What a StorageReference points at | `pin:…/store.go:521-526` (StoreContent: a JSON envelope, `ContentType: "application/json"`); `pin:docs/adr/063-store-substrate-and-resolver.md:278-285` ("the store a ref points at holds the RAW body bytes … *not* a `StoreContent` JSON envelope"); `pin:graph/embedding/worker.go:1070-1077` (warns when a read starts with `{`) | `message/storable.go:15-30` |
| A stream's waiting loop | `pin:…/component.go:947-979` `waitForStream` (30 tries, `time.After`) | `processor/graph-ingest/consume.go:249-265` `waitForStream` (5b) |
| Component lifecycle state | `pin:…/component.go:47-55` (nine fields), `:432-627` | `component/lifecycle_type.go` (`component.Lifecycle`, ADR-108) (5b) |

## 3. Adjacent claims

### Open pull requests

`gh pr list --state open --json number,title,isDraft,headRefName,changedFiles,files` on 2026-10-27: #432 (this
claim, draft, 0 files) and #431 (`claude/fix-216-consumer-rebind`, draft, 5 files, all under
`openspec/changes/fix-216-consumer-rebind/`, capability delta `graph-ingest-recovery`). No file or capability
overlap with this change. Behavioural note for 5b, not this change: the Component's JetStream write consumer is the
same class as #216 (a consumer whose stream is recreated). Merge order: independent; either may merge first.

### Open issues naming a carried package

Queries, each `gh issue list --state open --search <q> --limit 200`: `objectstore`, `"object store"`, `ObjectStore`,
`StoredMessage`, `StoreContent`, `storeregistry`, `ContentStorable`, `DataCache`, `internal/cache`,
`StorageReference`, `storage`, `NoLifecycleRetention`, `"instance name"`, `StoreProvider`, `cache`, `retention`,
`ADR-068`, `waitForStream`, `"consumer name"`, `ConsumeStreamWithConfig`, `MaxDeliver`, `ErrorCount`,
`"content store"`, `binary`, `StorageInstance`, `ADR-063`, `fusion`, `embedding`. Hits read in full.

| Issue | Names | Gist |
| --- | --- | --- |
| #280 | `storage/doc.go:82`, `:114-115`, `:206`; `storage/storage.go:40` (item 1); drift note 2026-10-24 | storage doc shows objectstore calls that do not exist; item 1 retires "in objectstore's PR"; item 3 is `pkg/lifecycle` |
| #33 | `pin:storage/objectstore/stored_message.go:90` (the last site) | doc comment names `payloadbuiltins.Register`; "retires with objectstore's PR" |
| #195 | objectstore 2 test-client sites | each package's test client through `natsfixture.Open` |
| #196 | `storage/objectstore/component.go:49` | lifecycle state through `component.Lifecycle` (ADR-108) |
| #321 | ADR-063 governs `storage/storeregistry`, `component/dependencies.go:17`, `:52`, `component/port_provide.go:6` | carry ADR-063 into `docs/adr` |
| #281 | `GenerateConfigSchema` drops a field whose tag it refuses | objectstore's `DataCache` tag `category:performance` (`pin:…/config.go:38`) is refused (`component/schema_tags.go` accepts `basic`, `advanced`), so the field is missing from the generated schema: semteams' published `schemas/objectstore.v1.json` has only `bucket_name` and `ports` (Component, 5b) |
| #36 | `internal/maxdelivery` | the pin's write-consumer comment names "the MaxDeliver parking follow-up" (`pin:…/component.go:1053-1057`) (5b) |
| #216 | graph-ingest consumer of a recreated stream | same class as the Component's write consumer (5b) |

Names the word, not the package: #9, #106, #109, #300, #134, #105, #406, #11, #141, #198, #20, #15, #350, #103,
issues #161, #10, #26, #24, #7, #110, #114, #31, #35, #96, #107, #155, #32, #94, #95, #16, #415, #428.

### Specs and records

- No `openspec/specs/*` names objectstore (`git grep -n -i objectstore -- openspec/specs`: empty).
  `openspec/specs/metric-registry/spec.md:62` ("No process-global registration"), `:73` ("A component registers only
  on the registry it is given"); `harness-boundaries`; `error-classification`.
- Foundation design: `openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md:90-91` — the Store is a
  lifecycle owner only "by delegation: `Close` closes a `pkg/cache` instance with a ticker goroutine"; `:112`
  (`NewStoreWithConfig(ctx, …)`, `Close()`); `:168-170` (owners in change 4 include `objectstore.Component` and
  `objectstore.Store`).
- Developer contract `.agents/contracts/semengine-developer.md:268-272` § Storage and retention carried only the first
  two pin bullets, "the rest govern graph state and the `storage` package, not yet ported"; the pin's next bullet
  (`pin:.agents/contracts/semstreams-developer.md:182-183` — "Large content uses backend-neutral `storage.Store` and
  `StorageReference` contracts. NATS ObjectStore is one bounded backend…") and its reviewer twin
  (`pin:.agents/contracts/semstreams-reviewer.md:175-176`) govern this package.
- `docs/contract.md:167`, `:176`, `:186` show `objectstore.Register` and `objectstore.RegisterPayloads` (5b).

### Admission-ledger rows

No `storage/objectstore` row. Rows this change edits: `storage` (`docs/admission-ledger.yaml:1993-2007`,
`disposition: carry`, "doc.go and storage.go match it"; #280 item 1 edits both files, so it becomes `adapt`),
`internal/cache` (`:812-842`, "admitted whole because six admitted packages import it at the pin"; objectstore is one
of the six). New rows: `storage/objectstore` and `docs/adr/063-store-substrate-and-resolver.md`.

## 4. Who calls the Store, and who the surface is for

At the pin, outside the package (`grep -rln '"github.com/c360studio/semstreams/storage/objectstore"'`, non-test):
`componentregistry/register.go:129` (`objectstore.Register`, 5b), `payloadbuiltins/register.go:47`
(`objectstore.RegisterPayloads`, 5b), `examples/processors/document/component.go:118`, `:720-750`, `:765`
(`*objectstore.Store`, `StoreContent`; a consumer adapter, so `StoreContent` is framework API). Test-only:
`graph/embedding/worker_storeresolver_integration_test.go:26`,
`processor/graph-embedding/offloaded_identity_integration_test.go:45`
(`objectstore.Config`, `NewStoreWithConfig`).

Consumers at symbol level:

- semsource: `processor/ast-source/bodystore.go:56-59` (`objectstore.NewStoreWithConfig(ctx, c.natsClient,
  objectstore.Config{BucketName: graph.BodyStoreBucket, InstanceName: graph.BodyStoreInstance})`), `:155`
  (`c.bodyStore.Put(ctx, w.key, []byte(w.body))`, keys content-addressed by sha256, `:25`, `:112`);
  `processor/doc-source/component.go:206-209` (same constructor; failure fails startup); `handler/doc/entities.go:408`
  (`store.Put`); `processor/supersession/versiondiff_serve.go:40-47` and `processor/code-context/component.go:234-241`
  (same constructor, used as `storage.Store(store)` by `fusion.MapStoreResolver`, which calls `Get`);
  `graph/bodystore.go:14` (`BodyStoreInstance = "objectstore"`, "must also match run.go's \"objectstore\" storage
  component name"); `cmd/semsource/run.go:917-922` (Component, `{"bucket_name": "CONTENT"}`, default ports).
- semteams: `internal/runtimecatalog/components.go:57` (`objectstore.Register`), `payloads.go:32`
  (`objectstore.RegisterPayloads`); `configs/flow-bootstrap.json` and `configs/e2e-flow-bootstrap.json` (Component,
  `{"bucket_name": "AGENT_CONTENT"}`); `cmd/semteams/main.go:526-530` reads `bucket_name` from that config.
- semconnect: no import; `gateway/cs-api/graph_mutations.go:23` (`schemaArtifactStorageInstance = "objectstore"`).
- semboids: none.
- No consumer sets `data_cache`, `KeyGenerator` or `MetadataExtractor`, calls `List`, `Delete`, `Open`, `Close`,
  `FetchContent`, `FetchBinary`, `GetMetadata`, `StoreContent`, or subscribes to `storage.objectstore.*`
  (`grep -rn` over the four trees, non-test).

Per Store symbol (pin-wide readers, in-package production readers, consumer readers):

| Symbol | Readers | Disposition (design) |
| --- | --- | --- |
| `NewStoreWithConfig`, `Config.BucketName`, `Config.InstanceName` | semsource (4 sites); in-package Component | keep |
| `NewStoreWithConfigAndMetrics` | in-package Component (`pin:…/component.go:492`) | keep (reader arrives in 5b) |
| `Put`, `Get`, `List`, `Delete`, `Open` | `storage.Store`/`StreamableStore`; semsource `Put`, fusion `Get`; graph-embedding `Open` via the registry | keep, contract fixed (D4) |
| `StoreContent`, `StoredContent`, `BinaryRef` | Component (in-package), examples/document | keep (framework API) |
| `InstanceName()` | Component `ProvidedStores` (`pin:…/component.go:131`) | keep |
| `Config.Logger` | Component (`pin:…/component.go:488`) | keep |
| `Config.DataCache`, `Close` | Component default (`pin:…/config.go:102-108`); Component Stop (`:662`) | dropped by owner ruling (PR #432 comment 6446982969) (D6) |
| `FetchContent`, `extractContentFromBaseMessage`, `extractTextFields`, `SetDecoder` | none outside the package; `SetDecoder` only from the Component (`:499`), for FetchContent | not carried |
| `FetchBinary`, `GetMetadata` | none | not carried |
| `GetFieldByRole`, `HasRole`, `GetBinaryRefByRole`, `HasBinaryContent` | none | not carried |
| `NewStoredContent`, `NewStoredContentWithBinary` | `StoreContent` only | unexported |
| `Store.Store`, `storeWithKeySource`, `DefaultKeyGenerator`, `Config.KeyGenerator`, `Config.MetadataExtractor`, `Config.Ports`, `DefaultConfig` | the Component's raw write lane and config resolution only | 5b |
| `Config.Validate` (`config.go:61-67`, returns nil) | none at the pin | not carried |

New surface the pin lacks: none.

## 5. The problem shape

Create-or-open a framework-provisioned NATS store, then assert it carries no lifecycle eviction, then serve
classified reads and writes. Closest instance: `natsclient/kvspec.go:232` `EnsureFrameworkBucket` with its
`RetentionNoLifecycle` arm calling `ReconcileNoLifecycleRetention` (`natsclient/kvspec.go:282-283`,
`natsclient/kv_retention.go:60`), sharing `CheckNoLifecycleRetention` with the pin's ObjectStore guard. The design
keeps the pin's guard and the shared predicate (D5). Classified not-found: `storage/storage.go:9-16` and the pin's
`Get` (`pin:…/store.go:318-324`); the design extends that one spelling to `Open` and `Delete`. No primitive is
established, so no adoption sweep.

## 6. The pin's decision records for the carried package

Fetched from the pin copy (`docs/adr`, 85 files); `grep -l -i -E` over them for `objectstore`, `object store`,
`StoreContent`, `StorageReference`, `ContentStorable`, `StoredMessage`, `storeregistry`, `StoreProvider`,
`BinaryStorable` and `content store` (one alternation) returned 19; each hit read for what it decides about this
package.

| ADR | Decides for objectstore | In `docs/adr/` |
| --- | --- | --- |
| 063 Store substrate and `StorageInstance` resolver | `StorageInstance` is the producing component's instance name (`:88-92`); the registry holds live `objectstore.Store` handles (`:271-275`); read contract: a ref points at raw body bytes, not a `StoreContent` envelope (`:278-285`); Component Stop closes its store (`:157-158`) and "the registrant (owner) owns `Close`; borrowers must not Close" (`:256`); open: multi-instance naming (`:349-354`, `:412-413`) | no (#321) |
| 068 Graph retention, deletion and GC | no reference-blind lifecycle retention on state the graph references (the guard's basis, `pin:…/store.go:115-122`, `retention.go:49-69`); orphaned-blob collection (`:332`) not implemented at the pin | no |
| 055 Write-intent taxonomy | `objectstore.StoredMessage` is a known multi-subject Graphable, regrouped by subject at ingest (`:410-418`) (5b) | no |
| 103 Payload registry is the single type authority | `StoredMessage`'s registration is the shape (`:80`) (5b) | no |
| 062 Deterministic graph fusion | NATS ObjectStore is one `storage.Store` implementation; fusion binds to the interface (`:101-112`, `:192-196`) | no |
| 073 Three-tier ingestion and retention | bulky content to ObjectStore via `ContentStorable`, reclaimed by refcount (`:83-84`, `:224`); not implemented | no |
| 094 Boot-only composition | "Stream, KV, and ObjectStore roots returned by Client/framework constructors retire before release" (`:147`); the Store returns no raw handle | yes |
| 075, 028, 032, 033, 045, 049, 051, 054, 056, 059, 072, 077 | name ObjectStore or `ContentStorable` in passing; decide nothing this package implements | 077 yes, others no |

ADR-108 (processor shell), ADR-113 (Start context) and ADR-095 (one-shot lifecycle) govern the Component (5b), not
the Store.

## 7. Shape sweep (pin copy, every non-test file of the package)

`R` = a candidate for required work (basis in the design), `O` = optional under Bounded port scope. "5b" marks a
finding in a file or symbol pull request 5b carries; its disposition is that design's, recorded here so it starts
from the list.

| # | Shape | `pin:storage/objectstore/<path>:<line>` — text | R/O |
| --- | --- | --- | --- |
| S1 | 4 doc describes absent behaviour | `doc.go:8` — `//   - Immutable storage with automatic versioning`; `:19`, `:22`, `:37-44`, `:231-237` (versions kept, Delete "preserves history"); probe P4: a second Put replaces the object and purges its chunks | R |
| S2 | 4 doc describes absent behaviour | `doc.go:29` — `//   - "events" port: Publishes storage events (stored, retrieved, deleted)` (only `stored` is published, `component.go:1131`, `:1166`); `:104-106`, `:130-139`, `:172` (`config.EnableCache`, `CacheStrategy`, `SetKeyGenerator`: none exist); README the same | R |
| S3 | 4 exported, no reader | `store.go:531` — `func (s *Store) FetchContent(ctx context.Context, ref *message.StorageReference) (*StoredContent, error) {` with `:582` `extractContentFromBaseMessage`, `:637` `extractTextFields`, `:69` `SetDecoder` | R |
| S4 | 4 exported, no reader | `store.go:699` — `func (s *Store) FetchBinary(ctx context.Context, ref BinaryRef) ([]byte, error) {`; `:404` `GetMetadata` | R |
| S5 | 4 exported, no reader | `content.go:110` — `func (s *StoredContent) GetFieldByRole(role string) string {`; `:119` `HasRole`, `:137` `GetBinaryRefByRole`, `:149` `HasBinaryContent` | R |
| S6 | 4 exported, internal reader only | `content.go:88` — `func NewStoredContent(entityID string, fields, contentFields map[string]string) *StoredContent {`; `:98` `NewStoredContentWithBinary` | R |
| S7 | 3 no-op layer | `content.go:154-163` — `MarshalJSON`/`UnmarshalJSON` through a type alias that only re-enters `encoding/json` | O |
| S8 | 1 second spelling | `store.go:322-324` maps not-found for `Get` only; `:346-349` — `return nil, errs.WrapTransient(err, "Store", "Open", "open object stream")` (a missing object is transient); `:368-371` Delete returns the raw error | R |
| S9 | 4 contract not kept | `store.go:386-399` — `entries, err := s.store.List(ctx)` … `keys = append(keys, entry.Name)`: an empty bucket returns `ErrNoObjectsFound` as a transient error (probe P2) and names come in put order (P7); `storage/storage.go:76-78` promises "lexicographic order" and "an empty slice" | R |
| S10 | 4 contract not kept | `store.go:368` — `err := s.store.Delete(ctx, key)`: a never-written key returns `ErrObjectNotFound` (probe P3); `storage/storage.go:82` promises nil | R |
| S11 | 1 second spelling | `store.go:671-681` — `return fmt.Sprintf("content/%04d/%02d/%02d/%02d/%s_%d", … now.UnixNano(),)` and `:685-695` (binary) beside the UUID nonce at `:788-796`; the code records the residual at `:762-769` | R |
| S12 | 4 test seam in production | `store.go:742-746` — `now func() time.Time` ("Unexported test seam") (5b) | R |
| S13 | 4 exported, no reader outside | `store.go:220` — `func (s *Store) Store(ctx context.Context, msg any) (string, error) {`; `:741` `DefaultKeyGenerator` (5b) | R |
| S14 | 4 dead fields | `store.go:42-43` — `client *natsclient.Client`, `bucketName string` (written at `:177-178`, read nowhere) | O |
| S15 | 4 phantom metric | `metrics.go:26-27` — `objectCount`, `storageBytes` (registered `:192-197`; `updateObjectCount` `:274`, `updateStorageBytes` `:281` have no caller) | R |
| S16 | 1 second spelling | `metrics.go:43` — `Namespace: "semstreams",` (×13) and `:159-205` `registry.RegisterCounterVec(prefix, …)` (removed API; tree: `metric.RegisterOrGet`) | R |
| S17 | 1 misnomer | `metrics.go:47` — `ConstLabels: prometheus.Labels{"kv_bucket": bucket},` on an ObjectStore | O |
| S18 | 1 second spelling | `retention.go:70-126` — the KV guard's twin (`natsclient/kv_retention.go:60-123`), differing in prefix and wording | O |
| S19 | 4 config phantom (5b) | `component.go:184-186` — `if userConfig.DataCache.Enabled \|\| userConfig.DataCache.MaxSize > 0 {`: `"data_cache": {"enabled": false}` keeps the default enabled cache | R |
| S20 | 4 config phantom (5b) | `component.go:188-193` — copies `KeyGenerator`/`MetadataExtractor` from JSON, but both are `json:"-"` (`config.go:44`, `:50`) | R |
| S21 | 4 unknown keys accepted (5b) | `component.go:173` — `if err := json.Unmarshal(rawConfig, &userConfig); err != nil {` | R |
| S22 | 4 back doors (5b) | `component.go:76-81` — `newStore`, `closeStore`, `subscribeCore`, `waitForStreamInput`, `consumeStream`, `waitConsumerClosed` func fields | R |
| S23 | 2 copied shell (5b) | `component.go:47-55`, `:432-682` — own lifecycle state and Start/Stop beside `component.Lifecycle` | R |
| S24 | 4 advertised absent (5b) | `component.go:1219-1225` — `ErrorCount: 0, // Would need error tracking`, `Uptime: time.Since(lastAct)`; `:1236-1240` DataFlow zeros | R |
| S25 | 4 silent skip (5b) | `component.go:726-738`, `:776-781` — non-BaseMessage or non-Graphable: no StoredMessage, Debug log only, no count | R |
| S26 | 4 drop (5b) | `component.go:700-704` — core-NATS write failure is logged and dropped, no count | R |
| S27 | 4 constant identity (5b) | `component.go:148` — `const constructedInstanceName = "objectstore"`: two instances stamp one `StorageInstance` | R |
| S28 | 1 second spelling (5b) | `component.go:947-979` `waitForStream` (fixed 30 tries, `time.After`) beside graph-ingest's | O |
| S29 | 4 exported, no reader (5b) | `component.go:685` `IsStarted`; `:84-85` `messagesReceived`, `messagesStored` (written, never read); `:108` `Event` | R |
| S30 | 1 second spelling (5b) | `component.go:332` `legacyObjectStoreConsumerName` beside `:303-306` the hashed name | O |
| S31 | 1 second spelling (5b) | `stored_message.go:14-87` `buildStoredMessage` (a map decoder beside `UnmarshalJSON` `:177`; drops triple fields it does not name) | O |
| S32 | 4 stale doc (5b) | `stored_message.go:90-91` — "Called from payloadbuiltins.Register" (#33) | R |
| S33 | 5 classification by error text | none: `grep -n -E 'strings\.(Contains\|HasPrefix)\(.*[Ee]rr\|\.Error\(\)'` over the non-test files finds only log arguments | — |

No finding, read in full: `retention.go` beyond S18 (reconcile then re-read and assert, fatal on a binding remainder,
one shared predicate), `register.go` (one registration call), `config.go` beyond S19-S20.

Defects and gates found in the same read (not shapes):

| # | `pin:path:line` — text | What |
| --- | --- | --- |
| G1 | `store.go:84` — `func NewStoreWithConfig(ctx context.Context, client *natsclient.Client, cfg Config) (*Store, error) {` (and every method) | no nil-context refusal at exported boundaries (context rules, `dev`) |
| G2 | `store.go:154-156` — `cache.NewFromConfig[[]byte](ctx, cfg.DataCache,` | a TTL or hybrid cache roots its cleanup goroutine in the constructor's context (`internal/cache/config.go:126-130`): continuing work under a call's context, outside a `Start` (ADR-113). The default LRU starts none (`:123-124`; `NewLRU` takes no context), and no consumer selects TTL or hybrid |
| G3 | `store.go:362-372` — cache delete, then store delete | a `Get` between the two re-caches the object, which then outlives its delete; a cached object also masks a delete made by another process (`storage.ErrObjectNotFound`, #600's reclaim). No current trigger: no consumer calls `Delete`, and semsource's keys are sha256 content addresses (`semsource:processor/ast-source/bodystore.go:25`) |
| G4 | `config.go:102-108` — `Strategy: cache.StrategyLRU, MaxSize: 1000` | the cache bounds entries, not bytes, and stores every write (`store.go:204-206`, `:290-292`, `:512-514`), while `Open`, the registry read path, bypasses it (`:342-349`): a Component holds up to 1,000 recently written objects of any size, read back only by `Get` |
| G5 | `store.go:457-474` | binaries are written one by one; a later failure leaves the earlier ones with no envelope pointing at them (orphans, never deleted) |
| G6 | `store.go:129-136` — `store, err := js.CreateObjectStore(ctx, storeConfig)` … `store, err = js.ObjectStore(ctx, bucketName)` | any create error falls back to open; probe P1: the same config re-creates without error, a different one returns `ErrBucketExists` |
| G7 | `component.go:450` — `runCtx, cancel := context.WithCancel(ctx)` (5b) | run context is Start's child (ADR-113) |
| G8 | `pin:docs/adr/063-store-substrate-and-resolver.md:278-285` vs `store.go:521-526` and `component.go:1123-1139` (5b) | the Component's ContentStorable lane emits a StorageReference to a JSON envelope; ADR-063 says a ref points at raw bytes; graph-embedding then embeds JSON (`pin:graph/embedding/worker.go:1070-1077`) |

## 8. External-provider code at the pin

None. Non-test imports of the nine files: stdlib, `component`, `graph`, `internal/lifecyclecleanup`, `message`,
`metric`, `natsclient`, `payloadregistry`, `pkg/cache`, `pkg/errs`, `pkg/types`, `storage`, `google/uuid`,
`nats.go`, `nats.go/jetstream`, `prometheus/client_golang`. `grep -n -i -E
'net/http|http\.|s3|aws|minio|gcs|azure|llm|openai|provider'` over them: only `component.StoreProvider` and a port
variable named `provider` (`component.go:118`, `:402-409`). Nothing leaves under the tier-0 ruling.

## 9. Collision table: the Store (a durable primitive)

| Dimension | Evidence |
| --- | --- |
| Semantic class | hold large or binary content outside the graph, addressed by `StorageReference{StorageInstance, Key}` |
| Owners | the Store (this change); the Component (5b) owns a Store per instance; consumer-owned readers that open a bucket directly: `semteams:cmd/semteams/tools/emitartifact/derivation.go:982-990` (`jetstream.ObjectStore`, `GetBytes`) |
| Catalogs | none: `graph/kvcatalog` catalogs KV buckets only (`git grep -n -i objectstore -- graph/kvcatalog/*.go ':!*_test.go'`: empty); `natsclient/backing_stream_prefix.go:44-46` (`ResourceObjectStore`) classifies `OBJ_` streams |
| Status | none of its own; `natsclient/storagereport` lists every ObjectStore the account holds (`docs/repository-map.md:66`) |
| Lifecycle | created or opened by the constructor; no expiry (probe P6: `MaxAge=0s MaxBytes=-1`); never deleted by the framework; stream provisioning refuses an `OBJ_` declaration (`natsclient/backing_stream_prefix.go:123-128`) |
| Ownership | one bucket may back several Store values (semsource opens `CONTENT` in four components); the instance name, not the bucket, is the registry key |
| Readers | `storage/storeregistry` resolvers; semsource fusion (`Get`); graph-embedding (`Open`, change 5) |
| Writers | Store `Put`, `StoreContent`; the Component's write lane (5b) |
| Recovery | none: content is not rebuilt; ADR-068's orphan collection and ADR-073's refcount are not implemented at the pin |

## 10. Size against the one-package rule

Non-test pin lines: 3,342 (`component.go` 1,242, `store.go` 799, `doc.go` 346, `metrics.go` 299, `stored_message.go`
222, `content.go` 163, `retention.go` 126, `config.go` 112, `register.go` 33). This change (Store): `store.go`,
`content.go`, `retention.go`, `metrics.go`, `config.go`, `doc.go` = 1,845 (whole `config.go` and `doc.go` counted
here although 5b edits both). Pull request 5b (Component): `component.go`, `register.go`, `stored_message.go` =
1,497. Each is under the bound of about 3,000 (#159). Pin lines that do not enter the tree in this change (S3-S6,
S13 moved to 5b, the cache, the gauges, the `Register*` boilerplate): about 850; tree lines after this change,
estimated: about 1,000.

## 11. Library behaviour the port relies on (#170, ADR-112)

Probe at nats.go v1.54.0 / nats-server v2.15.1 (`go.mod:16-17`), embedded server, JetStream file store, one run, under
`-race`; program, command and output on PR #432, comment 6445520357.

| # | Statement | Result |
| --- | --- | --- |
| P1 | `CreateObjectStore` on an existing bucket | same config: nil; different description: `ErrBucketExists` |
| P2 | `List` on an empty bucket | `ErrNoObjectsFound`, zero entries; also after every object is deleted |
| P3 | missing name | `Delete`, `GetBytes`, `GetInfo`, `Get` return `ErrObjectNotFound` for a never-written name |
| P3c | deleting an already-deleted name | nil, observed; nats.go publishes a fresh delete marker and purges (`jetstream/object.go:956-977`), a write that can fail; not promised |
| P4 | a second `Put` on one name | replaces it: `GetBytes` returns the second value; stream messages 4 → 2 (old chunks purged) |
| P6 | backing stream `OBJ_<bucket>` of `ObjectStoreConfig{Bucket, Description}` | `MaxAge=0s MaxBytes=-1 MaxMsgs=-1`, file storage |
| P7 | `List` order | put order, not sorted (`[k c/2 a/1 b/9 a/0]`); deleted names are not listed |
| P8 | one `jetstream.ObjectStore` handle, 8 goroutines × 20 `PutBytes`/`GetBytes`/`List` | no race reported, every read matched (one run: observed, not a promise) |
| P9 | a cancelled context | `GetBytes` and `PutBytes` return `context canceled` |
| P10 | `js.ObjectStore` on a missing bucket | `ErrBucketNotFound` |

Carried comments not probed (not promised, Bounded port scope): the pin's claim that `time.Now().UnixNano()` repeats
under microsecond quantization (`pin:…/store.go:758-760`).

## 12. Adopter seam inventory

Surfaces reached from outside: `objectstore.Config`, `NewStoreWithConfig`, `NewStoreWithConfigAndMetrics`, the
`Store` methods, `StoredContent`/`BinaryRef` (the envelope a `StoreContent` reference points at).

1. **What must a Store caller know?** (a) set `InstanceName` to the name its resolver uses, or it defaults to the
   bucket name; (b) a missing object is `storage.ErrObjectNotFound` from `Get` and `Open`; (c) `Put` on an existing
   key replaces it (no versions); (d) a `StoreContent` reference points at a JSON envelope, not raw bytes, so it is
   not what ADR-063's raw-body readers (graph-embedding) expect.
2. **If they do nothing:** semsource's calls compile and behave as today; `List` and `Delete` now match
   `storage.Store`'s doc (no caller depends on the old errors); no cache sits in front of `Get` (semsource sets none,
   so its standalone stores had a no-op cache, `internal/cache/config.go:115-116`); (d) a producer that hands a
   `StoreContent` reference to graph-embedding gets JSON embedded, with one Debug line.
3. **Where they find out:** (a) nowhere if wrong (a registry miss is "content-unresolved", ADR-063 #875); (b) typed
   runtime error; (c) doc; (d) log line at Debug.
4. **What should they have to know:** (a) and (d). (a) is 5b's instance-name question (S27); (d) is G8, filed for 5b.

## Intent check

This change moves no boundary in `AGENTS.md` § What this is for: no capability is admitted, deferred or excluded.
The cache removal (owner ruling, PR #432 comment 6446982969) drops a performance knob of an admitted package, not a capability.

## Open evidence questions

- E1. Does any deployment rely on the Component's default LRU cache for `Get` latency (fusion's body resolver is its
  only reader; graph-embedding's `Open` bypasses it)? Not measured; no consumer configures it (section 4).
- E2. Does `UnixNano` repeat between two calls on the CI runner or a consumer host? Not measured; the nonce (D4)
  removes the dependence on it.
