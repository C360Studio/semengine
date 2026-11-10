# Inventory: `setup-04a-07-substrate-seam-graph-clustering`

Package: `processor/graph-clustering`; with it, the question of `graph/structural` and the rest of `graph/inference`.

base: 999208811956aaed299e8a39d0a8e3788cbb1b04 (claim commit on origin/main 057bc8bfe2f737000a354fd1a38aa60d65cbf9f1)
pin: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128 (tarball via `gh api repos/C360Studio/semstreams/tarball/8b99efe9`,
extracted to `/tmp/gcpin/src`, local only)

Question: what must this change carry, change and drop to port `processor/graph-clustering` at tier 0, with no
provider, so that `COMMUNITY_INDEX` has a writer and graph-query's `localSearch` and text tier answer? Repositories
read (`docs/inventory-scope.md`): this tree, the pin snapshot, and at symbol level semsource (`a084ab6`), semboids
(`37dbdb0`), semconnect (`dff1265`), semteams (`27a0efe8`), semvocab and semsparql (local checkouts, grep only, HEAD
not verified against any pin). Labels: **measured** (command run here), **read** (code read).

## 0. Pin probes

**Measured** on `/tmp/gcpin/src`, go1.26.10, outputs `notes/pin-race.txt` and `notes/pin-repeat.txt`:
`go test -count=1 -race` and `go test -count=5 -cpu=1 -shuffle=on` over `./processor/graph-clustering/
./graph/structural/ ./graph/inference/`: all three `ok`, `rc=0` both, zero `--- FAIL`. Unit tests only: the six
`//go:build integration` files of graph-clustering did not run. The lifecycle suite was not run, as for every change-4
and change-5 port (`archive/2026-11-08-setup-04a-05-first-consumer-graph-embedding/inventory.md:16-22`).

Test-text rules, **measured** by grep on the pin copy:

| Package | `time.Sleep` | `Skip` | other build tags | `t.Parallel` |
| --- | --- | --- | --- | --- |
| processor/graph-clustering | 8 (`component_test.go` 2, `integration_test.go` 5, `entity_watch_scope_integration_test.go` 1) | 6, `component_test.go:671,747,769,784,806,1028` "requires real NATS connection" | none (6 `integration` files) | 8 over 4 files, none integration-tagged |
| graph/structural | 0 | 0 | none | 0 |
| graph/inference | 1 (`hierarchy_attack_test.go`, already ported slice) | 0 | none | 0 |

No pin test is carried as is (§ 8): the pin's tests drive label propagation, the readiness gate and the edge
providers, none of which port.

## 1. Sizes (**measured**, `wc -l`, pin)

| Package / lane | Prod | Test | Lifecycle owner |
| --- | --- | --- | --- |
| processor/graph-clustering, whole | 4,117 | 6,716 | `graphclustering.Component` |
| of which the anomaly lane (`anomaly.go` 365, `structural.go` 52, `similarity.go` 369; tests `anomaly_test.go` 233, `structural_test.go` 35, `review_client_test.go` 95) | 786 | 363 | same |
| graph/structural (`kcore.go`, `pivot.go`, `types.go`, `doc.go`) | 883 | 736 | none |
| graph/inference outside the ported hierarchy slice | 4,557 | 3,165 | `ReviewWorker` (tier 2) |
| of which `review_worker.go` 654 and `http_handlers.go` 335, tests 534 + 534 | 989 | 1,068 | `ReviewWorker` |
| graph/llm (not in the tree) | 1,065 | in total | none |

Pin imports of graph-clustering (**measured**, `grep` of import lines): `component graph graph/clustering
graph/inference graph/llm graph/readiness graph/structural internal/graphmutation internal/lifecyclecleanup
internal/semantictest message metric model natsclient pkg/errs pkg/resource vocabulary`. Not in this tree:
`graph/llm`, `graph/structural`; `graph/inference` is the hierarchy slice only (`docs/admission-ledger.yaml:2292-2325`).

## 2. Category 1, claimed gaps

- "Nothing writes `COMMUNITY_INDEX`" — **true**. `git grep -n "BucketCommunityIndex"` on base: writers none; the
  catalog names the owner `graph/kvcatalog/kvcatalog.go:188` — `derived(graph.BucketCommunityIndex,
  "graph-clustering", "Community detection index"),`; the maintainer that writes it has no production caller
  (`gopls references` on `clustering.NewMaintainer`: tests only).
