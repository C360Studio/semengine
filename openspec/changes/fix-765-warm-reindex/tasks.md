# Tasks: fix-765-warm-reindex

Each code task ends at its own tests passing; one clean-tree `task verify` runs before the push. One pull request,
`Addresses #765` (the 105k watermark plateau stays open on it).

## 1. Proposal

- [ ] 1.1 The owner fills `Approved-by:` in `proposal.md`, ruling on each bullet under "Decisions you are making by
  approving". Hold: OPEN-Q proposal awaits owner approval
- [ ] 1.2 Fable reads the revised `design.md` before any code (owner ruling, PR #774 comment 7217731180): with one
  writer per entity, do rules 2 to 5 keep "a record names every row the entity can own" true at every cut point,
  including a pending put whose outcome is unknown, and do rule 7 and the tombstone record leave any row no later
  apply can reach? Record the read as a PR comment; work its changes into the design before 1.1.

## 2. Code and test

- [ ] 2.1 Failing first, shown red on 3d823c82's code in `notes/2.1.md`: `TestWarmRestartSkipsIndexedEntities` and
  `TestLiveChangeUsesRecord` count list operations (`kv_operations_total`; listings at `owner_reconcile.go:46`).
  Accept: red before 2.3, green after.
- [ ] 2.2 The record bucket: `GRAPH_INDEX_RECORDS` in `graph/constants.go` and the catalog (`kvcatalog.go:169-174`),
  derived, owner graph-index, opened in `openIndexBuckets` (`index.go:54`); each bucket's generation read at start
  the way `readStream` does (`graph/follower/follower.go:546`). Accept: catalog tests pass.
- [ ] 2.3 The record per Decision 1 rules 1 to 6: pending (one union, carried forward) before the first row write,
  clean after the last (`index.go:502-508`); the change and pending paths replace the listing in
  `reconcileOwnedRows` (`owner_reconcile.go:46`) and in `deleteFromIndexes` (`index.go:606`), which stay as the
  fallback, with value-carrying rows put again there (`owner_reconcile.go:85`); the clean empty record after a
  tombstone or a damaged removal (`index.go:480-484`); the encoding and format constant with
  `TestRecordGoldenFixture`; replaces `lastProjections` and `computeIndexProjection` (`index.go:498-508`, `:521`).
  Accept: the first three requirements' tests. Rule 2's write order is proven by
  `TestRecordBucketDeletedWhileRunningWithholds` (no row is written when the pending put fails) and by
  `TestRestartConvergesFromEveryCutPoint` and `TestRecordMatchesListingOracle`, which add no hook to production
  code. The apply builds its ordered operations (record puts, row puts and deletes) in one function; the tests seed
  each cut state by running a prefix of that function's own list against NATS, repeated from the resulting pending
  record for two or more cuts in a row. They run a generated history of values in {A, B, C}, two alias-predicate
  sets, re-offers and deletes, check one golden record fixture, and record seed and checks per `docs/testing.md`.
- [ ] 2.4 Rule 7, too large. Accept: `TestOversizeRecordFallsBackToListing`.
- [ ] 2.5 Generations. Accept: `TestRestartRebuildsRecreatedIndexBucket`.
- [ ] 2.6 The ADR-077 amendment per Decision 2, under its port note: the condition at `:40-42` refuted by the step 2
  measurement, §2 owner listing as fallback and oracle, §4 replacement through the record. Accept: the amendment
  cites the measurement comment and this change.
- [ ] 2.7 The declared limits, the power-cut pointer and the reset in the graph-index README. Accept:
  `TestDeletingRecordsRebuildsEveryEntity`.
- [ ] 2.8 Re-run the #765 probe (branch `claude/probe-765-reconcile-cost`, 24f3cc51) at 40,000 entities, one lane, to
  the end, with a warm restart, an equal re-offer of every entity and 1,000 live changes; post the times and the
  per-entity breakdown on #765. Accept: the comment link in `notes/2.8.md`.

## 3. Close

- [ ] 3.1 `task verify` passes on a clean tree at the last code commit; the commit is recorded in `notes/3.1.md`.
- [ ] 3.2 Archive the change and sync the graph-index spec as the last content commit. Accept: `task spec:check`.
