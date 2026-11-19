# Tasks: fix-658-keep-serving-past-damage

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" means failing-first output
in `notes/<task>.md`, or a `task mutate:check` run. Premises are in `inventory.md` and `design.md`. Three pull
requests, merged in order: PR 1 (task 2), PR 2 (task 3), PR 3 on draft #693 (tasks 4 to 6), which closes the
issue, #658. PR 1 brings this change folder to `main`; PR 2 and PR 3 tick their tasks against that
copy. #657 stays open for its other follow-ups.

## 1. Design

- [x] 1.1 Architect inventory and design (`inventory.md`, `design.md`, spec deltas), INVENTORY PASS, a passing
  pre-owner design review, and rulings on Q1 to Q3.

## 2. PR 1: the count and graph-query's relay (design D3, D4)

- [x] 2.1 Tests first, then `graph.QueryResponse.DamagedEntities` (`damaged_entities`, omitted at 0) with its doc
  comment (`graph/query_response.go:8-24`). Unit: the field round-trips and is absent at 0.
- [x] 2.2 graph-query: `TestComposedReplyCarriesLargestDamagedCount` (stubbed index replies at 2 and 3 give 3, for
  `relationships`, `pathSearch`, `summary`, `entityByAlias`, `localSearch`, `globalSearch`, `searchGraph`), shown
  failing on `main`; then carry the count through `decodeIndexReply` (`processor/graph-query/query.go:367`) to
  `envelope` (`:381`), `localsearch.go:214` and `textsearch.go:207`. `TestForwardPassesBytesBothWays` stays.
- [x] 2.3 If Q2 is yes: `TestSearchTruncatedWhileDamaged`, then the `truncated` expressions (`localsearch.go:204`,
  `textsearch.go:196`, the merged path) and the `Truncated` doc comments
  (`graph/query_local_search_types.go:56`, `graph/query_global_search_types.go:65`, `:73`).
- [x] 2.4 Docs: `graph/README.md`, `processor/graph-query/README.md`, `processor/graph-query/MIGRATION.md` (a bullet
  for semsource's `truncated` reader and semconnect's predicate reader).

## 3. PR 2: fusion and graph-gateway (design D4; Q3 ruled yes, 2026-11-18)

