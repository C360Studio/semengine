# Tasks: simplify-04a-08-lifecycle

Each task names the outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`; a compile failure on a missing symbol counts as failing.

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and are accepted
  by the owner (owner questions 1–3; all yes, #154 comment 6214484329, 2026-10-16). Gate: `task spec:check` and `task docs:check`.

## 2. Failing tests

- [x] 2.1 Emitter: a transient `internal` reply is commit-unknown and fatal for create, reconcile and delete; a
  cancelled context sends nothing and is not committed, unavailable and transient (D1). Gate: fails on the base commit.
- [x] 2.2 `TestManagerWithoutClientRefusesNATSWork` (D2), with every listed call on `NewManager(nil, nil)` and no
  injection. Gate: fails on the base commit (two messages, not one).
- [x] 2.3 Despawn of a malformed six-part ID is `ErrEntityIDPatternMismatch` with no delete (D4). Gate: fails on base
  on the error type only (base sends no delete either).
- [x] 2.4 `Complete` follows declared edge order (D5). Gate: fails on base.
- [ ] 2.5 Children's exhaustive paging check (Offset and Limit -1..7, List's slice rule, non-nil empty result, read
  count within Offset + Limit + dropped), and the read-failure and unregistered-child-workflow cases (owner
  question 3: yes) (D6). Gate: fails on base.
- [ ] 2.6 One refusal test per double (D7). Gate: fails on base.

## 3. Implementation

- [x] 3.1 Emitter uses `ClassifyMutationError`; `lifecycleMutationOperation` removed; `ErrEmitFailed` doc (D1). Gate:
  2.1 and the existing emitter tests pass.
- [x] 3.2 One no-client error at the exact read and `ensureBucket`, keyed on the missing dependency; no emitter
  without a client; emitter refusal and dead branches removed (D2). Gate: 2.2; tests built with `newManagerForTest`
  pass unchanged.
- [x] 3.3 One watch loop, no Warn logs; the poison-log test becomes an error assertion (D3). Gate: watch tests pass.
- [x] 3.4 One matcher (D4). Gate: 2.3; `task lint`.
- [x] 3.5 Fail doc; Complete by edge order; rename the subtest at `query_transition_integration_test.go:347` from
  "the first by name" to "the first declared" (D5). Gate: 2.4 and the Complete/Fail integration tests.
- [ ] 3.6 Children reads until the page is full, with List's skip and slice rules (D6). Gate: 2.5.
- [ ] 3.7 Doubles refuse; failing tests stamped or re-aimed (D7). Gate: 2.6; `task test:unit`; `task test:integration`.
- [ ] 3.8 Docs and `_ = probe` (D8). Gate: review.

## 4. Documentation

- [ ] 4.1 `docs/admission-ledger.yaml` `pkg/lifecycle` contract: item (4) rewritten (owner question 1); the emitter,
  Complete and Children rules added. Merge `origin/main` first if #273 has landed its `component` row. Gate:
  `task ledger:check`, `task docs:check`.
- [ ] 4.2 File D-2 as `status:blocked`, linked from the PR body without a closing
  keyword. Gate: issue numbers on the PR body's follow-up line, read back.

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive the change (`openspec archive simplify-04a-08-lifecycle`) as the last content commit. Gate:
  `task spec:check`.
