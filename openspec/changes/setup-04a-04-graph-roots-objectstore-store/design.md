# Design: setup-04a-04-graph-roots-objectstore-store

Base `5d6042a`. Inventory: `inventory.md` (round 1, no `INVENTORY PASS` yet; drafted in one pass at the caller's
direction). Change 4, PR 5 of 9, first half (#9 comment 6386856657, item 4: "`storage/objectstore` (Store, then
Component)"; #9 comment 6386556850, item 1: "objectstore (3,342) splits into the Store, then the Component, and needs
none"). Tier-0 only (#9 comment 6386603659): no provider code (inventory 8). Draft PR #432.
Governing records: pin ADR-063 (carried here), ADR-068, ADR-062, ADR-073, ADR-094 (inventory 6); ADR-108, ADR-113 and
ADR-095 govern the Component in pull request 5b, not the Store. Probes: inventory 11 (P1-P10).

Coined terms: the *Store* is `objectstore.Store`, the NATS ObjectStore backend of `storage.StreamableStore`; the
*Component* is `objectstore.Component`, which pull request 5b ports; the *envelope* is the JSON `StoredContent` that
`StoreContent` writes; the *retention guard* is the boot check that a content store's backing stream `OBJ_<bucket>`
has no TTL and no binding `MaxBytes`.

## Options

- **A. One pull request, Store and Component.** 3,342 pin lines, over the bound; contradicts the owner's split.
- **B. Store first, then Component (chosen, ruled).** Each half under the bound. The Store has a consumer caller
  today (semsource, inventory 4), so it lands with a reader; the Component's readers of the Store's
  `NewStoreWithConfigAndMetrics`, `InstanceName` and `Logger` arrive in 5b.
- **C. Component first.** The Component cannot build without the Store.
- **D. Do nothing now.** graph-embedding (pull request 5 resolves content through the store registry) and change 5
  read stores the Component provides; the order stalls.

## D1. Unit of work and size

