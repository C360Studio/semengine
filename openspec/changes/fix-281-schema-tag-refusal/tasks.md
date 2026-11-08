# Tasks: fix-281-schema-tag-refusal

Quick track: proposal and tasks, with no design document and no spec delta, because no requirement changes. Codex
reviews the code. Each step ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Proposal

- [x] 1.1 Proposal and tasks committed (`e688450`); owner approval (2026-11-08, #281 comment 6722207550). Gate:
  `task spec:check` and `task docs:check`.

## 2. Code

- [ ] 2.1 Failing tests: a refused tag at the top level, in a nested object and in an array's item struct panics
  naming the field and the tag (`component`); `StoragePressureThresholds`' schema has its three headroom fields typed
  `float` (`natsclient/storagereport`). Gate: each fails on the base commit.
- [ ] 2.2 The three skip sites panic; the doc comment says so; the three `storagereport` tags use `type:float`.
  `task mutate:check` detects putting back the skip at each site. Gate: the tests in 2.1 pass; `task verify` passes on
  a clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record approves the head, with no BLOCKING or HIGH open. The PR body carries
  `implemented-by:` and `reviewed-by: codex`.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive --skip-specs`). Gate: `task spec:check` and
  `task docs:check`.
