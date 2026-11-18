# Inventory: fix-658-keep-serving-past-damage

base: be77e6b50049cae5707ac3a43590af35472b232c (main 8872c707 + empty claim commit; no code change)

Question: what changes when graph-index, graph-embedding and graph-lexical keep answering with a damaged entity
stored, as #657 ruling 3 and #658 (owner, 2026-11-18, comment 6962917422) ask, and who reads the replies and health
that change? Repositories read: this one; consumer checkouts semsource, semconnect, semteams, semsparql, semboids
(working trees read with grep, no git command run in them). No pin package is carried, so § 6 lists only the ADRs in
`docs/adr/` that record the stop.

A *damaged entity* is a stored `ENTITY_STATES` value `graph.UnmarshalEntityState` refuses
(`graph/entity_predicate_contract.go:287-303`). The *reset latch* is the follower's process-lifetime stop.

## 1. The claimed gap

- The latch: `graph/follower/follower.go:363-365` — `func (f *Follower) Reset(reason graph.StateResetReason) {` /
  `f.reset.CompareAndSwap(nil, &reason)`. First reason holds for the life of the process.
- The follower sets it itself in the startup replay, for every plug-in: `:299-309` — `if validating {` …
  `f.Reset(reason)`.
- It refuses every request: `:413-416` — `if r := f.reset.Load(); r != nil {` /
  `return errs.ClassifiedCode(errs.ErrorFatal, graph.ErrorCodeGraphStateResetRequired,`; envelope `reset_required`:
  `:458-465`; `Stuck`: `:378-379`.
- Plug-in call sites (four, `git grep -n '\.Reset(' -- '*.go' ':!*_test.go'`):
  - graph-index, value refused: `processor/graph-index/index.go:412-427` — `c.follower.Load().Reset(reason)`; for
    `noncanonical_entity_id` it also marks the entity failed and keeps the revision pending (`:421-423`, `:212`).
  - graph-index, key not a literal ID: `index.go:184-192` —
    `c.follower.Load().Reset(graph.GraphStateReasonNoncanonicalEntityID)` / `return false` (revision kept pending).
  - graph-embedding: `processor/graph-embedding/hop1.go:135-147` — `c.follower.Load().Reset(reason)` / `return`; the
    revision stays pending and the entity's earlier record is kept.
  - graph-lexical: `processor/graph-lexical/component.go:240-251` — `c.follower.Load().Reset(reason)`; the revision
    stays pending (`apply.go:158-161` returns before `complete`), the earlier text stays in the index.
- **A fourth affected component the issue does not name:** graph-clustering. It skips a damaged value written live
  (`processor/graph-clustering/component.go:269-290`), but the follower's replay check latches its reset for a damaged
  value already stored at start, and `tick` then publishes nothing (`component.go:322-324` —
  `if err := c.follower.Load().QueryReady(ctx); err != nil {` / `return`). Proven today by
  `TestResetStopsPublication` (`processor/graph-clustering/publication_integration_test.go:249-277`).