One pull request, one OpenSpec change. Pin lines carried: 1,845 (`store.go` 799, `doc.go` 346, `metrics.go` 299,
`content.go` 163, `retention.go` 126, `config.go` 112; inventory 10), of which about 850 do not enter the tree (D3,
D6). The tree gains about 1,000. Under the bound (#159); no waiver (ruling 6386556850, item 1). Pull request 5b
(`setup-04a-04-graph-roots-objectstore-component`, its own OpenSpec change) carries `component.go`, `register.go`
and `stored_message.go` (1,497), plus the deferred symbols in D3, and starts from inventory 7's 5b rows.

## D2. Packages and files

- `storage/objectstore/` (package `objectstore`): `doc.go`, `config.go`, `store.go`, `content.go`, `retention.go`,
  `metrics.go`, `README.md`, and their tests. No `component.go`, `register.go`, `stored_message.go` (5b).
- `storage/doc.go` and `storage/storage.go`: the objectstore examples show calls that exist (#280 item 1: `:82`,
  `:114-115`, `:206`; `storage.go:40`), and `Put`'s doc says what NATS does (probe P4: a second `Put` replaces).
- `docs/adr/063-store-substrate-and-resolver.md`: carried with a port note (D11; #321).
- `.agents/contracts/semengine-developer.md` § Storage and retention and the reviewer twin: the pin's "Large content
  uses backend-neutral `storage.Store` and `StorageReference`" bullet (inventory 3), now that `storage` and the Store
  are ported.

## D3. Surface: what ports, what does not

Ports, as at the pin unless D4 says otherwise:

```go
type Config struct {
    BucketName   string       `json:"bucket_name"` // default "MESSAGES"
    InstanceName string       `json:"-"`           // stamped into StorageReference.StorageInstance; default BucketName
    Logger       *slog.Logger `json:"-"`           // the retention guard's warning; nil means slog.Default()
}

func NewStoreWithConfig(ctx context.Context, client *natsclient.Client, cfg Config) (*Store, error)
func NewStoreWithConfigAndMetrics(ctx context.Context, client *natsclient.Client, cfg Config,
    registry *metric.MetricsRegistry) (*Store, error)
func (s *Store) Put(ctx context.Context, key string, data []byte) error
func (s *Store) Get(ctx context.Context, key string) ([]byte, error)
func (s *Store) Open(ctx context.Context, key string) (io.ReadCloser, error)
func (s *Store) List(ctx context.Context, prefix string) ([]string, error)
func (s *Store) Delete(ctx context.Context, key string) error
func (s *Store) StoreContent(ctx context.Context, cs message.ContentStorable) (*message.StorageReference, error)
func (s *Store) InstanceName() string
type StoredContent struct { /* EntityID, Fields, BinaryRefs, ContentFields, StoredAt, as at the pin */ }
type BinaryRef struct { /* ContentType, Size, Key */ }
```

Contract lines: `storage.Store`/`StreamableStore` for the five operations (`storage/storage.go:51`, `:94`); the
`object-store` spec and the package README for `StoreContent`, `StoredContent` and `BinaryRef` (framework API:
`pin:examples/processors/document/component.go:720-750` calls `StoreContent`, inventory 4); the ledger row.

Not carried (surface audit (a), no reader at the pin, in an admitted package or in a consumer; inventory 4):
`FetchContent`, `extractContentFromBaseMessage`, `extractTextFields`, `SetDecoder` (S3), `FetchBinary`,
`GetMetadata` (S4), the four `StoredContent` role helpers (S5). A reader of a `BinaryRef` calls `Get(ref.Key)`.
`NewStoredContent` and `NewStoredContentWithBinary` become unexported (S6). Optional, taken because the code is
rewritten anyway: the alias `MarshalJSON`/`UnmarshalJSON` (S7) and the dead fields `client`, `bucketName` (S14) go.

Deferred to 5b, because only the Component's raw write lane reads them: `Store.Store`, `storeWithKeySource`,
`DefaultKeyGenerator` (S12, S13), `Config.Ports`, `Config.KeyGenerator`, `Config.MetadataExtractor` (S20),
`DefaultConfig`. Carrying them here would land code whose only caller is a test (shape 4).

Owner question 1 (D6): `Config.DataCache` and `Close`.

## D4. Behaviour the Store guarantees (and where the pin differs)

Each line is observable and named in D9's tests.

- **Construction.** Refuses a nil context, a nil client, and a client with no JetStream, each with an invalid-input
  error. Creates the bucket with `ObjectStoreConfig{Bucket, Description}` or, when it exists, opens it (P1: the same
  config re-creates with no error; another config returns `ErrBucketExists`, then the open succeeds). Then runs the
  retention guard (D5). Returns a Store that holds no context and starts nothing. The pin did not refuse nil (G1).
- **Put** replaces any object at the key (P4); the doc says so. The pin's doc promised versions (S1).
- **Get and Open** of a missing object return an error that matches `storage.ErrObjectNotFound` and is not
  transient. At the pin only `Get` did; `Open` returned a transient error (S8), so a reader such as graph-embedding,
  which opens through the registry (ADR-063), retried an object that will never come back.
  Basis: `storage/storage.go:9-16` ("A not-found is deliberately NOT classified as transient").
- **Delete** of a key that was never written, or is already deleted, returns nil (P3); other failures are transient.
  Basis: `storage/storage.go:82` ("Returns nil if the key doesn't exist"). The pin returned the raw error (S10).
- **List** returns the matching names sorted, and an empty, non-nil slice when the bucket is empty or nothing
  matches (P2, P7). Basis: `storage/storage.go:77-78`. It reads every object's metadata, client-side (O(objects));
  the doc says so. The pin errored on an empty bucket and returned put order (S9).
- **StoreContent** validates the entity ID first (invalid input, nothing written); writes each binary field, then
  the envelope; returns a reference with the Store's instance name, the envelope key, `application/json` and the
  envelope size. Content and binary keys keep the pin's `content/…/<entity>_…` and `binary/…/<entity>_<field>_…`
  shape with a UUID nonce in place of the `UnixNano` suffix, so two calls never share a key. Basis: content
  preservation (admission gate, `.agents/protocol.md` § Bounded port scope); the pin's own comment names the
  same-entity overwrite residual (`pin:…/store.go:762-769`), and a replaced envelope silently changes what an
  earlier reference returns. Smallest fix: the nonce the pin's `DefaultKeyGenerator` already uses (S11).
- **Every exported call** refuses a nil context with an invalid-input error, before any I/O (G1).
- **Context.** Each operation passes its context to nats.go (P9: a cancelled context returns `context canceled`).
- **Concurrency.** A Store is safe for concurrent use, as `storage.Store` requires (`storage/storage.go:34-35`).
  The Store adds no shared mutable state after construction; the jetstream handle's safety is P8 (one run, not a
  library promise), and D9's `-race` test drives it again in the tree.

What stays, not changed (optional, Bounded port scope): create-then-open on any create error (G6); `List` reading all
metadata; partial `StoreContent` leaving orphan binaries on a later failure (G5), filed as an issue (D13).

## D5. Retention guard (ADR-068 as the pin applies it)

As at the pin: after create-or-open, read `OBJ_<bucket>`; if `natsclient.CheckNoLifecycleRetention` finds a TTL or a
binding `MaxBytes`, strip both with one stream update and log a warning naming the bucket and what was removed;
re-read and return a fatal error wrapping `natsclient.ErrGraphBucketRetention` if anything binding remains. A clean
bucket is read once. This matches the KV guard the tree already runs for `RetentionNoLifecycle` buckets
(`natsclient/kv_retention.go:60`, `natsclient/kvspec.go:282-283`). Fresh buckets are already clean (P6:
`MaxAge=0s MaxBytes=-1`), so `AGENTS.md` § 2 ("Non-evictable buckets have no TTL or binding MaxBytes") holds.
Consolidating `retention.go` with `natsclient.ReconcileNoLifecycleRetention` (S18) is optional and not done: it
would change `natsclient`'s exported function to take a prefix.

## D6. The data cache and `internal/cache` (owner question 1)

At the pin `Config.DataCache cache.Config` (`pin:…/config.go:38`) is public; in this tree that type is
`internal/cache.Config`, and a public field may not name an `internal/` type (`harness-boundaries`; `AGENTS.md` § 1).
Facts: no consumer sets `data_cache` (inventory 4); semsource's standalone Stores pass a zero config, which yields a
no-op cache (`internal/cache/config.go:115-116`); only the Component's default turns it on (LRU, 1,000 entries,
`pin:…/config.go:102-108`), and a `"data_cache": {"enabled": false}` cannot turn it off (S19). The cache bounds
entries, not bytes (G4); a TTL or hybrid cache roots a goroutine in the constructor's context (G2, ADR-113); a
cached object can outlive its delete (G3).

