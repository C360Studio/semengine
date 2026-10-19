# Inventory: setup-04a-03-boot-config-manager

base: `c0cabb75204389bfb89b37b2a460b59368c4215e` (origin/main `a6b6fac` plus the claim commit, which adds only the
proposal; draft PR #329)

Question: what porting `config.Manager` from the SemStreams pin `8b99efe9` carries, what it changes, and what the
owner must rule on before the design. Owner intent: #9 comment 6274501134 ("Two PRs, waive about 3,150 for the
first"; part 2 is "about 1,700 lines, under the cap, one lifecycle owner"). Part 1's rulings: #9 comment 6275744386.

Repositories read (`docs/inventory-scope.md`): this tree; the pin through a `git archive 8b99efe9` copy (local only);
the default branches of semsource `e4febc0d`, semconnect `d0d06e00`, semteams `512b4bf2` and semboids `8c03cc53`
through `gh api .../tarball/<sha>` (local only).

**The slice.** Carried non-test lines (`wc -l` at the pin): `manager.go` 1,362, `key_family.go` 281,
`bucket_name.go` 54, which is 1,697 in total. Imports: `graph`, `model`, `natsclient`, `pkg/errs`, `types`, nats.go
`jetstream`, and in-package part 1 (`SafeConfig.Mutate` `config/config.go:126`, `CompareVersions` `:754`,
`validateAuthorityPair` `:653`, `mintedSuffixBytes` `:602`, `validateJSONDepth` `config/security.go:100`,
`maxConfigSize` `:13`). All of these are in the tree. Four pin names have a different home here, and nothing else is
missing (scratch copy, `go build ./config/` clean after the four renames):

- `manager.go:192` and `:1176` `graph.EnsureCatalogBucket` / `graph.OpenCatalogReader` are
  `graph/kvcatalog/kvcatalog.go:251` / `:296`;
- `bucket_name.go:44` `name := graph.BucketSemStreamsConfig + "_" + org + "_" + stem` and `manager.go:1150` name
  `graph/constants.go:64` `BucketSemEngineConfig = "semengine_config"`;
- `manager.go:1178` `natsclient.ErrorCodeBucketNotReady` is `errs.CodeIndexNotReady` (`pkg/errs/errs.go:331`), the
  code `natsclient/kvspec.go:343` returns.

`config` importing `graph/kvcatalog` makes no cycle: `go list -deps ./graph/kvcatalog` names neither `config` nor
`component`, and `TestImportGraph` passes on the scratch copy.

**Pin probe** (go1.26.4 darwin/arm64, pin copy): `go test -race -count=1 -cpu 1 ./config/` ok;
`go test -count=5 -cpu 1 -shuffle=on ./config/` ok; the Manager, key-family and bucket-name tests under
`-tags integration -race` ok. Lifecycle suite: not run at the pin. Its `Owner` needs `Stop(ctx)`, and the pin has
`Stop(timeout time.Duration)` (`manager.go:478`). Probes P1-P5 below stand in for the checks it would fail.
On the scratch SemEngine copy, the pin tests with the module path, the four names, `semengine_config`, and a
`newTestClient` shim over part 1's `fixtureClient` pass under `-race`, unit and integration, after one change
(§ 7). Coverage, merged unit and integration profile: `config` 83.7%; `manager.go` 78.3%, `key_family.go` 91.8%,
`bucket_name.go` 100%.

Probes (scratch tests in the pin copy, nothing added to either tree):

- **P1.** `Start`, then `WatchModelRegistry()`, 200 ms to let its goroutine take the initial update, then
  `Stop(2*time.Second)`: Stop logs `Manager shutdown timeout`, takes 2.01 s and returns nil. Without the 200 ms it
  takes 1 ms, so the outcome depends on scheduling.
- **P2.** `Start(ctx)`, `OnChange("services.*")`, cancel `ctx`, then a KV put to `services.x`: no update arrives in
  2 s.
- **P3.** A second `Start` on the same Manager, and a `Start` after `Stop`, both fail with the minted-identifier
  guidance ("declare the stem ..."), not an already-started refusal.
- **P4 (#17).** A file declares component `a`; first boot; `DeleteComponentFromKV(a)`; Stop; a new Manager with the
  same file: `a` is present again.
- **P5.** A bucket at the same version holding `services.metrics` = `{"enabled":true,"name":"m"}`: `Start` returns
  nil, logs `Failed to apply KV config during sync ... unknown field "name"`, and `metrics` is gone from the
  effective configuration.

## 1. Shape sweep (pin paths)

- **M1, lifecycle (ADR-113, `lifecycle-suite`, `background-work`): Stop takes a fixed timeout.**
  `manager.go:478` `func (cm *Manager) Stop(timeout time.Duration) error {`; `:501`
  `case <-time.After(timeout):`; `:502` `cm.logger.Warn("Manager shutdown timeout", "timeout", timeout)`, then the
  watchers and channels are torn down and nil is returned (P1).
- **M2, lifecycle: continuing work runs under the Start context.** `:425`
  `watcher, err := kvHandle.Watch(ctx, pattern, jetstream.UpdatesOnly())`, `:447` (family watch), `:466`
  `go cm.processWatcher(ctx, watcher)`, `:471` `go cm.processFamily(ctx, family, familyWatchers[i])`, `:531` and
  `key_family.go:252` `case <-ctx.Done():` (P2). Whether nats.go also ends a watcher with the context passed to
  `Watch` is not promised here (`jetstream/kv.go:1347` passes it to the subscription; not probed).
- **M3, completed joins: the model-registry forwarder blocks Stop.** `:261` `cm.wg.Add(1)` and `:265`
  `for u := range in {`. `in` is a subscriber channel, closed at `:516` `close(ch)` only after the wait group, so
  Stop waits for the forwarder and the forwarder waits for Stop (P1).
- **M4, lifecycle: not one-shot.** `:329` `cm.shutdownCh = make(chan struct{})` runs on every Start. A second Start,
  and a Start after Stop, re-acquire the bucket and are refused only by the identity guard (P3). After Stop,
  `OnChange` (`:296`) appends a channel nothing closes (`:519` resets the map), and `WatchModelRegistry` starts a
  goroutine no Stop joins.
- **M5, copied shell (shape 2):** `shutdownCh`, `wg` and `stopped` (`:61-63`) are the state machine
  `internal/lifecycleguard.Guard` (`guard.go:31`) holds once. Not on #196's adopter list, which counted components
  and services.
- **M6, silent loss: Start logs and continues.** `:354` first-boot `pushToKV` error, then `:356`
  `// Continue anyway - UI won't have initial state but app can run`. `:366`, `:378`, `:395`, `:402` log
  `Failed to sync from KV on startup`. Inside `syncFromKV`, `:1327` `current.Services = make(types.ServiceConfigs)`
  runs first; then `:1345` (`Get` fails) and `:1353` (decode fails) skip the key with a Warn, so a service is
  dropped (P5). `getKVVersion` returns `"0.0.0", nil` on any `Get` error (`:1301`), not only not-found, and on a
  version that does not parse (`:1308`).
- **M7, Q1 parity: the KV `nats` key bypasses part 1's refusal.** `:728` `case "nats":`, `:730`
  `if err := json.Unmarshal(value, &currentConfig.NATS); err != nil {`. Part 1 removed `Token` etc. from
  `NATSConfig` and refuses them at load (`config/config.go:704` `rejectUnappliedNATSKeys`). A KV `nats` value with
  `"token"` is now dropped silently. At the pin it was applied: P5's run shows the URLs applied, and the pin field
  exists.
- **M8, #17: KV selection overlays components on the file.** `syncFromKV` resets only Services (`:1327`). The pin test
  `manager_integration_test.go:518` `TestConfigManagerKVSelectionReplacesOnlyServices` asserts that `file-component`
  stays (P4).
- **M9, key spelling.** `:766` `func sanitizeNATSKey(key string) string {` maps only spaces (`:769`). A component name
  holding `.` is written as a three-token key (`components.a.b`) that the watch patterns and `syncFromKV` skip
  (`:1338` `Skipping property-level key during sync`). At the pin the file copy survives because components are
  overlaid; under M8's fix it would not. Consumer survey: 0 of 227 component names in the 28 consumer config files
  contain `.`, space, `*` or `>`. semsource re-spells the key (second spelling, out of repo):
  `cmd/semsource/desired_config.go` `"components."+strings.ReplaceAll(handle, " ", "_")`.
- **M10, surface audit (a): exported with no outside reader.** `:296` `OnChange` and `:26` `type Update struct {` (read
  only by `WatchModelRegistry` in-package); `:869` `func (cm *Manager) PushToKV(ctx context.Context) error {` (Start
  uses unexported `pushToKV`). The pin search over non-test files outside `config/`
  (`git grep -n -e 'OnChange(' -e PushToKV -e 'config\.Update\b' 8b99efe9`) and the consumer tarballs find none.
- **Kept with readers (surface audit):**
  - `NewConfigManager`, `Start`, `Stop`, `GetConfig`, `WithKeyFamily`, `ManagerOption`: pin `internal/boot`,
    `internal/bootstrapobservability/bootstrap.go:163`, `service/component_manager.go:165`.
  - `NewKeyFamily`, `KeyFamily.Get/Put/Create/Delete/Names`, `KeyFamilyEntry`: pin
    `processor/rule/kv_config_integration.go:82`, `:291-366`, an admitted package not yet ported.
  - `KeyFamilyPut/Delete`: the `Operation` vocabulary of that entry.
  - `PutComponentToKV`, `DeleteComponentFromKV`, `BucketName`: semsource (§ 4).
  - `WatchModelRegistry`: the carried `model.Watcher` contract (`model/watch.go:13`, ledger row `model`, carry).
- **Optional (retained).**
  - `matchesPattern` (`:635` `return strings.HasPrefix(key, prefix+".")`) treats `services.*` as any depth, unlike
    a NATS `*`. It is a second spelling of wildcard matching, and it only routes notifications.
  - `:427` `// Ignore errors for patterns that don't exist yet` claims nats.go behaviour, unprobed.
  - The Stop comment (`:487-490`) claims a nats.go race, unprobed.
  - `handleUpdate` logs and skips a malformed runtime write (`:578-583`).
  - `notifySubscribers` does a blocking send under `mu.RLock` (`:1006-1012`). A concurrent watcher send could
    fill the buffer between drain and send. That needs a condition the code does not choose; MEDIUM at most (#185).
- **Shapes 3 and 5:** `interface {`, `err.Error()`, `strings.Contains(err` in the three files: no hit. Errors are
  classified with `errors.Is`/`errors.As` (`:1177-1178`, `:1183`).
- **Context rules:** no `context.Context` field (the struct comment at `:42-44` says so); no `context.Background`,
  `TODO` or `WithoutCancel` (only a comment, `:118`); the one root ADR-113 permits is absent at the pin (M2). Bucket
  acquisition already runs under Start's context (`:183`).

## 2. Pin ADRs

All 85 pin records were searched for `config/manager`, `manager.go`, `key_family`, `bucket_name`, `config.Manager`,
`ConfigManager`, `NewConfigManager`, `WithKeyFamily`, `KeyFamily`, `BucketName`, `semstreams_config`,
`platform_identity`, `PushToKV`, `PutComponentToKV`, `DeleteComponentFromKV`, `syncFromKV`, `WatchModelRegistry`,
`engineHighWater` and `CompareVersions`. Hits: 026, 029, 030, 040, 053, 055, 056, 058, 060, 063, 104. Titles were then
read for config, boot, lifecycle and identity records (094, 095, 102).

- **ADR-104** (platform authority unique by default): mint, adopt and refuse at Start (`manager.go:1021-1293`).
  Binding. In `docs/adr` with a ledger row; its port note says `config.Manager.Start` "is not yet ported" and bucket
  names are the pin's (`docs/adr/104-unique-platform-authority.md:12-13`).
- **ADR-102** decision 7 (identity never rewritten; `102-entity-id-segment-semantics.md:83`): `Create` not `Put`,
  refuse rather than repair (`:1080-1140`, `:1211-1283`). Binding; in `docs/adr`.
- **ADR-094** (boot-only composition): post-boot writes to `services.*`, `components.*`, `platform`, `nats`,
  `model_registry` are next-boot desired state only (`094-...md:37-41`). That is the contract of
  `PutComponentToKV`/`DeleteComponentFromKV` (`:774-781`, `:834-842`). Its "Terminal shutdown remains Start-owned"
  (`:44`) is superseded here by ADR-113. Not in `docs/adr`; its main subject is `service`.
- **ADR-095** (one-shot running lifecycle): one Start per instance. The pin Manager does not enforce it (M4).
  ADR-113 keeps it ("Unchanged"). Not in `docs/adr`.
- **ADR-113** (this repo, `docs/adr/113-start-context-bounds-start.md`): Start's context bounds Start; `Stop(ctx)` is
  the only stop; one run context created in Start and ended in Stop. Governs M1-M4.
- 026 and 029 concern `processor/rule.ConfigManager`; 058 and 030 concern `service/service_manager.go`; 040, 053,
  055, 056, 060 and 063 match `manager.go` in other packages. None governs this slice.
- No record governs `key_family.go` or `bucket_name.go` beyond ADR-104 (the bucket and the family both serve the
  identity record's owner-only bucket). Their contract is the pin spec `openspec/specs/component-runtime-config/
  spec.md` (641 lines), requirements at `:315` (Config Manager clauses `:326-375`), `:483`, `:496`, `:519`, `:545`
  and `:574`; it is not ported here.

## 3. Open issues and pull requests

`gh issue list --state open --search <q> --limit 200` for `config`, `manager`, `key_family`, `bucket_name`,
`KeyFamily`, `BucketName`.

- **#17** (deleted file-defined component returns at next boot, `syncFromKV`; labels `bug`, `status:blocked`; no
  comments): names `config/manager.go`. Its body calls itself a repair-before-port row that "must pass before the
  package is admitted". Reproduced (P4).
- **#195** (adopt `natsfixture.Open`): config's remaining 32 pin `NewTestClient` calls are here: `manager_test.go`
  8, `manager_integration_test.go` 4, `manager_identity_integration_test.go` 17,
  `manager_lifecycle_integration_test.go` 2, `key_family_integration_test.go` 1. `service` (18) is outside this
  slice.
- **#325** (case-variant key dropped by the loader merge): names `config`. KV keys match exactly (NATS keys are case
  sensitive; `updateConfig` switches on `parts[0]`). Values decode into typed structs, which `encoding/json`
  matches case-insensitively, so nothing is dropped for case. The Manager has no map merge.
- **#196** (lifecycleguard adoption): names `manager` as a word only; its list has no `config.Manager`.
- Names the word, not the package: #198, #27, #280, #9, #134, #84, #281, #7, #25, #109, #129.
- **Open pull requests** (`gh pr list --state open --json number,files`): #329 (this claim). Draft #327
  `claude/one-exclusive-address-check` has 0 files today; its issue #324 changes `component/registry.go` and
  `internal/flowgraph`, with no capability shared with `config`. Both may touch `docs/admission-ledger.yaml` and
  `docs/repository-map.md`.

## 4. Callers

- **This repo:** none. `graph/constants.go:52` and `graph/kvcatalog/kvcatalog.go:210` already name
  `config.BucketName` in comments.
- **Pin, non-test, outside `config/`:**
  - `NewConfigManager`/`Start`: `internal/bootstrapobservability/bootstrap.go:163-168`, called from
    `internal/boot/run.go:157` with the long-lived `runtimeCtx`.
  - `Stop`: `bootstrap.go:191` `_ = manager.Stop(5 * time.Second)`. `internal/boot/resources.go:37` converts its
    shutdown context to a duration through `stopWithinShutdownBudget` (`run.go:664-678`), and adds `ctx.Err()`
    because Stop returns nil on timeout.
  - `WithKeyFamily`: `run.go:159`.
  - `GetConfig`: `service/component_manager.go:165`, `service/metrics.go:94`, `service/service_manager.go:1901`.
  - `NewKeyFamily` and the family methods: `processor/rule/kv_config_integration.go:82-366`.
- **Consumers (symbol level):**
  - semsource:
    - `cmd/semsource/run.go:156` `NewConfigManager`, `:160` `Start(ctx)`,
      `:163` `configMgr.Stop(5*time.Second)`, `:368` `GetConfig`;
    - `:1270`, `internal/subwatch/subwatch.go:249`, `internal/sourcespawn/sourcespawn.go:228`, `:334` and
      `processor/source-manifest/*` `PutComponentToKV`;
    - `run.go:1294` and `subwatch.go:261` `DeleteComponentFromKV`;
    - `cmd/semsource/desired_config.go:34` `BucketName`, then `OpenCatalogReader` and a `components.<name>` read.
  - semteams `cmd/semteams/main.go:665` `NewConfigManager(..., config.WithKeyFamily(ruleManager.KeyFamily()))`,
    `:670` `Start(ctx)`, `:149` `defer configManager.Stop(5 * time.Second)`, `:675` `GetConfig`.
  - semboids `cmd/semboids/main.go:148`, `:152`, `:155` (same three).
  - semconnect: no use.
  - No consumer calls `OnChange`, `PushToKV`, `WatchModelRegistry` or `model.Watch`.
- **Consumer config files:** 28 with `platform` and `components`. `version`: semteams `2.0.0` (23) and `1.0.0` (2),
  semconnect `1.1.1` (2), semboids `1.2.0` (1).

## 5. Category 1, 2 and 5

- **Claimed gap:** `git grep -n -w -e NewConfigManager -e KeyFamily -e BucketName -e WithKeyFamily -- '*.go'`
  finds only comments and two unrelated `ObjectStore.BucketName` test comments. The Manager is absent.
- **Spellings of the facts modelled:**
  - The bucket prefix is spelled once (`graph/constants.go:64`), recognised once (`kvcatalog.SpecFor`, `:212`), and
    named per deployment only by `BucketName`.
  - The component key: `sanitizeNATSKey` in three writers, plus semsource (M9).
  - Wildcard matching: `matchesPattern` and NATS (M10, optional).
  - The deployment authority: established only in Start (`applyEffectivePlatformID`, `:1285`). This is the
    harness-boundaries requirement "No second spelling of deployment authority"
    (`openspec/specs/harness-boundaries/spec.md:519-523`). `config` is an excepted package
    (`internal/harness/contract/authority_test.go:224`), and `platformIdentityRecord.Org` is inside it.
- **Closest instance of the shape** (a lifecycle owner with startup work under Start's context and continuing work
  joined in `Stop(ctx)`):
  - graph-ingest: `processor/graph-ingest/component.go:446` `lifecycle lifecycleguard.Guard` and `:982`
    `runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))`;
  - its suite adapter `processor/graph-ingest/lifecycle_integration_test.go:347`
    `lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})`.
- **Same-class collision (configuration bucket, ported not new):**

| Dimension | Evidence |
| --- | --- |
| Semantic class | Next-boot desired configuration plus the create-once platform identity record |
| Owners | `config.Manager` (acquirer); `processor/rule.ConfigManager` writes `rules.*` via a key family (unported) |
| Catalogs | `graph/kvcatalog/kvcatalog.go:150-160` descriptor (strict no-lifecycle, owner-only, History 5) |
| Status | None. The Manager publishes no readiness or health (`grep -n -i 'health\|ready' manager.go`: comments only) |
| Lifecycle | Start acquires, mints or adopts, arbitrates, then watches; Stop ends watches (M1-M4) |
| Ownership | One acquirer. Co-processes on one pair share the bucket and converge through `Create` (`:1127-1132`) |
| Readers | `GetConfig` (service), `WatchModelRegistry`/`model.Watch`, semsource's `OpenCatalogReader` read |
| Writers | Manager writers; key-family holders; generic `update_kv` refused by `kvcatalog.IsFrameworkOwnedBucket` |
| Recovery | None; a foreign, malformed or pre-identity bucket refuses Start ("provision fresh NATS storage") |

## 6. Adopter seam (consumers that build on `config.Manager`)

1. **What they must know:** call `Start` before any write (else `errBucketNotAcquired`, `:174-175`); register key
   families before `Start`; bump `version` for a file edit to win over the bucket (`:347-404`). The last is the debt
   M8's fix extends to components.
2. **If they do nothing:** today a deleted file component returns (P4); a malformed stored service vanishes (P5); a
   live `model.Watch` makes every Stop wait its full timeout and return nil (P1).
3. **Where they find out:** the version rule is a Debug log (`:399` `File and KV versions match, syncing from KV`);
   P4 and P5 are a Warn or nothing. All are "log line" or lower.
4. **Should know:** nothing beyond calling Start then Stop. The gap is the version rule, which stays a rule; the rest
   is what M1-M8 correct.

Intent check: no boundary moves. `config` is admitted (ledger row `config`), and part 2 completes it.

## 7. Tests and the harness guards (pin test files)

Files: `manager_test.go` (565, `//go:build integration`), `manager_reconcile_test.go` (86), `key_family_test.go`
(85), `bucket_name_test.go` (112, `rapid`, `FuzzBucketName`), and the integration files `manager_integration_test.go`
(619, testify `suite`), `manager_identity_integration_test.go` (744), `manager_lifecycle_integration_test.go` (140),
`key_family_integration_test.go` (158).

- **Guards run on the scratch copy** (`go test ./internal/harness/contract/`, git-initialised):
  - `TestNoFixedAddressesInTests`: `manager_lifecycle_integration_test.go:32`, `:104`
    `URLs: []string{"nats://localhost:4222"},`.
  - `TestNoSleepsInTests`: `manager_integration_test.go:66`, `manager_lifecycle_integration_test.go:73`, `:83`,
    `:133`.
  - `TestNoSkippedTests`: `manager_test.go:129`, `:326`, `:438`, `manager_integration_test.go:455`.
  - All other guards pass: retained context, cleanup roots, bare `select {}`, process-global registration,
    `t.Parallel` in integration files (`key_family_test.go:17`, `:39`, `:53` are untagged), build tags, Own server,
    public signatures, import graph.
  - Pinned revive on `./config/...` and `scripts/lint-test-ports.sh`: clean.
- **Need adapting:** 32 `NewTestClient` calls (#195); `semstreams_config` literals (`bucket_name_test.go:37`,
  `:60`, `:109`, and the integration files); every `Stop(5 * time.Second)` (M1).
  `TestFirstBootMintsDistinctSuffixesPerDeployment` (`manager_identity_integration_test.go:277`) opens two clients
  in one test. On the shared server the second reset fails ("a connection from an earlier test is still open"), and
  it passes once each boot is a `t.Run` subtest. `TestConfigManagerKVSelectionReplacesOnlyServices` encodes #17.
  `TestStartRejectsNilContextWithoutSideEffects` reads `shutdownCh`.
- **`example_test.go`:** the five Manager examples (`:97`, `:144`, `:196`, `:223`) are commented-out code printing a
  constant. They use names that do not exist (`update.Key`, `update.Value`, `Platform.LogLevel`, an
  `Update(func...)`). Not worth carrying.
- **Sensitivity:** `task mutate:check` cannot drive an integration test (#129), and nearly all Manager behaviour
  needs NATS.
