# Inventory: refactor-362-remove-migration-overrides

base: 0e2f17c (main). Claim branch `claude/expiry-reporter-362` at f6fec97 (claim commit 8540948 plus the placement
inventory, staged as a rename to this change's directory). Claim: draft PR #421 (`Closes #362`, `Closes #423`).

Owner ruling, #362 comment 6439894887 (2026-10-27): remove `stream_migration_overrides` entirely, with the running
expiry reporter, its gauge `semengine_streams_migration_override_expired` and its example alert rule; keep the
finite-bounds rule for ordinary streams, `archival_streams`, and `natsclient.CheckNoLifecycleRetention`; #423 closes
as moot; operator quality of life lives in the storage observability service, documented and easy to turn on. The
owner's words: "it smells like we are trying to defend a user from themselves a bit too hard ... i am okay adding
operator qol with some helpful metrics but this smells like more than we need."

This change carries no package from the pin and states nothing about pin behaviour, so no pin probe runs. It removes
pin surface from three ported packages (`config`, `natsclient` text only, `service`).

Search used for every site below (tracked content only):
`git grep -n -i "MigrationOverride\|migration_override\|migration override\|exemptMigrationOverride\|override_expir\|overrideExpiry\|streamOverrideExpiry"`,
then `git grep -n -i -E "migration$|bridge|lapsed"` for line-broken spellings (found `config/doc.go:155`,
`configs/prometheus/rules/storage-pressure.yml:38`, which the first search misses).

## 1. The surface removed (the claimed gap, measured)

Exported Go surface that goes, all in `config`:
`StreamMigrationOverride` (`config/stream_bounds.go:93`), `StreamMigrationOverrides` (`:109`),
`Config.StreamMigrationOverrides` (`config/config.go:60`), `StreamMigrationOverrideStatus` (`stream_bounds.go:135`),
`StreamExceptionReport.MigrationOverrides` (`:166`), `ErrStreamMigrationOverrideInvalid` (`:72`),
`ErrStreamMigrationOverrideExpired` (`:77`), `ExpiredMigrationOverrides` (`:671`). Unexported: `exemptMigrationOverride`
(`:180`), `parseOverrideExpiry` (`:586`), `missingOverriddenStreamRemedy` (`:646`), and in `service` the whole
`streamOverrideExpiryReporter` (`service/stream_override_expiry.go:48`).

Consumer readers (adopter seam, below):

```bash
git -C <repo> grep -n -i "MigrationOverride\|migration_override\|StreamExceptionReport\|ValidateStreamDeclarations" \
  -- '*.go' '*.json' '*.yaml' '*.yml' ':!docs/**' ':!openspec/**'
```

in semsource, semconnect, semteams, semboids: 0 hits each. (The only hits outside those paths are SemStreams-era
evidence logs and a footprint JSON listing the pin's `config.Config` type.)

### Site list, with the disposition the ruling forces

Production code:

| Site | Lines (file size) | Disposition |
| --- | --- | --- |
| `service/stream_override_expiry.go` | all (139) | delete |
| `service/component_manager.go:73-79` field, `:90-91` supervisor comment, `:234-253` construction, register, first evaluate; `"time"` import (`:14`, only reader `:253`) | ~30 of the file | delete; comment `:90` names one loop |
| `service/component_manager_lifecycle.go:24` doc, `:212-225` `supervise` | ~14 | edit: `supervise` runs the health loop alone, the `sync.WaitGroup` and the `:221-223` comment go |
| `config/stream_bounds.go` (699) | sentinels `:69-77`; types `:85-109`, `:133-141`; report field `:164-166`; const `:179-180`; both-declared check `:530-535`; override loop `:543-577`; `parseOverrideExpiry` `:581-600`; Warn loop `:624-632`; remedy `:642-652`; `ExpiredMigrationOverrides` `:654-699`; about 170 lines | delete |
| `config/stream_bounds.go` | `boundsRemedy` `:476-477` (names the key); comments `:51`, `:61`, `:113-118`, `:145-161`, `:439`, `:503-506`, `:616-619` | edit |
| `config/stream_bounds.go:440`, `:507`, `:610` | `now time.Time` parameter of `planStreams` and `validateExceptionDeclarations` | edit: drop. Its only use is the expiry check; `revive.toml:31` enables `unused-parameter`, so leaving it fails `task lint` |
| `config/streams.go` | `:25-26`, `:324` doc; `:330` `planStreams` call; `:345` log field `migration_overrides`; `:468-473` resolveBounds case; `:602-621` refusal to create an overridden stream | delete/edit (~30 lines) |
| `config/stream_drift.go` | `:103-115` override branch of `boundsGovernedBy`; `:229` comment ("and bridged") | delete/edit (~14 lines) |
| `config/config.go` | `:56-60` field; `:63-66` archival comment that defends the split; `:217-219` `ValidateDocument` doc ("the one input ... is the current time") | delete/edit |
| `config/doc.go:26` ("override" in chosen names), `:155-156` | 3 lines | edit |
| `config/README.md:311` | 2 lines | edit |
| `natsclient/stream_bounds.go:6`, `:112-114` (remedy names the key) | 4 lines | edit |
| `configs/prometheus/rules/storage-pressure.yml:298-319` alert `StorageStreamMigrationOverrideExpired`; header `:38-39` | 22 + 2 (339) | delete/edit |
| `service/README.md:313-317`; `:665` ("13 alerts" becomes 12) | 6 | delete/edit |
| `service/doc.go:274-278` | 5 | delete |

Tests:

| Site | Lines | Disposition |
| --- | --- | --- |
| `service/stream_override_expiry_test.go` | all (372) | delete, except two helpers other tests use: `composeComponentManagerWithOverride` (`:130-162`; used by `component_manager_supervisor_test.go:36`) and `syncBuffer` (`:196-208`; used by `component_manager_supervisor_test.go:33`, `log_forwarder_test.go:48,120,145`). They move, the helper losing its overrides parameter |
| `service/stream_override_expiry_test.go:350-372` `TestComponentManagerFailedStartDoesNotLaunchOverrideExpiryLoop` | 23 | edit, not delete: it is the proof of "A failed `Start` SHALL launch no publisher" (`service-lifecycle/spec.md:508-512`; its `spec:` comment names that requirement). It keeps the health and done assertions |
| `service/stream_override_expiry_test.go:282`, `:316` (supervise holds done; Stop waits) | ~70 | delete: with one loop there is no second goroutine to join. Stop still ending the health loop is proved by `TestComponentHealthPublishFailureIsCounted` (`component_manager_supervisor_test.go:55-59`, nothing published after Stop) |
| `service/component_manager_supervisor_test.go:15-18` header, `:63-85` `TestOverrideGaugeUsesTheRegisteredCollector` | ~27 | edit/delete |
| `config/stream_bounds_test.go` (1051) | `TestMigrationOverride` `:489-662` (174), `TestExpiredMigrationOverrides` `:969-1051` (83), subtest `:725-740`, subtest `:891-901` | delete |
| `config/stream_bounds_test.go` | `:207`, `:453-454`, `:688`, `:761-785`, `:813-872` (round trip), `:791-804` (`TestStreamExceptionReport_KindsAreStructurallyDistinct`: its comparator type goes); 30 `planStreams(..., now, ...)` calls | edit (or delete `:791-804`, see question 2) |
| `config/stream_bounds_integration_test.go:156-233` (two tests) | 78 of 258 | delete |
| `config/stream_drift_test.go:230-250` (cases), `:259-282` (test) | ~45 | delete |
| `config/stream_drift_integration_test.go:286-351` (two tests) | 66 | delete |
| `config/load_test.go:391`, `:408` | 2 | edit (the chosen-name example keeps `archival_streams`) |
| `natsclient/stream_bounds_test.go:102` | 1 | edit |
| `natsclient/storagereport/storage_archival_pressure_test.go:24`, `:29` | comments | edit |

Specs (deltas in this change):

- `openspec/specs/service-lifecycle/spec.md:520-539` "A lapsed stream migration override is reported while running":
  REMOVED.
- `openspec/specs/composition-validation/spec.md:28-32` "Validation reads no host files and one clock": MODIFIED.
  After the removal nothing in `ValidateDocument` reads the clock
  (`git grep -n "time.Now" -- composition/ config/config.go config/stream_bounds.go`:
  only `stream_bounds.go:610`, which goes), so the clause becomes "reads no host files and no clock". Its scenario
  (`:34-39`) is about TLS files and stands.
- `openspec/specs/operator-surface/spec.md:720`: MODIFIED, drop the parenthetical. Its test
  (`service/storage_observability_http_test.go:531`, regex `semengine_storage_[a-z_]+`) never read that name.

## 2. What stays, and the code it shares with overrides

- The bounds rule: `missingBounds` (`stream_bounds.go:481-497`), `planStreams`'s undeclared list (`:451-462`),
  `boundsRemedy` first line (`:471-474`), and `natsclient.CheckStreamBounds` at the programmatic seam.
- `archival_streams`: `ArchivalStream` (`:111-131`), its validation (`:511-541`, less `:530-535`),
  `ArchivalStreamStatus` (`:149`), the Info log (`:633-639`), `resolveBounds` case (`streams.go:462-467`), drift
  governance (`stream_drift.go:99-102`) and its tests (`stream_bounds_test.go:668-785` less the subtests above;
  `stream_bounds_integration_test.go:128-154`, `:239-258`; `stream_drift_integration_test.go:456-494`).
- `natsclient.CheckNoLifecycleRetention`: no override reference (`git grep -n -i override -- natsclient/*.go`: only
  `stream_bounds.go:6`, `:113`).
- Shared code: `validateExceptionDeclarations` handles both kinds; the `streamExemption` enum (`:174-183`) drops to
  two values; `StreamExceptionReport` keeps one field. The comments that defend keeping the two kinds as distinct
  types (`config.go:63-66`; `stream_bounds.go:113-118`, `:145-161`, `:616-619`; the pin's stream-provisioning spec
  `:254-259`) lose the thing they distinguish from.
- Readers of what stays: `StreamExceptionReport` is read in production only by `logStreamExceptions`
  (`streams.go:335`) and the count log (`:345-346`); `ValidateStreamDeclarations`' one caller discards it
  (`config.go:270`); no consumer reads either (search above).
- Carried text, still wrong after the removal (earlier inventory F2): archival comments call the report
  "readiness's" (`stream_bounds.go:113`, `:143`, `:155`, `:436`, `:602-603`); readiness never reads it.

## 3. A document that still carries `stream_migration_overrides`

- Loading drops unknown top-level keys silently: `config/doc.go:48` ("Other unknown keys are accepted and ignored"),
  and the ledger's `config` row records it as retained (`docs/admission-ledger.yaml:2766`, "Retained: unknown keys
  elsewhere accepted"). Only named removed or unapplied keys are refused: `removedPlatformFields`
  (`config/config.go:693-715`, `platform.instance_id`, ADR-102) and `unappliedNATSKeys` (`:717-747`). Once the field
  is gone, `keycase.Check` (`:754`) does not see the key either.
- What the operator sees: if the entry named a stream that lacks bounds, validation and boot fail with
  `ErrStreamBoundsUndeclared`, naming the stream, its source and each missing field, then `boundsRemedy`
  (`stream_bounds.go:459-461`). If the stream has bounds, or is not provisioned, nothing happens and nothing is wrong.
  No path admits an unbounded stream through the dropped key.
- No config in this tree declares the key except tests (`git grep -n stream_migration_overrides -- configs/ '*.json'`:
  only the alert rule's description).

## 4. Ledger rows and decision records

- `docs/admission-ledger.yaml:2784` `config/stream_bounds.go` and `:2794` `config/stream_drift.go` are `carry`
  ("held byte-equal to the pin"); `task ledger:check` runs `go run ./internal/harness/pindiff check`
  (`Taskfile.yml:65`), which fails a carry row that differs from the pin. Both must become `adapt`, with what changed.
- `:2735` `config` (adapt): `contract` cites "the pin's stream-provisioning spec requirements for declarations,
  bounds, exemptions" (`:2745`); the pin requirement "Unbounded existing streams MUST be admitted only by an
  expiring override" (pin `openspec/specs/stream-provisioning/spec.md:331-370`, fetched at `8b99efe9`) is the one
  dropped. `known_risks` needs the removal and the clock change.
- `:846` `natsclient` (adapt, `:1794`): the remedy text change belongs in its changed-from-pin list.
- `:2840` `service` (adapt): `:2931` ("re-reads the configuration's stream overrides every minute"), `:2987`
  (`stream_override_expiry.go` in PR 3's file list), `:3011` ("the override gauge is registered through
  metric.RegisterOrGet"), `:3021` ("the override reporter is hosted by the component manager (#362)") need edits.
- ADRs here: `git grep -n -i "override\|archival_streams\|stream bounds" docs/adr/`: four hits, none about stream
  overrides (066 and 084 readiness overrides, 100 a proposal, 104 an environment override). ADR-100 is cited for the
  reporter at `component_manager.go:234` but does not govern it (earlier F3); the citation goes with the code.
- Pin ADRs: all 85 fetched at `8b99efe9`; `grep -l -i 'migration_override\|migration override\|MigrationOverride\|stream_migration'`:
  none. `archival|finite bound|max_bytes|expiry` hits only ADR-028 (a tool argument), ADR-056 (ownership waivers with
  an expiry, a different mechanism) and ADR-073 (graph retention death-triggers). No pin ADR records the override
  decision; it lives only in the pin's stream-provisioning spec, which is not ported here. So no ADR decision changes;
  the spec requirement drop is covered by the owner's ruling.

## 5. Storage observability (ruling item 4)

- Documented: `service/README.md:541-673` (what it does, report-only, the one-key enable at `:561-583`, keys,
  lifecycle, health text, route, the 19 gauges, the example rules). Specified: `service-lifecycle/spec.md:892-1012`
  (construction refusals, defaults, `RegisterAll` registers it and `ResolveServiceConfigs` does not add it,
  `:919-930`, lifecycle, health) and `operator-surface/spec.md:545-725` (route, metrics, alert rules). The brief's
  premise "no spec under openspec/specs" does not hold; the README's "is not ported here" (`:557`) refers only to the
  pin's text on deferring capacity admission.
- Easy to turn on: one `services` entry with `"enabled": true`; every config key optional (`:565-578`).
- Gaps measured:
  1. Nothing on the stream-bounds side points to it: `config/README.md:302-313`, `config/doc.go:150-157`, the
     archival Info hint (`stream_bounds.go:638`, "its only ceiling is the account tier limit") and `boundsRemedy` name
     no tool for watching an unbounded stream.
     (`git grep -n -i "storage-observability\|storage observability" -- config/`: none.)
  2. The example rules file says it is report-only (`storage-pressure.yml:9-12`) but not that every series it reads
     exists only while the service is enabled. With the service off no rule fires, the freshness rule included
     (`:328`, `time() - semengine_storage_report_collected_timestamp_seconds > 900` over an absent series), by design
     (rule 1, `:18-30`: nothing alerts on absence).
- Nothing broken in code; both gaps are a sentence each.

## 6. Adjacent claims

- Open issues (queries `override`, `expiry`, `archival`, `stream_bounds`, `stream bounds`, `storage observability`,
  `storage-observability`, `component manager`, `supervise`): kept #362 (this change) and #423 (F5 of the earlier
  inventory: a lapsed override refuses every later `SafeConfig.Mutate`, `config/config.go:138` ->
  `stream_bounds.go:609-611`). After the removal no validation input depends on the time, so the refusal cannot
  arise; #423 closes. Names the word only: #15, #9, #420, #103, #110, #195, #196, #280, #361, #7, #105, #216.
- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`): #421 (this claim, 1 file); #414 (draft,
  42 files: `graph/`, `processor/`, `composition/reserved_test.go`, its own change with deltas for
  `entity-state-follower`, `graph-index`, `graph-readiness`, `graph-transport-boundary`); #427 (draft, 3 files,
  `fix-422-census-in-contract`, a `harness-boundaries` delta). Neither touches `config/`, `service/`, `natsclient/`,
  `configs/`, the ledger, or the three specs above. No overlap; either may merge first.
- Archived designs that describe the reporter (history, not edited):
  `openspec/changes/archive/2026-10-22-setup-04a-03-boot-service-component-manager/design.md:99`, `:281-302` (D4),
  `:494-495`.
- Coverage: `config` has an 80% floor (`scripts/cover-check.sh:44`, merged unit+integration); code and its tests
  leave together, so `task cover:check` is the check, not a prediction.

## 7. Problem shape

"Remove an operator escape and its runtime reporter; leave the refusal it escaped from." Closest instance:
`platform.instance_id` removal (ADR-102), which kept a named refusal of the old key
(`config/config.go:693-715`). The other instance is #74's unapplied NATS keys (`:717-747`), refused because an
operator would otherwise believe a setting applied. Here the dropped key cannot make an operator believe anything
false that the bounds refusal does not already report (section 3).

## Collision table

Not triggered: the change adds no durable, communication or runtime-coordination primitive; it removes one loop.

## Adopter seam inventory

- Go consumers (semsource, semconnect, semteams, semboids): use none of the removed symbols (search in section 1).
  Compile break: none expected.
- Config authors: (1) what must they know: nothing new; (2) do nothing: an old entry is ignored, and a stream that
  needed it fails the bounds check by name; (3) found out: boot error and `--validate`; (4) should know: nothing.
- Alert authors who copied the example rule: their rule reads a series that no longer exists and never fires; there
  is nothing left for it to report. The README's alert count changes.
- Log readers: the boot line "Ensured JetStream streams" loses its `migration_overrides` field (`streams.go:345`);
  semboids' baseline logs show `migration_overrides=0` (evidence only).
- Operators who want capacity signals: one key turns storage observability on; the two gaps in section 5 are where
  they would miss that.

## Owner questions

1. Old key: accept and ignore it, as any other unknown top-level key (zero new code; a stream that needed it still
   fails loudly by name), or refuse it with a plain message as `platform.instance_id` is refused (about ten lines and
   a test, and a reader for a removed key, which the no-SemStreams-migration ruling argues against)?
2. With one exception kind left, keep `StreamExceptionReport{Archival}` and its distinct-types comments trimmed (the
   smallest diff), or collapse the report to the archival list (less surface; no outside reader)? Either way the
   comments defending the split go.
3. Fix the carried "readiness" wording on the archival lines (F2) in this PR, since those lines are edited anyway, or
   leave it?
4. Storage observability: add one sentence to `config/README.md` (bounds section) pointing to the service and its
   one-key enable, and one header line in `storage-pressure.yml` saying its series exist only while the service is
   enabled. Optionally the archival Info hint names it. Is that the right size?

## Inventory review

Claude `semengine-reviewer`, INVENTORY PASS at `f6fec97` plus this file. Three LOW findings, folded in here:

1. `config/README.md:36` lists "override" among the names a deployment chooses (the twin of `config/doc.go:26`);
   edit it with the removal.
2. `heldConfigSource` and its methods (`service/stream_override_expiry_test.go:253-275`) have no reader once the
   failed-Start test stops injecting a reporter; delete them. `isClosed`, `stopContext` and `discardLogger` live in
   other test files and stay.
3. The `resolveBounds` override-case comment (`config/streams.go:472-473`) is one more "readiness" wording site; it
   goes with the deleted case.
