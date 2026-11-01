# Tasks: fix-461-kvwatch-adoption

Quick track: proposal and tasks; no design document and no spec delta, because no requirement changes. Each test is
shown failing on the base commit in `notes/<task>.md` before its fix. The base's bare `watcher.Stop()` is the mutant,
since issue #129 keeps `task mutate:check` from running an integration test. A real-server test backs the watch up with
`natsfixture.AwaitNativeDeliveryBlocked` and, after the call returns, checks with `natsfixture.AwaitNoStacks` over
`natsfixture.NativeDeliveryStacks`, as `pkg/graphview`'s `TestRunLeavesNoWatchDelivery` does. Each step ends when
`task verify` passes on a clean tree at its commit (#171). One task is one developer's unit.

## 1. Proposal

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and tasks committed; owner approval. Gate: `task spec:check`
  and `task docs:check`.
  Hold: OPEN-Q proposal awaits owner approval
- [ ] 1.2 File one issue for the key-lister strands (`inventory.md` § 1, sites G and I): its triggers, that
  `pkg/lifecycle.List` has no production caller in this repository (consumer API), and the fix shape
  (`WatchAll(IgnoreDeletes(), MetaOnly())` read to the nil marker, then `kvwatch.Stop`). Link it from PR #494
  without a closing keyword. Gate: `gh issue view <n>` reads back.
- [ ] 1.3 Design review of record of the proposal and tasks: no `BLOCKING` finding.
  Hold: design review

## 2. Code

Each task first makes its package's test watcher close `Updates` after `Stop`, as nats.go does, so no existing test
waits out `kvwatch.DrainBudget`. A real-server test runs 10 cycles, as `TestRunLeavesNoWatchDelivery` does, where one
cycle's base failure is a coin flip.

- [ ] 2.1 `service`: a test in `service/message_logger_kv_watch_integration_test.go`, beside
  `TestKVWatchFallingBehindEndsStream`: a watch stream over a bucket holding more keys than both buffers (100 + 256)
  ends by falling behind, the handler returns, and no nats.go watch delivery is left. Client exit is not tested
  apart: it cannot be forced without falling behind. Fails on base; then `readKVWatch` stops through
  `kvwatch.Stop`. Gate: `go test -race -count=1 -tags integration ./service/`; `task verify`.
- [ ] 2.2 `pkg/lifecycle`: `lifecycleAtomicWatcher` (`watch_atomic_bootstrap_test.go:30`) closes `Updates` on
  `Stop`. A test in `pkg/lifecycle/manager_integration_test.go`: `WatchEvents` over more than 256 matching entities
  whose callback returns an error once the delivery is blocked (`manager_query.go:250`; fails on base every time)
  returns an error that `errors.Is` the callback's and leaves no nats.go watch delivery; a second case, 10 cycles,
  ends the context with the callback blocked and checks `errors.Is(err, context.Canceled)`. Then `runWatchLoop` stops
  through `kvwatch.Stop` and joins its wrapped error. Gate: `go test -race -count=1 -tags integration
  ./pkg/lifecycle/`; `task verify`.
- [ ] 2.3 `config`: a test in `config/manager_lifecycle_integration_test.go`, beside
  `TestBlockedKeyFamilyHandlerHoldsStop`, 10 cycles: a key family with more than 256 entries and a blocked handler;
  `Stop` returns nil once the handler is released, and no nats.go watch delivery is left. Fails on base; then
  `stopWatches` takes `release`'s context (`Start`'s on a failed `Start`, `Stop`'s otherwise; ADR-113) and stops each
  watch through `kvwatch.Stop`, still logging at Warn. `TestStopHonoursItsDeadlineWhenAConsumerDeleteIsDenied` and
  `TestManagerPassesTheLifecycleSuite` still pass. Gate: `go test -race -count=1 -tags integration ./config/`;
  `task verify`.
- [ ] 2.4 `natsclient/storagereport`: `fakeKeyWatcher` (`storage_report_consumer_test.go:38`) and `service`'s
  `fakeReportWatcher` (`service/storage_observability_http_test.go:51`) close `Updates` on `Stop`. A test in a new
  `storage_report_consumer_integration_test.go` (shared server in `fixture_integration_test.go`), 10 cycles: a
  test-side `ReportWatchStore` passed to `NewConsumer` opens the real watch over more than 256 rows and hands it to
  the consumer only after `AwaitNativeDeliveryBlocked`; the context then ends, `Run` returns, and no nats.go watch
  delivery is left. No production hook. Fails on base; then `watchOnce` stops through `kvwatch.Stop` and logs its
  error at Warn. Gate: `go test -race -count=1 -tags integration ./natsclient/storagereport/ ./service/`;
  `task verify`.
- [ ] 2.5 `graph/readiness` (no integration harness in the package): `fakeWatcher` (`watcher_test.go:56`) closes
  `Updates` on `Stop`. A unit test in `graph/readiness/watcher_test.go` under `testing/synctest`, on a scripted
  watcher whose 256-entry buffer is full, whose delivery is blocked on one more send, and whose channel closes only
  after that send is read (nats.go v1.55.0's shape, `internal/kvwatch` doc): `Run` returns only after the scripted
  delivery has ended. Fails on base (synctest reports the blocked goroutine); then `watchOnce` stops through
  `kvwatch.Stop`, which makes the `Run` doc's "its return is the join" true. Mutation check: `task mutate:check` with
  `kvwatch.Stop` replaced by `watcher.Stop()`, a detection. Gate: `go test -race -count=1 ./graph/readiness/`;
  `task verify`.
- [ ] 2.6 Ledger rows of `service`, `pkg/lifecycle`, `config`, `natsclient` (`storagereport`) and `graph/readiness`
  name `internal/kvwatch` (in-set; no exported signature names it) and the stop behaviour. Gate: `task docs:check`;
  `task verify`.

## 3. Code review

- [ ] 3.1 Code review of record by another agent at the head: no actionable findings. The PR body carries
  `implemented-by:` and `reviewed-by:`.
  Hold: code review

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive fix-461-kvwatch-adoption --skip-specs`; no spec
  to sync). Gate: `task spec:check` and `task docs:check`.
