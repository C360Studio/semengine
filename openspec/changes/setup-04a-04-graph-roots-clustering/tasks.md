# Tasks: setup-04a-04-graph-roots-clustering

Each task names its outcome and the gate that proves it. Tests come first. Each step ends only when `task verify`
passes on a clean tree at its commit (#171). One task is one developer's unit.

## 1. Design and measurement

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass the independent pre-owner review; owner
  acceptance of the rewritten design and its new exported surface (proposal, Impact). Gate: `task spec:check` and
  `task docs:check`.
  Owner accepted 2026-10-31 (proposal `Approved-by:`).
  Codex approved the design at `4cf1520` (PR #483 comment 6544356482); its two MEDIUM wording notes are fixed in the
  next commit.
- [ ] 1.2 The `community-grouping` spec delta (`specs/community-grouping/spec.md`), written to the 2026-10-31
  rulings; each scenario names its test. Gate: `task spec:check`.
- [ ] 1.3 Post the pin probes P1-P4 (inventory, Pin probe) on PR #483 with source, command, versions and output.
  Gate: the comment read back.
- [ ] 1.4 Comment on #68, #141 and #477 that their `graph/clustering` site leaves with the tier-2 lane to #485, no
  closing keyword. Gate: each comment read back.

## 2. Tests first

- [ ] 2.1 Port the pin's storage tests and term-extraction tests off the nil-KV store; tests of surface not carried
  are dropped, each named in `notes/2.1.md`. Gate: in 3.3.
- [ ] 2.2 The unit tests of design D5, each shown failing against a wrong change named in `notes/2.2.md`. Gate: in
  3.3.
- [ ] 2.3 `TestMaintainerMatchesModel` and `TestHistoryOrderIrrelevant` (design D5): seed, run count, completed checks
  in `notes/2.3.md`. Gate: in 3.3.
- [ ] 2.4 `TestTickCost` and the integration tests of design D5 on `natsfixture`. Gate: in 3.3.

## 3. The package

- [ ] 3.1 `types.go`, `storage.go` (design D3, D4). Gate: `go build ./graph/clustering`.
- [ ] 3.2 `terms.go` (design D1). Gate: `go build ./graph/clustering`.
- [ ] 3.3 `maintainer.go` (design D2). Gate: `go test -race -count=1 ./graph/clustering`,
  `go test -count=5 -cpu 1 -shuffle=on ./graph/clustering`, `task test:integration -- ./graph/clustering`.
- [ ] 3.4 Mutation checks of design D5 (`task mutate:check`), in `notes/3.4.md`, survivors reported as survivors.
  Gate: each named test fails on its mutant.

## 4. Ledger, records and docs

- [ ] 4.1 Ledger: `graph/clustering` (`adapt`), the `defer-exclude` rows of design D1 naming #485, `docs/adr/099-…`.
  Gate: `task ledger:check`.
- [ ] 4.2 Re-point the existing `defer-exclude` rows: `graph/embedding/http_embedder.go` to #489
  (`docs/admission-ledger.yaml:3752`, `:3756`) and `graph/query/classifier_llm_adapter.go` to #486 (`:3845`); and the
  `docs/repository-map.md:77-78` rows that name #32 as their home. Gate: `task ledger:check`, `task docs:check`.
- [ ] 4.3 `graph/clustering/doc.go` and `README.md`; ADR-099 with its port note; ADR-085 port note; repository map
  and `docs/adr/` rows; the `entity-or-bucket` skill line; the setup plan's tier-0 line ("community grouping by
  entity-ID prefix", ruling 2026-10-31). Gate: `task docs:check`.
- [ ] 4.4 `graph/clustering unit` in `scripts/cover-check.sh` and `d11Targets` in
  `internal/harness/toolcheck/cover_test.go`; the measured figure on the PR. Gate: `task cover:check`,
  `task test:tools`.

## 5. Evidence and review

- [ ] 5.1 `task verify` on a clean tree at the last commit before the archive commit; notes linked from a PR comment.
- [ ] 5.2 Cross-agent review; `reviewed-by:` in the PR body, which addresses #9 with no closing keyword.

## 6. Close

- [ ] 6.1 Archive (`openspec archive setup-04a-04-graph-roots-clustering`) as the last content commit, syncing
  `community-grouping` into `openspec/specs/`. Gate: `task spec:check` and `task docs:check` on the archive commit.
