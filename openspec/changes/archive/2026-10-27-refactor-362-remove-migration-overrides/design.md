# Design: refactor-362-remove-migration-overrides

Base `2a712fa`. Inventory: `inventory.md` (INVENTORY PASS at `f6fec97` plus that file, three LOW findings folded in).
Owner rulings: #362 comment 6439894887 (remove the overrides) and comment 6440188996 (the four removal details).
Retires #362 and #423 (`Closes #362`, `Closes #423`; tasks 2.1-2.2). Every `path:line` below is at the base.

## Context

- `stream_migration_overrides` (`config/config.go:60`) admits a named existing stream without finite bounds until a
  declared expiry. Validation refuses a lapsed entry, which reads the clock (`config/stream_bounds.go:610`).
- The component manager hosts `streamOverrideExpiryReporter` (`service/stream_override_expiry.go:48`): it registers
  the gauge at construction and evaluates once (`service/component_manager.go:234-253`), and `supervise` runs it beside
  the health publisher, joining both (`service/component_manager_lifecycle.go:212-225`).
- No configuration in this tree declares the key except tests, and no starter consumer reads a removed symbol or
  `StreamExceptionReport` (inventory § 1 and § 3).
- A document with an unknown top-level key loads (`config/doc.go:48`; ledger `docs/admission-ledger.yaml:2766`).
- `StreamExceptionReport` is read in production only by `logStreamExceptions` (`config/streams.go:335`) and the boot
  count log (`:345-346`). `ValidateStreamDeclarations`' one caller discards it (`config/config.go:270`).

## Options

The owner chose the shape; these are the choices the rulings settled, and the two left to the design.

- **Keep the overrides and move the reporter** (the earlier placement inventory): superseded by comment 6439894887.
- **Refuse the old key by name**, as `removedPlatformFields` refuses `platform.instance_id` (`config/config.go:693-715`):
  ruled out (6440188996, item 1). The refusal it would add buys nothing the bounds refusal does not already report.
