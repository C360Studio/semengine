# Tasks: fix-280-lifecycle-one-field-structs

Light process, owner 2026-10-28: proposal and tasks; no design document, no spec delta; Codex reviews the code.
The change moves no behaviour, so no new test comes first: the package's existing unit and integration tests, which
drive `Create`, `CreateFromOperator`, `TransitionWith` and `DespawnWith`, are the proof that nothing moved.

## 1. Proposal

- [x] 1.1 Proposal and tasks committed. Gate: `task spec:check` and `task docs:check`.

## 2. Code

- [x] 2.1 `createOutcome` and `transitionOutcome` deleted from `pkg/lifecycle/manager.go`; `createWithSource`,
  `createWithRegistration` and `transitionWithOutcome` return their plain values, and their two readers read them.
  Gate: `git grep -n 'createOutcome\|transitionOutcome' -- '*.go'` prints nothing; `go test -race -count=1
  -tags integration ./pkg/lifecycle/` passes; `task verify` passes on a clean tree.

## 3. Code review

- [x] 3.1 Codex code review of record on PR #440: no actionable findings at
  `5124bd7b7e2307d6094b59134a6c2677e866879e` (PR comment 6467722237, 2026-10-28T11:24:15Z). The PR body carries
  `implemented-by: claude (opus)` and `reviewed-by: codex`.

## 4. Archive

- [x] 4.1 Archive as the last content commit (`openspec archive`, no spec to sync). Gate: `task spec:check` and
  `task docs:check`.
