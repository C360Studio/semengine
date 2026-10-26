# Inventory: setup-04a-04-graph-roots-graph-index

base: `39a17a959216fe4f82564b89caf6df5ea671c889` (origin/main after PR #411; the claim commit `c4fe354` on
`claude/port-graph-index` adds no file). Pin paths are SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, written
`pin:<path>:<line>`, read from a copy of the pin fetched with `gh api repos/C360Studio/semstreams/tarball/8b99efe9`
into `/tmp/pin-8b99efe9-gi/src` (local only). Consumer cites marked *(local checkout, unpinned)* were read from the
owner's sister checkouts without git; the reviewer re-checks them with `gh api … ?ref=<sha>` (`docs/inventory-scope.md`
rule 4).

Status: not yet reviewed. No `INVENTORY PASS`; the design was drafted in the same pass at the caller's direction, as
on the three earlier change 4 pull requests.

Kind of change: a port. It carries `processor/graph-index` (4,527 non-test pin lines, 16 files) and, from packages
already ported, the pieces the ingest-kernel design deferred to graph-index: `graph.ComputeIndexStatus` with
`IndexStatusInputs` (`pin:graph/index_status.go:167-258`), `readiness.WithRevisionGauges`
(`pin:graph/readiness/gauges.go:57-76`) and `graph.QueryResponse`/`NewQueryResponse` (`pin:graph/query_contracts.go:17-28`;
design D18 of `setup-04a-02-ingest-kernel`). It adds one tree package, the ENTITY_STATES follower (ADR-108 D1, D4),
built from graph-index's watch loop.

Briefing hypotheses that did not hold:

