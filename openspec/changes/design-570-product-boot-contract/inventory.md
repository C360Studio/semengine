# Inventory: what a product's own main must know to boot SemEngine (#570)

base: 3987dc117c4aefd0087f6720a7234196cc97ea18 (the claim commit; its tree equals origin/main 8d21b346)
SemSource read: C360Studio/semsource origin/main e6c39b1b26f5ae908cd88d0e8d0f1f7e5e20456f, `cmd/semsource/` and
`config/config.go` only (boot path, read-only, `docs/inventory-scope.md` row "semsource": dependency closure and
imports). semboids read: origin/main ab4e0fb, `cmd/semboids/main.go` only, for one boot shape (F2). Its scope row
names "the workload it drives ... lifecycle use", so reading its composition root goes slightly past that row.
Track: `design` (the change may touch more than one capability: `service-lifecycle`, `config-manager`,
`config-loading`, `composed-stack`).

Sources the issue names, re-opened on this base:
`openspec/changes/archive/2026-11-09-test-557-composed-stack-smoke/inventory.md:137-149` (adopter seam inventory),
`notes/2.1.md` (minted ID), `notes/2.2.md` (bucket wins over file), `notes/probe.md` (watch across a broker restart),
and `inventory.md:109-136` (seams, probe summary).

Revision 2 (inventory review round 1): adds facts F1 to F6 (vocabulary, which copy each step reads, runtime
component writes at boot, NATS connection inputs, unregistered factories, optional calls and teardown), answers E1,
and completes sections 2, 3 and 5 and the collision table.

## 1. The seven items, re-checked on today's tree

Each item has four parts: whether it is still true (with pins), what a product's `main` sees if it does nothing,
where it finds out (compile > boot error > typed runtime error > log > doc > nowhere), and whether SemSource meets it.

### Item 1: `EnsureStreams` before `StartAll`

- `config/streams.go:328` — `func (sm *StreamsManager) EnsureStreams(ctx context.Context, cfg *Config) error {`
- `internal/smoke/stack_integration_test.go:272` —
  `if err := config.NewStreamsManager(client, logger).EnsureStreams(ctx, effective); err != nil {`
  (after `config.Manager.Start`, on the effective configuration)
- `docs/adr/058-boot-lifecycle-phases.md:106` — "must run **before** `StartAll` (which is post-wiring and starts
  services in" (ADR-058 classes streams as Phase A, composition-root work; `:108` — "lives in the **composition
  root**, factored into **plain shared helper")
- Stream consumers in production code (`git grep -n 'ConsumeStreamWithConfig\(Contexts\)\?(' -- '*.go'
  ':!*_test.go' ':!natsclient/*'`): two, and both wait for the stream and then fail Start.
  - graph-ingest: `Start` calls `setupSubscriptions` (`processor/graph-ingest/component.go:466` —
    `if err := c.setupSubscriptions(ctx, submitCtx); err != nil {`). That waits for the stream
    (`processor/graph-ingest/consume.go:85` — `created, err := c.waitForStream(ctx, streamName)`): 30 polls from
    100 ms, x1.5, capped at 2 s (`:192` — `maxInterval := 2 * time.Second`). That is about 49 s by arithmetic on
    those constants. Then it fails (`:218` — `return time.Time{}, fmt.Errorf("stream %s not available after %d
    retries", streamName, maxRetries)`).
  - objectstore: `storage/objectstore/component.go:439` — `handle, err := c.natsClient.ConsumeStreamWithConfig(ctx,`.
    It is preceded by its own wait (`:425` — `if err := c.waitForStream(ctx, binding.stream); err != nil {`), with
    `AutoCreate: false` (`:450`).
- Stream publishers that only log when the stream is missing: the health publishes.
  - `service/component_manager_lifecycle.go:254` — `err = cm.natsClient.PublishToStream(ctx, subject, data)`, then
    `:257` `cm.logger.Debug("Health publish failed", ...)` plus a counter.
  - `service/service_manager.go:648` (same shape).
  - Others: `service/metrics_forwarder.go:166` and `storage/objectstore/write.go:90`.
- Do-nothing path, traced from code, not run: `StartAll` returns graph-ingest's wrapped error after about 49 s
  (computed, not measured); an ended `ctx` ends it sooner. A composition without a stream-consuming component boots,
  and its health stream publishes then fail at Debug level plus a counter.
