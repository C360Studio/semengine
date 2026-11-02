# Proposal: fix-461-kvwatch-adoption

Approved-by: owner, 2026-11-01 in Enjoy, proposal at `f8b7ad1` approved as drafted; recorded on #461
(comment 6557756760).

## Why

Issue #461. At nats.go v1.55.0 a KV watcher's `Stop` only unsubscribes: a delivery already blocked on the watch's full
256-entry update buffer ends only when someone reads the buffer (`internal/kvwatch/kvwatch.go`, re-checked by PR #492).
`kvwatch.Stop` stops the watch and reads it until nats.go closes the channel, bounded by `kvwatch.DrainBudget` (5 s)
for the #454 window. Five callers use it. The inventory (`inventory.md` § 1) checks every other stop site in
production code. Five stop with a bare `Stop` and can return while such a delivery is still blocked, leaving it
running for good:

- `graph/readiness/watcher.go:279` (`Watcher.Run`'s `watchOnce`, on ctx end). Its doc says its return is the join;
  adopting, not a doc-only fix, because that would be a new nats.go claim needing a probe.
- `config/manager.go:601` (the teardown `release` runs for `Stop` and for a failed `Start`; readers return on the run
  context, not on close).
- `service/message_logger_kv_watch.go:174` (the `kv/{bucket}/watch` reader, on client exit or falling behind).
- `pkg/lifecycle/manager_query.go:224` (`Watch` / `WatchEvents`, on ctx end, callback or decode error).
- `natsclient/storagereport/storage_report_consumer.go:220` (`Consumer.Run`'s `watchOnce`, on ctx end).

These five adopt `kvwatch.Stop`, the smallest change that mirrors follower, embedding and graphview. Contexts follow
ADR-113: each site passes the context it holds; config's teardown gets `release`'s, which is `Start`'s on a failed
`Start` and `Stop`'s otherwise.

Stay as they are: `processor/graph-ingest/poison_inventory.go:330` (stops after the channel closed) and `:370`
(drains to close under its own 5 s bound, #456); `natsclient/kv.go:699` `keysByFilter` (its own drain, kept by the
owner on #420; moving `natsclient` onto `internal/kvwatch` would add mechanism to a public package, #170).

## What Changes

- Each of the five sites calls `kvwatch.Stop(ctx, watcher)` in place of `watcher.Stop()`. Errors are reported where
  the site reports today: logged by readiness (Debug), config (Warn, as at the pin, ruling Q4), service (Warn) and
  storagereport (Warn, today discarded). `pkg/lifecycle`'s `runWatchLoop`, which logs nothing, joins
  `fmt.Errorf("lifecycle: stop the watch: %w", err)` into its return, as `pkg/graphview` does; the `Watch` and
  `WatchEvents` docs say the error may carry a stop failure. No exported alias of `kvwatch.ErrDeliveryOpen` (#190);
  no exported signature changes or names `internal/kvwatch`.
- Doc comments of the five functions say the return waits for the watch's delivery to close, for at most
  `kvwatch.DrainBudget`. Test watchers close `Updates` once, after `Stop`, as nats.go does.
- One failing-first test per site (`tasks.md`). At `service`, `pkg/lifecycle`, `config` and `storagereport` a
  real-server test proves the caller returns and leaves no nats.go delivery behind with a backlogged watch. At
  `graph/readiness` a scripted watcher under `synctest` proves only that `Run` stops through the drain and waits for it;
  that nats.go needs the drain rests on `internal/kvwatch`'s own real-server evidence, not on this test. Ledger rows of
  the five packages name the new `internal/kvwatch` dependency.

## Not covered

- `pkg/lifecycle.List` (`manager_query.go:55-69`) and storagereport's reclaim (`storage_report.go:708`): nats.go's key
  lister can strand its goroutine or the watch delivery behind it (inventory sites G and I). A lister has no
  `Updates()`, so neither is a `kvwatch.Stop` swap; the fix is a new listing shape, filed as its own issue.
- `service/message_logger_kv_watch.go` moving onto `pkg/graphview` stays with #477.
- Folding graph-ingest's guard drain into `kvwatch.Stop` is optional and not done.

## Capabilities

None. No requirement under `openspec/specs/` changes: `config-manager` "Stop is the only stop" and `operator-surface`
"The KV watch ends on client exit, falling behind or Stop" already promise what this makes true. One evidence
paragraph in `operator-surface` changes by hand: it said the backlog was never probed, and `service`'s new test now
measures it (Codex review, PR #494).
