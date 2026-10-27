# Design: setup-04a-04-graph-roots-objectstore-store

Base `5d6042a`. Inventory: `inventory.md` (round 2: `INVENTORY PASS` by the early Claude reviewer; this round answers
its design findings). Change 4, PR 5 of 9, first half (#9 comment 6386856657, item 4: "`storage/objectstore` (Store,
then
Component)"; #9 comment 6386556850, item 1: "objectstore (3,342) splits into the Store, then the Component, and needs
none"). Tier-0 only (#9 comment 6386603659): no provider code (inventory 8). Draft PR #432.
Governing records: pin ADR-063 (carried here), ADR-068, ADR-062, ADR-073, ADR-094 (inventory 6); ADR-108, ADR-113 and
ADR-095 govern the Component in pull request 5b, not the Store. Probes: inventory 11 (P1-P10).

Coined terms: the *Store* is `objectstore.Store`, the NATS ObjectStore backend of `storage.StreamableStore`; the
*Component* is `objectstore.Component`, which pull request 5b ports; the *envelope* is the pin's JSON `StoredContent`
that
`StoreContent` wrote (D4 replaces it with the raw body); the *body* is the text a `ContentStorable` names under the
`body` role (`message.ContentRoleBody`, `message/content_storable.go:80`); the *retention guard* is the boot check that
a content store's backing stream `OBJ_<bucket>`
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
  `:114-115`, `:206`; `storage.go:40`), and the `Put` and `Delete` docs (`storage/storage.go:52-60`, `:80-84`) say
  what NATS does: a second `Put` replaces, a `Delete` removes, no versions are kept (probe P4).