- "#110's legacy trio leaves when graph-index ports": the trio already left `graph.IndexStatusResponse` in change 2
  (`docs/admission-ledger.yaml:2016-2017`, adaptation (5); spec `graph-transport-boundary`, "The readiness envelope
  carries no legacy fields"). What remains of #110 item 1 here is that `ComputeIndexStatus` ports without `LastSynced`
  and `Revision` (`pin:graph/index_status.go:175`, `:254-256`).
- "Any LLM or external-provider code inside graph-index": there is none (section 8).
- "graph-index alone is 4,527 lines, plus the follower": the follower is not extra pin code. It is graph-index's own
  watch loop and readiness glue (`pin:processor/graph-index/component.go:971-1115`, `:1226-1304`,
  `pin:processor/graph-index/watermark.go:1-207`), moved to a graph package. Pin lines carried are about 4,660
  (section 10).

## Pin probe

go1.26.4 darwin/arm64, on the pin copy, unit tests only (no build tag).

- `go test -race -count=1 -p 1 -cpu 1 -cover ./processor/graph-index`: ok, 10.4 s, coverage 80.0%.
- `go test -count=5 -p 1 -cpu 1 -shuffle=on ./processor/graph-index`: ok, 31.3 s. The shuffle seed was not printed
  (no `-v`); a re-run with `-v` records it (task 1.2).
- The lifecycle suite is not run against the pin: it is this tree's harness (`internal/harness/lifecycletest`), and
  graph-index at the pin does not build against it. The pin's own lifecycle tests are
  `pin:processor/graph-index/lifecycle_order_test.go`, `failed_start_subscription_test.go:48`, `:125`, `:201`,
  `lifecycle_integration_test.go` (integration, not run). The suite runs on the port (design D11).
- Integration tests (13 files, `//go:build integration`) were not run at the pin (Docker cost; #108's measurement in
  task 1.3 runs one of them).
- Test-text rules (`grep` over `pin:processor/graph-index/*_test.go`, 13,936 lines, 246 `func Test`):
  `time.Sleep` 24 (component_test.go 3, attack_test.go 1, attack_integration_test.go 7, index_operations_test.go 2,
  integration_test.go 4, predicate_layout_smoke_integration_test.go 1, query_integration_test.go 6); `t.Skip` 11
  (attack_integration_test.go:25, :30, :84, :149, :230; component_test.go:609, :684, :706, :721, :743, :1024);
  `t.Parallel` 13, none in an integration-tagged file (review_remediation_test.go 4, kv_contract_test.go 2,
  owner_reconcile_test.go 1, predicate_contract_test.go 2, predicate_index_test.go 4); build tags other than
  `integration`: `pin:processor/graph-index/race_enabled.go:1` — `//go:build race` and `race_disabled.go:1` —
  `//go:build !race` (production files; their constant is read only by `attack_integration_test.go:29`, to skip).

## 1. What exists (the claimed gap)

### The package is not in the tree

`git ls-files processor/graph-index`: empty. `git grep -n "graph.index.query" -- '*.go'`: empty. The ledger has no
`processor/graph-index` row (`grep -n "source_path: processor/graph-index" docs/admission-ledger.yaml`: empty).

### What graph-index needs, and where it is in the tree

Every qualified identifier the pin's non-test files use was checked in the tree (`/tmp/gi-scripts/sym.sh`, a
`git grep` for each declaration). Present: `graph/kvcatalog/kvcatalog.go:211` `SpecFor`, `:238` `OwnerOf`, `:251`
`EnsureCatalogBucket`, `:296` `OpenCatalogReader` (`CatalogReader` has `Status`, `:281`); `natsclient/kvspec.go:232`
`EnsureFrameworkBucket`; `natsclient/kv.go:110` `BucketLastSeq`, `:739` `IsKVTombstone`, `:758` `IsKVNotFoundError`,
`KVStore` `Get` `:77`, `Put` `:207`, `Delete` `:516`, `Keys` `:572`, `KeysByPrefix` `:594`, `KeysByFilter` `:616`;
`natsclient/kv_key_contract.go:118`, `:157` (validators); `natsclient/request.go:330` `SubscribeForRequests`;
`natsclient/client.go:970` `Subscription.Drain`; `graph/constants.go:6-19`, `:73` (bucket names);
`graph/state_contract.go:13`, `:39-49`, `:70`; `graph/mutation_responses.go:65`, `:86`; `graph/index_status.go:30`,
`:146-149`; `graph/entity_predicate_contract.go:288` `UnmarshalEntityState`; `graph/predicate_codec.go:23`, `:30`;
the query data types `graph/query_index_types.go:6-35`, `graph/query_name_types.go:7-26`,
`graph/query_predicate_types.go:6-41`; `graph/readiness/watcher.go:49` `KeyGraphIndex`, `:71` `DefaultHeartbeat`;
`graph/readiness/publisher.go:41` `NewPublisher`; `graph/readiness/gauges.go:37` `NewGauges`, `:109` `Set`, `:120`
`RecordPublishFailure`; `graph/readiness/readiness_gate.go:117` `EvaluateReadinessGate(r Reading)`;
`internal/resource/watcher.go:67`, `:78`; `internal/revlag/watermark.go:83` `New`, `:101` `Observe`, `:151` `Complete`,
`:217` `Observed`, `:237` `IndexedAt`; `pkg/lifecyclecleanup/lifecyclecleanup.go:36`; `internal/dispatch/keyed_pool.go:123`
`NewKeyedPool`, `:198` `SubmitBlocking`, `:284` `Shutdown`; `component/lifecycle_type.go` (`component.Lifecycle`);
`vocabulary/registry.go:452`, `:477`; `vocabulary/predicate_contract.go:70`; `pkg/types/entity_id.go:149`.

Absent, each with the record that deferred or removed it:

| Pin symbol | Tree | Record |
| --- | --- | --- |
| `graph.ComputeIndexStatus`, `IndexStatusInputs` (`pin:graph/index_status.go:167`, `:221`) | absent | deferred to change 4 with graph-index, ruling P, ingest-kernel D16 (`docs/admission-ledger.yaml:2085-2086`) |
| `readiness.WithRevisionGauges`, `ProducerNames` (`pin:graph/readiness/gauges.go:19`, `:60`) | absent; `NewGauges(subsystem)` | `WithRevisionGauges` deferred (same line); `ProducerNames` removed (`:2079-2080`, adaptation (9)) |
| `graph.QueryResponse`, `NewQueryResponse` (`pin:graph/query_contracts.go:17`, `:23`) | absent | deferred to change 4, ingest-kernel D18 (`docs/admission-ledger.yaml:2041`) |
| `graph.StatusReading`, `graph.EvaluateReadinessGate(StatusReading)` (`pin:graph/readiness_gate.go:100`, `:142`) | `readiness.EvaluateReadinessGate(Reading) DeferReason` | ledger adaptation (6) (`:2070-2071`); the gate's doc names this reader's call: `Reading{Status: s, Fresh: true}` (`graph/readiness/readiness_gate.go:80-81`) |
| `readiness.EnsureBucket` (`pin:graph/readiness/publisher.go:43`) | absent | removed; producers call `kvcatalog.EnsureCatalogBucket` (ledger adaptation (8)) |
| `graph.AllIndexStates` | `graph.IndexStates()` | ledger adaptation (13) |
| `pkg/revlag.Watermark.Indexed` | `IndexedAt` only | PR #407 (`docs/admission-ledger.yaml:3306`) |

