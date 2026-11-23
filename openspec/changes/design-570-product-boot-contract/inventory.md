# Inventory: what a product's own main must know to boot SemEngine (#570)

base: 3987dc117c4aefd0087f6720a7234196cc97ea18 (the claim commit; its tree equals origin/main 8d21b346)
SemSource read: C360Studio/semsource origin/main e6c39b1b26f5ae908cd88d0e8d0f1f7e5e20456f, `cmd/semsource/` and
`config/config.go` only (boot path, read-only, `docs/inventory-scope.md` row "semsource": dependency closure and
imports). Track: `design` (the change may touch more than one capability: `service-lifecycle`, `config-manager`,
`composed-stack`).

Sources the issue names, re-opened on this base:
`openspec/changes/archive/2026-11-09-test-557-composed-stack-smoke/inventory.md:137-149` (adopter seam inventory),
`notes/2.1.md` (minted ID), `notes/2.2.md` (bucket wins over file), `notes/probe.md` (watch across a broker restart),
and `inventory.md:109-136` (seams, probe summary).

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
- Do-nothing path, traced from code, not run: graph-ingest's `Start` calls `setupSubscriptions`
  (`processor/graph-ingest/component.go:466` — `if err := c.setupSubscriptions(ctx, submitCtx); err != nil {`), which
  waits for the input stream (`processor/graph-ingest/consume.go:85` — `created, err := c.waitForStream(ctx,
  streamName)`): 30 polls from 100 ms, x1.5, capped at 2 s (`:192` — `maxInterval := 2 * time.Second`), about 49 s by
  arithmetic on those constants. It then fails (`:218` — `return time.Time{}, fmt.Errorf("stream %s not available
  after %d retries", streamName, maxRetries)`), and `StartAll` returns the wrapped error. Found out: boot error, after
  about 49 s (computed, not measured). An ended `ctx` ends it sooner (`consume.go` select on `ctx.Done()`). What other
  components do without their streams: not traced (open question E1).
- Where the docs say it: `config/README.md:301` (what `EnsureStreams` does) and `service/README.md:750` (the METRICS
  stream). The boot example in `service/doc.go:68-97` and the `service/README.md` "Composing and starting" section
  (`:82`) do not call it. `docs/contract.md:191` — `// Build the service and component managers from these registries
  and start them.`
- New ordering fact: SemSource calls it on the FILE configuration, before `config.Manager.Start`: semsource
  `cmd/semsource/run.go:145` — `nc, ssCfg, err := setupNATS(ctx, flags.natsURL, semsourceCfg, logger,` -> `:261` —
  `if err := streamsManager.EnsureStreams(ctx, ssCfg); err != nil {`, then `:162` — `configMgr, err :=
  startConfigManager(ctx, ssCfg, nc, logger)`. The smoke test calls it on the effective configuration after `Start`.
  Whether streams ensured from the file can differ from the components a bucket-won boot runs (item 7): not measured
  (open question E2). No doc names the order.
- SemSource meets it: yes (`run.go:261`).

### Item 2: build the payload registry and register its types

