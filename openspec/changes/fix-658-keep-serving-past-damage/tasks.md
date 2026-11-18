# Tasks: fix-658-keep-serving-past-damage

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" means failing-first output
in `notes/<task>.md`, or a `task mutate:check` run. Premises are in `inventory.md` and `design.md`. Three pull
requests, merged in order: PR 1 (task 2), PR 2 (task 3), PR 3 on draft #693 (tasks 4 to 6), which closes the
issue, #658. #657 stays open for its other follow-ups.

## 1. Design

- [ ] 1.1 Architect inventory and design (`inventory.md`, `design.md`, spec deltas), INVENTORY PASS, a passing
  pre-owner design review, and rulings on Q1 to Q3.
  Hold: OPEN-Q proposal awaits owner approval

## 2. PR 1: the count and graph-query's relay (design D3, D4)

- [ ] 2.1 Tests first, then `graph.QueryResponse.DamagedEntities` (`damaged_entities`, omitted at 0) with its doc
  comment (`graph/query_response.go:8-24`). Unit: the field round-trips and is absent at 0.
- [ ] 2.2 graph-query: `TestComposedReplyCarriesLargestDamagedCount` (stubbed index replies at 2 and 3 give 3, for
  `relationships`, `pathSearch`, `summary`, `entityByAlias`, `localSearch`, `globalSearch`, `searchGraph`), shown
  failing on `main`; then carry the count through `decodeIndexReply` (`processor/graph-query/query.go:367`) to
  `envelope` (`:381`), `localsearch.go:214` and `textsearch.go:207`. `TestForwardPassesBytesBothWays` stays.
- [ ] 2.3 If Q2 is yes: `TestSearchTruncatedWhileDamaged`, then the `truncated` expressions (`localsearch.go:204`,
  `textsearch.go:196`, the merged path) and the `Truncated` doc comments
  (`graph/query_local_search_types.go:56`, `graph/query_global_search_types.go:65`, `:73`).
- [ ] 2.4 Docs: `graph/README.md`, `processor/graph-query/README.md`, `processor/graph-query/MIGRATION.md` (a bullet
  for semsource's `truncated` reader and semconnect's predicate reader).

## 3. PR 2: fusion and graph-gateway (design D4; only if Q3 is yes)

- [ ] 3.1 Hold: OPEN-Q Q3 decides whether this task is done here or filed as an issue. fusion: a test that an answer
  built after a read whose reply named `damaged_entities` N carries N (the largest seen), shown failing on `main`;
  then `fusionnats.decodeReply` (`client.go:446-465`) keeps the count and the `RetrievalClient` results carry it;
  `pkg/fusion/README.md`, `MIGRATION.md` (semsource's `fusiontest/memgraph.go` implements the interface).
- [ ] 3.2 graph-gateway: a test that a GraphQL root field over an enveloped verb reports the count, shown failing on
  `main`; then `resolve.go:82-86` and the schema; its README.

## 4. PR 3: the follower records damage (design D1, D5)

- [ ] 4.1 Tests first in `graph/follower`: the generated check over reports, completions and deletes against a
  reference model (seed and completed runs recorded); named cases first (report then higher completion clears; a
  completion at the same or a lower revision does not; a delete clears; `Stuck` names at most 10 IDs sorted, with
  reasons, the count and the repair); the envelope reads `degraded` with `failed_reasons["damaged_entity"]` and
  `QueryReady` answers nil. Rewrite `TestFollowerLatchesResetOnUnreadableSnapshotValue`
  (`follower_integration_test.go:476-503`) as a replay that does not stop, and the reset clause of
  `TestWithholdNarrowsQueryReady` (`follower_test.go:483-490`).
- [ ] 4.2 Then the follower: replace `Reset` and the latch (`follower.go:115`, `:299-309`, `:361-365`, `:378-379`,
  `:413-416`, `:458-465`, `resetRemedy` `:97-98`) with the per-key record and its two read methods; package comment.

## 5. PR 3: the four plug-ins (design D1, D2, D3)

- [ ] 5.1 graph-index: integration tests, replay and live, as the design's "Proving tests"; shown failing on `main`.
  Rewrite `TestNoncanonicalValueLatchesResetAndHoldsItsRevision` (`index_integration_test.go:480-503`) and the "a
  latched reset" subtest (`lifecycle_integration_test.go:403-410`). Then `index.go:184-192` and `:412-427` report,
  remove rows (`deleteFromIndexes`, `:520`) and complete; `queryRevision` (`query.go:79`) sets the count; `outgoing`
  refuses a damaged entity.
- [ ] 5.2 graph-embedding: rewrite the "after a reset" subtest (`withhold_integration_test.go:153-165`); add the replay
  case; shown failing on `main`. Then `hop1.go:135-147` reports, removes (`removeRecord`, `:121`) and completes;
  `search` counts within scope, `similar` refuses a damaged source (`query.go:94-143`).
- [ ] 5.3 graph-lexical: integration tests, replay and live, shown failing on `main`; rewrite `apply_test.go:191-202`.
  Then `component.go:240-251` and `apply.go:158-161` report, delete the entity's text and complete; `search` counts
  within scope (`query.go:81-110`).
- [ ] 5.4 graph-clustering: rewrite `TestResetStopsPublication` (`publication_integration_test.go:249-277`) so the
  replay publishes the other groups, shown failing on `main`, and `TestRefusedValueReportedNotWithheld` to expect
  `damaged_entity` and the ID in `Health`. Then `deliver` (`component.go:269-290`) reports and applies a delete in
  place of `Upsert(nil)`; comments at `:156`, `:317`, `component_integration_test.go:163-168`.

## 6. PR 3: records and docs

- [ ] 6.1 ADR-108 status and D4 (`docs/adr/108-processor-shell.md:8-11`, `:109-112`), ADR-109 D1 `:72-77`, D2
  `:88-95`, row `:195`: amended citing #657 ruling 3 and #658 comment 6962917422. ADR-074 port note `:13-18` and ADR-084
  (`:139-142`): one bullet each. `graph/state_contract.go:55-62` comment.
- [ ] 6.2 `docs/admission-ledger.yaml` rows graph/follower `:3620-3666`, graph-index `:3560-3566`, `:3606`, ADR-074
  `:3668-3680`, graph-embedding `:4598`, graph-clustering `:4927`; READMEs and MIGRATION notes listed in
  `inventory.md` § 3; `.agents/skills/query-pattern/SKILL.md:53-56`; `graph/index_status.go:80-81` (graph-index now
  emits `failed_reasons`).
- [ ] 6.3 File the follow-up: `reset_required`, `IndexStatusResponse.Code` and `Reason` have no producer
  (`status:blocked`, linked without a closing keyword). The adoption sweep issue for one damage-record type
  (graph-ingest `poison_inventory.go:84`, graph-clustering `maintainer.go:143`, the follower).
- [ ] 6.4 Before PR 3 is marked ready: post on #658 what counts as done; PR title and body match.

## 7. Close

- [ ] 7.1 Review request with the archive in the reviewed head; `reviewed-by:` in the PR body when the record lands.
- [ ] 7.2 Archive the change and sync the specs as the last content commit of PR 3.
