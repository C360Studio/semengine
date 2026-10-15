# Tasks: simplify-04a-04-errs-classify

Each task names the outcome and the gate that proves it. Tests come first and are shown failing on the base commit
before any production line changes; record the failing output in `notes/<task>.md`.

## 1. Design

- [x] 1.1 Inventory, proposal, design and spec deltas reviewed (inventory review, pre-owner design review) and accepted
  by the owner on #167. Gate: `task spec:check` and `task docs:check`.

## 2. Failing tests

- [ ] 2.1 `pkg/errs/errs_test.go` `TestClassify`: a bare `errors.New` is fatal; an error whose text says "timeout" and
  "transient" is fatal; a wrapped transient `ClassifiedError` and `context.DeadlineExceeded` stay transient.
  `natsclient/errors_test.go`: `classForHeader` row `unclassified_defaults_fatal`, and the round trip shows
  `errs.IsFatal` at the requester. Gate: the new rows fail on the base commit; `go test -race ./pkg/errs/...
  ./natsclient/...` fails only on them.
- [ ] 2.2 Commit-unknown is not retryable (design D2). Unit: `pkg/lifecycle/graph_emit_contract_test.go` (faults
  `deadline` and `malformed response`, each operation) and `pkg/projection/commit_ambiguity_test.go` assert `Class`
  fatal, `errs.IsTransient` false, `errs.IsFatal` true, `errors.As` to `*errs.ClassifiedError` yielding fatal with the
  reply's code, the cause still reachable by `errors.Is`, and `Code` unchanged. Not-committed rows keep the reply's
  class. Integration: flip `processor/graph-ingest/commit_ambiguity_integration_test.go` `:135`, `:137-139`, `:249` and
  `:381-383` to expect fatal with the same codes. Gate: the commit-unknown assertions fail on the base commit, the
  integration ones under `go test -tags integration -race -run 'CommitAmbiguity|CommitUnknown'
  ./processor/graph-ingest/`.
- [ ] 2.3 Transient on purpose (design D3): `TestTransportOutcomesKeepTheirCommitState` asserts the no-responders class
  field is transient; a new `pkg/lifecycle` test asserts the retry-exhaustion error of `Transition` and
  `UpdateFromOperator` (`manager.go:656-657`, `:768-769`) is transient by `errs.IsTransient` and `errs.Classify`.
  Gate: the class-field and `errs.Classify` checks pass on the base commit and fail once 3.1 lands alone (record the
  run); the `errs.IsTransient` check fails on the base commit (the error carries no class today) and passes after 3.2.

## 3. Implementation

- [ ] 3.1 `errs.Classify` returns `ErrorFatal` for an unclassified error; `classForHeader`'s default and
  `classifiedAppendFailure`'s starting class are fatal; doc comments say so. Gate: 2.1 passes.
- [ ] 3.2 Commit-unknown `MutationError`s in `pkg/projection` and `pkg/lifecycle` carry `ErrorFatal` and hold the cause
  in a fatal `*errs.ClassifiedError`, with no method added to `MutationError`; `projection`'s no-responders error is
  transient; `lifecycle.ErrUpdateRetriesExhausted` is transient. Gate: 2.2 and 2.3 pass; `task test:unit`,
  `task test:integration` and `task test:repeat`.

## 4. Documentation

- [ ] 4.1 `pkg/errs/doc.go` (unclassified → fatal; `IsFatal` stays strict), `docs/contract.md` (a commit-unknown error
  is fatal, and `errs.IsTransient` reports false for it), the `pkg/errs` row of `docs/admission-ledger.yaml` (contract
  and `known_risks`), and a dated amendment (owner ruling 4 on #167) to ADR-112 § 3 bullets `:144-146` (no
  responders' class) and `:152` (the unknown-error default). Gate: `task docs:check` and `task ledger:check`.
- [ ] 4.2 File every defect this change found and did not fix (protocol File step), linked from the PR body without a
  closing keyword. Filed at design: #257 (lifecycle's `ErrEmitFailed` path, fixed with #138 item a) and #258 (the
  `ErrorClass` zero value). Gate: every such issue number appears in the PR body's follow-up line.

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive the change (`openspec archive simplify-04a-04-errs-classify`) as the last content commit. Gate:
  `task spec:check`.
