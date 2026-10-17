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
  StreamListerSource`. The source calls `client.JetStream()` on each call and returns it as a `StreamLister`, so each
  collection resolves the live JetStream context, as the method value did (`storage_inventory.go:107-110`). No nil
  check is added: behaviour is unchanged.
- `storagereport` imports no `graph` package. `OwnerResolver` stays a function: kvcatalog's in-package tests import
  `storagereport`, so the reverse import would be a cycle.
- Tests move with their files. The integration test gets the package's own shared fixture (`TestMain`,
  `natsfixture.Shared`, the four helpers it uses), as `graph/kvcatalog/fixture_client_integration_test.go:18-44`
  does. The `// Own server:` comment stays above `natsfixture.New`. The storage rows of
  `nil_context_siblings_test.go` (`:207-208`, `:294-298`) move to a storagereport test of the same name.
- Test function names do not change, so the ledger's citations of storage tests by name (`admission-ledger.yaml:1229-
  1293`) stay true. Two comments that name an old path are fixed: `kvcatalog_storage_report_test.go:51` names
  `storagereport.TestStorageReportPublisher_HistoryDepthSurvivesACrashLoop`, and `storage_report.go:11` names
  `graph/kvcatalog/kvcatalog.go`.
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

The held-account cases of the new tests fail on their assertions on the base commit: the consumer never calls
`ForgetAccount` (notes/2.3.md).

Generated checks (`docs/testing.md`, order-dependent histories): examples suffice. The history is short and has few
distinct orderings: the account row arrives, the sync marker arrives, then either a delete or a replacement watch
that lacks the key. The two tests above name every case that matters:

- A held account against an absent one: only the held case calls `ForgetAccount`, and with the row last observed.
- A repeated tombstone: the second delete finds no held account and calls nothing, so the call is once only.
- A replacement watch: nothing is forgotten while its initial values are still arriving, and the one call comes at
  the sync marker.
- An undecodable account value in a replacement watch: the held row is kept and nothing is forgotten.

The oracle is the row the observer was given by `ObserveAccount`, not a field of the consumer. Codex's mutation
checks on PR #283 (review record, 2026-10-17) reported that removing either `ForgetAccount` call (in `removeAccount`
or in `replaceWith`) fails a test. That is the reviewer's report, not a run recorded in this change.

## D4. Text follows the code

- Spec: the requirement moves to `storage-report`, rewritten for D3. The nil-context refusal of `Collect` and
  `Publish`, covered today by transport-client's "every exported natsclient call", is stated there.
- Ledger natsclient row: the destination gets the note "storage_*.go in natsclient/storagereport since
  simplify-04a-13 (#222)". The contract names D1–D3. The #85 sentence (`:1167-1169`) goes.
- `docs/repository-map.md`: a row for the new package; the coverage count goes from 16 to 17.
- `graph/kvcatalog.go:88,:91` comments name `storagereport`.
- A comment on #281 gives the new path of `storage_pressure.go:82`.

## Exports (#190)

Kinds are #190's (owner direction, #223 comment 6214503370). **Plumbing** names its production caller. None of
these names has a production caller in this repository. For the rows kept, the caller is the pin's `service`, an
admitted package ported in change 3. The reviewer contract (`.agents/contracts/semengine-reviewer.md:441`) says a
symbol whose only caller is in an admitted package not yet ported is not dead surface. That departs from the
direction's "no caller here → unexported or dropped", and owner question 2 asks it. Field, config-field and signature
types of the names `service` reads, and the value sets of their string fields, are Plumbing for the same reader. No
symbol is **framework API** (no contract line names one; `docs/contract.md:139` offers the HTTP endpoint, not the Go
API) and none is a **back door**.

| Symbols | Action | Kind | Production caller (pin `service/`) |
| --- | --- | --- | --- |
| `NewStorageInventoryCollector`, `StorageInventoryCollector` (`Collect`, `Run`, `Latest`), `StorageInventoryConfig`, `StorageInventory`, `StorageResource`, `DefaultStorageInventoryInterval`, `DefaultStorageInventoryTimeout`, `OwnerResolver`, `StreamLister`, `StreamListerSource`, `InventoryPublisher` | moved | Plumbing | `storage_observability.go:111,:115,:160,:218-220,:237,:431` |
| `Client.AccountStreamLister` | becomes `AccountStreamLister(client)` | Plumbing | `storage_observability.go:219` |
| `NewStorageReportPublisher`→`NewPublisher`, `StorageReportPublisher`→`Publisher` (`Publish`), `StorageReportConfig`→`PublisherConfig`, `PublishResult`, `SkipReasonStaleInventory`, `ReportStore`, `ThresholdSource`, `StoragePressureThresholds` (`Resolve`), `ResolvedPressureThresholds` | moved, renamed | Plumbing | `storage_observability.go:96,:105,:129,:425-451` |
| `NewStorageReportConsumer`→`NewConsumer`, `StorageReportConsumer`→`Consumer` (`Run`, `Snapshot`), `StorageReportConsumerConfig`→`ConsumerConfig`, `StorageReportSnapshot`→`Snapshot`, `ReportWatchStore` | moved, renamed | Plumbing | `storage_observability.go:161,:203-205,:405`; `_http.go:123` |
| `StorageReportObserver`→`Observer` (+ `ForgetAccount`) | moved, renamed, one method added | Plumbing | `_metrics.go:339,:398,:413` |
| `ResourceReport`, `AccountReport`, `TierComparison`, `StorageTier` (+3 values), `Capacity` (`Limit`, `Usage`), `CapacityState` (+3), `AttributionState` (+3), `Growth` (`Rate`), `GrowthState` (+2), `GrowthUnavailable*` (3), `Projection`, `ProjectionUnavailable*` (4), `Pressure`, `PressureState` (+4), `PressureInput` (+4), `PressureBasis` (+2), `PressureUnavailable*` (5), `OvercommitmentState` (+3), `OvercommitmentUnavailable*` (2) | moved | Plumbing (types and field values of what `service` reads) | `_metrics.go:73-106,:356-360,:413-455,:580-586`; `_http.go:55,:96` |
| `DefaultStoragePressureThresholds`, `StorageReportGrowthObservations`, `StorageAccountReportKey`, `StorageReportKey`→`ResourceKey`, `NewCapacity`, `AccountReport.TierFor`, `StorageResource.Attributed` | moved (one renamed) | no production caller; kept by owner question 3 | read by `graph/kvcatalog` tests that check the bucket the catalog declares (inventory § 4) |
| `DeriveGrowth`, `Observation`, `UnknownGrowth`, `MinGrowthSampleInterval`, `Project`, `AssessPressure`, `PressureAgainstAccountTier`, `Pressure.AgainstAccountTier`, `DeriveAccountReport`, `AccountTierLimits`, `AccountTierLimitsFrom`, `UnknownAccountTierLimits`, `AccountLimitReader`, `UnknownCapacity`, `Capacity.Bounded`, `Default{Warning,High,Critical}{Headroom,Horizon}` (6), `DefaultReportConsumerRetryBackoff` | unexported (owner question 3) | Plumbing with no caller | none, here or at the pin (inventory § 4); `kvcatalog.go:91`'s comment is reworded |

## Open issues and merge order

- #85: retired (`Closes #85`; tasks 2.3, 3.3).
- #222: part a done here (tasks 3.1–3.2); the PR body references it without a closing keyword.
- #281: stays. It is component's `GenerateConfigSchema` defect; its line pin moves (task 4.3).
- #195: stays. Optional adoption; the moved integration test keeps its own client.
- #75 and #169: stay (6d).
- #278 merged (`838c1fa`) after this base; the branch merges `origin/main` before implementation (task 1.2). #279
  edits other rows of `docs/admission-ledger.yaml`; either order works, and the second to land merges `origin/main`.
  The later 6b–6d and 8d (kvcatalog, #133) merge after this one.

## Owner questions

1. #85: add `ForgetAccount(row)` to the observer (D3). Recommended: yes.
2. About 75 names are read only by `service` (ported in change 3). Your rule says no caller here means private or
   dropped; the reviewer contract says an admitted package not yet ported counts. Recommended: keep them public.
3. 22 names nothing outside the package reads become private in this move. Seven that the catalog's tests check stay
   public, including the bucket key and the growth-history depth, and so do `Resolve` with its result type.
   Recommended: yes.
4. Package `natsclient/storagereport`, with nine renames (D1); the pin called this `storage-observability`. OK?
5. The ~3,000-line limit is for ports, and this only moves code. Agreed it does not apply?
