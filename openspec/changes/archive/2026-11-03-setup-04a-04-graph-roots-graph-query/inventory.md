<!-- markdownlint-disable MD013 MD025 -->

# Owner summary

Nothing here is decided. Tags: *(measured)* I ran or counted it. *(read)* from reading the code, not run. *(reviewer)* the inventory
reviewer reported it and I did not re-read it. *(judgement)* my opinion.

**What the query component is.** At the frozen SemStreams version it is 6,745 lines of code and 10,028 lines of tests *(measured)*.
About 2,445 of those need not come at all: the language-model parts you already excluded, the store for language-model summaries, and
tests of the old hand-written start and stop code *(measured)*. Its community half cannot be copied, because the community records you
redesigned no longer list their members *(measured)*.

**The split: five pull requests, in this order, after the two flake-rule pull requests.**

| Step | What the step adds | When it can answer | Lines |
| --- | --- | --- | --- |
| 1 | The component starts; direct lookups by ID, alias, list of IDs, ID prefix, links, name | On merge. SemConnect cannot decode the lookup reply until one side changes *(read)* | 3,825 |
| 2 | Path search, counts by hierarchy, the graph overview | On merge. Its forwarding of similarity, time and place questions refuses until later changes *(read)* | 2,560 |
| 3 | Reading community records; search inside one entity's community | Refuses until a later change writes those records *(read)* | 3,246 |
| 4 | Free-text search across communities | Refuses until the later changes that add similarity search and community records *(read, not run)* | 2,937 |
| 5 | Questions about time, place and paths; the search call SemSource uses | Time and place, and SemSource's call, refuse until those later changes *(read)* | 1,760 |

Line counts are from a script *(measured)* and leave out new tests the rulings add, and the rewrite of the 47 test calls to a helper
that does not come across *(measured)*. Steps 1 and 3 are over your limit of about 3,000 as counted. Each can split in two, but the
first half of each then answers no request *(measured for step 1, hand sum for step 3)*.

**Rulings I need from you.** Letters and question numbers point to the detail in sections 13 and 15.

1. **May agents read how SemSource and SemConnect call us?** (F) Decide this first, because other items lean on it. Their
   allowed-reading lines list imports, baselines, a contract matrix and named seams, and in my judgement none clearly covers how they
   call these requests *(judgement)*. If you refuse: the list of who uses each request, the evidence for ruling 3, and part of the first
   Fable question fall. The old version's own readers still show every request is in use *(measured)*.
2. **How is the limit counted?** (A) Lines assigned from the old version, or lines each pull request really holds? This decides whether
   steps 1 and 3 split.
3. **How does search reach a community's members?** (Question 2) The decision record you ported says members are never stored and are
   read page by page through prefix queries. Each page re-reads every ID in the community, so a limit on results does not limit the
   cost *(measured)*. With 2 KB entities, in a one-million-entity graph, on an idle laptop:

   | Community | First 23 pages (about 10,000 entities) | All pages |
   | --- | --- | --- |
   | 10,000 members | 0.85 s | same |
   | 100,000 members | 3.9 s | 36 to 53 s over three walks |
   | Whole deployment, 1,000,000 members | 54.5 s | not run |

   The last row is reachable: a caller sends the top level, and that level's community is the whole deployment *(read)*. One search reads
   up to five communities by default *(read)*; these figures are per community. **I recommended keeping page-by-page reads in the last
   round, on a cost model these numbers refuted, so please disregard that.** I now have no firm recommendation. My lean is one page per
   community, flagged when cut, and refusing the top level *(judgement)*. Real community sizes below the top level are unknown.
4. **Should direct lookups skip re-checking each reply?** (Question 3) The old version re-checks. Forwarding is simpler, at the cost of a
   lost defence if the responding component sends a broken record. I recommend forwarding *(judgement)*.
5. **Should the answer-writer interface go?** (Question 6) You said on 24 October it stays, then dropped its twin in the community work. I
   recommend dropping it until language models arrive *(judgement)*.
6. **Should path search have hard limits?** (D, question 8) In the old version a caller can ask for two billion nodes *(measured)*. I
   recommend yes *(judgement)*.
7. **Three old decision records bind this work and are not in our tree:** (E) error classes over the wire, forwarding search scope
   untouched, and readiness depending only on community records *(measured)*. Port each, or say why not.
8. **One draft pull request per step?** (B)

Smaller recommendations on the overview's counts and the internal structure of search are in section 13.

**Questions worth a Fable session.** Each is one question over named evidence, and none has run.

- **Before step 1:** *What is the one reply format for query answers: which answers carry the wrapper, what does its revision number mean
  on a direct lookup, and what happens when a caller asks for a revision not reached yet?* Step 1 needs it.
- **Before step 3:** *Given that each page re-reads the whole community, how should a search reach a community's members?* This is ruling 3;
  it may change a ported decision.
- **Before step 3:** *While community records are being rewritten, what may one request see, and what must it refuse?*

If you run only two, I would pick the first two *(judgement)*.

**Also found:** the old version gives wrong answers quietly. A name shared by two entities comes back as "not found", and a large graph's
overview reports a partial count as complete *(measured)*. The port must fix both *(alias: your decision record; overview: the repository's
fail-closed rule)*.

---

# Inventory and proposed split: `processor/graph-query` (change 4, item 9)

base: `2bc62e5cf3ad0336825fdfa46bff13862a6e6c05` (`main`); claim commit `f5647d4`, draft PR #504. Pin `8b99efe9`.
Author: `semengine-architect` (Claude Opus 5.5). Status: inventory plus proposed split, **no target state, nothing decided**.
Round 3, after inventory reviews 1 and 2 (CHANGES REQUESTED against sha256 `86507c33`, `f72fcd35`); the review budget is spent. The contract's inventory stage normally stops
before options; the task asked for the split, ground-up questions and Fable candidates here, so sections 12-14 are framing for the owner.

Labels used below. **measured** = a command, probe or file read in this run. **read** = from reading code at the cited
line, not executed. **not measured** = judgement or a gap.

## 0. Findings that change the shape of this port

Each is evidence, not a ruling.

1. **The pin's community half cannot be carried as written.** It depends on `Community.Members`,
   `CommunitySummaryRecord`, `MembershipHash` and `SummaryKey`; the landed `graph/clustering` has none of them (measured,
   `graph/clustering/types.go:21-52`, owner ruling on PR #483). Sites: `community_cache.go:248,:312`,
   `summary_view.go:30-40,:284-300`, `graphrag.go:335,:357,:369,:1417,:1458,:1683,:1693,:1701,:1850,:2004,:2311,:2530`.
2. **A community is now a function of the ID** (ADR-099, ADR-102). The pin's `entityCommunity` reverse map has nothing to
   hold. Most `Members` sites become a prefix lookup or `MemberCount` (read). Only two enumerate members: `graphrag.go:335`
   (local) and `:1417` (text tier). `:2530` builds an entity-to-community lookup for sources, which a prefix test answers.
3. **Silent wrong answers at the pin, probed:**
   - An ambiguous alias on `entityByAlias` is swallowed; the raw input is then looked up as an ID and the caller sees
     "missing" (P5). The resolver returns `"", nil` (P5b). ADR-114 requires both to refuse.
   - `summary` on a 1,000-row page with predicates down returns no error, `total_entities:1000`,
     `entity_sample_truncated:false`, and no predicates field (P4): a truncated total is reported as complete, and
     "no predicates" and "graph-index down" are the same bytes.
   - `hierarchyStats` asks for 10,000, is clamped, and reports `totalEntities` from one page with no truncation field
     (read: `query.go:488`, `:533`; clamp `pin:graph/query_prefix_types.go:17`, `processor/graph-ingest/query.go:283`).
4. **#479 item 3's stated trigger is not reachable through `Start`.** `graphrag.go:476`'s `c.communityCache == nil` empty
   answer cannot fire: `Start` always builds the cache (`component.go:556`) before subscribing (read). The unresolved
   start node falls through to GraphRAG instead. The issue's symptom needs re-measuring on that path.
5. **The pin's entity validator refuses this tree's replies.** It reads `kvRevision`
   (`query.go:302`); this tree writes `kv_revision` (`graph/exact_entity.go:19`). Probe P6: "zero KV revision". Consumers
   decode `kvRevision` too (`semconnect gateway/cs-api/systems.go:808`, at `d0d06e00`).
6. **Tier 0 waits on later changes.** No responder for `graph.embedding.*` until change 5 or for
   `graph.spatial|temporal|clustering.*` until change 7; `COMMUNITY_INDEX`'s writer is change 7 (ledger row and README of
   `graph/clustering`). So until then, by reading `graphRAGTier2Fallback` (`graphrag.go:971-990`) and
   `searchgraph.go:68-115`, a free-text `globalSearch` or `searchGraph` answers `index_not_ready`. Not run end to end.

## 1. Pin probes (`/tmp/setup-04a-04-graph-query-stage/pin`, local only; tarball of `8b99efe9`)

| Run | Command | Result |
| --- | --- | --- |
| Race, one CPU | `go test -count=1 -race -cpu=1 ./processor/graph-query/` | ok, 6.98 s test, 16.9 s wall |
| Five shuffled, one CPU | `go test -count=5 -shuffle=on -cpu=1 ./processor/graph-query/` | ok, 27.2 s test, 32.1 s wall |
| Integration once (Docker 29.9.2) | `go test -tags integration -count=1 -timeout 6m ./processor/graph-query/` | ok, 10.7 s |
| Lifecycle suite | not run | same reason as `archive/2026-10-27-...-graph-index/inventory.md:43`: this tree's harness, the pin does not build against it. The pin's own `TestLifecycleOwner*` passed in the runs above |
| Test-text rules | see section 9 | 2 `time.Sleep`, 16 skips, build tag only `integration`, 12 failing own-deadline arms |

