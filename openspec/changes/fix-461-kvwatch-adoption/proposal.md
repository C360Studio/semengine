# Proposal: fix-461-kvwatch-adoption

Approved-by:

## Why

Issue #461. At nats.go v1.55.0 a KV watcher's `Stop` only unsubscribes: a delivery already blocked on the watch's full
256-entry update buffer ends only when someone reads the buffer (`internal/kvwatch/kvwatch.go`, re-checked by PR #492).
`kvwatch.Stop` stops the watch and reads it until nats.go closes the channel, bounded by `kvwatch.DrainBudget` (5 s)
for the #454 window. Five callers use it. The inventory (`inventory.md` § 1) checks every other stop site in
production code. Five stop with a bare `Stop` and can return while such a delivery is still blocked, leaving it
running for good:

- `graph/readiness/watcher.go:279` (`Watcher.Run`'s `watchOnce`, on ctx end). Its doc says its return is the join.
- `config/manager.go:601` (`Manager.Stop`'s teardown; the readers return on the run context, not on close).
- `service/message_logger_kv_watch.go:174` (the `kv/{bucket}/watch` reader, on client exit or falling behind).
- `pkg/lifecycle/manager_query.go:224` (`Watch` / `WatchEvents`, on ctx end, callback or decode error).
- `natsclient/storagereport/storage_report_consumer.go:220` (`Consumer.Run`'s `watchOnce`, on ctx end).

These five adopt `kvwatch.Stop`, the smallest change that mirrors what follower, embedding and graphview already
do. Contexts follow ADR-113: each site passes the context it already holds (config's teardown gets `Stop`'s).

These stay as they are, and say so: `processor/graph-ingest/poison_inventory.go:330` (stops after the channel closed)
and `:370` (drains to close under its own 5 s bound, #456); `natsclient/storagereport/storage_report.go:708` (a key
lister read to close); `natsclient/kv.go:699` `keysByFilter` (its own drain, kept by the owner on #420; moving
`natsclient` onto `internal/kvwatch` would add mechanism to a public package, #170).

## What Changes

- Each of the five sites calls `kvwatch.Stop(ctx, watcher)` in place of `watcher.Stop()`. Errors are reported where
  the site reports today: logged by readiness (Debug), config (Warn, as at the pin, ruling Q4), service (Warn) and
  storagereport (Warn, today discarded); joined into the returned error by `pkg/lifecycle`'s `runWatchLoop`, which
  logs nothing. No exported signature changes; no exported signature names `internal/kvwatch`.
- Doc comments of the five functions say the return waits for the watch's delivery to close, for at most
  `kvwatch.DrainBudget`.
- One failing-first test per site proves its caller returns and leaves no nats.go delivery behind with a backlogged
  watch (`tasks.md`).
- Ledger rows of the five packages name the new `internal/kvwatch` dependency.

## Not covered

- `pkg/lifecycle.List` (`manager_query.go:55-69`): an early return stops reading a `KeyLister`, and nats.go's lister
  goroutine can block on its key send for good. Not a `kvwatch.Stop` site (a lister has no `Updates()`); filed as its
  own issue.
- `service/message_logger_kv_watch.go` moving onto `pkg/graphview` stays with #477.
- Folding graph-ingest's guard drain into `kvwatch.Stop` is optional and not done.

## Capabilities

None. No requirement under `openspec/specs/` changes: `config-manager` "Stop is the only stop" and `operator-surface`
"The KV watch ends on client exit, falling behind or Stop" already promise what this makes true.
