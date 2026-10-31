# Tasks: setup-04a-04-graph-roots-clustering

Each task names its outcome and the gate that proves it. Tests come first. Each step ends only when `task verify`
passes on a clean tree at its commit (#171). One task is one developer's unit.

## 1. Design and measurement

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass the independent pre-owner review; owner
  acceptance, including the proposal's three questions. Gate: `task spec:check` and `task docs:check`.
  Hold: OPEN-Q proposal awaits owner approval
- [ ] 1.2 The `community-detection` spec delta (`specs/community-detection/spec.md`), staged against option B and
  rewritten if the owner rules otherwise on proposal question 1; each scenario names its test. Gate:
  `task spec:check`.
- [ ] 1.3 Post the pin probes P1-P4 (inventory, Pin probe) on PR #483 with source, command, versions and output.
  Gate: the comment read back.
- [ ] 1.4 Comment on #32, #68, #141 and #477 that their `graph/clustering` site is excluded with the tier-2 lane, no
  closing keyword. Gate: each comment read back.

## 2. Tests first

- [ ] 2.1 Port the pin's LPA, summarizer and PageRank unit tests onto a fake `CommunityStorage` (no nil-KV store);
  tests of dropped surface rewritten on kept surface or dropped, each named in `notes/2.1.md`. Gate: in 3.3.
- [ ] 2.2 New unit tests of design D6 (`TestDetectRefusesNilContext`, `TestOneLevel`, `TestSummaryDeterministic`),
  each shown failing against the pin's shape. Gate: in 3.3.
- [ ] 2.3 `TestDetectionMatchesModel` (design D6): seed, run count, completed checks in `notes/2.3.md`. Gate: in 3.3.
- [ ] 2.4 Integration tests on `natsfixture`: the pin's storage tests, `TestGetAllCommunitiesFailsOnCorruptEntry`,
  `TestPruneReportsCancel`. Gate: in 3.3.

## 3. The package

- [ ] 3.1 `types.go`, `storage.go` (design D3). Gate: `go build ./graph/clustering`.
- [ ] 3.2 `summarizer.go` tier-0 half and `pagerank.go` (design D4). Gate: `go build ./graph/clustering`.
- [ ] 3.3 `lpa.go` (design D2). Gate: `go test -race -count=1 ./graph/clustering`,
  `go test -count=5 -cpu 1 -shuffle=on ./graph/clustering`, `task test:integration -- ./graph/clustering`.
- [ ] 3.4 Mutation checks of design D6 (`task mutate:check`), in `notes/3.4.md`, survivors reported as survivors.
  Gate: each named test fails on its mutant.

## 4. Ledger, records and docs

- [ ] 4.1 Ledger: `graph/clustering` (`adapt`), the `defer-exclude` rows of design D1, `docs/adr/099-…`. Gate:
  `task ledger:check`.
- [ ] 4.2 `graph/clustering/doc.go` and `README.md`; ADR-099 with its port note; ADR-085 port note; repository map
  and `docs/adr/` rows; the `entity-or-bucket` skill line. Gate: `task docs:check`.
- [ ] 4.3 `graph/clustering unit` in `scripts/cover-check.sh` and `d11Targets` in
  `internal/harness/toolcheck/cover_test.go`; the measured figure on the PR. Gate: `task cover:check`,
  `task test:tools`.

## 5. Evidence and review

- [ ] 5.1 `task verify` on a clean tree at the last commit before the archive commit; notes linked from a PR comment.
- [ ] 5.2 Cross-agent review; `reviewed-by:` in the PR body, which addresses #9 with no closing keyword.

## 6. Close

- [ ] 6.1 Archive (`openspec archive setup-04a-04-graph-roots-clustering`) as the last content commit, syncing
  `community-detection` into `openspec/specs/`. Gate: `task spec:check` and `task docs:check` on the archive commit.
