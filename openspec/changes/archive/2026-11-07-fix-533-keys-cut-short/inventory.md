# Inventory: fix-533-keys-cut-short

base: 20b2b958193f18a10af4f03487107e014e432d4a (origin/main; claim commit afbdef1 on top, draft PR #541)
nats.go pin: `go.mod:17` — `github.com/nats-io/nats.go v1.55.0`; library paths are under
`$GOMODCACHE/github.com/nats-io/nats.go@v1.55.0/`. **read** = source read only, not probed.

## 1. The claimed gap (read)

- `jetstream/kv.go:1403-1423`: `Keys` adds `IgnoreDeletes(), MetaOnly()`, calls `WatchAll`, and ranges
  `watcher.Updates()` until `entry == nil`. A channel that closes before the marker ends the loop with the keys
  gathered so far. It returns `ErrNoKeysFound` when none were gathered (`:1418-1419`), and otherwise
  `slices.Sort` + `slices.Compact` (`:1421-1422`). So a listing cut short before its first key reads as an empty
  bucket.
- `WatchAll` is `kv.Watch(ctx, AllKeys)` (`:1398-1399`), `AllKeys = ">"` (`:495`), and `Watch` is
  `WatchFiltered(ctx, []string{keys})` (`:1393-1394`). So `keysByFilter(ctx, r, ">")` opens the same watch with the
  same options as `Keys` (`natsclient/kv.go:653` — `reader.WatchFiltered(ctx, []string{pattern},
  jetstream.IgnoreDeletes(), jetstream.MetaOnly())`). The wire should be unchanged, by reading.
- Newer nats.go: v1.55.0 (2026-10-23) is the latest release (`gh api repos/nats-io/nats.go/releases`). `Keys` on
  nats.go `main` is byte-identical to `:1403-1423`. Not fixed upstream.

## 2. Production callers (complete list)

`git grep -n -E '\.Keys\(' -- '*.go' ':!*_test.go'` gives the nine below (the other hits are `maps.Keys`).
`gopls references natsclient/kv.go:572:20` (KVStore.Keys) gives six of them: key_family, manager ×2, clustering,
embedding, graph-index. `gopls references graph/kvcatalog/kvcatalog.go:279:2` (CatalogReader.Keys) gives
graph_triples and message_logger. Only `natsclient/kv.go:579` and the two service sites call nats.go `Keys`
directly. No `ListKeys` production caller remains (`git grep -n ListKeys -- '*.go' ':!*_test.go'`: one doc line).

| Site | Static type | What a short list does |
| --- | --- | --- |
| `natsclient/kv.go:579` — `keys, err := kv.bucket.Keys(ctx)` | `jetstream.KeyValue` (`kv.go:49`), has `WatchFiltered` | Passes through. `:581-583` maps `ErrNoKeysFound` to nil,nil, so zero keys cut short reads as an empty bucket |
| `config/key_family.go:213` — `keys, err := store.Keys(ctx)` | `*natsclient.KVStore` (`:150`) | `Names` replies with a subset. No production caller (only `key_family_integration_test.go:103,129`, `key_family_test.go:71`) |
| `config/manager.go:1179` — `keys, err := kvStore.Keys(ctx)` | `*natsclient.KVStore` | Boot. Record listed with no config keys → `hasConfig=false` (`:1197`) → `arbitrate` pushes the file over the bucket (`:447-450`). Empty → mint → `Create` exists → adopt (`:1272-1274`), still `false`. Config keys without the record → refuses with a wrong diagnosis (`:1199-1210`) |
| `config/manager.go:1481` — `keys, err := kvStore.Keys(ctx)` | `*natsclient.KVStore` | `syncFromKV` replaces `Services`/`Components` with the listed subset (`:1510-1511`): config silently lost |
| `graph/clustering/storage.go:65` — `keys, err := s.kv.Keys(ctx)` | `*natsclient.KVStore` (`:24`) | `GetAllCommunities` returns a subset as every record. `Maintainer.Tick` (`maintainer.go:220`) seeds `stale` once and sets `listed = true` (`:224-228`), so unlisted stale records are never removed |
| `graph/embedding/storage.go:464` — `keys, err := s.index.Keys(ctx)` | `*natsclient.KVStore` (`:149`) | `ListGeneratedEntityIDs` returns a subset as "every key". No production caller, no test (`git grep ListGeneratedEntityIDs`) |
| `processor/graph-index/query.go:426` — `keys, err = c.predicateBucket.Keys(ctx)` | `*natsclient.KVStore` | `predicateList` replies with short counts and `Total`. The namespaced branch (`:428`) already uses `KeysByFilter` |
| `service/graph_triples_http.go:256` — `keys, err := reader.Keys(ctx)` | `kvcatalog.CatalogReader` (`:252`); interface has `WatchFiltered` (`kvcatalog.go:278`) | HTTP triples reply silently misses entities |
| `service/message_logger_http.go:392` — `keys, err := kv.Keys(ctx, jetstream.IgnoreDeletes())` | `jetstream.KeyValue` (`natsclient/client.go:1618`) | KV query reply silently misses entries |

## 3. What changes on the move to `keysByFilter` with `>`

a. Options: identical (§ 1). The extra `IgnoreDeletes()` at `message_logger_http.go:392` is already in nats.go's set.
b. No match: `keysByFilter` returns nil,nil (`kv.go:625-629` doc, `TestIntegration_FilteredKeyListingsNoMatch`).
   The `ErrNoKeysFound` branches at `kv.go:581`, `graph_triples_http.go:258` and `message_logger_http.go:393`
   become unreachable for these calls.
c. **Sort and repeat removal: not the same.** `Keys` sorts and compacts (`jetstream/kv.go:1421-1422`);
   `keysByFilter` appends in delivery order (`kv.go:691`). #512 measured repeats in listings under concurrent
   overwrites. Sorting already happens at clustering `storage.go:68`, triples `:217`, message_logger `:399`.
   Dedup already happens in graph-index `predicateSummaries`, which counts sets (`query.go:578-596`). No dedup in:
   triples (a repeated key yields its triples twice), message_logger (repeated entry), `GetAllCommunities`
   (repeated record), embedding/key_family (repeated IDs/names). Manager: the `configKeys` count only; `syncFromKV`
   re-applies an entry. Repo precedent: `pkg/lifecycle/manager_query.go:65-70` — `FilteredKeys(ctx, bucket, ">")`,
   then `sort.Strings`, then `slices.Compact` (#534).
d. Time bound: `KVStore.Keys` keeps `applyTimeout` (`kv.go:576`). `graphTripleKeys` sets its own
   `DefaultKVOptions().Timeout` (`:253`). `queryKVBucket` uses the request context only.
e. Cleanup: `keysByFilter` stops and drains up to 5 s past ctx (`kv.go:605`, `:700-720`); nats.go only `defer Stop()`.

## 4. Interfaces and fakes

- No exported interface needs widening. The `KVStore` field is already `jetstream.KeyValue`, and `CatalogReader`
  already carries `WatchFiltered` (`kvcatalog.go:278`).
- After the move, `CatalogReader.Keys` (`kvcatalog.go:279`) has no production reader. Its doc says "Its method set is
  the exact union consumed by current framework catalog readers" (`:272-273`; ADR-110 `:64` cites the read-only
  reader). Dropping the method narrows an exported interface, which breaks consumers that call it, not consumers
  that implement it. Keeping it makes `:272-273` false. Decision (inventory review, 2026-11-07): drop it, with a
  migration note; the in-tree path is `kvcatalog_integration_test.go:96-98` (list through `natsclient.FilteredKeys`).
  Tests that stop compiling (integration-tagged, missed by the first gopls run):
  `graph/kvcatalog/kvcatalog_test.go:28-35` (`TestCatalogReader_ExportedSurfaceIsReadOnly` pins `"Keys"`),
  `graph/kvcatalog/kvcatalog_integration_test.go:93`, and `processor/graph-index/index_integration_test.go:233-235`
  (`harness.keys`, with an `ErrNoKeysFound` branch).
- Test fakes that override `Keys` over a nil embedded interface. Each fails once its path reads `WatchFiltered`:
  - `graph/clustering/maintainer_test.go:25-26` `fakeKV` embeds `jetstream.KeyValue`; `Keys` at `:72-88` with
    `beforeKeys` and `keysCalls` (`:31`, `:37`, `:115`); used by `maintainer_test.go:402-408` and
    `maintainer_prop_test.go:419-422`. It needs `WatchFiltered`, with the hook and counter moved there.
  - `service/graph_triples_http_test.go:38-47` `failingGraphTripleReader` (`TestGraphTripleKeysPreservesCallerPolicy`
    reads the deadline in `Keys`) and `:453-462` `stubEntityReader` ("Keys lists keys in the order given";
    uses at `:528`, `:545`, `:554`). Both need `WatchFiltered`.
  - Fine as is: `processor/graph-ingest/component_test.go:203-232` `mockKVBucket.WatchFiltered` matches `>`
    (`:291-292`), covering `derived_time_test.go:69` and `fact_lane_fence_test.go:247`. Not on these paths:
    `graph/embedding/memkv_test.go:165-171` (both methods `errNotImplemented`) and
    `pkg/lifecycle/manager_test.go:302` (panics).
- Real-NATS tests that call `KVStore.Keys` and should pass unchanged:
  `natsclient/kv_key_contract_integration_test.go:389` and `:664` (the `wrapper.list` wire observation; same watch,
  by reading), `natsclient/nil_context_siblings_test.go:234`, config `manager_identity_integration_test.go:263,598`,
  `manager_arbitration_integration_test.go:253`, `key_family_integration_test.go:103,129`, graph-index
  `query_integration_test.go:535,621,994`, `service/message_logger_kv_integration_test.go:157` (comment names
  `ErrNoKeysFound`).
- No existing test drives a cut-short whole-bucket listing. The filtered cut-short tests are in
  `natsclient/kv_watcher_ownership_test.go:102` and `:265`.

## 5. Adjacent claims

- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`): #541 is this claim, 0 files. #530
  (48 files) touches `processor/graph-index/index.go` and `plan_prop_test.go`, the package of `query.go:426`, but no
  file here. It changes no `transport-client` spec. No file overlap, so no merge order is forced.
- Issues: #512 (listing repeats and omits under overwrites) constrains § 3c: do not reintroduce repeats nats.go
  removed today. The omission half of #512 stays #512's. #496 is closed (PR #532, 5f5036d); its storagereport move
  is the precedent (`natsclient/storagereport/storage_report.go:707-713`; migration note `:59-63`). Other
  `--search Keys` hits (#525 #110 #109 #9) name the word, not this surface.
- ADR-112 (natsclient passes nats.go through) and ADR-110 (`CatalogReader`). This is not a porting change, so no
  pin ADRs and no pin probe.

## 6. Problem shape

"List completely or return an error, before acting." The closest instances, both adopting `FilteredKeys(…, ">")`:
`storage_report.go:710` and `pkg/lifecycle/manager_query.go:65`. This adds no new primitive, so no collision table
or adoption sweep is needed.

## 7. Spec

Widen `openspec/specs/transport-client/spec.md:743-747`, which today reads: "`KVStore.KeysByFilter` and
`natsclient.FilteredKeys` SHALL list keys through one implementation over the reader's `WatchFiltered`. They SHALL
return every key in the watcher's initial snapshot, and nil with no error when it holds none."
Correction (inventory review): the sibling requirements' scenarios say "either call" (`:752`, `:761`, `:768`,
`:777`) and "The two filtered listings differ in their time bound" (`:780-790`) names two calls, so they must
change too, or `Keys` gets its own requirement. Draft
MODIFIED text (≈390 chars; the header must match exactly, or use RENAMED for "Both"): "`KVStore.KeysByFilter`,
`natsclient.FilteredKeys` and `KVStore.Keys`, which lists with the filter `>`, SHALL list keys through one
implementation over the reader's `WatchFiltered`. They SHALL return every key in the watcher's initial snapshot,
and nil with no error when it holds none. Changed: `KVStore.Keys` read through nats.go's `Keys`, which returned the
keys gathered so far when its watch closed early (#533)." If `Keys` keeps sort and compact (§ 3c), the text says so.
`skip_specs` does not fit: `KVStore.Keys` changes observably (an error instead of a partial list).

## 8. Adopter seam (`KVStore.Keys`, `CatalogReader`, `FilteredKeys`: framework API)

1. Must know today: that `Keys` can return a partial list with a nil error. Nothing tells them. After: nothing new.
2. Doing nothing today: silent truncation (§ 2). After: a typed error on a cut-short listing. A consumer test fake
   that overrides only `Keys` over a nil embedded `jetstream.KeyValue` panics; that is a migration note, as at
   `storage_report.go:59-63`. A consumer calling `CatalogReader.Keys` fails to compile and lists through
   `natsclient.FilteredKeys` instead.
3. Where they find out today: nowhere. After: a runtime error from the listing call.
4. Should know: nothing. Consumer call sites of `KVStore.Keys` and `CatalogReader.Keys`: not measured.

## 9. Stale text the change touches (inventory review)

- `service/message_logger_http.go:398` ("nats.go promises the keys deduplicated").
- `service/message_logger_kv_integration_test.go:157` (comment names `ErrNoKeysFound`).
- `graph/clustering/storage.go:69`: its sort is redundant once `KVStore.Keys` sorts.
- `processor/graph-ingest/component_test.go:244` `mockKVBucket.Keys` becomes dead code.
