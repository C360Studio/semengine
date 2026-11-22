# Proposal: fix-707-poison-gauge-read-at-scrape

Approved-by:

Track: `quick`. No capability changes: one package, and no spec requirement is added or changed.

## Why

No test catches (#707) graph-ingest's `poisoned_entities` gauge being written out of order. Today a small mutex
orders reading the record's size and setting the gauge, and only that code guards the order. No test can: the test
would have to see a second writer waiting on that mutex, and Go offers no way to see that without a hook in the
code or a timing guess. A test gauge that parks the first `Set` either hangs on the correct code or races on the
broken one. `testing/synctest` does not count a mutex wait as blocked, so `synctest.Wait` never returns there
(probe on go1.26.11: the test timed out). Alerting reads this gauge, so a regression would go unnoticed.

## What Changes

- The gauge is computed when it is scraped: a `prometheus.GaugeFunc` that returns the damaged-entity record's
  size. Nothing writes it, so no write can land out of order, and the mutex and the gauge-setting helper go.
- The metric keeps its name, help and gauge type. With one graph-ingest per registry, which is how components are
  built today, operators see no change.
- With two graph-ingest instances on one registry, which nothing runs and nothing promises, the gauge now reads
  the first instance's record instead of whichever instance wrote last. The comment in `metrics.go` says so.
- A new test changes the record directly, with no gauge call, and asserts the gauge equals the record's size after
  each change. It is shown to fail at `-cpu 1` against the current code, where the gauge is written.
  `TestGaugeFollowsConcurrentRecordAndClear`, which can no longer fail, is removed.

## Non-goals

No change to `internal/damage`, the follower, or the `-cpu 1` setting of the gates. No other component's gauges.

## Impact

`processor/graph-ingest/{metrics.go,poison_inventory.go,component.go}`, `poison_report_test.go`,
`poison_scoping_test.go`. One PR (#713), `Closes #707`.