- **Option 1 (recommended): drop the cache.** No `DataCache`, no cache metrics, no `Close` (the Store then owns no
  resource; `jetstream` handles need no close). `internal/cache` stays internal and loses one importer. `Get` always
  reads NATS, one round trip; no one has measured that the cache matters (E1). 5b's Component refuses a
  `data_cache` key as unknown; no consumer config carries one. Removing admitted surface by name needs the owner's
  ruling (architect contract, Surface audit).
- **Option 2: keep the cache behind the Component only.** Public `Config` has no cache field; the Component (5b)
  keeps an unexported config with `data_cache` and passes it to an unexported constructor; the Store keeps `Close`,
  and the cache's goroutine must move off the constructor's context (G2). Pin behaviour kept; G3, G4, S19 stay to
  fix.
- **Option 3: move `internal/cache` back to `pkg/cache`.** Reverses the floor's design D5; a whole cache package
  becomes public API for one field. Not recommended.
- **Option 4: a public objectstore mirror type** (`CacheConfig`) mapped to `internal/cache.Config`: a second
  spelling of one config. Not recommended.

Under Option 1, ADR-063's "on Stop, the objectstore Component `Close()`s its store" (`:157-158`) and "the registrant
owns `Close`" (`:256`) no longer describe anything: the port note says so (D11).

