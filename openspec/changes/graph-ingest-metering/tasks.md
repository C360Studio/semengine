# Tasks: graph-ingest-metering

Each task names its outcome and the gate that proves it. Tests come first. Each new test fails by assertion on the
base commit, recorded in `notes/<task>.md`, except where a task says how else it is shown able to fail. Codex review
stops end batches 1–3. Each stop's task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory reviewed (INVENTORY PASS, round 2). Proposal and design pass pre-owner review and are accepted
  by the owner, with owner questions 1–3 answered. Gate: `task spec:check` and `task docs:check`.
  Answered 2026-10-18 (design.md, Owner questions; #300).

## 2. One wrapper (batch 1, #303, #300 B)

- [x] 2.1 Failing test `TestRefusedMutationMetersAndLogsOnce`, calling `meteredCanonicalMutation(route)` (the same
  name and signature on the base and after 2.2). Its revision-mismatch rows fail by assertion on the base with two
  WARN lines. Also `TestMutationOutcomeCountedAtTheDecision`, which passes on the base. Gate: as stated.
- [x] 2.2 D1: one wrapper; outcomes counted at each handler's decision; `meteredMutation`, `recordCanonicalResponse`,
  `canonicalErrorOutcome` and WARN 2 go; the re-decode tests go; `mutation_rejection_metric_test.go` and
  `structural_predicate_gate_test.go` call the one wrapper; the two `task mutate:check` runs D1 names, recorded in
  `notes/2.2.md`. Also: the
  `graph-transport-boundary` MODIFIED delta, and the help text at `component.go:99` naming a count per reply. Gate:
  `git grep -n -w -e meteredMutation -e recordCanonicalResponse -e canonicalErrorOutcome -- '*.go'` returns
  nothing; `go test -race ./processor/graph-ingest/`; `task verify` on a clean tree (review stop).

## 3. Data flow (batch 2, #302)

- [ ] 3.1 Failing test `TestIntegration_DataFlowCountsInputMessages`. Gate: it fails by assertion on the base.
- [ ] 3.2 D2: counts move to the consume callback and the D1 wrapper; the write-site message, byte and last-activity
  increments go; append counts `entities_updated_total`. Also: `TestComponent_DataFlow_ReturnsMetrics` rewritten;
  README § Health gains one paragraph on `DataFlow`; the `graph-ingest-recovery` ADDED delta; the ledger row's
  `proving_tests`. Gate: 3.1 passes; `git grep -n 'messagesProcessed' -- processor/graph-ingest ':!*_test.go'` shows
  only the field, the two increments and the `DataFlow` read; `task verify` on a clean tree (review stop).

## 4. Query codes (batch 3, #300 B; questions 2 and 3)

- [ ] 4.1 Failing test `TestQueryRefusalsCarryACode`. Gate: its uncoded rows fail on the base.
- [ ] 4.2 D3 as the owner answered:
  - every query refusal uses `rejectInvalid`/`rejectInternal`, or a fatal `internal` for the invariant failures;
  - `:118` marshals `graph.EntityBatchResponse{}`;
  - `natsclient.ErrorCodeResponseTooLarge` (if question 3 is answered yes), with the `natsclient` ledger row's
    contract line;
  - the `graph-transport-boundary` ADDED delta (if question 2 is answered yes).

  Gate: `git grep -n 'errs.Classified(' -- processor/graph-ingest/query.go` returns nothing;
  `go test -race ./processor/graph-ingest/ ./natsclient/`; `task verify` on a clean tree (review stop).

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 #300 gets a comment, posted before the merge, that section B is done by PR #307 when it merges and the
  issue stays open. The change is archived (`openspec archive graph-ingest-metering`) as the last content commit.
  Gate: `task spec:check`.
