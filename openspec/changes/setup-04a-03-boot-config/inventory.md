# Inventory: setup-04a-03-boot-config

base: `7f7136f1e6f38352a6800913d481f8f37f3450b1` (origin/main `0ec8f5a` plus the claim commit, which adds only the
proposal; draft PR #322)

Question: what porting the `config` core from the SemStreams pin `8b99efe9` carries, what it drops, and what the owner
must rule on before the design. Owner intent: #9 comment 6274501134 ("Two PRs, waive about 3,150 for the first").
Order: #9 comment 6270875286 (fourth package of change 3).

Repositories read (`docs/inventory-scope.md`): this tree; the pin through a `git archive 8b99efe9` copy (read-only
object read; local only); the default branches of semsource `e4febc0d`, semconnect `d0d06e00`, semteams `512b4bf2`
and semboids `8c03cc53` through `gh api .../tarball/HEAD` (local only).

**The package slice.** Carried non-test lines (`wc -l` at the pin): `config.go` 1,070, `security.go` 188, `streams.go`
737, `stream_bounds.go` 699, `stream_drift.go` 332, `doc.go` 122, which is 3,148 in total. Imports: `model`,
`pkg/platform`, `pkg/security`, `pkg/types`, `types`, `component`, `natsclient`, `pkg/errs` and nats.go `jetstream`,
all already in the tree (`docs/admission-ledger.yaml` rows `model`, `pkg/platform`, `pkg/security`, `pkg/types`,
`types`, `component`, `natsclient`, `pkg/errs`). The six files with the module path rewritten build in a copy of this
tree with no other file (`go build ./config/`, clean). No file here references `manager.go`, `key_family.go`,
`bucket_name.go`, `helpers.go`, `env.go` or `minimal_config.go`.

**Pin probe** (go1.26.4 darwin/arm64, on the pin copy): `go test -race -count=1 -cpu 1 ./config/` ok;
`go test -count=5 -cpu 1 -shuffle=on ./config/` ok. Lifecycle suite: not applicable. `StreamsManager` holds a client
and a logger (`streams.go:95-98`) and has no `Start` or `Stop`; `Manager` is PR 2's. On the scratch SemEngine copy,
the in-scope unit tests (adapted as in § 7) pass under `-race`, and five times shuffled. The five in-scope
integration files pass on `natsfixture` (§ 7). Coverage, merged profile: 75.3%.

## 1. Shape sweep (pin paths)

- **S0, required (owner ruling, #9 comment 6274501134): three files not ported.** `helpers.go` (220 lines,
  11 exported `Get*`/`Has*` helpers), `env.go` (56, `ExpandEnvWithDefaults`), `minimal_config.go` (72,
  `MinimalConfig`, `CoreServicesConfig`, `LoadMinimalConfig`). That is 15 exported symbols and 348 lines. The pin
  search (`git grep -w` over the 15 names, non-test, outside the three files) finds only a doc comment,
  `service/doc.go:249`. The same search over the four consumer tarballs finds nothing.
- **S1, required (surface audit a): dead exported surface.** No non-test reader at the pin outside the defining
  file, and none in the four consumers (symbol greps in § 4):
  - `config.go:33-35` `StorageModeMemory`/`KV`/`Hybrid`
  - `config.go:368` `type BucketConfig struct {`
  - `config.go:393` `AddLayer`, `:398` `EnableValidation`, `:409` `func (l *Loader) Load() (*Config, error) {`, which is
    reached only through `LoadFile`
  - `:779` `SaveToFile`, with `security.go:100` `safeWriteFile`
  - `config.go:896` `func (c *Config) String() string {`: no `%v`/`%s`/`slog.Any` formatting of a `*Config` at the
    pin or in a consumer, and it marshals `nats.password` and `nats.token` in clear
  - `streams.go:86` `func DeriveStreamSubjects(subject string) []string {`
  - `streams.go:228` `func FrameworkStreamAutoCreate(cfg *Config, name string)
    (*natsclient.StreamAutoCreateConfig, bool) {`.
    Its doc names a dashboard recreate path that nothing at the pin has; its only callers are
    `stream_bounds_test.go:978-1085`.
- **S2, required (surface audit, unexported, read only by tests or by nothing):**
  - `config.go:625` `loadJSONFile`: no caller.
  - `config.go:691` `mergeConfigs` and `:734` `removeNilValues`: doc "primarily used for testing"; caller
    `config_test.go:285`.
  - `security.go:116` `validateEnvVar` and `maxEnvVarLen`: no caller.
- **S3, required (surface audit b): fields parsed and read by nothing.**
  - `config.go:175-178` `Username`, `Password`, `Token`, `TLS` (`NATSTLSConfig`, `:183-188`), the env overrides
    `:761-769` and the decoder copies `:1051-1054`. Only `cfg.NATS.URLs` is read at the pin
    (`internal/boot/run.go:432-433`); this is #74.
  - `config.go:173-174` `MaxReconnects`, `ReconnectWait`, defaulted at `:512-513` and decoded at `:1050`,
    `:1058-1067`: no reader.
  - `config.go:202-203`, `:219-220` `JetStreamConfig.Domain`, `RetentionPolicy`, `ReplicationFactor`. The pin's own
    comment says "Not currently honored anywhere" (`:217`).
  - `config.go:656-662` and `:679-689` turn a `graph.<bucket>.ttl` string into nanoseconds for a `graph` key that no
    `Config` field has.

  Readers that exist: `JetStream.Enabled`, `MaxMemory` and `MaxFileStore` (`streams.go:311-357`
  `VerifyJetStreamLimits`), and `URLs` (boot).
- **S4, required unless the owner keeps it (Q2): fail open.** `config.go:561-593` `mergeFromMap` returns `base` on
  every marshal or unmarshal error (`:563`, `:569`, `:574`, `:583`, `:588`). Probe on the scratch copy:
  - `{"platform":{"org":"acme","id":"p1"},"nats":{"urls":"nats://other:4222"}}` loads with no error, `org=""` and
    `urls=[nats://localhost:4222]`.
  - `{"components":{"x":{"type":5}}}` loads with no error and zero components.

  Real trigger: an operator writes a list as a string. `Validate` then says "platform.org is required" about a file
  that sets it. The semteams and semboids loaders call no `Validate` after loading.
- **S5, required (#69 ruling 1): the env prefix.** `config.go:388` `envPrefix:  "STREAMKIT",`. The #69 ruling
  (closed, 2026-10-06) moves every outward-facing env var to `SEMENGINE_` and names these overrides. Note that the
  pin's prefix is `STREAMKIT_`, not the `SEMSTREAMS_` #69 assumed.
- **S6, required (surface audit c): text that is not true.**
  - `doc.go:15-18`, `:36-56`, `:76`: Manager text belongs to PR 2.
  - `doc.go:24-33` uses `AddLayer`/`EnableValidation`/`Load`.
  - `doc.go:84`, `:87` say `STREAMKIT_`.
  - `doc.go:94-100`: the layering example claims `log_level` survives a merge, but `PlatformConfig` has no
    `log_level` field, so the key is dropped silently. Probed on `testdata/base.json` + `production.json`.
  - `doc.go:108` says "no symlinks". `security.go:85` comment `// Check if it's a regular file (not symlink, ...)` sits
    over `os.Stat`, which follows links. Probe: a symlink to a valid file loads.
  - `security.go:49` `strings.Contains(filepath.ToSlash(absPath), "..")` does not catch traversal, because
    `filepath.Abs` has already cleaned the path. It does refuse any absolute path with `..` inside a name, for example
    `/etc/x/app..v2.json`, so it is a false refusal, not dead code. Absolute paths are otherwise not confined.
  - `README.md` (393 lines) documents the Manager, the `nats.username`/`token`/`tls` and `jetstream.domain` keys, and
    StreamKit-era services.
- **S7, optional: retained.**
  - `config.go:508` `Region: "gulf_mexico",` is a consumer-domain value used as a framework default.
  - `config.go:518-526`: default `discovery` and `heartbeat` services (their reader is `service`, later).
  - `config.go:1026-1070`: the custom `UnmarshalJSON` exists for `ReconnectWait` parsing. It becomes dead if S3's
    fields go.
  - `streams.go:67` `DeriveStreamName` is exported and read only in-package (`stream_bounds.go:378`).
  - Unknown keys are accepted silently anywhere in the file, which is surface audit item b. Measured: semteams carries
    an unknown top-level `tier` in 25 of its 28 config files.
  - `GOVERNANCE_VERDICT_AUDIT` and `MAX_DELIVERY_EVENTS` are provisioned at every boot (`streams.go:174-197`,
    `:279-292`), though their writers (rule core, change 6; `internal/maxdelivery`, change 7) are not ported yet.
- **Shapes 2, 3 and 5** (copied shell, a layered interface with one implementation, an error classified by its text):
  searched (`go func`, `interface {`, `err.Error()`, `strings.Contains(err`), no hit in the six files.

  Context rules: no `context.Context` field, no goroutine. The one root is `stream_bounds.go:432`
  `logger.Log(context.Background(), level, msg, args...)`, a log call with no work under it. `VerifyJetStreamLimits`
  bounds `AccountInfo` with `context.WithTimeout(ctx, 5*time.Second)` (`streams.go:330`) under the caller's context.

## 2. Pin ADRs

All 85 pin records were searched for the six file paths, `config.Loader`, `NewLoader`, `StreamsManager`,
`EnsureStreams`, `ValidateStreamDeclarations`, `SafeConfig`, `StreamMigrationOverride`, `ArchivalStream`,
`VerifyJetStreamLimits`, `frameworkStreams`, `LoadFromBytes`, "ordinary stream", "archival", "backing stream",
"firehose", `KV_`, `OBJ_`, `GOVERNANCE_VERDICT_AUDIT` and `MAX_DELIVERY_EVENTS`. The pin spec the slice implements
(`openspec/specs/stream-provisioning/spec.md`) was also read for the records it cites. The titles were then read for
config, boot, stream and authority
records.

- `config.go`:
  - **ADR-102** O-2 (one field `platform.id`; `instance_id` refused: `config.go:876-894`) and O-14 (the authority
    pair is bounded at load: `:806-874`).
  - **ADR-104** (the entropy suffix reserve: `:801-829`).
  - Both are in `docs/adr` with ledger rows. Binding; kept.
- `config.go` and `stream_bounds.go`: **ADR-100** (compositions are validated) cites `stream_bounds.go:259-320`. Its
  decisions are implemented by `composition`, a later PR, so it does not bind here.
- `streams.go`: **ADR-055** §3a (`GOVERNANCE_VERDICT_AUDIT` is a framework guarantee: `streams.go:159-186`, `:285`)
  and §5 T1 (the duplicates window: `:46`, `:480`). **ADR-039** names the same stream. **ADR-070** mentions
  `EnsureStreams` in passing.
- `streams.go`: **ADR-068** (graph retention, deletion and GC lifecycle; status Proposed 2026-07-04, design-only,
  amended by ADR-091). It decides that graph buckets carry no lifecycle retention, with no TTL, MaxAge or MaxBytes
  (`068-graph-retention-deletion-lifecycle.md:95-98`). The pin spec gives the retention contract of `KV_` and `OBJ_`
  backing streams to graph-retention, "ADR-068/073" (`stream-provisioning/spec.md:13-17`). The carried code that
  enforces it: `streams.go:374-378` `ErrBackingStreamNotProvisionable`, `:394-396` `checkOrdinaryStream`, and the
  guard first in `createStream` (`:588-606`). Not in `docs/adr`. In the tree, `natsclient/backing_stream_prefix.go:90`,
  `:121`, `:127` cite ADR-068/073, and `docs/adr/112-natsclient-contract.md:264` cites ADR-068.
- `stream_bounds.go`: **ADR-073** §1 (the graph's three-tier ingestion and retention contract; status Proposed
  2026-07-08, design-only, pending re-review, amended by ADR-091). Firehose data lives in JetStream streams with
  `MaxAge` plus a `MaxBytes` backstop, and on the stream tier a binding `MaxBytes` is fine
  (`073-graph-ingestion-retention-contract.md:75-78`). That is the bounds rule `missingBounds`
  (`stream_bounds.go:482`) enforces. It also names the backing-stream split above. Not in `docs/adr`.
- `stream_drift.go`, `security.go`, `doc.go`: no record governs them. The contract for the stream files is the pin spec
  `openspec/specs/stream-provisioning/spec.md` (626 lines, 11 requirements). Its readiness clauses are implemented
  in `service` (for example `service/stream_override_expiry.go:115`).
- ADR-058 and ADR-094 (boot phases, boot-only composition) govern `service`, not this slice.

## 3. Open issues

`gh issue list --state open --search config --limit 200` returned 13 issues.

- **#74** (TLS/token/credentials parsed, never applied) names `config/config.go:175-188`, `:761-768` and `:1051-1054`.
  This is in scope (S3).
- **#195** (adopt `natsfixture.Open`) names `config` 41 pin `NewTestClient` calls. 9 are in this slice's
  integration files; 32 are in Manager and key-family tests (PR 2).
- **#17** (deleted component, `config.Manager syncFromKV`) belongs to PR 2.
- Names the word, not the package: #198, #27, #280, #84 (`pkg/security`), #9, #281, #7, #25, #134, #109.

## 4. Callers

- **This repo:** none. `natsclient.ErrBackingStreamNotProvisionable` (`natsclient/backing_stream_prefix.go:77`),
  `natsclient.ErrStreamBoundsUndeclared` and `natsclient.StreamAutoCreateConfig` (`natsclient/stream.go:83`) are
  in-tree targets of the slice.
- **Pin, non-test, outside the six files:**
  - `NewLoader`/`LoadFile`: `internal/boot/run.go:728-729`, `composition/cli/main.go:122`.
  - `LoadFromBytes`: `processor/agentic-tools/executors/composition_tools.go:123`.
  - `GetOrg`/`GetPlatform`: `internal/boot/run.go:475-476`.
  - `NewStreamsManager`, `VerifyJetStreamLimits`, `EnsureStreams`:
    `internal/bootstrapobservability/bootstrap.go:233-237`.
  - `ComponentConfigs`: `service/component_manager.go:161`, `:178`.
  - `StreamConfigs`: `composition/analyze.go:23`, `service/component_manager.go:164`.
  - `Clone`: `composition/validate.go:30`.
  - `ExpiredMigrationOverrides` and `StreamMigrationOverrides`: `service/stream_override_expiry.go:111-115`.
  - `SafeConfig`/`NewSafeConfig`/`Mutate`/`CompareVersions`/`ModelRegistry`: `config/manager.go` (PR 2).
  - `Security`: `deps.Security` readers in `output/httppost`, `output/websocket`, `input/websocket`.
- **Consumers (symbol level):**
  - semsource `cmd/semsource/run.go`: `:261-262` `NewStreamsManager`/`EnsureStreams`; `:575-576`
    `NATSConfig{JetStream: JetStreamConfig{Enabled: true}}`; `:1003` `StreamDiscardNew`. It also uses `Config`,
    `ComponentConfigs`, `SafeConfig`, `NewSafeConfig`, `PlatformConfig`, `StreamConfig(s)`.
  - semteams `cmd/semteams/main.go:642-644` (`EnsureStreams`) and `:951` (`LoadFile`).
  - semboids `cmd/semboids/main.go:137-138` (`EnsureStreams`) and `:246` (`LoadFromBytes`).
  - semconnect: no import.
  - No consumer calls `AddLayer`, `EnableValidation`, `Load`, `SaveToFile`, `String`, `DeriveStreamSubjects`,
    `FrameworkStreamAutoCreate`, `StorageMode*` or `BucketConfig`.
- **Consumer config files:** 28 files with `platform` and `components` across the four consumers. Their `nats` blocks
  carry only `urls`, with no `jetstream` keys (a python survey over the tarballs).

## 5. Overlapping pull requests

- Draft #320 (`claude/port-flowgraph`, head `c3a5016`) changes only `openspec/changes/port-flowgraph/*` today. Its
  tasks add a ledger row and a `docs/repository-map.md` row, which both conflict with this change at the file end and
  at the map's entry count (`docs/repository-map.md:97`, "77 entries").
- No shared capability: `component/flowgraph` does not import `config` at the pin. `composition` does
  (`composition/validate.go:9`), so it merges after this change.

## 6. Category 5 shapes in the tree

- **Refuse a named key at load:** the pin's `config.go:882`
  `func rejectRemovedPlatformFields(raw map[string]any) error {`.
- **Refuse a setting nothing applies yet, naming the key:** `openspec/specs/security-config/spec.md:64` "Settings no
  SemEngine code applies yet are refused".
- **The `natsfixture` test client:** `graph/kvcatalog/fixture_client_integration_test.go:15-50` (`shared`, `TestMain`,
  `fixtureClient`).

## 7. Tests and the harness guards (pin test files)

- **In scope, carried unchanged** (each compiles alone against the six files and passes): `platform_config_test.go`,
  `stream_drift_test.go`, `streams_jetstream_limits_test.go`, `streams_test.go`.
- **In scope, need adapting:**
  - `config_test.go` and `safe_config_test.go`: the fixed-address guard. The scratch run of
    `TestNoFixedAddressesInTests` names `config_test.go:34`, `:57`, `:115`, `:262`, `:293`, `:314` and
    `safe_config_test.go:205`. Other edits follow S1-S5.
  - `streams_backing_stream_guard_test.go:253-255` and its integration twin `:118`: `graph.OwnerOf` is
    `kvcatalog.OwnerOf` here (`graph/kvcatalog/kvcatalog.go:238`).
  - `stream_bounds_test.go:969-1085`: five `FrameworkStreamAutoCreate` tests.
  - `readme_example_test.go:76`: `EnableValidation`.
  - Five integration files (`stream_bounds_`, `stream_drift_`, `streams_backing_stream_guard_`,
    `streams_duplicates_`, `streams_jetstream_limits_integration_test.go`): 9 `natsclient.NewTestClient(t,
    natsclient.WithJetStream())` calls, which do not exist here. Replacing them with the kvcatalog `fixtureClient`
    pattern makes all 23 integration tests pass on the scratch copy.
- **Out of scope, or mixed:**
  - `integration_test.go`: three tests. Two exercise only `helpers.go`. The third mixes `SafeConfig` concurrency with
    `GetString`/`GetInt`/`GetBool`, which `safe_config_test.go:12` `TestSafeConfig_ThreadSafety` already covers.
  - `example_test.go`: the `Loader` example uses `AddLayer`. The `SafeConfig` and `componentAccess` examples are
    commented-out code that prints a constant. The Manager examples are PR 2's.
  - `env_test.go`: `env.go`.
  - `bucket_name_test.go`, `key_family*_test.go`, `manager*_test.go`: PR 2.
- **Guards:**
  - `time.Sleep`, `Skip`, build tags other than `integration`: none in the in-scope files.
  - `t.Parallel`: only a comment at `stream_bounds_test.go:458`.
  - No `// Own server:` is needed, because the shared server is reset.
  - Pinned revive on the scratch copy: clean.
  - Reviewer re-run (round 1), clean: cleanup roots, retained context, bare `select {}`, public signatures naming
    `internal/` types, process-global registration. `config` is on the authority-field exception list
    (`internal/harness/contract/authority_test.go:224`).
- **Testdata:** `testdata/base.json` and `testdata/production.json` are read only by `example_test.go:18-44`.
  `example_config.json` is read by `config_test.go:354`. `config/rules/agentic-memory/*.json`: no reader at the pin.
  The pin's `test/predicate_rule_authoring_test.go:20` walks `config/rules`, and an archived pin review calls the
  directory "coincidentally-named ... unrelated".