- Other follower users: none (`git grep -n 'follower.Open' -- '*.go'`: graph-index `component.go:236`, graph-embedding
  `component.go:324`, graph-lexical `component.go:185`, graph-clustering `component.go:192`). Spatial and temporal are
  not ported (#109).
- No existing per-entity damage record in the follower: `git grep -n -i 'damaged' -- graph/follower/`: no hit.

## 2. Every spelling of "damaged" and "left out of a reply"

Damage records today:

- graph-ingest's poison inventory: `processor/graph-ingest/poison_inventory.go:84` (`recordEntityPoison`), cleared on
  delete, commit and valid read at a higher revision (`:195`, `:206`, `:217`), health sample of 10 IDs with reasons
  (`:227-241`, `entityPoisonSampleCap`).
- graph-clustering's maintainer refused set: `graph/clustering/maintainer.go:143-147`; `IncompleteError` names up to
  10 (`:33-43`); the component reports it as `refused_entity` (`processor/graph-clustering/component.go:292-314`).
- The follower's reset latch (above): one reason, no entity ID (`follower.go:115`).
- graph-index's failed set holds a `noncanonical_entity_id` entity beside transient write failures
  (`index.go:244-250`), so it withholds queries as `index_not_ready` (Withhold is nil, `component.go:245`).

Reply counts of records left out (all whole-store or whole-index, none for damaged entities):

- `graph.SemanticData` / `SimilarData`: `unreadable_records` (undecodable EMBEDDING_INDEX records), `failed_records`
  (`graph/query_search_types.go:51-67`).
- `graph.LexicalSearchData.BodiesNotIndexed`, "the whole index, not the scope" (`graph/query_lexical_types.go:29-36`).
- `graph.LocalSearchData.Damaged` / `TextSearchData.Damaged` (from the batch, #656) and `UnreadableGroups`
  (`graph/query_local_search_types.go:68`, `graph/query_global_search_types.go:83-85`).
- graph-index replies have no count: skipped malformed rows go to a log and `query_rows_skipped_total` only
  (`processor/graph-index/query.go:713-718`).
- The envelope every graph-index, graph-embedding and graph-lexical reply shares, `graph.QueryResponse`, has `data`,
  `indexed_revision`, `producer`, `timestamp` only (`graph/query_response.go:19-24`).
- `fusion.Response.BodiesNotIndexed`, copied from the word index's reply onto every response
  (`pkg/fusion/contract.go:181-189`).

## 3. Adjacent claims

- Open PRs: only #693 (this claim, 0 files) — `gh pr list --state open`.
- Issues naming the packages (queries `follower`, `graph-index`, `graph-embedding`, `graph-lexical`, `graph-clustering`,
  `damaged`, `reset_required`, `poison`, `graph-query`): #657 (ruling), #658 (this), #109 (follower adopters, spatial
  and temporal still to plug in — they inherit the new contract), #110 (item 3: temporal writes `reset_required` into
  `HealthStatus.Status`, fixed at its port), #602 (a generation field on `graph.QueryResponse` — another additive
  envelope field, unclaimed, no conflict), #554 (follower and graph-index integration await helpers; tests this change
  adds follow the rule #554 fixes). Names the word only: #108, #141, #195, #196, #477, #485, #574, #665, #683 and the
  epics.
- Specs asserting the stop (each becomes MODIFIED or REMOVED):
  - `openspec/specs/entity-state-follower/spec.md:64-73` "The startup replay validates each stored entity";
    `:120-128` "Reset is sticky"; `:130-140` "The envelope" (`reset_required`).
  - `openspec/specs/graph-index/spec.md:56-67` and `:69-78`.
  - `openspec/specs/graph-embedding/spec.md:151-174`.
  - `openspec/specs/graph-lexical/spec.md:74-91`, `:93-98` (reply fields).
  - `openspec/specs/graph-clustering/spec.md:41-57` ("or a latched reset").
  - `openspec/specs/composed-stack/spec.md:85-99` says no envelope reports `reset_required`: stays true.
  - `openspec/specs/fusion/spec.md:22-33` ("A fatal seed read fails the call", "`graph_state_reset_required` from a
    reset") and `:78-90` ("A walk read that fails is skipped", a relation target's read returning
    `graph_state_reset_required`); `:286-295` and `:327-340` copy the word index's `bodies_not_indexed` onto
    `Resolution` and every response (the nearest shape for a count).
  - `openspec/specs/graph-gateway/spec.md:88-103` (refusals keep class and code in `errors[].extensions`) and
    `:47-56` (an answer holds exactly the selected fields): no text on the reset.
  - `openspec/specs/graph-readiness/spec.md:12-25`: the gate's `hard_stop` names `reset_required`.
