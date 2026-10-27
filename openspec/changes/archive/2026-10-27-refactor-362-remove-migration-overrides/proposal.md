# Proposal: refactor-362-remove-migration-overrides

## Why

Issue #362 asked where the running expiry reporter for `stream_migration_overrides` should live. The owner ruled
instead that the overrides go (#362, comment 6439894887): "it smells like we are trying to defend a user from
themselves a bit too hard ... this smells like more than we need." A migration override admitted an existing
unbounded stream for a declared period; a reporter in the component manager re-read the configuration every minute
and set the gauge `semengine_streams_migration_override_expired` when one lapsed
(`service/stream_override_expiry.go:48`, constructed at `service/component_manager.go:234-253`). Deployments start
fresh and no configuration here or in the four starter consumers declares an override (inventory § 1), so the bridge
admits nothing anyone uses. Issue #423, a lapsed override refusing every later `SafeConfig.Mutate`, goes with it.

## What Changes

- `stream_migration_overrides` leaves `config.Config` with its types, sentinels and `ExpiredMigrationOverrides`. A
  document that still sets the key loads like any document with an unknown top-level key; a stream that relied on it
  fails validation and boot by name, as any stream without bounds does (#362, comment 6440188996, item 1).
- `StreamExceptionReport` goes: `ValidateStreamDeclarations` returns the archival streams as a plain list (item 2).
- Validation reads no clock, so `ValidateDocument` reads nothing outside the document.
- The component manager runs one loop, the health publisher. The reporter, its gauge and the example alert rule
  `StorageStreamMigrationOverrideExpired` go.
- Comments that call the archival list "readiness's" are corrected (item 3).
- Three pointers to the storage observability service: one sentence in `config/README.md`, a header note in
  `configs/prometheus/rules/storage-pressure.yml`, and the archival boot log hint (item 4).
- `docs/admission-ledger.yaml`: `config/stream_bounds.go` and `config/stream_drift.go` move from `carry` to `adapt`.

## Capabilities

- `service-lifecycle`: one REMOVED requirement (the lapsed-override reporter); two MODIFIED requirements stop naming
  the override reporter among what the component manager holds and stops.
- `composition-validation`: one RENAMED and MODIFIED requirement, "reads no host files and no clock".
- `operator-surface`: one MODIFIED requirement, the example alert rules, without the migration alert.
- `config-loading`: one MODIFIED requirement, "override" leaves the list of names a deployment chooses.

## Non-goals

- Refusing the old key by name, as `platform.instance_id` is refused (ruled out, item 1).
- Any change to the finite-bounds rule, `archival_streams`, or `natsclient.CheckNoLifecycleRetention` (ruling item 2).
- Any change to the storage observability service's code, keys or metrics.
- Collapsing the two-value `streamExemption` enum or renaming `validateExceptionDeclarations`.
