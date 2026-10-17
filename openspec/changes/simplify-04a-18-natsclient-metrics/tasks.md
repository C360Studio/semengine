# Tasks: simplify-04a-18-natsclient-metrics

Each task names its outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`; a compile failure on a missing symbol counts as failing.

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec deltas pass pre-owner review and are
  accepted by the owner with rulings on owner questions 1-3. Gate: `task spec:check` and `task docs:check`.
- [x] 1.2 Merge `origin/main` into the claim branch if it moved. Gate: `task verify` on the merge.

## 2. Failing tests

- [ ] 2.1 The refusal-counter tests, with `connectScripted` taking extra options (Design, Tests, D1). Gate: they fail
  on base, where no counter rises.
- [ ] 2.2 The publish oracle, with the not-connected row (D3). Gate: passes on base; detects the mutant that drops the
  `publish_to_stream` count (`task mutate:check`, reported as detected or not).
- [ ] 2.3 The key-listing tables over both entry points, and the empty real-bucket row (D4). Gate: the `FilteredKeys`
  rows fail on base; the empty row passes on base.

## 3. Implementation

- [ ] 3.1 The refusal counter is registered, and `handleError` counts the two refusals before logging (D1, as Q1 is
  ruled). Gate: 2.1 passes; `task test:unit` under `-race`.
- [ ] 3.2 One metrics owner per registry is written in the `WithMetrics` doc comment, the README and
  `processor/graph-ingest/component.go:723-725` and `processor/graph-ingest/README.md:145-146`, with no #75 reference
  left (D2). This task runs after #289 merges and `origin/main` is merged in. Gate:
  `git grep -nE '#75([^0-9]|$)' -- processor natsclient docs/admission-ledger.yaml` is empty; `task docs:check`.
- [ ] 3.3 One publish path; the `AutoCreate` line goes (D3). Gate: 2.2 passes; `task test:integration`.
- [ ] 3.4 One key listing; `collectFilteredKeys`, its tests and the dead branches go (D4). Gate: 2.3 passes;
  `go build ./...`; `task test:integration`.

## 4. Documentation

- [ ] 4.1 Spec delta matches the code; ADR-112 §2 amendment; README and `doc.go` (D5). Gate: `task spec:check`,
  `task docs:check`.
- [ ] 4.2 Ledger natsclient and graph-ingest rows (D5). Gate: `task ledger:check`.
- [ ] 4.3 PR #296's title and body match Q1's ruling and declare `Closes #75 #169 #222`. Gate: read back
  `closingIssuesReferences`.

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive (`openspec archive simplify-04a-18-natsclient-metrics`) as the last content commit. Gate:
  `task spec:check`.
