# Proposal: fix-408-revlag-simplify

Approved-by:

## Why

Owner ruling on #408 (comment 6714095087, 2026-11-07): "all three". The owner's framing: the package borders on
clever; simple, idiomatic Go wins, and consumers migrate.

1. `TestWatermark_ConcurrentReadersAndWriters` (`internal/revlag/watermark_test.go:458-525`) can pass without its
   reader seeing any writer progress (#408).
2. `Observe` walks back to place a lower revision (`internal/revlag/watermark.go:122-131`) and re-homes a revision
   observed again (`:112-115`). The only caller observes in delivery order on one goroutine
   (`graph/follower/follower.go:239`, `:264`).
3. The time behind `IndexedAt` and `staleness_ms` is a carried-forward "covered" estimate (`watermark.go:58-71`,
   `:179-206`). It drops times on purpose, so while the index is behind it reads older than its own definition.

Design track: two capabilities (`revision-watermark`, `entity-state-follower`) and an ADR-083 decision 3 amendment.

## What Changes

- `IndexedAt`'s time becomes the commit time of the oldest delivered write not yet applied: the lowest in-flight
  revision. When nothing is in flight, it is the commit time of the revision `Observed` returns. Zero still means
  unknown. The covered estimate and its bookkeeping are removed.
- `Observe` returns an error and changes nothing for a revision at or below `Observed`, revision 0 included. In-flight
  revisions are kept in observation order with no walk-back. Re-observing a revision is no longer supported.
- `graph/follower` declares the feed lost when `Observe` refuses a delivered revision. This fails closed, through the
  existing lost-feed path.
- `TestWatermark_ConcurrentReadersAndWriters` reads at explicit checkpoints, with exact expected floors and times.
  It uses no sleeps and no read counts. It is shown able to fail.
- `TestPropWatermarkHistory` checks against the test's own reference model. `pinref_test.go`, the pin's code, is
  deleted. Retention is proved by live-heap growth, not by reading private fields.
- ADR-083 gets a SemEngine amendment to decision 3, appended to the record; its port note, its SemEngine disposition
  and ADR-109's kept-row are updated in place. Staleness stays reported, never gating (ADR-085 D2).
- Doc comments in `graph/readiness`, `graph` and `graph/follower`, the admission-ledger rows and the repository map
  are updated to the new rule.

## Impact

- `staleness_ms` from graph-index is lower while the index is behind, by up to the time revisions spend in flight.
  It is reported from the first delivery instead of after the first covered completion. Caught up is unchanged
  (0 when ready). Nothing in this repository gates on it. Readers outside the repository see the new meaning; this
  is a migration note, not a question.
- Breaking for `internal/revlag` callers: `Observe` returns an error. There is one caller, in this change.
- Overlap: PR #544 touches `pkg/graphview/view.go` and the ledger, but not the lines this change edits; #482
  (open, unclaimed) edits another bullet of ADR-083's port note. Merge order is free; whichever lands second merges
  `origin/main`.
- Retires #408 (`Closes #408`).