- Found out: a boot error for stream consumers. For health, a Debug log line and a counter: close to nowhere.
- Where the docs say it: `config/README.md:301` (what `EnsureStreams` does) and `service/README.md:750` (the METRICS
  stream). The boot example in `service/doc.go:68-97` and the `service/README.md` "Composing and starting" section
  (`:82`) do not call it. `docs/contract.md:191` — `// Build the service and component managers from these registries
  and start them.`
- Ordering: SemSource calls it on the FILE configuration, before `config.Manager.Start`.
  - semsource `cmd/semsource/run.go:145` — `nc, ssCfg, err := setupNATS(ctx, flags.natsURL, semsourceCfg, logger,`
    -> `:261` — `if err := streamsManager.EnsureStreams(ctx, ssCfg); err != nil {`, then `:162` —
    `configMgr, err := startConfigManager(ctx, ssCfg, nc, logger)`.
  - The smoke test calls it on the effective configuration after `Start`.
  - F2 covers the whole class of "which copy" questions. No doc names the order.
- SemSource meets it: yes (`run.go:261`).

### Item 2: build the payload registry and register its types

- `internal/smoke/stack_integration_test.go:287` — `if err := message.RegisterPayloads(payloads); err != nil {`
- `docs/contract.md:146-191`: the "What is your job as a consumer" sketch shows `payloadregistry.New()` and the
  `RegisterPayloads` loop. `:152` — "it compiles there), and SemEngine's are fixed when each package is copied over."
  This is stale: SemEngine's packages are copied now. The sketch stays in this change's scope.
- AGENTS.md rule (verify+review): "imports register nothing, only `main` aggregates". Registration by `main` is
  intended.
- Do-nothing path, no registry: `processor/graph-ingest/component.go:272` — `return nil,
  errs.WrapInvalid(errors.New("payload registry is required"), "CreateGraphIngest", "factory", "PayloadRegistry
  required")` gives a boot error. The component manager itself does not refuse a nil registry
  (`service/component_manager.go:179-185` checks only `Manager` and `ComponentRegistry`).
- Do-nothing path, conditional type: with `enable_hierarchy`, graph-ingest refuses at construction when the
  container type is not registered (`processor/graph-ingest/component.go:294-299`, "register it with
  inference.RegisterPayloads"). This is a boot error that names the fix.
- Do-nothing path, registry without the product's own type: each such input is a poison message.
  `processor/graph-ingest/ingest.go:23` — `// every error it returns is a poison message there (counted, logged and`
  (terminated). Found out: a log line plus a counter, per message, at runtime. The mutation RPC refuses with code
  `message_type_unregistered` (`processor/graph-ingest/canonical_mutations.go:146-148`): a typed runtime error.
- SemSource meets it: yes. semsource `cmd/semsource/run.go:332` — `func buildPayloadRegistry()
  (*payloadregistry.Registry, error) {` registers five packages' payloads, `inference` among them (`:340`).

### Item 3: `http_port` and `metrics` default to fixed ports on every address

- `service/service_manager.go:165` — `if cfg.HTTPPort == 0 {` / `:166` — `cfg.HTTPPort = 8080` (unchanged since
  2026-11-09)
- `service/service_manager_http.go:75` — `listener, err := net.Listen("tcp", ":"+strconv.Itoa(cfg.HTTPPort))` (every
  address; `ManagerConfig` has no host key: `service/service_manager_config.go:10-27`)
- `service/service_configs.go:90` — "materializeServiceConfig(resolved, "metrics",
  json.RawMessage(`{"path":"/metrics","port":9090}`))". Metrics is on unless the configuration sets it to
  `enabled: false`; `:88-89` say an explicit false is kept.
- `service/metrics.go:78` — `cfg.Port = 9090`; `metric/handler.go:43` — `port = 9090`. Port 0 means 9090 at both
  layers.
