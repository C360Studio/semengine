# Proposal: fix-707-poison-gauge-read-at-scrape

Approved-by: cglusky (2026-11-22) <https://github.com/C360Studio/semengine/issues/707#issuecomment-7078191176> sha256:97846710cdb9

Track: `quick`. Capability modified: `graph-ingest-recovery` ("graph-ingest reports its poisoned entities": one
scenario's proving tests changed; the requirement text is unchanged). One package.

## Why

No test catches graph-ingest's `poisoned_entities` gauge being written out of order (#707). Today graph-ingest
reads the record's size and sets the gauge itself, and a small mutex keeps those in order. A test would have to see
a second writer waiting on that mutex, which Go offers no way to see without a hook in the code or a timing guess
(`testing/synctest` does not count a mutex wait as durably blocked). Testing that a scrape-time gauge calls its
function would test Prometheus, not this code (owner, 2026-11-22).

## What Changes

- The gauge is computed when it is scraped: a `prometheus.GaugeFunc` that returns the damaged-entity record's
  size. graph-ingest no longer writes it, so no write can land out of order, and the mutex and the gauge-setting
  helper go.
- The metric keeps its name, help and gauge type. With one graph-ingest per registry, which is how components are
  built today, operators see no change. With two on one registry, which nothing runs and nothing promises, the
  gauge reads the first instance's record; the comment in `metrics.go` says so.
- No new test. `TestGaugeFollowsConcurrentRecordAndClear`, which can no longer fail, is removed. The existing
  tests that read the gauge after a poisoned read and after its repair (`TestPoisonRepairRecoveryWithoutRestart`,
  the boot-sweep tests) keep proving it is wired to the record. The spec scenario that named the removed test cites
  those and the record's own concurrency test instead.

## Non-goals

No change to `internal/damage`, the follower, or the `-cpu 1` setting of the gates. No other component's gauges:
graph-lexical sets its gauges in the same locked step as the change, so it has no such order to get wrong.

## Impact

`processor/graph-ingest/{metrics.go,poison_inventory.go,component.go}`, `poison_report_test.go`,
`poison_scoping_test.go`; the `graph-ingest-recovery` spec. One PR (#713), `Closes #707`.
