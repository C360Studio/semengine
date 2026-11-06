# Tasks: fix-521-empty-prefix-page-size

Light process, as on #454 and #456: proposal, tasks and spec delta; no design document; Codex reviews the code.

## 1. Proposal

- [ ] 1.1 Proposal, tasks and spec delta committed; Claude inventory review of the proposal. Gate: `task spec:check`
  and `task docs:check`.

## 2. Code

- [ ] 2.1 `TestMarshalFittingPrefixPageMeasuresAnEmptyPageAsSent` in
  `processor/graph-ingest/query_prefix_unit_test.go`, failing on the base commit for the nil page. Gate: the test
  fails on base.
- [ ] 2.2 `marshalFittingPrefixPage` counts a nil page as `null`. Gate: the test in 2.1 and the existing
  `TestMarshalFittingPrefixPage*` tests pass; `task verify` passes on a clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record at the head; the PR body carries `implemented-by: claude (opus, medium)` and
  `reviewed-by: codex`. Hold: waiting on the Codex review, which the owner starts.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive`, syncing the added requirement into
  `openspec/specs/graph-transport-boundary/spec.md`). Gate: `task spec:check` and `task docs:check`.
