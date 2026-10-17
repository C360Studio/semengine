# Tasks: simplify-04a-18-natsclient-metrics

Each task names its outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`; a compile failure on a missing symbol counts as failing.

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec deltas pass pre-owner review and are
  accepted by the owner with rulings on owner questions 1-3. Gate: `task spec:check` and `task docs:check`.
- [ ] 1.2 Merge `origin/main` into the claim branch if it moved. Gate: `task verify` on the merge.

## 2. Failing tests

- [ ] 2.1 The refusal-counter tests (Design, Tests, D1). Gate: they fail on base, where no counter rises.
- [ ] 2.2 The publish oracle (D3). Gate: passes on base; detects the mutant that drops the `publish_to_stream` count
  (`task mutate:check`, reported as detected or not).
- [ ] 2.3 The key-listing tables over both entry points (D4). Gate: the `FilteredKeys` rows fail on base.

## 3. Implementation

- [ ] 3.1 `handleError` counts the two refusals (D1). Gate: 2.1 passes; `task test:unit` under `-race`.
- [ ] 3.2 `WithMetrics` doc comment and README state one client per registry (D2). Gate: `task docs:check`.
- [ ] 3.3 One publish path; the `AutoCreate` line goes (D3). Gate: 2.2 passes; `task test:integration`.
- [ ] 3.4 One key listing; `collectFilteredKeys` and its tests go (D4). Gate: 2.3 passes; `go build ./...`;
  `task test:integration`.

## 4. Documentation

- [ ] 4.1 Spec delta matches the code; ADR-112 §2 amendment; README and `doc.go` (D5). Gate: `task spec:check`,
  `task docs:check`.
- [ ] 4.2 Ledger natsclient row (D5). Gate: `task ledger:check`.
- [ ] 4.3 PR #296's title and body match Q1's ruling and declare `Closes #75 #169 #222`. Gate: read back
  `closingIssuesReferences`.

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive (`openspec archive simplify-04a-18-natsclient-metrics`) as the last content commit. Gate:
  `task spec:check`.
