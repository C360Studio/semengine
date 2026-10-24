# Proposal: setup-04a-03-boot-service-heartbeat-metrics-forwarder

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). `service` is split
by lifecycle owner into seven pull requests (owner, 2026-10-20, #9 comment 6296023694, option B; order in
`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md` § 2.3). PRs 1-5 merged: `BaseService`
and the shared files as `3453d9b` (#343), `Manager` and `Metrics` as `7c4dd2c` (#353), `ComponentManager` as `ad9fda0`
(#359), `MessageLogger` as `05d6ec6` (#367), and `StorageObservabilityService` as `a1c5f21` (#376). This change is
PR 6: the metrics forwarder (`MetricsForwarder`), which publishes the metrics registry's samples on a ticker, and the
heartbeat service (`HeartbeatService`), which reports component health on a ticker. Two lifecycle owners in one pull
request, as the accepted split says (inventory § 2.3 row 6), 751 non-test pin lines at SemStreams `8b99efe9`.

## What Changes

- Port `metrics_forwarder.go` and `heartbeat.go` with their tests. `RegisterAll` registers both constructors.
- Apply ADR-113 (settled) to each service's run context and ticker goroutine (`metrics_forwarder.go:216-221`,
  `heartbeat.go:166-171`).
- Resolve the shape-sweep findings that name either service (inventory § 3), and decide how the heartbeat settings are
  decoded so a miscased key is no longer silently overridden by the default (#344).

## Capabilities

To be filled by the design: the `service-lifecycle` deltas for both services' start, stop and readiness promises.
`skip_specs` is set until the design names the deltas.

## Non-goals

`LogForwarder` (PR 7). The follow-ups from PRs 3-5 (#361-#364).
