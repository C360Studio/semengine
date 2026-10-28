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
- [x] 2.2 `stopEntityStateGuardWatcher` drains under `context.WithTimeout(context.WithoutCancel(ctx), budget)` with a
  named 5 s budget and returns an error saying the delivery did not close after `Stop`; `startEntityStateGuard`
  returns it classified transient; the doc comment names the bound. Gate: the test in 2.1 and the existing
  `TestEntityStateGuard*` tests pass; `task verify` passes on a clean tree.

## 3. Code review

- [x] 3.1 Codex code review of record: no actionable findings at `bfecc0fd45bb8726f982e23173ccdca38a386fc0`
  (PR #459 comment 6474224479). The PR body carries `implemented-by: claude (opus)` and `reviewed-by: codex`.

## 4. Archive

- [x] 4.1 Archive as the last content commit (`openspec archive`, syncing the added requirement into
  `openspec/specs/graph-ingest-recovery/spec.md`). Gate: `task spec:check` and `task docs:check`.
