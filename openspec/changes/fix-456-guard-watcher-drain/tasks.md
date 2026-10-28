# Tasks: fix-456-guard-watcher-drain

Light process, as on #454: proposal, tasks and spec delta; no design document; Codex reviews the code.

## 1. Proposal

- [x] 1.1 Proposal, tasks and spec delta committed; Claude inventory review of the proposal. Gate: `task spec:check`
  and `task docs:check`.

## 2. Code

- [x] 2.1 `TestEntityStateGuardGivesUpOnADeliveryThatNeverCloses` in
  `processor/graph-ingest/query_contract_guard_test.go`, failing on the base commit: under `testing/synctest`, the
  sweep on a scripted watcher that never closes its channel does not return, after the end-of-snapshot marker or with
  a cancelled context.
  Gate: the test fails on base (`go test -overlay` if it cannot compile there).
- [ ] 2.2 `stopEntityStateGuardWatcher` drains under `context.WithTimeout(context.WithoutCancel(ctx), budget)` with a
  named 5 s budget and returns an error saying the delivery did not close after `Stop`; `startEntityStateGuard`
  returns it classified transient; the doc comment names the bound. Gate: the test in 2.1 and the existing
  `TestEntityStateGuard*` tests pass; `task verify` passes on a clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record; the PR body carries `implemented-by: claude (opus)` and `reviewed-by: codex`.
  Hold: Codex reviews the code (owner starts Codex).

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive`, syncing the added requirement into
  `openspec/specs/graph-ingest-recovery/spec.md`). Gate: `task spec:check` and `task docs:check`.