- "localSearch and the text tier refuse until it is written" — **true**.
  `processor/graph-query/localsearch.go:322` — `return clustering.Community{}, communityNotReady(errors.New("no view of
  " + graph.BucketCommunityIndex))`; `processor/graph-query/textsearch.go:105` — `return nil, 0,
  communityNotReady(fmt.Errorf("%s holds no record under %q", graph.BucketCommunityIndex, levelPrefix))`; the smoke
  test pins it, `internal/smoke/smoke_integration_test.go:305` — `// localSearch: nothing in this stack writes
  community records, so its answer is the refusal.`
- "The follower cannot feed the maintainer" — **true** for the value. `graph/follower/follower.go:46-51`: `Entry` has
  `Key`, `Revision`, `Deleted`, `Bytes`, no value; the replay end is the follower's own
  (`follower.go:244-251`). Accepted design text says change 7 "runs its own watch or extends the follower"
  (`openspec/changes/archive/2026-10-31-setup-04a-04-graph-roots-clustering/design.md:22-27`).
- "The watch's cross-key ascending order is unproven" (#525) — **true, and already checked at run time**.
  `internal/revlag/watermark.go:37-40` — `It accepts revisions only in ascending order: Observe refuses a revision at or
  below Observed. That ascending delivery holds for a KV watch is not promised by nats.go or nats-server (ADR-066)`;
  `graph/follower/follower.go:281-286` declares the feed lost on a refusal and still delivers the entry. No test in the
  tree drives a real watch over interleaved keys and asserts ascending order (`git grep -n -i ascending -- '*.go'`: unit
  tests and comments only).

## 3. Category 2, every spelling of the fact (who keeps `COMMUNITY_INDEX` current)

- Pin: a timer rescan with label propagation, `pin:processor/graph-clustering/doc.go:18` — `// graph-clustering is a
  Tier 1+ component. It polls ENTITY_STATES on a timer`; loop `pin:component.go:1449` — `func (c *Component)
  runDetectionLoop(ctx context.Context) {`.
- Tree: `graph/clustering.Maintainer` (Upsert/Delete/BootstrapDone/Tick), spec `community-grouping`; ledger
  `docs/admission-ledger.yaml:4048-4117` says "processor/graph-clustering (change 7) drives the Maintainer from its
  ENTITY_STATES watch".
- Pin's second read path: four NATS verbs, `pin:query.go:24` — `sub1, err := subscribe(ctx,
  "graph.clustering.query.community", c.handleQueryCommunityNATS)` (and `.members`, `.entity`, `.level`). Pin readers:
  `pin:processor/graph-query/router.go:40` and `pin:processor/graph-query/graphrag.go:2281`, both not carried by the
  tree's graph-query (`openspec/changes/archive/2026-11-06-setup-04a-04-graph-roots-graph-query-communities/design.md:115`).
  Consumers: none (`grep -rln 'graph\.clustering\.query'` over semsource, semconnect, semteams, semboids, semvocab,
  semsparql: zero files). The tree reads records through `pkg/graphview` (`processor/graph-query/community.go:82-95`).
- Readiness spellings: the pin gates detection on graph-index's and graph-embedding's `GRAPH_STATUS` keys
  (`pin:component.go:1509` — `watcher := readiness.NewWatcher(c.natsClient, readiness.KeyGraphIndex,`); it publishes no
  key of its own (**measured**, `grep -n "Publisher" pin:processor/graph-clustering/*.go`: none). The tree's keys:
  `graph/readiness/watcher.go:50-62` (graph-index, graph-embedding, graph-ingest, rule).

## 4. Category 3, adjacent claims

- Open PRs (**measured**, `gh pr list --state open`): #582 (this claim, no files) and #583
  `test(smoke): ask fusion a word question...` (draft, `Closes #581`, no files yet; will edit `internal/smoke`). Overlap:
  `internal/smoke` and the `composed-stack` spec. Merge order: #583 first; this change merges `origin/main` after.
- Specs touched: `community-grouping` (reader of its caller SHALL, `spec.md:191-197`), `entity-state-follower`,
  `composed-stack` (`spec.md:24-25` pins the localSearch refusal), `graph-readiness` (keys).
- Ledger rows: `graph/clustering` `:4048`, `graph/clustering/summarizer.go` `:4119` and `enhancement_worker.go`
  `:4137` (tier 2, #485), `graph/inference` `:2292`, `graph/structural` `:2630`, `graph/llm` `:2611`, `model/wire`
  `:2596`. No `processor/graph-clustering` row yet.
- Open issues: § 9.

## 5. Category 4, who it is for (surface the pin does not have)

- `follower.Entry.Value` (new field): plug-in plumbing; reader graph-clustering's `Deliver` (this change).
- `readiness.KeyGraphClustering` and the `graph-clustering` `GRAPH_STATUS` key: the follower requires a key
  (`follower.go:56-58`); contract line ADR-088 decision 1 (`docs/adr/088-readiness-is-per-producer-aggregation-is-the-consumers.md:46`,
  one key per producer, as information) and ADR-109 D3. No in-tree reader; the composed smoke test reads it as an
  operator would. Owner question (design Q2).
- `processor/graph-clustering.Register`, `Config`, `DefaultConfig`: framework API; semsource
  `cmd/semsource/run.go:312` — `"graph-clustering": graphclustering.Register,`; semboids `configs/flock.json:317-345`.

## 6. Category 5, the problem shape

Shape: one ordered feed of an authoritative bucket applied to an in-memory projection, published on a timer, with
readiness from the feed. Closest instance: graph-embedding on the follower (`processor/graph-embedding/component.go:256`
— `f, err := follower.Open(ctx, follower.Config{`), and graph-index (`processor/graph-index/component.go:236-245`).
Difference: both re-read current truth per key (ADR-108 D4, `docs/adr/108-processor-shell.md:111-118`); the maintainer
cannot (it would pair a newer value with an older revision across keys and break `entity_revision`,
`community-grouping` spec `:195-197`).

## 7. Category 6, the pin's decision records

**Measured**: `grep -l` over `pin:docs/adr/*.md` for `graph-clustering`, `COMMUNITY_INDEX`, `ANOMALY_INDEX`,
`graph/structural`, `KCore`, `anomaly`; then read.

| ADR | Decides, for this package | In tree |
| --- | --- | --- |
| 099 community partition from identity | prefix partition, detection removed, removed keys fail with guidance | ported |
| 087 summary store ownership | COMMUNITY_SUMMARIES worker-exclusive (tier 2) | record only |
| 086 semantic co-location edges | semantic edges in LPA, default off | not ported (gone with LPA) |
| 061 remove community SemanticProvider | removal pattern; "anomaly engine ... already-disabled" (`:89`) | not ported |
| 082 bounded-lag readiness for periodic consumers | clustering's gate on graph-index lag | not ported |
| 085 gate on health, report freshness | `FreshnessWithin` built only in clustering (`:55-62` in tree) | ported |
| 083 / 088 / 109 readiness | one key per producer; reply carries readiness; Health drops error count (`109:98-107` names clustering `:876`) | ported |
| 108 processor shell | D5: "graph-clustering and graph-query have no follower" (`108:125-126`) | ported |
| 054 indexing eligibility | anomaly storm; "Move 1" disables the anomaly engine (`:21-24`); indexing-profile check (`pin:component.go:2540`) | not ported |
| 073 ingestion retention | clustering is timer rebuild, no cursor (`:135-136`) | not ported |
| 079 per-entity poison response | poison latch on clustering reads (`:20`) | not ported |
| 090 authoritative state, materialized views | structural prerequisites internal to anomaly detection (`pin:component.go:252-254`) | not ported |
| 065 composite-key sharding | storage key listing (`:138-142`) | not ported |
| 081 graph view subscription | readers of COMMUNITY_INDEX | ported |

## 8. Shape sweep (read on the pin copy)

1. Second spelling: the four `graph.clustering.query.*` verbs beside graph-query's view (§ 3). Required drop: dead
   surface, the tree's graph-query dropped their readers by accepted design (S9), no consumer reads them.
2. Copied shell: `pin:component.go:951` `Start`, `:1143` `Stop`, `:1196-1285` cleanup, `:1287` — `func (c *Component)
   waitForDependencies(ctx context.Context) error {` (the follower's `Open` wait again). Required: `component.Lifecycle`
   and `graph/follower` (ADR-108, #196).
3. One-implementation interfaces: `pin:component.go:540-553` — `type entityBucketReader interface {` and two more, test
   seams over one KV type. Not carried (their code does not port).
4. Exported surface with no reader: `CommunityRequest` .. `LevelResponse` (`pin:query.go:63-108`); `batch_size`
   (`pin:doc.go:85` — `// is accepted in config but batch-threshold triggering is not implemented.)`); the back door
   `pin:component.go:664` — `subscribeForRequests func(...)`. Required drop.
5. Classification by error text: none (**measured**, `grep -n "strings.Contains\|err.Error()"` over production files:
   zero; `similarity.go:305-356` classify by `errors.As` and codes).

Also: `pin:metrics.go:96` — `metricsOnce sync.Once` and `:168` — `_ = prometheus.DefaultRegisterer.Register(...)`
(global registry, `metric-registry` spec: required, metrics on the injected registry). Context sweep: one private
`cancel context.CancelFunc` (`pin:component.go:651`); no `context.Background`/`TODO`/`WithoutCancel` in production
(**measured**).

## 9. Open issues naming a carried package

**Measured**: `gh issue list --state open --search <term>` for `clustering`, `graph-clustering`, `inference`,
`structural`, `llm`, `anomaly`, `COMMUNITY_INDEX`, `wire`; each hit read.

| Issue | Names | Disposition (design) |
| --- | --- | --- |
| #525 watch order across keys | graph-clustering | retire: probe test plus publication stops on a lost feed (task 2.2) |
| #32 capability seam | graph-clustering, graph/llm, model/wire | retire if owner agrees (Q3): tree-wide seam test (task 3.2) |
| #35 http_embedder split | — | already CLOSED (state CLOSED, measured) |
| #198 graph/inference rest at change 7 | graph/inference, graph-clustering | stays: anomaly lane (Q1) |
| #485 tier-2 community summaries | graph-clustering `component.go:2268-2330` | stays: tier 2, out of scope (owner, 2026-11-10) |
| #488 tier-2 review worker | graph/inference | stays: tier 2 |
| #141 UpdateWithRetryRead | `graph/inference/storage.go:159` (change 7) | stays: anomaly lane |
| #196 lifecycleguard adoption | `processor/graph-clustering/component.go:646` | stays open (other rows); this package adopts `component.Lifecycle` |
| #195 natsfixture.Open adoption | graph-clustering 18 sites | stays open (other packages); new tests use `natsfixture.Open` |
| #477 graphview adoption | clustering site (tier 2 enhancement worker) | stays: no view in this change |
| #581 smoke fusion word question | graph-clustering (community answers later) | stays: claimed by #583 |

Names the word, not the package: #9, #11, #68 (`graph/clustering/metrics.go`, tier 2), #110, #111, #134 (hierarchy
slice, not carried), #195's other rows, #300, #350, #484, #486, #487, #554.

## 10. Adopter seam inventory

- **semsource** sends `detection_interval`, `enable_llm` (always, `cmd/semsource/run.go:961-976`) and optionally
  `entity_id_edges`. Must know: (1) `enable_llm` and `entity_id_edges` are refused at boot (ADR-099: removed keys fail
  with guidance). Default path: boot error naming the key (rank: boot error). Gets: communities by source prefix with no
  graph-index dependency.
- **semboids** (`configs/flock.json:317-345`, `internal/boidgraph/clustering_spike_integration_test.go:199-222`) sends
  `batch_size`, `min_community_size`, `entity_id_edges`, input/output ports; its spike asserts two LPA communities from
  explicit edges. Under prefix grouping that assertion no longer holds (owner accepted the record break 2026-10-31,
  ledger `:4100`). Rank: boot error for keys; the spike's expectation is a migration note.
- **graph-query / products reading COMMUNITY_INDEX** must know nothing new: a record's `entity_revision` is the honest
  mark; before the first publication they get `index_not_ready` (typed runtime error).
- What they should have to know: nothing beyond the removed keys. The gap is the config migration, carried in
  `processor/graph-clustering/MIGRATION.md`.

## 11. Intent check

| Capability (`AGENTS.md`) | This change | Ruling |
| --- | --- | --- |
| index (community index) | admitted, ported | #9 comment 6724212898 item 1 |
| label propagation, edge providers | excluded | PR #483 comment 6543795826 |
| LLM summaries | deferred, tier 2 | PR #483 comment 6543960797; #485 |
| LLM review worker | deferred, tier 2 | #97; #488 |
| anomaly detection (k-core, semantic gap, transitivity) | admitted (ledger `:2313-2314`, #198); order open | **no ruling on order** (Q1) |
| query, fusion, ingest, vocabulary, provenance, tier ladder; durable execution | untouched | — |
