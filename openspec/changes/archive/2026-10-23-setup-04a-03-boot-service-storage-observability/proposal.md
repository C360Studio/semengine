# Proposal: setup-04a-03-boot-service-storage-observability

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). `service` is split
by lifecycle owner into seven pull requests (owner, 2026-10-20, #9 comment 6296023694, option B; order in
`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md` § 2.3). PRs 1-4 merged: `BaseService`
and the shared files as `3453d9b` (#343), `Manager` and `Metrics` as `7c4dd2c` (#353), `ComponentManager` as `ad9fda0`
(#359), and `MessageLogger` as `05d6ec6` (#367). This change is PR 5: the storage observability service
(`StorageObservabilityService`), the lifecycle owner that collects JetStream and KV storage reports, publishes them as
metrics, and serves the latest report over HTTP. One lifecycle owner, 1,540 non-test pin lines at SemStreams
`8b99efe9`.

## What Changes

- Port `storage_observability.go`, `storage_observability_http.go` and `storage_observability_metrics.go` with their
  tests. `RegisterAll` registers the storage observability service's constructor.
- Adapt to `natsclient/storagereport` (inventory § 2.4: 40 names, nine renames, `AccountStreamLister` a function) and
  `graph/kvcatalog`; the pin tests name four identifiers that package has since unexported (inventory § 2.3 row 5).
- Apply ADR-113 (settled) to the pin's run context (`storage_observability.go:287`) and its collector and consumer
  goroutines (`:294`, `:298`, joined `:304`).
- Resolve the shape-sweep findings that name the storage service (inventory § 3): S1 run context, S3 local lifecycle
  state (`storage_observability.go:167`, toward `internal/lifecycleguard`, #196), and S6 the plain config decoder
  (`storage_observability.go:178`).

## Capabilities

To be filled by the design: the `service-lifecycle` and `operator-surface` deltas for the storage service's start,
stop and readiness promises and the route it serves. `skip_specs` is set until the design names the deltas.

## Non-goals

`MetricsForwarder`, `HeartbeatService` and `LogForwarder` (PRs 6-7). The follow-ups from PRs 3 and 4
(#361-#364, #369).
