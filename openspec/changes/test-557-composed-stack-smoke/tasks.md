# Tasks: test-557-composed-stack-smoke

Design track. Tests are the deliverable. Each code task ends at a clean-tree local `task verify` (#171); the
integration tests run under `task test:integration -- ./internal/smoke/`. "Shown to fail" is a failing run or
`task mutate:check` output in `notes/<task>.md`. No `time.Sleep`, no `Skip`, no test deadline of its own, no fixed
port, no `t.Parallel`, no `context.WithValue`.

## 1. Design

- [ ] 1.1 Pre-owner design review passes; the owner fills `Approved-by:` and rules on the proposal's four decisions.
  Hold: OPEN-Q proposal awaits owner approval. Gate: `task spec:check`, `task docs:check`.

## 2. Tests

- [ ] 2.1 `internal/smoke` package, composition root, child helper, fixed inputs, `checkAnswers` and
  `TestSmoke_OneConfigAnswers` (D1 to D5). Shown to fail: graph-embedding disabled in the configuration, and the
  graph-index edge mutant (D7).
- [ ] 2.2 `TestSmoke_StackRestartKeepsAnswers` (case 1); graph-index's reindex counter recorded in the notes (#108).
- [ ] 2.3 `TestSmoke_KillMidIngestKeepsAcknowledgedWrites` (case 2), with the mid-ingest precondition asserted.
  Shown to fail: graph-ingest acknowledging before its commit.
- [ ] 2.4 `TestSmoke_BrokerRestartBetweenRuns` (case 3).
- [ ] 2.5 `TestSmoke_BrokerRestartUnderRunningStack` (case 4). Shown to fail: the follower's stream-creation
  comparison inverted. If it fails on main, the issue filed with its output is linked here (D8).
- [ ] 2.6 Only if 2.5 fails on main and the fix is within one package and smaller than the test (D8): the fix with
  its failing-first package test; otherwise ticked with the reason (passed on main, or the separate PR that merged
  first).

## 3. Follow-ups and docs

- [ ] 3.1 `docs/test-infrastructure.md` names `internal/smoke` and what it proves.
- [ ] 3.2 If #558 is still open: a comment on #558 asking it to add `semantic` and `similar` to `checkAnswers`; if it
  merged first, those checks are in 2.1.
- [ ] 3.3 A follow-up issue for the adopter gap: what a product's own `main` must know (inventory, adopter seam),
  ports (H2) among it.

## 4. Archive

- [ ] 4.1 `openspec archive test-557-composed-stack-smoke` and the `composed-stack` spec sync as the last content
  commit. Gate: `task spec:check`, `task docs:check`.
