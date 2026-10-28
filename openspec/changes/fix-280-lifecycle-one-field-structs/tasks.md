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

- [ ] 3.1 Codex code review of record on the PR, and `reviewed-by: codex` in the PR body beside
  `implemented-by: claude (opus)`.
  Hold: Codex review, started by the owner.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive`, no spec to sync). Gate: `task spec:check` and
  `task docs:check`.
