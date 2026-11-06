# Tasks: fix-521-empty-prefix-page-size

Quick track: proposal and tasks; no design document and no spec delta, because no requirement changes. Codex reviews the
code. Each step ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Proposal

- [ ] 1.1 Proposal and tasks committed; Claude inventory review of the proposal; owner approval. Gate:
  `task spec:check` and `task docs:check`. Hold: OPEN-Q proposal awaits owner approval

## 2. Code

- [x] 2.1 `TestMarshalFittingPrefixPageMeasuresAnEmptyPageAsSent` in
  `processor/graph-ingest/query_prefix_unit_test.go`, failing on the base commit for the nil page (fatal `internal`:
  encoded 27 bytes exceeds max payload 26). Gate: the test fails on base.
- [x] 2.2 `marshalFittingPrefixPage` counts a nil page as `null`. Gate: the test in 2.1 and the existing
  `TestMarshalFittingPrefixPage*` tests pass; `task verify` passed on a clean tree at `0c63487`.

## 3. Code review

- [ ] 3.1 Codex code review of record at the head; the PR body carries `implemented-by: claude (opus, medium)` and
  `reviewed-by: codex`. Hold: waiting on the Codex review, which the owner starts.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive --skip-specs`). Gate: `task spec:check` and
  `task docs:check`.
