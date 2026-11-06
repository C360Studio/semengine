# Tasks: fix-496-key-lister-strands

Design track. Tests come first.

- Each code task ends at a clean-tree local `task verify` at its commit (#171).
- Integration tests run under `go test -race -count=1 -tags integration <pkg>`.
- "Shown to fail": a real-server test fails on the base commit, or a unit test meets `task mutate:check`.
  Either is recorded in `notes/<task>.md`.
- No `time.Sleep`, no `Skip`, no deadline of a test's own. A test waits on `natsfixture.AwaitNativeDeliveryBlocked`
  or `AwaitNoStacks`.
- No production hook, and no rebuild of nats.go faults (#301).
- One task is one developer's unit. Tasks 2.2 and 2.3 depend on 2.1. Every task's tree compiles: an interface
  method is removed only in the task that migrates its last caller.

## 1. Design

- [x] 1.1 Pre-owner design review of record by Codex on `proposal.md`, `design.md` and both spec deltas, with no
  `BLOCKING` finding. Then owner approval of the design and of the two exported changes (`CatalogReader`,
  `ReportStore`). Owner chose 2026-11-06: `CatalogReader` drops `ListKeys*` (D2); #534 stays separate.
  - Done: Codex design review of record APPROVE at `32e2e14` (PR #532); owner approval recorded on #496.
  - Gate: `task spec:check`, `task docs:check`.

## 2. Code

- [ ] 2.1 `graph/kvcatalog` (design D2, test 5).
  - `kvcatalog_integration_test.go:98-104` lists through `natsclient.FilteredKeys(ctx, reader, …)`. This fails to
    compile on base, because `CatalogReader` has no `WatchFiltered`; record it.
  - Then `CatalogReader` gains `WatchFiltered`. It keeps `ListKeys` and `ListKeysFiltered` here, because
    `pkg/lifecycle` still returns it as `entityStatesReader` (`manager.go:213-214`); task 2.2 drops them.
  - `TestCatalogReader_ExportedSurfaceIsReadOnly` adds `WatchFiltered`.
  - Gate: `go test -race -count=1 ./graph/kvcatalog/` and with `-tags integration`; `task verify`.
- [ ] 2.2 `pkg/lifecycle` (D1, D2, tests 1 and 2).
  - Write `TestListLeavesNoListerAfterAReadError` (graph-ingest started first, every entity undecodable, D3
    test 1) and show it failing on base in every run of a stated `-count`; record it, and that `getEntity`'s error
    is not skipped at `manager_query.go:64-70`. If base does not fail every run, stop for review (D3).
  - Then `List` reads `natsclient.FilteredKeys(ctx, bucket, ">")`, and `entityStatesReader` swaps `ListKeys` for
    `WatchFiltered`.
  - `fakeBucket` (`manager_test.go:276-292`) gains a scripted `WatchFiltered` that closes `Updates` once, after
    `Stop`, as nats.go does.
  - Unit cases 2(i) and 2(ii).
  - `List`'s doc states the error.
  - `CatalogReader` drops `ListKeys` and `ListKeysFiltered` (owner, 2026-11-06), with
    `TestCatalogReader_ExportedSurfaceIsReadOnly` and `reader_capabilities_test.go` updated; the doc comment keeps
    "exact union".
  - Gate: `go test -race -count=1 -tags integration ./pkg/lifecycle/ ./graph/kvcatalog/`; `task mutate:check` for
    2(i) and 2(ii); `task verify`.
- [ ] 2.3 `natsclient/storagereport` (D1, D2, tests 3 and 4).
  - Write `TestPublishStoppedMidReclaimLeavesNoDelivery`, 10 cycles, in a new `storage_report_integration_test.go`,
    with its test-side store overriding `ListKeys`. Show it failing on base: at least one stranded cycle in each run
    of a stated `-count` (D3 test 3); record it, or stop for review.
  - Then `reclaim` reads `natsclient.FilteredKeys(ctx, p.store, ">")`. `ReportStore` swaps `ListKeys` for
    `WatchFiltered`, the `ErrNoKeysFound` branch (`storage_report.go:702-704`) goes, and the test's override moves
    to `WatchFiltered`.
  - `fakeReportStore` (`storage_report_test.go:159-196`) lists through a scripted `WatchFiltered`. `liveKeys` reads
    its keys directly.
  - Unit test 4.
  - `ReportStore`'s doc names the migration.
  - Gate: `go test -race -count=1 -tags integration ./natsclient/storagereport/ ./service/ ./graph/kvcatalog/`;
    `task mutate:check` for test 4; `task verify`.
- [ ] 2.4 Migration notes in the PR body for `CatalogReader` and `ReportStore` (D2).
  - Gate: `gh pr view` and `gh issue view` read back.

## 3. Code review

- [ ] 3.1 Code review of record by another agent at the head, with no actionable findings. The PR body carries
  `implemented-by:` and `reviewed-by:`.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive fix-496-key-lister-strands`). This syncs
  `storage-report` and creates `lifecycle-query`; its Purpose is written then.
  - Gate: `task spec:check`, `task docs:check`.
