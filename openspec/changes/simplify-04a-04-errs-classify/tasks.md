# Tasks: simplify-04a-04-errs-classify

Each task names the outcome and the gate that proves it. Tests come first and are shown failing on the base commit
before any production line changes; record the failing output in `notes/<task>.md`.

## 1. Design

- [ ] 1.1 Inventory, proposal, design and spec deltas reviewed (inventory review, pre-owner design review) and accepted
  by the owner on #167. Gate: `task spec:check` and `task docs:check`.

## 2. Failing tests

- [ ] 2.1 `pkg/errs/errs_test.go` `TestClassify`: a bare `errors.New` is fatal; an error whose text says "timeout" and
  "transient" is fatal; a wrapped transient `ClassifiedError` and `context.DeadlineExceeded` stay transient.
  `natsclient/errors_test.go`: `classForHeader` row `unclassified_defaults_fatal`, and the round trip shows
  `errs.IsFatal` at the requester. Gate: the new rows fail on the base commit; `go test -race ./pkg/errs/...
  ./natsclient/...` fails only on them.
- [ ] 2.2 Commit-unknown is not retryable (design D2): `pkg/lifecycle/graph_emit_contract_test.go` (faults `deadline`
  and `malformed response`, each operation) and `pkg/projection/commit_ambiguity_test.go` assert `Class` fatal,
  `errs.IsTransient` false, `errs.IsFatal` true, the cause still reachable by `errors.Is`, and `Code` unchanged.
  Not-committed rows keep the reply's class. Gate: the commit-unknown assertions fail on the base commit.
- [ ] 2.3 Transient on purpose (design D3): `TestTransportOutcomesKeepTheirCommitState` asserts no-responders is
  transient; a new `pkg/lifecycle` test asserts `Update`'s retry-exhaustion error is transient by `errs.IsTransient` and
  `errs.Classify`. Gate: both pass on the base commit and fail once 3.1 lands alone (record the run).

## 3. Implementation

- [ ] 3.1 `errs.Classify` returns `ErrorFatal` for an unclassified error; `classForHeader`'s default and
  `classifiedAppendFailure`'s starting class are fatal; doc comments say so. Gate: 2.1 passes.
- [ ] 3.2 Commit-unknown `MutationError`s in `pkg/projection` and `pkg/lifecycle` carry and present `ErrorFatal`;
  `projection`'s no-responders error is transient; `lifecycle.ErrUpdateRetriesExhausted` is transient. Gate: 2.2 and
  2.3 pass; `task test:unit` and `task test:repeat`.

## 4. Documentation

- [ ] 4.1 `pkg/errs/doc.go` (unclassified → fatal), `docs/contract.md` (a commit-unknown error is fatal, and
  `errs.IsTransient` reports false for it), the `pkg/errs` row of `docs/admission-ledger.yaml` (contract and
  `known_risks`), and ADR-112 § 3 (dated amendment, if the owner agrees on #167). Gate: `task docs:check` and
  `task ledger:check`.
- [ ] 4.2 File every defect this change found and did not fix (protocol File step), and link each from the PR. If the
  owner rules so on #167, that includes a comment on #138 for lifecycle's `ErrEmitFailed` path. Gate: issue numbers in
  a PR comment.

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive the change (`openspec archive simplify-04a-04-errs-classify`) as the last content commit. Gate:
  `task spec:check`.
