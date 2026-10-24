# Tasks: setup-04a-03-service-startup-seam

Port-refactor #364. Behaviour is preserved, so the oracle is that the named tests pass on the base and after each
task; a task's new or rewritten tests must pass on the base before its production change (record it in
`notes/<task>.md`, with any compile failure from a rename). Each task ends only when `task verify` passes on a clean
tree at its commit (#171). Review stops (Codex) after the design, after task 2.5 and after the docs.

## 1. Design

- [ ] 1.1 Inventory and design pass review and are accepted by the owner. Gate: `task spec:check` and
  `task docs:check`.

## 2. Refactor

- [ ] 2.1 Tests first, the gauge (design D1, "Proving tests"): in `service/startup_observability_test.go` and
  `service/startup_observability_amendment_test.go`, call `registerStartupCollector(registry, serviceSnapshot,
  componentSnapshot) error` in place of `newStartupMetricWriter`; drop both `cm.setStartupMetricWriter(writer)` lines;
  rename `TestStartupCollectorReportsExactlyNineSeries` and
  `TestStartupCollectorRejectsForeignCollectorInsteadOfAdoptingIt`; add
  `TestComponentManagerOnItsOwnPublishesNoStartupGauge` and
  `TestStartupGaugeReadsZeroComponentsForAStandInComponentManager` (a manager whose `component-manager` is
  `newStandInComponentManager` gathers the five component series at zero); make `TestStartupGaugeVecHasOnlyPrivateServicePackageOwner`
  find `semengine_startup_units` in exactly one non-test file. Gate: compile failure on the base noted.
- [ ] 2.2 The collector (design D1): `service/startup_metrics.go` holds the collector and `registerStartupCollector`;
  `service/service_manager.go` registers it in `initializeStartupMetrics` and drops the `startupMetrics` field and the
  `publishServices` calls; `service/component_manager.go` and `service/component_manager_lifecycle.go` drop the
  writer field, `setStartupMetricWriter` and `recordComponentStartupMetrics`. One compile unit. Gate:
  `go test -race ./service/` and `task verify`.
- [ ] 2.3 Tests first, the concrete component manager (design D2, "Proving tests"): rewrite
  `service/component_startup_test.go` on `newTestComponentManager`, deleting `componentStartupDouble`; cases
  `TestComponentStartupServesStartupRoutesCountsAndComponentRoutes`, `TestComponentStartupGatesReadiness` (healthy,
  unhealthy, starting, failed, stopping), `TestConfigureRefusesACompositionWithoutAComponentManager`. Gate: green on the
  base plus 2.2.
- [ ] 2.4 Callers take the concrete type (design D2): `service/service_manager_http.go` (`:65`, `:272`, `:478`; drop
  the check at `:514`; `startupComponentPrefix` comment) and `service/rule_pack_bind.go` (`:41` and its doc). Gate:
  `task verify`.
- [ ] 2.5 The interface goes (design D2): `service/service_manager.go` (`:34-47`, `:516`) and
  `service/component_manager_http.go` (`:22-25`). Gate: `task verify`.
- [ ] 2.6 Sensitivity (`notes/2.6.md`): each mutant detected or reported as a survivor: the collector caches the
  component counts at registration (`TestComponentStartupSnapshotDistinguishesAdmittedAndLifecycleStarts`); the
  collector omits `starts_failed` (`TestStartupCollectorReportsExactlyNineSeries`); the manager treats every
  `component-manager` as a stand-in (`TestComponentStartupGatesReadiness`); `ComponentsImplementing` returns nil for
  the framework's manager (`components_implementing_test.go`).

## 3. Docs, ledger and archive

- [ ] 3.1 `service/README.md` names the gauge beside `/services`; `docs/admission-ledger.yaml:2996` drops
  "withComponents and the post-construction startup metric setter stay for the service manager (#364)" and states the
  collected gauge.
- [ ] 3.2 Spec delta synced into `openspec/specs/operator-surface/spec.md`; archive as the last content commit.
