# Inventory: fix-496-key-lister-strands

base: 09733a9e90068b3b7ef050a93d9836cd7a0d4601 (origin/main; claim commit 4b50a09 on top)
nats.go pin: `go.mod:17` — `github.com/nats-io/nats.go v1.55.0`. Source paths below are under
`$GOMODCACHE/github.com/nats-io/nats.go@v1.55.0/`.
Labels: **measured** = a test in the tree drives the pinned library. **read** = source read only.

## 0. Premise: are we trying to out-clever nats here?

No. Both strands are inside nats.go's own `ListKeys` goroutine. A caller that stops reading `Keys()`
before the channel closes cannot free it. nats.go's own `Keys()` (`jetstream/kv.go:1403-1424`) and this
repo's `keysByFilter` both avoid the lister. Avoiding `ListKeys` drops a nats.go helper and adds no
mechanism. How real each trigger is, per site, is in § 1.

## 1. Production call sites of `ListKeys` / `KeyLister`

Search: `git grep -n -e ListKeys -e KeyLister -- '*.go' | grep -v _test.go`. It finds 2 call sites and
3 interface declarations. `ListKeysFiltered` has no production caller.

### a. `pkg/lifecycle/manager_query.go:52` — `lister, err := bucket.ListKeys(ctx)`

- Stop is at `:56` — `defer lister.Stop()`. The loop is `:59` — `for key := range lister.Keys() {`.
  The early return is `:70` — `return nil, fmt.Errorf("lifecycle: List Get %q: %w", key, getErr)`.
  The issue cites `:55`/`:69`; the lines have drifted.
- **The production bucket is the exported `kvcatalog.CatalogReader`.**
  - `pkg/lifecycle/manager.go:209` — `bucket, err := kvcatalog.OpenCatalogReader(ctx, m.natsClient,
    graph.BucketEntityStates)`.
  - `graph/kvcatalog/kvcatalog.go:274-282` declares `Get`, `Watch`, `WatchAll`, `Keys`, `ListKeys`,
    `ListKeysFiltered` and `Status`. It has **no `WatchFiltered`**.
  - `kvcatalog_test.go:28-36` pins that method set.
  - The local interface is `manager.go:24-27` (`ListKeys`, `Watch`). The test double is `fakeBucket`
    (`manager_test.go:276-292`).
  - The bucket is all of `ENTITY_STATES`, not only lifecycle entities.
- Callers: no production caller (`gopls references pkg/lifecycle/manager_query.go:37:20` finds only
  tests). `List` is consumer API.
- **Strands: yes, and real.**
  - **Trigger 1:** an error that is not skipped, while more than 256 initial keys have not yet been sent
    to `List`.
    - The lister goroutine (`kv.go:1441` `go func()`) then blocks for good at `kv.go:1450`
      (`kl.keys <- entry.Key()`). That send is outside the `select` (`kv.go:1445-1454`).
    - `Stop` only unsubscribes (`kv.go:1495-1497`, then `:1236-1241`).
    - If `w.updates` (256) also filled before `Stop`, the delivery callback stays blocked at
      `kv.go:1320` while holding `w.mu` (`kv.go:1308-1309`).
    - Ordinary error sources: `projectTriples` refusing a triple's type (`manager.go:271-274`), and
      `getEntity` failing (`manager.go:262-265`).
  - **Trigger 2** (the deadline ends mid-`List`) mostly reduces to trigger 1: `getWithRevision(ctx, …)`
    returns the context's error, which is not skipped, at `:70`.
