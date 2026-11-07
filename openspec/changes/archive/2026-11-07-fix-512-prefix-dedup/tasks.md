# Tasks: fix-512-prefix-dedup

Quick track (owner, 2026-11-07): proposal, tasks and a spec delta, no design document. Codex reviews the code. Each
step ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Proposal

- [x] 1.1 Proposal, tasks and spec delta committed; owner approval (2026-11-07, #512 comment 6710261026). Gate:
  `task spec:check` and `task docs:check`.

## 2. Code

- [x] 2.1 `TestHandleQueryPrefix_RepeatedListedKeyServedOnce` (`e852e85`) lists one key twice through
  `mockKVBucket.listFilteredFunc` and expects the entity once and `total` counting it once. On base it failed: the
  page held `...002` twice and `total` was 4, not 3. Gate: it fails on the base commit, failure recorded.
- [x] 2.2 The prefix query calls `slices.Compact` after the sort (`7cb76bb`); `task verify` passed on a clean tree at
  `7cb76bb`. Gate: the test in 2.1 and the existing graph-ingest
  tests pass; `task verify` passes on a clean tree.

## 3. Code review

- [x] 3.1 Codex code review of record approves the head, with no BLOCKING or HIGH open: approve at `9f8137b` (PR #543
  comment 6710677483); its one MEDIUM (exported `Total`/`TotalEntities` comments and READMEs still described
  repeats) is fixed after it, and Codex re-reviews that fix. The PR body carries
  `implemented-by:` and `reviewed-by: codex`.

## 4. Archive

- [x] 4.1 Archive as the last content commit (`openspec archive`), with the spec synced. Gate: `task spec:check` and
  `task docs:check`.
