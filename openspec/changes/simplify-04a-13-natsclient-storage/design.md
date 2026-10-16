# Design: simplify-04a-13-natsclient-storage

This design depends on `INVENTORY PASS` for `inventory.md`; premises cite its sections. It ports nothing from the pin,
so no pin probe applies; pin facts are caller counts from the pin snapshot (§ 4).

## D1. The package is `natsclient/storagereport`

The code produces and reads the storage report, so the package is named for it. It sits under `natsclient` because it
reads JetStream account state through natsclient's backing-stream and KV-key helpers and its `*Client`
(inventory § 2). A child package importing its parent is the shape `graph/kvcatalog` already uses.

Options:

- (a) `natsclient/storagereport`. Recommended.
- (b) Top-level `storagereport`. Same code; it would sit beside the unrelated top-level `storage` package (the
  message store interfaces), and readers would take one for the other.
- (c) `internal/storagereport`. Smallest public surface, but the pin exports this surface and the `service` port
  (change 3) would need to stay in-repo forever; it changes the ledger contract "every exported operation as at the
  pin". Owner call, not recommended in a move.
- (d) Do nothing. #222 part a stays open.

The pinned revive fails on seven names that start with the package name (inventory, shape sweep). They become
`PublisherConfig`, `Publisher`, `ResourceKey`, `Observer`, `Snapshot`, `ConsumerConfig`, `Consumer`, and the two
constructors follow their types (`NewPublisher`, `NewConsumer`). Every other name is kept: renaming
`StorageInventory`, `StorageResource` and the rest is optional polish no linter asks for.

## D2. What changes in the move, and what does not

- `git mv` each file, so history follows. The package clause changes and natsclient names are qualified. No logic
  changes.
- `nilContextErrorOf` is one line over `errs.WrapInvalid`. The new package keeps a private copy rather than natsclient
  exporting a helper. The refusal names the new type names.
- `Client.AccountStreamLister` cannot move as a method. It becomes `func AccountStreamLister(client *natsclient.Client)
  StreamListerSource`, which returns `client.JetStream()` as a `StreamLister`. A nil client gives a source that returns
  an invalid-data error.
- `storagereport` imports no `graph` package. `OwnerResolver` stays a function: kvcatalog's in-package tests import
  `storagereport`, so the reverse import would be a cycle.
- Tests move with their files. The integration test gets the package's own shared fixture (`TestMain`,
  `natsfixture.Shared`, the four helpers it uses), as `graph/kvcatalog/fixture_client_integration_test.go:18-44`
  does. The `// Own server:` comment stays above `natsfixture.New`. The storage rows of
  `nil_context_siblings_test.go` (`:207-208`, `:294-298`) move to a storagereport test of the same name.
- `storagereport` becomes a coverage target, `merged` like natsclient. At base, the storage code's coverage is 94.1%
  merged and 93.4% unit only; natsclient without it is 87.4% merged (inventory § 3; estimate from the base unit profile
  and an integration profile of an identical natsclient tree).

Observable: every moved test passes with only qualifier and name edits. `go build ./...`, both test lanes, `task lint`
(no stutter findings) and `task cover:check` pass.

## D3. The observer is told that the account row is gone (#85)

`Observer` gains `ForgetAccount(row AccountReport)`, with `row` the last account row the consumer held, as
`ForgetResource` carries the last resource row. The consumer calls it once:

- on a delete or purge of `StorageAccountReportKey` when it held an account (`removeAccount`);
- at a replacement watch's sync marker when the initial values lack the account key and it held one (`replaceWith`).

No call is made for a tombstone of an account it never held, or when the account value is undecodable (the held row
is kept, as today). The call happens after the snapshot changes and outside the lock, like the other observer calls.
`logAccountRetracted` and its Warn go.

Options:

- (a) `ForgetAccount(row AccountReport)`. Recommended; owner question 1.
- (b) `ObserveAccount(AccountReport{})`. No signature change, but an empty report then means both "no tiers" and
  "no account". This repository keeps unknown and empty apart (`Snapshot.Synced`, `AccountKnown`).
- (c) Keep the declared gap.

Who it is for: the consumer calls it (in-package production caller). The one implementer is the pin's `service`
metrics surface, which already deletes per-tier series (inventory § 4, `:479`).

Tests:

- `TestReportConsumer_AccountDeleteReachesObserver`: a held account then a delete gives one `ForgetAccount` with the
  held row and `AccountKnown` false; a delete with no held account calls nothing.
- `TestReportConsumer_ResyncRetractionReachesObserver`: a replacement watch without the account key gives one
  `ForgetAccount` at the marker, none before it; an undecodable account value gives none.
- `TestReportConsumer_AccountRetractionIsLogged` (`storage_report_resync_test.go:240`) is deleted with the log.

The new tests fail to compile on the base commit: `recordingObserver` has no `ForgetAccount`.

## D4. Text follows the code

- Spec: the requirement moves to `storage-report`, rewritten for D3. The nil-context refusal of `Collect` and
  `Publish`, covered today by transport-client's "every exported natsclient call", is stated there.
