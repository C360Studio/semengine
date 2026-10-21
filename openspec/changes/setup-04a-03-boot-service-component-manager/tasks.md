# Tasks: setup-04a-03-boot-service-component-manager

Each task names its outcome and the gate that proves it. This change ports the component manager of `service` (PR 3
of 7 in the split the owner accepted, #9 comment 6296023694). Tests come first. Each developer unit below is one or two
files; its pin line ranges and the names its tests call are in `notes/2.1.md` or `notes/2.2.md`, so a unit's brief
cites only that section and the design sections it names. Record in the note how each test fails on the base commit,
or which mutant it detects. Review stops (Codex) come after 2.2 and 3.3. Each task ends only when `task verify` passes
on a clean tree at its commit (#171).

## 1. Design

- [ ] 1.1 Shape sweep (`sweep.md`) and design (`design.md`) pass the pre-owner review and are accepted by the owner,
  including the owner's answers to Q-A (each component's Start context, ADR-113) and Q-B (a component that cannot be
  built); the probe note (sweep § 3, source and output) is posted on #359. Gate: `task spec:check` and
  `task docs:check`; each comment read back.
- [ ] 1.2 The spec deltas design D9 names, with the owner's answers, under `specs/service-lifecycle/` and
  `specs/operator-surface/`; `skip_specs` removed from `.openspec.yaml`. Gate: `task spec:check`.

## 2. Port (D1-D7)

- [ ] 2.1 Tests first, one unit per commit (`notes/2.1.md`, one section each). Gate for each unit: its tests fail on
  the base (the names do not exist; the compile errors noted), and `go vet` passes with a throwaway stub supplied
  through `-overlay` and kept outside the repository, as PR 2 did.
  - [ ] 2.1a Helpers and the lifecycle-suite adapter (D2, D7): `component_manager_helpers_test.go`,
    `component_manager_suite_test.go`.
  - [ ] 2.1b Start, Stop and Q-A's tests (D2): `component_manager_lifecycle_test.go`, `component_manager_owner_test.go`.
  - [ ] 2.1c Phases and stores (D2, D3): `component_manager_start_barrier_test.go`,
    `component_manager_storeregistry_test.go`.
  - [ ] 2.1d Construction, the empty configuration and Q-B (D1, D3): `component_manager_rulepack_init_test.go`,
    `component_manager_boot_only_test.go`.
  - [ ] 2.1e Composition projections (D3, D5): `component_manager_port_facts_test.go`, `component_manager_paths_test.go`.
  - [ ] 2.1f Routes and the golden flowgraph (D5): `component_manager_http_test.go` (with `testdata/`), `attack_test.go`.
  - [ ] 2.1g Type catalog and the OpenAPI document (D5): `component_catalog_test.go`,
    `component_manager_gaps_removed_test.go`.
  - [ ] 2.1h The supervisor (D4): `stream_override_expiry_test.go`, `component_manager_supervisor_test.go`.
  - [ ] 2.1i Rule-pack binding (D6): `components_implementing_test.go`, `rule_pack_bind_test.go`.
  - [ ] 2.1j PR 2's cut startup tests (archive `2026-10-21-setup-04a-03-boot-service-manager/notes/2.1.md` parts 3
    and 5): `startup_observability_test.go`, `startup_observability_amendment_test.go`.
  - [ ] 2.1k Integration, boot findings (D3, D5): `component_manager_integration_test.go`,
    `component_manager_boot_findings_integration_test.go`.
  - [ ] 2.1l Integration, boot-only and bucket guards (D3): `component_manager_boot_only_integration_test.go`,
    `framework_owned_bucket_guards_integration_test.go`.
  - [ ] 2.1m Integration, the process restart (D7): `config_boot_process_restart_integration_test.go`,
    `fixture_client_integration_test.go`.
  - [ ] 2.1n The Start/Stop model check (Generated checks): `component_manager_prop_test.go`.
- [ ] 2.2 The five pin files, the module path rewritten, as D1-D6 say, one unit per commit (`notes/2.2.md`). Gate for
  each unit: the 2.1 units it names pass under `-race`; for the last, `go test -race ./service/ ./pkg/projection/` and
  `go test -tags integration ./service/` pass, `go build ./...`, and `task verify` passes on a clean tree (review stop).
  - [ ] 2.2a `PredicateReconciler` and the rule-pack binding (D6): `pkg/projection/mutation_types.go`,
    `service/rule_pack_bind.go`. Unblocks 2.1i.
  - [ ] 2.2b The override reporter and the empty configuration (D1, D4): `service/stream_override_expiry.go`,
    `service/component_manager_config.go`. Unblocks 2.1h's reporter tests.
  - [ ] 2.2c Construction, composition, stores, projections (D1, D3): `service/component_manager.go`. Unblocks 2.1d,
    2.1e.
  - [ ] 2.2d Start, Stop, phases and the supervisor (D2, D4): `service/component_manager_lifecycle.go`,
    `service/startup_metrics.go`. Unblocks 2.1a-c, 2.1h, 2.1j, 2.1n.
  - [ ] 2.2e Routes and the OpenAPI schemas (D5): `service/component_manager_http.go`,
    `service/service_manager_http.go`. Unblocks 2.1f, 2.1g.
  - [ ] 2.2f Registration and the stale field comment (D1, sweep C12): `service/register.go`, `component/lifecycle.go`.
    Unblocks 2.1k-m.
- [ ] 2.3 Sensitivity: each mutant D7 lists makes its named test fail and the revert makes it pass, through
  `task mutate:check` for the unit tests and by hand with `-tags integration` where a test needs NATS (#129), recorded
  in `notes/2.3.md` with the `rapid` seed and check count. Gate: `task verify` passes on a clean tree.

## 3. Text, ledger, guidance, ADRs (D8)

- [ ] 3.1 `service/README.md` and `service/doc.go` for the component manager, with D1's contract lines and D8's
  declared degrades; pin ADR-095 and ADR-096 in `docs/adr/` with port notes; ADR-094's port note rewritten for the
  component manager's part. Gate: `go doc ./service` lists only the kept surface; `task docs:check`.
- [ ] 3.2 The `SchemaRef` rule (D8 Guidance) in the developer and reviewer contracts, with its rows in `AGENTS.md` § 1
  and `.agents/rules-index.md`; the `service` ledger row edited in place, the `pkg/projection` row's sentence, the ADR-095
  and ADR-096 rows; the `docs/repository-map.md` rows. Gate: `task verify` passes on a clean tree (`ledger:check`,
  `cover:check` with `service` at or above its floor, the number recorded on the PR, `docs:check`, the `AGENTS.md`
  size and row-count checks).
- [ ] 3.3 #196 told that the component manager composes the guard; #195 told that this PR's 12 test clients use
  `fixtureClient`; #280 told that item 2 is the message logger's (D8). Any defect found and not fixed is filed
  (`status:blocked` on #359, linked without a closing keyword). Gate: each comment and issue read back (review stop).

## 4. Verify, review and close

- [ ] 4.1 `task verify` passes on a clean tree at the implementation head; the coverage of `service` is recorded on
  the PR.
- [ ] 4.2 Cross-agent review of record at both stops, as PR comments; `reviewed-by:` in the PR body.
- [ ] 4.3 The change is archived (`openspec archive setup-04a-03-boot-service-component-manager`) in the last content
  commit, syncing `service-lifecycle` and `operator-surface` into `openspec/specs/`. Gate: `task spec:check`.
