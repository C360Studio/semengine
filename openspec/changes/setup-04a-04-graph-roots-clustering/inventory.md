# Inventory: setup-04a-04-graph-roots-clustering

base: `f3b48f6612282b61e4dc72d34331353216cbf42d` (origin/main at the claim; the claim commit `4aa3599` on
`claude/port-graph-clustering` adds no file). Pin paths are SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`,
written `pin:<path>:<line>`, read from `git archive 8b99efe9` into `/tmp/cl-pin` (local only). All 33 files of
`graph/clustering` there are blob-equal to `gh api repos/C360Studio/semstreams/contents/graph/clustering?ref=8b99efe9`
(`git hash-object` equals the API `sha` for each). `gh api repos/C360Studio/semstreams/compare/8b99efe9...main`:
`identical`, ahead 0, so no unmerged SemStreams work touches the package. Consumer cites: semboids
`8c03cc53836ced93a5df7064473c63ff144e64f1` (the SHA SETUP 03B cites) read for "the workload it drives"
(`docs/inventory-scope.md`, semboids row).

Status: round 4 adds § 13, the premises of the 2026-10-31 rulings; §§ 1-12 are unchanged since `INVENTORY PASS`.
Round 3 (`INVENTORY PASS` at `5c2f610`; § 1 gains the admitted reads, § 12 the semboids record read and its
scope). Round 2 (round 1 `INVENTORY CHANGES REQUESTED` at `180741d`: the ADR-099 row quotes its deciding clauses;
semsource's level use measured; SummaryStatus, e2e readers, two shape findings and the consumer scope corrected).
Round 1 was inventory only. The design (`design.md`) is drafted beside it at the orchestrator's request and is
not reviewable until this inventory has `INVENTORY PASS` (architect contract, Required workflow step 3).

Kind of change: a port of one package by the owner's order ruling (#9 comment 6386856657, item 8: "`graph/clustering`
without the LLM summarizer (#141, #68)") under the change 4 scope ruling (#9 comment 6386603659: tier-0 halves only;
"`EnhancementWorker` holds the interface, not `*LLMSummarizer`").

Briefing hypotheses checked:

- "Port the tier-0 half; only the LLM summarizer is tier 2": the tier-2 lane is four files, not one region:
  `summarizer.go:499-863`, `enhancement_worker.go`, `summary_store.go`, `metrics.go` (§ 8). The worker exists only to
  run an LLM summarizer; its only implementation is `*LLMSummarizer` (`pin:graph/clustering/enhancement_worker.go:56`,
  `:106-110` refuses a nil one).
- "The tier-0 half is LPA plus the statistical summarizer" (SETUP 03B design row `:218`): the pin's own accepted
  ADR-099 (2026-08-23) decides that LPA's edge-synthesis chain, the semantic-edge provider and the fake hierarchy
  leave the tree. The pin never implemented it (§ 6). 03B did not read ADR-099 (`git grep -n '099' -- openspec/changes/
  archive/2026-10-01-setup-03b-contract-boundary/`: no hit on the ADR).
- "#111, #108, #300, #408 name the package": none does (§ 3).

## Pin probe

Run on `/tmp/cl-pin`, go1.26.4 darwin/arm64, `GOPROXY=off GOFLAGS=-mod=mod`:

- `go test -race -count=1 ./graph/clustering`: ok 1.444 s.
- `go test -count=5 -cpu 1 -shuffle=on ./graph/clustering`: ok; seed `-test.shuffle 1793448128639624000`, no failure.
- `go test -count=1 -cover ./graph/clustering`: 57.0 % of statements (unit). The 80 % floor applies (03B D10, extended
  list names `graph/clustering`).
- `go test -tags integration -count=1 ./graph/clustering`: ok (Docker; `TestMain` uses `natsclient.NewSharedTestClient`,
  `pin:graph/clustering/main_test.go:22`).
- Test-text rules: `time.Sleep` none; `Skip`/`Skipf`/`SkipNow` none; build tags: `integration` only, four files
  (`main_test.go`, `storage_integration_test.go`, `summary_store_integration_test.go`,
  `enhancement_worker_race_integration_test.go`).
- Lifecycle suite: no service or component is ported. The one lifecycle owner, `EnhancementWorker`
  (`Start`/`Stop`, `pin:graph/clustering/enhancement_worker.go:215,511`), is tier 2 and not carried (§ 8).
- Behaviour probes (`/tmp/cl-probe-src/zz_probe_test.go`, package `clustering_test`, run with `-run TestProbe -v`):
  - P1: `EntityIDProvider`'s sibling cache is built once and `ResetEdgeCache` does not refresh it. Before: `e1`
    sees `[e2]`; after adding `e3` to the base and calling `ResetEdgeCache` (what `DetectCommunities` calls each
    cycle, `pin:graph/clustering/lpa.go:174-176`): `e1` sees `[e2]`, `e3` sees `[e1 e2]` (asymmetric virtual edges).
    `ClearCache` has no production caller (`grep -rn ClearCache processor/graph-clustering`: none); the chain is built
    once in `Start` (`pin:processor/graph-clustering/component.go:1052`, `:1354`).
  - P2: two triangles joined by one edge, no virtual edges. Level 0: two communities; level 1: one community of six;
    level 2: two communities again. Level 2 is finer than level 1; `ParentID` is nil at every level.
  - P3: `StatisticalSummarizer.SummarizeCommunity` on two entities with tied term scores, 200 runs: 5 distinct
    `Keywords` orders (`sort.Slice` without a tie-break, `pin:graph/clustering/summarizer.go:205`).
  - P4: `NewQueryManagerProvider(nil).GetAllEntityIDs` returns "full graph scan not yet implemented" on every call
    (`pin:graph/clustering/provider.go:53-67`).

## 1. The claimed gap

The package is absent here: `ls graph/` at base has no `clustering`; `git grep -n 'graph/clustering"' -- '*.go'`: no
hit. Already present: `graph.Provider` (`graph/types.go:93` — `type Provider interface {`), the bucket names
(`graph/constants.go:31` — `BucketCommunityIndex = "COMMUNITY_INDEX"`, `:38` — `BucketCommunitySummaries =
"COMMUNITY_SUMMARIES"`) and their catalog rows (`graph/kvcatalog/kvcatalog.go:188-190`, owner `graph-clustering`),
`natsclient.KVStore.Keys` with the empty-bucket case (`natsclient/kv.go:571`, `:582` — `if err ==
jetstream.ErrNoKeysFound {`), `natsclient.FilteredKeys` (`natsclient/kv.go:630`), `types.EntityID` prefix methods
(`pkg/types/entity_id.go:268-285`), `message.ParseEntityID` delegating to `pkg/types` (`message/types.go:13`).
The admitted reads of entity state (`.agents/skills/query-pattern/SKILL.md:14-19`): `graph.NewExactEntityReader`
(`graph/exact_entity.go:46`), graph-ingest's `batch` and `prefix` verbs (`graph/query_verbs.go:33`; full states per page,
`graph/query_prefix_types.go:9-60`, limit `MaxPrefixQueryLimit` `:7`), graph-index's verbs, owner bucket access. The
pin's `EntityProvider` (`pin:graph/clustering/lpa.go:44`) and `EntityQuerier`
(`pin:graph/clustering/enhancement_worker.go:33`) live in files not carried.

## 2. Every current spelling of the facts the package models

- Community partition by explicit edges: none in the tree.
- Grouping by entity-ID prefix: `pkg/types/entity_id.go:268` (`DeploymentPrefix`), `:274` (`SourcePrefix`), `:279`
  (`TaxonomyPrefix`), `:285` (`TypePrefix`); hierarchy containers in `graph/inference` (`graph/inference/
  container_entity.go:17` — "Transitional until gh606 retires containers"). Ported ADR-102 calls the containers "a
  second spelling of the ADR-099 partition" (`docs/adr/102-entity-id-segment-semantics.md:129`) and fixes ADR-099's
  levels (`:103` — "ADR-099 levels are 0 = 4 parts, 1 = 3 parts (source), 2 = 2 parts."). The pin's LPA levels 0-2
  are re-runs over one entity set (P2), a different meaning of "level" from the one ADR-102 records.
- At the pin, the package spells "explicit edge exists" twice: `EntityIDProvider.isExplicitEdge` (neighbour set) and
  `explicitEdgeWeight` (numeric weight) (`pin:graph/clustering/entityid_provider.go:461`, `:445`); both go with the
  provider chain if it is not carried.
- Entity-ID parsing in the package: `semtypes.ParseEntityID` (`pin:graph/clustering/entityid_provider.go:212`) and
  `message.ParseEntityID` (`pin:graph/clustering/summarizer.go:19`); here the second delegates to the first.

## 3. Adjacent claims

### Open pull requests

`gh pr list --state open --json number,title,isDraft,changedFiles,files` at base: #483 (this claim, 0 files); #465
(markdownlint-cli2 bump: `package.json`, `package-lock.json`); #464 (nats.go 1.54.0 → 1.55.0: `go.mod`, `go.sum`).
No file or capability overlap. #464 changes the library the storage tests drive; if it merges first, the integration
run is repeated at the new version. #465 changes `task docs:check`'s linter; no ordering need.

### Open issues naming the package

Queries: `gh issue list --state open --search clustering`, `community`, `COMMUNITY_INDEX` (each `--limit 200`), and
each issue the brief named, read in full with comments.

| Issue | Names it at | Disposition (design § D7) |
| --- | --- | --- |
| #32 capability seam | body edit and 2026-10-24 ruling comment: `graph/clustering/summarizer.go` from `:499` | stays: since 2026-10-31 only the provider plumbing; the clustering tier-2 rows name #485 (§ 13) |
| #68 metric `sync.Once` | 2026-10-24 drift comment: `graph/clustering/metrics.go:43-71` | stays: `metrics.go` is tier-2-only (read only by the worker), excluded |
| #141 `UpdateWithRetryRead` adoption | body: `graph/clustering/summary_store.go:179` (also ADR-110 D7) | stays: `summary_store.go` excluded |
| #477 `pkg/graphview` adoption | body: `graph/clustering` `enhancement_worker.go:239` | stays: worker excluded; the site moves with #32 |
| #9 epic | order ruling item 8 | addressed |

Names the word, not the package: #105, #106, #196, #198, #406, #11, #479, and #195 (its list counts
`natsclient.NewTestClient` sites; the package's tests use `NewSharedTestClient`, and `git grep -c` for
`natsclient\.NewTestClient(` at the pin under `graph/clustering` finds none). Named in the brief but not naming the
package (body and comments read): #111 (graph-ingest, graph-query summary, NL fallback, spatial),
issue #108 (graph-index), #300 (graph-ingest), #408 (revlag).

### Specs, ledger, records

- No `openspec/specs/` capability names the package (`git grep -n -i clustering -- openspec/specs`: none).
- Ledger: no `source_path: graph/clustering` row. `graph/llm` row (`docs/admission-ledger.yaml:2539`) names
  `graph/clustering` as its next reader; `graph/structural` row (`:2558`) names `processor/graph-clustering`
  (change 7); `graph/inference` row (`:2226`) defers its anomaly half to change 7 with graph-clustering.
- SETUP 03B design row `:218` admits `graph/clustering` (5,516) at tier 0 ("LPA ... and `StatisticalSummarizer` ... are
  deterministic"); D10 extended list puts it under the 80 % floor; `:698` names the proving harness "communities
  written to `COMMUNITY_INDEX` with `EnableLLM` false", consumer semboids. Setup plan tier 0: "clustering on explicit
  edges" (`docs/setup-plan.md:78-79`); tier 2: "clustering summaries" (`:82`).
- Foundation design: non-context `Stop` adapt row names `graph/clustering` (`:423`) for `EnhancementWorker`.

## 4. Who reads the surface (pin-wide production readers, outside the package)

Command: per exported name, `grep -n "clustering\.<Name>\b"` over the 15 non-test importing files at the pin.

| Reader | Symbols |
| --- | --- |
| `processor/graph-clustering` (change 7) `component.go` | `NewLPADetector`, `CommunityDetector`, `NewNATSCommunityStorage`, `NATSCommunityStorage`, `NewStatisticalSummarizer`, `NewEntityIDProvider`, `EntityIDProvider(Config)`, `DefaultEntityIDProviderConfig`, `NewSemanticEdgeProvider`, `SemanticEdgeProvider`, `SemanticEdgeParams`, `WeightConfig`, `DefaultSemantic*`, `Provider`, `NewLLMSummarizer`, `LLMSummarizerConfig`, `NewEnhancementWorker`, `EnhancementWorker(Config)`, `EntityQuerier` |
| `processor/graph-clustering` `similarity.go`, `metrics.go`, `anomaly.go`, `query.go` | `SemanticNeighborFinder`, `ErrSemanticIndexNotReady`, `ErrSemanticQueryTransient`, `SemanticEdgeMetrics`, `Provider`, `Community`; `storage.GetAllCommunities` (`anomaly.go:342`) |
| `processor/graph-query` (item 9) | `Community` (`community_cache.go`, `graphrag.go`); `CommunitySummaryRecord`, `MembershipHash`, `SummaryKey`, `SummaryStatusEnhanced/Failed` (`summary_view.go`, `component.go`) |
| SemStreams e2e harness (not admitted, not counted as readers) | `test/e2e/client/nats.go`, `test/e2e/scenarios/{tiered_semantic,tiered_statistical,validate_partition_colocation,validate_thematic_eval}.go`, `test/e2e/scenarios/community/validator.go`: `Community`, `CommunitySummaryRecord`, `MembershipHash`, `SummaryKey`, statuses |
| semboids `8c03cc53` | test only: `internal/boidgraph/lpa_unit_test.go` (`NewLPADetector`, `DetectCommunities`, `Community`, a fake `CommunityStorage`) |

No production reader anywhere: `UpdateCommunities`, `InferRelationshipsFromCommunities`, `InferenceConfig`,
`DefaultInferenceConfig`, `InferredTriple`, `WithProgressiveSummarization`, `ComputePageRank`,
`ComputePageRankForCommunity`, `PageRankConfig`, `DefaultPageRankConfig`, `PageRankResult` (package-internal reader:
`ComputeRepresentativeEntities` from `summarizer.go:235`), `QueryManagerProvider`, `PredicateProvider`,
`RelationshipQuerier`, `Relationship`, `Direction*`, `NewNATSCommunityStorageWithConfig`, `CommunityStorageConfig`,
`GetCreatedTriples`, `ClearCache`, `GetCacheStats`, `GetSiblingEdgeMetrics`, `IsActive`; storage `GetCommunity`,
`GetEntityCommunity`, `GetCommunitiesByLevel` (readers only the detector getters and the dormant inference),
`DeleteCommunity`, `Clear`. Detector getters `GetCommunity`, `GetEntityCommunity`, `GetCommunitiesByLevel`: no
caller (`grep -rn 'c.detector\.' processor/graph-clustering`: `DetectCommunities` only). `Community` fields with no
production reader (`grep -rn '\.<Field>\b' processor/graph-query processor/graph-clustering gateway`): `ParentID`,
`SummaryTruncated`, `LLMSummary` (0 each; `graphrag.go:2414` says the field is empty since ADR-087). `SummaryStatus` is
stored as `"statistical"` (`pin:graph/clustering/summarizer.go:126`); its one reader is the e2e harness
(`pin:test/e2e/scenarios/tiered.go:761`), no production reader.

## 5. The problem shape

Iterative partition over a provider-supplied graph, write-then-prune into a derived bucket, and a deterministic
statistical digest per group. Closest instance here: write-then-prune of a derived view in `graph/embedding.Storage`
(`graph/embedding/storage.go:138` — "Every read-modify-write goes through natsclient.KVStore.UpdateWithRetryRead"),
which holds a `*natsclient.KVStore` (`:149`). The design adopts its store shape (KVStore, not a raw
`jetstream.KeyValue`). No reusable primitive is established, so no adoption sweep is owed.

## 6. Decision records at the pin, and their state here

Search: every `docs/adr/*.md` at the pin for `graph/clustering`, `clustering.`, `LPADetector`, `COMMUNITY_INDEX`,
`COMMUNITY_SUMMARIES`, `MembershipHash`, `EntityIDProvider`, `SemanticEdgeProvider`, `EnhancementWorker`, `community
detection`, `Label Propagation`; then titles naming clustering, communities, summaries, partitions.

| ADR | Decides | Here |
| --- | --- | --- |
| 099 Community partition derived from entity identity; detection removed (Accepted 2026-08-23) | see 6b | not ported (`docs/adr/100-…:16`); ADR-102 (ported) amends its levels and binds them (`docs/adr/102-…:103`, `:129`). Not implemented at the pin (P1, P2) |
| 087 Community summary store ownership | worker-exclusive, content-addressed `COMMUNITY_SUMMARIES`; one `MembershipHash` | not ported; governs the tier-2 lane only |
| 086 Semantic co-location edges | mutual-kNN semantic tier, weighted LPA; outcome default-off | not ported; ADR-099 removes the mechanism |
| 085 Gate on health, report freshness | write-then-prune, never clear before rebuild | ported (`docs/adr/085-…:17` names `LPADetector` as not ported) |
| 061 Remove orphaned `SemanticProvider` | removal-with-recoverability pattern | not ported; history |
| 081, 082, 084 | graphview sites; bounded-lag readiness of graph-clustering; readiness licenses health | 081, 084 ported; 082 governs the component (change 7) |
| 034, 049, 054, 065, 073, 079 | name graph-clustering, community detection or `storage.go:206` in passing (034 `:16` a release tag; 079 `:20` a second watcher) | n/a |

## 6b. ADR-099's deciding clauses, quoted

- Base partition (`pin:docs/adr/099-community-partition-derived-from-entity-identity.md:25-30`): "The community base
  partition is **derived, not detected**: `community(entity, level)` is the entity ID prefix at that level"; "membership
  is a pure function of the ID and is **never stored** — group records carry bounded metadata only, and member
  enumeration rides the existing paginated prefix-query lane." ADR-102 restates the levels here: "ADR-099 levels are 0 =
  4 parts, 1 = 3 parts (source), 2 = 2 parts." (`docs/adr/102-entity-id-segment-semantics.md:103`), and "Hierarchy
  containers are a second spelling of the ADR-099 partition and are retired with it" (`:129`).
- Removal (`:35-36`): "LPA and its supporting machinery — edge synthesis and its weights/caps configuration, the
  semantic-edge provider, detection determinism — **leave the tree**."
- Re-entry (`:36-40`): "Detection may return only as an overlay under a re-entry contract: explicit relationship edges
  in, a namespace the serving path does not read out, a fixture that can fail as the shipping gate, and never a
  readiness dependency of graph-query."
- The ruling it records (`pin:docs/proposals/gh606-derived-communities-design.md:321-326`): "Q1 (largest) — **RULED:
  REMOVE.** LPA + providers + `semantic_edge_provider.go` + weights/caps config + detection-determinism machinery leave
  the tree now"; "Owner basis: LPA over this entity-ID model is re-derivation of the IDs — waste of compute"; "by this
  epic's own ruling an unmeasured overlay cannot ship anyway." The recommendation it accepted names the file: "REMOVE
  the LPA path from the tree now — `lpa.go` detector, provider chain, `semantic_edge_provider.go`" (`:192-194`).
- Overlay contract (`:183-191`): "detection survives only as an optional overlay over EXPLICIT relationship edges,
  default-off, shipping only once a fixture can measure it (the current ground-truth check — never better than 1/3,
  warn-only — cannot)"; "output = overlay records in a namespace the serving path does not read; gate = a fixture that
  can fail; it never writes the base partition records and never becomes a readiness dependency of graph-query."
- Measurement (ADR-099 `:16-21`): "largest community 47–82% of the graph"; "The hierarchy levels above 0 were re-runs
  over the full entity set, not a hierarchy"; "The e2e coherence check could not exceed 1 of 3 and was warn-only."
- The ruled design for the base: record shape (`gh606…design.md:128-145`, no `Members`, no `entity.{level}.{id}` keys,
  `membership_hash` via `clustering.MembershipHash`), write-on-change (`:106-107`), Q5 "deterministic text-bearing rep
  selection" (`:335`), Q6 "record singleton groups" (`:338`), Q8 level 1 served by default (`:344`); summaries
  only for groups of two or more (`:219`).
- Order of rulings: ADR-099 2026-08-23; SETUP 03B admitted "clustering on explicit edges" 2026-10-01 without reading
  it (§ intro).

Pin capability spec `graph-clustering` (`pin:openspec/specs/graph-clustering/spec.md`, 14 requirements): edge
synthesis `:6`, `:29`, `:45`; non-destructive rebuild `:66`; projection identity `:135`; semantic edges `:164`,
`:213`, `:256`; determinism `:301`; LLM summary store `:320`, hash `:358`, volume `:372`; component `:385`, `:408`.

## 7. Library behaviour the port relies on (#170, ADR-112)

Carried: `kv.Put` failure classes (`nats.ErrMaxPayload` permanent, `pin:graph/clustering/storage.go:138`),
`jetstream.ErrKeyNotFound` on `Get`. Changed: empty-bucket key listing moves to `natsclient.KVStore.Keys`
(`natsclient/kv.go:582`), already tested in the tree, so the design adds no new library statement. No timing,
ordering or bound statement is added.

## 8. Lifecycle owners, NATS use, payloads, and the tier split (non-test pin lines)

| File | Lines | Tier | Lifecycle / NATS |
| --- | --- | --- | --- |
| `types.go` | 126 | 0 | none |
| `storage.go` | 526 | 0 (`MembershipHash` `:42-48` is the tier-2 key) | `COMMUNITY_INDEX` Put/Get/Keys/Delete; no goroutine |
| `lpa.go` | 820 | 0 | none; `mu` held for a whole run (`:76`, `:163`; the getters take it too, `:616-637`) |
| `summarizer.go` `:1-498` | 498 | 0 | none |
| `summarizer.go` `:499-863` (`LLMSummarizer`, `ProgressiveSummarizer`) | 365 | 2 | imports `graph/llm` (`:12`) |
| `pagerank.go` | 335 | 0 | none |
| `entityid_provider.go` | 521 | 0 (ADR-099: removed) | none |
| `provider.go` | 272 | 0 (dead) | none |
| `semantic_edge_provider.go` | 935 | 1 by measurement (ADR-086 on neural; not provider-gated in code) | none |
| `enhancement_worker.go` | 549 | 2 | `Start`/`Stop`, `WatchAll` on `COMMUNITY_INDEX`, writes `COMMUNITY_SUMMARIES` |
| `summary_store.go` | 254 | 2 | `COMMUNITY_SUMMARIES` CAS loop `:179` |
| `metrics.go` | 117 | 2 | Prometheus, worker only |
| `doc.go` | 198 | — | — |

No payload type, no subject. Total 5,516; tier-2 1,285; ADR-099-removed and dead 1,728; semantic 935.

## 9. Shape sweep (pin copy, every non-test file)

| # | Shape | Pin cite | Text |
| --- | --- | --- | --- |
| S1 | back door (#186) | `storage.go:64`, `:78-80`, `:153-155`, `:201`, `:270`, `:400-415` | `testStore map[string]*Community // In-memory store for testing when kv is nil` |
| S2 | exported surface, no reader | `storage.go:189` | `func (s *NATSCommunityStorage) GetCreatedTriples() []message.Triple {` (triples built only for it, `:160-186`) |
| S3 | classification by error text | `storage.go:286`, `:434`, `:481`; `summary_store.go:237`; `enhancement_worker.go:27-28` | `strings.Contains(err.Error(), "no keys found")` |
| S4 | silent skip (fail-closed rule) | `storage.go:250-258`, `:302-311` | `continue // Skip errors for individual entries` |
| S5 | silent success on cancel | `storage.go:290`, `:438`, `:451`, `:485`, `:496` | `if stderrors.Is(err, context.Canceled) {` → `return nil` |
| S6 | advertised-absent: hierarchy | `lpa.go:576-605`; `types.go:14`, `:20-21`; `doc.go:6-9` | `// Link communities to their parents (future enhancement)` (P2) |
| S7 | exported surface, no reader | `lpa.go:607`, `:616-637`, `:640-771` | `InferRelationshipsFromCommunities` ("DORMANT", `:787`, `:811`), `UpdateCommunities`, three getters |
| S8 | layered interface, one impl | `types.go:55-78` | `CommunityDetector` (one implementation; component holds it) |
| S9 | nondeterministic order on ties | `summarizer.go:205`, `:284`, `:412` | `sort.Slice(scores, func(i, j int) bool {` (P3) |
| S10 | advertised-absent provider | `provider.go:53-67` | `"full graph scan not yet implemented - use PredicateProvider instead"` (P4) |
| S11 | stale cache, no refresh caller | `entityid_provider.go:493-503`, `:316-372` | `func (p *EntityIDProvider) ClearCache() {` (P1) |
| S12 | wrong sentinel for empty input | `storage.go:111`, `:197`, `:322`, `:345` | `errs.WrapInvalid(errs.ErrMissingConfig, …, "id is empty")` |
| S13 | two parsers of one ID | `summarizer.go:19`, `entityid_provider.go:212` | `message.ParseEntityID` / `semtypes.ParseEntityID` |
| S14 | unexported-reader-only exports | `pagerank.go:10-62` | `ComputePageRank`, `PageRankConfig`, … |
| S16 | silent clamp of configured values | `lpa.go:97-119` | `if maxN > MaxIterationsLimit {` (config refuses only `<= 0`, `pin:processor/graph-clustering/component.go:385`; `:1385` feeds it) |
| S17 | log-only degrade | `lpa.go:262-268` | a `Prune` failure is a Warn; the run reports success while stale records stay until the next cycle |
| S15 | doc drift | `doc.go:30-35`, `:96-122`, `:160-168` | names the LLM worker, member-of triples, `Pause`/`Resume` |

## 10. Size against the one-package rule (#159)

Whole package 5,516 pin lines; tier-0 half (less 1,285 tier-2) 4,231; less `provider.go` (dead) 3,959; less
`semantic_edge_provider.go` 3,024; less `entityid_provider.go` (ADR-099) 2,503 (`types` 126, `storage` 526, `lpa` 820,
`summarizer` 1-498, `pagerank` 335, `doc` 198). Only the last is under the ~3,000 bound without a waiver. The parts
ADR-099's base keeps: `types` 126, `storage` 526, `summarizer` 1-498, `doc` 198 = 1,348 pin lines considered, plus
new partition code (no pin code exists for it).

## 11. Collision table

No new durable, communication or coordination primitive is proposed: `COMMUNITY_INDEX` and its catalog row exist
(§ 1); the port supplies its writer. Same-class note: "community level" already has ADR-102's meaning (prefix length)
in the tree; the pin's LPA levels mean something else (§ 2).

## 12. Adopter seam inventory

Scope (`docs/inventory-scope.md`): semboids' row allows the workload it drives (streams, rates, entities) and symbol
use of `pkg/lifecycle` and `processor/rule`. Relied on, as the workload it drives: `configs/flock.json`; the boid ID
shape `%s.%s.sim.flock.boid.%d` (`internal/boidgraph/payload.go:42` at `8c03cc53`); and how it reads the
`COMMUNITY_INDEX` records that workload fills: it decodes `level` and `members` from each record and keeps level 0
(`internal/api/graphstream_decode.go:78` — `Members []string` with tag `members`; `:86` — `return c.Members, true, nil`).
Under option C the records carry no `members`, so semboids sees empty communities until it changes its own read.
Not relied on: its `graph/clustering` symbol use in `internal/boidgraph/lpa_unit_test.go`, outside the row; using
it needs a scope-doc change or a new owner ruling (#22 is closed). semsource's row allows its use of retained
contracts; its `graph.query.searchGraph` request is one.

Reached from outside: products composing `processor/graph-clustering` (semboids `configs/flock.json:288-300` at
`8c03cc53`: `"enable_llm": false`, `"entity_id_edges": {"include_siblings": false, "include_system_peers": false}`,
so LPA on explicit flock edges only); semboids reads level-0 `Members` from `COMMUNITY_INDEX` through a graphview
(`internal/api/graphstream.go:34`, `:44` — `communities map[string][]string // community ID → members (level 0)`);
semboids' test implements `CommunityStorage` and runs `NewLPADetector` on two cliques
(`internal/boidgraph/lpa_unit_test.go:139-165`). semsource composes graph-clustering only behind
`graph.enable_clustering` (default false; its README calls it "tier-2 community routes").

1. What must they know: a `Provider` returns neighbours for `"both"` and positive weights; the detector is the one
   writer of `COMMUNITY_INDEX`; a provider that caches edges implements `ResetEdgeCache`; which levels exist.
2. If they do nothing: a fake `CommunityStorage` with removed methods still compiles (extra methods are allowed); a
   query for a level that is not a hierarchy returns a partition with no stated meaning (P2), silently.
3. Where they find out: interface shrink, compile error for callers of removed methods; level meaning, nowhere.
4. What they should know: the provider contract only; level meaning should be a refusal, not a doc.

## 13. Round 4 premises (rulings of 2026-10-31)

- Watch precedent for change 7: graph-index declares its `ENTITY_STATES` input as a KV watch port
  (`processor/graph-index/config.go:136` — `{Name: "entity_watch", Config: component.KVWatchPort{Bucket:
  graph.BucketEntityStates}}`) and runs it through `graph/follower`, whose `Entry` carries `Key`, `Revision` and
  `Deleted` (`graph/follower/follower.go:46-51`) and whose `Deliver` is called in delivery order on one goroutine
  (`:67-69`). The maintainer's `Upsert`/`Delete` take exactly those facts.
- ID bound: `MaxEntityIDBytes = 256` (`pkg/types/entity_id.go:13`), checked by the canonical parser (`:197`).
- Position order: `TaxonomyPrefix` returns `org.platform.system.domain` (`pkg/types/entity_id.go:279-281`),
  `SourcePrefix` `org.platform.system` (`:274-276`), `DeploymentPrefix` `org.platform` (`:268-270`). ADR-099's own text
  at the pin says "level 0 = system (4 parts), 1 = domain (3)" (`pin:docs/adr/099-…:25`); ADR-102 amends that phrase
  (`docs/adr/102-entity-id-segment-semantics.md:55`), and the change follows ADR-102.
- Corpus document frequency at the pin exists to stop common predicates and type parts crowding out distinctive
  keywords (`pin:graph/clustering/lpa.go:326-333`); the ruling drops it so one entity cannot re-rank every group.
- Key-scan cost of graph-ingest's `prefix` verb: "the server fetches ALL keys matching the prefix, sorts them, then
  slices the requested page" (`graph/query_prefix_types.go:30-33`); held for item 9.
- Tier tracker (owner, 2026-10-31): epic #484; #485 LLM community summaries (every clustering tier-2 item, including
  `MembershipHash`); #486 classifier LLM adapter; #487 answer synthesis; #488 inference review worker; #489 HTTP
  embedder (epic #11). #32 keeps only `graph/llm`, `model/wire`, `go-openai`. Existing ledger rows to re-point:
  `docs/admission-ledger.yaml:3741` (`graph/embedding/http_embedder.go`), `:3833` (`graph/query/classifier_llm_adapter.go`).

## Intent check

| Capability (`AGENTS.md`, setup plan) | State | Ruling |
| --- | --- | --- |
| Clustering on explicit edges, tier 0 | admitted | 03B Q11 (#8 comment 5930898291, 2026-10-01); predates no ruling here, but postdates and did not read ADR-099 |
| LPA detection | removed by the owner in SemStreams | ADR-099 / gh606 Q1 "RULED: REMOVE" (2026-08-23); no SemEngine ruling |
| ID-derived base partition | ruled in SemStreams; levels already bound here | ADR-099; ADR-102 d6 (ported, `docs/adr/102-…:103`) |
| Detection overlay (explicit edges, own namespace, default off) | allowed in SemStreams only with a fixture that can fail | ADR-099 `:36-40`; no SemEngine ruling |
| Clustering summaries (LLM), tier 2 | deferred | #9 6386603659; #32 |
| Statistical community summary | admitted | 03B row `:218`; ADR-099 keeps it (gh606 design `:103-104`) |
| EntityID virtual edges, semantic edges | removed | ADR-099 `:35-36`; no SemEngine ruling |
| Inferred `clustered-with` triples | no reader | no ruling; setup plan `:106-108` (relevance is not an asserted fact) |

Each "no SemEngine ruling" row is owner question 1.

## Open evidence questions

- Measured (semsource `4093d3ce421371f4a99d7168e372552899bf6795`): its one production community path sends
  `{"query": …, "summarize_threshold": 1}` to `graph.query.searchGraph` (`processor/mcp-gateway/query_tools.go:103`),
  no `level`, so the pin decodes level 0 (`pin:processor/graph-query/searchgraph.go:90`). Its
  `scripts/measure-communities/main.go:44` is an operator script, default level 0. No production request for level > 0.
- Whether the owner's ADR-099 ruling binds SemEngine is a ruling, not evidence: owner question 1.
