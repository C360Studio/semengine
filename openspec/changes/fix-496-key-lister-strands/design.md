# Design: fix-496-key-lister-strands

Base `098fdbd`. Evidence: `inventory.md` (INVENTORY PASS, round 2). Binding:

- ADR-112 and owner #170: natsclient passes nats.go through and gains no mechanism.
- ADR-113: context ownership. Each site passes the context it holds: `List` the caller's, `reclaim` the
  collector's `Run` context.
- #301: no fault rebuilds of nats.go.
- Owner 2026-11-02: a consumer break is a migration note.

**Goal.** `List` and `reclaim` return either a complete key listing or an error. Neither leaves a nats.go
goroutine behind.

**Non-goals:** #533 (callers of `Keys()`), a narrower `List` scan, de-duplicating keys.

## D1. The listing

| Option | Cost |
| --- | --- |
| (a) Do nothing | Leaves a real `List` trigger (any read error) and silent short lists at both sites |
| (b) Drain `lister.Keys()` before returning | Fixes only `List`'s early return. On ctx end the lister still leaves the delivery blocked (`kv.go:1451`), and 1b stays: the lister reads `Updates()` without `ok` (`kv.go:1446-1449`) |
| (c) A `WatchAll` loop plus `kvwatch.Stop` at each site (the issue's shape) | Fits `CatalogReader` unchanged. Adds a third and fourth spelling of the snapshot loop beside `keysByFilter` and nats.go `Keys()`; each copy must check `ok` and ctx, with its own tests |
| (d) A shared `WatchAll` listing in `internal/kvwatch` | One copy, but a second spelling of `keysByFilter` (`natsclient/kv.go:652-720`), which the owner kept as the one listing (#420) |
| **(e) `natsclient.FilteredKeys(ctx, r, ">")`** | No new loop. Behaviour already specified and tested (`transport-client/spec.md:743-791`). Costs one exported method on `CatalogReader` and on `ReportStore` (D2) |

**(e).** `WatchAll` is `WatchFiltered([">"])` (`kv.go:1398-1400`, `:1393-1395`), so the server request does not
change. Each site already gathers every key before acting:

- `reclaim` drains fully before deleting (`storage_report.go:710-716`).
- `List` already costs O(N) per call (its doc).

So the returned slice changes neither memory nor order of work. `List` now reads entities after the watch has
stopped. A listing cut short is wrapped as `lifecycle: List keys for workflow %q: %w` or `list published report
keys: %w`, and `errors.Is` still finds the context's error.

## D2. Surface changes and migration

Each new method's reader is a production caller in this change, so it is plumbing as well as framework API.

- **`kvcatalog.CatalogReader`** (`graph/kvcatalog/kvcatalog.go:274-282`):
  - Adds `WatchFiltered`. The private `catalogReader` embeds the interface (`:287-289`), so it delegates with no
    new code.
  - Drops `ListKeys` and `ListKeysFiltered`. Basis: a `KeyLister` handed out by an exported reader is the #496
    strand for any consumer that stops reading early, and after this change no framework code reads it.
    Optional cleanup, not required by #496: the owner rules on it in task 1.1. If kept, only `WatchFiltered` is
    added. (The doc's "exact union" (`:272-273`) is already false on base: `ListKeysFiltered` has no reader.)
  - `TestCatalogReader_ExportedSurfaceIsReadOnly` (`kvcatalog_test.go:20-37`) changes with it.
  - Reader: `pkg/lifecycle.List`.
  - Migration: a consumer calling `ListKeys` on a `CatalogReader` calls `natsclient.FilteredKeys(ctx, r, ">")`
    (or a pattern). A custom implementation adds `WatchFiltered`.
  - #294 (the write-guard pair) is untouched.
- **`storagereport.ReportStore`** (`storage_report.go:62-67`): `ListKeys` becomes `WatchFiltered`.
  - The production value is `jetstream.KeyValue` (`service/storage_observability.go:340`, `:404-408`), which has it.
  - Migration: a custom store implements `WatchFiltered` in place of `ListKeys`.
- **`pkg/lifecycle`**: the private `entityStatesReader` (`manager.go:24-27`) makes the same swap. No exported
  signature changes. `List`'s doc says it returns an error when the listing does not complete.

## D2a. Why a new capability

No spec owns `Manager.List`. `graph-entity-writes` binds `pkg/lifecycle`'s `Watch`/`WatchEvents` only to the
entity decode rule (`openspec/specs/graph-entity-writes/spec.md:397`); a listing's completeness is not a write or
decode rule, so it goes in its own `lifecycle-query` capability.

## D3. Behaviour and proving tests

Each test passes `-race`. Shown able to fail means: real-server tests fail on the base commit, recorded in
`notes/<task>.md`; unit tests are checked by `task mutate:check`, because #129 keeps it off integration tests.

1. **`List` with a read error leaves nothing running.**
   - `TestListLeavesNoListerAfterAReadError` in `pkg/lifecycle/manager_integration_test.go`.
   - `ReadExactEntity` is an RPC to graph-ingest (`graph/exact_entity.go:80`), so the test starts graph-ingest
     first (`startGraphIngest`, `harness_gate_integration_test.go:86-116`), then puts the entities raw.
   - 600 entities of the workflow are put raw into `ENTITY_STATES`. The first by stream order holds bytes that do
     not decode (last-per-subject delivery goes lowest sequence first).
   - `List` returns an error carrying `unreadable_entity_state` (`getEntity`, `manager.go:224-230`; the decode
     rule in `graph-entity-writes`).
   - Afterwards `natsfixture.AwaitNoStacks` finds no `jetstream.(*kvs).ListKeys.func1` and no
     `NativeDeliveryStacks`.
   - On base it should fail: more than 256 keys are left, so the lister blocks at `kv.go:1450`. Not measured; the
     lister must have forwarded about 258 keys before the RPC returns. Task 2.2's note records the base failure
     count over a stated `-count`; below 100%, put the bad entity after a few dozen good ones.
2. **`List` cut short is an error, never a short list.**
   - Unit cases in `pkg/lifecycle/manager_test.go` over a scripted `fakeBucket.WatchFiltered`: one matching key,
     then (i) the context is cancelled, or (ii) the channel closes before the end marker with the context live.
   - Each returns no participants and an error; in (i) it matches `context.Canceled`.
   - Mutant: ignore `FilteredKeys`'s error.
3. **`reclaim` stopped mid-listing leaves nothing running.**
   - `TestPublishStoppedMidReclaimLeavesNoDelivery` in a new
     `natsclient/storagereport/storage_report_integration_test.go`, on the shared server
     (`fixture_integration_test.go`). 10 cycles, as `graph/follower`'s `TestFollowerRunReturnsAndLeavesNothing`.
   - 600 stale rows are put raw.
   - A test-side `ReportStore` embeds the real `jetstream.KeyValue`. Its listing method opens the real watch,
     waits for `natsfixture.AwaitNativeDeliveryBlocked`, cancels `Publish`'s context itself, and then returns the
     handle. That orders the cancel after the backlog.
   - `Publish` returns an error matching `context.Canceled`, and `Deleted` is 0. No delivery or lister stack
     remains.
   - On base the store overrides `ListKeys`, and each cycle strands with probability of about one half (the
     lister's random `select`). After the fix it overrides `WatchFiltered`.
   - Precedent with no production hook: fix-461 task 2.4's test-side `ReportWatchStore`, accepted on PR #494.
     It uses a real server and real nats.go; only the cancel is ordered.
4. **`reclaim` over a short listing deletes nothing.**
   - A unit case in `storage_report_test.go`: `fakeReportStore.WatchFiltered` closes before the end marker.
   - `Publish` reports `Deleted` 0 and an error.
   - Mutant: ignore the listing error.
5. **`CatalogReader` serves `FilteredKeys`.**
   - `kvcatalog_integration_test.go:98-104` lists through `natsclient.FilteredKeys(ctx, reader, …)` in place of
     `ListKeys*`, as a product would.

Generated checks (`docs/testing.md` § Decide): not used.

- The history is short: a listing that completes or does not, then an early return or a stop.
- Every order that changes the outcome is an example above:
  - an error after the first key (1);
  - cancel or close before the end marker (2, 4);
  - stop with a backlog, over the random `select`, 10 cycles (3).
- `FilteredKeys`' own spec scenarios cover cancel and close for every pattern.

## Residuals (#185)

- **R1 (MEDIUM).** Tests 2(ii) and 4 model a channel closing with the context live using a scripted watcher. A
  real connection `Close` or a failed ordered reset mid-snapshot (`js.go:2365-2367`) cannot be forced without a
  hook, so it is not reproduced. The handling is `FilteredKeys`' tested `ok` check (`natsclient/kv.go:676-682`).
- **R2 (MEDIUM).** In the #454 window, `FilteredKeys` waits at most 5 s past the context and then returns an error
  (transport-client "stops its watcher before returning"). It fails closed, inherited as specified.
- **R3 (LOW).** `List`'s deadline ending mid-read has no separate real-server test. It is the same early return as
  test 1, and after the fix no read starts before the listing ends.
- **R4 (MEDIUM, filed as #534).** nats.go may report a key twice on a busy bucket (`kv.go:1431-1432`), and `List`
  pages with `Offset`/`Limit` over an unsorted, undeduplicated list (`manager_query.go:84-92`), against "sort/dedup
  before limit" (`AGENTS.md` § 2). It predates this change and `FilteredKeys` keeps it; #534 stays open unless the
  owner folds it into task 2.2.

## Merge order

- #530 shares no file and goes either way.
- #502 names `storage_report_test.go`. Whichever merges second merges `origin/main` in.
