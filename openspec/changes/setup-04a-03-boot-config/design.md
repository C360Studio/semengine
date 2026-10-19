# Design: setup-04a-03-boot-config

This design depends on `INVENTORY PASS` for `inventory.md`. Its spec delta is `specs/config-loading/spec.md`. Pin
paths are at `8b99efe9`; repository lines are at `7f7136f`. Size: 3,148 carried pin lines, under the owner's waiver of
about 3,150 (#9 comment 6274501134). The drops below lower the count.

**Decision records.**

- ADR-102 (O-2, O-14) and ADR-104 bind `config.go`. Kept unchanged: `instance_id` is refused, and the declared
  authority pair is bounded at every load.
- ADR-055 §3a and §5 bind the `GOVERNANCE_VERDICT_AUDIT` declaration and the duplicates window in `streams.go`. Kept
  unchanged. ADR-055 is not copied to `docs/adr`: its normative content (envelopes, verdict publishing) is implemented
  in `processor/rule` and graph-ingest, and is raised when the rule core ports (change 6).
- ADR-100 binds `composition`, not this slice.
- No record governs `stream_bounds.go`, `stream_drift.go` or `security.go` (inventory § 2).
- ADR-113 does not apply: there is no lifecycle owner, no goroutine and no retained context.

## D1. Drop the dead surface (S1, S2, required)

- **Dropped:** everything in S1 and S2:
  - `StorageMode*`, `BucketConfig`, `Loader.AddLayer`, `EnableValidation`, `Load`, `Config.SaveToFile` with
    `safeWriteFile`, and `Config.String`;
  - `DeriveStreamSubjects`, `FrameworkStreamAutoCreate`;
  - `loadJSONFile`, `mergeConfigs`, `removeNilValues`, `validateEnvVar`;
  - the `graph.<bucket>.ttl` conversion and `parseDurationWithDays`.

  The basis is the surface audit (architect contract, Extraction slices). Nothing reads these at the pin outside their
  own tests, in this repo, or in the four consumers (inventory § 4).
- **Observable after the drop:**
  - `LoadFile(path)` and `LoadFromBytes(data)` apply the defaults, one document, the environment overrides and the
    authority-pair reserve, and return the `Config` without calling `Validate`. That is what every pin caller gets
    today, since nothing turns validation on.
  - Callers validate with `cfg.Validate()`, as boot does (pin `internal/boot/run.go:87`).
- **`FrameworkStreamAutoCreate`:** dropped. `streams.go` is an adapt anyway because `DeriveStreamSubjects` is also
  dead. Keeping it would need a contract line for a recreate path that has no reader.
- **Kept, with readers:**
  - `NewLoader`, `LoadFile`, `LoadFromBytes`, `GetOrg`, `GetPlatform`, `Validate`, `Clone`;
  - `SafeConfig`/`NewSafeConfig`/`Get`/`Update`/`Mutate`;
  - `CompareVersions` (reader `manager.go:370`, PR 2);
  - the stream declarations, `StreamsManager`, `EnsureStreams`, `VerifyJetStreamLimits`,
    `ValidateStreamDeclarations`, `ExpiredMigrationOverrides`, and the exception types and sentinels they return.
- **Options:**
  - (a) Chosen.
  - (b) Carry whole: about 15 symbols nothing reads, a `String` that prints secrets, and test-only code (#186).
  - (c) Do nothing: `service` and `composition` cannot build.

## D2. NATS settings nothing applies (S3, Q1)

- **Recommended:**
  - `NATSConfig` keeps `URLs` and `JetStream`, and `JetStream` keeps `Enabled`, `MaxMemory` and `MaxFileStore`.
  - Loading refuses each of these keys when present, with an error naming the key and saying it is not applied yet
    (#74): `nats.username`, `nats.password`, `nats.token`, `nats.tls`, `nats.max_reconnects`, `nats.reconnect_wait`,
    `nats.jetstream.domain`, `nats.jetstream.retention_policy`, `nats.jetstream.replication_factor`.
  - The shape is the pin's `rejectRemovedPlatformFields` (`config.go:882`), run on each raw document in `LoadFile`
    and `LoadFromBytes`, as `security-config` does for ACME (`openspec/specs/security-config/spec.md:64`).
  - The `_NATS_USERNAME`, `_PASSWORD` and `_TOKEN` overrides go with their fields. The custom `UnmarshalJSON`
    (`config.go:1026-1070`) goes too, because the default decoder now does all it did.
  - This retires #74 (`Closes #74`, task 2.3). The boot PR that builds the client adds each field back with its
    wiring and a test that fails when it is ignored.
- **Alternatives:**
  - Keep as at the pin, with #74 open for the boot PR. This breaks the surface-audit rule that a kept field has a test
    that fails when the field is ignored.
  - Drop the fields without refusing them. The keys would then be accepted silently, which is the same defect.
- **Consumer cost:** none measured. All 28 consumer config files set only `nats.urls`, and semsource sets
  `JetStream.Enabled` in code.
- Unknown keys elsewhere stay accepted (S7, optional): semteams' `tier` key is in 25 files, and a general refusal is
  a separate owner decision.

## D3. Fail closed on a malformed document (S4, Q2)

- **Recommended:** `mergeFromMap` returns its error, and `LoadFile`/`LoadFromBytes` fail with
  `failed to load <path>: <decode error>` (or `failed to parse config: ...`) instead of returning the defaults.
- This is a change from the pin's behaviour, so it waits for the owner's answer.
- Real trigger: `"urls": "nats://x:4222"` today loads as `org=""` and `urls=[nats://localhost:4222]` with no error
  (inventory § 1, S4).

## D4. Environment prefix and true text (S5, S6, required)

- `NewLoader` reads `SEMENGINE_PLATFORM_ID`, `_PLATFORM_TYPE`, `_PLATFORM_REGION` and `_NATS_URLS` (#69 ruling 1).
- `doc.go` is rewritten to this PR's surface: no Manager, no layering example, no symlink claim. It says that
  absolute paths are not confined and that symlinks are followed.
- The `security.go:85` comment is corrected. The dead `..` check at `security.go:49` stays, because it is harmless;
  the doc stops claiming it.
- `README.md` is cut to the `Config`, `Loader`, `SafeConfig` and stream sections, with a JSON example that loads. The
  Manager sections come back with PR 2.
- `example_config.json` is carried, because `config_test.go:354` reads it. `config/rules/agentic-memory/` and
  `testdata/` are not carried: nothing reads them once `example_test.go` stays out.
- The pin's agent contracts carry storage and stream guidance (`kv-or-stream`, already in this repo). No new section
  applies. The slice builds no `GenericJSON`.

## D5. Tests

- **Carried unchanged:** `platform_config_test.go`, `stream_drift_test.go`, `streams_jetstream_limits_test.go`,
  `streams_test.go`.
- **Adapted:**
  - `config_test.go`: URL literals replaced by hosts that do not match the guard (for example
    `nats://a.invalid:14222`). The default-URL assertion compares with `NewLoader().getDefaults()`. `STREAMKIT_`
    becomes `SEMENGINE_`. The username/password override test becomes a refusal test. `TestLoader_MergeConfigs` and
    `TestConfig_Save` are dropped. `EnableValidation` becomes `Validate()`.
  - `safe_config_test.go:205-206`: the literal, and `ReconnectWait`.
  - `stream_bounds_test.go`: the five `FrameworkStreamAutoCreate` tests removed.
  - `streams_backing_stream_guard_test.go` and its integration twin: `kvcatalog.OwnerOf`.
  - `readme_example_test.go`: `Validate()`.
  - The five integration files: `fixtureClient(t)` from a new `fixture_client_integration_test.go`, copied from
    `graph/kvcatalog`. This is #195's adoption for these files.
- **Dropped:** `integration_test.go` (helpers, or a duplicate of `TestSafeConfig_ThreadSafety`), `example_test.go` (no
  oracle; Manager examples are PR 2's), `env_test.go`.
- **New:** one test per refused key and env var name (D2, D4), for both `LoadFile` and `LoadFromBytes`. One test for
  the malformed document (D3). A `LoadFromBytes` round trip, because semboids reads it as framework API.
- **Sensitivity:** `task mutate:check` detects four mutants:
  - removing one key from the refusal table;
  - `mergeFromMap` returning `base` on error;
  - the prefix reverted to `STREAMKIT`;
  - `LoadFromBytes` skipping the refusal.
- Examples suffice for these checks: each rule is a fixed list or one branch.
- **Coverage:** measured and recorded on the PR. `config` joins `scripts/cover-check.sh` (80%, merged profile) in PR
  2, when the package is whole (03B D10; architect contract, One package per pull request).

## D6. Ledger rows

```yaml
- source_path: config
  source_sha: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128
  consumer_purpose: >-
    the boot configuration (Config, Loader, SafeConfig) and the JetStream stream declarations and provisioning
    Config.Validate checks; read by service, composition and boot (change 3) and by semsource, semteams and semboids
    (LoadFile, LoadFromBytes, NewStreamsManager, EnsureStreams); part 1 of 2, config.Manager is part 2
    (#9 comment 6274501134)
  destination: config
  contract: >-
    the config-loading spec; the pin's stream-provisioning spec requirements for declarations, bounds, exemptions,
    backing-stream refusal and drift as at the pin (readiness clauses arrive with service); ADR-102 O-2/O-14 and
    ADR-104 as at the pin
  dependencies_and_side_effects: >-
    model, pkg/platform, pkg/security, pkg/types, types, component, natsclient, pkg/errs (all in tree), nats.go
    jetstream; reads files and env vars; creates and updates JetStream streams in EnsureStreams; no goroutine
  known_risks: >-
    dropped as unread: StorageMode*, BucketConfig, Loader.AddLayer/EnableValidation/Load, Config.SaveToFile,
    Config.String, DeriveStreamSubjects, FrameworkStreamAutoCreate, loadJSONFile, mergeConfigs, validateEnvVar, the
    graph ttl conversion; helpers.go, env.go, minimal_config.go not ported; NATS auth/TLS/reconnect and three
    JetStream keys refused until boot wires them (Q1); a malformed document fails (Q2); env prefix SEMENGINE_ (#69).
    Retained: unknown keys accepted, gulf_mexico default region, framework streams for writers not yet ported.
    manager.go, key_family.go, bucket_name.go follow in part 2
  proving_tests: config/*_test.go, config/*_integration_test.go
  owner: "setup-04a-03-boot-config developer"
  disposition: adapt
- source_path: config/stream_bounds.go
  source_sha: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128
  consumer_purpose: stream bounds, exemptions and the declaration plan, held byte-equal to the pin
  destination: config/stream_bounds.go
  contract: as at the pin (task ledger:check)
  dependencies_and_side_effects: see the config row
  known_risks: none beyond the config row
  proving_tests: config/stream_bounds_test.go, config/stream_bounds_integration_test.go
  owner: "setup-04a-03-boot-config developer"
  disposition: carry
- source_path: config/stream_drift.go
  source_sha: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128
  consumer_purpose: drift detection and in-place repair of ordinary streams, held byte-equal to the pin
  destination: config/stream_drift.go
  contract: as at the pin (task ledger:check)
  dependencies_and_side_effects: see the config row
  known_risks: none beyond the config row
  proving_tests: config/stream_drift_test.go, config/stream_drift_integration_test.go
  owner: "setup-04a-03-boot-config developer"
  disposition: carry
```

The `processor/graph-ingest` package row plus its `component.go` file row is the precedent for a package row beside
file rows. `docs/repository-map.md` gains a `config/` row, and its entry count goes up by three.

**Issues:**

- #74 is retired by this PR if Q1 is answered yes. Otherwise it stays open for the boot PR, as a required
  correction under the surface audit.
- #195 stays open: this PR moves 9 of config's 41 calls to `natsfixture`, and the other 32 are PR 2's.
- #17 belongs to PR 2.

**Merge order:** independent of #320. Whichever merges second keeps both ledger appends and resets the map count.
`composition` comes after this PR.

## Owner questions

- **Q1 (D2).** Should loading refuse the NATS username, password, token, TLS block, reconnect settings and the three
  unused JetStream keys, naming each key, until boot wires them? Recommended: yes, which closes #74.
- **Q2 (D3).** Should a file with a wrong-typed value fail to load, instead of being silently replaced by defaults?
  Recommended: yes.