- `internal/smoke/stack_integration_test.go:287` — `if err := message.RegisterPayloads(payloads); err != nil {`
- `docs/contract.md:146-191`: the "What is your job as a consumer" sketch shows `payloadregistry.New()` and the
  `RegisterPayloads` loop. `:152` — `it compiles there), and SemEngine's are fixed when each package is copied over.`
  (stale: SemEngine's packages are copied now).
- AGENTS.md rule (verify+review): "imports register nothing, only `main` aggregates". Registration by `main` is
  intended.
- Do-nothing path, no registry: `processor/graph-ingest/component.go:272` — `return nil,
  errs.WrapInvalid(errors.New("payload registry is required"), "CreateGraphIngest", "factory", "PayloadRegistry
  required")` gives a boot error. The component manager itself does not refuse a nil registry
  (`service/component_manager.go:179-185` checks only `Manager` and `ComponentRegistry`).
- Do-nothing path, registry without the product's own type: each such input is a poison message —
  `processor/graph-ingest/ingest.go:23` — `// every error it returns is a poison message there (counted, logged and`
  (terminated). Found out: log line plus counter, per message, at runtime. The mutation RPC refuses with code
  `message_type_unregistered` (`processor/graph-ingest/canonical_mutations.go:146-148`): a typed runtime error.
- SemSource meets it: yes. semsource `cmd/semsource/run.go:332` — `func buildPayloadRegistry()
  (*payloadregistry.Registry, error) {` registers five packages' payloads.

### Item 3: `http_port` and `metrics` default to fixed ports on every address

- `service/service_manager.go:165` — `if cfg.HTTPPort == 0 {` / `:166` — `cfg.HTTPPort = 8080` (unchanged since
  2026-11-09)
- `service/service_manager_http.go:75` — `listener, err := net.Listen("tcp", ":"+strconv.Itoa(cfg.HTTPPort))` (every
  address; `ManagerConfig` has no host key: `service/service_manager_config.go:10-27`)
- `service/service_configs.go:90` — `materializeServiceConfig(resolved, "metrics",
  json.RawMessage(`{"path":"/metrics","port":9090}`))` (metrics is on unless the configuration sets it to
  `enabled: false`; `:88-89` say explicit false is kept)
- `service/metrics.go:78` — `cfg.Port = 9090`; `metric/handler.go:43` — `port = 9090` (port 0 means 9090 at both
  layers)
- `service/metrics.go:46` — `testListener net.Listener` (a test-only field; open issue #614)
- The bound address: `metric/handler.go:298` — `func (s *Server) Address() string {` reports the bound listener's
  address, but the service `Manager` exposes no reader for either listener (its exported methods,
  `service/service_manager.go:150,266,306,322,342,674`, `service/service_manager_http.go:44,127`, include none).
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
- ADR-095 (one-shot running lifecycle) decides one Start per owner. ADR-113 (settled; not re-opened) governs Start and
  Stop contexts.
- Do-nothing path: a second `StartAll` returns `errs.ErrAlreadyStarted`: a typed runtime error at the call.
- SemSource meets it: no. It starts once and exits (semsource `cmd/semsource/run.go:202` — `if err :=
  manager.StartAll(signalCtx); err != nil {`, then `StopAll` and return).

### Item 5: the client must reconnect; a watch stalls about 20 s after a restart

- `natsclient/client.go:311` — `maxReconnects:   -1, // infinite by default` / `:312` — `reconnectWait:   2 *
  time.Second,` (the issue cited `:317`; moved)
- Timing evidence at the pinned version:
  `openspec/changes/archive/2026-11-09-test-557-composed-stack-smoke/notes/probe.md:3` — nats.go v1.55.0 (the version
  in `go.mod:18`), image `nats:2.15.0-alpine` (the image pin), a short outage (189 to 280 ms). Puts reached the watch
  19.53 s, 19.56 s and 19.46 s after the restart (`probe.md:18`). This is a program posted with source, command,
  versions and output: an ADR-112 probe at the pinned version, for a short outage only. No in-tree test asserts the
  stall's length. `TestSmoke_BrokerRestartUnderRunningStack` (`internal/smoke/smoke_integration_test.go:1094`) proves
  recovery, not timing. Any design text about the length must say it is nats.go's and not promised.
- `graph/follower/follower.go:23` — `//     timing (about 20 s measured at nats.go v1.54.0; TestFollowerDeclaresLostFeed
  deletes the` (names v1.54.0; the tree pins v1.55.0)
- `openspec/specs/composed-stack/spec.md:85-99`: a running stack recovers from a broker restart. `:8` still says a
  longer outage is not covered, but #574 was closed by PR #727 (`cd94648e`).
- Do-nothing path: the defaults reconnect forever. During the stall, readiness envelopes and answers lag; the follower
  reads behind until the watch resumes. Found out: nowhere but the doc comment above.
- SemSource meets it: sets the same values explicitly and adds an exit policy. semsource `cmd/semsource/run.go:478` —
  `natsclient.WithMaxReconnects(-1),`; `:484` — `natsclient.WithConnectionLossTimeout(connectionLossGrace),`
  (`natsclient/options.go:62`).

### Item 6: `platform.id` is a stem; the effective ID is minted at first boot

- `config/manager.go:103` — `const platformIdentityKVKey = "platform_identity"`; `:105-117`: the record
  `{org, stem, id}`. `:108` — `// under, so it carries exactly these three fields.` The type is unexported and there
  is no exported reader.
- ADR-104 already decides the outside-producer answer. `docs/adr/104-unique-platform-authority.md:138` — `- Adopter
  fixtures and e2e stop predicting the pair from a configuration file and read` (the record). `:66-70`: the opt-out
  is pre-creating the record with `id == stem`. `:178-184`: the cross-repo contract, "The record's shape is normative
  in the `component-runtime-config` capability spec". No spec of that name exists here (`ls openspec/specs`).
  `openspec/specs/config-manager/spec.md:75-120` states adopt, mint and refuse, but not the record's field shape.
- `config/README.md:214-220` documents the mint and adopt rules and the key name (`:217`). It does not give the
  record's JSON shape or tell a producer outside the process to read it.
- Do-nothing path: a producer that builds IDs from the declared stem gets every entity refused with
  `authority_foreign` (`notes/2.1.md:8-10`). Found out: a typed runtime refusal per entity, the first time it
  publishes.
- In-process `main`: reads `GetConfig().Get().Platform.ID` after `Start` (`config/README.md:195`; smoke
  `stack_integration_test.go:324`; semsource `run.go:398` — `platform := types.PlatformMeta{Org:
  effective.Platform.Org, Platform: effective.Platform.ID}`).
- SemSource meets it in-process only. Producers outside the process: none seen in the files read.

### Item 7: after the first boot the bucket, not the file, decides

- `config/manager.go:451` — `func (cm *Manager) arbitrate(ctx context.Context, kvStore *natsclient.KVStore, hasConfig
  bool) error {`; a newer file version pushes (`:476-484`); an equal version syncs from the bucket, logged at Debug
  only (`:492-496`); an older one logs a Warn.
- `config/README.md:222-242`: the "Which copy wins" table, with `:228` — `| The versions are equal, or the file's is
  older | The bucket wins |`, and `:241-242`: "raise the file's `version` to write it again". The rule is documented.
  The issue's "has no effect on later boots" holds only while `version` is unchanged.
- `openspec/specs/config-manager/spec.md:136` (Version arbitration requirement).
- Open issues on the same path: #646 (a newer-file push leaves keys the file dropped, and they return) and #647 (an
  accepted runtime change does not advance the bucket's version).
- Do-nothing path: a `main` that never sets or raises `version` (no `version`, or `0.0.0`) keeps the first boot's
  configuration forever; ports and components edited in the file are ignored. Found out: a Debug log line at equal
  versions; nowhere else.
- SemSource meets it in full. semsource `cmd/semsource/run.go:610` — `Version: fmt.Sprintf("%d.%d.0",
  compositionGeneration, cfg.ConfigRevision),`; `cmd/semsource/config_drift.go:28` — `func startConfigManager(...)`:
  drift warning (`:135`), a prune of retained components, and a `// shortcut:` tied to #646 (`:64`). The file is 257
  lines.

## 2. Every current spelling of the boot sequence

The fact being modelled is the ordered startup a product's `main` runs.

| Spelling | Where | Order it uses |
| --- | --- | --- |
| Smoke composition root | `internal/smoke/stack_integration_test.go:250-316` | client Connect -> `config.Manager.Start` -> `EnsureStreams(effective)` -> component registry -> payload registry -> `ConfigureFromServices` -> `StartAll` |
| `service` package doc | `service/doc.go:68-97` | registry -> `ConfigureFromServices` -> `StartAll` (no client, no config.Manager, no streams) |
| `service/README.md` | `:82` "Composing and starting" | as `doc.go`; no `EnsureStreams` (grep: only `:750`) |
| `config/README.md` | `:177-198` | config.Manager Start/Stop only; streams at `:301` |
| `docs/contract.md` | `:146-191` | registries only; ends at "start them" |
| ADR-058 | `docs/adr/058-boot-lifecycle-phases.md:91-130` | Phase A (connection, streams, registries) before `StartAll`, in "plain shared helper functions" |
| SemSource | semsource `cmd/semsource/run.go:95-103,145-190` | client -> `EnsureStreams(file)` -> config.Manager Start -> component registry -> payload registry -> service manager -> `StartAll` |

Searches: `git grep -n 'EnsureStreams' -- ':!*_test.go' ':!openspec/changes/archive'` gives `config/README.md`,
`config/doc.go`, `config/stream_bounds.go`, `config/streams.go`, `docs/admission-ledger.yaml`, `natsclient/stream.go`,
`openspec/specs/service-lifecycle/spec.md`, `service/README.md`. No exported boot helper:
`git grep -n -i 'func Boot\|func Run(' -- service config` found none at package level. `ls cmd examples` shows
neither exists.

## 3. Adjacent claims

- Open PRs (`gh pr list --state open`, 2026-11-23): #739 (this claim, 0 files), #696, #697, #698 (Dependabot,
  `go.mod`/`go.sum`). No file or capability overlap. #696 moves the nats-server Go library (2.15.2 -> 2.15.3), not the
  image the item 5 probe used.
- Open issues on `service`, `config` or the boot path (searches: `service`, `config`, `boot`, `EnsureStreams`,
  `http_port`, `platform identity`, `minted`, `reconnect`, `StartAll`):
  - #614 metrics test-only listener / port 0 (item 3, direct)
  - #646 newer-file push leaves dropped keys (item 7, direct)
  - #647 a runtime change does not advance the version (item 7, direct)
  - #740 warm restart: ENTITY_STATES watchers replay on one connection and trip slow-consumer at boot (boot path and
    reconnect, adjacent; not one of the seven; filed today, no milestone)
  - #625 flake-like failure in `TestConfigManagerStartupVersionArbitration` (item 7 test, adjacent)
  - #196 lifecycleguard adoption (item 4, adjacent; settled shape)
  - #361 CLOSED (alpha.8), noted only
  - The rest name the word, not the boot path: #9 #11 #24 #25 #27 #29 #36 #68 #84 #108 #109 #110 #134 #195 #300 #350
    #477 #484 #485 #502 #570 #630 #641 #665 #682 #702 #703 #712 #722.
