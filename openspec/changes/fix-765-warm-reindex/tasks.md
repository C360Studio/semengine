# Tasks: fix-765-warm-reindex

Each code task ends at its own tests passing; one clean-tree `task verify` runs before the push. One pull request,
`Addresses #765` (the 105k watermark plateau stays open on it).

## 1. Proposal

- [ ] 1.1 The owner fills `Approved-by:` in `proposal.md`, ruling on each bullet under "Decisions you are making by
  approving". Hold: OPEN-Q proposal awaits owner approval
- [ ] 1.2 Fable reads `design.md` and `inventory.md` before any code (owner ruling, #765 comment 7196901233): is a
  mark naming a fully written revision, with the generations it was written under, enough for a restart to skip the
  entity, or does one of Decision 3's cases need the background check? Record the read as a PR comment; work its
  changes into the design before 1.1.

## 2. Code and test

- [ ] 2.1 Failing first, shown red on 3d823c82's code in `notes/2.1.md`: `TestWarmRestartSkipsIndexedEntities`
  counts list and put operations during a warm restart (`kv_operations_total`; listings at `owner_reconcile.go:46`).
  Accept: red before 2.3, green after.
- [ ] 2.2 The mark bucket: `GRAPH_INDEX_MARKS` in `graph/constants.go` and the catalog (`kvcatalog.go:169-174`),
  derived, owner graph-index, opened in `openIndexBuckets` (`index.go:54`); each bucket's generation read at start
  the way `readStream` does (`graph/follower/follower.go:546`). Accept: catalog tests pass.
- [ ] 2.3 The mark per Decision 1 rules 1 to 4: written after the writes (`index.go:508`), read before the plan is
  applied, the delete mark after a clean delete (`index.go:662`); replaces `lastProjections` and
  `computeIndexProjection` (`index.go:498-508`, `:521`), with `reindex_events_total` read from the mark. Accept: the
  first two requirements' tests. `TestRestartConvergesFromEveryCutPoint` adds no hook to production code. It seeds
  each state a cut write can leave, from a generated history of values in {A, B, C}: any subset of the row changes
  applied, plus the older mark or none, written to NATS with the package's own row and mark builders. It records
  seed and checks per `docs/testing.md`.
- [ ] 2.4 Rule 5, generations. Accept: `TestRestartRebuildsRecreatedIndexBucket`.
- [ ] 2.5 The declared limits and the reset in the graph-index README; an amendment note under ADR-077's port note
  (a matching mark skips the listing). Accept: `TestDeletingMarksRebuildsEveryEntity`.
- [ ] 2.6 Re-run the #765 probe (branch `claude/probe-765-reconcile-cost`, 24f3cc51) at 40,000 entities, one lane, to
  the end; post time to ready and the per-entity breakdown on #765. Accept: the comment link in `notes/2.6.md`.

## 3. Close

- [ ] 3.1 `task verify` passes on a clean tree at the last code commit; the commit is recorded in `notes/3.1.md`.
- [ ] 3.2 Archive the change and sync the graph-index spec as the last content commit. Accept: `task spec:check`.
