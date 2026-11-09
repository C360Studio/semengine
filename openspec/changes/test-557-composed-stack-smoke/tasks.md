# Tasks: test-557-composed-stack-smoke

Design track. Tests are the deliverable. Each code task ends at a clean-tree local `task verify` (#171); the
integration tests run under `task test:integration -- ./internal/smoke/`. "Shown to fail" is a mutant applied in a
scratch copy and its failing integration run, recorded verbatim with the patch in `notes/<task>.md` (design D7;
`task mutate:check` runs no integration test). No `time.Sleep`, no `Skip`, no test deadline of its own, no fixed
port, no `t.Parallel`, no `context.WithValue`.

## 1. Design

- [x] 1.1 Pre-owner design review passes; the owner fills `Approved-by:` and rules on the proposal's five decisions.
  Gate: `task spec:check`, `task docs:check`.

## 2. Tests

- [x] 2.1 `internal/smoke` package, composition root, child helper, fixed inputs, `checkAnswers` and
  `TestSmoke_OneConfigAnswers` (D1 to D5), with every wait ordered after its write and `localSearch` checked as its
  refusal. Shown to fail: graph-embedding disabled in the configuration, and the graph-index edge mutant (D7).
- [x] 2.2 `TestSmoke_StackRestartKeepsAnswers` (case 1), with a new entity and an edit after the restart;
  graph-index's reindex counter recorded in the notes (#108).
- [ ] 2.3 `TestSmoke_KillMidIngestKeepsAcknowledgedWrites` (case 2): hold on one fixed entity, published last, with an
  appending statement; ack floor at its sequence minus 1 with exactly one pending, and the absent applied-sequence
  record, asserted before the kill. Shown to fail: graph-ingest's applied-sequence guard off.
- [ ] 2.4 `TestSmoke_BrokerRestartBetweenRuns` (case 3), URL unchanged across `Restart`, new writes after.
- [ ] 2.5 `TestSmoke_BrokerRestartUnderRunningStack` (case 4). Shown to fail: the follower's stream-creation
  comparison inverted. If it fails on main, the issue filed with its output is linked here (D8); if the fix is a
  separate PR, this task carries `Hold: #<that PR>` until it merges.
- [ ] 2.6 Only if 2.5 fails on main and the fix is within one package and smaller than the test (D8): the fix with
  its failing-first package test; otherwise ticked with the reason (passed on main, or the separate PR that merged
  first).

## 3. Follow-ups and docs

- [ ] 3.1 `docs/test-infrastructure.md` names `internal/smoke` and what it proves.
- [ ] 3.2 If #558 is still open: a comment on #558 asking it to add `semantic` and `similar` to `checkAnswers`; if it
  merged first, those checks are in 2.1.
- [ ] 3.3 A follow-up issue for the adopter gap: what a product's own `main` must know (inventory, adopter seam),
  ports (H2) among it.
- [ ] 3.4 An issue for `config.Manager`'s watch ending silently (`config/manager.go:633-635`), which no check can
  observe.

## 4. Archive

- [ ] 4.1 `openspec archive test-557-composed-stack-smoke` and the `composed-stack` spec sync as the last content
  commit. Gate: `task spec:check`, `task docs:check`.
