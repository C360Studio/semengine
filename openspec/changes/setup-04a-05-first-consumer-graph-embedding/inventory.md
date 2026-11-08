# Inventory: change 5, `setup-04a-05-first-consumer`

Packages: processor/graph-embedding, pkg/fusion.

base: d4d9eae5b338e614129861b4213ddac269029fe6 (claim commit on origin/main 5b4c268c7f1e9ef9d9e0d23252189088d4193ff0)
pin: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128 (tarball via `gh api repos/C360Studio/semstreams/tarball/8b99efe9`,
extracted to `/tmp/c5pin/src`, local only)

Question: what must change 5 carry, change and drop to port `processor/graph-embedding` and `pkg/fusion` (with
`fusionnats`, `fusionvocab`) at tier 0, and how is it split? Repositories read (`docs/inventory-scope.md`): this tree,
the pin snapshot, semsource (symbol level, local checkout read with grep only, HEAD not verified against `e4febc0d`),
and a zero-hit grep of semteams, semconnect, semboids. Labels: **measured** (command run here), **read** (code read).

## 0. Pin probes: NOT RUN

The three runs the contract asks for (lifecycle suite, unit tests 5x shuffled at one CPU and once under `-race`,
test-text rule list) were attempted for the first two and **denied by the session's permission classifier** (running
downloaded code). Owed before design review: the caller or owner runs
`go test -count=1 -race ./processor/graph-embedding/ ./pkg/fusion/...` and
`go test -count=5 -cpu=1 -shuffle=on` on the same set, on a pin copy. The lifecycle suite was not run by any change-4
port either (`archive/2026-11-03-...-graph-query/inventory.md:124`: the pin does not build against this tree's
harness). Every pin-behaviour statement below is **read**, not probed.

Test-text rules, **measured** by grep on the pin copy:

| Package | `time.Sleep` | `t.Parallel` | `Skip` | other build tags |
| --- | --- | --- | --- | --- |
| processor/graph-embedding | 7 (component_test 2, integration_test 3, predicate_contract 1, query_failure_honesty 1) | 47 over 10 files (none integration-tagged) | 6, `component_test.go:513,590,612,627,649,788` "requires real NATS connection" | none |
| pkg/fusion (+ subpackages) | 1 (`engine_test.go`, Fuse path, not carried) | 6 (`fusionnats/client_test.go` 5, `metrics_internal_test.go` 1) | 0 | none |

## 1. Sizes (measured, `wc -l`, pin)

| Package | Prod | Test | Total | Lifecycle owner |
| --- | --- | --- | --- | --- |
| processor/graph-embedding | 3,253 | 5,701 | 8,954 | `graph-embedding.Component` |
| pkg/fusion | 2,789 | 3,723 | 6,512 | none |
| of which the `Fuse`/`SubQuery` path (`engine.go` 332, `subquery.go` 119, `evidence.go` 64, `client.go` 29; tests `engine_test.go` 332, `subquery_test.go` 85) | 544 | 417 | 961 | none |
| pkg/fusion/fusionnats | 658 | 1,403 | 2,061 | `fusionnats.Client` (one `readiness.Watcher` goroutine) |
| pkg/fusion/fusionvocab | 51 | 52 | 103 | none |
| pkg/fusion/lensregistry | 89 | 114 | 203 | none |