- Specs: `service-lifecycle`, `config-manager` (`:75-160`), `composed-stack` (`:8` stale, `:85-99`),
  `component-registration`.
- ADRs: ADR-058 (Phase A in the composition root), ADR-094 (boot-only composition), ADR-095 (one-shot lifecycle, item
  4), ADR-104 (minted ID, record read contract, item 6), ADR-112 (natsclient promises; item 5 timing), ADR-113
  (settled), ADR-115 (HTTP surfaces, item 3).
- Owner rules in force: plain words; no port history in prose (#606); no SemStreams state migration; natsclient never
  reinvents nats.go (#170, ADR-112).

## 4. Who it is for

Not applicable at inventory: this phase introduces no symbol. The named consumer is SemSource (pins alpha.7). The
in-tree product-shaped caller is `internal/smoke`.

## 5. Problem shape and closest instances

Shapes: (a) ordered composition-root wiring, and (b) observe instead of predict, for the bound port and the minted ID.

- (a) Closest instance: ADR-058's Phase A in plain helper functions. No such helper exists in SemEngine today
  (section 2 searches).
- (b) Closest instances: `metric/handler.go:298` `Address()` (reports the bound address); the `platform_identity`
  record (ADR-104 "the framework observes; nobody computes"); `config.Manager.GetConfig().Get().Platform.ID` after
  `Start`.

## 6. Pin decision records

This change carries no package from the pin. The ADRs that govern the surfaces it reads are listed in section 3.

## Same-class collision table (boot ordering: runtime coordination)

| Dimension | Evidence |
| --- | --- |
| Semantic class | The order and inputs of a process's startup |
| Owners | each product `main`; `internal/smoke` runStack; ADR-058 assigns it to the composition root |
| Catalogs | `component.Registry`, `payloadregistry.Registry`, `service.NewServiceRegistry` + `RegisterAll` |
| Status | `/readyz` (`docs/contract.md:132`); `service/service_manager.go:60-61` stop flag |
| Lifecycle | one-shot `config.Manager.Start`, `StartAll`/`StopAll` (ADR-095, ADR-113) |
| Ownership | config bucket keyed by declared pair (`config/README.md:173-175`); one writer assumed (semsource `config_drift.go:61-63`) |
| Readers | smoke test, SemSource |
| Writers | `config.Manager.pushToKV`, `EnsureStreams` |
| Recovery | restart = new process (item 4); version bump to re-push (item 7) |

## Adopter seam inventory (a developer writing a product `main`)

1. Must know: the seven items, plus one more: call `EnsureStreams` on the effective configuration after
   `config.Manager.Start` (the smoke test's order; SemSource does the opposite). Eight facts is far over two, so this
   is a design finding.
2. Doing nothing:
   - streams: a boot error after about 49 s (computed)
   - registry: a boot error
   - own payload type unregistered: per-message poison, logged
   - ports: 8080 and 9090 on every interface
   - restart: a typed error
   - stall: silent lag
   - stem used as ID: per-entity `authority_foreign`
   - no version bump: file edits silently ignored
3. Found out:
   - boot error: items 1, 2 (registry), 3 (clash)
   - typed runtime error: items 4, 6
   - log only: item 2 (type), item 7 (Debug)
   - doc or nowhere: item 3 (every interface), item 5, the stream order
4. Should know: ideally only the configuration and the registries a product owns. The gap between 1 and 4 is the
   design work, and the "prefer observation" question applies to items 3 and 6 (the bound port and the minted ID are
   values the framework holds).

Intent check: not triggered (no boundary moves).

## Open evidence questions

- E1: which other tier-0 components fail, and how, when their streams are missing (only graph-ingest traced).
- E2: whether `EnsureStreams` on the file configuration can miss a stream a bucket-won boot's components need.
- E3: whether the about-49 s wait holds as computed (not run).
- E4: whether any SemSource or other consumer process produces entities from outside the SemEngine process (not found
  in the files read).