- **1b. A shortened list returned with a nil error (read; MEDIUM by #185).** `List` has no completeness
  check after the loop, and the loop ends whenever `kl.keys` closes. That happens on two paths:
  - (i) The lister picks `case <-ctx.Done()` (`kv.go:1451`) first. The loop then drains whatever is
    buffered. If that is nothing, because the buffer is empty at that moment or no buffered key matches
    the workflow (`matchesWorkflow`, `manager_query.go:488`), the loop ends.
  - (ii) The watch channel closes while the context is still live. The lister reads `case entry :=
    <-watcher.Updates():` without the `ok` flag (`kv.go:1446-1449`), so a closed channel reads as the nil
    end marker and the lister returns normally. The channel closes from the sub's closed handler
    (`kv.go:1365-1369`). nats.go runs that handler from `pDone` when the delivery goroutine exits
    (`nats.go:3883-3887`), for example after a connection `Close`, or after a failed ordered-consumer
    reset unsubscribes (`js.go:2365-2367`).
  - Precedent for treating this as a defect: `openspec/specs/transport-client/spec.md:782-784` (#222).
    `keysByFilter` checks `ok` (`natsclient/kv.go:676-682`).
  - `ListKeys` may also report duplicate keys (`kv.go:1431-1432`). Read; minor.

### b. `natsclient/storagereport/storage_report.go:701` — `lister, err := p.store.ListKeys(ctx)`

- Stop is at `:708` — `defer func() { _ = lister.Stop() }()`. The loop at `:713` drains fully, then `:718`
  — `if err := ctx.Err(); err != nil {`.
- **Store types.**
  - The interface is the exported `ReportStore` (`:62-67`), which has no watch method.
  - The production value is `jetstream.KeyValue`: `service/storage_observability.go:340` —
    `bucket jetstream.KeyValue`, passed to `storagereport.NewPublisher(bucket, …)` at `:404-408`.
  - The same package already declares `ReportWatchStore { WatchAll(...) }`
    (`storage_report_consumer.go:51-53`) for the report consumer. That consumer already stops through
    `kvwatch.Stop` (`:231`).
- **Context chain.**
  - `StorageInventoryCollector.Run` (`storage_inventory.go:310-323`) runs `collectAndLog`, which calls
    `publish` (`:348`), then the collector's publisher, then `Publish` (`storage_report.go:226`), then
    `reclaim` (`:316`).
  - The collector's publisher is `lazyReportPublisher` (`service/storage_observability.go:226`, `:374`,
    `:387-395`). It is one path, not two.
  - Publish runs under Run's context. The 15 s timeout (`storage_inventory.go:72`, `:282`) bounds only
    `Collect`.
- **Strands: yes, real but rare and cheap.**
  - Trigger: Run's context ends (service stop) during the listing, while more than 256 entries are still
    to be forwarded.
  - The lister exits at `kv.go:1451-1452`, and its deferred `watcher.Stop` unsubscribes. Nobody reads
    `w.updates` after that, so a delivery blocked at `kv.go:1320` holds `w.mu`, and the closed handler
    (`kv.go:1365-1369`) never runs.
  - It needs more than about 256 report rows (resources plus `_account.tiers`), and a stop that lands in
    a listing window of milliseconds.
  - What strands is `jetstream.(*kvs).WatchFiltered.func1`. It costs nothing at process exit, only a
    stop and restart inside one process.
- **1b(ii) applies here too (read; MEDIUM by #185).** If the channel closes while the context is live,
  the lister returns normally, and `:718` cannot tell that from completion.
  - `reclaim` then builds `stale` from a short listing. That fails open.
  - The likely outcome is that rows are left undeleted until the next collection. The reverse is not
    possible: `stale` holds only keys that were read.
- **Side finding (read):** `:702-704` — `if errors.Is(err, jetstream.ErrNoKeysFound) {` is unreachable.
  `ListKeys` returns only `WatchAll`'s error (`kv.go:1433-1438`). `ErrNoKeysFound` comes only from
  `Keys()` (`kv.go:1421-1423`).

### Declarations only (no call)

- `graph/kvcatalog/kvcatalog.go:279-280`. The doc comment (`:272-273`) says the method set is "the exact
  union consumed by current framework catalog readers".
- `pkg/lifecycle/manager.go:25`.
- `natsclient/storagereport/storage_report.go:67`.

Whether `CatalogReader` must change depends on the shape chosen (§ 3c). A `WatchAll`/`Watch` shape fits
it unchanged. A `WatchFiltered`/`FilteredKeys` shape needs a new exported method on it. If both sites
leave `ListKeys`, `CatalogReader.ListKeys*` loses its last production reader.

## 2. nats.go behaviour at v1.55.0

| Fact | Where | Label |
| --- | --- | --- |
| Watch buffer is 256 | `jetstream/kv.go:1277` — `w := &watcher{updates: make(chan KeyValueEntry, 256)}` | read. The 256 figure is relied on by `graph/follower/follower_integration_test.go:251` (600 puts): **measured** for WatchFiltered |
| Delivery sends under `w.mu` | `kv.go:1308-1309` lock/defer-unlock; send `:1320`; nil marker `:1329` | read |
| The channel closes only from the sub's closed handler, which takes `w.mu` | `kv.go:1365-1369`; run from `pDone` at `nats.go:3883-3887` | read |
| A failed ordered-consumer reset unsubscribes | `js.go:2365-2367` | read |
| `Watcher.Stop` only unsubscribes; a delivery blocked at Stop runs until the buffer is read | `kv.go:1236-1241`; `internal/kvwatch/kvwatch.go:25-27` | **measured**: `TestFollowerRunReturnsAndLeavesNothing` (`graph/follower/follower_integration_test.go:248`) |
| A subscribe context makes nats.go unsubscribe when it ends | `js.go:2073-2078`; option at `kv.go:1348` | read |
| Lister: buffer 256 (`:1439`), goroutine (`:1441`), receive without `ok` (`:1446-1449`), send outside `select` (`:1450`), ctx exit (`:1451`) | `kv.go:1439-1456` | read |
| `KeyLister.Stop` is the inner watcher's Stop, with no drain | `kv.go:1495-1497` | read |
| `WatchAll` = `Watch(">")` = `WatchFiltered([">"])` | `kv.go:1398-1400`, `:1393-1395`; `AllKeys` at `:495` | read |
| nats.go `Keys()` is WatchAll + IgnoreDeletes + MetaOnly read to the marker, then a plain Stop; it ranges `Updates()` | `kv.go:1403-1424` | read |

No test measures the `ListKeys` strand itself. The #461 inventory (sites G and I) is a source read.

## 3. Existing helpers

### a. `internal/kvwatch.Stop` (`internal/kvwatch/kvwatch.go:36-63`)

- It stops the watch, then drains until the channel closes, for at most `DrainBudget` of 5 s (`:18`).
- Production callers: `config/manager.go:610`, `graph/embedding/storage.go:538`, `vector_cache.go:87`,
  `worker.go:322`, `graph/follower/follower.go:254`, `graph/readiness/watcher.go:286`,
  `natsclient/storagereport/storage_report_consumer.go:231`, `pkg/graphview/view.go:253`,
  `pkg/lifecycle/manager_query.go:232`, `service/message_logger_kv_watch.go:177`.
- Both affected packages already import it: `pkg/lifecycle/manager_query.go:19` and
  `natsclient/storagereport/storage_report_consumer.go:35`.

### b. natsclient `keysByFilter` (`natsclient/kv.go:652-720`)

- It sits behind `KVStore.KeysByFilter` (`:616`) and `natsclient.FilteredKeys` (`:629`).
- It reads `WatchFiltered(ctx, []string{pattern}, IgnoreDeletes(), MetaOnly())` (`:653`) to the nil
  marker, and checks `ok` (`:676-682`).
- A context end, or a channel close before the marker, returns no keys and an error (`:667-690`).
- It then stops the watcher and drains for up to 5 s itself (`:697-718`).
- The owner kept that drain on #420 (site J in
  `openspec/changes/archive/2026-11-02-fix-461-kvwatch-adoption/inventory.md:29`).

### c. Is the issue's shape already used, and what does each existing shape cost?

"WatchAll + IgnoreDeletes + MetaOnly, read to the nil marker, then kvwatch.Stop" does not exist
verbatim. Neighbouring shapes, and what each costs at these two sites:

- **`natsclient.FilteredKeys(ctx, r, ">")`.** This is the same server request (`WatchAll` is
  `WatchFiltered([">"])`). The spec already covers it (§ 6), and it checks `ok`. It has no production
  caller; its test callers are `natsclient/kv_filter_test.go:32`, `kv_key_contract_integration_test.go:384`,
  `:676` and `kv_watcher_ownership_test.go:97`. It needs a reader with `WatchFiltered`.
  - **`List`:** `CatalogReader` has none (`kvcatalog.go:274-282`). Using it means either adding
    `WatchFiltered` to the exported `CatalogReader` (and updating its pinned method set,
    `kvcatalog_test.go:28-36`; see #294), or an adapter from `WatchAll` to `WatchFiltered`, which is added
    mechanism. `fakeBucket` changes either way.
    - Not affected: `graph/embedding/memkv_test.go`'s `memKV` is a `jetstream.KeyValue` double that already
      has `WatchFiltered` (`:170-180`). It is not a `CatalogReader` double.
  - **storagereport:** `jetstream.KeyValue` has `WatchFiltered`, so the change is only to widen
    `ReportStore`. The unit fake `fakeReportStore` (`storage_report_test.go:159`) changes.
- **The `WatchAll`/`Watch(">")` loop plus `kvwatch.Stop`** (the issue's shape).
  - **`List`:** it fits `CatalogReader` and the local interface unchanged. `manager.go:26` already has
    `Watch`.
  - **storagereport:** `ReportStore` gains `WatchAll`, the method `ReportWatchStore` already declares
    (`storage_report_consumer.go:51-53`).
  - It is a third key-listing loop beside `keysByFilter` and nats.go `Keys()`. It must check `ok` (1b(ii))
    and the context to avoid 1b.
- **nats.go `Keys()`.** No drain, and it has the 1b(ii) weakness: it ranges until close
  (`kv.go:1415-1420`) and checks no context.
- Precedent for leaving the lister: `FilteredKeys` moved off nats.go `ListKeysFiltered` for exactly 1b
  (`openspec/specs/transport-client/spec.md:782-784`).

### d. Adjacent, not in #496's scope (filed separately as #533)

Production callers of nats.go `Keys()` inherit "a context end or a channel close returns a partial list
with a nil error" (read, `kv.go:1415-1424`): `natsclient/kv.go:579`, `config/key_family.go:213`,
`config/manager.go:1179`, `:1481`, `graph/clustering/storage.go:65`, `graph/embedding/storage.go:464`,
`processor/graph-index/query.go:426`, `service/graph_triples_http.go:256`, `service/message_logger_http.go:392`.

## 4. Open issues and PRs on these files

Searches: `gh issue list --state open --search <q>` for `ListKeys`, `KeyLister`, `storagereport`,
`reclaim`, `kvwatch`, `manager_query`, `storage_report.go`, `lifecycle`, `"pkg/lifecycle"`.

- **#496** — this lane.
- **#502** — names `natsclient/storagereport/storage_report_test.go` (3 `require.Eventually` polls),
  `storage_report_consumer_test.go` and `storage_inventory_test.go`. It overlaps only if the fix edits
  those test files. It is not retired here.
- **#294** — "kvcatalog: merge the write-guard pair when the rule package is ported" (open). It is
  relevant only if a shape changes `CatalogReader`. It is not retired here.
- Name the word, not the package: #281, #196, #29, #109, #24, #129, #10, #9, #7, #81, #300, #20, #195,
  #110, #80.

`gh pr list --state open` lists two PRs:

- **#532** — this claim, draft, 0 files.
- **#530** — draft. It touches `graph/`, `processor/graph-index`, `processor/graph-query` and the
  `setup-04a-04-graph-roots-graph-query-search` change. No shared file and no shared capability.

## 5. Existing tests, and whether a failing test can be written

### What exists

- **`List` unit:** `pkg/lifecycle/manager_test.go:276-292`. The fake lister is a prefilled channel and
  its `Stop` returns nil, so it cannot show a strand.
- **`List` integration:** `pkg/lifecycle/query_transition_integration_test.go:145-185`, plus
  `graph_state_contract_test.go:71`, `:79`. These use small fixtures with no backlog.
- **Reclaim unit:** the fake `ReportStore.ListKeys` (`natsclient/storagereport/storage_report_test.go:159-174`)
  and `TestPublish_ReclaimNeverDeletesTheAccountRow` (`storage_report_account_test.go:120`).
- **Reclaim on real NATS:** `graph/kvcatalog/storage_report_integration_test.go:118`, `:200-202`. It
  reclaims one row, with no backlog and no cancellation.
- **Other test callers of `ListKeys`:**
  - `graph/kvcatalog/storage_report_integration_test.go:34` (`bucket.ListKeys(ctx)`)
  - `graph/kvcatalog/kvcatalog_integration_test.go:98-104` (`ListKeys`/`ListKeysFiltered` plus
    `requireKeyListerStopped` at `:119`)
  - `graph/embedding/memkv_test.go:173`, `:177` (double, `errNotImplemented`)
  - `natsclient/kv_key_contract_integration_test.go:271-613` (nats.go contract tests)
- **No-hook precedent:** a real server, a real backlog of more than 256 entries, and a goroutine-dump
  check through `natsfixture.AwaitNativeDeliveryBlocked`, `AwaitNoStacks` and `NativeDeliveryStacks`
  (`internal/harness/natsfixture/watchdelivery.go:22-63`). Used by
  `graph/follower/follower_integration_test.go:248-310`.

### Whether each trigger can get a failing test

- **`List` trigger 1: can be written, and deterministic (read).** Put about 600 workflow entities into
  `ENTITY_STATES`, with the first by stream order carrying a triple `projectTriples` refuses. Expect
  `List` to return that error. Then check that no stack contains `jetstream.(*kvs).ListKeys.func1` or
  `WatchFiltered.func1`. The trigger is bad data; it needs no hook and no server trick.
- **`List` and storagereport 1b(ii): open question.** It needs the watch channel to close while the
  context is live. A connection `Close` mid-listing has no point to park on without a hook.
  `kv_watcher_ownership_test.go:19-60` models it with a scripted watcher, not nats.go.
- **`List` 1b(i):** it needs the context to end between reads, and there is no park point without a hook.
  Open question.
- **Reclaim strand: open question.**
  - The one seam that is not a hook is the exported `ReportStore`. A test store can embed a real
    `jetstream.KeyValue`, wait for `AwaitNativeDeliveryBlocked`, then cancel and return the handle.
  - Whether the lister then picks `ctx.Done` is random, so the test loops over cycles, as the follower
    test does (`:261-264`).
  - The method it wraps changes with the fix.
  - Open question: does the owner's #301 "no outsmarting nats" ruling (2026-10-27) admit that wrapper?

## 6. Spec requirements on these sites

- **`pkg/lifecycle` `Manager.List`:** none. Search: `git grep -n -i -e 'Manager.List' -e 'lifecycle.List'
  -e '`List`' -- openspec/specs`. Its only hits are graph-view and object-store.
- **Reclaim:** none. `git grep -n -i reclaim -- openspec/specs` finds nothing. The closest is
  `openspec/specs/storage-report/spec.md:64` (Collect and Publish refuse a nil context).
- **The shape in 3b and 3c:** `openspec/specs/transport-client/spec.md:743-791`.
- **Rulings and ADRs:**
  - ADR-112 (`docs/adr/112-natsclient-contract.md:130`, "Anything not backed is not promised").
  - ADR-113 for context ownership. Settled; not an owner question.
  - #420 (`keysByFilter` keeps its own drain).

## 7. Options and recommendation

Held until `INVENTORY PASS` (architect contract, steps 2 and 4).

## Adopter seam (`List` is consumer API; `ReportStore` and `CatalogReader` are exported)

- **What a consumer must know today:**
  - A `List` that errors or whose context ends can leave a goroutine behind. This is not documented
    anywhere.
  - `List` can return a short list with a nil error (1b).
- **If they do nothing:** a silent leak, and a possible silent truncation.
- **Where they find out:** nowhere.
- **What they should have to know:** nothing.
- **Migration notes, not owner questions (owner, 2026-11-02):**
  - A custom `ReportStore` breaks if the interface gains a watch method or drops `ListKeys`.
  - A custom `CatalogReader` implementation breaks only if a shape adds `WatchFiltered` to it.

Skipped:

- No probe of the `ListKeys` strand or of 1b(ii); both are source reads.
- No pin probe, because this change carries no pin package.
- No full read of the #461 archive beyond sites G, I and J.