- Ledger natsclient row: the destination gets the note "storage_*.go in natsclient/storagereport since
  simplify-04a-13 (#222)". The contract names D1–D3. The #85 sentence (`:1167-1169`) goes.
- `docs/repository-map.md`: a row for the new package; the coverage count goes from 16 to 17.
- `graph/kvcatalog.go:88,:91` comments name `storagereport`.
- A comment on #281 gives the new path of `storage_pressure.go:82`.

## Exports (#190)

Kinds: **Pin plumbing**: no caller in this repo; the pin's admitted `service` (ported in change 3) reads it.
**Reached**: not named by `service`, but reached through a pin-plumbing type's fields, return values or config.
**Internal**: read only inside the seven files and tests. No symbol is a back door. Action for every row: moved;
renames as shown. Owner question 2 covers the Pin plumbing kind.

| Symbols | Action | Kind | Pin reader or note |
| --- | --- | --- | --- |
| `NewStorageInventoryCollector`, `StorageInventoryCollector` (`Collect`, `Run`, `Latest`), `StorageInventoryConfig`, `StorageInventory`, `StorageResource`, `DefaultStorageInventoryInterval`, `DefaultStorageInventoryTimeout` | moved | Pin plumbing | `service/storage_observability.go:111,:115,:160,:218,:220,:431` |
| `Client.AccountStreamLister` | becomes `AccountStreamLister(client)` | Pin plumbing | `service/storage_observability.go:219` |
| `NewStorageReportPublisher`→`NewPublisher`, `StorageReportPublisher`→`Publisher` (`Publish`), `StorageReportConfig`→`PublisherConfig`, `PublishResult`, `ThresholdSource`, `StoragePressureThresholds` | moved, renamed | Pin plumbing | `:425-451` |
| `NewStorageReportConsumer`→`NewConsumer`, `StorageReportConsumer`→`Consumer` (`Run`, `Snapshot`), `StorageReportConsumerConfig`→`ConsumerConfig`, `StorageReportSnapshot`→`Snapshot`, `ReportWatchStore` | moved, renamed | Pin plumbing | `:161,:203-205,:405`; `_http.go:123` |
| `StorageReportObserver`→`Observer` (+ `ForgetAccount`) | moved, renamed, one method added | Pin plumbing | `_metrics.go:339,:398,:413` |
| `ResourceReport`, `AccountReport`, `StorageTier`, `TierFile`, `TierMemory`, `TierUnknown`, `CapacityState`, `CapacityBounded`, `CapacityUnbounded`, `CapacityUnknown`, `AttributionAttributed`, `AttributionNotApplicable`, `OvercommitmentState`, `OvercommitmentWithin`, `OvercommitmentOver`, `OvercommitmentNotApplicable`, `OvercommitmentUnavailableUnknownLimit`, `PressureState`, `PressureNormal`, `PressureWarning`, `PressureHigh`, `PressureCritical`, `PressureBasisAccountTier` | moved | Pin plumbing | `_metrics.go:73-106,:580-586`; `_http.go:55,:96` |
| `Capacity` (`Limit`, `Usage`, `Bounded`), `NewCapacity`, `UnknownCapacity`, `AttributionState`, `AttributionUnattributed`, `Growth` (`Rate`), `GrowthState`, `GrowthKnown`, `GrowthUnknown`, `GrowthUnavailable*` (3), `Projection`, `ProjectionUnavailable*` (4), `Pressure` (`AgainstAccountTier`), `PressureInput*` (5), `PressureBasis`, `PressureBasisOwnBound`, `PressureUnavailable*` (5), `TierComparison`, `AccountReport.TierFor`, `StorageResource.Attributed`, `OvercommitmentUnavailableUnboundedLimit`, `OwnerResolver`, `StreamLister`, `StreamListerSource`, `InventoryPublisher`, `ReportStore`, `AccountLimitReader` | moved | Reached | fields of `ResourceReport`/`AccountReport`; config fields of the pin-plumbing constructors |
| `DeriveGrowth`, `Observation`, `Project`, `AssessPressure`, `PressureAgainstAccountTier`, `DeriveAccountReport`, `AccountTierLimits`, `AccountTierLimitsFrom`, `UnknownAccountTierLimits`, `UnknownGrowth`, `ResolvedPressureThresholds`, `StoragePressureThresholds.Resolve`, `DefaultStoragePressureThresholds`, `Default{Warning,High,Critical}{Headroom,Horizon}` (6), `MinGrowthSampleInterval`, `StorageReportGrowthObservations`, `SkipReasonStaleInventory`, `DefaultReportConsumerRetryBackoff`, `StorageAccountReportKey`, `StorageReportKey`→`ResourceKey` | moved (one renamed) | Internal | kvcatalog test cross-pins the two growth constants (`kvcatalog_storage_report_test.go:57`); owner decides, recommended kept in a move |

## Open issues and merge order

- #85: retired (`Closes #85`; tasks 2.3, 3.3).
- #222: part a done here (tasks 3.1–3.2); the PR body references it without a closing keyword.
- #281: stays. It is component's `GenerateConfigSchema` defect; its line pin moves (task 4.3).
- #195: stays. Optional adoption; the moved integration test keeps its own client.
- #75 and #169: stay (6d).
- #278 and #279 share no hunk with this change. Either order works; the second to land merges `origin/main`. The
  later 6b–6d and 8d (kvcatalog, #133) merge after this one.

## Owner questions

1. The #85 shape (D3).
2. The Pin plumbing kind moves whole (Exports).
3. The name and the nine renames (D1).
4. A count waiver for #159 (3,415 non-test lines moved; a move, not a port).
