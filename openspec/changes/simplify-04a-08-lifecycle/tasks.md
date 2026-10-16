# Tasks: simplify-04a-08-lifecycle

Each task names the outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`; a compile failure on a missing symbol counts as failing.

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and are accepted
  by the owner (owner questions 1–3). Gate: `task spec:check` and `task docs:check`.

## 2. Failing tests

- [ ] 2.1 Emitter: a transient `internal` reply is commit-unknown and fatal for create, reconcile and delete; a
  cancelled context sends nothing and is not committed, unavailable and transient (D1). Gate: fails on the base commit.
- [ ] 2.2 `TestManagerWithoutClientRefusesNATSWork` (D2). Gate: fails on the base commit (messages differ).
- [ ] 2.3 Despawn of a malformed six-part ID is `ErrEntityIDPatternMismatch` with no delete (D4). Gate: fails on base.
- [ ] 2.4 `Complete` follows declared edge order (D5). Gate: fails on base.
- [ ] 2.5 Children's exhaustive paging check, and the read-failure case if owner question 3 is yes (D6). Gate: fails
  on base.
- [ ] 2.6 One refusal test per double (D7). Gate: fails on base.

## 3. Implementation

- [ ] 3.1 Emitter uses `ClassifyMutationError`; `lifecycleMutationOperation` removed; `ErrEmitFailed` doc (D1). Gate:
  2.1 and the existing emitter tests pass.
- [ ] 3.2 One no-client refusal; emitter refusal and dead branches removed (D2). Gate: 2.2.
- [ ] 3.3 One watch loop, no Warn logs; the poison-log test becomes an error assertion (D3). Gate: watch tests pass.
- [ ] 3.4 One matcher (D4). Gate: 2.3; `task lint`.
- [ ] 3.5 Fail doc; Complete by edge order (D5). Gate: 2.4 and the Complete/Fail integration tests.
- [ ] 3.6 Children filters, then pages (D6). Gate: 2.5.
- [ ] 3.7 Doubles refuse; failing tests stamped or re-aimed (D7). Gate: 2.6; `task test:unit`; `task test:integration`.
- [ ] 3.8 Docs and `_ = probe` (D8). Gate: review.

## 4. Documentation

- [ ] 4.1 `docs/admission-ledger.yaml` `pkg/lifecycle` contract: item (4) rewritten (owner question 1); the emitter,
  Complete and Children rules added. Gate: `task ledger:check`, `task docs:check`.
- [ ] 4.2 File D-2 (and D-1 if owner question 3 is no) as `status:blocked`, linked from the PR body without a closing
  keyword. Gate: issue numbers on the PR body's follow-up line, read back.

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive the change (`openspec archive simplify-04a-08-lifecycle`) as the last content commit. Gate:
  `task spec:check`.
