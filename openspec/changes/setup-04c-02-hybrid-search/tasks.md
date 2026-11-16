# Tasks: setup-04c-02-hybrid-search

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" is failing-first output
in `notes/<task>.md` or `task mutate:check`. Each code task names its pull request (design D9): A, then B.

## 1. Design

- [x] 1.1 Architect inventory and design (`inventory.md`, `design.md`): INVENTORY PASS, then pre-owner design review
  passes; the owner fills `Approved-by:` and rules on design questions 1 to 3 (#668, all three as recommended).

## 2. PR A: the merge

- [x] 2.1 Tests first: `TestMergeRanks` (table plus generated check against a reference model), D1's three integration
  tests, D3's table, `TestSearchGraphAsksBothAtOnce` (D4) and `TestSearchGraphMergedTruncation`, shown failing on
  main (`notes/2.1.md`).
- [x] 2.2 `graph.GlobalSearchData.Merge` (was `Fallback`), `graph.TextSearchMatch.WordRank` / `.VectorRank`, and
  their doc comments; the merge in searchGraph (design D1-D4); `TestLosingTheProviderKeepsWordSearch` updated.
- [x] 2.3 Docs: `processor/graph-query/README.md` ("searchGraph's search by meaning"),
  `processor/graph-query/doc.go:67-73`, `processor/graph-query/MIGRATION.md` (`:30-36`, `:181`, `:329-335`, plus
  the alpha.5 tier 1 line, design Migration Plan), `.agents/skills/query-pattern/SKILL.md:84`,
  `docs/repository-map.md:91`.

## 3. PR B: the tier 1 scorecard

- [ ] 3.0 Open PR B's own draft claim (its own branch and worktree) after PR A merges.
- [ ] 3.1 `natsfixture.New` takes `natsfixture.TB` (design D6, nats-fixture delta); every existing caller unchanged;
  `task verify`.
- [ ] 3.2 Tests first for the tool's flag refusals and tally arithmetic (hand-computed ranks), shown failing; the tool
  joins the tool packages in `scripts/test-pkgs.sh`.
- [ ] 3.3 `internal/harness/scorecard` and `task scorecard:tier1` (design D6, D7); nothing is added to `task verify`;
  `docs/test-infrastructure.md` "When we write harness code" names harness programs run by Task (`pindiff`,
  `mutcheck`, `scorecard`).
- [ ] 3.4 First run against semembed (`Snowflake/snowflake-arctic-embed-s`); output, command and commits in
  `notes/probes/scorecard-<model>-<commit>.md`. Set tier 1's floor from it (design D8), citing that note, and rerun
  to show the verdict passes, and that a floor one point above the measured row exits 1.

## 4. Close-out

- [ ] 4.1 Spec sync and archive as the last content commit; PR B body carries `Closes #668` and `Closes #666`.
