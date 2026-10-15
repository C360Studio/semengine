# Tasks: simplify-04a-06-projection

Each task names the outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`; a compile failure on a missing symbol counts as failing.

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and are accepted
  by the owner on #138 (owner questions 1–3; answered 2026-10-15, #138 comment 6201479528: keep the names, add
  `Clone` here, fix #267 here). Gate: `task spec:check` and `task docs:check`.

## 2. Failing tests

- [x] 2.1 `TestClassifyMutationErrorMapsEachOutcome` (package `projection_test`, design D1), including rows for
  natsclient's `ErrNotConnected` and `ErrCircuitOpen` wrapped as transport `commit_unknown` (they stay unknown until
  #268). Gate: fails on the base commit (no symbol).
- [x] 2.2 A "context ended before send" row in `TestTransportOutcomesKeepTheirCommitState` for Create, Append and
  Delete: not committed, `MutationUnavailable`, transient, `errors.Is(err, context.Canceled)`. Gate: it passes on
  the base commit (today's behaviour, now pinned) and fails under a mutant that maps `deadline` to unknown
  (`task mutate:check`).
- [ ] 2.3 `TestContractCloneIsIndependent` (D5). Gate: fails on the base commit.
- [ ] 2.4 `TestCreateReplyNamingAnotherEntityIsCommitUnknown` and an append unknown-outcome row (D4). Gate: they pass on
  the base commit and after; one `task mutate:check` run each, against graphmutation's check, shows them able to fail.
- [x] 2.5 A `Reconcile` whose read fails with `nats.ErrTimeout` or
  `natsclient.ErrNotConnected` reads not committed, sends no mutation, and reports operation `read-authoritative`
  (extends `TestReconcileReadFailureIsNotCommitted`). Gate: fails on the base commit.

## 3. Implementation

- [x] 3.1 Export `ClassifyMutationError`, which computes `Kind` once; every write uses it; `MutationUnavailable`'s doc
  says "not delivered". Gate: 2.1 and 2.2 pass.
- [ ] 3.2 `MutationReceipt` without `Commit`; tests in `pkg/projection` and
  `processor/graph-ingest/commit_ambiguity_integration_test.go` read the error's `Commit`. Gate: `task test:unit`;
  `go test -tags integration -race -run 'CommitAmbiguity|CommitUnknown' ./processor/graph-ingest/`.
- [ ] 3.3 Remove the five interfaces and both revive directives. Gate: `task build`, `task lint`.
- [ ] 3.4 Remove the dead layers (D4) and use one nil-client refusal shape. Gate: 2.4 and all existing tests pass.
- [ ] 3.5 `contract.Contract.Clone`; both copies removed (owner question 2: accepted). Gate: 2.3 passes;
  `task ledger:check`.
- [ ] 3.6 Doc comments declare each method's failure outcomes (D7). Gate: review.
- [x] 3.7 Every `ReadAuthoritative` failure is not committed (#267). Gate:
  2.5 passes.

## 4. Documentation

- [ ] 4.1 `docs/admission-ledger.yaml`: the `pkg/projection` contract (the mapping is exported; no commit state on the
  receipt); the `pkg/projection/contract` row `adapt` with current `known_risks` (owner question 2). Gate:
  `task ledger:check`, `task docs:check`.
- [ ] 4.2 Link #268 from the PR body without a closing keyword (filed `status:blocked`); the PR body closes #267.
  Comment on #257 with the two lifecycle items #138 item 4 leaves for 9b once this PR closes #138: a context that
  ended before sending, which lifecycle reports commit-unknown (`graph_emit.go:108-115`), and
  `lifecycleMutationOperation`'s `default:` (`graph_emit.go:125-126`), which should return an error, not
  `MutationOperationAppend`. Name `ClassifyMutationError` as the mapping 9b calls. Gate: the issue numbers appear on
  the PR body's follow-up line, and the #257 comment is read back.

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive the change (`openspec archive simplify-04a-06-projection`) as the last content commit. Gate:
  `task spec:check`.
