# Tasks: fix-461-kvwatch-adoption

Quick track: proposal and tasks; no design document and no spec delta, because no requirement changes. Each test is
shown failing on the base commit in `notes/<task>.md` before its fix. The base's bare `watcher.Stop()` is the mutant,
since issue #129 keeps `task mutate:check` from running an integration test. A real-server test backs the watch up with
`natsfixture.AwaitNativeDeliveryBlocked` and, after the call returns, checks with `natsfixture.AwaitNoStacks` over
`natsfixture.NativeDeliveryStacks`, as `pkg/graphview`'s `TestRunLeavesNoWatchDelivery` does. Each step ends when
`task verify` passes on a clean tree at its commit (#171). One task is one developer's unit.

## 1. Proposal

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and tasks committed; owner approval. Gate: `task spec:check`
  and `task docs:check`.
- [x] 1.2 File one issue (#496) for the key-lister strands (`inventory.md` § 1, sites G and I): its triggers, that
  `pkg/lifecycle.List` has no production caller in this repository (consumer API), and the fix shape
  (`WatchAll(IgnoreDeletes(), MetaOnly())` read to the nil marker, then `kvwatch.Stop`). Link it from PR #494
  without a closing keyword. Gate: `gh issue view <n>` reads back.
- [x] 1.3 Design review of record of the proposal and tasks: no `BLOCKING` finding. Codex APPROVE at `eaa018c`
  (PR #494 comment); its two MEDIUM corrections are applied to proposal and task 2.2.

## 2. Code

In `pkg/lifecycle` and `natsclient/storagereport` (and `service`'s report fake), each task first gives its package's
test watcher one `closeUpdates` helper that closes the channel once per channel; the fake's `Stop` calls it, as
nats.go closes `Updates` after `Stop`, and every test that closes the channel today to simulate a lost watch calls it
instead. Nothing sends on a fake's channel after its `Stop`. So no existing test waits out `kvwatch.DrainBudget` or
closes a channel twice. A real-server test runs 10 cycles, as `TestRunLeavesNoWatchDelivery` does, where one cycle's
base failure is a coin flip.

- [x] 2.1 `service`: a test in `service/message_logger_kv_watch_integration_test.go`, beside
  `TestKVWatchFallingBehindEndsStream`: a watch stream over a bucket holding more keys than both buffers (100 + 256)
  ends by falling behind, the handler returns, and no nats.go watch delivery is left. Client exit is not tested
  apart: it cannot be forced without falling behind. Fails on base; then `readKVWatch` stops through
  `kvwatch.Stop`. Gate: `go test -race -count=1 -tags integration ./service/`; `task verify`.
- [x] 2.2 `pkg/lifecycle`: `lifecycleAtomicWatcher` (`watch_atomic_bootstrap_test.go:30`) gets `closeUpdates`; the
  lost-watch close at `:204` calls it. A test in `pkg/lifecycle/manager_integration_test.go`: `WatchEvents` over more
  than 256 matching entities whose callback returns an error once the delivery is blocked (`manager_query.go:250`; fails
  on base every time) returns an error that `errors.Is` the callback's and leaves no nats.go watch delivery; a second
  case, 10 cycles, ends the context with the callback blocked and checks `errors.Is(err, context.Canceled)`. A unit case
  in `pkg/lifecycle/watch_atomic_bootstrap_test.go`: a scripted watcher whose `Stop` returns a non-benign sentinel and
  whose updates then close; through `Watch` or `WatchEvents`, `errors.Is` finds both the primary error and the sentinel
  (fails on base, which discards it). Then `runWatchLoop` stops through `kvwatch.Stop` and joins its wrapped error.
  Gate: `go test -race -count=1 -tags integration ./pkg/lifecycle/`; `task verify`.
- [x] 2.3 `config`: a test in `config/manager_lifecycle_integration_test.go`, beside
  `TestBlockedKeyFamilyHandlerHoldsStop`, 10 cycles: a key family with more than 256 entries and a blocked handler;
  `Stop` returns nil once the handler is released, and no nats.go watch delivery is left. Fails on base; then
  `stopWatches` takes `release`'s context (`Start`'s on a failed `Start`, `Stop`'s otherwise; ADR-113) and stops each
  watch through `kvwatch.Stop`, still logging at Warn. `TestStopHonoursItsDeadlineWhenAConsumerDeleteIsDenied` and
  `TestManagerPassesTheLifecycleSuite` still pass. Gate: `go test -race -count=1 -tags integration ./config/`;
  `task verify`.
- [x] 2.4 `natsclient/storagereport`: `fakeKeyWatcher` (`storage_report_consumer_test.go:38`) and `service`'s
  `fakeReportWatcher` (`service/storage_observability_http_test.go:51`) get `closeUpdates`; the closes at
  `storage_report_consumer_test.go:314`, `storage_report_resync_test.go:36,84,119,298` and
  `reviewer_storage_snapshot_probe_test.go:33,43` call it. A test in a new
  `storage_report_consumer_integration_test.go` (shared server in `fixture_integration_test.go`), 10 cycles: a
  test-side `ReportWatchStore` passed to `NewConsumer` opens the real watch over more than 256 rows, waits for
  `AwaitNativeDeliveryBlocked`, then waits for `<-ctx.Done()` (the test cancels) before it returns the watcher, so
  the hand-off is ordered after the cancel; `Run` returns, and no nats.go watch
  delivery is left. No production hook. Fails on base; then `watchOnce` stops through `kvwatch.Stop` and logs its
  error at Warn. Gate: `go test -race -count=1 -tags integration ./natsclient/storagereport/ ./service/`;
  `task verify`.
- [x] 2.5 `graph/readiness` (no integration harness in the package): the test runs entirely under `testing/synctest`, so
  `kvwatch.DrainBudget` costs no real time and `fakeWatcher` (`watcher_test.go:56`, sharing one channel per
  `watchSource`, `:85-90`) stays as it is. A unit test in `graph/readiness/watcher_test.go` on a scripted watcher whose
  `Stop` leaves the channel open and whose delivery has a long tail: its buffer full, then many sends (1,000) queued
  behind it, the channel closing only after the last is read (nats.go v1.55.0's shape, `internal/kvwatch` doc). The
  scripted `fakeBucket.watch` waits on `<-ctx.Done()` before it returns the watcher, so ctx has ended before `Run` reads
  (a live ctx would read all entries and pass on base). After ctx ends, `Run` returns only after the scripted delivery
  has ended. On base the random `select` in `watchOnce` reads a handful at most, so it fails every run (on the test's
  own assertion that the delivery ended, `notes/2.5.md`). Then `watchOnce` stops through `kvwatch.Stop`, which makes the
  `Run` doc's "its return is the join" true. Mutation check: `task mutate:check` with `kvwatch.Stop` replaced by
  `watcher.Stop()`, recorded as detections out of 3 mutant runs (3 of 3 expected). Gate: `go test -race -count=1
  ./graph/readiness/`; `task verify`.
- [x] 2.6 Ledger rows of `service`, `pkg/lifecycle`, `config`, `natsclient` (`storagereport`) and `graph/readiness`
  name `internal/kvwatch` (in-set; no exported signature names it) and the stop behaviour. Gate: `task docs:check`;
  `task verify`.
- [x] 2.7 Flake #499, folded in on the owner's word (2026-11-02): `TestSafeConfig_ThreadSafety` waits on its
  `WaitGroup`, not a fixed 10 s. Before and after under the same load: `notes/2.7.md`. Gate: `task verify`.

## 3. Code review

- [x] 3.1 Code review of record by another agent at the head: no actionable findings. The PR body carries
  `implemented-by:` and `reviewed-by:`. Codex APPROVE at `ba40e29`, no actionable findings.

## 4. Archive

- [x] 4.1 Archive as the last content commit (`openspec archive fix-461-kvwatch-adoption --skip-specs`; no spec
  to sync). Gate: `task spec:check` and `task docs:check`.
