# Tasks: simplify-04a-06-projection

Each task names the outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`; a compile failure on a missing symbol counts as failing.

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and are accepted
  by the owner on #138 (Q1–Q4). Gate: `task spec:check` and `task docs:check`.

## 2. Failing tests

- [ ] 2.1 `TestClassifyMutationErrorMapsEachOutcome` (package `projection_test`, design D1). Gate: fails on the base
  commit (no symbol).
- [ ] 2.2 A "context ended before send" row in `TestTransportOutcomesKeepTheirCommitState` for Create, Append and
  Delete; it asserts `Kind` per Q3. Gate: it passes or fails on the base commit as Q3's answer predicts; record which.
- [ ] 2.3 `TestContractCloneIsIndependent` (D5). Gate: fails on the base commit.
- [ ] 2.4 `TestCreateReplyNamingAnotherEntityIsCommitUnknown` and an append unknown-outcome row (D4). Gate: they pass on
  the base commit and after; one `task mutate:check` run each, against graphmutation's check, shows them able to fail.

## 3. Implementation

- [ ] 3.1 Export `ClassifyMutationError`, which computes `Kind` once; every method uses it. Gate: 2.1 and 2.2 pass.
- [ ] 3.2 `MutationReceipt` without `Commit`; tests in `pkg/projection` and
  `processor/graph-ingest/commit_ambiguity_integration_test.go` read the error's `Commit`. Gate: `task test:unit`;
  `go test -tags integration -race -run 'CommitAmbiguity|CommitUnknown' ./processor/graph-ingest/`.
- [ ] 3.3 Remove the five interfaces and both revive directives. Gate: `task build`, `task lint`.
- [ ] 3.4 Remove the dead layers (D4) and use one nil-client refusal shape. Gate: 2.4 and all existing tests pass.
- [ ] 3.5 `contract.Contract.Clone`; both copies removed (if Q2 is accepted). Gate: 2.3 passes; `task ledger:check`.
- [ ] 3.6 Doc comments declare each method's failure outcomes (D7). Gate: review.

## 4. Documentation

- [ ] 4.1 `docs/admission-ledger.yaml`: the `pkg/projection` contract (the mapping is exported; no commit state on the
  receipt); the `pkg/projection/contract` row `adapt` with current `known_risks` (Q2). Gate: `task ledger:check`,
  `task docs:check`.
- [ ] 4.2 File D-1 (unless Q4 brings it in) and D-2 (`status:blocked`, linked from the PR body without a closing keyword).
  Comment on #257 that `ClassifyMutationError` is the mapping 9b calls. Gate: the issue numbers appear on the PR
  body's follow-up line.

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive the change (`openspec archive simplify-04a-06-projection`) as the last content commit. Gate:
  `task spec:check`.
