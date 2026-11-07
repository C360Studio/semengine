# Tasks: fix-534-list-sort-dedup

Quick track: proposal and tasks, with no design document and no spec delta, because no requirement changes. Codex
reviews the code. Each step ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Proposal

- [x] 1.1 Proposal and tasks committed; owner approval (2026-11-07, #534 comment 6707061949). Gate: `task spec:check`
  and `task docs:check`.

## 2. Code

- [x] 2.1 `TestManager_ListPagesEntityIDOrderEachOnce` in `pkg/lifecycle/manager_test.go` (`946243c`) pages `List`
  over a snapshot holding a repeated key out of key order, and fails on the base commit: with no options `List`
  returns `d b a b c` (the repeat twice, listing order), the `Limit: 2` page `d b` and the `Offset: 2` page `a b`,
  against the oracle `a b c d`. Gate: the test fails on base.
- [x] 2.2 `List` sorts and removes repeated keys before reading entities; doc comments state the order (`5692ecb`).
  `task mutate:check` detects dropping either `sort.Strings` or `slices.Compact`. Gate: the test in 2.1 and the
  existing `pkg/lifecycle` tests pass; `task verify` passed on a clean tree at `5692ecb`.

## 3. Code review

- [x] 3.1 Codex code review of record approves the head, with no BLOCKING or HIGH open: approve at `d3784e6` (PR #539
  comment 6707906068); its one MEDIUM at `bd5c77f` (complexity comments) is fixed in `d3784e6`. The PR body carries
  `implemented-by:` and `reviewed-by: codex`.

## 4. Archive

- [x] 4.1 Archive as the last content commit (`openspec archive --skip-specs`). Gate: `task spec:check` and
  `task docs:check`.
