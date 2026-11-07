# Tasks: fix-534-list-sort-dedup

Quick track: proposal and tasks, with no design document and no spec delta, because no requirement changes. Codex
reviews the code. Each step ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Proposal

- [x] 1.1 Proposal and tasks committed; owner approval (2026-11-07, #534 comment 6707061949). Gate: `task spec:check`
  and `task docs:check`.

## 2. Code

- [ ] 2.1 A unit test in `pkg/lifecycle/manager_test.go` pages `List` over a snapshot holding a repeated key out of
  key order, and fails on the base commit. Gate: the test fails on base.
- [ ] 2.2 `List` sorts and removes repeated keys before reading entities; doc comments state the order. Gate: the test
  in 2.1 and the existing `pkg/lifecycle` tests pass; `task verify` passes on a clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record approves the head, with no BLOCKING or HIGH open. The PR body carries
  `implemented-by:` and `reviewed-by: codex`.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive --skip-specs`). Gate: `task spec:check` and
  `task docs:check`.
