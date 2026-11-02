# Inventory: fix-461-kvwatch-adoption

base: `15db0d8` (claim commit on `1d9d4bd`). nats.go pinned at v1.55.0 (`go.mod`).

The question (issue #461, "Ask"): per KV-watch stop site, does it drain `Updates()` until the channel closes, or can it
return while a nats.go delivery blocked on the full 256-entry buffer is still running? The premise is the doc comment
of `internal/kvwatch/kvwatch.go:36` — `func Stop(ctx context.Context, watcher jetstream.KeyWatcher) error {`
(at v1.55.0 `Stop` only unsubscribes; a blocked delivery ends only when the buffer is read), re-checked by PR #492
and driven on a real server by `graph/follower`'s `TestFollowerRunReturnsAndLeavesNothing`.

## 1. Sites (the claimed gap)

Searches: `git grep -nE '\.(Watch|WatchAll|WatchFiltered|ListKeys|ListKeysFiltered)\(' -- '*.go' ':!*_test.go'`;
`git grep -nE 'KeyWatcher|KeyLister|\.Updates\(\)|\.Keys\(\)' -- '*.go' ':!*_test.go'`;
`git grep -nE '\.Stop\(\)' -- '*.go' ':!*_test.go' | grep -iE 'watch|lister|keys'`;
`git grep -nE '\.(History|Keys)\(ctx' -- '*.go' ':!*_test.go'`.

| # | Stop site | Stopped today | Can return with a delivery running? | Disposition |
|---|---|---|---|---|
| A | `graph/readiness/watcher.go:279` — `if stopErr := watcher.Stop(); stopErr != nil {` | deferred bare `Stop` | Yes: `watchOnce` returns on ctx end at `:287` — `return ctx.Err()` — and nothing reads `Updates()` after. Single-key watch, `apply` never blocks: a 256-entry backlog has no named real-world trigger (#185: MEDIUM at most). Doc `:186` — `// before it returns, so its return is the join.` — is false on that path. | adopt |
| B | `config/manager.go:601` — `if err := watcher.Stop(); err != nil {` | bare `Stop` per watch on the teardown goroutine (`:571`), then `cancel()` at `:612` | Yes: after `cancel()` `processWatcher` returns at `:622` and `processFamily` at `config/key_family.go:252`, without reading to close. Trigger: Stop while a key-family handler (consumer code, `family.handle`) is slow with more than 256 family entries pending. | adopt |
| C | `service/message_logger_kv_watch.go:174` — `if err := watcher.Stop(); err != nil {` | deferred bare `Stop` in `readKVWatch` | Yes: returns on client exit at `:184` and on falling behind at `:200`. Trigger: an operator stream over a bucket holding more keys than the two buffers (100 SSE + 256); falling behind means the reader has stopped reading by construction. One stranded delivery per such request. | adopt |
| D | `processor/graph-ingest/poison_inventory.go:330` — `_ = watcher.Stop()` | bare `Stop` after the channel closed (`!ok`) | No: the channel is closed, so the delivery has ended. | already drains |
| E | `processor/graph-ingest/poison_inventory.go:370` — `_ = watcher.Stop()` | `stopEntityStateGuardWatcher` reads `updates` to close at `:378` — `case _, open = <-updates:` — bounded by `guardWatchDrainBudget` (5 s, #456) | No. A second spelling of `kvwatch.Stop` (discard count, own sentinel, spec `graph-ingest-recovery` names its 5 s); consolidating is optional. | already drains; stays |
| F | `pkg/lifecycle/manager_query.go:224` — `defer watcher.Stop()` | deferred bare `Stop` in `runWatchLoop` | Yes: returns on ctx end `:228`, callback error `:250`, decode error `:243`. Trigger: a consumer's `onEvent` error or a bad entity during the bootstrap of a workflow pattern over a large `ENTITY_STATES`. | adopt |
| G | `pkg/lifecycle/manager_query.go:55` — `defer lister.Stop()` | `KeyLister.Stop` = inner `watcher.Stop` (nats.go v1.55.0 `jetstream/kv.go:1495-1497`) | Yes, inside nats.go's lister; the caller cannot drain it (`KeyLister` exposes no `Updates()`; nats.go's goroutine `kv.go:1441-1455` owns the watch), so not a `kvwatch.Stop` site. Two ordinary triggers, HIGH by #185: (1) a `getWithRevision` error that is not skipped, `:69` — `return nil, fmt.Errorf("lifecycle: List Get %q: %w", key, getErr)`; (2) the caller's deadline ending mid-`List`, while nats.go's goroutine waits at `kv.go:1450` — `kl.keys <- entry.Key()`, a send outside its `select`, so `ctx.Done` cannot free it. Either way more than 256 keys are left unread and the goroutine (and the watch delivery behind it) stays for good. `ListKeys` lists the whole bucket. No production caller in this repository (`List` is consumer API). | not this change; filed issue (task 1.2) |
| H | `natsclient/storagereport/storage_report_consumer.go:220` — `defer func() { _ = watcher.Stop() }()` | deferred bare `Stop` in `watchOnce` | Yes: returns on ctx end at `:234`. Trigger: shutdown while the report watch is backlogged (initial values of a large account); weak, MEDIUM at most. | adopt |
| I | `natsclient/storagereport/storage_report.go:708` — `defer func() { _ = lister.Stop() }()` | `KeyLister`, ranged at `:713` — `for key := range lister.Keys() {` — with no early exit | Yes on ctx end, inside nats.go's lister: its goroutine leaves through `case <-ctx.Done(): return` (`kv.go:1451`) and stops reading the watch's updates; a delivery blocked on the full 256-entry buffer holds `w.mu` (`kv.go:1308-1309`, `w.mu.Lock()` / `defer w.mu.Unlock()`), so the channel never closes. Trigger: shutdown during a reclaim over more than about 256 report rows. Not drainable by the caller, so not a `kvwatch.Stop` site. | not this change; same filed issue as G (task 1.2) |
| J | `natsclient/kv.go:699` — `stopErr := watcher.Stop()` (`keysByFilter`) | own 5 s drain to close | No. Owner kept it on #420; moving it onto `internal/kvwatch` adds mechanism to a public package (#170). | non-goal |

Not stop sites: `natsclient/kv.go:729` — `watcher, err := kv.bucket.Watch(ctx, pattern)` (returns the watcher to its
caller, site E); `service/storage_observability.go:371` — `return bucket.WatchAll(ctx, opts...)` (pass-through to site
H); `graph/follower/follower.go:223` — `_ = watcher.Stop()` (after close). nats.go-owned watches: `.Keys(ctx)`
(7 sites) and `.History(ctx` (`natsclient/storagereport/storage_report.go:616`) are read by nats.go to the nil
marker (not to close); no caller-side stop, outside this change.
Already on `kvwatch.Stop`: `graph/follower/follower.go:254`, `pkg/graphview/view.go:253`,
`graph/embedding/vector_cache.go:87`, `graph/embedding/worker.go:322`, `graph/embedding/storage.go:538`.

Sites G and I go to one filed issue, not this change. The fix that covers both is a new listing shape, not a swap:
`WatchAll(IgnoreDeletes(), MetaOnly())` read to the nil marker, then `kvwatch.Stop`, in place of `ListKeys` in two
packages, with its own tests of trigger 1, trigger 2 and the reclaim path. Draining `Keys()` to close would fix only
G's trigger 1. Keeping it out leaves this change one swap per site.

## 2. Spellings of "stop a watch and drain it to close"

`internal/kvwatch/kvwatch.go:36` (5 callers above), site E's `stopEntityStateGuardWatcher`, site J's `keysByFilter`.
Adopters add no spelling.

## 3. Adjacent claims

- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`, 2026-11-01): #494 (this claim, 0 files),
  #495 (`graph-index` #415, 0 files). No overlap today; #495 will touch `processor/graph-index`, not a site here.
- Open issues (`gh issue list --state open --search <q>` for `kvwatch`, `readiness`, `graph/readiness`,
  `message logger`, `message_logger`, `storagereport`, `storage report`, `lifecycle`, `pkg/lifecycle`,
  `manager_query`, `poison_inventory`, `graph-ingest`, `config manager`, `config/manager`, `service/`,
  `watcher.Stop`, `KeyLister`, `ListKeys`): naming a site package: #461 (this); #477 (`pkg/graphview` adoption, names
  site C `service/message_logger_kv_watch.go:137`; would replace C's watch with a shared view that already stops
  through `kvwatch.Stop`; stays, this lands first); #110 (readiness legacy fields), #196 (lifecycleguard adoption,
  `config`/`service`), #195 (natsfixture.Open adoption), #300 (graph-ingest optional items), #109 (follower shell),
  #129 (`mutate:check` cannot run integration tests; bears on proof, below): all stay, unrelated work. The rest name
  the word, not the package (#9 #7 #24 #20 #80 #81 #103 #108 #16 #161 #281 #350 #405 #428 #482 and others).
- Open `class:flake`: none.
- Specs: `config-manager` "Stop is the only stop"; `operator-surface` "The KV watch ends on client exit, falling
  behind or Stop" (the KV watcher SHALL have ended before the handler returns); `lifecycle-suite` line 180 (watchers
  released after Stop). Each already promises what adoption makes true. `graph-readiness`, `storage-report`, and no
  spec for `pkg/lifecycle.Watch`, state nothing about the watch's delivery.
- Ledger rows (`docs/admission-ledger.yaml`): `graph/readiness` (`:2099`, adapt), `config` (`:2800`, adapt),
  `service`, `natsclient` incl. `storagereport` (row at `:848`, adapt), `pkg/lifecycle` (`:2294`, adapt). No `carry` row
  is touched, so `ledger:check` is unaffected.

## 4. Who it is for

No new exported symbol, subject, bucket or config field. New import edge to `internal/kvwatch` from `graph/readiness`,
`config`, `service`, `natsclient/storagereport`, `pkg/lifecycle` (`go list -f '{{.Imports}}'`: readiness and
storagereport import no `internal/` package today; config, service, pkg/lifecycle do). No exported signature names it.

## 5. Problem shape

"Stop a KV watch and wait for its delivery to close, bounded": `internal/kvwatch/kvwatch.go:36`. This change adopts it.

## 6. Pin decision records

No package is carried from the pin; ADR-113 governs context ownership (applied, not raised).

## Adopter seam

Reached from outside: `readiness.Watcher.Run` (no production caller here; kept for ports, `graph-readiness` spec line
7), `config.Manager.Stop`, `kv/{bucket}/watch` over HTTP, `storagereport.Consumer.Run`, `lifecycle.Manager.Watch` /
`WatchEvents` (consumer API). 1. Nothing new to know. 2. Doing nothing now strands no goroutine; a call may take up to
`kvwatch.DrainBudget` (5 s) longer only in the #454 window. 3. n/a. 4. Gap: `Watch`/`WatchEvents` may return an error
joined with `kvwatch.ErrDeliveryOpen`, a sentinel a consumer cannot import (an error naming a framework internal);
`pkg/graphview` set the precedent (spec `graph-view-subscription`, accepted 2026-10-30).