- `service/metrics.go:46` — `testListener net.Listener` (a test-only field; open issue #614)
- The bound address: `metric/handler.go:298` — `func (s *Server) Address() string {` reports the bound listener's
  address. The service `Manager` exposes no reader for either listener. Its exported methods are
  `service/service_manager.go:150,266,306,322,342,674`, `service/service_manager_http.go:44,127` and
  `service/rule_pack_bind.go:23` (`ProjectionBinders`); none reads an address.
- Docs: `service/README.md:128` — ``| `http_port` | `8080` | the main listener's port, on every interface |`` (stated),
  and `service/doc.go:104`.
- Do-nothing path: binds 8080 and 9090 on every interface. A second process on one host fails with
  `bind HTTP listener: ...` (`service_manager_http.go:77`): a boot error. Exposure on every interface: nowhere but the
  README row.
- SemSource meets it: yes, and copies the defaults into its own layer. semsource `config/config.go:205` —
  `c.HTTPPort = 8080`; `cmd/semsource/run.go:1061` — `smCfgJSON, err := json.Marshal(map[string]any{"http_port":
  cfg.HTTPPort})`; `:1066` — `metricsPort := 9091`.

### Item 4: a `ServiceManager` cannot be restarted

- `service/service_manager.go:330` — `// StartAll and a StartAll after StopAll (errs.ErrAlreadyStarted); a refused`
  (the issue cited `:329-331`; still there)
- `service/doc.go:40` — `// after any Stop (errs.ErrAlreadyStarted): a BaseService runs once, so build a`;
  `config/README.md:204-207`: `config.Manager` is the same ("build a new `Manager` to start again").
- ADR-095 (one-shot running lifecycle) decides one Start per owner. ADR-113 (settled; not re-opened) governs Start
  and Stop contexts.
- Do-nothing path: a second `StartAll` returns `errs.ErrAlreadyStarted`: a typed runtime error at the call.
- SemSource meets it: no. It starts once and exits (semsource `cmd/semsource/run.go:202` — `if err :=
  manager.StartAll(signalCtx); err != nil {`, then `StopAll` and return).

### Item 5: the client must reconnect; a watch stalls about 20 s after a restart

- `natsclient/client.go:311` — `maxReconnects:   -1, // infinite by default` / `:312` — `reconnectWait:   2 *
  time.Second,` (the issue cited `:317`; moved)
- Timing evidence at the pinned version:
  - The probe: `openspec/changes/archive/2026-11-09-test-557-composed-stack-smoke/notes/probe.md:3`. It ran
    nats.go v1.55.0 (the version in `go.mod:18`) against image `nats:2.15.0-alpine` (the image pin), with a short
    outage (189 to 280 ms).
  - The result: puts reached the watch 19.53 s, 19.56 s and 19.46 s after the restart (`probe.md:18`).
  - What it counts as: a program posted with source, command, versions and output. That is an ADR-112 probe at the
    pinned version, for a short outage only.
  - No in-tree test asserts the stall's length. `TestSmoke_BrokerRestartUnderRunningStack`
    (`internal/smoke/smoke_integration_test.go:1094`) proves recovery, not timing.
  - Any design text about the length must say it is nats.go's and not promised.
- `graph/follower/follower.go:23` — `//     timing (about 20 s measured at nats.go v1.54.0; TestFollowerDeclaresLostFeed
  deletes the` (names v1.54.0; the tree pins v1.55.0; filed as #741)
- `openspec/specs/composed-stack/spec.md:85-99`: a running stack recovers from a broker restart. `:8` still says a
  longer outage is not covered, but #574 was closed by PR #727 (`cd94648e`); filed as #741.
- Do-nothing path: the defaults reconnect forever. During the stall, readiness envelopes and answers lag, and the
  follower reads behind until the watch resumes. Found out: nowhere but the doc comment above.
- SemSource meets it: it sets the same values explicitly and adds an exit policy. semsource
  `cmd/semsource/run.go:478` — `natsclient.WithMaxReconnects(-1),`; `:484` —
  `natsclient.WithConnectionLossTimeout(connectionLossGrace),` (`natsclient/options.go:62`).

### Item 6: `platform.id` is a stem; the effective ID is minted at first boot

- `config/manager.go:103` — `const platformIdentityKVKey = "platform_identity"`; `:105-117`: the record
  `{org, stem, id}`. `:108` — `// under, so it carries exactly these three fields.` The type is unexported and there
  is no exported reader of the record.
- ADR-104 already decides the outside-producer answer.
  - `docs/adr/104-unique-platform-authority.md:138` — "- Adopter fixtures and e2e stop predicting the pair from a
    configuration file and read" (the record).
  - `:66-70`: the opt-out is pre-creating the record with `id == stem`.
  - `:178-179` — "A sister conforms when: its composition root passes `deps.Platform` unchanged; its configuration
    files declare the STEM and accept the minted suffix".
  - `:183`: "The record's shape is normative in the `component-runtime-config` capability spec". No spec of that
    name exists here (`ls openspec/specs`).
  - `openspec/specs/config-manager/spec.md:75-120` states adopt, mint and refuse, but not the record's field shape.
- `config/README.md:214-220` documents the mint and adopt rules and the key name (`:217`). It does not give the
  record's JSON shape or tell a producer outside the process to read it.
- An outside process's existing read path: semsource reads the config bucket directly.
  `cmd/semsource/desired_config.go:57` — `bucket, err := semconfig.BucketName(org, stem)`; `:61` —
  `reader, err := kvcatalog.OpenCatalogReader(ctx, client, bucket)`. Through exported SemEngine calls, that is the
  pair an outside process would use to read `platform_identity`; it would then decode the unexported shape itself.
- Do-nothing path, outside producer: one that builds IDs from the declared stem gets every entity refused with
  `authority_foreign` (`notes/2.1.md:8-10`). Found out: a typed runtime refusal per entity, the first time it
  publishes.
- Do-nothing path, in-process `main`: see F2. A `main` that builds `deps.Platform` from the configuration it loaded
  boots with no error, and its own entities carry the stem.
- In-process reading done right: `GetConfig().Get().Platform.ID` after `Start` (`config/README.md:195`; smoke
  `stack_integration_test.go:324`; semsource `run.go:398` — `platform := types.PlatformMeta{Org:
  effective.Platform.Org, Platform: effective.Platform.ID}`).
- SemSource meets it in-process, correctly. Producers outside the process: none seen in the files read.

### Item 7: after the first boot the bucket, not the file, decides

- `config/manager.go:451` — `func (cm *Manager) arbitrate(ctx context.Context, kvStore *natsclient.KVStore, hasConfig
  bool) error {`. A newer file version pushes (`:476-484`); an equal version syncs from the bucket, logged at Debug
  only (`:492-496`); an older one logs a Warn.
- `config/README.md:222-242`: the "Which copy wins" table, with `:228` — "| The versions are equal, or the file's is
  older | The bucket wins |", and `:241-242`: "raise the file's `version` to write it again". The rule is documented.
  The issue's "has no effect on later boots" holds only while `version` is unchanged.
- `openspec/specs/config-manager/spec.md:136` (Version arbitration requirement).
- Open issues on the same path: #646 (a newer-file push leaves keys the file dropped, and they return) and #647 (an
  accepted runtime change does not advance the bucket's version).
- Do-nothing path: a `main` that never sets or raises `version` (no `version`, or `0.0.0`) keeps the first boot's
  configuration forever, and ports and components edited in the file are ignored. Found out: a Debug log line at
  equal versions; nowhere else.
- SemSource meets it in full.
  - semsource `cmd/semsource/run.go:610` — `Version: fmt.Sprintf("%d.%d.0", compositionGeneration,
    cfg.ConfigRevision),`.
  - `cmd/semsource/config_drift.go:28` — `func startConfigManager(...)`: a drift warning (`:135`), a prune of retained
    components (`:89` `DeleteComponentFromKV`), and a `// shortcut:` tied to #646 (`:64`). The file is 257 lines.

## 1b. Boot facts the issue does not list

### F1: register the vocabulary before its readers are built

- `graph/entitytext/text.go:38-41` — "The vocabulary's label predicates are read once, here, as the name index reads
  them, so a product registers its own before it builds the components that read text"; `:48` —
  `labels:   vocabulary.DiscoverLabelPredicates(),`.
- Read once at construction, through `entitytext.New`:
  - graph-lexical: `processor/graph-lexical/component.go:122`, with `:96` — "It reads the vocabulary's label
    predicates, so a product registers its own first."
  - graph-embedding: `processor/graph-embedding/component.go:209`.
  - graph-clustering: `graph/clustering/maintainer.go:106`.
- Read once at Start: graph-index's `processor/graph-index/index.go:87` —
  `c.aliasPredicates = vocabulary.DiscoverAliasPredicates()` and `:88` —
  `c.namePredicates = vocabulary.DiscoverLabelPredicates()`, called from Start
  (`processor/graph-index/component.go:265`). Nothing checks for an empty `namePredicates` (`git grep -n
  namePredicates -- processor/graph-index ':!*_test.go'` shows only the field, the assignment and reads at `:380`,
  `:491`, `:527`).
- Smoke registers its labels before `StartAll`: `internal/smoke/stack_integration_test.go:111-112` (comment),
  `:282-283` — `vocabulary.Register(predicate, vocabulary.WithAlias(vocabulary.AliasTypeLabel, 0))`.
- SemSource registers at import time: semsource `cmd/semsource/run.go:18-19` (blank import of `source/ast`), which
  runs `source/ast/vocabulary.go:20-23` registration from `init`.
- Do-nothing path, or a late registration: the name index and the text rules see no product label predicates. Name
  lookup and text search silently miss product names; no error, no log.
- Found out: nowhere. No boot doc mentions the vocabulary (`git grep -n -i vocabulary -- docs/contract.md
  service/README.md service/doc.go config/README.md`: no hits). Only the doc comments above say it.

### F2: which copy each boot step reads, the loaded file or the effective configuration

- `config.Manager.Start` never writes the caller's `*Config`.
  - `NewConfigManager` wraps the pointer (`config/manager.go:144` — `config:      NewSafeConfig(cfg),`;
    `config/config.go:68-75`).
  - Every change goes through `Mutate`, which swaps in a clone (`config/config.go:123-133`, `draft :=
    sc.config.Clone()` ... `sc.config = draft`).
  - The minted ID is applied that way (`config/manager.go:1485-1489`, `applyEffectivePlatformID`).
  - So after `Start`, the caller's `cfg.Platform.ID` still holds the stem, and `GetConfig().Get()` holds the minted
    ID.
- Platform: `service/component_manager.go:217` — `platform:          deps.Platform,`, copied as given. Nothing in
  `service` compares it with `config.Manager`'s effective ID (`git grep -n 'Platform' -- service/*.go ':!*_test.go'`:
  only this line). graph-ingest checks authority against it: `processor/graph-ingest/authority_gate.go:52` —
  `return semtypes.ValidateEntityIDAuthority(subject, c.platform.Org, c.platform.Platform, importLane)`.
- Do-nothing path, `deps.Platform` built from the loaded cfg:
  - The boot has no error.
  - In-process entities carry the stem, while `platform_identity` records the minted ID.
  - Outside producers that read the record, as ADR-104 tells them to, get `authority_foreign`.
  - This silently undoes ADR-104 (`:178-179`, "passes `deps.Platform` unchanged" assumes it came from the effective
    configuration).
  - Found out: nowhere at boot.
- Seen in a consumer: semboids origin/main ab4e0fb `cmd/semboids/main.go:146` — `platform := extractPlatformMeta(cfg)`,
  built from the loaded cfg before `NewConfigManager` (`:148`), at `:381-389`. semboids is still on SemStreams
  `v1.0.0-beta.160` (`go.mod:6`), and minting arrived in the beta.163 wave (ADR-104:134). So this is harmless today
  and becomes the silent case when semboids moves to SemEngine.
- The same class elsewhere:
  - `ConfigureFromServices(effective.Services, ...)`: smoke `:304`, semsource (`createServiceManager`, `run.go:397`,
    `:420`). A `main` passing the file's services would run the file's services while the component manager reads
    the effective components.
  - `EnsureStreams` (item 1): smoke uses effective; semsource uses the file.
  - No doc states which copy each step must read.

### F3: component writes before the component manager land in this boot

- Writers: `config/manager.go:994` — `func (cm *Manager) PutComponentToKV(...)` and `:923` —
  `func (cm *Manager) DeleteComponentFromKV(...)`. The in-memory apply is synchronous (`:1015-1017` `applyRevision`
  before return).
- The component manager snapshots once when it is built: `service/component_manager.go:186-189` — "Runtime
  construction reads one snapshot of the configuration. Later desired writes remain configuration-manager state for
  the next process."
- So a write after `Start` and before `ConfigureFromServices` lands in this boot; a write after it lands only in the
  next boot.
- SemSource uses the window: semsource `cmd/semsource/run.go:174` — `watches, err := admitDiscovered(ctx,
  semsourceCfg, expandResult, ssCfg.Components, configMgr, logger)` -> `cmd/semsource/discovery.go:274`
  (`PutComponentToKV`), `:290` (`DeleteComponentFromKV`). Also `config_drift.go:89` (a prune).
- Found out: the component-manager doc comment only.

### F4: NATS connection inputs are the `main`'s to build

- `config/config.go:713` — `unappliedNATSKeys      = []string{"username", "password", "token", "tls",
  "max_reconnects", "reconnect_wait"}`: these keys are refused at load, naming the key (`:707-711`).
- `:588` — `var unappliedNATSEnv = []string{"_NATS_USERNAME", "_NATS_PASSWORD", "_NATS_TOKEN"}`: credential env vars
  are refused (`:596`).
- So the `main` builds the client and its credentials or TLS itself, through natsclient options (smoke
  `stack_integration_test.go:250`; semsource `run.go:478-484`).
- Environment overrides (`config/config.go:566-582`, prefix `SEMENGINE`, `:366`): `SEMENGINE_PLATFORM_ID` replaces
  `platform.id`, and `SEMENGINE_NATS_URLS` the URLs. `config/README.md:145` lists them.
- Because the bucket is named by the declared pair (`config/README.md:173-175`), a stem changed from the environment
  opens another bucket. That means a first boot there and a new minted ID: items 6 and 7, derived, not run.
- Specs: `openspec/specs/config-loading/spec.md`, `openspec/specs/security-config/spec.md`.

### F5: an enabled component whose factory is not registered

- SemEngine refuses boot, naming every failure: `service/component_manager.go:233-262` (`initialize`). At `:261-262`:
  "Every enabled component the manager cannot build refuses boot. The creation pass still finishes, so the refusal
  names every failure."
- SemSource carries its own guard with a comment that does not match SemEngine: semsource
  `cmd/semsource/desired_config.go:42-43` — "The pinned ComponentManager logs such a component and skips it". The
  guard is at `:45`. This is a doc gap: nothing a product reads first says the manager refuses.
- Found out: a boot error.

### F6: optional and conditional calls, and teardown

- Required in every boot spelling seen:
  - `cfg.Validate()` (smoke `:246`)
  - `client.WaitForConnection` (smoke `:258`)
  - `service.RegisterAll` (smoke `:300`)
- Optional, before `StartAll`:
  - `Manager.StartHealthListener` (`service/service_manager_http.go:127`): probes answer during a slow boot, such as
    item 1's wait.
  - `Manager.UseHTTPMiddleware` (`:44`).
- Conditional:
  - `service.ConfigureRulePackMutations` (`service/rule_pack_bind.go:79`), "after ConfigureFromServices and before
    StartAll" (`:76-78`). No production `ProjectionBinder` exists yet (`git grep -n 'ProjectionBinder\b' -- '*.go'
    ':!*_test.go'` outside `rule_pack_bind.go`: docs only), so today it has no effect.
  - `inference.RegisterPayloads` when `enable_hierarchy` (item 2).
- Exported with no production caller: `service.WireGraphRuntime` (`service/graph_runtime.go:16`) and
  `StreamsManager.VerifyJetStreamLimits` (`config/streams.go:229`).
- Teardown order (smoke defers, so they run in reverse): `manager.StopAll` (`:319`), then `configManager.Stop`
  (`:269`), then `client.Close` (`:254`). SemSource has the same order (`run.go:148-152`, `:163-168`, `:215`). No
  boot doc states it.

## 2. Every current spelling of the boot sequence

The fact being modelled is the ordered startup a product's `main` runs.

| Spelling | Where | Order it uses |
| --- | --- | --- |
| Smoke composition root | `internal/smoke/stack_integration_test.go:242-320` | load -> `Validate` -> client Connect -> `WaitForConnection` -> `config.Manager.Start` -> `EnsureStreams(effective)` -> component registry -> vocabulary labels -> payload registry -> `RegisterAll` -> `ConfigureFromServices(effective)` with `deps.Platform` from effective -> `StartAll`; teardown `StopAll`, `Manager.Stop`, `Close` |
| `service` package doc | `service/doc.go:68-97` | registry -> `UseHTTPMiddleware` (optional) -> `ConfigureFromServices` -> `StartAll` -> `StopAll` (no client, no config.Manager, no streams, no vocabulary) |
| `service/README.md` | `:82` "Composing and starting" | as `doc.go`; no `EnsureStreams` (grep: only `:750`) |
| `config/README.md` | `:177-198` | config.Manager Start/Stop only; streams at `:301` |
| `docs/contract.md` | `:146-191` | component and payload registries only; ends at "start them" |
| ADR-058 | `docs/adr/058-boot-lifecycle-phases.md:91-130` | Phase A before `StartAll`: NATS connection, streams, metrics, loggers, payload/tool/component registries, KV buckets, the ownership Registry, lifecycle workflow registrations, rule-pack projection contracts (`:99-101`), in "plain shared helper functions" |
| SemSource | semsource `cmd/semsource/run.go:95-190` | client (with exit policy) -> `EnsureStreams(file)` -> config.Manager Start -> drift and prune -> `PutComponentToKV` discovered -> component registry (+ own factory guard) -> payload registry -> service manager with `deps.Platform` from effective -> `StartAll`; vocabulary at import time |

Searches:

- `git grep -n 'EnsureStreams' -- ':!*_test.go' ':!openspec/changes/archive'` gives `config/README.md`,
  `config/doc.go`, `config/stream_bounds.go`, `config/streams.go`, `docs/admission-ledger.yaml`,
  `natsclient/stream.go`, `openspec/specs/service-lifecycle/spec.md` and `service/README.md`.
- No exported boot helper: `git grep -n -i 'func Boot\|func Run(' -- service config` found none at package level.
- `ls cmd examples`: neither exists.

## 3. Adjacent claims

- Open PRs (`gh pr list --state open`, 2026-11-23):
  - #739 is this claim; it now carries this inventory file.
  - #696, #697 and #698 are Dependabot PRs touching `go.mod`/`go.sum`. No file or capability overlap. #696 moves the
    nats-server Go library (2.15.2 -> 2.15.3), not the image the item 5 probe used.
- Open issues on `service`, `config` or the boot path (searches: `service`, `config`, `boot`, `EnsureStreams`,
  `http_port`, `platform identity`, `minted`, `reconnect`, `StartAll`):
  - #614 metrics test-only listener / port 0 (item 3, direct)
  - #646 newer-file push leaves dropped keys (item 7, direct)
  - #647 a runtime change does not advance the version (item 7 and F3, direct)
  - #740 warm restart: ENTITY_STATES watchers replay on one connection and trip slow-consumer at boot (boot path and
    reconnect, adjacent; not one of the seven; filed 2026-11-23, no milestone)
  - #741 doc drift: `composed-stack` spec `:8` and `graph/follower/follower.go:23` (item 5, filed from round 1)
  - #625 flake-like failure in `TestConfigManagerStartupVersionArbitration` (item 7 test, adjacent)
  - #196 lifecycleguard adoption (item 4, adjacent; settled shape)
  - #361 CLOSED (alpha.8), noted only
  - The rest name the word, not the boot path: #9 #11 #24 #25 #27 #29 #36 #68 #84 #108 #109 #110 #134 #195 #300 #350
    #477 #484 #485 #502 #570 #630 #641 #665 #682 #702 #703 #712 #722.
- Specs:
  - `service-lifecycle`
  - `config-manager` (`:75-160`)
  - `config-loading`
  - `security-config`
  - `composed-stack` (`:8` stale, `:85-99`)
  - `component-registration`
  - `vocabulary-packs`
- ADRs:
  - ADR-058: Phase A in the composition root
  - ADR-094: boot-only composition
  - ADR-095: one-shot lifecycle (item 4)
  - ADR-104: minted ID and the record read contract (item 6, F2)
  - ADR-112: natsclient promises (item 5 timing)
  - ADR-113: settled
  - ADR-115: HTTP surfaces (item 3)
- Owner rules in force: plain words; no port history in prose (#606); no SemStreams state migration; natsclient never
  reinvents nats.go (#170, ADR-112).
- `task spec:check` fails on this change until its design commit adds deltas or declares none; expected at this
  stage.

## 4. Who it is for

Not applicable at inventory: this phase introduces no symbol. The named consumer is SemSource (pins alpha.7); semboids
is the next to move (F2). The in-tree product-shaped caller is `internal/smoke`.

## 5. Problem shape and closest instances

Shapes: (a) ordered composition-root wiring, and (b) observe instead of predict, for the bound port and the minted ID.

- (a) Closest instance: ADR-058's Phase A in plain helper functions. No such helper exists in SemEngine today
  (section 2 searches).
- (b) Closest instances:
  - `metric/handler.go:298` `Address()` reports the bound address.
  - The `platform_identity` record: ADR-104 says "the framework observes; nobody computes".
  - `config.Manager.GetConfig().Get().Platform.ID` after `Start`.
  - From outside the process: `config.BucketName` + `kvcatalog.OpenCatalogReader` (semsource
    `desired_config.go:57-61`), the exported pair that reaches the record.

## 6. Pin decision records

This change carries no package from the pin. The ADRs that govern the surfaces it reads are listed in section 3.

## Same-class collision table (boot ordering: runtime coordination)

| Dimension | Evidence |
| --- | --- |
| Semantic class | The order and inputs of a process's startup |
| Owners | each product `main`; `internal/smoke` runStack; ADR-058 assigns it to the composition root |
| Catalogs | `component.Registry`, `payloadregistry.Registry`, the `vocabulary` registry (`vocabulary.Register`, read by `DiscoverLabelPredicates` / `DiscoverAliasPredicates`), `service.NewServiceRegistry` + `RegisterAll` |
| Status | `/readyz` (`docs/contract.md:132`); `service/service_manager.go:60-61` stop flag; `StartHealthListener` probes during boot |
| Lifecycle | one-shot `config.Manager.Start`, `StartAll`/`StopAll` (ADR-095, ADR-113); teardown `StopAll`, `Manager.Stop`, `Close` |
| Ownership | config bucket keyed by declared pair (`config/README.md:173-175`); one writer assumed (semsource `config_drift.go:61-63`) |
| Readers | `config.Manager` (arbitration); component-manager snapshot (`service/component_manager.go:186-189`); smoke test; SemSource via `GetConfig` and directly via `BucketName` + `OpenCatalogReader` (`desired_config.go:57-61`) |
| Writers | `config.Manager.pushToKV`; `PutComponentToKV` / `DeleteComponentFromKV` (`config/manager.go:994,923`), used at boot by SemSource (`discovery.go:274,290`, `config_drift.go:89`); `EnsureStreams` (streams) |
| Recovery | restart = new process (item 4); version bump to re-push (item 7) |

## Adopter seam inventory (a developer writing a product `main`)

1. Must know: the seven items, plus F1 to F6.
   - Call `EnsureStreams`, build `deps.Platform` and pass services, all from the effective configuration after
     `config.Manager.Start`.
   - Register the vocabulary before the components are built.
   - Runtime component writes before `ConfigureFromServices` land in this boot.
   - Build the NATS client and its credentials yourself.
   - Know the teardown order.
   - That is about thirteen facts, far over two: a design finding.
2. Doing nothing:
   - streams: a boot error after about 49 s (computed); health publishes fail at Debug
   - registry: a boot error; own payload type unregistered: per-message poison, logged
   - ports: 8080 and 9090 on every interface
   - restart: a typed error
   - stall: silent lag
   - outside producer uses the stem: per-entity `authority_foreign`
   - in-process `deps.Platform` from the file: no error, entities carry the stem (silent)
   - no version bump: file edits silently ignored
   - vocabulary late or missing: name and text lookups silently miss product names
   - unregistered factory: a boot error naming it
3. Found out:
   - boot error: items 1 (consumers), 2 (registry, hierarchy type), 3 (clash), F5
   - typed runtime error: items 4, 6 (outside producer)
   - log only: item 1 (health, Debug), item 2 (type), item 7 (Debug)
   - doc or nowhere: item 3 (every interface), item 5, F1, F2, F3, F6 teardown
4. Should know: ideally only the configuration and the registries a product owns. The gap between 1 and 4 is the
   design work. The "prefer observation" question applies to items 3 and 6 and to F2: the bound port, the minted ID
   and the effective configuration are values the framework holds.

Intent check: not triggered (no boundary moves).

## Open evidence questions

- E1: answered in item 1 (graph-ingest and objectstore wait, then fail Start; health publishes log at Debug).
- E2: across the F2 class, which steps can diverge in practice when the file and the effective configuration differ
  (streams for a bucket-won boot's components, services, platform). Traced from code, not run.
- E3: whether the about-49 s wait holds as computed (not run).
- E4: whether any SemSource or other consumer process produces entities from outside the SemEngine process (not found
  in the files read).
