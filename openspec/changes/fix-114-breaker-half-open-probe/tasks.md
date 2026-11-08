# Tasks: fix-114-breaker-half-open-probe

Quick track: proposal and tasks, with no design document and no spec delta, because no requirement changes. Codex
reviews the code. Each step ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Proposal

- [ ] 1.1 Proposal and tasks committed; owner approval of the lost-probe rule. Hold: OPEN-Q, owner approval in
  `proposal.md`. Gate: `task spec:check` and `task docs:check`.

## 2. Code

- [ ] 2.1 Failing tests in `model/breaker_test.go` for each case the proposal lists. Gate: each fails on the base
  commit.
- [ ] 2.2 `IsHealthy` reserves the probe, `EndpointStatus` reserves nothing, the lost-probe rule, doc comments, ledger
  row. Gate: the tests in 2.1 pass under `-race`; `task verify` passes on a clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record approves the head, with no BLOCKING or HIGH open. The PR body carries
  `implemented-by:` and `reviewed-by: codex`.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive --skip-specs`). Gate: `task spec:check` and
  `task docs:check`.
