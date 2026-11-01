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
- [ ] 1.2 File the `pkg/lifecycle.List` key-lister defect (`inventory.md` § 1, site G) as its own issue; link it from
  PR #494 without a closing keyword. Gate: `gh issue view <n>` reads back.
- [ ] 1.3 Design review of record of the proposal and tasks: no `BLOCKING` finding.
  Hold: design review

## 2. Code

- [ ] 2.1 `service`: a test in `service/message_logger_kv_watch_integration_test.go`, beside
  `TestKVWatchFallingBehindEndsStream`: a watch stream over a bucket holding more keys than both buffers ends
  (falling behind, and client exit), the handler returns, and no nats.go watch delivery is left. Fails on base; then
  `readKVWatch` stops through `kvwatch.Stop`. Gate: `go test -race -count=1 -tags integration ./service/`;
  `task verify`.
- [ ] 2.2 `pkg/lifecycle`: a test in `pkg/lifecycle/manager_integration_test.go`: `WatchEvents` over more than 256
  matching entities, its callback blocked, the context ended, returns `context.Canceled` and leaves no nats.go watch
  delivery. Fails on base; then `runWatchLoop` stops through `kvwatch.Stop`, its error joined into the return.
  Gate: `go test -race -count=1 -tags integration ./pkg/lifecycle/`; `task verify`.
- [ ] 2.3 `config`: a test in `config/manager_lifecycle_integration_test.go`, beside
  `TestBlockedKeyFamilyHandlerHoldsStop`: a key family with more than 256 entries and a blocked handler; `Stop`
  returns nil once the handler is released, and no nats.go watch delivery is left. Fails on base; then `stopWatches`
  takes `Stop`'s context and stops each watch through `kvwatch.Stop`, still logging its error at Warn.
  `TestStopHonoursItsDeadlineWhenAConsumerDeleteIsDenied` and `TestManagerPassesTheLifecycleSuite` still pass.
  Gate: `go test -race -count=1 -tags integration ./config/`; `task verify`.
- [ ] 2.4 `natsclient/storagereport`: a test in a new `storage_report_consumer_integration_test.go` (the package's
  shared server is in `fixture_integration_test.go`): `Consumer.Run` over a backlogged report watch returns on context
  end and leaves no nats.go watch delivery. Fails on base; then `watchOnce` stops through `kvwatch.Stop` and logs its
  error at Warn. Gate: `go test -race -count=1 -tags integration ./natsclient/storagereport/`; `task verify`.
- [ ] 2.5 `graph/readiness` (no integration harness in the package): a unit test in `graph/readiness/watcher_test.go`
  under `testing/synctest`, on a scripted watcher whose `Stop` leaves the channel open and whose delivery is blocked
  on a full buffer, closing the channel after that send is read (nats.go v1.55.0's shape, `internal/kvwatch` doc):
  `Run` returns only after the scripted delivery has ended. Fails on base (synctest reports the blocked goroutine);
  then `watchOnce` stops through `kvwatch.Stop`; the `Run` doc comment's "its return is the join" becomes true.
  Mutation check: `task mutate:check` with `kvwatch.Stop` replaced by `watcher.Stop()`, a detection.
  Gate: `go test -race -count=1 ./graph/readiness/`; `task verify`.
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
