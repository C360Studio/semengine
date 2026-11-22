# Tasks: fix-707-poison-gauge-read-at-scrape

Tests come first. Each code task ends at a clean-tree local `task verify`. Single PR (#713), `Closes #707`.

## 1. Proposal

- [ ] 1.1 Proposal (`proposal.md`); `task spec:check` and `task docs:check` pass.
  Hold: OPEN-Q proposal awaits owner approval (#707)

## 2. Code

- [ ] 2.1 Test first: `TestPoisonGaugeReadsTheRecord` asserts the scraped gauge reads 1, 2, 1, 0 after each
  direct report and clear, with no gauge call. Shown failing against the current written gauge
  (`notes/2.1.md`). The scraped metric is still typed
  gauge, with the same name and help.
- [ ] 2.2 The gauge becomes a `prometheus.GaugeFunc` over the record; `setEntityPoisonGauge`, `poisonGaugeMu` and
  the test's gauge swap go; `TestGaugeFollowsConcurrentRecordAndClear` is removed; the `metrics.go` comment on a
  shared registry is reworded.

## 3. Close-out

- [ ] 3.1 Archive: `openspec archive fix-707-poison-gauge-read-at-scrape --yes`, which syncs
  `graph-ingest-recovery`; `task spec:check` and
  `task docs:check` pass.