- **Keep `StreamExceptionReport{Archival}`**: ruled out (item 2); the list is the whole answer.
- Left to the design: `streamExemption` keeps its two remaining values, `exemptNone` and `exemptArchival`
  (`stream_bounds.go:174-183`), because `resolveBounds` (`config/streams.go:462-467`) switches on it; a bool would
  change that reader for no behaviour. Drift stops reading it (D1, Codex design review on PR #421).
  `validateExceptionDeclarations` keeps its name. Both are the smaller diff.

## D1. The key goes, and an old key admits nothing

Observable behaviour:

- `LoadFile` and `LoadFromBytes` accept a document that sets `stream_migration_overrides`, and the entry has no effect.
- `Validate`, `ValidateDocument` and `EnsureStreams` refuse a stream the entry named that lacks bounds with
  `ErrStreamBoundsUndeclared`, naming the stream, its source and each missing field (`stream_bounds.go:451-462`).
- `boundsRemedy` (`stream_bounds.go:471-477`) and `natsclient`'s `boundsAtSeamRemedy`
  (`natsclient/stream_bounds.go:112-114`) name `archival_streams` as the one exception and no longer name the key.

Removed (inventory § 1 site list): `StreamMigrationOverride`, `StreamMigrationOverrides`,
`Config.StreamMigrationOverrides`, `StreamMigrationOverrideStatus`, `ErrStreamMigrationOverrideInvalid`,
`ErrStreamMigrationOverrideExpired`, `ExpiredMigrationOverrides`, `exemptMigrationOverride`, `parseOverrideExpiry`,
`missingOverriddenStreamRemedy`. Their branches go with them: the both-declared check (`stream_bounds.go:530-535`), the
override loop (`:543-577`), the Warn loop in `logStreamExceptions` (`:624-632`), the boot line's `migration_overrides`
field (`streams.go:345`), the `resolveBounds` case (`streams.go:468-473`), the refusal to create an overridden stream
(`streams.go:602-621`) and the drift branch (`stream_drift.go:103-115`).

With that branch gone, every stream governs all three retention fields, so the drift helpers that chose which fields
to reconcile have nothing left to choose (Codex design review on PR #421, MEDIUM): `governedBounds`,
`boundsGovernedBy` and `declaresDiscard` (`stream_drift.go:86-124`) are deleted, and `reconcileStreamConfig`
compares age, bytes and discard directly (`:217-245`, the three `gov.` guards go). Behaviour is unchanged for
ordinary and archival streams; the drift tests that pin it, `TestCreateStream_ArchivalDriftRepairsToUnlimited`
among them, stay and must pass unchanged.

Proving tests:

- New, `TestConfigValidate_MigrationOverrideKeyAdmitsNothing` (`config/stream_bounds_test.go`): one document declares
  a `streams` entry with no bounds and a `stream_migration_overrides` entry for it expiring in 2099. `LoadFromBytes`
  returns no error, and `Validate` returns `ErrStreamBoundsUndeclared` whose text names the stream and `max_age`. It
  fails on the base, where the override admits the stream.
- Kept: `TestConfigValidate_OrdinaryStreamMustDeclareBounds` (`stream_bounds_test.go:79`); the unknown-key load case
  (`config/load_test.go:379`); the archival tests (`stream_bounds_test.go:668-785`, less the subtests at `:725-740`).
- Deleted with the surface: `TestMigrationOverride`, `TestExpiredMigrationOverrides`, the subtests at
  `stream_bounds_test.go:725-740` and `:891-901`, the override cases of the round trip (`:813-872`),
  `stream_bounds_integration_test.go:156-233`, `stream_drift_test.go:230-282`,
  `stream_drift_integration_test.go:286-351`, and the override lines of `load_test.go:391,408` and
  `natsclient/stream_bounds_test.go:102`.

## D2. Validation returns the archival list

- `ValidateStreamDeclarations(cfg *Config) ([]ArchivalStreamStatus, error)`: one entry per `archival_streams` entry,
  sorted by stream name, nil when there is none.
- `StreamExceptionReport` is deleted. `ArchivalStreamStatus` (`stream_bounds.go:149`) stays as the list's element.
- `planStreams` returns the declarations, the list and the error; `logStreamExceptions` takes the list and logs one
  Info line per archival stream.
- The boot line "Ensured JetStream streams" keeps `count` and `archival` and loses `migration_overrides`
  (`streams.go:345`).
- Who reads it: `logStreamExceptions` and the count log; `ValidateStreamDeclarations` stays exported for `Validate`'s
  path and the README's documented call (`config/README.md:311-312`). No consumer reads it (inventory § 1).

Proving tests: the archival tests' assertions move from `report.Archival` to the list unchanged in value.
`TestStreamExceptionReport_KindsAreStructurallyDistinct` (`stream_bounds_test.go:791-804`) is deleted: the type it pins
is gone. The log subtest at `:761-785` becomes one archival entry and one line (D6).

## D3. Validation reads no clock

- The `now time.Time` parameter leaves `planStreams` and `validateExceptionDeclarations` (`stream_bounds.go:440`,
  `:507`); `ValidateStreamDeclarations` (`:610`) and `EnsureStreams` (`streams.go:330`) stop calling `time.Now`.
  Leaving it would fail `task lint` (`revive.toml:31`, `unused-parameter`).
- `ValidateDocument` reads nothing outside the document; its doc comment (`config/config.go:217-219`) says so.
- #423 cannot arise: `SafeConfig.Mutate` (`config/config.go:138`) validates with no time-dependent input left.
- Proof: by removal. Task 2.2 records `git grep -n "time.Now" -- config/stream_bounds.go config/config.go composition/`
  returning nothing. `TestValidateReadsNoTLSFile` stays unchanged as the requirement's scenario.

## D4. The component manager runs one loop

Observable behaviour:

- `NewComponentManager` registers no `semengine_streams_migration_override_expired`.
- `Start` launches the health publisher alone; `Stop` returns only after it returned; a failed `Start` launches none.

Removed: `service/stream_override_expiry.go`, the `overrideExpiry` field (`component_manager.go:73-79`), its
construction (`:234-253`) and the `time` import if nothing else reads it (`:14`). `supervise` runs
`publishHealthLoop` and closes `done` when it returns, with no second goroutine to join. ADR-113 holds as before: the
loop runs under `Start`'s one run context, which `Stop` ends.

Tests (`service/`):

- Kept unchanged: `TestComponentHealthPublishFailureIsCounted` (nothing published after `Stop`,
  `component_manager_supervisor_test.go:55-59`) and `TestComponentManagerPassesTheLifecycleSuite` (the supervisor is
  held while `done` is open).
- Moved to `component_manager_supervisor_test.go` and renamed `TestComponentManagerFailedStartLaunchesNoPublisher`:
  the failed-`Start` test (`stream_override_expiry_test.go:350-372`). It no longer injects a reporter; it keeps the
  `done` and health-publish assertions, proving "A failed `Start` SHALL launch no publisher".
- Moved to the same file: `syncBuffer` (`:196-208`), and `composeComponentManagerWithOverride` (`:130-162`) as
  `composeComponentManager`, without its overrides parameter.
- Deleted: the rest of `stream_override_expiry_test.go` (with `heldConfigSource`, `:253-275`) and
  `TestOverrideGaugeUsesTheRegisteredCollector` (`component_manager_supervisor_test.go:63-85`).

Spec: the REMOVED requirement (`service-lifecycle/spec.md:520-539`), and two MODIFIED requirements that drop "the
override reporter" (`:368`, `:407`).

## D5. The example alert rule goes; the rules file says what feeds it

- `StorageStreamMigrationOverrideExpired` (`configs/prometheus/rules/storage-pressure.yml:298-319`) is deleted, and
  the header's rule 2 (`:38-39`) names an archival stream as the unbounded case.
- A header note after `:12`: every series these rules read is published by the storage observability service and
  exists only while it is enabled (`"storage-observability": {"enabled": true}` under `services`); with it off, no
  rule here fires.
- `service/README.md:665`: 13 alerts become 12.
- Proof: `TestStorageAlertRules_ReferenceOnlyPublishedMetrics`, `_KeyNothingOnRowDisappearance`,
  `_MakeUnboundedResourcesVisible` and `_AreAnnotatedAndReportOnly` pass unchanged; their regex never read the
  removed name (`service/storage_observability_http_test.go:531`). Spec: `operator-surface` MODIFIED (`:720`).

## D6. Wording: readiness, and where to watch an archival stream

- The archival comments that call the list "readiness's" (`stream_bounds.go:113`, `:143`, `:155`, `:436`,
  `:602-603`) say "validation's" or "the boot log's"; readiness never reads it. The comments that defend two
  exception kinds (`config.go:63-66`; `stream_bounds.go:113-118`, `:145-161`, `:616-619`) lose that defence.
- The archival Info hint (`stream_bounds.go:638`) keeps "permanent by contract" and the account tier limit, and adds
  that the storage-observability service reports the stream's size.
- `config/README.md`, bounds section (`:302-313`): the override sentence goes; one sentence points to the storage
  observability service and its one-key enable (`service/README.md`, "Enabling it", `:561-583`).
- Proof: the log subtest at `stream_bounds_test.go:761-785` asserts one line at `level=INFO` naming the stream, without
  `expires`, and containing `storage-observability`. Rewritten against the list, it does not compile on the base;
  the task 2.5 mutant shows the hint assertion can fail.
- Other text sites (inventory § 1 and its review findings, plus three this design found): `config/doc.go:26`,
  `:155-156`; `config/README.md:36`, `:311`; `service/README.md:274`, `:313-317`; `service/doc.go:238`, `:274-278`;
  `service/component_manager.go:90-91`; `service/component_manager_lifecycle.go:24`;
  `service/component_manager_suite_test.go:93`; `natsclient/stream_bounds.go:6`. The `config-loading` MODIFIED
  requirement drops "override" from the names a deployment chooses.

## Ledger

- `config/stream_bounds.go` (`:2784`) and `config/stream_drift.go` (`:2794`): `carry` becomes `adapt` in task 2.2, the
  task that edits both files, or `task ledger:check` fails (`Taskfile.yml:65`). `consumer_purpose` drops "held
  byte-equal to the pin"; `contract` reads "as at the pin, less stream_migration_overrides (#362)".
- `known_risks` of `config/stream_bounds.go`, changed from the pin: the override types, sentinels, loop and
  `ExpiredMigrationOverrides` removed; `StreamExceptionReport` replaced by a list of `ArchivalStreamStatus`; the `now`
  parameter dropped, so validation reads no clock; the archival boot hint names the storage-observability service.
- `known_risks` of `config/stream_drift.go`, changed from the pin: the override branch removed, and with it
  `governedBounds`, `boundsGovernedBy` and `declaresDiscard`; drift compares all three fields directly.
- `config` (`:2735`): `contract` (`:2745`) drops "exemptions" for "the archival exemption"; `known_risks` records the
  dropped pin requirement (pin `openspec/specs/stream-provisioning/spec.md:331-370`), the list return and the clock.
- `natsclient` (`:846`): the remedy edit joins its changed-from-pin list (`:1794`).
- `service` (`:2840`): `:2931`, `:2987`, `:3011` and `:3021` lose the reporter. `:3021`'s "flow-path reachability is
  computed in service, not composition (#363)", stale since #413, is corrected in the same edit (orchestrator ruling).

## Decision records

No ADR here or at the pin records the override decision (inventory § 4); it lived only in the pin's
stream-provisioning spec, which is not ported, and the owner's ruling covers dropping it. The ADR-100 citation at
`component_manager.go:234` goes with the code it annotated.

## Merge order

Draft #414 and draft #427 touch none of `config/`, `service/`, `natsclient/`, `configs/`, the ledger or these four
specs (inventory § 6). Either may merge first.

## Adoption

Not an establishing change: it removes a loop and adds no primitive.
