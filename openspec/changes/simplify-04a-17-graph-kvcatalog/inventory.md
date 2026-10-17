# Inventory: simplify-04a-17-graph-kvcatalog

base: `bdc4c1ef7aba59d6762a3645ecbbf3ddfbe4a161` (origin/main `2d9ce7cacf08c4e7c7e2fdb90910478ba4264960` plus the empty
claim commit; draft PR #292)

Question: where `graph/kvcatalog` rebuilds, repeats or misplaces what it holds, and which of its exports each reader
needs. Scope: #133 (body items 1–5 and comment item a), epic #223, PR 8d (owner 2026-10-16, #223 comment 6216623583:
"**8d — `graph/kvcatalog`**: #133."). Not in scope: `graph/readiness` (8e, #289), the `natsclient` circuit breaker
(6c, #290), graph-ingest (PR 10).

This change brings in no package from the pin. `graph/kvcatalog` was split from the pin's `graph` by change 2 (design
D16) and is covered by the `graph` ledger row (`docs/admission-ledger.yaml:1953-1954`), so the pin probe runs are not
owed. #133's line numbers are at PR #93 head `7bf1eb8`; every pin below is at the base commit. The package holds 400
non-test lines (`kvcatalog.go` 307, `owned_bucket_retention.go` 83, `tombstone.go` 10).

Repositories read (`docs/inventory-scope.md`): this tree; the SemStreams pin `8b99efe9` as a tarball fetched with
`gh api repos/C360Studio/semstreams/tarball/8b99efe9c66a4faa4fa509f9f62cc6bad8392128` (local only,
`/tmp/arch8d/pin/src`); semsource `4093d3c` (main), semconnect `dff1265`, semteams `9a043281`, semboids `37dbdb0`,
working trees read without git writes.

Inheritance from 8c: 8c's design said 8d inherits `kvcatalog_test.go:165`. 8c itself removed that assertion
(`TestErrorCodeBucketNotReady_MatchesIndexNotReady`; `git show 2d9ce7c -- graph/kvcatalog/`, 7 lines deleted). Today's
`:165` is a comment line of `TestConfigBucketFamilyResolvesEveryMember`. Nothing is left to inherit.

## 1. The claimed gaps (re-measured)

### Item 1: the table is rebuilt on every lookup

- `graph/kvcatalog/kvcatalog.go:42` — `func KVCatalog() []natsclient.BucketSpec {` builds 18 rows (two closures,
  three named rows, one literal) on each call. `:40-41` — "The slice is rebuilt per call so no caller can mutate the
  catalog for another."
- `:197` — `for _, spec := range KVCatalog() {` in `SpecFor`. Every `SpecFor`, `IsFrameworkOwnedBucket`, `OwnerOf`,
  `EnsureCatalogBucket`, `OpenCatalogReader` and `FrameworkOwnedWriteRefusal` call rebuilds it.
- `owned_bucket_retention.go:53` — `for _, spec := range KVCatalog() {`.
- `natsclient.BucketSpec` (`natsclient/kvspec.go:122-134`) holds strings, a `RetentionPolicy{Kind, TTL}`, three
  string enums, a `uint8` and an `int`: no slice, map or pointer, so a returned value cannot alias the table.
- 18 rows, not #133's 17: `TestKVCatalog_EveryRowValidates` asserts 18 (`kvcatalog_test.go:69`).

### Item 2: a check and its refusal are always called as a pair

- `kvcatalog.go:215` — `func IsFrameworkOwnedBucket(bucket string) bool {`; `:294` — `func
  FrameworkOwnedWriteRefusal(bucket string) string {`.
- No caller of either in this tree outside the package's own tests (`git grep -n -w -e IsFrameworkOwnedBucket -e
  FrameworkOwnedWriteRefusal`: only `graph/kvcatalog/*`, `docs/adr/110-write-seam.md:64`, `:141`).
- At the pin, three call sites, each the pair (`gtypes` is the pin's `graph` import):
  - `processor/rule/actions.go:2264` — `if gtypes.IsFrameworkOwnedBucket(bucket) {`, `:2265` — `return
    fmt.Errorf("update_kv cannot write %s", gtypes.FrameworkOwnedWriteRefusal(bucket))`;
  - `processor/rule/kv_writer.go:87-88` (same pair, "update_kv cannot acquire %s");
  - `processor/rule/config_validation.go:382-385` (same pair inside a rule-validation message).
- `FrameworkOwnedWriteRefusal`'s not-in-catalog branch (`kvcatalog.go:296-298`) is reached by no pin caller: each
  calls it only after `IsFrameworkOwnedBucket` returned true.
- `kvcatalog.go:120` — "(at the pin IsFrameworkOwnedBucket has exactly two callers, both rule update_kv". The pin has
  three (above). The pin's own comment says two (pin `graph/kvcatalog.go:117`).
- The refusal sentence names the owner twice for an operational row: `:299` — `remedy := fmt.Sprintf("its writes
  belong to %s", spec.Owner)` and `:306` — `"framework-owned bucket %q (owned by %s); %s"`.

### Item 3: a map holds one family

- `kvcatalog.go:188` — `var nameFamilies = map[string]bool{graph.BucketSemEngineConfig: true}`. Its one reader is
  `:198` — `if nameFamilies[spec.Name] {`.

### Item 4: errors name the package they left

- `kvcatalog.go:241` — `"graph", "EnsureCatalogBucket", "resolve framework bucket")`; `:276` — `"graph",
  "OpenCatalogReader", "resolve framework bucket")`.
- `owned_bucket_retention.go:45`, `:50`, `:71` — component `"graph"` in `EnsureCatalogRetentionClean`.
- `errs.Wrap` puts the component into the message: `pkg/errs/errs.go:366` — `return fmt.Errorf("%s.%s: %s failed:
  %w", component, method, action, err)`. Operators read `graph.EnsureCatalogBucket: …`.
- Test reader: `kvcatalog_test.go:51` — `assert.Equal(t, "graph", classified.Component)`.
- The two resolution refusals are one sentence built twice (`:239-241`, `:274-276`), and a third time as text at
  `:297`.

### Item 5: citations that resolve nowhere here

`git grep -n -E 'ADR-[0-9]+|#[0-9]{3,4}' -- graph/kvcatalog/` and a read of every comment:

- ADRs not in `docs/adr/` (`ls docs/adr`: 066, 083-085, 088, 102, 104, 107-112): ADR-072 (`kvcatalog.go:150`),
  ADR-087 (`:172`), ADR-068 D1 (`owned_bucket_retention.go:15`).
- ADRs that resolve: ADR-083 (`:73`), ADR-104 (`:108`, `:131`), ADR-102 d7 (`:110`; `docs/adr/102…:104`, item 7).
- SemStreams issue numbers: `#1188` (`kvcatalog.go:187`; `:114` already says "SemStreams #1188"), `#1168`
  (`kvcatalog_test.go:90`), `#715` (`owned_bucket_retention_test.go:10`), `#610/#611`
  (`owned_bucket_retention_integration_test.go:66`).
- Labels that resolve nowhere: `framework-bucket-catalog` (`kvcatalog.go:2`, a pin spec name; no such capability under
  `openspec/specs/`); "F2" (`:191`, `kvcatalog_test.go:147`), "F2/F3" (`owned_bucket_retention_test.go:9`);
  "architect-census" (`kvcatalog_test.go:72`); `reopen-framework-owned-bucket-guards`
  (`owned_bucket_retention_test.go:26`); `feedback_unconditional_resource_wiring` (`owned_bucket_retention.go:38`, a
  memory-file name); "the spec's … scenario" (`kvcatalog_attribution_test.go:155`; no spec here names `OwnerOf`:
  `git grep -n -i -e OwnerOf -e attribution -- openspec/specs` is empty).
- The package comment says `IsFrameworkOwnedBucket` is "the rule update_kv write guard" (`kvcatalog.go:16`). `rule` is
  not ported (change 6); ADR-110 Decision 1 already says so (`docs/adr/110-write-seam.md:64-67`).

### Comment item a: `IsKVTombstone` is in the wrong package

- `graph/kvcatalog/tombstone.go:8` — `func IsKVTombstone(operation jetstream.KeyValueOp) bool {`. It reads a
  JetStream operation and no catalog row.
- Production callers here: `graph/readiness/watcher.go:377`, `processor/graph-ingest/component.go:1287`.
- `graph/readiness` production imports of `kvcatalog`: `watcher.go:39` (for `IsKVTombstone`) and `publisher.go:11`
  (for `EnsureCatalogBucket` at `:48`, which 8e's D3 removes: #135 owner ruling 1(b), 2026-10-17).
- At the pin, nine production sites: `processor/graph-index/component.go:1034`, `graph-index-temporal/component.go:769`,
  `:797`, `graph-index-spatial/component.go:751`, `:779`, `graph-embedding/component.go:1483`, `:1519`,
  `graph-ingest/component.go:1330`, `graph/readiness/watcher.go:389`.
- Consumer: semconnect `conformance/cmd/index-readiness/main.go:124` — `if graph.IsKVTombstone(entry.Operation()) {`.

## 2. Every spelling of the facts being modeled

- **"A KV entry removes its key":** `tombstone.go:9`; `pkg/lifecycle/manager_query.go:260` — `if entry.Operation() ==
  jetstream.KeyValueDelete || entry.Operation() == jetstream.KeyValuePurge {` (inline copy);
  `natsclient/storagereport/storage_report.go:631`, `storage_report_consumer.go:273`, `:315` — `!=
  jetstream.KeyValuePut` (the complement; nats.go has three operations, `Put`, `Delete`, `Purge`). `git grep -n -e
  KeyValueDelete -e KeyValuePurge -e 'Operation()' -- '*.go' ':!*_test.go'`.
- **"A bucket is in the catalog, and who owns it":** `SpecFor` (`kvcatalog.go:196`), `OwnerOf` (`:223`, `SpecFor`
  plus `.Owner`), `IsFrameworkOwnedBucket` (`:215`, `SpecFor` plus `Write`), the not-in-catalog sentence three times
  (§ 1 item 4). No other file names a catalog row's owner: `git grep -n -e '"graph-ingest"' -e '"graph-index"' --
  '*.go' ':!*_test.go'` gives `graph/query_verbs.go:18` (`ResponderGraphIngest`, a responder name, not ownership) and
  the catalog.
- **"The config bucket is a name family":** `graph/constants.go:50-64` (the prefix and its member grammar, "derived
  only by config.BucketName"); `kvcatalog.go:188` (the map); `:199` (the member test).
- **"Every catalog row is owner-only":** each row sets `Write: natsclient.WriteOwnerOnly` (`:51`, `:140`);
  `TestKVCatalog_DeclaredPolicies` asserts it for every row (`kvcatalog_test.go:106-108`). `IsFrameworkOwnedBucket`
  still filters on `Write` (`:217`), as `natsclient/kvspec.go:46` says the guard set is derived from that field.

## 3. Adjacent claims

- Open PRs (`gh pr list --state open --json number,title,isDraft,changedFiles`; none over 100 files; file lists from
  `gh api --paginate repos/C360Studio/semengine/pulls/<n>/files`):
  - #292: this claim, 0 files.
  - #289 (draft, 8e `graph/readiness`, 10 files): `graph/readiness/publisher_test.go`, `watcher.go`,
    `watcher_test.go`, and `openspec/changes/simplify-04a-15-graph-readiness/*`. Its diff keeps
    `kvcatalog.IsKVTombstone` in `watcher.go` (old `:377`, hunk `@@ -372,8 +315,6 @@`). Its design plans more:
    `publisher.go` (D3, `EnsureBucket` goes), graph-ingest's `readiness.go` (calls `kvcatalog.EnsureCatalogBucket`),
    and the `graph/readiness` ledger row (proposal `:37`). It lists #133 as staying for 8d ("`IsKVTombstone`,
    `watcher.go:377`", design `:122`) and states "8d and PR 10 come after this change" (design `:125-129`), following
    the owner's 2026-10-17 decision to take 8e ahead of 8d (#223 comment 6232804491).
  - #290 (draft, 6c `natsclient` circuit breaker, 6 files, all under
    `openspec/changes/simplify-04a-16-natsclient-circuit/`). Its design edits
    `graph/kvcatalog/owned_bucket_retention.go:62-63` ("exempted from the shared circuit breaker" goes, design
    `:69`), `natsclient/client.go`, `stream.go`, `request.go`, `errors.go`, `consumer_policy.go`, `owned_delivery.go`,
    `README.md`, `doc.go`, and the `natsclient` ledger row (proposal `:33-41`). It does not list `natsclient/kv.go`.
  - #242–#248: Dependabot bumps (`go.mod`, `go.sum`, `package*.json`, `ci.yml`). No overlap.
- Specs: no capability spec names a `kvcatalog` export (`git grep -n -i -e kvcatalog -e catalog -e tombstone --
  openspec/specs`: one hit, `graph-entity-writes/spec.md:233`, "the catalog descriptor of `ENTITY_STATES`" declares
  `History` 1, held by `TestEntityStatesKeepsOneRevision`). `transport-client` "An exported call refuses a nil context
  before it acts" (`spec.md:193`) binds `natsclient` only.
- Ledger: `graph` row `docs/admission-ledger.yaml:1953-1954` (destination names `kvcatalog.go`,
  `owned_bucket_retention.go` and `IsKVTombstone`), `:1981` (dependencies), `:1993` (proving tests
  `graph/kvcatalog/*_test.go`). `graph/readiness` row `:2015` lists `graph/kvcatalog` as a dependency. `natsclient`
  row `:828`: its `contract` records each changed surface ("Changed surface, simplify-04a-13 (#222): …"), so a name
  added to `natsclient` belongs there; #290 (6c) also edits this row (§ 3, #290).
- `natsclient/README.md` lists the package's KV helpers (`### KVStore Abstraction` `:48`, API Reference
  `### Functions` `:197`).
- Coverage floor: `graph/kvcatalog merged` (`scripts/cover-check.sh:33`); `natsclient merged` (`:29`).
- Docs naming the surface: `graph/README.md:28-29`, `:129` ("the bucket catalog, its acquisition, the owned-bucket
  retention check and `IsKVTombstone`"); `docs/repository-map.md:64`; `.agents/rules-index.md:44`
  (`kvcatalog.OpenCatalogReader`, `TestCatalogReader_ExportedSurfaceIsReadOnly`); skills `entity-or-bucket`
  (`:28`, `:46`, `:71`, `:77`), `kv-or-stream` (`:56`), `query-pattern` (`:33`) name the file, not a symbol;
  `natsclient/kvspec.go:120` ("The catalog (kvcatalog.KVCatalog) is a slice"); `natsclient/storagereport/
  storage_inventory.go:109`, `:207` (`kvcatalog.OwnerOf`); `graph/readiness/publisher.go:19` cites
  "graph/kvcatalog.go", a path that does not exist (8e's file).
- ADR-110 (local) Decision 1 names `IsFrameworkOwnedBucket` "read by nothing but tests" and the rule refusal at three
  pin sites (`docs/adr/110-write-seam.md:64-67`); Decision 7 defines a framework-owned bucket as "a catalog row with
  `WriteOwnerOnly` (`IsFrameworkOwnedBucket`)" (`:141`). Lines there are cited at fixed commits (`:10-13`).

### Open issues naming the package

`gh issue list --state open --search "<q>" --limit 200` for `kvcatalog`, `IsKVTombstone`, `OpenCatalogReader`,
`CatalogReader`, `EnsureCatalogBucket`, `SpecFor`, `KVCatalog`, `IsFrameworkOwnedBucket`, `FrameworkOwnedWriteRefusal`,
`OwnerOf`, `EnsureCatalogRetentionClean`, `owned_bucket_retention`, `tombstone`, `catalog`:

| Issue | Names it in | Disposition |
| --- | --- | --- |
| #133 | title, body, comment | retired |
| #135 | body item e (`kvcatalog.EnsureCatalogBucket`), ruling 1(b) | stays: 8e (#289) |
| #195 | body (`graph/kvcatalog` `fixture_client_integration_test.go:29`, "already on it") | stays: nothing left here |
| #223 | epic lines | `Addresses #223` |
| #17, #26, #27, #105, #106 | name the word, not the package | — |

## 4. Readers of the surface

`git grep -n -w "kvcatalog\.<Name>" -- '*.go'` per export (known positive: `EnsureCatalogBucket` has 2 production
files). Pin: `grep -rn -w --include='*.go' "graph\.<Name>"` and `gtypes\.<Name>` over the tarball, test files apart.
Consumers: `grep -rn --include='*.go'` for `graph.<Name>` and `kvcatalog.` in the four trees.

- `KVCatalog`: no production reader outside the package (`kvcatalog.go:197`, `owned_bucket_retention.go:53`). Pin:
  none; tests only (`test/contract/kvcatalog_literal_contract_test.go`,
  `shared_config_bucket_acquisition_contract_test.go`). Consumers: none.
- `SpecFor`: none outside (test: `graph/readiness/publisher_test.go:42`). Pin:
  `processor/graph-index/component.go:903`, `processor/rule/kv_writer.go:86` (`if _, catalogued :=
  gtypes.SpecFor(bucketName); catalogued {`). Consumers: a semsource test.
- `OwnerOf`: none (tests; a doc mention in `storagereport`). Pin: `service/storage_observability.go:223`,
  `graph-index/component.go:95`, `:886`. Consumers: none.
- `IsFrameworkOwnedBucket`, `FrameworkOwnedWriteRefusal`: none. Pin: `rule/actions.go:2264`, `kv_writer.go:87`,
  `config_validation.go:382`. Consumers: none.
- `EnsureCatalogBucket`: `processor/graph-ingest/component.go:1172`, `:1199`; `graph/readiness/publisher.go:48`. Pin:
  15 sites (config, graph-clustering, the graph indexes, graph-embedding, service). Consumers: semconnect
  `cmd/cs-graph-backend/main.go:319`; a semsource test.
- `CatalogReader`, `OpenCatalogReader`: `pkg/lifecycle/manager.go:209`. Pin: 19 sites. Consumers: semsource
  `internal/graphstatus/reader.go:51`, `:81`, `:94`.
- `EnsureCatalogRetentionClean`: none. Pin: `service/graph_runtime.go:25`. Consumers: none.
- `IsKVTombstone`: `graph/readiness/watcher.go:377`, `processor/graph-ingest/component.go:1287`. Pin: 9 sites (§ 1).
  Consumers: semconnect `conformance/cmd/index-readiness/main.go:124`.

`service` (change 3), `graph-index` (change 4) and `rule` (change 6) are admitted (`docs/setup-plan.md`; 6a owner
ruling 2, #223 comment 6226158511, applied the "read only by an admitted package not yet ported" clause).

## 5. Problem shape

- **Item 1:** a fixed table read through a function. Closest instance: `pkg/types`' unexported package-level
  `frameworkIdentityFamilies` table (`pkg/types/framework_identity_families.go:25`), read by value through exported
  functions (`:33-45`). 8c's `graph.IndexStates()` (`graph/index_status.go:157`) instead builds its list per call, as
  `KVCatalog` does today.
- **Item 2:** a predicate plus its refusal, merged into one check that returns an error. Closest instance:
  `natsclient.BucketSpec.Validate() error` (`natsclient/kvspec.go:146`), a check whose nil result is the pass.
- **Item 4:** one refusal built once. Closest instance: `natsclient.nilContextErrorOf` (`natsclient/request.go:29`).
- **Item a:** a predicate over a library type, homed beside the code that wraps that library. Closest instance: the
  `natsclient` KV helpers in `natsclient/kv.go` (`KVStore`).

No new durable, communication or coordination primitive, and no boundary moves: the collision table and the intent
check are not triggered.

## 6. Pin decision records

All 85 pin ADRs (in the tarball's `docs/adr`) searched for `kvcatalog`, `KVCatalog`, `SpecFor`,
`IsFrameworkOwnedBucket`, `OpenCatalogReader`, `EnsureCatalogBucket`, `EnsureCatalogRetentionClean`, `IsKVTombstone`,
`FrameworkOwnedWriteRefusal`, `OwnerOf`, `CatalogReader`, `bucket catalog`, `update_kv`, `owner-only`,
`WriteOwnerOnly`, `tombstone`, `KeyValuePurge`; titles read. Hits on `OwnerOf` in ADR-056 and ADR-073 are a lease
registry's method, not this one.

- **ADR-068** (Graph retention, deletion & GC), D1 (`:190-211`): live graph buckets set no TTL and no binding
  `MaxBytes`; "framework-owned graph catalog descriptors use no lifecycle retention". Governs every
  `RetentionNoLifecycle` row and `EnsureCatalogRetentionClean`. Not in `docs/adr/`; its rule is `AGENTS.md` § 2,
  "Non-evictable buckets have no TTL or binding `MaxBytes`".
- **ADR-074** (Canonical predicate contract), `:64-65`: "Generic rule `update_kv` actions cannot target framework-owned
  graph buckets." Governs item 2.
- **ADR-104** (local; pin copy `:71`, `:81-84`): the config family is owner-only and strict no-lifecycle; "the
  ownership predicate is consulted by nothing but the `update_kv`" guards. Governs the config row and item 2.
- **ADR-102** (local) item 7, "Identity is never rewritten" (`docs/adr/102…:104`): why the config row never
  reconciles.
- **ADR-083** (local): the `GRAPH_STATUS` row's description. **ADR-072**, **ADR-087** (not local): the
  `GRAPH_INGEST_APPLIED_SEQ` and `COMMUNITY_SUMMARIES` descriptions.
- **ADR-081** (Graph view subscription, not local), `:78`, `:90`: graph-view consumers see tombstones on the same lane
  as values. It decides that tombstones are handled, not where the predicate lives.
- **ADR-110** (SemEngine, local) Decisions 1 and 7 (§ 3).

## 7. Shape sweep (current tree)

1. **Second spelling:** the tombstone test (three spellings, § 2); the not-in-catalog refusal (three, § 1 item 4);
   the pair (item 2).
2. **Copied shell:** none.
3. **Layered interface with one implementation:** `CatalogReader` (`kvcatalog.go:249`) with `catalogReader`
   (`:262`). #133 notes it as a known Go shape with a stated reason, not a finding: the private wrapper stops a
   caller asserting back to the write-capable `jetstream.KeyValue` (`TestIntegration_OpenCatalogReader_
   DelegatesOnlyReadCapabilities`, `kvcatalog_integration_test.go:71`).
4. **Exported surface with no reader:** `KVCatalog` has no production reader outside the package, here, at the pin or
   in a consumer (§ 4). No back door: no exported hook swaps a dependency or reads private state.
5. **Classification by error text:** none (`owned_bucket_retention.go:66` uses `errors.Is`).
6. Not in #133:
   - `EnsureCatalogRetentionClean` does not refuse a nil context at entry. A nil context with a live client reaches
     `client.GetKeyValueBucket(nil, …)`, which returns an invalid-class error (`natsclient/client.go:1875-1876`,
     `request.go:29-30`), and `owned_bucket_retention.go:71` wraps it as transient: the outer class says "retry" for a
     caller bug. MEDIUM at most (#185): its one caller, the pin's `service/graph_runtime.go:25`, passes a real
     context.
   - `graph/readiness/publisher.go:19` cites `graph/kvcatalog.go` (§ 3), 8e's file; filed as #293.

## Adopter seams

Who reaches this surface from outside: semconnect (`EnsureCatalogBucket`, `IsKVTombstone`), semsource
(`OpenCatalogReader`, `CatalogReader`), and SemEngine package authors porting graph-index, graph-embedding, the
temporal and spatial indexes, `service` and `rule`.

1. **What they must know today.**
   - To refuse a generic write, call `IsFrameworkOwnedBucket` first and `FrameworkOwnedWriteRefusal` only when it is
     true; the second alone returns a sentence for any bucket.
   - That the tombstone predicate lives in the catalog package, and that a KV watch has two removal operations.
   - That `EnsureCatalogRetentionClean` given a nil context returns a transient error.
2. **If they do nothing.** A writer that calls only the refusal function and checks for an empty string refuses every
   bucket (it is never empty): fail closed, but wrong. A watcher that tests `op == jetstream.KeyValueDelete` alone
   keeps a purged key as live.
3. **Where they find out.** Doc comments for all three.
4. **What they should have to know.** One call that returns nil or the refusal; the tombstone predicate beside the KV
   client; a nil context refused as invalid.
