# Inventory: setup-04a-03-service-startup-seam

base: b3a4fa9037676ab342006b60dbaa1e5b2cfdf682 (main; claim commit bbabb2f is empty)

Port-refactor #364 of `service`, a SemEngine package carried from the SemStreams pin `8b99efe9`. No new package is
carried, so no pin probe runs; pin statements below cite pin files read through `gh api contents?ref=8b99efe9`.

## 1. The claimed gap

- The seam: `service/service_manager.go:40` — `type componentStartup interface {` (six methods, `:41-46`; the issue
  lists five of them plus `GetComponentHealth`, and omits `withComponents`, which is `:45`).
- One implementer: `service/component_manager_http.go:25` — `var _ componentStartup = (*ComponentManager)(nil)`; one
  test implementer: `service/component_startup_test.go:40` — `var _ componentStartup = (*componentStartupDouble)(nil)`.
- The setter: `service/component_manager_lifecycle.go:322` —
  `func (cm *ComponentManager) setStartupMetricWriter(writer *startupMetricWriter) {`; its field
  `service/component_manager.go:92` — `startupMetrics atomic.Pointer[startupMetricWriter]`.
- The issue's suggestion, "pass the writer through Dependencies at construction", collides with the build order:
  `service/service_manager.go:373` — `if err := m.initializeStartupMetrics(); err != nil {` runs after
  `createComponentManager` and `sealComposition` (`:367-372`), and the writer takes the component manager's snapshot
  (`service/service_manager.go:526` — `componentSnapshot = components.startupSnapshot`).

## 2. Every spelling of the fact (startup counts and the gauge)

- Service counts: `service/service_manager.go:452` —
  `m.serviceOutcomes = make(map[string]*serviceInvocationOutcome, len(m.order))`, summed at `:486-500`.
- Component counts: `service/component_manager.go:141-143` — `startInvoked bool` / `startCompleted bool` /
  `startFailed bool`, summed at `service/component_manager_lifecycle.go:293` —
  `func (cm *ComponentManager) startupSnapshot() startupUnitCounts {`.
- Readers: `/readyz` and `/services` through `service/service_manager_http.go:467` —
  `func (m *Manager) currentStartupSnapshot() startupSnapshot {`; the gauge through `service/startup_metrics.go:34` —
  `type startupMetricWriter struct {`, pushed from `service/service_manager.go:463,475` (`recordStartInvoked`,
  `recordStartCompleted`) and `service/component_manager_lifecycle.go:94,106` — `cm.recordComponentStartupMetrics()`.
- No other spelling: `git grep -n 'startup_units\|startupMetricWriter\|setStartupMetricWriter' -- ':!*.go'
  ':!openspec/changes/archive'` gives only `docs/admission-ledger.yaml:2996`.

## 3. Adjacent claims

- Open PRs touching `service/` or the operator-surface/service-lifecycle specs:
  `gh pr list --state open --json number,title,files` → #401 (this claim) only; #397 and #400 touch neither.
- Open issues naming the seam or the managers (`gh issue list --state open --search` on `componentStartup`,
  `startupMetric`, `startup gauge`, `component manager`, `service manager`, `readyz`): #364 (retired here);
  #362, #363 (serial after this PR, owner 2026-10-24: "This one (#364) goes first"); #361, #196, #280, #195, #340,
  #216, #110, #9, #7 name the word or a neighbouring concern, not this seam.
- Specs: no requirement states the seam or the gauge (`git grep -n 'startup_units\|componentStartup\|startup gauge'
  -- openspec/specs` empty). `openspec/specs/operator-surface/spec.md:18` ("the component manager's read-only routes
  when it provides them") and `:40` (`/readyz`) stay true unchanged. `openspec/specs/service-lifecycle/spec.md:876`
  (the heartbeat refuses a non-`*ComponentManager`) is untouched.
- Ledger: `docs/admission-ledger.yaml:2996` — "withComponents and the post-construction startup metric setter stay for
  the service manager (#364)".
- Seam origin: `openspec/changes/archive/2026-10-21-setup-04a-03-boot-service-manager/design.md:196-212` (D3).

## 4. Who it is for

No new exported symbol, port, subject, bucket or config field. New unexported collector type: production caller
`service/service_manager.go` `initializeStartupMetrics` (`:510`).

## 5. The problem shape

A metric mirroring state another value already holds. Closest instances: `semengine_streams_migration_override_expired`
(pushed at each evaluation, `service/README.md:309`) and `semengine_storage_*` (rebuilt from NATS reads,
`openspec/specs/operator-surface/spec.md:429-440`); both compute their values, so neither reads at scrape. No
production `prometheus.Collector` exists (`git grep -n 'Collect(ch' -- '*.go'`: only `metric/handler_test.go:68`,
`metric/lifecycle_test.go:203`, `service/metrics_forwarder_test.go:366`, `service/metrics_owner_test.go:149`).

## 6. Pin decision records

None govern the writer or the seam: the 85 pin ADRs searched for `startup_units`, `startupMetricWriter`,
`setStartupMetricWriter`, `startup metric`, `startupSnapshot` match nothing; `readyz` matches ADR-088 (readiness is
per producer), whose rules this change does not touch. Pin facts: pin `service/service_manager.go:593` —
`componentManager, _ := m.services["component-manager"].(*ComponentManager)`; pin `:605-613` builds the writer and calls
`componentManager.setStartupMetricWriter(writer)`; pin `service/component_manager.go:756-767` push and setter; pin
`:1848-1849` — `healthByName := componentManager.GetComponentHealth()` / `if len(healthByName) !=
componentCounts.Admitted {`, reachable at the pin because `GetComponentHealth` skipped nil components (pin `:1323`).

## Shape sweep

Shape 3 (one implementation): `service/service_manager.go:40`, introduced by PR 2 D3, not the pin. No other shape.

## Adopter seam inventory

Reached from outside: the gauge's exposition, `/readyz`, `/services`, `RegisterInstance`/`Registry.Register` of any
`component-manager`. An adopter must know nothing new: label set, values and refusal of a foreign collector are
unchanged; a non-framework `component-manager` gets the pin's zero-count path as today.
