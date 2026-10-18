# Tasks: simplify-04a-19-graph-ingest

Each task names its outcome and the gate that proves it. Tests come first; each new test fails by assertion on the base
commit, recorded in `notes/<task>.md`, except where a task says how else the test is shown able to fail. Review stops
(Codex) end batches 2–6; each stop's task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass pre-owner review and are accepted by the
  owner, with owner questions 1 and 2 answered. Gate: `task spec:check` and `task docs:check`.
  Hold: owner answers to design questions 1 and 2.

## 2. Bookkeeping after a failed write (batch 1, #174)

- [ ] 2.1 Failing tests (D1): `TestIntegration_UnconfirmedAppendInvalidatesCache` and
  `TestIntegration_UnconfirmedDeleteClearsPoison` (asserts the poison record is gone, not `Healthy`). Gate: each fails
  by assertion on the base.
- [ ] 2.2 D1: the five writes run the cache invalidation and the poison re-check on an error return. Gate: 2.1 passes;
  `go test -race -tags integration -run 'Unconfirmed|CommitAmbiguity|CommitUnknown|Poison' ./processor/graph-ingest/`;
  `task verify` on a clean tree (review stop).

## 3. ADR-109's follow-up (batch 2, #183, #216 test)

- [ ] 3.1 Failing tests (D2, D3): `TestIntegration_ReadinessEnvelope_BootstrapCompleteWithBacklog`,
  `TestIntegration_HealthReadsLostConsumer`, `TestIntegration_HealthClearsWhenObservationSucceeds`,
  `TestIntegration_HealthIgnoresErrorCount`; `TestIntegration_UnconfirmedDeleteClearsPoison` gains its `Healthy`
  assertion. Gate: each new test or assertion fails by assertion on the base.
- [ ] 3.2 D2 and D3: the latch, `bootBacklogDrained` and the fallback branch go; the latch tests go and the scope tests
  stay; `Health()` reads poison and the latest tick's failed observation; `graph/index_status.go`'s scope and complete
  doc comments, `README.md` § Health, and the `graph-ingest-recovery` delta. Gate: 3.1 passes; `go test -race
  ./processor/graph-ingest/ ./graph/...`.
- [ ] 3.3 D4: `PublishedAt` leaves `graph.IndexStatusResponse` and the publisher; `TestPublishStampsPublishedAt` goes;
  `TestIndexStatusResponseWire` (`graph/index_status_test.go:41`) keeps the key in its legacy input and asserts it is
  dropped; `graph/readiness/watcher.go:22`'s comment; the `graph-transport-boundary` delta (RENAMED and MODIFIED);
  ledger rows `graph` and `graph/readiness`. Gate: `git grep -n -i 'published_at\|PublishedAt' -- '*.go'
  ':!*_test.go'` returns nothing, and in test files only `graph/index_status_test.go` has it; `task verify` on a clean
  tree (review stop).

## 4. Test-only fields and the help text (batch 3, #295, #287, #139 A)

- [ ] 4.1 D5: the synctest loop test, which passes on the base too (the override is unset there); its ability to
  fail is shown instead by `task mutate:check` runs recorded in `notes/4.1.md`, each reported as detected or not: the
  ticker built with a wrong interval, and the first publish before the ticker removed; `statusInterval`, `statusNowFn`
  and their methods go; the five integration tests call
  `refreshReadinessStatus`. Gate: `git grep -n -w -e statusInterval -e statusNowFn` returns nothing;
  `go test -race -tags integration -run 'Readiness' ./processor/graph-ingest/`.
- [ ] 4.2 D7 test-only code and D6: `repopulateHook` and `handleMessage` go, their tests rewritten as D7 says; the
  help text. Gate: `git grep -n -w -e repopulateHook -e handleMessage` returns nothing; `task verify` on a clean tree
  (review stop).

## 5. Rulings and the factory (batch 4, #139 I, J, K, A)

- [ ] 5.1 Ruling K: failing tests first (`{}` and `{"enable_hierarchy":true}` build with the default ports; a present
  `ports` block is used as written; `Validate` leaves the config unchanged), then the change. Gate: the tests fail on
  the base and pass after.
- [ ] 5.2 Ruling I: `handlesMu` goes; `notes/5.2.md` lists every handle access and the goroutine it runs on (D7), and
  records the `-race` run of the lifecycle and failed-start tests as evidence. Gate: `go test -race -count=3 -tags
  integration -run 'Lifecycle|FailedStart|Settlement'
  ./processor/graph-ingest/`.
- [ ] 5.3 Ruling J and row A: the 15 remaining literals build through `CreateGraphIngest`; the nil guards on
  dependencies the factory refuses or always builds go, and `panicTB`; the nil applied-sequence-bucket branch stays.
  Gate: `grep -c '&Component{' processor/graph-ingest/*_test.go` totals 0; `task verify` on a clean tree (review stop).

## 6. Plain functions and text (batch 5, #139 D, F7, I, E, J)

- [ ] 6.1 The four receiver-free methods become functions; `graph.SourceIndexingProfile` at the profile statement; the
  generated-code headers go and what revive then reports is fixed; `metrics_test.go` checks counter deltas and
  `TestComponent_DataFlow_ReturnsMetrics` checks that the rates move. Gate:
  `task lint`; `go test -race ./processor/graph-ingest/`.
- [ ] 6.2 `doc.go`, carried citations, the `platform` comment; the ledger row's `known_risks` and `proving_tests`.
  Gate: `task verify` on a clean tree, which runs `ledger:check` and `docs:check` (review stop).

## 7. Verify, review and close

- [ ] 7.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 7.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 7.3 The follow-up issues D8 names are open (Q1's remainder or none, Q2's fault injector or none, and the two
  defects), each linked from the PR body without a closing keyword; #216 has a comment naming the test; the change is
  archived (`openspec archive simplify-04a-19-graph-ingest`) as the last content commit. Gate: `task spec:check`.
