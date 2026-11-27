# Tasks: fix-765-warm-reindex

Each code task ends at its own tests passing; one clean-tree `task verify` runs before the push. One pull request,
`Addresses #765` (the 105k watermark plateau stays open on it).

## 1. Proposal

- [ ] 1.1 The owner fills `Approved-by:` in `proposal.md`, ruling on each bullet under "Decisions you are making by
  approving". Hold: OPEN-Q proposal awaits owner approval
- [ ] 1.2 Fable reads `design.md` and `inventory.md` before any code (owner ruling, #765 comment 7196901233): are
  rules 1 to 6 of Decision 1 enough for a restart to trust a complete record, and does the durability point under
  Risks need Decision 3 (b)? Record the read as a PR comment; work its changes into the design before 1.1.

## 2. Code and test

- [ ] 2.1 Failing first, shown red on 3d823c82's code in `notes/2.1.md`: `TestWarmRestartSkipsIndexedEntities`
  counts list and put operations during a warm restart (`kv_operations_total`, `metrics.go`; listings at
  `owner_reconcile.go:46`). Accept: red before 2.3, green after.
- [ ] 2.2 The record bucket: `GRAPH_INDEX_RECORDS` in `graph/constants.go` and the catalog (`kvcatalog.go:169-174`),
  derived, owner graph-index, opened in `openIndexBuckets` (`index.go:54`). Accept: catalog tests pass.
- [ ] 2.3 The record: written per Decision 1 rules 1, 2 and 5, read for rule 3, fallback rule 4; replaces
  `lastProjections` and `computeIndexProjection` (`index.go:498-508`, `:521`), `reindex_events_total` read from it.
  A golden test of key output tied to the format version. Accept: the graph-index spec delta's first two
  requirements' tests pass, `TestRestartConvergesFromEveryCutPoint` recording seed and checks per `docs/testing.md`.
- [ ] 2.4 Rule 6, recreated index bucket. Accept: `TestRestartRebuildsRecreatedIndexBucket`.
- [ ] 2.5 Re-run the #765 probe (branch `claude/probe-765-reconcile-cost`, 24f3cc51) at 40,000 entities, one lane, to
  the end; post time to ready and the per-entity breakdown on #765. Accept: the comment link in `notes/2.5.md`.
- [ ] 2.6 ADR-077 amendment per Decision 4, and the graph-index README's restart section with the record-bucket reset.

## 3. Close

- [ ] 3.1 `task verify` on a clean tree, push, CI green once before merge.
- [ ] 3.2 Archive the change and sync the graph-index spec as the last content commit. Accept: `task spec:check`.