- [x] 3.1 fusion: `TestReadsPassOnDamagedCount` and `TestResponseCarriesDamagedCount`, shown failing on `main`; then
  `fusionnats.decodeReply` (`client.go:446-465`) returns the count, `Resolution.DamagedEntities`, the
  `fusion.Neighbors` and `fusion.Names` result types (`retrieval.go:41,43`), their callers (`engine_facets.go:255`,
  `engine_lens.go:298`, `:451`), `fusion.Response.DamagedEntities` (`contract.go:181-191`); `pkg/fusion/README.md`,
  `MIGRATION.md` (semsource's `source/fusion/fusiontest/memgraph.go` returns the structs).
- [x] 3.2 graph-gateway: `TestAnswerCarriesDamagedCount`, shown failing on `main`; then `resolve.go:82-86` keeps the
  count and the response carries `extensions.damagedEntities`; its README.

## 4. PR 3: the follower records damage (design D1, D5)

- [x] 4.1 Tests first in `graph/follower`: the generated check over reports, completions and deletes against a
  reference model (seed and completed runs recorded); named cases first (report then higher completion clears; a
  completion at the same or a lower revision does not; a delete clears; `Stuck` names at most 10 IDs sorted, with
  reasons, the count and the repair); the envelope reads `degraded` with `failed_reasons["damaged_entity"]` and
  `QueryReady` answers nil. Rewrite `TestFollowerLatchesResetOnUnreadableSnapshotValue`
  (`follower_integration_test.go:476-503`) as a replay that does not stop, and the reset clause of
  `TestWithholdNarrowsQueryReady` (`follower_test.go:483-490`). The generated check draws completions at, below and
  above a report.
- [x] 4.2 Then the follower: replace `Reset` and the latch (`follower.go:115`, `:299-309`, `:361-365`, `:378-379`,
  `:413-416`, `:458-465`, `resetRemedy` `:97-98`) with the per-key record and its two read methods; package comment.

## 5. PR 3: the four plug-ins (design D1, D2, D3)

- [x] 5.1 graph-index: integration tests, replay and live, as the design's "Proving tests", among them
  `TestDamagedEntityLeftOutAndNamed` (the graph-transport-boundary delta's scenario); shown failing on `main`.
  Rewrite `TestNoncanonicalValueLatchesResetAndHoldsItsRevision` (`index_integration_test.go:480-503`) and the "a
  latched reset" subtest (`lifecycle_integration_test.go:403-410`), and the two reset subtests of
  `TestErrorCountCountsOutcomesNotOperations` (`index_integration_test.go:528-550`) as one error per newly damaged
  entity. Waits: a request with `min_revision` B's revision. Then `index.go:184-192` and `:412-427` report,
  remove rows (`deleteFromIndexes`, `:520`) and complete; `queryRevision` (`query.go:79`) sets the count; `outgoing`
  refuses a damaged entity.
- [x] 5.2 graph-embedding: rewrite the "after a reset" subtest (`withhold_integration_test.go:153-165`) and the
  reset step of `TestHealthIgnoresErrorCount` (`health_integration_test.go:76-86`, which would wait forever); add the
  replay case; shown failing on `main`. Waits: the follower's floor, then the vector cache past B's delete. Then
  `hop1.go:135-147` reports, removes (`removeRecord`, `:121`) and completes;
  `search` counts within scope, `similar` refuses a damaged source (`query.go:94-143`).
- [x] 5.3 graph-lexical: integration tests, replay and live, shown failing on `main`; rewrite `apply_test.go:191-202`.
  Waits: a request with `min_revision` B's revision.
  Then `component.go:240-251` and `apply.go:158-161` report, delete the entity's text and complete; `search` counts
  within scope (`query.go:81-110`).
- [ ] 5.4 graph-clustering: rewrite `TestResetStopsPublication` (`publication_integration_test.go:249-277`) so the
  replay publishes the other groups, shown failing on `main`, and `TestRefusedValueReportedNotWithheld` to expect
  `damaged_entity` and the ID in `Health`; add `TestDamagedOverwriteLeavesGroup`. Waits: the floor, then a tick by
  hand. Then `deliver` (`component.go:269-290`) reports and applies a delete in
  place of `Upsert(nil)`; comments at `:156`, `:317`, `component_integration_test.go:163-168`.

## 6. PR 3: records and docs

- [ ] 6.1 ADR-108 status and D4 (`docs/adr/108-processor-shell.md:8-11`, `:109-112`), ADR-109 D1 `:72-77`, D2
  `:88-95`, row `:195`: amended citing #657 ruling 3 and #658 comment 6962917422. ADR-074 port note `:13-18` and ADR-084
  (`:139-142`): one bullet each. `graph/state_contract.go:55-62` comment.
- [ ] 6.2 `docs/admission-ledger.yaml` rows graph/follower `:3620-3666`, graph-index `:3560-3566`, `:3606`, ADR-074
  `:3668-3680`, graph-embedding `:4598`, graph-clustering `:4927`; READMEs and MIGRATION notes listed in
  `inventory.md` § 3; `.agents/skills/query-pattern/SKILL.md:53-56`; `graph/index_status.go:80-81` (graph-index now
  emits `failed_reasons`). The "always absent today" lines PR 2 left: `pkg/fusion/README.md:69-70`,
  `pkg/fusion/MIGRATION.md:131-132`, `gateway/graph-gateway/README.md:80`, `gateway/graph-gateway/MIGRATION.md:84`,
  `processor/graph-query/MIGRATION.md:127`; ledger rows pkg/fusion `:4682`, pkg/fusion/fusionnats `:4802` (its
  contract `:4824`: an nl Resolve copies `damaged_entities` too, as do byName and relationships) and graph-gateway
  `:1931` (answers carry `extensions.damagedEntities`).
- [ ] 6.3 Remove `graph.IndexStateResetRequired` from `graph/index_status.go` and `IndexStates()`, the
  `IndexStatusResponse.Code` and `Reason` fields (`:33-36`), the gate's branch (`graph/readiness/readiness_gate.go:127`)
  and their tests (`publisher_test.go:88`, `readiness_gate_test.go:101`, `:331`,
  `internal/smoke/stack_integration_test.go:663`);
  `TestEvaluateReadinessGate_UnrecognizedStateFailsClosed` gains `reset_required`; a semconnect migration note
  (`conformance/cmd/index-readiness/main.go:155`). File the adoption-sweep issue for one damage-record type
  (graph-ingest `poison_inventory.go:84`, graph-clustering `maintainer.go:143`, the follower).
- [ ] 6.4 Before PR 3 is marked ready: post on #658 what counts as done; PR title and body match.

## 7. Close

- [ ] 7.1 Review request with the archive in the reviewed head; `reviewed-by:` in the PR body when the record lands.
- [ ] 7.2 Archive the change and sync the specs as the last content commit of PR 3.