The race and repeat runs passed on an otherwise unloaded machine, once each. That is no evidence against load flakes.

Behaviour probes: source, log and replay command in `notes/probes/` (`pin-behaviour-probe.go.txt`, `.log`, `README.md`). Fakes are stated in the log:

| # | Input | Result |
| --- | --- | --- |
| P0 | `DefaultConfig` JSON | accepted back; `query_timeout` is `5000000000`; a second marshal adds `recheck_interval` (`ApplyDefaults`) |
| P1 | `"query_timeout":"5s"`, the form the schema and `doc.go:49` advertise | refused: `cannot unmarshal string into ... time.Duration` |
| P2 | unknown key `no_such_key` | accepted silently |
| P3 | `doc.go:44-46` per-operation port example | refused: `exactly one graph_queries input port is required, got 3`; `doc.go:44` alone: `input must be the required graph_queries port` |
| P4 | summary: 1,000-row prefix page, predicate verb refusing | no error; `total_entities:1000`; `entity_sample_truncated:false`; no `predicates` |
| P5 | `entityByAlias`, alias verb refuses `alias_ambiguous` | asks entity verb with `{"id":"sn-1"}`; the probe's fake answered not-found, while real graph-ingest refuses this non-ID as `invalid_request` (`processor/graph-ingest/query.go:79-82`, pin `:82`) |
| P5b | resolver, same refusal, through `resolvePartialEntityID` | `""`, nil error; this path also asks `graph.ingest.query.suffix`, which the fake refuses |
| P6 | `{"entity":{"id":"a"},"kv_revision":7}` through `validateEntityQueryResponse` | `zero KV revision` |
| P7 | `applyLimits(-5, 2000000000)` with `maxDepth` 10 | `-5, 2000000000`: only an over-large depth is capped |

Prefix verb cost, a temporary test on an archive of this tree at `2bc62e5` (`/tmp/setup-04a-04-graph-query-stage/tree`, local
only; source `processor/graph-ingest/zz_prefixcost_integration_test.go`; log `probe-prefixcost2.log`). It drives the real verb
through graph-ingest on Docker NATS: `Limit: 1000`, 2 KB bodies, 64 seeding workers, one client, one run, an otherwise idle
laptop. Only the first page is read before each first walk, so those walks are mostly cold reads. Total run 50 s. Bucket counts confirmed by `kv.Status`.

| Bucket | Group | One page | All pages |
| --- | --- | --- | --- |
| 100,000 | 1,000 | 7 ms | 3 pages, 29 ms |
| 100,000 | 10,000 | 19-22 ms | 23 pages, 0.84 s |
| 1,000,000 | 1,000 | 7-14 ms | 3 pages, 26 ms |
| 1,000,000 | 10,000 | 21 ms | 23 pages, 0.85 s |
| 1,000,000 | 100,000 | about 155 ms | 222 pages: 35.8 s (this run); 53.0 s, then 38.1 s (reviewer's run) |
| 100,000 | empty prefix | 164 ms | not walked |
| 1,000,000 | empty prefix | 1.43 s | not walked |

Bounded walks, the reviewer's re-run by build overlay, one run, 2 min 46 s (`notes/probes/prefix-cost-bounded-walks.log`). Same
bucket of 1,000,000, 2 KB bodies, first 23 pages, 10,396 entities:

| Group | First and last page | 23 pages |
| --- | --- | --- |
| 10,000 members | about 20 ms | 0.85 s (all of it) |
| 100,000 members | 157 ms, 172 ms | 3.86 s |
| `acme.ops`, the whole deployment, 1,000,000 members | 1.77 s, 2.70 s | 54.5 s |

What it shows: cost follows the size of the group, not the bucket. **A result limit does not bound the cost, because every page rescans
the whole group:** the same 23 pages cost 0.85 s, 3.86 s or 54.5 s by group size alone. A full walk is worse than linear. **A page at
2 KB per entity holds about 450 entities, not 1,000**, because of the 1 MB message limit, so `summary` and `hierarchyStats` totals fall
further short than P4 suggests. What it does not show: load, concurrent readers, real entity shapes. Spread between runs is wide: 35.8
to 53.0 s on one group.

Group sizes. Level 2 is known by definition: the whole deployment (ADR-102 note in `docs/adr/099-*.md:19`). A caller reaches it by
sending `level` (`pin:graphrag.go:162`, `:1380`). Text search walks up to `DefaultMaxCommunities` = 5 groups per request (`:29`); figures
above are per group. Sizes at levels 0 and 1 in real deployments: not measured.

## 2. Per file at the pin

Totals measured with `wc -l`: production 6,745, test 10,028, 16,773.

| Production file | Lines | What it does | Tests that belong to it |
| --- | --- | --- | --- |
| `graphrag.go` | 2,609 | local and global search, strategies, digests, labels, text scoring | `graphrag_test` 672, `graphrag_enrich_test` 434, `community_query_behavior_test` 564, `entity_digest_labels_test` 310, `slice_e_contract_test` 274, `classifier_defense_test` 62, part of `query_error_class_test` |
| `component.go` | 807 | config, lifecycle by hand, health, supervisors, LLM wiring, `Register` | `component_test` 1,458, `component_integration_test` 990, `attack_test` 571, `lifecycle_owner_test` 244, `test_owner_*` 305, `graph_query_lifecycle_test` 25 |
| `query.go` | 714 | 16-row operation table and most handlers | `query_error_class_test` 621, `operation_inventory_test` 200, `batch_passthrough_integration_test` 159 |
| `community_cache.go` | 431 | `COMMUNITY_INDEX` watch, generation, lease, member maps | `community_cache_test` 321, `community_generation_test` 217 |
| `pathrag.go` | 416 | breadth-first path search | `pathrag_test` 227, `TestComponent_PathSearch_*` in `component_test` |
| `summary_view.go` | 301 | `COMMUNITY_SUMMARIES` view (tier 2 store) | `summary_view_test` 558, `summary_view_lifecycle_integration_test` 158, `summary_bucket_late_attach_integration_test` 126, `community_summary_wire_integration_test` 180, part of `community_summary_join_test` 51 |
| `answer.go` | 299 | `AnswerSynthesizer`, LLM and template | `answer_test` 383 |
| `searchgraph.go` | 295 | `searchGraph`: global, then semantic fallback | `searchgraph_test` 146, search-graph tests in `community_query_behavior_test`, `entity_digest_labels_test` |
| `metrics.go` | 293 | 15 collectors, `sync.Once`, global fallback | none direct |
| `summary.go` | 233 | `summary` verb | `summary_test` 400 |
| `doc.go` | 171 | package doc | none |
| `entity_resolver.go` | 116 | partial-ID resolver | none direct; reached by `graphrag.go:422`, `:1016` |
| `router.go` | 60 | second spelling of downstream subjects | `router_test` 372 |

## 3. Reader measurement

Question: who reads each `graph.query.*` verb and each exported symbol? Repositories per `docs/inventory-scope.md`:
semstreams at the pin (code facts); semsource, semconnect; semboids (workload it drives); semsparql (its read adapter).
Semteams citations are struck: its row does not cover this question. **Open question for the owner:** the semsource and
semconnect rows list dependency closure, imports, the 03A baseline, the retained-contract-matrix column, the qualification corpus and,
for semconnect, identity, indexes, the typed mutation client and vocabulary seams. In my judgement none clearly covers wire-subject
literals. This inventory keeps them and does no further reads of that kind until the owner rules (section 15, F). semvocab: not relevant.

Verb matrix, measured by subject literal in non-test Go (file level, not call-site level, for the pin):

| Verb | Downstream | Pin production readers | Consumer production readers | Downstream in tree today |
| --- | --- | --- | --- | --- |
| `entity` | `graph.ingest.query.entity` | graph-gateway, fusionnats | semconnect `systems.go:80` | yes |
| `batch` | `graph.ingest.query.batch` | fusionnats, research-graph-execute | semconnect `systems.go:81` | yes |
| `prefix` | `graph.ingest.query.prefix` | graph-gateway, fusionnats | semsource `supersession/prefixquery.go:19` | yes |
| `entityByAlias` | alias, then entity | graph-gateway | none in production; semsource `subject_ownership_test.go:44` | yes |
| `relationships` | `outgoing` / `incoming` | graph-gateway, fusionnats, research-graph-execute | through fusion | yes |
| `byName` | `graph.index.query.byName` | fusionnats | through fusion | yes |
| `hierarchyStats` | prefix page | graph-gateway | none | yes |
| `pathSearch` | entity, outgoing, incoming | graph-gateway | none | yes |
| `summary` | prefix, predicateList | graph-gateway | none; semsource cut its tool (`mcp-gateway/component.go:118`) | yes |
| `searchGraph` | global, then semantic | graph-gateway, research-graph-classify, research-graph-execute | semsource `mcp-gateway/query_tools.go:107` | needs change 5, change 7 |
| `globalSearch` | classifier, strategies | graph-gateway | none (`searchGraph` delegates) | needs change 5, change 7 |
| `localSearch` | community, batch | graph-gateway | none | needs change 7 |
| `semantic` | `graph.embedding.query.search` | graph-gateway, fusionnats | through fusion | change 5 |
| `similar` | `graph.embedding.query.similar` | graph-gateway | none | change 5 |
| `spatial` | `graph.spatial.query.bounds` | graph-gateway | none | change 7 |
| `temporal` | `graph.temporal.query.range` | graph-gateway, research-graph-execute | none | change 7 |

- How the pin column was measured. `graph-gateway` builds 14 subjects at run time with `querySubject(c.queries.graph, "...")`
  (`pin:gateway/graph-gateway/component.go:431`, `:1108-1188`), not as literals; `batch` and `byName` it does not use
  (measured). `research-graph-execute` has no `entity` literal (`subquery.go:130` is a switch case): removed from the row.
  **Disputed from round 1:** `agentic/tools.go:376` and `agentic-tools/executors/graph_query.go:368,:370` mention verbs in a
  comment and an error string only, and `gated-dag/reader.go:64` requests `graph.ingest.query.prefix` directly, with a comment
  saying so (`:59-61`). None is a production reader of `graph.query.*` (measured), so none is added.
- No verb is dead. Every one has a pin consumer-adapter reader, so each is framework API under the owner's #190 ruling:
  it stays, and the design names its contract line. `graph-gateway` and `research-graph-*` are out of the tree
  (`docs/admission-ledger.yaml:1916` for the gateway).
- Consumer production use is 4 of 16 by literal (`entity`, `batch`, `prefix`, `searchGraph`), and 3 more through fusion
  (`semantic`, `byName`, `relationships`, change 5). semboids spells none; semsparql goes straight to `graph.ingest.query.*`.
- Consumer-owned verbs on the same family, which #106 must keep declarable: semsource `sources`, `sourceSummary`,
  `status`, `predicates`, `versionDiff` (at `e4febc0d`, reviewer round 1: `source-manifest/component.go:30,:42`,
  `status.go:22,:28`; `supersession/versiondiff_serve.go:21` from the local checkout, not re-checked).
- Semsource's one community path sends `{"query":…,"summarize_threshold":1}`, no level (`query_tools.go:103`, re-read at
  `e4febc0d`).

