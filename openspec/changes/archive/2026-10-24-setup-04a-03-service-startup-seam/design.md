# Design: setup-04a-03-service-startup-seam

Base `b3a4fa9`. Inventory: `inventory.md`. Retires #364 (`Closes #364`, tasks 2.1-2.2).

## Context

Each startup count already has one home: service outcomes on the manager (`service/service_manager.go:452`), each
component's start flags on its record (`service/component_manager.go:141-143`). `/readyz` and `/services` read both
on demand (`service/service_manager_http.go:467-530`). Only the gauge is pushed: the manager pushes after each service
outcome (`service/service_manager.go:463-484`) and the component manager pushes from inside its `Start`
(`service/component_manager_lifecycle.go:94,106`), which is the sole reason it must be handed the writer after
construction (`:320-324`). The writer and setter are the pin's (pin `component_manager.go:756-767`,
`service_manager.go:605-613`); the interface is PR 2's (archived `setup-04a-03-boot-service-manager` D3).

## Options

- **A. A shared startup recorder** built by the manager and handed through `Dependencies`. Duplicates the records'
  start flags; a `ComponentManager` composed by `RegisterInstance` (`service/service_manager.go:323`) was built with
  other `Dependencies` and would hold the wrong recorder or none; the registry is only known at `StartAll`
  (`:516-523`), so the recorder needs its own late setter. Moves the setter, adds a type.
- **B. The gauge reads the counts when collected** (chosen). Removes the setter, the push calls and the writer's
  stored state; adds nothing a caller sees. The common spot the owner asked for is the service manager, which already
  answers `/readyz` and `/services` from the same two snapshots.
- **C. Concrete type only, push writer kept.** Leaves the post-construction setter the issue names.
- **Do nothing.** Leaves sweep shape 3 and the setter.

## D1. The startup gauge reads the counts when collected

At the point `StartAll` registers the writer today (`initializeStartupMetrics`, `:373`, after sealing and before any
listener binds), the manager registers one collector for `semengine_startup_units{owner,stage}` on the same registry,
refused as today when the registry already holds a collector of that name (no `metric.RegisterOrGet`). Each
collection reports exactly nine series: `services` × {`admitted`, `starts_invoked`, `starts_completed`,
`starts_failed`} and `components` × {`admitted`, `lifecycle_participants`, `starts_invoked`, `starts_completed`,
`starts_failed`}, with the values `/services` would report at that moment. The component values are the registered
`*ComponentManager`'s `startupSnapshot()`, or zero when the `component-manager` service is not one. With no registry
(none in `Dependencies` and no `metrics` service) nothing is registered, as today. A component manager never registers
or writes the gauge, so one started on its own publishes nothing.

Invariant (spec delta): a collected value is never older than the counts at the start of that collection, so a count
never falls while startup runs. The writer's mutex existed to keep that true under push
(`service/startup_metrics.go:31-33`); reading at collection makes it hold with no stored value. The read waits on no
service's or component's `Start` or `Stop`: the component counts are lock-free record reads
(`openspec/specs/service-lifecycle/spec.md:470-475`) and the manager releases its lock before calling any service
(`service/service_manager.go:725-739`).

Example tests suffice here, with no generated checks (`docs/testing.md`, order-dependent histories): each count only
grows, and the collector holds no state between collections, so no ordering of starts and collections can leave an
older value standing. The orderings that matter are a start held mid-way, a failure, a later start finishing before an
earlier one, and a finished startup; the synchronized examples below drive each of them, holding and releasing starts
on explicit signals.

Proving tests (kept, constructor renamed): `TestStartupCollectorReportsExactlyNineSeries` (was
`TestStartupMetricWriterInitializesExactlyNinePrivateSeries`), `TestStartupMetricsExposeFixedOwnerStagePairs`,
`TestStartupMetricHasNoUnitIdentityLabel`, `TestStartupCollectorRejectsForeignCollectorInsteadOfAdoptingIt`,
`TestStartupMetricRegistrationConflictFailsBeforeListenersOrChildren` (these two are the probe of
`prometheus/client_golang v1.25.0`'s refusal), `TestComponentStartupSnapshotDistinguishesAdmittedAndLifecycleStarts`,
`TestComponentMetricPublicationRemainsMonotonicUnderReverseCompletion`,
`TestManagerPrebindPublishesConcreteComponentCountsBeforeComponentManagerStart`. The first two of the last three drop
their `setStartupMetricWriter` line. New: `TestComponentManagerOnItsOwnPublishesNoStartupGauge` (a started
`ComponentManager`'s registry gathers no `semengine_startup_units`) and
`TestStartupGaugeReadsZeroComponentsForAStandInComponentManager` (the component series read zero for a stand-in).
`TestStartupGaugeVecHasOnlyPrivateServicePackageOwner` keeps its `metric/` checks and finds the gauge's name in exactly
one non-test file, `startup_metrics.go`.

## D2. The concrete component manager

`componentStartup` and its assertion (`service/component_manager_http.go:22-25`) go. The four manager sites
(`service/service_manager.go:516`, `service/service_manager_http.go:65,272,478`) and `ComponentsImplementing`
(`service/rule_pack_bind.go:41`) check `.(*ComponentManager)`, as the pin did (pin
`service_manager.go:593,1142,1543,1812`). A `component-manager` that is not the framework's keeps the pin's path: no
startup or component routes, zero component counts, readiness from services alone; about 25 tests compose one
(`newStandInComponentManager`, `service/service_manager_test.go:63`). The methods stay unexported; `withComponents`
stays for its two callers.

The readiness check `len(healthByName) != componentCounts.Admitted` (`service/service_manager_http.go:514`) goes: both
sides come from the component list fixed at construction (`service/component_manager.go:561-566`,
`service/component_manager_lifecycle.go:293-294`), so it cannot fire; at the pin it could (pin
`component_manager.go:1323` skipped nil components).

Proving tests: `component_startup_test.go` runs against `newTestComponentManager`
(`service/component_manager_helpers_test.go:101`). `TestComponentStartupServesStartupRoutesCountsAndComponentRoutes`:
before the commit `/components/list` answers and a gateway component's `/gateway/hello` answers 503 `NOT READY`;
`/services` and the gauge report two admitted and one lifecycle participant; after the commit `/gateway/hello`
answers. `TestComponentStartupGatesReadiness` (in the operator-surface `/readyz` requirement): with one lifecycle
component and one plain component, READY; plain component unhealthy → `not_ready`; lifecycle component's `Start` held →
`starting`; a sibling's `Start` failed while the other is held → `failed`; the component manager's own `Stop` called →
`stopping`. `TestConfigureRefusesACompositionWithoutAComponentManager` uses `newStandInComponentManager`.

## ADR-113

No context is created, held or passed differently; no `Start` or `Stop` changes.

## Merge order

Before #363 and #362 (owner, 2026-10-24); no other open pull request touches `service/`.

## Adoption

No cross-package primitive: the other mirroring gauges compute their values (`service/README.md:309`;
`openspec/specs/operator-surface/spec.md:429-440`), so none should read at scrape.