### The follower does not exist

`git grep -n -i -E "follower|ENTITY_STATES watch" -- '*.go'`: empty. Watchers of `ENTITY_STATES` in production code
(`git grep -n -E 'WatchAll\(|\.Watch\(ctx|BucketEntityStates' -- '*.go' ':!*_test.go'`):
`pkg/lifecycle/manager_query.go:203` (a per-workflow pattern watch, a workflow reader, ADR-108 "Boundaries") and none
other. The gap is real.

## 2. Every current spelling of the facts modeled

| Fact | Spellings at the pin (carried) | Spellings in the tree |
| --- | --- | --- |
| Component lifecycle (one Start, Stop, rollback, Health, DataFlow) | `pin:processor/graph-index/component.go:219-260` fields, `:506-858` | `component/lifecycle_type.go` (`component.Lifecycle`, ADR-108); graph-ingest composes it |
| Keyed per-entity ordering | `pin:processor/graph-index/keyed_dispatcher.go:12-74` | `internal/dispatch/keyed_pool.go:37` (graph-ingest `keyed_ingest.go:174`) |
| Readiness projection over a watermark | `pin:processor/graph-index/watermark.go:25-118` + `pin:graph/index_status.go:221` | `graph/readiness/index_status.go:98` `ComputeBacklogStatus` (the backlog shape only) |
| Readiness gate | `pin:processor/graph-index/query.go:208-262` calls the pin gate | `graph/readiness/readiness_gate.go:117` (no reader yet; ADR-109 D3 names graph-index as its one reader) |
| Readiness gauges | `pin:processor/graph-index/metrics.go:93-96` (shared set + revision gauges) | `graph/readiness/gauges.go:16` |
| `degraded` word | `graph.IndexStateDegraded` | also the shell's literal `degraded` (processor-shell D4; noted on #110) |
| The owned-bucket set | `pin:…/component.go:71-76` (Validate), `:871-876` (createOutputBuckets "belt"), `:952-965` (assignBucket), `:132-176` (ApplyDefaults), `:178-205` (DefaultConfig) | `graph/kvcatalog/kvcatalog.go:171-176` (owner `graph-index`) |
| Request subjects `graph.index.query.*` | `pin:processor/graph-index/query.go:31-80`, `:94-105` | none; the verb table `graph/query_verbs.go:29` holds graph-ingest's three |
| Reply envelope | `graph.NewQueryResponse` at 11 sites (`query.go:141`…`:679`, `name_index.go:310`, `:348`) | none (D18 shape decided, code deferred) |
| A handler's deadline | `context.WithTimeout(ctx, 5*time.Second)` in each handler (`query.go:114`, `:265`, `:333`, `:371`, `:577`, `:620`, `name_index.go:269`; predicateList has none) | natsclient's per-request handler timeout (`natsclient/request.go:326-327`) |

## 3. Adjacent claims

### Open pull requests

