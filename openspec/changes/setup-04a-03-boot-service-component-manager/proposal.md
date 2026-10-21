# Proposal: setup-04a-03-boot-service-component-manager

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). `service` is split
by lifecycle owner into seven pull requests (owner, 2026-10-20, #9 comment 6296023694, option B; order in
`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md` § 2.3). PR 1, `BaseService` and the
shared files, merged as `3453d9b` (#343); PR 2, `Manager` and `Metrics`, merged as `7c4dd2c` (#353). This change is
PR 3: the component manager (`ComponentManager`), the lifecycle owner that holds the composed components, starts and
stops them, and serves their diagnostics. One lifecycle owner, 2,487 non-test pin lines at SemStreams `8b99efe9`.

## What Changes

- Port `component_manager.go`, `component_manager_config.go`, `component_manager_http.go`,
  `stream_override_expiry.go` and `rule_pack_bind.go` with their tests, and the PR 3 tests PR 2 cut from
  `startup_observability_test.go` and its amendment file (archive
  `2026-10-21-setup-04a-03-boot-service-manager/notes/2.1.md`, parts 3 and 5). `RegisterAll` registers the component
  manager's constructor. `attributeShutdownError` and `publishComponents` land with their callers (sweep P16).
- The component manager satisfies PR 2's unexported `componentStartup` interface (`service/service_manager.go`), the
  seam that replaced the pin's four concrete `*ComponentManager` checks; the design says whether it does so as is.
- Restore the one-method `PredicateReconciler` interface in `pkg/projection` (owner, #9 comment 6296023694, Q2), so
  the rule-pack binding and the later rule-processor port name the same type.
- Per-component start contexts against ADR-113 ("Start's context bounds Start; Stop(ctx) is the only stop"): the
  design states the component manager's observable behaviour at the pin's per-component cancel
  (`component_manager.go:622` on the pin) and the owner's reading settles it.
- Cite pin ADR-063 (store substrate) through #321 rather than carrying it in this change. Carry pin ADR-095 and
  ADR-096 with port notes, as PR 1's design said (design, Decision records).

## Capabilities

Named in `design.md` D9; the delta files are written in task 1.2, after the owner accepts the design, and
`skip_specs` is removed then.

- `service-lifecycle` (ADDED): the component manager's Start bounds Start, with the owner's answer on each component's
  Start context and the composition root reacting to the root context ending (design D2, ADR-113); the boot set composed
  once
  and sealed (Q-B); providers before consumers, and the store registry they fill; component health published with drops
  counted; a lapsed stream migration override reported while running; rule-pack contracts checked before any reconciler
  is injected.
- `operator-surface` (ADDED): the component manager's routes under `/components`, and the composition projections
  served from the result kept at construction. "/openapi.json documents the mounted routes" is unchanged: `RegisterAll`
  registers the component manager's spec instead of an `init` (design D5 option a).

## Non-goals

`MessageLogger`, `StorageObservabilityService`, `MetricsForwarder`, `HeartbeatService` and `LogForwarder` (PRs 4-7).
Carrying ADR-063 (#321). The heartbeat half of #344 (PR 6).
