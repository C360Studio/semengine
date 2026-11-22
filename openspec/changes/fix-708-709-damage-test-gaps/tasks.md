# Tasks: fix-708-709-damage-test-gaps

Each code task ends at a clean-tree local `task verify`. Single PR, closing #708 and #709.

## 1. Proposal

- [x] 1.1 Proposal (`proposal.md`); `task spec:check` and `task docs:check` pass. Approved 2026-11-21 (#708).

## 2. Tests

- [x] 2.1 #708: `TestHealthCountMatchesNamedIDsWhileChanging` runs its writer and reader in parallel under `-cpu 1`.
  Shown failing at `-cpu 1` against the two-read Health code (`notes/2.1.md`).
- [x] 2.2 #709: a same-key concurrent report/clear test in `internal/damage/record_test.go`, under `-race`. Shown
  failing against the two mutants named in the proposal (`notes/2.2.md`).

## 3. Close-out

- [ ] 3.1 Archive: `openspec archive fix-708-709-damage-test-gaps --yes`; `task spec:check` and `task docs:check`
  pass.