`gh pr list --state open --json number,title,isDraft,headRefName,changedFiles,files` on 2026-10-26: #414 (this claim,
draft, 0 files) and #413 (`claude/flow-paths-composition-363`, draft, 0 files, "flow-path reachability moves out of
service (closes #363)"; its body names `service` and `composition`). This change edits neither package. No overlap;
no merge order between them.

### Open issues naming a carried package

Queries, each `gh issue list --state open --search <q> --limit 200`: `graph-index`, `"graph index"`, `follower`,
`readiness`, `revlag`, `watermark`, `ENTITY_STATES`, `ensureQueryReady`, `keyed_dispatcher`, `index_not_ready`,
`ComputeIndexStatus`. Hits read in full (body and comments, `gh issue view <n> --comments`).

| Issue | Names | Gist |
| --- | --- | --- |
| #107 | `pin:processor/graph-index/keyed_dispatcher.go:12-64` | graph-index and `embedding.Worker` adopt `KeyedPool`; the private dispatcher does not port |
| #108 | `pin:…/component.go:974`, `:301-316`, `query.go:208-258` | measure rebuild-on-restart, record the number in change 4's design, then decide a resume point |
| #103 | `pin:processor/graph-index/query.go:141-679` | one reply envelope with `indexed_revision`; `min_revision` on requests (D18 decided the shape) |
| #110 | `graph/index_status.go:134-138` trio; graph-index port | item 1 at graph-index's port (change 4); items 2, 3 elsewhere |
| #109 | `pin:…/component.go:971-1068` | the follower and the shell; follower half lands here (ADR-108 D2) |
| #106 | `pin:processor/graph-index/query.go:31-106` | `graph.index.query.*` verbs join the responder-owned table; second subscriber refused at Start |
| #68 | `pin:processor/graph-index/metrics.go:36-37`, `:112-119` | drop `sync.Once` and the global-registry fallback at the port |
| #195 | graph-index 26 test clients | `natsfixture.Open` for each package's test client (adoption list) |
| #196 | ADR-108 adoption order includes graph-index | guard adoption, through the shell |
| #408 | `internal/revlag/watermark_test.go:501-519` | revlag's concurrent-reader test can pass without observing progress |
| #300 | graph-ingest only (sections C, D, F, H) | named by the brief; touches no graph-index file |

Names the word, not the package: #405, #141, #111, #16, #161, #216, #280, #9 (epic), #7 (epic).

### Specs and records

- `openspec/specs/graph-readiness/spec.md` (gate; "one in-process reader today"), `graph-transport-boundary/spec.md:13`
  ("Reserved request subjects have one declaration", graph-ingest's three), `:50` ("The responder serves exactly its
  declared verbs", graph-ingest), `:100` ("Authority reads are not enveloped"), `:111` (legacy fields),
  `revision-watermark/spec.md`, `component-lifecycle/spec.md`, `background-work/spec.md`, `metric-registry/spec.md`,
  `lifecycle-suite/spec.md`, `component-registration/spec.md`.
- Ingest-kernel design D18 (`openspec/changes/archive/*-setup-04a-02-ingest-kernel/design.md:1159-1210`): envelope
  `data, indexed_revision, producer, timestamp`, constructor refuses an empty producer; `min_revision` declared with its
  first embedder, the `graph.query.*` request types, which arrive with graph-query; `TestQueryResponseCarriesIndexedRevisionAndProducer`
  named for change 4.
- Developer contract `:155-158` and reviewer contract `:226-228`: "For the indexes graph-index brings in change 4 (none
  is written in this tree yet)… Every index a query can see implements replacement, including `[A] -> [B] -> []`".
- Foundation design `:84-97`: `graph-index.revisionCoalescer` is an unexported lifecycle owner, covered by its package's
  tests; `:353`: the pin's three failed-start tests are change 4's.

### Admission-ledger rows

No `processor/graph-index` row. Rows this change edits: `graph` (`:1997-2054`, `known_risks` "Deferred to change 4:
QueryResponse/NewQueryResponse"), `graph/readiness` (`:2056-2092`, "ComputeIndexStatus and WithRevisionGauges deferred
to change 4"), `pkg/revlag` (`:3286-3324`, "no importer until graph-index"), `component`, `internal/dispatch`
(adopter added). New rows: `processor/graph-index`, the follower package, and each ADR carried (section 6).

## 4. Who the new surface is for

| New surface (the pin lacks it) | For |
| --- | --- |
| The follower package and its exported type and methods | plumbing: production caller `processor/graph-index` (this change); adopters in changes 5 and 7 (ADR-108 Consequences) |
| `graph.QueryResponse` fields `indexed_revision`, `producer` | framework API: D18's contract line; consumer semconnect decodes `graph.QueryResponse[graph.PredicateData]` (`gateway/cs-api/systems.go:993`, *(local checkout, unpinned)*) |
| Eight `graph.QueryVerbs()` entries, responder `graph-index` | plumbing: graph-index subscribes from them; graph-query (PR 9) routes to them; consumer semsource wires `graph.index.query.*` (`cmd/semsource/run.go:864` *(local checkout, unpinned)*) |
| `readiness.NewRevisionGauges` (name proposed) | plumbing: the follower |

## 5. The problem shape

Two shapes. (1) *A per-revision projection pipeline over an authoritative KV bucket*: snapshot-then-live watch, keyed
ordered apply, a low-water-mark watermark, a sticky reset latch, and a published status. Closest instance in the tree:
`graph/readiness/watcher.go:195-300` (`Watcher.Run`: one key, snapshot-then-live, `nil` marker at `:294`, run on the
caller's goroutine, refuses nil ctx) — same watch shape, no apply and no watermark. The design adopts its `Run(ctx)`
shape. At the pin the same pipeline is copied four times (ADR-108 Context: graph-index `:971-1066`, graph-embedding
`:1417-1716`, spatial `:690-856`, temporal `:712-886`). ADR-081 (pin) decides `pkg/graphview` is not this shape: the
per-revision pipelines "Stay raw `WatchAll`" (`pin:docs/adr/081-graph-view-subscription.md:204`).
(2) *Readiness rides the reply*: an in-process gate answering a classified refusal. Closest instance:
graph-ingest's request gate `startCompleted` (`processor/graph-ingest/component.go`, processor-shell D5) and the
classified refusals of graph-ingest's query handlers (`processor/graph-ingest/query.go`).

### Adoption sweep (the follower establishes a primitive)

Adopters, never an obligation of this change; tracking issue: #109 (already open for exactly this).
`pin:processor/graph-embedding/component.go:1417-1716` (change 5), `pin:processor/graph-index-spatial/component.go:690-856`
and `pin:processor/graph-index-temporal/component.go:712-886` (change 7); `pin:processor/rule` decides in change 6
(ADR-108 Consequences). `ComputeIndexStatus`'s other producer: graph-embedding (`pin:processor/graph-embedding/readiness.go:43`).

## 6. The pin's decision records for the carried packages

Fetched from the pin copy (`docs/adr`, 85 files); `grep -l -E 'graph-index|graphindex|revlag|ensureQueryReady|keyed_dispatcher|KeyedPool|revision_coalescer|NAME_INDEX|PREDICATE_INDEX|INCOMING_INDEX|OUTGOING_INDEX|ALIAS_INDEX|ComputeIndexStatus'`
returned 20; read for what they decide about graph-index. Plus ADR-072 and ADR-079 (title match: keyed lanes; poison).

| ADR | Decides for graph-index | In `docs/adr/` |
| --- | --- | --- |
| 066 Honest graph-index readiness | revision-lag watermark, target = LastSeq, stuck detector §4, ready ≠ started | yes |
| 083 Readiness as distributed state | envelope on `GRAPH_STATUS`, status subject removed | yes |
| 084 Readiness licenses health, not absence | bootstrap latch D2; `index_not_ready` means not built or broken D4 | yes |
| 085 Gate on health report freshness | four gate conditions, defer reasons | yes |
| 088 Per-producer readiness | one key per producer | yes |
| 108 Processor shell and follower | shell D1-D3; follower D1, D4; graph-index's second Start refused D3(a) | yes |
| 109 Ready after boot | the reply carries readiness D1; Health from stuck states D2; bucket is information D3; per-request catch-up D4 | yes |
| 113 Start context bounds Start | run context created in Start, ended in Stop | yes |
| 095, 058 | one-shot lifecycle; collectors at construction | yes |
| 065 Predicate-index composite-key sharding | hashed predicate keys | no; superseded for PREDICATE by 078 |
| 068 Graph retention and deletion | no TTL on graph buckets; "ALIAS_INDEX remains the base graph-index exception… production removal is incomplete" (`:34-36`) | no |
| 074 Canonical predicate contract | sticky `reset_required` on unreadable or noncanonical state (`:69-71`) | no (ADR-109 cites it as "not carried") |
| 077 Bounded owner discovery, source-owned INCOMING | the owner matrix (`:32-42`): PREDICATE, NAME, INCOMING layouts and owner filters; worker maximum by config (`:144`) | no |
| 078 Raw canonical predicate membership keys | PREDICATE `predicate3.entity6`, the nine-token filters | no |
| 060 Unified RPC error contract | handlers return one classified error (`:322`) | no (natsclient's ADR-112 carries the client half) |
| 072 Keyed-concurrent entity ingest | keyed lanes by entity | no (ported `internal/dispatch` without it) |
| 079 Per-entity poison | graph-ingest's; graph-index none | no |
| 081 Graph view subscription | per-revision pipelines stay raw `WatchAll` (`:204`) | no (governs `pkg/graphview`, PR 7) |
| 082 Bounded-lag readiness | consumer-class split; `FailedCount → degraded` | no (replaced by ADR-109 D4 for consumers) |
| 048 Bounded dispatcher | graph-index keeps `pkg/worker` (`:217`) | no; stale at the pin (graph-index uses its own dispatcher) |
| 044, 073, 075, 086, 099 | name graph-index in passing; decide nothing it implements | no |

## 7. Shape sweep (pin copy, every carried file)

`R` = a candidate for required work (basis in the design), `O` = optional under Bounded port scope.

| # | Shape | `pin:path:line` — text | R/O |
| --- | --- | --- | --- |
| S1 | 2 copied shell | `component.go:506` — `func (c *Component) Health() component.HealthStatus {` … `:858` (Health, DataFlow, Initialize, Start, Stop, stopOwnedRuntime) | R |
| S2 | 2 copied follower | `component.go:971` — `func (c *Component) watchEntityStates(ctx context.Context, bucket entityStatesReader) {` … `:1115`; `:1232` `statusMetricsLoop`; `watermark.go:25` `computeIndexStatus` | R |
| S3 | 1 second spelling | `keyed_dispatcher.go:12` — `type keyedDispatcher[T any] struct {` | R |
| S4 | 1 second spelling | `component.go:71-76` `requiredBuckets`, `:871-876` `ownedOutputs`, `:952` `assignBucket`, `:132-176` and `:178-205` port lists twice | O |
| S5 | 1 second spelling | `component.go:1417-1420` — `targetID, _ := triple.Object.(string)` into `map[string]interface{}{"id": …}` re-read at `:1696-1697` | O |
| S6 | 1 second spelling | `query.go:114` — `ctx, cancel := context.WithTimeout(ctx, 5*time.Second)` (seven handlers) beside natsclient's handler timeout | O |
| S7 | 1 second spelling | `component.go:1597` — `atomic.AddInt64(&c.messagesProcessed, 1)` and seven more counter sites with three meanings (entity indexed, one KV write, one delete); `errors` at 20 sites | R |
| S8 | 3 one-implementation interface | `component.go:209` — `type entityStatesReader interface {`; `:214` `entityStatesStatusReader` (one production implementer, `kvcatalog.CatalogReader`; test doubles) | O |
| S9 | 4 exported, no reader | `component.go:1751` `UpdateOutgoingIndex`, `:1841` `UpdateIncomingIndex` (with `updateIncomingIndexBatch` `:1869`), `:1985` `UpdatePredicateIndex`, `:2194` `DeleteFromPredicateIndex`, `:2236` `DeleteFromAliasIndex`, `name_index.go:175` `UpdateNameIndex` — production callers: none (`grep -rn '\.Update…\|\.Delete…' --include='*.go' . \| grep -v _test`); each writes an index outside the watch path | R |
| S10 | 4 exported, internal reader only | `component.go:1938` `UpdateAliasIndex` (caller `:1506`), `:2049` `DeleteFromIndexes` (callers `:1342`, `:1737`) | R |
| S11 | 4 unexported, test-only | `component.go:1734` `handleEntityDelete`; `:1361` `processEntityUpdate`, `:1371` `processEntityUpdateResult` ("retains direct-call completion semantics for unit callers"); `:1129-1131` nil-pool branch; `revision_coalescer.go:54` `Close`; `kv_contract_benchmark.go:13`, `:17`; `race_enabled.go:5`, `race_disabled.go:5` `raceEnabled` | R |
| S12 | 4 back door | `component.go:362` — `subscribeForRequests func(` (swaps the NATS subscribe for tests, read at `query.go:25-28`) | R |
| S13 | 4 test-only field read in production | `component.go:293` — `statusInterval time.Duration` ("TEST SEAM only") | R (new code; design D5) |
| S14 | 4 dead fallback | `component.go:269` `nameIndexReady` ("DEPRECATED by ADR-066"), `name_index.go:239` `nameIndexIsReady`, `watermark.go:42-58` and `:62-66` (pre-Start/unit-test fallbacks), `query.go:224-226` (`watermark == nil` → ready) | R |
| S15 | 4 phantom config | `component.go:45` — `BatchSize int` (validated `:124`, defaulted `:136`, read by nothing) | R |
| S16 | 4 test-only atomics | `component.go:305-306` `reindexTotal`, `reindexUnchanged` (no production reader; the metric `reindex_events_total` is the reader) | O |
| S17 | 5 classification by error text | none: a `grep -n -E` for `strings.Contains` or `strings.HasPrefix` on an error, or for `.Error()`, over the non-test files came back empty | — |

Defects and gates found in the same read (not shapes):

| # | `pin:path:line` — text | What |
| --- | --- | --- |
| G1 | `component.go:636` — `ctx, cancel := context.WithCancel(parent)` | run context is a child of Start's ctx: ending it after Start stops the component (ADR-113) |
| G2 | `component.go:621-623` — `if c.runCancel != nil { return nil }` | second Start on a running component returns nil (ADR-108 D3(a)) |
| G3 | `component.go:527` — `Healthy:    c.running && errorCount == 0,` | error count lowers Healthy (ADR-109 D2) |
| G4 | `component.go:390` — `if err := json.Unmarshal(rawConfig, &config); err != nil {` | unknown config keys accepted (`AGENTS.md` § 1, verify+review) |
| G5 | `component.go:974-985`, `:1000-1004` | failed `WatchAll` and a closed `Updates()` return with no mark (ADR-108 D4) |
| G6 | `component.go:971-1066` | no validating phase in the snapshot (ADR-108 D4) |
| G7 | `metrics.go:36-37` `metricsOnce sync.Once`; `:112-119` `prometheus.DefaultRegisterer`; `:46` `Namespace: "semstreams"`; `:103-109` returns ignored | metric-registry spec (verify), #68, `RegisterOrGet` rule |
| G8 | `component.go:1500-1507` (`ALIAS_INDEX has no owner-complete axis… alias retirement remains explicit`); `pin:docs/adr/068…:34-36` | a dropped or changed alias, or a deleted entity's alias, keeps resolving: `[A] -> [B]` fails on the alias read (developer contract `:158`) |
| G9 | `component.go:580-598` | `Initialize` re-validates config (ADR-108 D3(d)) |

## 8. External-provider code at the pin

None. The non-test imports (`grep` of the import blocks of the 16 files) are stdlib, `component`, `graph`,
`graph/readiness`, `internal/lifecyclecleanup`, `metric`, `message`, `natsclient`, `pkg/errs`, `pkg/resource`,
`pkg/revlag`, `pkg/types`, `vocabulary`, `nats.go/jetstream`, `prometheus/client_golang`.
`grep -n -i -E 'net/http|http\.|llm|openai|embed|model/|provider'` over the non-test files: three comment hits
naming graph-embedding (`component.go:933`, `metrics.go:27`, `watermark.go:13`), no code. Nothing leaves under the
tier-0 ruling (#9 comment 6386603659).

## 9. Collision table: the follower (a runtime-coordination primitive)

| Dimension | Evidence |
| --- | --- |
| Semantic class | keep a projection of `ENTITY_STATES` honest: watch, replay, validate, latch reset, track applied revisions, publish status |
| Owners | at the pin: graph-index, graph-embedding, spatial, temporal (ADR-108 Context); in the tree: none. Not owners: `pkg/lifecycle/manager_query.go:203` (workflow reader), `pkg/graphview` (pin, ADR-081: serving views, not per-revision pipelines), `graph/readiness.Watcher` (watches `GRAPH_STATUS`) |
| Catalogs | `graph/kvcatalog/kvcatalog.go:78` (`ENTITY_STATES`, owner graph-ingest), `:89` (`GRAPH_STATUS`, readiness producers), `:171-176` (graph-index's five) |
| Status | `GRAPH_STATUS` key `graph-index` (`graph/readiness/watcher.go:49`); gauges `semengine_graph_index_*`; `Health()` stuck states; the classified `index_not_ready` / `graph_state_reset_required` replies |
| Lifecycle | started by graph-index's `start` under its run context; ended by its cleanup; no restart (ADR-108 D3(a)) |
| Ownership | one follower per component instance; graph-index is a singleton per process at the pin (one `GRAPH_STATUS` key) — two instances would publish one key twice: searched `KeyGraphIndex` (one writer) |
| Readers | graph-index's query gate and `Health`; operators and semsource's MCP gateway read the key (`processor/mcp-gateway/tools.go:141`, `processor/supersession/versiondiff_serve.go:204`, `processor/source-manifest/workbench_capabilities.go:30` *(local checkout, unpinned)*); semconnect's conformance probe (`conformance/cmd/index-readiness/main.go:35`) |
| Writers | the follower only (publisher), graph-index never |
| Recovery | rebuild from scratch on every process start (#108); reset latch holds until restart (ADR-074 `:69-71`, ADR-084 D4); a lost feed: no recovery at the pin (G5) |

## 10. Size against the one-package rule

Pin lines carried (non-test, at the pin): `processor/graph-index` 4,527 (`component.go` 2,265, `query.go` 816,
`name_index.go` 405, `watermark.go` 207, `metrics.go` 182, `owner_reconcile.go` 152, `incoming_index.go` 133,
`revision_coalescer.go` 91, `doc.go` 83, `keyed_dispatcher.go` 74, `predicate_index.go` 45, `predicate_codec.go` 28,
`kv_contract_benchmark.go` 19, `key_validation.go` 17, two race files 10); plus `pin:graph/index_status.go:162-258`
(about 95), `pin:graph/readiness/gauges.go:55-76` (about 20), `pin:graph/query_contracts.go:17-28` (12). Total about
4,660. The bound is about 3,000 (#159). Pin lines that do not enter the tree (S3, S9-S15, the shell's scaffolding
S1): about 900; tree lines after the port, estimated: graph-index about 3,250, the follower about 450, `graph` and
`graph/readiness` about 130.

Lifecycle owners in the carried code (foundation design `:84-97`): `graph-index.Component` (exported) and
`revisionCoalescer` (unexported, 91 lines); the follower is a new one. No split by lifecycle owner brings a pull
request under the bound: without the query handlers (`query.go`, `name_index.go:268-348`) graph-index still carries
about 3,630 pin lines.

## 11. Library behaviour the port relies on (#170, ADR-112)

| Statement | Probe at nats.go v1.54.0 / nats-server v2.15.1 (`go.mod:16-17`) |
| --- | --- |
| `WatchAll` sends one `nil` entry after the initial values | empty bucket only: `natsclient/kv_integration_test.go:239-245`. Non-empty: no probe; the follower's integration test drives it (design D11) |
| `Updates()` closes when the watcher ends for a reason other than `Stop` | not promised; no probe. The design treats a close as a lost feed whatever caused it |
| A KV revision and `LastSeq` share one sequence space | `natsclient/kv.go:100-110` states it; probe: `pin:processor/graph-index/revision_space_integration_test.go:38` returns with the port (ADR-109 D4) |
| Concurrent `Status` and `Get` on one KV handle race under `-race` (`pin:…/component.go:236-239`) | carried comment; not promised (Bounded port scope) |
| `watcher.Stop()` via `defer` races nats.go's handler (`pin:…/component.go:986-988`) | carried comment; not promised |
| A filtered key listing may repeat a key (`pin:…/owner_reconcile.go:19`) | carried comment; not promised; the code deduplicates either way |

## 12. Adopter seam inventory

Surfaces reached from outside: the component (`graphindex.Register`, its config keys), its eight request subjects and
their replies, its `GRAPH_STATUS` key, and the follower package (component authors in later changes).

Consumers at symbol level *(local checkout, unpinned)*: semconnect `cmd/cs-graph-backend/main.go:126`
(`graphindex.Register`), `gateway/cs-api/systems.go:79`, `:993` (`graph.index.query.predicate`, decodes
`graph.QueryResponse[graph.PredicateData]`); semteams `internal/runtimecatalog/components.go:64`; semboids
`componentregistry/register.go:35`; semsource `cmd/semsource/run.go:772-781` (config `coalesce_ms`, `workers`), `:864`
(port `graph.index.query.*`), three readers of key `graph-index`.

1. **What must a config author or caller know?** (a) `workers` (lanes) and `coalesce_ms` mean what they did;
   (b) `batch_size` is gone and a config carrying it fails at construction naming the key; (c) a query during the
   startup replay is refused with the transient code `index_not_ready` and should be retried; (d) a reply now carries
   `indexed_revision` and `producer`.
2. **If they do nothing:** (b) boot error naming `batch_size` (semsource does not set it for graph-index); (c) a caller
   that treats any error as final gives up during startup, as at the pin; (d) additive JSON fields, decoders keep working.
3. **Where they find out:** (b) boot error; (c) typed runtime error (`RequestClassified`), as at the pin; (d) compile
   time for Go decoders that use `graph.QueryResponse`.
4. **What should they have to know:** (c) only. A follower author (changes 5, 7) must know to report every delivered
   revision as complete exactly once it is applied: forgetting holds the watermark, and the envelope reads
   `building`, then `degraded` after the stuck window (observable, not silent).

## Open evidence questions

- E1. #108's number (rebuild time and KV writes per entity at the pin's load shape): not measured; task 1.3.
- E2. Does a broker restart close a KV watcher's `Updates()` at nats.go v1.54.0? Not probed; decides whether a lost
  feed happens on ordinary restarts (owner question 2).