- `docs/adr/063-store-substrate-and-resolver.md`: carried with a port note (D11; #321).
- `.agents/contracts/semengine-developer.md` § Storage and retention and the reviewer twin: the pin's "Large content
  uses backend-neutral `storage.Store` and `StorageReference`" bullet (inventory 3), now that `storage` and the Store
  are ported. The pin's "one bounded backend" is reworded, because the retention guard removes every age and size
  bound from a content store: NATS ObjectStore is one backend; no graph, query or reply contract names its bucket or
  stream, only a `StorageReference`. New rule, so it gets a row in `AGENTS.md` § 4 (or § 2) and in
  `.agents/rules-index.md`, gate `review` (no command enforces it).

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
```

Contract lines: `storage.Store`/`StreamableStore` for the five operations (`storage/storage.go:51`, `:94`); the
`object-store` spec and the package README for `StoreContent` (framework API:
`pin:examples/processors/document/component.go:720-750` calls `StoreContent`, inventory 4); the ledger row.

Not carried (surface audit (a), no reader at the pin, in an admitted package or in a consumer; inventory 4):
`FetchContent`, `extractContentFromBaseMessage`, `extractTextFields`, `SetDecoder` (S3), `FetchBinary`,
`GetMetadata` (S4), the four `StoredContent` role helpers (S5). With D4's raw-body `StoreContent`, nothing writes or
reads the envelope, so `StoredContent`, `BinaryRef`, `NewStoredContent`, `NewStoredContentWithBinary` (S6) and the alias
`MarshalJSON`/`UnmarshalJSON` (S7) go with it, and so does the binary branch of `StoreContent`: no type implements
`message.BinaryStorable` at the pin or in a consumer (`grep -rn 'BinaryFields()'`, non-test: only the interface and
`pin:…/store.go:458`). Optional, taken because the code is rewritten anyway: the dead fields `client`, `bucketName`
(S14) go.

Deferred to 5b. The criterion: symbols whose only production reader at the pin is the Component's raw write lane
(`pin:…/component.go:1154-1158`, `storeWithKeySource`) or its config resolution (`:169-196`): `Store.Store`,
`storeWithKeySource`, `DefaultKeyGenerator` (S12, S13), `Config.Ports`, `Config.KeyGenerator`,
`Config.MetadataExtractor` (S20), `DefaultConfig`. The Store's own operations do not read them. Symbols the Component
reads that are part of the Store itself (`NewStoreWithConfigAndMetrics`, `InstanceName`, `Logger`) stay here, with
their reader arriving in 5b. `Config.Validate` (`pin:…/config.go:61-67`) returns nil and has no caller at the pin: not
carried.

Not carried, by owner ruling (D6): `Config.DataCache` and `Close`.

## D4. Behaviour the Store guarantees (and where the pin differs)

Each line is observable and named in D9's tests.

- **Construction.** Refuses a nil context, a nil client, and a client with no JetStream, each with an invalid-input
  error. Creates the bucket with `ObjectStoreConfig{Bucket, Description}` or, when it exists, opens it (P1: the same
  config re-creates with no error; another config returns `ErrBucketExists`, then the open succeeds). Then runs the
  retention guard (D5). Returns a Store that holds no context and starts nothing. The pin did not refuse nil (G1).
- **Put** replaces any object at the key (P4); the doc says so. The pin's doc promised versions (S1).
- **Get and Open** of a missing object return an error that matches `storage.ErrObjectNotFound` and is not
  transient. At the pin only `Get` did; `Open` returned a transient error (S8). Basis: `storage/storage.go:9-16` ("A
  not-found is deliberately NOT classified as transient").
- **Delete** of a key that was never written returns nil: nats.go returns `ErrObjectNotFound` there (P3), and the
  Store maps it. The pin returned that raw error (S10); this case is the one shown failing against the pin. Basis:
  `storage/storage.go:81` ("Returns nil if the key doesn't exist").
- **Delete** of a key already deleted returned nil in the probe (P3c), observed, not promised: nats.go then publishes
  a fresh delete marker and purges (`jetstream/object.go:956-977`), a write that can fail. The Store passes that
  result through, classifying a failure as transient; the pin did the same, so this case is a regression check only.
- **List** returns the matching names sorted, and an empty, non-nil slice when the bucket is empty or nothing
  matches (P2, P7). Basis: `storage/storage.go:77-78`. It reads every object's metadata, client-side (O(objects));
  the doc says so. The pin errored on an empty bucket and returned put order (S9).
- **StoreContent** obeys ADR-063's read contract: "the store a ref points at holds the RAW body bytes at `Key` …
  *not* a `StoreContent` JSON envelope" (`pin:docs/adr/063-store-substrate-and-resolver.md:278-285`). It validates
  the entity ID first (invalid input, nothing written). It refuses a payload that implements `message.BinaryStorable`
  with any binary field (invalid input, nothing written): binary fields have no place in a raw-body reference, and
  dropping them silently would be an undeclared drop. It then writes the body (`RawContent()[ContentFields()["body"]]`)
  as raw UTF-8 bytes, an empty object when the payload names no body, and returns a reference with the Store's
  instance name, that key, `text/plain; charset=utf-8` and the body's size. Opening the reference returns the body.
  No envelope is written: the other text fields (title, abstract) and the role map had one reader, `FetchContent`,
  which is not carried (D3), and the graph already holds them as triples. At the pin the reference pointed at the
  envelope (`pin:…/store.go:521-526`), so the pin's document adapter
  (`pin:examples/processors/document/component.go:720-724`)
  handed graph-embedding JSON where it reads body text (`pin:graph/embedding/worker.go:1030-1036`, `:1070-1077`); this
  is G8, resolved here. The content key keeps the pin's `content/…/<entity>_…` shape with a UUID nonce in place of the
  `UnixNano` suffix, so two calls never share a key. Optional (S11), taken
  because it is cheap: the pin's `DefaultKeyGenerator` already uses the same nonce, and `google/uuid` is in `go.mod`.
  Its trigger, two calls for one entity within one clock step, depends on clock granularity, a condition the code
  does not choose, and no real-world trigger is measured (E2; #185). The pin records the residual
  (`pin:…/store.go:762-769`).
- **Every exported call** refuses a nil context with an invalid-input error, before any I/O (G1).
- **Context.** Each operation passes its context to nats.go (P9: a cancelled context returns `context canceled`).
- **Concurrency.** A Store is safe for concurrent use, as `storage.Store` requires (`storage/storage.go:34-35`).
  The Store adds no shared mutable state after construction; the jetstream handle's safety is P8 (one run, not a
  library promise), and D9's `-race` test drives it again in the tree.

What stays, not changed (optional, Bounded port scope): create-then-open on any create error (G6); `List` reading all
metadata. G5 (orphan binaries after a partial `StoreContent`) no longer arises: `StoreContent` writes one object.

## D5. Retention guard (ADR-068 as the pin applies it)

As at the pin: after create-or-open, read `OBJ_<bucket>`; if `natsclient.CheckNoLifecycleRetention` finds a TTL or a
binding `MaxBytes`, strip both with one stream update and log a warning naming the bucket and what was removed;
re-read and return a fatal error wrapping `natsclient.ErrGraphBucketRetention` if anything binding remains. A clean
bucket is read once. This matches the KV guard the tree already runs for `RetentionNoLifecycle` buckets
(`natsclient/kv_retention.go:60`, `natsclient/kvspec.go:282-283`). Fresh buckets are already clean (P6:
`MaxAge=0s MaxBytes=-1`), so `AGENTS.md` § 2 ("Non-evictable buckets have no TTL or binding MaxBytes") holds.
Consolidating `retention.go` with `natsclient.ReconcileNoLifecycleRetention` (S18) is optional and not done: it
would change `natsclient`'s exported function to take a prefix.

## D6. The data cache and `internal/cache` (ruled: dropped)

At the pin `Config.DataCache cache.Config` (`pin:…/config.go:38`) is public; in this tree that type is
`internal/cache.Config`, and a public field may not name an `internal/` type (`harness-boundaries`; `AGENTS.md` § 1).
Facts:

- No consumer sets `data_cache` (inventory 4). semsource's standalone Stores pass a zero config, which yields a no-op
  cache (`internal/cache/config.go:115-116`). Only the Component's default turns it on: LRU, 1,000 entries
  (`pin:…/config.go:102-108`); a `"data_cache": {"enabled": false}` cannot turn it off (S19).
- The default cache stores every write: `Put`, the raw lane and `StoreContent` each `Set` the bytes
  (`pin:…/store.go:204-206`, `:290-292`, `:512-514`). The registry's read path, `Open` (graph-embedding), bypasses it
  (`:342-349`). Only `Get` (fusion's body resolver) reads from it. So a semteams or semsource Component holds up to
  1,000 recently written objects of any size in memory, counted by entries, not bytes, mostly never read back.
- The default LRU starts no goroutine (`internal/cache/config.go:121-124`; `NewLRU` takes no context). Only the TTL
  and hybrid strategies root a goroutine in the constructor's context (G2), and no consumer selects either.
- A cached object outliving a delete (G3) has no current trigger: no consumer calls `Delete`, and semsource's keys
  are sha256 content addresses (`semsource:processor/ast-source/bodystore.go:25`, `:112`).

**Ruled (owner ruling, PR #432 comment 6446982969): drop the cache.** The Store carries no `internal/cache` import, no
`Config.DataCache`, no cache
metrics and no `Close` (it then owns no resource; `jetstream` handles need no close), and the Component (5b) does not
reinstate it. `internal/cache` stays internal and loses one importer. Each `Get` reads NATS, one round trip; memory no
longer grows with recent writes. 5b's Component refuses a `data_cache` key as unknown; no consumer config carries one.
Moving `internal/cache` back to `pkg/cache`, or adding a public mirror type of its config, were not taken.

ADR-063's "on Stop, the objectstore Component `Close()`s its store" (`:157-158`) and "the registrant owns `Close`"
(`:256`) no longer describe anything: the port note says so (D11).

## D7. Metrics

Collectors register on the registry given to `NewStoreWithConfigAndMetrics` through `metric.RegisterOrGet`, namespace
`semengine`, subsystem `objectstore` (S16; `metric-registry` spec). `NewStoreWithConfig` records nothing. Carried:
operation counters (read, write, delete, list, by operation), latency histograms, and `operation_errors_total` (one
count per call that returns an error; there are no retries inside the Store, so a count is an outcome). Not carried:
`object_count` and `storage_bytes` (registered, never set, S15) and the cache hit and miss counters (D6).
The bucket is a variable label `bucket` on each collector, registered once per process under one `RegisterOrGet`
key, not the pin's constant `kv_bucket` label under a per-bucket key (`pin:…/metrics.go:47`, `:156-162`). A constant
label would make a second bucket's candidate differ in descriptor and fail construction (`metric/registry.go:44-47`).
The rename from `kv_bucket` is optional (S17), free because the namespace change already renames every series and no
consumer reads them (inventory 4). Two Stores on one bucket, or on two buckets, share the collectors.

## D8. Context and lifecycle (ADR-113, ADR-108)

Settled; applied, not reopened. With no cache (D6) the Store is not a lifecycle owner: its constructor does its work
under
the context it is given and keeps neither the context nor any goroutine. ADR-113's run-context rule and
`component.Lifecycle` (ADR-108) bind the Component in 5b, where the inventory's G7, S22, S23 and S24 are fixed.

## D9. Proving tests

Unit (no Docker): `TestNewStoreRefusesNilContext`, `TestStoreMethodsRefuseNilContext` (table over the seven
exported calls), `TestConfigDefaults` (bucket `MESSAGES`, instance defaults to bucket),
`TestStoreContentRefusesInvalidEntityIDBeforeWriting`, `TestStoreContentRefusesBinaryFields`,
`TestStoreContentKeysAreUniquePerCall` (10,000 keys for one
entity and field, no repeat; independent of the clock), metrics: `TestStoreMetricsRegisterOnInjectedRegistry` (names
under `semengine_objectstore_`, no `object_count`), `TestNilRegistryRecordsNothing`.

Integration (`natsfixture`, `//go:build integration`, shared server via the reset; #195):
`TestPutReplacesObjectAtKey`, `TestGetAndOpenReportMissingAsNotFound` (`errors.Is(err, storage.ErrObjectNotFound)`
and `!errs.IsTransient(err)` for both), `TestDeleteMissingKeySucceeds` (never written, failing against the pin; deleted
twice, a
regression check),
`TestListSortedAndEmpty` (empty bucket → empty slice, nil error; names put out of order come back sorted; prefix
filter), `TestStoreContentReferenceOpensToBody` (the product path: a `ContentStorable` with a title, an abstract and a
body is stored, the returned reference's `StorageInstance` resolves through a `storeregistry.Registry` holding the
Store,
`Open(ref.Key)` returns exactly the body bytes, not JSON, and `ContentType` and `Size` match; failing against the pin's
shape), `TestStoreContentWithoutBodyStoresEmptyObject`, `TestStoreHistoryMatchesModel` (below),
`TestRetentionGuardStripsAndAsserts` (a bucket created with `MaxAge` and
`MaxBytes` is stripped and logged; the pin's `retention_integration_test.go` ported), `TestRetentionGuardFailsClosed`
(the pin's `TestReconcileNoLifecycleRetention` fatal case, ported), `TestStoreConcurrentUseUnderRace`
(8 goroutines Put/Get/List on one Store under `-race`), `TestFreshBucketHasNoLifecycleRetention`
(`OBJ_<bucket>` `MaxAge` 0, `MaxBytes` ≤ 0),
`TestGetReadsNATSEveryTime` (no cached copy, D6), `TestTwoStoresOnOneBucketShareCollectors`,
`TestTwoBucketsOnOneRegistryBothConstruct` (the variable label, D7). Each
test is shown failing against the pin's shape where the pin differs
(S8, S9, the never-written half of S10, G1), recorded in `notes/2.1.md`. Ported pin tests rewritten without `t.Parallel`
in integration files and
through the fixture; dropped pin tests named with reasons in `notes/2.1.md`.

Generated checks (`docs/testing.md:417-430`): the Store's promises are a history (replace, delete, delete twice, list
by prefix, reads from a second Store on the bucket), so one is used. `TestStoreHistoryMatchesModel` (integration,
`pgregory.net/rapid` v1.3.0, already in `go.mod`) draws up to 30 operations over four keys under two prefixes (`a/1`,
`a/2`, `b/1`, `b/10`) and two values each: `Put`, `Delete`, `Get` and `List(prefix)`, each on one of two Stores over
one bucket. A plain `map[string][]byte` is the model: `Put` sets, `Delete` removes, `Get` of an absent key must match
`storage.ErrObjectNotFound`, `List` must equal the model's sorted matching keys. The key set makes prefix overlap
(`b/1`, `b/10`), replace, delete-twice and delete-then-put reachable in every run. Bounded at 50 runs; the seed and the
completed-run count are recorded in `notes/2.1.md`, and a failure prints its replayable sequence. The named examples
stay for the boundaries they claim (empty bucket, never-written delete, not-found classification).

Coverage: `storage/objectstore merged` joins `scripts/cover-check.sh` at 80% (the Store is a consumer-facing
storage primitive, like `storage/storeregistry`).

## D10. Probes

Inventory 11. The program, command, versions and output are on PR #432, comment 6445520357 (task 1.2). Every library
statement in D4, D5 and the spec cites P1-P10; P3c and P8 are stated as observed runs, not promises, and the tree's own
`-race` test (D9) is the standing check for P8.

## D11. Ledger, records, docs

- New ledger row `storage/objectstore`, `disposition: adapt`, contract "the Store half (this change); the Component
  arrives in setup-04a-04-graph-roots-objectstore-component", with each adaptation above numbered, `known_risks`
  naming S27 as 5b's disposition, `StoreContent`'s raw-body change (G8 resolved) and the deferred symbols.
- `storage` row: `carry` → `adapt` (doc.go and storage.go edited for #280 item 1, and `Put`'s and `Delete`'s docs no
  longer claim versions, probe P4).
- The docs that describe a Store `Close` (D6): `storage/storeregistry/storeregistry.go:19-21`, `:74-75`
  (the owner closes the store on Stop) and `storage/doc.go:86`, `:208` (`defer store.Close()`); the
  `storage/storeregistry` row becomes `adapt` for the comment edit.
- `internal/cache` row: importer count updated (objectstore no longer imports it, D6).
- `docs/adr/063-store-substrate-and-resolver.md` with a port note (the ADR-074 note's shape): pin path and SHA; carried
  with this change; what is ported (`storage`, `storage/storeregistry`, the Store; the Component in 5b); decisions
  kept (instance name as `StorageInstance`, the registry, the raw-body read contract); what changed (by the D6
  ruling, the Store has no `Close`, so `:157-158` and `:256` describe nothing here); what is open (multi-instance
  naming,
  `:412-413`, decided in 5b); the read contract now kept by `StoreContent` too (G8 resolved, D4). Its own ledger row.
  Closes #321.
- ADR-068: not carried (graph-index made the same call); its one decision this package implements is already the
  tree's rule (`AGENTS.md` § 2, `natsclient/kv.go:179`); orphan collection is not implemented at the pin.
- `doc.go` and `README.md` say only what ships (S1, S2): no versions, replace on `Put`, sorted client-side `List`,
  no cache, `StoreContent` storing the raw body only, a compiled example (`ExampleNewStoreWithConfig` in a test file).
- `docs/repository-map.md`: a `storage/objectstore` row.

## D12. Spec delta

New capability `object-store` (`specs/object-store/spec.md`): construction, retention guard, `Put`, `Get`/`Open`
not-found, `Delete`, the history model, `List`, `StoreContent`'s raw-body reference, nil context, metrics. Every
requirement is under the validator's 500
characters and names its test.

## D13. Issues and merge order

| Issue | Disposition |
| --- | --- |
| #321 | retired: `Closes #321` (task 4.2 carries ADR-063) |
| #280 | item 1 done here (task 4.3); item 3 (`pkg/lifecycle`) stays, so the PR says "advances #280", no closing keyword |
| #195 | advanced: this change's integration tests use the fixture; the Component's two sites are 5b's |
| #33 | stays: the last site is `stored_message.go:90`, which 5b carries; 5b closes it |
| #196 | stays: the Component row adopts through `component.Lifecycle` in 5b |
| #281 | stays: objectstore's refused `category:performance` tag is on `DataCache` (gone, D6) |
| #36, #216 | stay: the Component's write consumer (5b) names both |

Filed by this design: none. G5 no longer arises and G8 is resolved here (D4); the Component's ContentStorable lane
calls `StoreContent`, so in 5b it emits raw-body references with no change of its own. Not filed: S27 (the constant
instance name, which ADR-063 leaves open, `:349-354`, `:412-413`, and the pin calls an owner question,
`component.go:140-147`); its code enters with 5b, whose design decides it from inventory 7.

Consumer check for D4 (pin and the SHAs in the inventory header): `StoreContent` callers are the pin's document adapter
(`pin:examples/processors/document/component.go:720-750`) and the Component (5b); no consumer calls it. Readers of the
envelope: `FetchContent` only (not carried); no consumer names `StoredContent`, `FetchContent`, `content_fields` or
`binary_refs` (`grep -rn --include='*.go'`, non-test, four trees: empty). The one reader of what the reference points
at, graph-embedding, expects the raw body. No reader breaks.

Merge order: #431 touches only graph-ingest's OpenSpec change; either may merge first. 5b follows this change.

## D14. Decisions kept, changed, dropped

- Kept: ADR-063's instance name as `StorageInstance` and its raw-body read contract, now also for `StoreContent`;
  ADR-068's no-lifecycle rule on
  content stores via the pin's guard; ADR-062's one-backend-of-`storage.Store` stance.
- Changed: `storage.Store`'s documented `List` and never-written `Delete` behaviour is now implemented (the pin's code
  broke its own
  interface doc); `Open` joins `Get` in reporting not-found; `StoreContent` keys carry a nonce; `StoreContent` writes
  and references the
  raw body, not the pin's envelope, and refuses binary fields (D4).
- Dropped (owner ruling, PR #432 comment 6446982969): the data cache and `Close`, which changes ADR-063 `:157-158`,
  `:256` as noted in D11.

## Rulings

- The data cache is dropped (D6; owner ruling, PR #432 comment 6446982969): no `internal/cache`, no `Config.DataCache`,
  no `Store.Close`, and the
  Component in 5b does not reinstate it.
- The split is ruled (#9 comment 6386556850, item 1). ADR-113 and ADR-108 are settled and only applied (D8).
