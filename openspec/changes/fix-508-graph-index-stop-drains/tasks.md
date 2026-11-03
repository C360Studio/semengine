# Tasks: fix-508-graph-index-stop-drains

Quick track: proposal and tasks; no design document and no spec delta, because no requirement changes. Codex reviews the
code. Each step ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Proposal

- [ ] 1.1 Proposal and tasks committed; owner approval. Gate: `task spec:check` and `task docs:check`.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Code

- [ ] 2.1 `TestStopWithAStuckHandlerStopsEveryOtherVerb` in `processor/graph-index/lifecycle_integration_test.go`,
  failing on the base commit; the failure is recorded in `notes/2.1.md`. Gate: the test fails on base.
- [ ] 2.2 `cleanup` begins every drain and joins their errors. Gate: the test in 2.1 and the package's integration tests
  pass; `task verify` passes on a clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record at the head. The PR body carries `implemented-by:` and `reviewed-by:`.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive`). Gate: `task spec:check` and `task docs:check`.