## D7. Metrics

Collectors register on the registry given to `NewStoreWithConfigAndMetrics` through `metric.RegisterOrGet`, namespace
`semengine`, subsystem `objectstore` (S16; `metric-registry` spec). `NewStoreWithConfig` records nothing. Carried:
operation counters (read, write, delete, list, by operation), latency histograms, and `operation_errors_total` (one
count per call that returns an error; there are no retries inside the Store, so a count is an outcome). Not carried:
`object_count` and `storage_bytes` (registered, never set, S15) and, under Option 1, the cache hit and miss counters.
The bucket label is `bucket`, not the pin's `kv_bucket` (S17, optional; free here because the namespace change already
renames every series, and no consumer reads them, inventory 4). Two Stores on one bucket share collectors
(`RegisterOrGet` returns the held one); two buckets are two label values.

## D8. Context and lifecycle (ADR-113, ADR-108)

Settled; applied, not reopened. Under Option 1 the Store is not a lifecycle owner: its constructor does its work under
the context it is given and keeps neither the context nor any goroutine. ADR-113's run-context rule and
`component.Lifecycle` (ADR-108) bind the Component in 5b, where the inventory's G7, S22, S23 and S24 are fixed.

## D9. Proving tests

Unit (no Docker): `TestNewStoreRefusesNilContext`, `TestStoreMethodsRefuseNilContext` (table over the seven
exported calls), `TestConfigDefaults` (bucket `MESSAGES`, instance defaults to bucket),
`TestStoreContentRefusesInvalidEntityIDBeforeWriting`, `TestStoreContentKeysAreUniquePerCall` (10,000 keys for one
entity and field, no repeat; independent of the clock), metrics: `TestStoreMetricsRegisterOnInjectedRegistry` (names
under `semengine_objectstore_`, no `object_count`), `TestNilRegistryRecordsNothing`.

