# Tasks: setup-04a-03-boot-service-component-manager

Each task names its outcome and the gate that proves it. This change ports the component manager of `service` (PR 3
of 7 in the split the owner accepted, #9 comment 6296023694). Tests come first. Each developer unit below is one or two
files; its pin line ranges and the names its tests call are in `notes/2.1.md` or `notes/2.2.md`, so a unit's brief
cites only that section and the design sections it names. Record in the note how each test fails on the base commit,
or which mutant it detects. Review stops (Codex) come after 2.2 and 3.3. The `task verify` gate (#171) applies to 2.1
and 2.2 as wholes, met
at the 2.2f commit: a 2.1 or 2.2a-2.2e sub-unit is ticked on its own stated gate (build and vet with the overlay
stub), since `go vet ./service/` cannot pass before the last unit lands. Every other task ends only when `task
verify` passes on a clean tree at its commit.

## 1. Design

- [x] 1.1 Shape sweep (`sweep.md`) and design (`design.md`) passed the pre-owner review (round 1 at `61ffb6d`, round 2
  at `5393ce7`, nits fixed at `a32ee4a`) and were accepted by the owner on 2026-10-21 (#359 comment, owner's word):
  Q-B yes (boot fails, naming the components); Q-A withdrawn as a question, ADR-113 governs (option A). The probes'
  output and the build probe's commands and diff are on #359 (comments 6324262845 and 6324713111). Gate:
  `task spec:check` and `task docs:check`; each comment read back.
- [x] 1.2 The spec deltas design D9 names, with Q-B's answer and ADR-113's rule, under `specs/service-lifecycle/` and
  `specs/operator-surface/`; `skip_specs` removed from `.openspec.yaml`. Gate: `task spec:check` (strict validation
  with the deltas present; exit 0).
- [x] 1.3 The lock-simplification amendment (owner, 2026-10-21, #359 comment 6325195862: design D2 "State shape" and
  "No wait for readers", D4, D5, D7, D8, Generated checks; sweep C10, C11) passes review: round 3 PASS WITH FINDINGS
  at `3708774` (no BLOCKING or HIGH; two MEDIUM, three NIT), findings applied. Gate: `task spec:check` and
  `task docs:check`.

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
  - [ ] 2.1e Composition projections (D3, D5): `component_manager_port_facts_test.go`,
    `component_manager_paths_test.go`.
  - [ ] 2.1f Routes and the golden flowgraph (D5): `component_manager_http_test.go` (with `testdata/`),
    `attack_test.go`.
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
  - [ ] 2.1o The state shape and the manager's health (D2 "State shape" and "No wait for readers", D4, #360):
    `component_manager_state_test.go`.
  - [ ] 2.1p graph-ingest answers reads during and after its `Stop` (D2 "No wait for readers"):
    `processor/graph-ingest/discoverable_reads_test.go`. It passes on the base; its gate is D7's mutant (task 2.3)
    instead of a failure on the base.
- [ ] 2.2 The five pin files, the module path rewritten, as D1-D6 say, one unit per commit (`notes/2.2.md`). Every
  2.1 test file is in package `service`, so no unit's tests can run until all six units have landed. Gate for 2.2a-2.2e:
  `go build ./service/ ./pkg/projection/` and `go vet ./service/` (with and without `-tags integration`) pass with a
  throwaway stub for what the later units add, supplied through `-overlay` and kept outside the repository, as 2.1
  does. Gate for 2.2f, the last: `go test -race ./service/ ./pkg/projection/` and `go test -tags integration ./service/`
  pass, `go build ./...`, and `task verify` passes on a clean tree (review stop). "Exercised by" names the 2.1 units
  whose tests reach that unit's code.
  - [ ] 2.2a `PredicateReconciler` and the rule-pack binding (D6): `pkg/projection/mutation_types.go`,
    `service/rule_pack_bind.go`. Exercised by 2.1i (with 2.2c-2.2d for a real manager).
  - [ ] 2.2b The override reporter and the empty configuration (D1, D4): `service/stream_override_expiry.go`,
    `service/component_manager_config.go`. Exercised by 2.1d, 2.1h.
  - [ ] 2.2c Construction, composition, stores, projections (D1, D3): `service/component_manager.go`. Exercised by
    2.1c, 2.1d, 2.1e (with 2.2e), 2.1k, 2.1l, 2.1o.
  - [ ] 2.2d Start, Stop, phases and the supervisor (D2, D4): `service/component_manager_lifecycle.go`,
    `service/startup_metrics.go`. Exercised by 2.1a, 2.1b, 2.1c, 2.1h, 2.1j (with 2.2c and 2.2e), 2.1n, 2.1o.
  - [ ] 2.2e Routes (D5) and `var _ componentStartup = (*ComponentManager)(nil)`, whose six methods 2.2c-2.2e
    provide: `service/component_manager_http.go`. Exercised by 2.1e, 2.1f, 2.1g, 2.1j, 2.1o.
  - [ ] 2.2f Registration, the component manager's OpenAPI spec (D5 option a), the stale field comment (D1, sweep
    C12) and the read contract line on `LifecycleComponent` (D2, "No wait for readers"): `service/register.go`,
    `component/lifecycle.go`. Exercised by 2.1g, 2.1k-m, 2.1o, 2.1p.
- [ ] 2.3 Sensitivity: each mutant D7 lists makes its named test fail and the revert makes it pass, through
  `task mutate:check` for the unit tests and by hand with `-tags integration` where a test needs NATS (#129), recorded
  in `notes/2.3.md` with the `rapid` seed and check count. Gate: `task verify` passes on a clean tree.

## 3. Text, ledger, guidance, ADRs (D8)

- [ ] 3.1 `service/README.md` and `service/doc.go` for the component manager, with D1's contract lines and D8's declared
  degrades; pin ADR-095 and ADR-096 in `docs/adr/` with port notes; ADR-094's port note rewritten for the component
  manager's part; a dated note under ADR-113's Decision naming the composition root as what reacts to the root
  context ending (design D2, "Who reacts"; what `:66-69` left to change 3, no new ruling). Gate: `go doc ./service`
  lists only the kept surface; `task
  docs:check`.
- [ ] 3.2 The `SchemaRef` rule (D8 Guidance) in the developer and reviewer contracts, with its rows in `AGENTS.md` § 1
  and `.agents/rules-index.md`; the `service` ledger row edited in place, the `pkg/projection` row's sentence, the
  ADR-095 and ADR-096 rows; the `docs/repository-map.md` rows. Gate: `task verify` passes on a clean tree
  (`ledger:check`, `cover:check` with `service` at or above its floor, the number recorded on the PR, `docs:check`, the
  `AGENTS.md` size and row-count checks).
- [ ] 3.3 #196 told that the component manager composes the guard; #195 told that this PR's 12 test clients use
  `fixtureClient`; #280 told that item 2 is the message logger's (D8); the PR body declares `Closes #360` (D8).
  #361-#364 stay open, listed in D8 as told. Any defect found and not fixed is filed
  (`status:blocked` on #359, linked without a closing keyword). Gate: each comment and issue read back (review stop).

## 4. Verify, review and close

- [ ] 4.1 `task verify` passes on a clean tree at the implementation head; the coverage of `service` is recorded on
  the PR.
- [ ] 4.2 Cross-agent review of record at both stops, as PR comments; `reviewed-by:` in the PR body.
- [ ] 4.3 The change is archived (`openspec archive setup-04a-03-boot-service-component-manager`) in the last content
  commit, syncing `service-lifecycle` and `operator-surface` into `openspec/specs/`. Gate: `task spec:check`.
