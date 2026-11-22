# Tasks: fix-657-damaged-read-reports

Tests come first; each is shown failing on `main` before its fix. Each code task ends at a clean-tree local
`task verify`, and adds the `- **AND** the test that verifies this is ...` line to each new scenario it proves.
Single PR, `Closes #657`.

## 1. Proposal

- [x] 1.1 Proposal (`proposal.md`) and spec deltas; `task spec:check` and `task docs:check` pass. Approved 2026-11-22
  (#657).

## 2. Code

- [x] 2.1 Refusal detail: tests that `UnmarshalEntityState`'s refusal carries `Detail["entity_ids"]` [key]; that
  `graph.StateRefusalEntityIDs` returns [B] and [B, C] from graph-ingest's entity verb on damaged B and prefix verb
  over damaged B and C, read through `RequestClassified`, and nil for other errors; and that graph-index `outgoing` on
  damaged B does. Then add the helper; `stateRefusal`, `noncanonicalRefusal`, the follower's fallback refusal and
  `aggregateEntityPoisonError` set the detail; fix the comments at `graph/state_contract.go:62-67` and
  `pkg/errs/errs.go:302-307`.
- [x] 2.2 Value filter: a test with B recorded damaged and then a membership row for B stored (on `main` it answers
  transient `internal`); then the value filter refuses fatal with the IDs it read.
- [x] 2.3 Skipped rows: extend `TestMalformedIndexRowsAreSkippedLoggedAndCounted` to assert each reply's
  `data.unreadable_records` (absent on `main`); then add the field to the seven reply types and count per request.
- [ ] 2.4 Embedding scope: a `FindSimilar` test with two unreadable records, one in scope (on `main` the scoped call
  counts 2); then `graphview.View.ListCounted` takes a key filter and `FindSimilar` passes `keep`.

## 3. Close-out

- [ ] 3.1 Archive: `openspec archive fix-657-damaged-read-reports --yes`, which syncs the five capabilities;
  `task spec:check` and `task docs:check` pass.
