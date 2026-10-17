# Design: simplify-04a-17-graph-kvcatalog

This design rests on the inventory, which passed review (`INVENTORY PASS`, round 1). Premises cite its sections; line
numbers are at the base commit. The package changes spelling, not behaviour, except D4's nil-context refusal and its
error text.

## D1. One table, built once; `KVCatalog` goes (#133 item 1)

- The 18 rows become an unexported package-level `catalog`, built once from today's `KVCatalog` body. `SpecFor` and
  `EnsureCatalogRetentionClean` range it. A `BucketSpec` holds no slice, map or pointer (inventory § 1 item 1), so the
  value `SpecFor` returns cannot change the table.
- `KVCatalog` is removed. It is carried from the pin, no contract line offers it (the `graph` ledger row's `contract`
  does not name it; no spec or README does), and nothing outside the package reads it: not here, not in an admitted
  pin package (its only pin readers are two `test/contract` tests), not in a consumer (inventory § 4). Under #190
  that is dead surface, which is not protected (reviewer contract § Port-time and pattern review, `:400-402`).
  #133's `slices.Clone` shape was written before #190 and assumed the export stays.
- Owner question 1 asks only whether a product needs the full list. If yes, the answer is its contract line (a
  `graph/README.md` sentence): `KVCatalog` stays and returns `slices.Clone(catalog)`, with
  `TestKVCatalogReturnsACopy` (a caller's edit of the result does not change `SpecFor`).
- `natsclient/kvspec.go:120` names `kvcatalog.KVCatalog`; it is reworded to name the catalog.
- Options: (a) chosen; (b) keep the per-call build (no reader needs it); (c) do nothing.
- Tests: the existing table tests (`TestKVCatalog_EveryRowValidates`, `_DeclaredPolicies`) read `catalog`. There is no
  observable change to test beyond them.

## D2. The name family is one comparison (#133 item 3)

`nameFamilies` goes. `SpecFor` tests `spec.Name == graph.BucketSemEngineConfig` for the one family row. Members, the
bare prefix and near misses resolve as today: `TestConfigBucketFamilyResolvesEveryMember` passes unedited.

## D3. The write-guard pair stays for the rule port (#133 item 2)

- `IsFrameworkOwnedBucket` and `FrameworkOwnedWriteRefusal` stay as they are. Their only readers are the pin's `rule`
  (three sites, inventory § 1 item 2), an admitted package not ported yet, so both are plumbing under 6a owner ruling
  2 (#223 comment 6226158511).
- Options:
  - (a) Merge them into a new `CheckGenericWrite(bucket) error`. It would be a new export with no caller here and no
    contract line; ruling 2 covered names moved from the pin, not new ones, so it needs an owner ruling first.
  - (b) Keep the bool and make the refusal return `error`: still two calls, and still a changed export with no
    caller.
  - (c) **Chosen:** leave the pair, and let the `rule` port (change 6) pick the shape with its three call sites in
    hand. One of them, `kv_writer.go:86-88`, also asks `SpecFor` first, so the right merge is that port's to see.
    This is the simpler choice: no new export, no owner question, no test churn here.
- #133 item 2 goes to a follow-up issue for the `rule` port, filed before this change merges (tasks 4.3). The
  "exactly two callers" comment is corrected in D6.

## D4. Errors name the package; a nil context is invalid (#133 item 4)

- One unexported resolver builds the not-in-catalog refusal once: `errs.WrapInvalid` with component `"kvcatalog"`,
  operation `EnsureCatalogBucket` or `OpenCatalogReader`, and today's inner text. The message prefix becomes
  `kvcatalog.<Operation>:` (inventory § 1 item 4).
- `EnsureCatalogRetentionClean` uses component `"kvcatalog"` in its three wraps, and refuses a nil context first: an
  invalid-class error, before the nil-client check and any NATS call, as `natsclient.EnsureFrameworkBucket` does
  (`natsclient/kvspec.go:237-240`). Today a nil context with a live client comes back transient (inventory § 7,
  item 6; MEDIUM at most).
- Tests:
  - `TestOpenCatalogReader_InvalidNamePreservesClassification` asserts component `"kvcatalog"`, and gains the same
    row for `EnsureCatalogBucket`. It fails by assertion on the base commit.
  - `TestIntegration_EnsureCatalogRetentionClean_RefusesNilContext`: the package's fixture client, a nil context;
    the outer class is invalid and the error names the nil context. Base: transient.

## D5. The tombstone predicate moves to natsclient (#133 comment item a)

- `natsclient.IsKVTombstone(op jetstream.KeyValueOp) bool`, in `natsclient/kv.go`, with today's body and doc. Its test
  moves to `natsclient/kv_tombstone_test.go`. `graph/kvcatalog/tombstone.go` and its test go.
- Callers: `graph/readiness/watcher.go:377`, `processor/graph-ingest/component.go:1287`, and
  `pkg/lifecycle/manager_query.go:260`, whose inline copy adopts it (inventory § 2). The `KeyValueDelete/KeyValuePurge`
  wording in its doc comment (`:167`) becomes "a delete or purge". The storage report's three
  `!= jetstream.KeyValuePut` tests stay: they ask "is this a value", the complement, and read clearly as written.
- Records: the `natsclient` ledger row's `contract` gains "Changed surface, simplify-04a-17 (#133): IsKVTombstone,
  moved from graph/kvcatalog" (inventory § 3); `natsclient/README.md` API Reference `### Functions` (`:197`) gains an
  `IsKVTombstone` entry. With 8e's D3 merged, `graph/readiness` imports `kvcatalog` in no production file, and its
  ledger row's dependency list drops it.
- The pin's seven other sites and semconnect's tool switch at their port or cutover; the compiler finds them.
- Owner question 2. Options: (a) chosen; (b) inline the two-value comparison at every caller (three here, seven at
  the pin's ports, one in semconnect), the shape that produced the lifecycle copy; (c) keep it in `kvcatalog`, the
  catalog package, which reads no catalog row for it.

## D6. Text follows the code (#133 item 5)

- The package comment names what the package holds after D1–D5 and says the rule engine's `update_kv` guard calls
  `IsFrameworkOwnedBucket` once `rule` is ported (change 6).
- ADR-068, ADR-072, ADR-087 and the `#610`, `#611`, `#715`, `#1168`, `#1188` citations are prefixed "SemStreams".
  "F2", "F2/F3", "architect-census", `reopen-framework-owned-bucket-guards`, `framework-bucket-catalog`,
  `feedback_unconditional_resource_wiring` and "the spec's … scenario" are replaced by what they stand for or removed.
  The "exactly two callers" sentence says three pin sites in `rule`.
- `owned_bucket_retention.go:62-63` is left to 6c (#290).
- Also updated: `graph/README.md:129`; the `graph` ledger row (destination: `IsKVTombstone` moved to `natsclient`;
  contract: one adaptation line for `graph/kvcatalog`).

## Exports (#190)

Kinds as in 8c: **API** has a contract line plus a driving test; **Plumbing** has a production caller here, or at the
pin in an admitted package not ported yet (6a owner ruling 2).

- `SpecFor`: keep. Plumbing: pin `processor/graph-index/component.go:903`, `processor/rule/kv_writer.go:86`.
- `OwnerOf`: keep. Plumbing: pin `service/storage_observability.go:223`, `graph-index/component.go:95`, `:886`.
- `EnsureCatalogBucket`: keep. Plumbing: `processor/graph-ingest/component.go:1172`, `:1199`;
  `graph/readiness/publisher.go:48` (graph-ingest's `readiness.go` after 8e). semconnect calls it.
- `CatalogReader`, `OpenCatalogReader`: keep. Plumbing: `pkg/lifecycle/manager.go:209`. Also held by
  `.agents/rules-index.md:44` and `TestCatalogReader_ExportedSurfaceIsReadOnly`; semsource calls them.
- `EnsureCatalogRetentionClean`: keep, D4. Plumbing: pin `service/graph_runtime.go:25`.
- `IsFrameworkOwnedBucket`, `FrameworkOwnedWriteRefusal`: keep, D3. Plumbing: pin `rule/actions.go:2264`,
  `kv_writer.go:87`, `config_validation.go:382`.
- `KVCatalog`: remove, D1. Carried, no contract line, no reader in the reader set; owner question 1 can give it a
  contract line.
- `IsKVTombstone`: moves to `natsclient`, D5. Plumbing: the three callers in D5; semconnect calls it.

## Open issues and merge order

- Retired: #133 (tasks 2.1–3.2; item 2 routed to its follow-up issue, task 4.3).
- Stay: #135 (8e, #289), #195 (nothing left in this package) (inventory § 3).
- Merge order:
  - #289 (8e) merges first, per the owner's 2026-10-17 order (#223 comment 6232804491). Batch 1 touches no
    `graph/readiness` file and starts now; task 3.1 edits `watcher.go` and holds until #289 merges.
  - #290 (6c) shares three things with this change: `owned_bucket_retention.go` (its `:62-63`, this change's `:39-50`
    and `:71`); `natsclient/README.md` (its circuit sections, this change's one `### Functions` entry); and the
    `natsclient` ledger row's `contract`, where both add a "Changed surface" sentence. Either order works.
    Whichever lands second merges `origin/main` and keeps both sentences in that row.
  - PR 10 (graph-ingest, unclaimed) inherits D5's one-line edit.

## Owner questions

1. The function that lists every framework bucket (`KVCatalog`) has no caller anywhere: not in this repository, not
   in the SemStreams code we port from (only two of its own tests), not in any consumer. Your 2026-10-12 rule says an
   export like that is removed unless it is offered to products. **Does any product need the full bucket list?** If
   not, it goes. If yes, it stays and the README says so.
2. Where should the "is this KV entry a delete or a purge" check live? **Recommended: `natsclient`, beside the KV
   client.** You said `natsclient` exists to make the framework's life easier with small, plain code (#170, comment
   6109833136), and this is one two-value comparison that eleven places need; one copy stops a watcher checking delete
   and forgetting purge. The cost: semconnect's readiness conformance tool calls it from its old home today and must
   change one import when it adopts SemEngine (the compiler points to it). The alternative is writing the comparison
   out at each place.

FYI, no ruling needed:

- (a) Error text changes: `graph.` becomes `kvcatalog.` in the prefix of the catalog's refusals.
- (b) The write-guard pair is untouched; the `rule` port decides whether to merge it (D3).