Integration (`natsfixture`, `//go:build integration`, shared server via the reset; #195):
`TestPutReplacesObjectAtKey`, `TestGetAndOpenReportMissingAsNotFound` (`errors.Is(err, storage.ErrObjectNotFound)`
and `!errs.IsTransient(err)` for both), `TestDeleteMissingKeySucceeds` (never written; deleted twice),
`TestListSortedAndEmpty` (empty bucket → empty slice, nil error; names put out of order come back sorted; prefix
filter), `TestStoreContentWritesBinariesThenEnvelope` (reference fields; envelope decodes to `StoredContent` whose
`BinaryRefs` keys `Get` back the bytes), `TestRetentionGuardStripsAndAsserts` (a bucket created with `MaxAge` and
`MaxBytes` is stripped and logged; the pin's `retention_integration_test.go` ported), `TestRetentionGuardFailsClosed`
(the pin's `TestReconcileNoLifecycleRetention` fatal case, ported), `TestStoreConcurrentUseUnderRace`
(8 goroutines Put/Get/List on one Store under `-race`), `TestFreshBucketHasNoLifecycleRetention`
(`OBJ_<bucket>` `MaxAge` 0, `MaxBytes` ≤ 0),
`TestTwoStoresOnOneBucketShareCollectors`. Each test is shown failing against the pin's shape where the pin differs
(S8-S11, G1), recorded in `notes/2.1.md`. Ported pin tests rewritten without `t.Parallel` in integration files and
through the fixture; dropped pin tests named with reasons in `notes/2.1.md`.

Coverage: `storage/objectstore merged` joins `scripts/cover-check.sh` at 80% (the Store is a consumer-facing
storage primitive, like `storage/storeregistry`).

## D10. Probes

Inventory 11. The program and its output are posted on PR #432 (task 1.2) so the evidence opens on another machine;
`/tmp/os-probe` is local only. Every library statement in D4, D5 and the spec cites P1-P10; P8 is stated as one
observed run, and the tree's own `-race` test (D9) is the standing check.

## D11. Ledger, records, docs

- New ledger row `storage/objectstore`, `disposition: adapt`, contract "the Store half (this change); the Component
  arrives in setup-04a-04-graph-roots-objectstore-component", with each adaptation above numbered, `known_risks`
  naming G5's issue, G8's issue for 5b, and the deferred symbols.
- `storage` row: `carry` → `adapt` (doc.go and storage.go edited for #280 item 1 and `Put`'s replace semantics).
- `internal/cache` row: importer count updated under Option 1.
- `docs/adr/063-store-substrate-and-resolver.md` with a port note (the ADR-074 note's shape): pin path and SHA; carried
  with this change; what is ported (`storage`, `storage/storeregistry`, the Store; the Component in 5b); decisions
  kept (instance name as `StorageInstance`, the registry, the raw-body read contract); what changed (under Option 1,
  the Store has no `Close`, so `:157-158` and `:256` describe nothing here); what is open (multi-instance naming,
  `:412-413`, decided in 5b); the conflict G8 named with its issue. Its own ledger row. Closes #321.
- ADR-068: not carried (graph-index made the same call); its one decision this package implements is already the
  tree's rule (`AGENTS.md` § 2, `natsclient/kv.go:179`); orphan collection is not implemented at the pin.
- `doc.go` and `README.md` say only what ships (S1, S2): no versions, replace on `Put`, sorted client-side `List`,
  no cache, the envelope format, a compiled example (`ExampleNewStoreWithConfig` in a test file).
- `docs/repository-map.md`: a `storage/objectstore` row.

## D12. Spec delta

New capability `object-store` (`specs/object-store/spec.md`): construction, retention guard, `Put`, `Get`/`Open`
not-found, `Delete`, `List`, `StoreContent`, nil context, metrics. Every requirement is under the validator's 500
characters and names its test.

## D13. Issues and merge order

| Issue | Disposition |
| --- | --- |
| #321 | retired: `Closes #321` (task 5.1 carries ADR-063) |
| #280 | item 1 done here (task 5.2); item 3 (`pkg/lifecycle`) stays, so the PR says "advances #280", no closing keyword |
| #195 | advanced: this change's integration tests use the fixture; the Component's two sites are 5b's |
| #33 | stays: the last site is `stored_message.go:90`, which 5b carries; 5b closes it |
| #196 | stays: the Component row adopts through `component.Lifecycle` in 5b |
| #281 | stays: objectstore's refused `category:performance` tag is on `DataCache` (gone under Option 1; else 5b's) |
| #36, #216 | stay: the Component's write consumer (5b) names both |

Filed by this design (protocol File step; linked without a closing keyword): G5 (orphan binaries after a partial
`StoreContent`) as optional maintenance; G8 (the Component's ContentStorable lane points a `StorageReference` at a
JSON envelope, against ADR-063's read contract) for 5b; S27 (the constant instance name) for 5b.

Merge order: #431 touches only graph-ingest's OpenSpec change; either may merge first. 5b follows this change.

## D14. Decisions kept, changed, dropped

- Kept: ADR-063's instance name as `StorageInstance` and its raw-body read contract; ADR-068's no-lifecycle rule on
  content stores via the pin's guard; ADR-062's one-backend-of-`storage.Store` stance.
- Changed: `storage.Store`'s documented `List` and `Delete` behaviour is now implemented (the pin's code broke its own
  interface doc); `Open` joins `Get` in reporting not-found; `StoreContent` keys carry a nonce.
- Dropped (owner question 1): the data cache and `Close`, which changes ADR-063 `:157-158`, `:256` as noted in D11.

## Rulings

Owner question for acceptance: question 1 (D6). The split itself is ruled (#9 comment 6386556850, item 1); ADR-113 and
ADR-108 are settled and only applied (D8).
