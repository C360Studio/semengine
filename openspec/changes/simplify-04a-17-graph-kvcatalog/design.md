# Design: simplify-04a-17-graph-kvcatalog

This design depends on `INVENTORY PASS` for `inventory.md`. Premises cite its sections; line numbers are at the base
commit. The package changes spelling, not behaviour, except D4's nil-context refusal and the error text in D3 and D4.

## D1. One table, built once (#133 item 1)

- The 18 rows become an unexported package-level `catalog`, built once from today's `KVCatalog` body. `SpecFor` and
  `EnsureCatalogRetentionClean` range it. A `BucketSpec` holds no slice, map or pointer (inventory § 1 item 1), so the
  value `SpecFor` returns cannot change the table.
- `KVCatalog` has no production reader outside the package, here, at the pin or in a consumer (inventory § 4).
  Owner question 1. Recommended: remove it. If kept, it returns `slices.Clone(catalog)`, with
  `TestKVCatalogReturnsACopy` (a caller's edit of the result does not change `SpecFor`).
- `natsclient/kvspec.go:120` names `kvcatalog.KVCatalog`; it is reworded to name the catalog.
- Options: (a) chosen; (b) keep the per-call build (no reader needs it); (c) do nothing.
- Tests: the existing table tests (`TestKVCatalog_EveryRowValidates`, `_DeclaredPolicies`) read `catalog`. There is no
  observable change to test beyond them.

## D2. The name family is one comparison (#133 item 3)

`nameFamilies` goes. `SpecFor` tests `spec.Name == graph.BucketSemEngineConfig` for the one family row. Members,
the bare prefix and near misses resolve as today: `TestConfigBucketFamilyResolvesEveryMember` keeps its cases; only its
two `IsFrameworkOwnedBucket` assertions change, with D3.

## D3. One check for a generic write (#133 item 2)

- `CheckGenericWrite(bucket string) error` replaces `IsFrameworkOwnedBucket` and `FrameworkOwnedWriteRefusal`. It
  returns nil when a generic KV writer (the rule engine's `update_kv`, ADR-074 `:64-65`) may write the bucket: the
  name resolves to no catalog row, or to a row whose `Write` is not `WriteOwnerOnly`. Otherwise it returns an error
  whose text is `framework-owned bucket "<name>" (owned by <Owner>)`, followed by `; use the graph mutation APIs` for
  an authoritative or derived row. The operational remedy, "its writes belong to" and the owner again, goes
  (inventory § 1 item 2).
- The error is unclassified. Each pin caller adds its own context ("update_kv cannot write", "cannot acquire", a
  rule-validation message), and the rule port (change 6) classifies it there. The pin's not-in-catalog sentence
  (`kvcatalog.go:296-298`) had no caller and goes.
- Who adopts: the three pin sites (inventory § 1 item 2), at the rule port. The compiler finds them.
- Options: (a) chosen. (b) Keep `IsFrameworkOwnedBucket` and make the refusal return `error`: two calls for one
  decision remain. (c) Do nothing: the rule port carries the pair.
- Tests:
  - `TestCheckGenericWrite`, one table: every catalog row is refused, and the error names the bucket and the owner,
    with "graph mutation APIs" exactly for authoritative and derived rows; a config member is refused; `AGENT_LOOPS`,
    `EMBEDDINGS_CACHE`, `CONTEXT_INDEX`, the bare config prefix and `semengine_config_` give nil.
  - The six tests that assert `IsFrameworkOwnedBucket` call `CheckGenericWrite`: five unit tests in three files, and
    `TestIntegration_EnsureCatalogRetentionClean_IgnoresOrphanedEmbeddingsCache`.
  - Sensitivity: `task mutate:check` with the catalog lookup replaced by `return nil` must be detected by
    `TestCheckGenericWrite`. Removing the `Write` filter is an equivalent mutant today, since every row is owner-only
    (inventory § 2); the PR records it as such.
  - Generated checks are not used (`docs/testing.md`): the inputs are the 18 rows and the family grammar, which the
    table and `TestConfigBucketFamilyResolvesEveryMember` enumerate.

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
  `pkg/lifecycle/manager_query.go:260`, whose inline copy adopts it (inventory § 2). The storage report's three
  `!= jetstream.KeyValuePut` tests stay: they ask "is this a value", the complement, and read clearly as written.
- With 8e's D3 merged, `graph/readiness` imports `kvcatalog` in no production file, and its ledger row's dependency
  list drops it.
- The pin's seven other sites and semconnect's tool switch at their port or cutover; the compiler finds them.
- Owner question 2. Options: (a) chosen; (b) inline the two-value comparison at every caller (three here, seven at
  the pin's ports, one in semconnect), the shape that produced the lifecycle copy; (c) keep it in `kvcatalog`, the
  catalog package, which reads no catalog row for it.

## D6. Text follows the code (#133 item 5)

- The package comment names what the package holds after D1–D5 and says the rule engine's `update_kv` guard calls
  `CheckGenericWrite` when `rule` is ported (change 6).
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

- `SpecFor`: keep. Plumbing: pin `processor/graph-index/component.go:903`.
- `OwnerOf`: keep. Plumbing: pin `service/storage_observability.go:223`, `graph-index/component.go:95`, `:886`.
- `EnsureCatalogBucket`: keep. Plumbing: `processor/graph-ingest/component.go:1172`, `:1199`;
  `graph/readiness/publisher.go:48` (graph-ingest's `readiness.go` after 8e). semconnect calls it.
- `CatalogReader`, `OpenCatalogReader`: keep. Plumbing: `pkg/lifecycle/manager.go:209`. Also held by
  `.agents/rules-index.md:44` and `TestCatalogReader_ExportedSurfaceIsReadOnly`; semsource calls them.
- `EnsureCatalogRetentionClean`: keep, D4. Plumbing: pin `service/graph_runtime.go:25`.
- `IsFrameworkOwnedBucket`, `FrameworkOwnedWriteRefusal`: replaced by `CheckGenericWrite`, D3. Plumbing: pin
  `rule/actions.go:2264`, `kv_writer.go:87`, `config_validation.go:382`.
- `CheckGenericWrite`: new, D3. Plumbing: the same three pin sites.
- `KVCatalog`: remove (recommended) or keep with a copy, D1. No reader; owner question 1.
- `IsKVTombstone`: moves to `natsclient`, D5. Plumbing: the three callers in D5; semconnect calls it.

## Open issues and merge order

- Retired: #133 (tasks 2.1–3.2).
- Stay: #135 (8e, #289), #195 (nothing left in this package) (inventory § 3).
- Merge order: #289 (8e) merges first, per the owner's 2026-10-17 order (#223 comment 6232804491). Batch 1 touches no
  `graph/readiness` file and starts now; task 3.1 edits `watcher.go` and holds until #289 merges. #290 (6c) shares
  `owned_bucket_retention.go` (its `:62-63`, this change's `:39-50` and `:71`) and the ledger (another row): either
  order; whichever lands second merges `origin/main`. PR 10 (graph-ingest, unclaimed) inherits D5's one-line edit.

## Owner questions

1. `KVCatalog` lists every catalog row, and nothing outside the package reads it: not here, not in the SemStreams code
   we port from (only two of its contract tests), and not in any consumer. **Recommended: remove it.** The table stays
   in the package, read through `SpecFor`. Keeping it costs a copy on each call and an export nobody calls.
2. Where should the "is this KV entry a delete or purge" check live? **Recommended: `natsclient`, beside the KV
   client.** Your rule for `natsclient` is "pass nats.go through, add no mechanism"; this is a two-value comparison,
   not mechanism, and keeping one copy stops a watcher from checking delete and forgetting purge. The alternative is
   writing the comparison out at each of the eleven places that need it.

FYI, no ruling needed:

- (a) The rule engine's port (change 6) calls `CheckGenericWrite` instead of the pair. ADR-110 Decisions 1 and 7 cite
  `IsFrameworkOwnedBucket` at fixed commits and are left as written.
- (b) Error text changes: `graph.` becomes `kvcatalog.` in the prefix, and the operational refusal names its owner
  once.
- (c) semconnect's readiness conformance tool switches to `natsclient.IsKVTombstone` when it adopts SemEngine.
