# Tasks: fix-512-prefix-dedup

Quick track (owner, 2026-11-07): proposal, tasks and a spec delta, no design document. Codex reviews the code. Each
step ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Proposal

- [ ] 1.1 Proposal, tasks and spec delta committed. Hold: OPEN-Q owner approval of the proposal. Gate:
  `task spec:check` and `task docs:check`.

## 2. Code

- [ ] 2.1 A unit test over `mockKVBucket` with a listing repeating one key expects the entity once and `total`
  counting it once. Gate: it fails on the base commit, failure recorded.
- [ ] 2.2 The prefix query calls `slices.Compact` after the sort. Gate: the test in 2.1 and the existing graph-ingest
  tests pass; `task verify` passes on a clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record approves the head, with no BLOCKING or HIGH open. The PR body carries
  `implemented-by:` and `reviewed-by: codex`.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive`), with the spec synced. Gate: `task spec:check` and
  `task docs:check`.