Exported top-level symbols: 50 at the pin (measured with the AST tool, `lanes3.py`; methods not counted. Round 1's 47
missed `ReasonAnswerSynthesis*`, `answer.go:93,:103,:109`). Readers outside the package, re-measured with `grep 'graphquery\.'`:

| Symbol | Reader | Class |
| --- | --- | --- |
| `Register` | `pin:componentregistry/register.go:178`; semconnect `cmd/cs-graph-backend/main.go:127` (on the branch of semconnect PR 74, head `dff1265`, not its `main`) | framework API |
| `CreateGraphQuery`, `Component` | semsource `internal/governance/live_graph_integration_test.go:535,:567,:575` (test) | test reader only outside the package |
| `DefaultConfig`, `DirectionOutgoing`, `PathSearchRequest`, `PathSearchResponse` | `pin:processor/graph-index/replacement_reconcile_integration_test.go` (test) | test reader only outside the package |
| `MinSemanticRelevance` | `pin:test/e2e/scenarios/research-graph/scenario_test.go` (test) | test reader only outside the package |
| the other 42 | no reader outside the package | see below |

No exported symbol is fully dead: each of the 42 has an in-package production reader (the three `Reason*` constants lose
theirs if the LLM path stays out and question 6 drops the fallback; read). Wire types among them
(`GlobalSearch*`, `LocalSearch*`, `PathSearch*`, `Source`, `Relationship`, `CommunitySummary`, `EntityDigest`,
`HierarchyChild`) are the shape consumers decode by hand, so they are candidates for a contract line. Plumbing
(`StaticRouter`, `NewStaticRouter`, `PathSearcher`, `NewPathSearcher`, `RelationshipEntry`, `SemanticHit`, `PathRAGResult`,
thresholds and score weights) has no case for being exported (optional under Bounded port scope).

A first run of this measurement listed false semconnect hits; a bash `grep -r` reaches hidden nested worktrees, and the
direct re-measure shows only `Register`. Treat the table above as the result.

Dead or advertised-absent surface (read unless a probe is named):

| Item | Cite |
| --- | --- |
| Six router entries no code routes to: `predicate`, `predicateList`, `predicateStats`, `predicateCompound`, `community`, `anomaly` | `router.go:16-43`; `Route(` callers measured |
| `IncludeSiblings` parsed, advertised, no reader | `pathrag.go:28`, `doc.go:64` |
| `PathEntity.Type` always `"entity"` | `pathrag.go:139,:184` |
| Five documented metrics do not exist under those names (`graph_query_duration_seconds` has no collector at all) | `doc.go:158-162`, `README.md:351-355`, `metrics.go:62-161` |
| Port example refused by the code | `doc.go:44-46`, P3 |
| Duration config advertised as `"5s"` refused | `doc.go:49`, `component.go:51`, P1 |
| Config schema lists 3 of 7 fields, twice | `component.go:327`, `:789` |
| `Dependencies: DepModelRegistry` once the provider paths leave | `component.go:784` |
| Suffix verb, removed by change 2 | `entity_resolver.go:15,:102` |
| Storage fallback to `graph.clustering.query.entity`, redundant once the prefix derives the group | `graphrag.go:2281` |

## 4. Open issues naming the package

Query: `gh issue list --state open --search <term>` for `graph-query`, `graphquery`, `graph.query`, `GraphRAG`, `PathRAG`,
`processor/graph-query`, `searchgraph`, `query.go`, `community`, `min_revision`; bodies and comments read. PR labels are
from section 12. All dispositions are proposals.

| Issue | What | Proposed disposition |
| --- | --- | --- |
| #405 | resolver and suffix | Retired by 9.5, with the resolver's first caller. The ADR-114 half for `entityByAlias` lands in 9.1 |
| #406 | lease in a context value | Retired by 9.3. Required: `AGENTS.md` §1, reviewer `BLOCKING` |
| #31 | agentic label predicates | Retired by 9.3 (`graphrag.go:1718-1720`), with its `go list -deps` test |
| #105 | relay and NL split | **Stays**: owner ruled it folds into #32 (comment 2026-10-24). Its one live item, `IncludePredicates` as `*bool` (`graph/query_summary_types.go:28`), rides 9.2. Question 5 asks whether the owner's new direction reopens it |
| #103 | one envelope, `min_revision`, generation marker | Needs a ruling (question 4). `min_revision` lands in 9.1 if ruled. Stays open while the generation marker (#15) is unanswered |
| #106 | responder-owned table, second subscriber refused at boot | Retired by 9.5, the last PR that adds verbs. Boot refusal lands in 9.1 using `composition.Analyze`. Accepted commitment: change 2 D20 |
| #479 | item 3, zone/area start node | Retired by 9.5, after re-measuring (section 0 item 4). Owner's comment 2026-10-30: 9.4 returns the classifier's invalid-input refusal to the caller |
| #111 | silent degrades | **Stays** for item 4 (spatial, change 7). Item 2 in 9.2; item 3 at `graphrag.go:841-850` in 9.4 and `:1117-1128` in 9.5 |
| #108 | graph-index rebuild | **Stays, names the word only** (`query.go`); not this package. ADR-114 points its cost here |
| #486, #487 | tier 2 classifier adapter, answer synthesis | **Stay**, tier 2, epic #484 |
| #32 | provider plumbing | **Stays**; exit condition is tier 1 admitting a provider or an owner ruling |
| #68 | `sync.Once`, global registry | Graph-query's lines (`metrics.go:50-51`, `:185`) go in the PR carrying the collectors; required by the `metric-registry` spec. Issue stays open unless this is its last site (not measured) |
| #195 | `natsfixture.Open` | 1 site here; stays for its other packages |
| #196 | lifecycle guard | `component.go:177` retires in 9.1; stays for other packages |
| #477 | graphview adoption | `community_cache.go:69` is a site; 9.3 if question 1 chooses the view. Stays |
| #16 | stream captures reserved subject | **Stays**, no code here: each verb added to `graph.QueryVerbs()` widens `composition/analyze.go:128` (adopter seam, section 11) |
| #15 | recovery across stream generations | **Stays**; bears on question 4 |
| #155 | `model` reader | **Stays**; the provider paths stay excluded, so there is still no reader |
| #485 | LLM community summaries | **Stays**; takes `summary_view.go` and its tests |
| #484, #9, #500 | epics and rules | #9 addressed by every PR; #500's cap and rules govern, section 9 |

Names the word, not the package: #109, #110, #141, #300, #350, #24, #7, #11.

## 5. Binding input

- **ADR-114 Consequences** (`docs/adr/114-*.md:110-113`): stop swallowing; propagate `alias_ambiguous`, `index_not_ready`,
  `internal`; not found keeps falling through. Sites `query.go:156-177` and `entity_resolver.go:48,:62-65`, confirmed by
  P5/P5b. A detail the design must settle: `alias_ambiguous` is class invalid (`processor/graph-index/query.go`, read).
- **Reader consistency contract** (held): `archive/2026-10-31-...-clustering/design.md:130-132`, `proposal.md:46-47`,
  `graph/clustering/README.md:126-127`, #9 comment 6547667079: "which committed generation of `COMMUNITY_INDEX` a reader
  sees during a tick". Facts: `Tick` writes record by record, then removes; no tick marker exists (read,
  `graph/clustering/maintainer.go:183-240`; spec "Removal follows publication").
