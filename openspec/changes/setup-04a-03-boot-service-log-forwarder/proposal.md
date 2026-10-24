# Proposal: setup-04a-03-boot-service-log-forwarder

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). `service` is split
by lifecycle owner into seven pull requests (owner, 2026-10-20, #9 comment 6296023694, option B; order in
`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md` § 2.3). PRs 1-6 merged: `BaseService`
and the shared files as `3453d9b` (#343), `Manager` and `Metrics` as `7c4dd2c` (#353), `ComponentManager` as `ad9fda0`
(#359), `MessageLogger` as `05d6ec6` (#367), `StorageObservabilityService` as `a1c5f21` (#376), and `MetricsForwarder`
with `HeartbeatService` as `66c797a` (#383). This change is PR 7, the last: the log forwarder (`LogForwarder`), 124
non-test pin lines at SemStreams `8b99efe9` (inventory § 2.3 row 7).

The owner ruled on 2026-10-19 (#9 comment 6272104271, option b; record: the archived `port-logforwarderpolicy` design,
Owner question) that `LogForwarder` is ported and says plainly that it forwards nothing yet, that `pkg/logging` and the
boot wiring stay out of the port set, and that this same pull request decides whether the mandatory
`flow-service.websocket` exclusion (`internal/logforwarderpolicy/policy.go:14`) stays.

## What Changes

- Port `log_forwarder.go` with its tests. `RegisterAll` registers its constructor.
- State in the package docs and the service's own docs that it forwards nothing yet.
- Apply ADR-113 (settled) to its run context, if it holds one.
- Resolve the shape-sweep findings that name it (inventory § 3; S6 applies to `LogForwarderConfig`), and decide the
  `flow-service.websocket` exclusion.

## Capabilities

To be filled by the design: the `service-lifecycle` deltas for the log forwarder's start, stop and readiness promises.
`skip_specs` is set until the design names the deltas.

## Non-goals

`pkg/logging` and the boot wiring (owner ruling above). The follow-ups that stay open past this pull request: #361-#364
and #384.
