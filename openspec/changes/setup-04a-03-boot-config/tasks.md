# Tasks: setup-04a-03-boot-config

Each task names its outcome and the gate that proves it. Tests come first. Record in `notes/<task>.md` how each test
fails on the base commit, or which mutant it detects. Review stops (Codex) come after 2.2 and 3.1. Each stop's task
ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`). Proposal, design and spec delta pass pre-owner review and are
  accepted by the owner, with Q1 and Q2 answered. Gate: `task spec:check` and `task docs:check`.

## 2. Port (D1-D5)

- [x] 2.1 Tests first, in `config/`:
  - the pin's in-scope test files with the module path rewritten, adapted and dropped per D5;
  - `fixture_client_integration_test.go` copied from `graph/kvcatalog`;
  - new tests per D5 (refused keys for both loaders, the `SEMENGINE_` env names, both malformed documents, the
    `LoadFromBytes` round trip);
  - `example_config.json` carried.

  Gate: the tests fail on the base (the package does not exist; note the compile error).
- [x] 2.2 `config.go`, `security.go`, `streams.go` from the pin with the module path rewritten, D1's surface removed,
  and D2 (Q1 ruled yes), D3 (Q2 ruled yes) and the `SEMENGINE_` prefix, which the 2.1 tests require;
  `stream_bounds.go` and `stream_drift.go` byte-equal; the pin's `config/README.md` carried (rewrapped to 120
  columns) as `readme_example_test.go`'s input until 3.1 cuts it. Gate: `go test -race ./config/` and
  `go test -tags integration ./config/` pass; `task verify` passes on a clean tree (review stop).
- [ ] 2.3 Sensitivity for D2, D3 and the prefix. Gate: `task mutate:check` detects each of the five mutants D5 names,
  recorded in `notes/2.3.md`; `task verify` passes on a clean tree; the PR body carries `Closes #74`.

## 3. Text and ledger (D4, D6)

- [ ] 3.1 `config/doc.go` and `config/README.md` per D4; the three ledger rows per D6; the `docs/repository-map.md`
  row and entry count. Gate: `go doc ./config` lists only the kept surface; `task verify` passes on a clean tree,
  which runs `ledger:check` (the two carry rows byte-equal) and `docs:check`; `notes/3.1.md` shows `ledger:check`
  failing on a one-byte edit to each of `config/stream_bounds.go` and `config/stream_drift.go`, then passing (review
  stop).

## 4. Verify, review and close

- [ ] 4.1 `task verify` passes on a clean tree at the implementation head; the coverage of `config` (merged profile)
  is recorded on the PR.
- [ ] 4.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 4.3 Any defect found and not fixed is filed. Then the change is archived
  (`openspec archive setup-04a-03-boot-config`) as the last content commit. Gate: `task spec:check`.