- **Prefix-page rescans** (held): same design, `graph/query_prefix_types.go:30-33` ("fetches ALL keys ... on every
  request"). Measured in section 1: the scan is of the keys under the prefix, not of the whole bucket.
- **ADR-099 on member enumeration** (`docs/adr/099-*.md:57-58`): members are never stored and enumeration "rides the existing
  paginated prefix-query lane". This is the ported decision for question 2.
- **No LLM provider code** (#9 comment 6386603659): excludes `LLMAnswerSynthesizer` (`answer.go:122-244`) and
  `initLLMClassifier`, `initAnswerSynthesizer` and their fields (`component.go:419-480`). That comment also says the
  `AnswerSynthesizer` interface stays; the later clustering ruling dropped the matching summarizer interface
  (#9 comment 6547667079). See question 6.
- **#103 / D18** (`archive/2026-10-12-setup-04a-02-ingest-kernel` design): every `graph.query.*` reply is one envelope;
  graph-query wraps graph-ingest's replies once. **ADR-109 Decision 4:** an unmet `min_revision` is ordinary catch-up,
  never `index_not_ready`. **Ledger:** `MinRevisionField` returns with these request types.
- **#106 / D20:** the table is extended, a second subscriber refused at boot, consumer verbs stay declarable. The table lost
  its request and reply types (`graph/query_verbs.go:5-11`); D18's "declared reply type" now lives only in a comment.

## 6. Governing pin ADRs

Searches: every file in `pin:docs/adr` (85) for `graph-query|graphquery|graph\.query|PathSearcher|StaticRouter|globalSearch|
localSearch|searchGraph|PathRAG|GraphRAG|AnswerSynthesi|communityCache|community cache|COMMUNITY_SUMMARIES|COMMUNITY_INDEX`;
then titles read. Local ADRs present: 030 058 063 066 074 077 078 081 083 084 085 088 094 095 096 098 099 100 102 104 107
108 109 110 111 112 113 114.

| ADR | Decides, for this package | Status here | For the owner |
| --- | --- | --- | --- |
| 060 RPC error contract | classified errors over the wire; `Degraded`+`DegradedReason` pattern | **not in `docs/adr`**; `natsclient` already implements it | binding, not ported: say why or port it |
| 062 Deterministic fusion | fusion reads `byName`, `batch`, `relationships`, `searchGraph` | not here, belongs to change 5 | keeps those verbs |
| 071 Domain-scoped retrieval | `semantic` forwards raw bytes so `Scope` rides through (`:84-85`) | **binding here, not ported; no port-or-why yet** (question E) | a relay that decodes must not drop `scope` |
| 081 Graph view | names `community_cache.go:49` as a view site | ported | question 1 follows it, or changes nothing |
| 085 Gate on health | cache keyed by `(level, ID)` because a late delete collapsed level 0 (`pin:docs/adr/085-*.md:133-137`; local `:159-164`) | ported | keep the observable: an ID at two levels never collides |
| 087 Summary store | `{level}.{membership_hash}` store; **Decision 5:** readiness on `COMMUNITY_INDEX` only | not ported | **drop** the store (with #485); Decision 5, readiness on `COMMUNITY_INDEX` only, is **kept and binding**: port-or-why owed (question E) |
| 099 Prefix partition | base partition from ID; membership "**never stored**", "member enumeration rides the existing paginated prefix-query lane" (`docs/adr/099-*.md:57-58`); "GraphRAG and PathRAG advertised surfaces hold shape-for-shape" (`pin:...:44`, local `:73`) | ported, with PR #483's note | **Binding on question 2.** An answer other than the prefix lane changes this decision |
| 084, 066, 083 | `index_not_ready` means not built or broken | ported | keep |
| 109 | catch-up per request | local | keep; question 4 |
| 108, 113 | shell and run context | local | apply; not owner questions |
| 065 | predicate list reply used by `summary.go` | not ported | no decision on this package; reader only |
| 086, 061 | community semantic edges removed | not ported | no live decision here |
| 059, 055, 054, 046, 043, 036, 034, 037, 073, 105 | one-line mentions | not governing | none |

Guidance owed with the package (contract, "Guidance returns"): the pin's `query-pattern` skill is in this tree without
GraphRAG/PathRAG (`.agents/skills/query-pattern/SKILL.md:50-52`) and must grow when verbs land. At the pin, `.agents`, `.claude` and
`.codex` name this package only in that skill (measured: `grep -rlE 'graph-query|graphquery|GraphRAG|PathRAG'` hits only
`.agents/skills/query-pattern/SKILL.md`); sections on NATS RPC and graph in general were not compared.

## 7. Ledger and what already landed

| Ledger line | Row | What it says for this port |
| --- | --- | --- |
| `:191` | `processor/graph-query/lifecycle_owner_test.go` @ `5457b345` | Shapes went to `internal/harness/probe`. Stays as is; no new carry (`internal/harness/probe/probe.go:4`) |
| `:1905-1912` | `vocabulary/agentic` excluded | Drop three predicates (#31) |
| `:1916` | `gateway/graph-gateway` | Excluded; a consumer adapter, its verb use is framework API |
| `:2090` | `graph` | `MinRevisionField` and generation marker still deferred to this item |
| `:2555` | `graph/llm` | Item 9 is no longer a reader: excluded |
| `:3783` | `graph/query` | Item 9 calls it. Signature is `(*ClassificationResult, error)` (`graph/query/classifier_chain.go:90`); the pin's `graphrag.go:497` needs the error |
| `:3876` | `pkg/graphview` | Names `summary_view.go` as this package's reader; that file leaves, so the row needs updating or question 1 supplies a new reader |
| `:3959` | `graph/clustering` | Item 9 decodes `Community`; `communityKey` is unexported (`storage.go:108`) |

A new row is owed: `processor/graph-query` (none today, measured), `adapt`. Plus `defer-exclude` rows for
`answer.go`'s LLM half, `component.go`'s wiring, `summary_view.go`, each naming #487/#486/#485 (the owner's current tier
tracker) and #32.

| Landed surface | Cite | Pin's version | Consequence |
| --- | --- | --- | --- |
| Processor shell | `component/lifecycle_type.go`; `processor/graph-index/component.go:43` | hand-rolled, `component.go:172-186` | replaces the lifecycle block |
| Verb table | `graph/query_verbs.go:29`, guard `internal/harness/contract/reservedsubjects_test.go:23,:32` | `query.go:49-66`, `router.go` | any literal fails verify |
| Envelope | `graph.NewQueryResponse(producer, revision, data)` `graph/query_response.go:28` | one-argument `NewQueryResponse(summary)` `summary.go:112` | needs a producer and revision |
| Classified RPC | `natsclient/errors.go:231`, `natsclient/request.go:330` | same | keep |
| `graph/query` | `classifier_chain.go:90`; hints typed | no error return | adapt |
| `pkg/graphview` | `Run`, `Get`, `List`, `WaitCaughtUp`, `Applied`: `view.go:216,:403,:425,:326,:392` | `Start`/`Stop` | `summary_view.go`'s shape is obsolete |
| `graph/clustering` | bounded `Community`, `GetAllCommunities`, Maintainer | members, hash | section 0 |
| Catalog reader | `kvcatalog.OpenCatalogReader` `graph/kvcatalog/kvcatalog.go:296`, takes `*natsclient.Client` | `graph.OpenCatalogReader` | moved |
| Stored-value rule | `graph.UnmarshalEntityState(key, data, &entity)` `graph/entity_predicate_contract.go:288` | two arguments, `pin:graph/entity_predicate_contract.go:255`; `query.go:314` | adapt; the key comes from the request |
| Exact reader | `graph.NewExactEntityReader` returns an error `graph/exact_entity.go:46` | no error | adapt `pathrag.go:82` |
| Alias refusal | `graph.ErrorCodeAliasAmbiguous` `graph/mutation_responses.go:104` | absent | read it |
| Alias reply | `graph.AliasData` in `QueryResponse` | `graph.AliasQueryResponse` | absent here; adapt |
| Relationship replies | `graph.QueryResponse` | `graph.Incoming/OutgoingQueryResponse` | absent here; adapt |
| Consumer policy | no JetStream consumer in this package (measured: no `CreateConsumer` read in pin files) | n/a | entry points not needed |
| Delivery settlement | request/reply only | n/a | not needed |

## 8. Shape sweep, read on the pin copy

Required means tied to a named obligation (Bounded port scope). Optional findings force nothing.

| # | Pin `path:line` — text | Shape | Disposition and obligation |
| --- | --- | --- | --- |
| S1 | `graphrag.go:72-88` — `context.WithValue(ctx, communityRequestLeaseKey{}, state)` | no ctx values | **Required**: §1 row, #406 |
| S2 | `component.go:505` — `componentCtx, cancel := context.WithCancel(ctx)` | run context from Start's ctx | **Required**: ADR-113, run context ends only in `Stop` |
| S3 | no `context.Context` struct field; no `Background`/`TODO`/`WithoutCancel` in production (measured grep) | retained context | clean |
| S4 | `component.go:172-186` — `lifecycleUsed bool` and eleven siblings | lifecycle by hand | **Required**: §1 row, #196, ADR-108 |
| S5 | `component.go:150-169`, `:202`; `community_cache.go:27-29` — 17 func fields, e.g. `searchGraphBeforeFinalize func()`; 15 exist only for tests, the two `open*Reader` are also set in production (`component.go:279-284`) | back door | **Required**: §4 row (#186, #190) |
| S6 | `component.go:226-227` — `json.Unmarshal(rawConfig, &config)` | unknown keys accepted (P2) | **Required**: §1 row; shape at `processor/graph-index/component.go:107` |
| S7 | `metrics.go:51`, `:185` — `metricsOnce sync.Once`, `prometheus.DefaultRegisterer.Register` | metrics registry | **Required**: `metric-registry` spec, #68 |
| S8 | `metrics.go:168` — `_ = registry.RegisterCounter(...)` | ignored registration | **Required**: `RegisterOrGet` row |
| S9 | `router.go:16-43`; `pathrag.go:261,:288`; `summary.go:100`; `entity_resolver.go:102`; `graphrag.go:1617,:2281` | second spelling of subjects | **Required** where reserved (`graph-transport-boundary`, verify). `embedding`, `clustering`, `spatial`, `temporal` subjects are not yet in the table: **unknown** whether 9.2 adds them, or change 5/7 |
| S10 | `query.go:166-177`; `entity_resolver.go:62-65` — swallow | classified handlers | **Required**: ADR-114, P5 |
| S11 | `summary.go:102`, `:140`, `:230` — predicate failure and malformed request swallowed | fail closed | **Required**: §1 row, #111 item 2, P4 |
| S12 | `query.go:489,:536`; `summary.go:64-92`; | truncation undeclared | **Required**: §1 "any skip declared", P4 |
| S13 | `graphrag.go:841-850`, `:1117-1128` — fallback on Debug | undeclared degrade | **Required**: #111 item 3 |
| S14 | `graphrag.go:959`, `:929`; `searchgraph.go:102-108` — lower tier taken on any semantic error, Debug/Warn | undeclared degrade | **Required**: §1 "Failure paths fail closed; any skip, drop, or degrade is declared", where a caller cannot tell; `Degraded` exists. Exact scope for the design |
| S15 | `graphrag.go:2278-2281`, `:2264` — community lookup swallows every failure | fail closed | Goes if question 1 removes it |
| S16 | `graphrag.go:1718-1720` — `agvocab.IdentityDisplayName` | closure | **Required**: #31 |
| S17 | `query.go:302` — `json:"kvRevision"` | second spelling | **Required**: build, P6 |
| S18 | `pathrag.go:195-205`, P7; `pathrag.go:100-105` — request `max_nodes`, negative depth, request `timeout` not bounded | input bounds | **Required** for entity IDs and predicates (§2 validators, `pathrag.go:91` validates nothing); the numeric bounds have a real trigger (a caller's request) but no named rule: **owner or design call** |
| S19 | `query.go:299-345` — relay decodes every reply to validate it, then returns the bytes | double work | Optional; question 3 |
| S20 | `graphrag.go:796`, `searchgraph.go:220` — strategies return `[]byte`, re-decoded to strip or test emptiness | shape | Optional; question 5 |
| S21 | `component.go:34` — `natsRequester`, 7 methods, one production implementer | layered interface | Optional. `Connect`, `Request`, `JetStream` have no production call on this field (measured: `grep` for `c.natsClient.` and `p.nats.`); narrowing is cheap |
| S22 | `answer.go:69` — `AnswerSynthesizer`, one tier-0 implementer | layered interface | Owner's comment keeps it; question 6 |
| S23 | `component.go:327`, `:789` — two config schemas | second spelling | Optional, but a field that stays needs a test (Surface audit) |
| S24 | `component.go:51`, `:53` — `time.Duration` in JSON (P1) | advertised absent | **Required** by Surface audit (c): fix the doc or the type; graph-index used integers (`config.go:26-28`) |
| S25 | `component.go:188-198`, `:750-767` — `recordError`, `metricsMu`, `healthMu` counters | copied shell | Replaced with S4 |
| S26 | `query.go:160,:182`; `pathrag.go:254,:285`; `graphrag.go:467,:485`; `searchgraph.go:81` — `_ := json.Marshal` | ignored error on known-good shape | Optional |
| S27 | no `strings.Contains(err...)` in production (measured grep) | classification by text | clean |
| S28 | `graphrag.go:120-123` — `errs.ClassifiedCode(errs.ErrorTransient, gtypes.ErrorCodeIndexNotReady, ...)` | classified handlers | clean; keep |

Rule rows with nothing to carry: **BaseMessage** (no payload published; no `BaseMessage`, `RegisterPayloads` or
`NewGenericJSON` in production files; replies are plain JSON on request/reply); **OpenAPI** (not a service; nothing in
`cmd/openapi-generator` or `schemas` names the package). Whether query replies should be registered payloads is not
something any rule asks; noted only.

## 9. Tests under #501's rules

Read from `origin/claude/flake-gates-500` at `decef66` (draft; not merged, the rule can still move):
`openspec/changes/flake-gates-own-deadline/{proposal,design}.md`, `Taskfile.yml:96-109`,
`internal/harness/contract/testtext_test.go`. Rule: no `select` arm `case <-time.After(...)` or `case <-x.C:` whose body
fails, unless `// Own deadline: <reason>` is on the line above; `-timeout 5m` on `test:unit` and `test:repeat`.

The `time.After` count of 12 is confirmed, but one is a pass arm; one more failing arm uses a ticker:

| Site | Body | Under the guard |
| --- | --- | --- |
| `answer_test.go:365` | mock delay, does not fail | allowed |
| `community_generation_test.go:167` | `t.Fatal` | refused |
| `summary_view_test.go:546` | `t.Fatal` | refused; file not carried |
| `component_integration_test.go:97,:109` | `t.Error` | refused |
| `:124,:165,:171,:183,:441,:517,:531` | `t.Fatal` | refused |
| `:150` (`case <-observationDeadline.C:`) | `t.Fatal` | refused |

**12 failing arms**: 11 `time.After`, 1 ticker. Hand-read; #501's guard itself was not run against the pin copy.

- Other refused text: `time.Sleep` at `component_test.go:792`, `test_owner_proof_test.go:135`; 16 `t.Skip` in integration
  tests (`component_integration_test.go:32` and so on, 5 files), refused by the no-skip rule.
- Not gated but review work (#501 D4, #502): `context.WithTimeout` in tests, 58 sites (`component_test` 24, `attack_test`
  14, `component_integration_test` 9, others). Most are outer bounds; two are the 100 ms deadlines at `component_test.go:836`,
  `:971`. #501's D2 hazard applies.
- Five-minute budget: the five repeats took 27 s for this package at the pin (measured, unloaded). Comfortable.
- Mocks: `component_test.go:28-44` implements the seven-method `natsRequester`; narrowing S21 touches every test using it.

| Test file | Lines | Recommendation | Why |
| --- | --- | --- | --- |
| `pathrag_test` | 227 | carry | behaviour; classification branches |
| `summary_test` | 400 | carry and change | pins the swallow at `TestHandleQueryGraphSummary_PredicateFailure_StillReturnsEntityFacet` that #111 reverses |
| `query_error_class_test` | 621 | carry and adapt | the classification tests worth having |
| `component_test`, `TestComponent_PathSearch_*` | ~490 | carry and adapt | real traversal behaviour; the `:792` sleep goes |
| `graphrag_test` pure functions | 672 | carry | scoring, filter, cap are plain functions |
| `searchgraph_test`, `classifier_defense_test`, `slice_e_contract_test` | 482 | carry; rename the last | behaviour by strategy. Name `slice_e` means nothing here |
| `batch_passthrough_integration_test` | 159 | carry, remove skips, `natsfixture.Open` | real wire |
| `entity_digest_labels_test` | 310 | carry and adapt | check for hook use on adapting |
| `operation_inventory_test` | 200 | rewrite | asserts a table that becomes `graph.QueryVerbs()` |
| `component_test`, config and lifecycle part | ~520 | rewrite small | the shell and lifecycle suite already prove `Start`/`Stop` |
| `router_test` | 372 | drop or rewrite | nine tests of a map, and the map is S9 |
| `attack_test` | 571 | rewrite the few that assert behaviour | header reads "generated by Reviewer Agent"; own 14 deadlines; duplicates others |
| `component_integration_test` | 990 | rewrite from behaviour | 10 of 12 failing arms and 11 skips |
| `community_cache_test`, `community_generation_test` | 538 | rewrite from behaviour | built on `Members` and the back doors; keep the cross-level collision cases as behaviour (ADR-085) |
| `community_query_behavior_test` | 564 | rewrite from behaviour | built on `*BeforeFinalize` hooks (S5) |
| `graphrag_enrich_test` | 434 | rewrite | built on `Members`, PageRank-order reps |
| `lifecycle_owner_test`, `test_owner_*`, `graph_query_lifecycle_test` | 574 | **do not carry** | shapes live in `internal/harness/probe`; the lifecycle moves to the shell |
| `summary_view_*`, `summary_bucket_*`, `community_summary_wire_*` | 1,022 | **do not carry** (#485) | tier 2 store |
| `answer_test`, LLM half | ~300 | **do not carry** (#487) | provider path |
| `community_summary_join_test` | 51 | rewrite 1 test | readiness on `COMMUNITY_INDEX` only (ADR-087 Decision 5) |

Unknown: `test:repeat` also excludes `contract`; whether a graph-query guard belongs there is for the design.

## 10. Overlapping open PRs (`gh pr list --state open`)

| PR | State | Overlap | Merge order |
| --- | --- | --- | --- |
| #504 | draft, 0 files | the claim | hosts the first of this package's PRs, or the owner directs one draft per PR |
| #501 | draft, 88 files at `40ee4cd` (measured) | none in this package; touches `graph/clustering/maintainer_test.go`, `pkg/graphview/integration_test.go`, `processor/graph-index/*`, `processor/graph-ingest/*`, `Taskfile.yml`, role contracts | **before** any test code here (#504 body) |
| second #500 PR | not opened | #159 row, the cap, `testtext_test.go`, role contracts; `AGENTS.md` still reads "non-test" until then | after #501; ruling #500 comment 6580240783 already binds |

No other open PR exists (measured: only #504 and #501). A word search for `class:flake` returned #270; its labels are
`bug` only (measured), so it is not a known flake under the merge gate.

## 11. Adopter seam, and collisions

Surfaces reached from outside: the `graph.query.*` wire; `Register`; the config.

**1. What must they know?** That `graph.query.*` is framework-owned, with named consumer verbs allowed beside it; that a
stream on `graph.>` is refused at boot (#16); that some verbs need change 5/7 components; that an answer may be degraded
(`degraded_reason`); that `kv_revision` replaced `kvRevision`. That is more than two facts: a finding by the contract's measure.

**2. If they do nothing?** Traced on the pin: an alias collision reads as "not found" or, on a non-ID, a malformed request (P5); a big graph's summary reads as
complete (P4); a missing index reads as an empty facet (P4); unknown config keys are ignored (P2). Semconnect's
`kvRevision` decoder sees revision 0 unless its decode refuses it (**not measured**). If D18's envelope wraps
`entity`/`batch`, the same decoder sees an empty entity (read; `systems.go:808`).

**3. Where do they find out?** Unknown key, port shape, stream capture, second subscriber: boot. Refusals: typed runtime.
Degraded: a response field. Truncation at the pin: **nowhere**.

**4. What should they have to know?** Which verb to call, and to read a declared flag. **Prefer observation:** the caller
should not predict readiness or tier; it sends the request and reads the refusal or the degraded flag. It should not compare
revisions by polling (#103).

Collision table for a community read view (the only new coordination primitive in sight):

| Dimension | Evidence |
| --- | --- |
| Semantic class | one reader's view of published group records |
| Owners | `graph/clustering` writes; pin `communityCache`; `pkg/graphview.View`; `graph/follower` (`ENTITY_STATES`, different job); `NATSCommunityStorage.GetAllCommunities` (one-shot) |
| Catalogs | `graph/kvcatalog/kvcatalog.go:188`, owner `graph-clustering` |
| Status | `index_not_ready` per request; `GRAPH_STATUS` not gated (ADR-109, ADR-085) |
| Lifecycle | pin: supervisor restarts on loss (`component.go:710`); `View`: one `Run`, replacement is a new view (ledger `:3876`) |
| Ownership | single writer, no claim (ADR-099) |
| Readers | graph-query only in tree; change 7's anomaly reads via `GetAllCommunities` |
| Writers | `Maintainer`, change 7 wires it |
| Recovery | rewatch; `Maintainer` republishes from entity changes |

Problem shape: read-through view, fail closed on loss. Closest instance: `pkg/graphview` (ADR-081). Verb-table generalising:
closest is `processor/graph-index/query.go:36-53` and `composition/analyze.go:141`.

**Intent check** (the split and question 2 touch a boundary):

| Capability | State | Ruling |
| --- | --- | --- |
| Ingest, index | admitted, landed | #8, #9 |
| Query (entity, alias, prefix, relationships, path, summary) | admitted | #9 comment 6386856657 |
| Community-aware search, tier 0 | admitted | #9; ADR-099; PR #483 rulings |
| Answer synthesis by LLM; LLM summaries; LLM classifier adapter | deferred | #9 comment 6386603659; #485-#487 |
| Member-list communities, label propagation | removed | PR #483 rulings (ADR-099) |
| Vocabulary, provenance | admitted; not touched | #8 |
| Fusion | change 5 | #9 D2 |
| Tier ladder | admitted | `AGENTS.md`, #8 Q14 |
| Durable execution | unrelated | #24 |

Adoption sweep: a new primitive may result from question 1 or 3; listed there if it does.

## 12. Proposed split

**Reproducer:** `python3 -I /tmp/setup-04a-04-graph-query-stage/lanes3.py` (local only), reading `decls.tsv` from
`tools/lanes/main.go` (a parser over the pin's package-level declarations, doc comments and leading gaps included). It prints
this table exactly and checks each lane's **production** references by name. Tests are assigned by function name and are not closure-checked. Round 1's `lanes2.py` is superseded; its hand moves are now in the
script.

| PR | Delivers | Production | Test | Total | Depends on |
| --- | --- | --- | --- | --- | --- |
| **9.1** Shell, verb table, authority relays | Component on `component.Lifecycle`, strict config, `graph.query.*` table, second subscriber refused at boot, `entity`, `entityByAlias` (ADR-114), `batch`, `prefix`, `relationships`, `byName`; reply shape and `min_revision` as ruled | 1,385 | 2,440 | **3,825** | #501, second #500 PR; F2 |
| **9.2** Composed reads | `pathSearch`, `hierarchyStats`, `summary` with declared truncation and predicates state; relays `semantic`, `similar`, `spatial`, `temporal` | 917 | 1,643 | **2,560** | 9.1 |
| **9.3** Community reader, local search | Community read with the consistency contract, `localSearch`, and what it needs: entity loading, semantic call, text scoring, labels, template answer, collector constructor; #406, #31 | 1,647 | 1,599 | **3,246** | 9.1; F1; question 2 |
| **9.4** Global search | Classifier call with refusal, `globalSearch` coordinator, semantic-first, text tier, digests and enrichment, sources | 1,357 | 1,580 | **2,937** | 9.3 |
| **9.5** Structured strategies, `searchGraph` | Entity lookup, path intent, resolver (#405), #479 item 3, temporal, spatial, semantic strategies, `searchGraph` | 921 | 839 | **1,760** | 9.4 and 9.2 (`PathSearcher`, `querySemantic`) |
| Not carried | LLM half of `answer.go`, LLM wiring, `summary_view.go`, their tests, owner-proof tests | 518 | 1,927 | 2,445 | #32, #485-#487 |

Sum 6,745 + 10,028 (measured). **Round 1's 9.3 did not compile on its own lines; the reviewer was right.** `handleLocalSearch`
needs `loadEntities`, `searchEntitiesSemantic`, `filterEntitiesByQuery`, `synthesizeQueryAnswer`, `resolveCommunitySummary`,
`CommunitySummary`, `EntityDigest` and metric recorders. They now sit in 9.3, and the digest and enrichment code that only global
search reads (`:1665-1712`, `:1780-1885`, `:1887-1913`, `:1934-1951`, `:1971-2079`, `:2125-2159`) moved to 9.4.

The closure check reports 36 name matches pointing to a later lane or to the excluded row. All are accounted for:

| Count | What | Why it is not a cycle |
| --- | --- | --- |
| 26 | `Component`, `Config`, `CreateGraphQuery`, `Start`, `cleanup`, `graphQueryOperations` naming later handlers, fields and thresholds | wiring: each PR adds its own field and table row. Hand work, declared; lines stay where the declaration is |
| 3 | `Start` calling `initLLMClassifier`, `initAnswerSynthesizer`, `superviseSummaryView` | removed by the exclusion |
| 1 | `resolveCommunitySummary` calling `summaryFor` | removed: statistical summary only |
| 5 | `handleGlobalSearchWithLease` switch naming 9.5's strategies | stepping stone, awkward point 4 |
| 1 | `filterEntitiesByQuery` | false match: a parameter named `minScore` |

Not captured by the numbers: the metric constructor is charged whole to 9.3 (149 lines), though 9.3 reads about a third of the
collectors. Registering each with its reader moves about 90 lines to 9.4 (hand estimate). New tests the rulings require (ADR-114
refusals, unknown config keys, second subscriber, poison declaration) are **not counted anywhere above**, in any PR. Nor is this rewrite:
carried and adapted tests call the not-carried `newGraphQueryTestOwner` 47 times (`component_test` 22, `attack_test` 14,
`component_integration_test` 9, `batch_passthrough_integration_test` 2; measured), so each site changes.

**Size against the limit.** #500 comment 6580240783 counts "production and test Go lines together", and its reason is the size of
a review stop. Whether that means pin lines assigned, or what a PR carries and writes, is **owner question A**. Until then neither
reading is assumed. 9.1 and 9.3 are the two affected:

| PR | As assigned | Alternatives |
| --- | --- | --- |
| 9.1 | 3,825, over | (a) Split: **9.1a** shell, table, boot refusal (987 + 1,528 = 2,515); **9.1b** relays (398 + 912 = 1,310). 9.1a answers no request: a named stepping stone. Scripted, approximate at the test edge. (b) If question A counts what the PR carries: dropping `router.go`, `router_test` (432) and `attack_test`'s 9.1 share (436) gives 2,957, with no margin; section 9 keeps a few attack cases, and new required tests go on top. So (b) probably ends over 3,000 (judgement) |
| 9.3 | 3,246, over | (a) Move collectors: about 3,155, still over. (b) Split: **9.3a** community read and its supervisor, about 1,425 (hand sum), no verb of its own: a stepping stone; **9.3b** local search and shared retrieval, about 1,820. (c) Wait for F1 and question 1: rewriting the cache may shrink this (not measured) |

**Each PR alone.** 9.1 is the first thing to answer on `graph.query.*`. Semconnect calls `entity` and `batch`, but its decoder reads
`kvRevision` and this tree writes `kv_revision` (`systems.go:808`), so it does not decode that reply without a change on one side
(read). 9.2 finishes the verbs that need no later change, except the plain relays. 9.3 answers `localSearch` once change 7 writes
the bucket; until then, `index_not_ready`, testable with a real bucket. 9.4 answers `globalSearch`. 9.5 finishes the package. A
package split this way is whole when its last PR lands (contract, "One package per pull request"). Each PR declares only the verbs
it serves.

**Merge order:** #501, second #500 PR, 9.1, 9.2, 9.3, 9.4, 9.5. 9.2 and 9.3 can run in parallel after 9.1, but both edit
`graph/query_verbs.go` and `component.go`; the later one merges `origin/main`.

**Where the boundaries are awkward, plainly:**

1. **9.1 and 9.3 are over** as assigned; above.
2. **9.3 carries shared retrieval it did not obviously own.** It is the first reader. Question 2 may change what it needs.
3. **`searchGraph` comes last, and that is sound.** It is semsource's one search call (measured), but by section 0 item 6 it needs
   changes 5 and 7 to answer anything but a refusal, and no consumer cutover was found before them (reviewer, round 1; `AGENTS.md`:
   consumer need decides order). Moving it into 9.4 makes 9.4 about 3,730 (scripted lines plus `searchgraph.go` 295 and its tests,
   hand sum). An alternative, 9.4 = coordinator, semantic-first, `searchGraph` and 9.5 = text tier and strategies, gives no answer
   without embedding until 9.5. Semsource at tier 0 composes embedding: archived foundation design (`setup-04a-foundation`
   `design.md:78`, "unconditionally") and `tier0-statistical.json:8` (bm25; reviewer, round 1); no other composition was checked.
4. **9.4 interim:** `resolveStrategy` can name a strategy 9.5 has not delivered. The stepping stone falls those to `graphrag`
   and says so; it must be declared, since a question with a time range would otherwise lose its filter silently. `tryPathIntentSearch`
   is also called directly (`graphrag.go:776`, the `pathrag` case) and is absent in 9.4, so path questions change behaviour too and
   must be declared the same way.
5. **Relays in 9.2 have no backend in the tree** until changes 5 and 7. They are proved against a test responder. Whether
   a "no responders" reply classifies as stated for queries was **not probed** (`docs/contract.md` speaks of writes).
6. **#111 spans three PRs and change 7.** It stays open throughout. **#106** closes last, its boot refusal first.

## 13. Ground-up questions

Not settled here. Each states the observable behaviour, options, a recommendation, and labels.

### Q1. How does a request read communities?

**Behaviour at stake:** a `localSearch` for entity E answers from E's group at the asked level; with no usable view, it refuses
`index_not_ready`; a level-1 group never answers for a level-0 request; a view loss never serves stale silently.

| Option | Trade-offs |
| --- | --- |
| (a) Adapt the pin: generation, lease, maps, supervisor | Most lessons kept; 431 lines + lease + hooks to prove it (measured), and #406 still to redo. The reverse map is now pointless (read) |
| (b) `graphview.View[clustering.Community]`; key from level + ID prefix | `Get`/`List` exist (measured); ADR-081 names this site; gives `pkg/graphview` the reader its row has lost (measured). Needs exported key helper from `clustering` (measured: `communityKey` unexported). Loss: `Run` returns, a supervisor restarts with a new view (read). **Poison:** `Get` returns `*PoisonError`; `List` excludes poisoned keys with no report (`pkg/graphview/view.go:397-402`, `:421-424`), and a `Hooks.OnPoison` callback exists (`:64-65`). `List` also copies every match under the view's mutex, per request (`:425-441`). The pin's cache also skips a bad record with a Warn (`community_cache.go:214-219`) |
| (c) No view: `Get` per request, per group | No watcher at all (read). A round trip per group; a text tier reading every group at a level is a full listing per query (read; cost not measured). Nothing to go stale |

**Recommendation:** (b), with `Get` for anything keyed by entity, and enumeration held until F1 rules. Clauses: the reverse map is
removable (read); it is less code (not measured until written); the lease exists only to detect a generation swap during a
request, and is the part to re-justify under the contract below (judgement).

**The held contract within it.** Which state may a reader observe during a tick? Facts: writes are per key, then removals; old
records stay readable on failure (spec, measured); no tick marker (read). Sub-options: **(i)** latest per key, no tick atomicity,
declared, with `Applied()` revision on the reply; **(ii)** per-request snapshot (`List`) from one view, so one request is
self-consistent but may span a tick; **(iii)** a published tick marker, so readers hold the last complete tick: new writer
surface in a merged package. **(ii) as it stands is the undeclared-skip path:** `List` drops a poisoned record silently, against `AGENTS.md` §1 ("any skip, drop,
or degrade is declared"). Round 1 recommended it without having read this. Three ways to declare it, each with a cost: a poison count from `List` (a
change to a merged package); `SnapshotAndSubscribe`, which already returns `Entries` and `Poisoned` together under the mutex
(`view.go:452-476`), but copies the whole view and attaches a subscription per call (read); or a flag fed by `OnPoison`, which fires after
unlock (`:628`), so it is not atomic with `List`, and fires nothing on a heal (`:612`), so the flag needs `OnApply` tracking too.
**Recommendation:** (i) for keyed reads; for enumeration, (ii) only with one of those signals, which one left to F1. Group counts only drift by the next tick; a mixed read is a counting error, not a wrong entity
(judgement, not measured). Copy cost under the mutex: not measured; level-0 group count in real deployments: not measured.
**Fable candidate F1.**

### Q2. With no member list, how does a search reach a group's entities?

**Behaviour at stake:** text search over top groups returns matching entities, bounded, with any truncation declared; local
search returns E's group's matching entities.

**The standing decision is ported ADR-099 (`docs/adr/099-*.md:57-58`):** membership is "never stored" and "member enumeration rides the
existing paginated prefix-query lane". That is option (a). Anything else changes that sentence, and is the owner's. Round 1 left this
out, and cited only ADR-099's framing in favour of (c).

Pin: load up to 10,000 members (`graphrag.go:1417`, `MaxTotalEntitiesInSearch`), then keyword-filter in process (read).

| Option | Trade-offs |
| --- | --- |
| (a) Page `prefix` to a bound, declare truncation | ADR-099's decision. **A bound limits results, not cost:** each page rescans the group (section 1). About 10,000 entities: 0.85 s from a 10,000-member group, 3.86 s from 100,000, 54.5 s from the 1,000,000-member deployment, per group, up to 5 groups per request. About 20 MB crosses the wire (arithmetic, not measured) |
| (b) One page per group, declared `truncated` | Still the prefix lane, so within ADR-099's wording (judgement). About 450 entities at 2 KB. Page cost by group: 20 ms, 155 ms, 1.8-2.7 s (measured). Incomplete for big groups, honestly |
| (b+) (b), and refuse or redirect enumeration at level 2 | Level 2 is the whole deployment by definition (measured text). The refusal is new behaviour against "shape-for-shape" (`pin:...:44`); semsource sends no level (reported in `archive/...clustering/inventory.md:353-354`) |
| (c) Answer at group level: keywords, type counts, `RepEntities`; members only by other verbs | No enumeration. **Changes ADR-099's sentence**, and "shape-for-shape". Named reader: semsource decodes `entities`, `entity_ids`, `entity_digests`, `count` and reads community summaries (at `e4febc0d`, `query_tools.go:130`, `graph_matches.go:107`; reviewer, round 1). Text-tier recall falls (not measured) |
| (d) Keys-only prefix verb or an index in graph-ingest | A keys-only verb still lists every key under the prefix; only an index removes the rescan. New surface in a merged package. Cost not measured |

**Recommendation: withdrawn. Round 2 reversed from (c) to (a) on a model this table refutes:** it held that the bound decides the cost, and
it ignored level 2 and the five-group walk. I have no firm recommendation. My lean, as judgement only, is (b+): it stays on the ported lane,
and its worst case is one page per group. Sizes at levels 0 and 1 in real deployments are **not measured**, and they decide whether (b)'s
truncation is routine. **This is now a Fable candidate (F3), not a direct ruling:** it is a bounded judgement, has evidence, and may change a
ported decision.

### Q3. Is a relay for authority verbs the right shape?

**Behaviour at stake:** `graph.query.entity` answers a caller with the authority's reply or its refusal, class and code intact.

| Option | Trade-offs |
| --- | --- |
| (a) Relay and validate as the pin | Two hops; full decode of every reply in the relay (`query.go:299-345`, read); extra CPU unmeasured. Authority already validates (`graph.UnmarshalEntityState`, in-tree) |
| (b) Thin relay: forward bytes, propagate refusals | One responsibility; `scope` survives (ADR-071). Loses a defence against a broken authority: `TestPublicEntitySurfacesRejectCompleteCandidatePoisonBeforeSuccess` pins it. Needs the envelope to wrap raw bytes |
| (c) No relay: graph-ingest and graph-index answer `graph.query.*` directly | One hop; deletes about 340 lines (measured range). Amends D18, which says graph-query wraps graph-ingest's replies once (the governing text; the `graph-transport-boundary` requirement "Authority reads are not enveloped" binds only `graph.ingest.query.*`, spec `:85-87`, and `git grep 'graph\.query\.' openspec/specs` has no hits, measured). Consumers' deployments with no graph-query keep working (read). Moves the family's ownership |

**Recommendation:** (b), accepting its stated cost: a lost defence against a broken authority. (c) is cleaner on paper but amends D18 for a cost nobody has measured. Judgement.

### Q4. What does a reply carry, and what does `min_revision` do?

**Behaviour at stake:** a caller holding a write's revision can tell whether an answer reflects it, without polling and without
misreading `index_not_ready`.

Evidence: D18 says one envelope everywhere; the pin has 14 bare, 2 enveloped (measured `query.go:49-66`); consumers decode
bare `entity`/`batch` (measured). `indexed_revision` is defined for an index; an authority read's "revision" is per entity
(`kv_revision`); a `batch` has many. Graph-query holds no index state, so `min_revision` must be answered downstream. A
recreated bucket restarts the sequence (#103 comments); no marker exists.

| Option | Trade-offs |
| --- | --- |
| (a) D18 as written | Accepted; one shape. Every consumer decode changes; `indexed_revision` on an authority read needs a definition (unsolved) |
| (b) Envelope for index and composed verbs; authority verbs bare with `kv_revision`, table declares which | Matches what consumers decode (measured), keeps the sniff gone. Amends D18, the governing text: owner's. No spec names `graph.query.*` (measured) |
| `min_revision` (i): answer normally, caller compares `indexed_revision` | No wait, no new code; no read-your-writes promise |
| (ii): bounded wait in graph-index | The promise, at a cost: a timer in a read path; wall-clock correctness |
| (iii): new classified code, e.g. `revision_not_reached` | Observation, not prediction; caller chooses; new surface |
| Generation marker: now, or stays with #15 | Now: wire changes once. Later: a stale revision stays undetectable (#103 addendum) |

**Recommendation:** (b), (iii), marker stays with #15 (out of F2). The second is the weakest of the three. (i) meets ADR-109
("no reply waits") with no surface, and may be enough. **Fable candidate.**

### Q5. How is the search half structured?

**Behaviour at stake:** none directly; what changes is whether a strategy can be tested without a running component, and whether
degrade decisions can be made on a typed response.

Evidence: 63 functions, most methods on `*Component` (#105; measured declarations); strategies return `[]byte`, re-decoded
(`graphrag.go:796`, `searchgraph.go:220`); strategy dispatch by bare string; `Degraded` means synthesis or community.

| Option | Trade-offs |
| --- | --- |
| (a) Carry the file seam | Least change; keeps 2,600 lines' shape; hardest for reviewers |
| (b) Small internal types in one package; strategies return a typed response, the handler marshals once | Fits the owner's #105 ruling, needs no new package; removes the re-decode; 9.3-9.5 are rewrites more than ports |
| (c) Split packages: tier-0 relay and NL coordinator | What #105 asked; owner ruled against on 2026-10-24, before the new direction; tier 0 already builds with no provider |

**Recommendation:** (b). Measured clauses: re-decode sites. Judgement: reviewability. Ask whether "improve from ground up"
reopens #105; I would not, since exclusion already meets its lock-in concern (#105 comment 2026-10-24).

### Q6. Keep `AnswerSynthesizer`?

**Behaviour at stake:** a response's `answer` text from statistics at tier 0, with `degraded` unset.

(a) Keep the interface, as #9 comment 6386603659 says. Cost: an interface with one implementer until #487. (b) A plain function;
Issue #487 reintroduces the seam with the provider. Precedent: PR #483 dropped the summarizer interface (measured). **Recommendation:**
(b), an owner change to the 2026-10-24 line. The `answer` field stays either way: semsource reads it (`query_tools.go:126`, at `e4febc0d`; reviewer, round 1). Note the ruling's
excluded range `answer.go:122-244` contains `classifyDegradedReason` (`:222-234`), which non-LLM `synthesizeQueryAnswer` calls
(`graphrag.go:2093-2123`). With template only and no error path, it and the `Reason*` constants have no way to fire (read): under
(b) they go, under (a) they stay as an unreachable branch; `lanes3.py` charges them to 9.3 for now.

### Q7. Where do `summary` and `hierarchyStats` get counts?

**Behaviour at stake:** a total is either true or declared partial (P4).

(a) Keep the page, set truncation true when a cursor is returned, fail or flag on predicates: smallest, honest, still a scan.
(b) Answer from `COMMUNITY_INDEX` for prefixes of 2-4 segments (`MemberCount`, `TypeCounts` top 10): exact and cheap, but needs
change 7's writer, and has no answer below level 0. (c) Page to completion: the rescan, in a read path. **Recommendation:**
(a) in 9.2; (b) as a follow-up when change 7 lands.

### Q8. Path search cost and bounds

**Behaviour at stake:** a large traversal is bounded and says so; a bad request is refused.

One `RequestClassified` per visited node (two with `both`) (read); `max_nodes` unbounded and negative depth accepted (P7).
(a) Keep, add a server cap and validate: small. (b) A batched adjacency verb in graph-index: fewer hops, new surface, nobody
has measured this hurting. **Recommendation:** (a).

## 14. Fable candidates

Each is one question over named evidence, and each precedes a design. All evidence is in this repository or at the pin.

**F2, before 9.1 (Q4).** *"What is the one reply contract for `graph.query.*`: which verbs carry the envelope, what does
`indexed_revision` mean on an authority read, and how is an unmet `min_revision` answered?"* **The generation marker of #103 is out**
of this question; it stays with #15, and its answer does not change which verbs carry the envelope (judgement). Evidence: #103 and all
its comments; D18 of `archive/2026-10-12-setup-04a-02-ingest-kernel`; ADR-109 Decision 4; `graph/query_response.go`;
`processor/graph-index/query.go:79-95`; pin `query.go:49-66`; `openspec/specs/graph-transport-boundary/spec.md:85-95` as the scope limit;
semconnect `systems.go:808,:993` at `d0d06e00`.

**F1, before 9.3 (Q1).** *"While a tick is writing group records, what may one request observe of `COMMUNITY_INDEX`, and what must it
refuse?"* Evidence: `graph/clustering/types.go`, `maintainer.go:183-240`; `openspec/specs/community-grouping/spec.md` ("Removal follows
publication"); `archive/2026-10-31-...-clustering/design.md` D6; ADR-085 (`pin:...:133-137`), ADR-087 Decision 5, ADR-081;
`pkg/graphview/view.go:64-65`, `:397-441`, `:452-476` (`SnapshotAndSubscribe`), `:602-631`; pin `community_cache.go:66-119,:200-259`,
`graphrag.go:279-397`, `:1372-1498`, all read in this run. Enumeration cost is F3's; a request's cost during a tick is not part of F1.

**F3, before 9.3 (Q2).** *"Given that every page of a prefix read rescans the whole group, how should a search reach a group's entities?"*
Evidence: section 1's two cost tables and `notes/probes/prefix-cost-full-walks.log`, `notes/probes/prefix-cost-bounded-walks.log`; `docs/adr/099-*.md:19`,
`:57-58`, `:73`; `graph/query_prefix_types.go:30-33`; `processor/graph-ingest/query.go` prefix handler; pin `graphrag.go:162`, `:279-397`,
`:1372-1498`, `:29`; Q2's option table. Real sizes at levels 0 and 1 are unknown to it, and it should say what its answer depends on.

The task asked for one or two. There are now three. If only two run: F2, which gates 9.1, and F3, which may change a ported decision
(judgement). F1 can then go to the owner direct.

## 15. Owner questions outside the Fable candidates

Owner summary ruling numbers map as: 1 = F; 2 = A; 3 = Q2; 4 = Q3; 5 = Q6; 6 = D and Q8; 7 = E; 8 = B. Q5 and Q7 are the "smaller recommendations".

- A. Does the size limit count pin lines assigned, or lines the PR carries and writes? (section 12)
- B. One draft PR per sub-PR with #504 hosting 9.1? (section 10)
- C. Q2 (ADR-099's sentence), Q3, Q5, Q6, Q7, Q8 as above.
- D. Numeric bounds on a request (S18): required, or the design's call?
- E. Three binding, unported ADRs: 060 (classified errors), 071 (`semantic` forwards `scope` untouched), 087 Decision 5 (readiness on
  `COMMUNITY_INDEX` only). For each: port with a note, or say why not. No recommendation made; 087 has only Decision 5 live.
- F. Do the semsource and semconnect rows of `docs/inventory-scope.md` cover wire-subject literals? In my judgement not clearly (section
  3). **Decide first. If refused, these fall:** the consumer column of the verb matrix and the 4-of-16 count; Q2's named reader; Q4(b)'s
  "matches what consumers decode"; F2's semconnect evidence; section 11's decoder trace; section 12's semconnect and `searchGraph`
  statements. What stands: the pin column, so every verb remains framework API under the owner's 2026-10-12 ruling.

## 16. Unknowns and what was not done

- **Review budget is spent.** Reviewed in two rounds (CHANGES REQUESTED, sha256 `86507c33`, `f72fcd35`). This text, round 3, has not been
  re-reviewed. Whatever is open goes to the owner as an open question, not a settled point: Q2 has no firm recommendation; the sizing
  reading (A) and the reading scope (F) are unresolved; the owner questions in section 15 are not rulings.
- Prefix cost: measured in two idle runs with a wide spread, section 1; under load, and group sizes at levels 0 and 1: not measured. `View` poison behaviour: read, section
  13. Keyword-search recall under Q2(c): not measured.
- Semconnect, resolved (reviewer, round 1): `main` is `beta.160`; open PR 74, head `dff1265` (the local checkout), carries the pin. So
  `cmd/cs-graph-backend/main.go:127` cites a PR branch. `systems.go:80-81,:808,:993` agree at `d0d06e00` (measured by `gh api`).
- Semsource: `prefixquery.go:19`, `query_tools.go:107`, `:103` were read by `gh api` at `e4febc0d`; other semsource cites are local at
  `4093d3c` or reviewer-reported. Rule 4's re-check of the rest is owed if the owner keeps them under question F.
- #68's other open sites, S9's later subjects: not measured.

- `README.md` at the pin was read by headings and the metrics table only, so the "advertised absent" list is a floor, not a
  ceiling. `graphrag.go` was read in the listed ranges (`:1-160`, `:279-492`, `:633-1000`, `:1374-1570`, `:1716`, `:2218-2318`);
  the digest code at `:1753-2106` and `:2473-2561` by declaration and grep only, and it is not in F1's evidence.

### Conduct notes for the reviewer

- `docs/inventory-scope.md` rule 4 says sister checkouts are never the target of `git` commands; this run ran one read-only
  `git -C <repo> rev-parse --short HEAD` on the consumer checkouts. Nothing changed. Later cites used `gh api`.
- A `semstreams-ui` grep ran for `graph.query.` (row allows only `output/websocket`). It hit one generated file, not used here.
- Semteams was read beyond its row's scope; the reviewer struck those citations (round 1) and they are gone from every
  conclusion. Semsource and semconnect wire literals also go beyond those rows' wording: flagged to the owner, section 15 F.
- Round 2: no sister-repository reads. Facts marked "reviewer, round 1" were reported by the inventory reviewer and were not
  re-read here, to obey the instruction to do no further out-of-scope reads.
- `git fetch origin claude/flake-gates-500` ran in the worktree to read #501's rules. No working-tree change.
- All writes: under `/tmp/setup-04a-04-graph-query-stage/` only, including a `git archive` of this tree with one probe test added. A temporary probe test was added to and removed from the pin copy.
