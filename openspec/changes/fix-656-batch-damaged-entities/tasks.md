# Tasks: fix-656-batch-damaged-entities

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" means failing-first output
in `notes/<task>.md`, or a `task mutate:check` run. Premises are in `inventory.md` and `design.md`. One pull
request, #692, which closes #656. #657, #658 and #659 stay open.

## 1. Design

- [ ] 1.1 Architect inventory and design (`inventory.md`, `design.md`, spec deltas). Needs INVENTORY PASS, a passing
  pre-owner design review, and rulings on O1 and O2. INVENTORY PASS and design review PASS at e7b495dd (Claude
  reviewer); O1 and O2 ruled 2026-11-18, D3 rewritten to the O1 ruling; review PASS at 87c23833 (round 3).
  Hold: OPEN-Q proposal awaits owner approval

## 2. Code

- [ ] 2.1 graph and graph-ingest, tests first (design D1).
  - Write these tests: `TestIntegration_BatchQuery_NamesDamagedEntities` and
    `TestIntegration_BatchQuery_AllDamaged` in `processor/graph-ingest/batch_missing_integration_test.go`; the
    generated accounting check; the damage-plus-fault case (`TestBatchDamageBesideFaultIsTransient`).
  - Rewrite the `batch` subtest of `TestAggregateReadNamesEveryPoisonedEntity`
    (`poison_scoping_test.go:289-294`) and the `batch` site of `TestEveryEntityReadRefusesTheSameStoredPoison`
    (`:429-430`). Their `prefix` subtests and sites stay untouched and keep passing.
  - Then add `graph.MissingDamaged` and its doc comment, change the batch path, seed the `damaged` counter series, and
    make the debug log at `query.go:199` give each reason's count, not a fixed `not_found`.
  - `notes/2.1.md` shows each new test failing on `main`.
- [ ] 2.2 fusionnats, tests then code (design D2).
  - Remove `entitiesAlone` and its branch (`client.go:276-282`, `:293-342`) and map `damaged` in
    `unhydratedReason` (`:347-356`).
  - Rewrite `poison_test.go` as the D2 tests and add `"damaged"` to the reasons drawn in `reconcile_prop_test.go:101`.
  - Flip the precondition in `TestPoisonedSeedOverRealResponders` (`client_integration_test.go:439-445`), shown
    failing on `main`.
  - Rewrite `TestOwnValidationNotRetried` for the one-request path, and keep a test that every seed damaged answers
    `deferred` `all_seeds_unhydrated` (its proof today, `TestAllSeedsPoisonedDefer`, is deleted).
  - Update `Entities`' doc comment (`:251-267`).
- [ ] 2.3 graph-query, tests then code (design D3). Write `TestSearchNamesDamagedHit`, then add `Damaged` to
  `graph.LocalSearchData` and `graph.TextSearchData`, change `readMatches` (`matches.go:113-131`) and its two callers,
  and update the `Missing` and `Truncated` doc comments (`graph/query_local_search_types.go:52-53`,
  `graph/query_global_search_types.go:60-61` and its searchGraph paragraph `:66-69`), the `answerText` comment
  (`textsearch.go:171`), and the `Truncated:` expressions (`localsearch.go:203`, `textsearch.go:194`) so a damaged
  hit sets them. Also write `TestSearchGraphMergedNamesDamagedHit`.
- [ ] 2.4 Docs:
  - `processor/graph-ingest/README.md:123-128` (including "reads them one at a time"), and `:172` if the counter's
    reasons are listed there.
  - `processor/graph-query/doc.go:51`.
  - `docs/admission-ledger.yaml`: graph-query `:4407`, `:4436`; fusionnats contract item (6) `:4828-4834`, side
    effects `:4844`, and `proving_tests` `:4875-4878`, which name tests this change deletes.
  - `processor/graph-query/README.md:150-152`.
  - `processor/graph-query/MIGRATION.md:259-261`, plus a SemConnect bullet: the `missing` switch at
    `gateway/cs-api/systems.go:896` must accept `damaged`; a semsparql bullet: `sparql/executor.go:1564` still fails
    the query, but with its own unclassified error in place of fatal `graph_state_reset_required`.
  - `pkg/fusion/README.md:114-120`, `pkg/fusion/MIGRATION.md:96-100`, `pkg/fusion/retrieval.go:110-111`.
  - `docs/repository-map.md:91`: a damaged hit no longer refuses a search.
  - `graph/README.md:84-87` (the `TextSearchData` fields), and a `processor/graph-query/MIGRATION.md` bullet for search
    readers: the new `damaged` field, a damaged hit no longer refuses, and semsource's mcp-gateway, which reads only
    `truncated` (`processor/mcp-gateway/graph_matches.go:130`), reads `damaged` to name it.
- [ ] 2.5 Before the PR is marked ready: post on #656 what counts as done, and make the PR title and body match.

## 3. Close

- [ ] 3.1 Codex review of record at the head; `reviewed-by:` in the PR body.
- [ ] 3.2 Archive the change and sync the specs as the last content commit.
