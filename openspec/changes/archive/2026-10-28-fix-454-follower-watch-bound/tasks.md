# Tasks: fix-454-follower-watch-bound

Light process, owner 2026-10-28: proposal, tasks and spec delta; no design document; Codex reviews the code.

## 1. Proposal

- [x] 1.1 Proposal, tasks and spec delta committed; Claude inventory review of the proposal. Gate: `task spec:check`
  and `task docs:check`.

## 2. Code

- [x] 2.1 `TestStopWatchGivesUpOnADeliveryThatNeverCloses` in `graph/follower/follower_test.go`, failing on the base
  commit: under `testing/synctest`, `stopWatch` with a cancelled context on a scripted watcher whose update channel
  never closes does not return. Gate: the test fails on base (`go test -overlay` if it cannot compile there).
- [x] 2.2 `stopWatch` drains under `context.WithTimeout(context.WithoutCancel(ctx), budget)` with a named 5 s budget
  and then returns an error saying the delivery did not close after `Stop`; the `stopWatch` and `Run` doc comments
  name the bound. Gate: the test in 2.1 and `TestFollowerRunReturnsAndLeavesNothing` pass; `task verify` passes on
  a clean tree.

## 3. Code review

- [x] 3.1 Codex code review of record: APPROVE at `c1c0015c3655bbbe2b4b7a8403f659cdf312834f`, relayed by the owner
  on 2026-10-28 (Codex's review was not posted on PR #455; the owner's relay is recorded as a PR comment). The PR body
  carries `implemented-by: claude (opus)` and `reviewed-by: codex`.

## 4. Archive

- [x] 4.1 Archive as the last content commit (`openspec archive`, syncing the modified requirement into
  `openspec/specs/entity-state-follower/spec.md`). Gate: `task spec:check` and `task docs:check`.