- Tests asserting the stop: `graph/follower/follower_integration_test.go:476-503`
  (`TestFollowerLatchesResetOnUnreadableSnapshotValue`), `follower_test.go:483-490` (`TestWithholdNarrowsQueryReady`,
  reset clause); `processor/graph-index/index_integration_test.go:480-503`
  (`TestNoncanonicalValueLatchesResetAndHoldsItsRevision`), `lifecycle_integration_test.go:403-410` ("a latched reset");
  `processor/graph-embedding/withhold_integration_test.go:153-165` ("after a reset");
  `processor/graph-lexical/apply_test.go:191-202`; `processor/graph-clustering/publication_integration_test.go:249-277`,
  `component_integration_test.go:163-175` (comment on the replay latch). `internal/smoke/stack_integration_test.go:663`
  checks `reset_required` is absent: rewritten in PR 3. Also: graph-index `TestErrorCountCountsOutcomesNotOperations`, subtests
  "one reset that holds the entity" and "one reset that completes" (`index_integration_test.go:528-550`, an
  `ErrorCount` of 1 per reset); graph-embedding `TestHealthIgnoresErrorCount` (`health_integration_test.go:76-86`),
  which waits for "graph state reset required" in `LastError` and would wait forever once the latch goes.
- ADR text (this repository's copies): ADR-074 port note `docs/adr/074-canonical-predicate-contract.md:13-18` and
  decision `:87-93` ("Stored-state poison is permanent for the process lifetime"); ADR-084 D4
  `docs/adr/084-readiness-licenses-health-not-absence.md:139-142` ("`reset_required` … never self-clears"); ADR-108
  status `docs/adr/108-processor-shell.md:8-11` and D4 `:109-112` (the latch the follower holds); ADR-109 D1 `:72-77`,
  D2 `:88-95` ("`reset_required` never clears in process") and the kept-decisions row `:195`; ADR-110 `:152-154` (names
  the sticky state as not decided there).
- Ledger rows: graph/follower `docs/admission-ledger.yaml:3620-3666` (contract and proving tests name the latch);
  graph-index `:3549` (`:3560-3566`, `:3606`); ADR-074 `:3668-3680`; graph-embedding `:4598`; graph-clustering `:4884`
  (`:4927`). graph-lexical has no row (`git grep -n 'source_path: processor/graph-lexical' docs/admission-ledger.yaml`:
  none).
- Docs: `processor/graph-index/README.md:92-100`, `:160-164`; `processor/graph-embedding/README.md:126-133`, `:199`,
  `MIGRATION.md:72-86`; `processor/graph-clustering/README.md:92-106`, `MIGRATION.md:76-82`;
  `processor/graph-lexical/README.md:43-60`; `graph/README.md:25-27`; `.agents/skills/query-pattern/SKILL.md:53-56`.

## 4. Who it is for (surface the design will add)

To be named in the design: an envelope count (readers: graph-query, fusionnats, graph-gateway, semsource, semconnect;
§ Adopter seam) and follower methods (production callers: the four plug-ins' delivery and query handlers).

## 5. The problem shape

"A per-entity damage record that clears itself on repair, named in health, counted in the reply." Closest instances:
graph-ingest's poison inventory (`poison_inventory.go:84-241`) and graph-clustering's refused set with
`refused_entity` (`maintainer.go:143-147`, `component.go:292-314`, Withhold 0 at `:203`). Readiness shape for "counted,
not withheld": `follower.Config.Withhold` (`follower.go:83-87`), used by graph-clustering and graph-lexical.

## 6. Decision records

Listed in § 3. Each records the stop as a decision: ADR-074 (whole store under another rule, remedy a wipe),
ADR-084 D4, ADR-108 D4, ADR-109 D1/D2. The owner's ruling (#657 ruling 3; #658 comment 6962917422) changes that
decision.

## Same-class collision table: "stored entities this component refused"

| Dimension | Evidence |
| --- | --- |
| Owners | graph-ingest poison inventory; graph-clustering maintainer refused set; follower reset latch; graph-index failed set (identity mismatch only) |
| Status | graph-ingest Health sample (`poison_inventory.go:227`); clustering `refused_entity` in `failed_reasons`; follower `reset_required` envelope and `Stuck`; graph-index `failed_count` |
| Lifecycle | graph-ingest and clustering clear on a later good write or delete; the follower latch never clears |
| Ownership | each per process, per component; no shared bucket |
| Readers | `Health()` of each; GRAPH_STATUS readers (semsource `processor/mcp-gateway/tools.go:141-144`, `processor/supersession/versiondiff_serve.go:204`, `processor/source-manifest/workbench_capabilities.go:108-114`; semconnect `conformance/cmd/index-readiness/main.go:155-179`) |
| Writers | the decode sites in § 1 |
| Recovery | graph-ingest self-heals; follower: wipe and restart (`resetRemedy`, `follower.go:98`) |

## Adopter seam inventory

Readers of the affected replies (consumer working trees):

- semsource `processor/mcp-gateway/graph_matches.go:100` decodes `QueryResponse[GlobalSearchData]` and reads
  `truncated` only (`:130`). semsource runs graph-gateway (`cmd/semsource/run.go`) and uses `pkg/fusion`
  (`processor/code-context/*.go`, `source/fusion/lens/*`; `source/fusion/fusiontest/memgraph.go` implements fusion's
  `RetrievalClient`).
- semconnect `gateway/cs-api/systems.go:993-997` decodes `QueryResponse[PredicateData]` and returns `Data.Entities`;
  `conformance/cmd/index-readiness/main.go:155-156` fails on `reset_required`, `:179` gates on the readiness verdict.
- semteams, semsparql, semboids: no reader of these verbs or of GRAPH_STATUS (grep for the subjects,
  `QueryResponse`, `GRAPH_STATUS`: no production hit).
- In this repository: graph-query decodes every index envelope in one helper, `decodeIndexReply`
  (`processor/graph-query/query.go:367-376`; callers `matches.go:58`, `merge.go:151`, `query.go:244,316,337`,
  `summary.go:85`) and builds replies in `envelope` (`query.go:381`) and two direct sites (`localsearch.go:214`,
  `textsearch.go:207`); its forwards pass bytes unchanged (graph-query spec `:20-31`, `:229-245`).
  `pkg/fusion/fusionnats/client.go:446-465` (`decodeReply`) and `gateway/graph-gateway/resolve.go:82-86` keep only
  `Data`, so any envelope field stops there.
- graph-query also reads `COMMUNITY_INDEX` records into search replies: `localsearch.go:206-208` (`groupRecord`,
  `attachedRecord`) and `processor/graph-query/community.go`; graph-clustering builds those records, and a damaged
  entity is in none of them.
- graph-query's `pathSearch` hops through `outgoing`/`incoming` and a failed hop ends the walk with the hop's class and
  code (`processor/graph-query/pathsearch.go:102-120`; graph-query spec "A failed read ends a path search").
- fusion's `RetrievalClient` (`pkg/fusion/retrieval.go:15-43`): `Resolve` returns `Resolution` (a struct),
  `Neighbors` `[]Edge` and `Names` `[]string`; semsource's `source/fusion/fusiontest/memgraph.go` implements it.
  `fusion.Response.Truncated` means the budget cut nodes off (`pkg/fusion/README.md:71`), `relations_truncated` a
  node's missing ref (`contract.go:103-106`), `bodies_not_indexed` the word index's count (`contract.go:181-189`).
- graph-gateway answers a classified refusal in `errors[].extensions` (`gateway/graph-gateway/component.go:187-197`)
  and keeps only an envelope's `Data` (`resolve.go:82-86`).

1. What must they know today? That `graph_state_reset_required` from these verbs means "do not retry, wipe".
2. If the components serve and nothing else changes: a caller that reads `data` only gets an answer missing the damaged
   entity's rows and is not told — the case ruling 1 forbids. semsource reads `truncated` only.
3. Where they find out: nowhere, unless the reply carries it and every relay passes it on.
4. What they should have to know: nothing to stay correct; reading one count to explain a gap.

## Measurements not made

No test was run. The cost of an extra decode per delivered value was not measured (the design avoids it).
