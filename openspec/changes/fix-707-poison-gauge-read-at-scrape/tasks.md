# Tasks: fix-707-poison-gauge-read-at-scrape

Tests come first. Each code task ends at a clean-tree local `task verify`. Single PR (#713), `Closes #707`.

## 1. Proposal

- [x] 1.1 Proposal (`proposal.md`); `task spec:check` and `task docs:check` pass. Approved 2026-11-22 (#707).

## 2. Code

- [x] 2.1 The gauge becomes a `prometheus.GaugeFunc` over the record, with the same name, help and type;
  `setEntityPoisonGauge`, `poisonGaugeMu` and the test's gauge swap (`poison_scoping_test.go`) go;
  `TestGaugeFollowsConcurrentRecordAndClear` is removed; the `metrics.go` comment on a shared registry is reworded.
  Every existing test that reads the gauge passes unchanged in what it asserts.

## 3. Close-out

- [ ] 3.1 Archive: `openspec archive fix-707-poison-gauge-read-at-scrape --yes`, which syncs
  `graph-ingest-recovery`; `task spec:check` and
  `task docs:check` pass.
