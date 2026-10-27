# Tasks: refactor-362-remove-migration-overrides

Removal of `stream_migration_overrides` (#362; #423 closes with it). Owner rulings: #362 comments 6439894887 and
6440188996.

- Tests first. A new test that compiles on the base runs on the base before the production change, and its failure
  (or its pass, for a test that pins kept behaviour) is recorded in `notes/<task>.md`.
- Developers do not commit; the orchestrator commits each task.
- Each task ends only when `task verify` passes on a clean tree at its commit (#171).
- Tasks 2.1 and 2.2 are each one compile unit: a removed symbol and every reader of it leave in the same task.
- Review stops (Codex) come after the design (1.1) and after the code (2.5).

## 1. Design

- [x] 1.1 Inventory and design pass review and are accepted by the owner. Codex design review PASS on PR #421
  (MEDIUM folded into D1); owner accepted 2026-10-27 (Enjoy).
  Gate: `task spec:check` and `task docs:check`.

## 2. Code

- [x] 2.1 The component manager runs one loop (design D4, `design.md:98-124`).
  - Files: delete `service/stream_override_expiry.go`; edit `service/component_manager.go`,
    `service/component_manager_lifecycle.go`.
  - Tests first, in `service/component_manager_supervisor_test.go` and `service/stream_override_expiry_test.go`: move
    `syncBuffer` and the helper under its old name and signature, and move and rename the failed-`Start` test to
    `TestComponentManagerFailedStartLaunchesNoPublisher` without its reporter injection. Run it and
    `TestComponentHealthPublishFailureIsCounted` on the base; both pass (`notes/2.1.md`).
  - Then delete the reporter, its field, construction and `supervise`'s second goroutine; delete the rest of
    `stream_override_expiry_test.go` and `TestOverrideGaugeUsesTheRegisteredCollector`; only now rename the helper to
    `composeComponentManager` and drop its overrides parameter; edit the comments at
    `component_manager.go:90-91`, `component_manager_lifecycle.go:24`, `component_manager_suite_test.go:93` and the
    header of `component_manager_supervisor_test.go:15-18`.
  - Gate: `go test -race ./service/` and `task verify`.
- [x] 2.2 The key goes, and validation returns the archival list (design D1, D2, D3 and D6, `design.md:33-96`,
  `:140-148`; Ledger, first three bullets). One compile unit: removing the override types breaks every reader of
  `StreamExceptionReport`, so the report type leaves in the same task.
  - Files: `config/config.go`, `config/stream_bounds.go`, `config/streams.go`, `config/stream_drift.go`, their tests
    (`stream_bounds_test.go`, `stream_bounds_integration_test.go`, `stream_drift_test.go`,
    `stream_drift_integration_test.go`, `load_test.go`) and `docs/admission-ledger.yaml` (`:2784`, `:2794` only).
  - Tests first: add `TestConfigValidate_MigrationOverrideKeyAdmitsNothing` to `config/stream_bounds_test.go` and
    record its failure on the base (`notes/2.2.md`).
  - Then remove the surface and tests D1 lists, drop the `now` parameter (D3), and edit `boundsRemedy`'s second line
    and the override comments at `config.go:56-66`, `:217-219`, `stream_bounds.go:51`, `:61`, `:439`, `:503-506`,
    `streams.go:25-26`, `:324` and `stream_drift.go:229`.
  - Delete `governedBounds`, `boundsGovernedBy` and `declaresDiscard` and compare the three fields directly in
    `reconcileStreamConfig` (design D1, last paragraph); the existing drift tests pass unchanged.
  - Delete `StreamExceptionReport` and `TestStreamExceptionReport_KindsAreStructurallyDistinct`; return the list
    from `planStreams` and `ValidateStreamDeclarations`; move the assertions at `stream_bounds_test.go:207`, `:688`
    and the archival tests to the list; name the service in the archival hint; correct the readiness comments D6
    lists. The log subtest at `:761-785` becomes one archival entry asserting one `level=INFO` line that names the
    stream, has no `expires`, and contains `storage-observability`; the hint assertion's ability to fail is shown by
    the mutant in 2.5.
  - Move both ledger rows from `carry` to `adapt`, with the changed-from-pin text of design Ledger, in this task, or
    `task ledger:check` fails.
  - Record `git grep -n "time.Now" -- config/stream_bounds.go config/config.go composition/` returning nothing.
  - Gate: `go test -race ./config/`, `go test -tags integration ./config/`, `task ledger:check`,
    `task cover:check` and `task verify`.
- [x] 2.3 The seam's remedy names one exception (design D1, `design.md:40-42`).
  - Files: `natsclient/stream_bounds.go` (`:6`, `:112-114`) and `natsclient/stream_bounds_test.go` (`:102`, the line
    asserting the removed key, deleted; the subtest's name says it names the archival escape).
  - Gate: `go test -race ./natsclient/` and `task verify`.
- [x] 2.4 The example alert rule goes (design D5, `design.md:126-136`).
  - File: `configs/prometheus/rules/storage-pressure.yml`: delete `:298-319`, edit `:38-39`, add the header note.
  - The four `TestStorageAlertRules_*` tests pass unchanged.
  - Gate: `go test -race -run TestStorageAlertRules ./service/` and `task verify`.
- [ ] 2.5 Sensitivity and code review (`notes/2.5.md`): report each mutant as detected (naming the test) or as a
  survivor, with `task mutate:check`, unit tests only.
  - `supervise` closes `done` before `publishHealthLoop` returns (`TestComponentManagerPassesTheLifecycleSuite` or
    `TestComponentHealthPublishFailureIsCounted`).
  - `planStreams` returns a nil archival list (the archival tests, `stream_bounds_test.go:668-785`).
  - The archival hint drops the service name (the log subtest of 2.2).
  - Hold: Codex code review on PR #421, citing the commit and a local `task verify`.

## 3. Docs, ledger and archive

- [ ] 3.1 Docs and ledger (technical writer; design D5, D6 and Ledger, `design.md:126-168`):
  - `config/README.md:36`, `:302-313` (the override sentence goes; the storage observability sentence), `config/doc.go:26`,
    `:155-156`.
  - `service/README.md:274`, `:313-317`, `:665`; `service/doc.go:238`, `:274-278`.
  - `docs/admission-ledger.yaml`: the `config`, `natsclient` and `service` row edits in design "Ledger", including
    the stale `:3021` #363 sentence.
  - Gate: `task docs:check` and `task ledger:check`.
- [ ] 3.2 Sync the four spec deltas into `openspec/specs/` and archive, as the last content commit.
  - Gate: `task spec:check` and `task docs:check`.
