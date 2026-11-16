# Tasks: setup-04c-02-hybrid-search

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" is failing-first output
in `notes/<task>.md` or `task mutate:check`. Each code task names its pull request (design D9): A, then B.

## 1. Design

- [ ] 1.1 Architect inventory and design (`inventory.md`, `design.md`): INVENTORY PASS, then pre-owner design review
  passes; the owner fills `Approved-by:` and rules on design questions 1 to 3.
  Hold: OPEN-Q proposal awaits owner approval

## 2. PR A: the merge

- [ ] 2.1 Tests first: `TestMergeRanks` (table plus generated check against a reference model), D1's three integration
  tests, D3's table, and `TestSearchGraphAsksBothAtOnce` (D4), shown failing on main (`notes/2.1.md`).
- [ ] 2.2 `graph.GlobalSearchData.Merge` (was `Fallback`), `graph.TextSearchMatch.WordRank` / `.VectorRank`, and
  their doc comments; the merge in searchGraph (design D1-D4); `TestLosingTheProviderKeepsWordSearch` updated.
- [ ] 2.3 Docs: graph-query README ("searchGraph's search by meaning"), `doc.go:67-73`, MIGRATION.md
  (`:30-36`, `:181`, `:329-335`).

## 3. PR B: the tier 1 scorecard

- [ ] 3.1 `natsfixture.New` takes a four-method interface; every existing caller unchanged; `task verify`.
- [ ] 3.2 Tests first for the tool's flag refusals and tally arithmetic (hand-computed ranks), shown failing; the tool
  joins the tool packages in `scripts/test-pkgs.sh`.
- [ ] 3.3 `internal/harness/scorecard` and `task scorecard:tier1` (design D6, D7); nothing is added to `task verify`.
- [ ] 3.4 First run against semembed (`Snowflake/snowflake-arctic-embed-s`); output, command and commits in
  `notes/probes/scorecard-<model>-<semembed commit>.md`. Set the floor from it (design D8) and rerun to show the verdict
  passes, and that a floor one point above the measured row fails.

## 4. Close-out

- [ ] 4.1 Spec sync and archive as the last content commit; PR B body carries `Closes #668` and `Closes #666`.
