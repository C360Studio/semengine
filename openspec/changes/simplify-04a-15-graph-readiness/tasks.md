# Tasks: simplify-04a-15-graph-readiness

Each task names its outcome and the gate that proves it. Tests come first; record in `notes/<task>.md` how each fails
on the base commit, or, for a refactor whose behaviour does not change, which mutant it detects. Review stops (Codex)
after 2.2, 3.3 and 4.2. Each stop's task ends only when `task verify` passes on a clean tree at its commit (#171).
Batch 1 can start on the base; batches 2 and 3 start after this branch merges `origin/main` with #286 in it.

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and are accepted
  by the owner, with owner questions 1 and 2 answered. Gate: `task spec:check` and `task docs:check`.

## 2. The Watcher (batch 1, D2)

- [ ] 2.1 `watcher_test.go` rewritten in `synctest.Test` bubbles against the base production code, using none of
  `withClock`, `withRebindDelay`, `withAppliedHook` and no wall-clock `time.After` guard; `TestWaitForFirst` added.
  Gate: the tests pass on the base production code; `task mutate:check` detects a freshness multiplier of 4 and a
  removed `firstOnce.Do`.
- [ ] 2.2 The hooks removed; `NewWatcher(src, key, heartbeat, logger)`; `Run` refuses a non-positive heartbeat and a
  nil logger (new rows in `TestWatcher_RunValidatesWiring`, failing to compile on the base);
  `freshnessMultiplier` and `freshnessWindow` unexported; `TestFreshnessWindow`'s zero and negative rows
  (`watcher_test.go:630-635`), which test the default D2 removes, go, and its 2-second row stays. Gate: `go test
  -race ./graph/readiness/`; `task cover:check` holds the `graph/readiness` floor; `task verify` passes on a clean
  tree (review stop).

## 3. Gate, publisher, gauges (batch 2, D1, D3, D4, D5)

- [ ] 3.1 Failing tests: `TestNewPublisherRefusesIncompleteWiring` (replaces the base test that pins the no-op),
  `TestReadingZeroValueFailsClosed`, `TestDeferReasonsReturnsACopy`, and the five-family assertion in
  `TestGauges_FullyQualifiedNamesPreserveTheWireContract`. Gate: each fails on the base (compile or assertion, named
  in the note); the five-family assertion fails on the base only to compile, so its note also records that `task
  mutate:check` detects a `Register` that drops one family or builds one collector with another subsystem.
- [ ] 3.2 D1: `EvaluateReadinessGate(Reading) DeferReason`, `StatusReading` removed, `DeferReasons()`, the gate tests
  on one result column. Gate: 3.1's gate tests; `task mutate:check` detects the `!r.Fresh` check removed.
- [ ] 3.3 D3, D4, D5: `NewPublisher` returns an error; no nil-receiver checks; the `timeout` field and `EnsureBucket`
  removed; `NewGauges(subsystem)`, with `ProducerNames`, `MetricNames` and its test removed; the alias and
  `bucket_alias_test.go` removed (each as owner questions 1 and 2 were answered); graph-ingest's `createStatusBucket`,
  `publishReadinessStatus`, `component.go:748-750` and its integration test. Gate: `go test -race ./graph/readiness/
  ./processor/graph-ingest/...`; `task mutate:check` detects a `NewPublisher` that accepts an empty key; `task verify`
  passes on a clean tree (review stop).

## 4. Text and tests (batch 3, D6)

- [ ] 4.1 #132 item i: the four `graph.IndexStatusResponse` round-trip tests and `TestIndexStatusResponseHasNoLegacyFields`
  as one table test in `graph/index_status_test.go`. Gate: `go test ./graph/ ./graph/readiness/`; `task cover:check`.
- [ ] 4.2 The package comment and the comments D6 names; the `graph/readiness` ledger row. Gate: `go doc
  ./graph/readiness` shows the ADR-109 wording; `task verify` passes on a clean tree, which runs `ledger:check` and
  `docs:check` (review stop).

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Comment on #110 for #135 (g) (`Reading.Raw` goes with fusion's port); any follow-up issues filed; then the
  change is archived (`openspec archive simplify-04a-15-graph-readiness`) as the last content commit. Gate:
  `task spec:check`.
