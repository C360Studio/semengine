# Tasks: setup-04a-03-service-startup-seam

Port-refactor #364. Behaviour is preserved, so the oracle is that the named tests pass on the base and after each
task; a task's new or rewritten tests must pass on the base before its production change (record it in
`notes/<task>.md`, with any compile failure from a rename). Each task ends only when `task verify` passes on a clean
tree at its commit (#171). Review stops (Codex) after the design, after task 2.3 and after the docs.

## 1. Design

- [x] 1.1 Inventory and design pass review and are accepted by the owner (Codex INVENTORY PASS and design APPROVE on
  `c90aee2`, PR #401; owner 2026-10-24: no design stop when the fix is clean). Gate: `task spec:check` and
  `task docs:check`.

## 2. Refactor

- [ ] 2.1 Tests first, the concrete component manager (design D2, "Proving tests"): rewrite
  `service/component_startup_test.go` on `newTestComponentManager`, deleting `componentStartupDouble`; cases
  `TestComponentStartupServesStartupRoutesCountsAndComponentRoutes`, `TestComponentStartupGatesReadiness` (healthy,
  unhealthy, starting, failed, stopping), `TestConfigureRefusesACompositionWithoutAComponentManager`. The rewritten
  tests drive a real `ComponentManager`, which implements `componentStartup` on the base, so they pass on the base with
  no production change. Gate: `task verify`.
- [ ] 2.2 One compiling unit: the collector and the concrete type (design D1 and D2). Tests first, in the working tree
  only: in `service/startup_observability_test.go` and `service/startup_observability_amendment_test.go`, call
  `registerStartupCollector(registry, serviceSnapshot, componentSnapshot) error` in place of `newStartupMetricWriter`;
  drop both `cm.setStartupMetricWriter(writer)` lines; rename `TestStartupCollectorReportsExactlyNineSeries` and
  `TestStartupCollectorRejectsForeignCollectorInsteadOfAdoptingIt`; add
  `TestComponentManagerOnItsOwnPublishesNoStartupGauge` and
  `TestStartupGaugeReadsZeroComponentsForAStandInComponentManager` (a manager whose `component-manager` is
  `newStandInComponentManager` gathers the five component series at zero); make
  `TestStartupGaugeVecHasOnlyPrivateServicePackageOwner` find `semengine_startup_units` in exactly one non-test file.
  Record the base compile failure in `notes/2.2.md`. Then, in the same commit:
  `service/startup_metrics.go` holds the collector and `registerStartupCollector`; `service/service_manager.go`
  registers it in `initializeStartupMetrics`, drops the `startupMetrics` field and the `publishServices` calls, and
  drops `componentStartup` (`:34-47`, `:516`); `service/component_manager.go` and
  `service/component_manager_lifecycle.go` drop the writer field, `setStartupMetricWriter` and
  `recordComponentStartupMetrics`; `service/component_manager_http.go` drops the assertion (`:22-25`);
  `service/service_manager_http.go` (`:65`, `:272`, `:478`; drop the check at `:514`; `startupComponentPrefix`
  comment) and `service/rule_pack_bind.go` (`:41` and its doc) check `.(*ComponentManager)`. Gate:
  `go test -race ./service/` and `task verify`.
- [ ] 2.3 Sensitivity (`notes/2.3.md`): each mutant detected or reported as a survivor: the collector caches the
  component counts at registration (`TestComponentStartupSnapshotDistinguishesAdmittedAndLifecycleStarts`); the
  collector omits `starts_failed` (`TestStartupCollectorReportsExactlyNineSeries`); the manager treats every
  `component-manager` as a stand-in (`TestComponentStartupGatesReadiness`); `ComponentsImplementing` returns nil for
  the framework's manager (`components_implementing_test.go`).

## 3. Docs, ledger and archive

- [ ] 3.1 `service/README.md` names the gauge beside `/services`; `docs/admission-ledger.yaml:2996` drops
  "withComponents and the post-construction startup metric setter stay for the service manager (#364)" and states the
  collected gauge.
- [ ] 3.2 Spec delta synced into `openspec/specs/operator-surface/spec.md`; archive as the last content commit.