Archived 04A D2 (`openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md:171-173`) puts change 5 at
`output/websocket pkg/buffer pkg/fusion pkg/fusion/fusionnats pkg/fusion/fusionvocab processor/graph-embedding
vocabulary/bfo vocabulary/cco`. Since then: `vocabulary/bfo`, `vocabulary/cco` are `defer-exclude` to semvocab
(`docs/admission-ledger.yaml:2687`, `:2703`, #234). The owner's 2026-11-08 direction names only graph-embedding and
pkg/fusion; `output/websocket` and `pkg/buffer` are not mentioned (owner question).

Pin imports (measured): graph-embedding imports `component graph graph/embedding graph/readiness
internal/lifecyclecleanup metric model natsclient pkg/cache pkg/errs pkg/resource pkg/revlag pkg/types
storage/storeregistry vocabulary`. In this tree: `pkg/lifecyclecleanup`, `internal/cache`, `internal/resource`,
`internal/revlag`; `model` only via the HTTP path (`component.go:918-953`). pkg/fusion imports `graph message metric
pkg/errs storage`; fusionnats `graph graph/readiness pkg/errs pkg/fusion pkg/types`; fusionvocab `pkg/fusion
vocabulary vocabulary/cco`. All present except `vocabulary/cco`.

## 2. Category 1, claimed gaps

- "fusionvocab imports `vocabulary/cco` for two IRI strings" (`docs/admission-ledger.yaml:2712-2714`): **false**.
  `pin:pkg/fusion/fusionvocab/signals.go:40` — `return float64(len(cco.Parents(classIRI)))`: a class-depth function
  over the BFO/CCO tree. This tree's nearest fact is per entity type: `vocabulary/entity_type.go:33` —
  `Superclasses []string`, and `LookupEntityTypesByClassIRI` (`:184-196`). No transitive-ancestor function exists
  (`grep -n -i 'parent\|ancestor' vocabulary/*.go`: none outside `entity_type.go`).
- Embedding verb wire types already exist: `graph/query_search_types.go:21` — `type SemanticData struct {` and `:27`
  `SimilarData`, table rows `graph/query_verbs.go:87-90` (`Enveloped: true`). The pin replies differently:
  `pin:processor/graph-embedding/query.go:58-62` `SimilarResponse{EntityID, Similar, Duration}`, `:85-90`
  `SearchResponse{Query, Results, Duration, EmbedderType}`, not enveloped.
- `graph.embedding.query.status` (`pin:query.go:39`): no row in `graph/query_verbs.go`; `git grep -n 'query\.status'
  -- '*.go'`: 0 hits. Pin ADR-083 removed graph-index's twin (`pin:processor/graph-index/name_index.go:252`).

## 3. Category 2, spellings of the modelled facts

- Readiness envelope: `graph.IndexStatusResponse` (`graph/index_status.go`) and the fusion copy
  `pin:pkg/fusion/contract.go:99` — `type IndexStatus struct {` (with `Phase`, `Revision`, `LastSynced`, which the tree
  no longer has; `FirstFailureAt` it still has, `graph/index_status.go:85`) and the state mirror `:73-83`; fusionnats
  decodes `reading.Raw`
  (`pin:fusionnats/client.go:227`). #110 item 2.
- Readiness producer for graph-embedding at the pin: its own watermark, latch, status bucket and heartbeat
  (`pin:component.go:732`, `:986`, `:1341-1416`, `readiness.go`). In the tree that job is `graph/follower`
  (`graph/follower/follower.go:1-7`), adopted by graph-index (`processor/graph-index/component.go:236-245`).
- Failure breakdown on the envelope: `graph/index_status.go:85` — the `FirstFailureAt` field
  (JSON `first_failure_at`) and `FailedReasons` still exist (#548 keeps them). The tree's only producer,
  `graph/follower/follower.go:424-426`, sets `FailedCount` only (`Config.Failed func() int64`, `:70-71`). At the pin,
  graph-embedding is the only producer of both: `pin:processor/graph-embedding/readiness.go:152-171` —
  `failedCount, failedReasons, firstFailureAt := c.failedSnapshot()`, attached when `failedCount > 0`. The owner's
  2026-11-08 #110 ruling ("nothing sets it") holds for the tree, not for this port.
- Bucket ownership: `graph/kvcatalog/kvcatalog.go:186-187` already declares `EMBEDDING_INDEX` and `EMBEDDING_DEDUP`
  as `graph-embedding`'s.
- Readiness as a request verb: `graph.embedding.query.status` (pin) beside the `GRAPH_STATUS` key.
- Store resolution for offloaded bodies: `graph/embedding/worker.go:133` `StoreResolver`, `pin:pkg/fusion/hydrate.go:42`
  `StoreResolver`, and `storage/storeregistry` (ADR-063 converges them). Fusion PR's question.
- Graph subjects: `pin:fusionnats/client.go:26-31` spells six `graph.query.*` subjects; the tree allows them only in
  `graph/query_verbs.go` (AGENTS.md § 2, `verify+review`).

## 4. Category 3, adjacent claims

- **Open PR #548** (ready, not draft; `claude/revlag-simplify-408`, Closes #408): edits `internal/revlag/watermark.go`
  (`Observe` now returns an error and refuses a revision at or below `Observed`), `graph/follower/follower.go`,
  `graph/index_status.go`, `graph/readiness/index_status.go`, `pkg/graphview/view.go`, ADR-083, ADR-109, specs
  `entity-state-follower`, `revision-watermark`. Overlaps every graph-embedding readiness path and the graphview the
  vector cache moves onto. No other open PR overlaps (`gh pr list --state open`: #548, #550 only).
- Ledger: `graph/embedding` row `known_risks` (`docs/admission-ledger.yaml:3778-3790`): change 5 states the reaction
  to a lost-watch error from `Worker.Run`/`VectorCache.Run`, and its failed map never puts back a failure older than
  the entity's latest terminal outcome. `internal/revlag` row (`:3418-3450`). ADR-071 row (`:4390-4402`).
- Specs: `entity-state-follower`, `embedding-index`, `graph-readiness`, `graph-transport-boundary`,
  `component-lifecycle`, `component-registration`, `metric-registry`, `graph-query`.
- 03B D5 (`archive/2026-10-01-setup-03b-contract-boundary/design.md:338-356`): the lens path of pkg/fusion,
  `fusionnats`, `fusionvocab` are **keep**; `fusion.Fuse`/`SubQuery`/`GraphQueryClient`/`Evidence` (named caller
  `research-graph-execute`) **defer (not ported)**; `searchGraph` keeps a degraded semantic fallback at tier 0/1.
- Change 4 text design D7 (`archive/2026-11-07-...-graph-query-text/design.md:136-146`): the semantic fallback of
  `searchGraph` was not carried; "Change 5 decides the semantic tier of the text strategy (R3)". D9 (`:160-161`):
  ADR-062 and ADR-071 are change 5's; fusion's BM25 adapter's `entity_digests` are gone.
- setup-plan `:439`: prove a true no-embedder composition.

## 5. Category 4, who it is for

New exported surface the port adds beyond the pin: none planned for PR A. Ported framework API with consumer callers
(semsource, measured `grep -rhno 'fusion[a-z]*\.[A-Z]\w*'`): `fusion.Entity` 24, `Request` 23, `NewBodyResolver` 23,
`NewEngine` 22, `Want` 19, `Node` 19, `MapStoreResolver` 18, `Response` 15, `fusionnats.New` 13, `RetrievalClient` 13,
`IndexStatus` 13, `StateReady` 7, `fusionvocab.New` 5 (top of list). semteams, semconnect, semboids: 0 imports.
graph-embedding has no Go importer outside the pin's composition (`componentregistry/register.go`, e2e). SemSource
composes it with `coalesce_ms`, `embedder_type`, `batch_size`, `text_suffixes`
(`semsource:cmd/semsource/run.go:812-838`) and reads its `GRAPH_STATUS` key (`processor/mcp-gateway/tools.go:142`).

## 6. Category 5, problem shapes

- ENTITY_STATES follower plug-in: `processor/graph-index/component.go:236` — `f, err := follower.Open(ctx,
  follower.Config{`. Adopt.
- Serving projection over one KV watch: `processor/graph-query/community.go` on `pkg/graphview` (#524). Adopt for the
  vector cache (#477 ruling).
- Component lifecycle: `component.Lifecycle` (`component/lifecycle_type.go:63`, `:100` `Health(stuck ...string)`).
- Readiness rides the reply: `follower.QueryReady` (`graph/follower/follower.go:360`). Fusion's pre-ask watch gate
  has no tree instance; ADR-109 D3 leaves it to "each port's question" (`docs/adr/109-ready-after-boot.md:137`).

## 7. Category 6, pin ADRs (measured, grep of `docs/adr` at the pin; in-tree status)

| ADR | Governs | In tree |
| --- | --- | --- |
| 054 indexing eligibility | `indexingEligible` (`pin:component.go:1769-1779`, always true, Phase 1 lenient) | no |
| 062 deterministic graph fusion | pkg/fusion engine | no (`docs/adr/063-*.md:12` "not carried") |
| 063 store substrate and resolver | StorageRef lane, fusion hydrate | yes |
| 066 honest readiness | two-hop watermark, fusion copy (`:338-339`) | yes |
| 071 domain-scoped NL retrieval | search `scope` | yes |
| 081 graph view | vector cache site | yes |
| 083, 084, 085, 088 readiness | status bucket, gate, fusion watch | yes |
| 082 bounded-lag readiness | Decision `:64-66`: exact consumers, "fusion honesty envelope", MUST gate on `Ready`; conflicts with dropping fusion's pre-ask gate (PRs C, D) | no |
| 075 framework package admission | titles only; `:76` names fusion framework-owned | no |
| 108, 109, 113 (SemEngine) | shell, ready-after-boot, run context | yes |
| 040, 059, 061, 065, 068, 073, 086, 055 | name the strings only | read titles; not binding on carried code |

## 8. Shape sweep (read on the pin copy)

graph-embedding:

| # | Shape | Pin | Disposition |
| --- | --- | --- | --- |
| G1 | copied shell | own Health/Start/Stop/lifecycleMu/startDone/runtimeDone (`component.go:540-905`) | required (ADR-108, #196) |
| G2 | copied follower | `watchEntityStates`, `latchGraphStateReset`, status bucket, heartbeat (`:1417-1743`, `:986`, `:1341-1416`, `readiness.go`) | required (ADR-108 D2, #109) |
| G3 | second spelling | `graph.embedding.query.status` (`query.go:39`) beside `GRAPH_STATUS` | required (ADR-083, ADR-109 D3) |
| G4 | phantom config | `BatchSize` (`component.go:77`) validated, defaulted, read by nothing | required (Surface audit (b)) |
| G5 | dead branch | `indexingEligible` returns true on every path (`:1769-1779`) | optional; free to drop |
| G6 | fixed timeout | handlers `context.WithTimeout(requestCtx, 30*time.Second)` (`query.go:106`, `:183`), status 5 s (`readiness.go:57`) | required with the verbs (natsclient budget, ADR-112) |
| G7 | inline subjects | `query.go:24,31,39` | required (verify rule) |
| G8 | error-count health term | `Healthy: c.running && errorCount == 0 && ...` (`component.go:567`) | required (ADR-109 D2) |
| G10 | failure breakdown with no tree home | `failed_reasons`, `first_failure_at` (`readiness.go:152-171`); follower carries a count only | required decision (owner): carry via the follower, or drop with #110's ruling; a silent drop is not allowed |
| G9 | tier-1 code | `embedder_type: http`, `model.ResolveEndpointWithConfig` (`:918-953`) | required out (#32, #489) |

pkg/fusion: F1 second spelling `IndexStatus` copy (`contract.go:99`) required (#110 item 2); F2 deferred `Fuse` path
(03B D5); F3 dead `lensregistry` (pin callers: its own test and `storage/storeregistry/storeregistry_test.go` only);
F4 inline subjects (`fusionnats/client.go:26-31`) required; F5 invented root, `watch.Start(context.WithoutCancel(ctx))`
from a request (`fusionnats/client.go:140`) required (AGENTS.md § 1); F6 `New(nats, timeout)`/`Close()` with no
context (`:77`, `:89`, 04A D5 exclusion). Not swept in detail yet: engine_lens.go, engine_facets.go, engine_graph.go
(fusion PR's inventory).

## 9. Open issues naming a carried package

Queries: `gh issue list --state open --search <term>` for `graph-embedding`, `fusion`, `fusionnats`, `fusionvocab`,
`lensregistry`, `embedding`. Kept: #109, #110, #196, #195, #477, #489, #32, #155, #482, #11, #484. Names the word, not
the package: #9, #10, #134, #141 (its graph/embedding part is done, `comment 2026-10-28`).

## 10. Adopter seam

Who: SemSource's composition author and its fusion lens author.

1. Must know: `batch_size` is gone (boot refusal names it); `embedder_type` accepts only `bm25` at tier 0; the
   embedding verbs reply enveloped `results`; if fusion's gate is dropped (owner question; ADR-082 says it gates),
   "not ready" comes back on the answer. SemSource's MCP gateway passes the `graph-embedding` status entry through
   to agents (`semsource:processor/mcp-gateway/tools.go:142`): `failed_reasons` and `first_failure_at` vanish from it
   unless G10 carries them (silent; found nowhere).
2. Do nothing: boot fails naming `batch_size` (loud). Old reply decoders read empty `similar` (silent); only a
   direct caller of `graph.embedding.query.*` is exposed, and none was found in semsource (grep, 0 hits).
3. Find out: boot error for config; `MIGRATION.md` for the reply shape (doc level: the finding).
4. Should know: nothing beyond the config key.

Collision table: not triggered for PR A (no new durable, communication or coordination primitive; the follower,
graphview and Lifecycle are adopted).
