# Inventory: setup-04a-03-boot-service

base: `30ae761f93b5d3e08049e812843763a5b8e865c4` (`main` `3a8611f` plus the two claim commits, which add only
`proposal.md`; draft PR #343)

Status: INVENTORY PASS (semengine-reviewer, round 2, `ad946b7`).

Question: what porting `service` from the SemStreams pin `8b99efe9` carries, how it splits into one pull request per
lifecycle owner and in what order, and what the owner must rule on before the first owner's design. This is an
inventory; the split in § 2.3 is a proposal for the owner to accept on #9, not a design.

Owner intent, verbatim. `AGENTS.md` § What this is for: SemEngine is "a live semantic knowledge-graph framework with
durable execution"; "TrustGraph and Temporal had a tiny baby — a pragmatic, Go-idiomatic, NATS-based, offline-first and
edge-capable baby." 2026-10-01: live semantic knowledge graph framework; rules are key; restart safety and idiomatic
Go are key; the shrink must raise quality and DX. 2026-10-07: clean idiomatic pragmatic Go, simple-yet-detailed over
complex. 2026-10-19 (PR #320): "let's fix them so we do not create debt on simple fixes as we port".

Repositories read (`docs/inventory-scope.md`): this tree; the pin through `gh api .../semstreams/tarball/8b99efe9`
(scratch copy, local only), its `docs/adr` included; the default branches of semsource `e4febc0d`, semconnect
`d0d06e00`, semteams `512b4bf2` and semboids `8c03cc53` through `gh api .../tarball/<sha>` (local only), for
symbol-level use of `service`. Tools (local only, in neither tree): a `golang.org/x/tools/go/packages` program that
lists, per pin `service` file, the names it uses from other `service` files ("file edges" below); a second that lists
every use of an exported `service` name in pin packages outside `service`; and a build of the pin's non-test `service`
files inside a scratch copy of this tree ("scratch build").

## 1. Admission

- D2 row 3, `openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md:147`: change `setup-04a-03-boot`,
  7 packages, 20,123 lines, 10 owners. Package list `:165-166`: "closure(service) − changes 1–2. Owners:
  `config.Manager` and the nine ported `service` owners." Owner count,
  `openspec/changes/archive/2026-10-01-setup-04a-foundation/inventory.md:441-443`: "ten in `service`",
  `MilestoneService` "[not ported]".
- No ledger row yet: `git grep -n 'source_path: service$' docs/admission-ledger.yaml` finds none.
- Excluded or changed by earlier rulings:
  - `service/milestone_service.go` (237 lines) is not ported: 03B `design.md:211`, #30. It is the only
    `service` → `agentic/agentrun` edge (pin `milestone_service.go:10`).
  - `service/rule_pack_bind.go` is ported: 03B `design.md:212` withdrew its non-port.
  - Log forwarder, ruling (b), #9 comment 6272104271: `LogForwarder` is ported and says it forwards nothing yet;
    `pkg/logging` and the boot wiring stay out; the same pull request decides the mandatory `flow-service.websocket`
    exclusion (`internal/logforwarderpolicy/policy.go:14` — `const mandatoryWebSocketExclusion =
    "flow-service.websocket"`).
  - `internal/componentadmission`: ledger `:2398` destination `none` (ruling C, drop the access token).
  - `component.Dependencies.ToolRegistry`/`ToolRegistryReader` and `LifecycleManager` are already dropped here
    (ledger `component` row `:2260-2261`, #29, #102 ruling D); `service.Dependencies` carries both at the pin
    (`dependencies.go:34`, `:36`).
- Seams 03B fixed for this port: SS#1218 one `ErrAlreadyStopped` (03B `design.md:488`), SS#1220 the registry
  after a failed start (`:489`); operator surface rows `:722-728`, each "served as at the pin" except the flowgraph
  projection ("a tested contract").

## 2. Lifecycle owners and the proposed split

### 2.1 Owners (definition: `setup-04a-foundation/design.md:84-97`)

Re-derived on the pin copy: every type in `service` with a `go` statement or acquired resource and an ending method
(`grep -n '^func (' ... '\) (Start|Stop|Close|Shutdown|Drain|Run)\('` plus the `go`/`NewTicker`/`net.Listen`/
`Subscribe`/`Watch` sweep). Ten types; nine ported. Line counts are `wc -l` of the owner's own files.

| Owner | What `Start` leaves running (pin) | Ended by | Own files, non-test lines |
| --- | --- | --- | --- |
| `BaseService` | `base.go:273-274` — `s.healthTicker = time.NewTicker(s.healthInterval)` / `go s.healthMonitor(runtimeCtx, &s.wg)`; `:280` — `go s.contextMonitor(runtimeCtx, &s.wg)`; join goroutine `:281` | `Stop` `:299`, or the Start context ending (`:459` — `<-ctx.Done()`) | `base.go` 495 |
| `Manager` | `StartAll` `service_manager.go:390`: listener `:1176` — `listener, err := net.Listen("tcp", ":"+strconv.Itoa(m.config.HTTPPort))`, serve goroutine `:1204`; health publisher `:707` with ticker `:682`; Metrics' server through `startStartupMetricsServer` `:631`; `StartHealthListener` `:1280` binds a second listener | `StopAll` `:838`, `StopHealthListener` `:1355`, `Stop` `:1054` | `service_manager.go` 1,958, `service_manager_config.go` 82, `startup_metrics.go` 104, `graph_triples_http.go` 250 |
| `ComponentManager` | every component's `Start` (per-component cancel `component_manager.go:622`), `BaseService` `:493`, supervisor `:501` — `go cm.supervise(runtimeCtx, supervisorDone)` running the health loop (ticker `:1478`) and the override-expiry reporter `:547` | `Stop` `:774` | `component_manager.go` 1,526, `component_manager_config.go` 26, `component_manager_http.go` 673, `stream_override_expiry.go` 130, `rule_pack_bind.go` 132 |
| `Metrics` | `metric.Server` under `runCtx` (`metrics.go:134` — `runCtx, cancel := context.WithCancel(ctx)`), or the Manager claims it first (`:307` — `func (m *Metrics) claimManagerServer() (*metric.Server, error) {`) | `Stop` `:202` | `metrics.go` 382 |
| `MessageLogger` | NATS subscriptions under `runCtx` `message_logger.go:714`; retry goroutine `:801`; per-request SSE watch goroutine `message_logger_kv_watch.go:230` | `Stop` `message_logger.go:808` | 1,234 + `message_logger_http.go` 614 + `message_logger_kv_watch.go` 397 = 2,245 |
| `StorageObservabilityService` | `BaseService`, collector and consumer loops under `runCtx` (`storage_observability.go:287`, goroutines `:294`, `:298`), join `:304` | `Stop` `:315` | 663 + `_http.go` 254 + `_metrics.go` 623 = 1,540 |
| `MetricsForwarder` | `BaseService`, ticker `metrics_forwarder.go:216`, `:218` — `go mf.publishLoop(ctx)`, join `:221` | `Stop` `:235` | 471 |
| `HeartbeatService` | `BaseService`, ticker `heartbeat.go:166`, `:168` — `go hb.heartbeatLoop(ctx)`, join `:171` | `Stop` `:185` | 280 |
| `LogForwarder` | `BaseService` only (`log_forwarder.go:96-110`) | `Stop` `:112` | 124 |
| `MilestoneService` (not ported, #30) | — | — | 237 |

Not an owner (no ending method) but background work: `pprof.go:28` — `func MaybeStartPProf(debug bool, port int) {`,
whose `:38` — `go func() {` serves until the process exits (finding S4).

Package total: 11,819 non-test lines in 34 files; 11,582 without `milestone_service.go`. Tests: 65 files, 18,653
lines; 18,255 without the two milestone test files. Shared files with no owner: `doc.go` 307, `service_configs.go` 192,
`schema.go` 160, `openapi_types.go` 119, `registry.go` 72, `pprof.go` 52, `configurable.go` 50, `dependencies.go` 43,
`middleware.go` 42, `openapi_registry.go` 40, `graph_runtime.go` 35, `register.go` 27, `openapi.go` 25; `README.md`
669 lines.

### 2.2 How the files depend on each other at the pin (measured)

- Every constructor has the `Constructor` signature (`dependencies.go:43`), whose `*Dependencies` holds
  `dependencies.go:37` — `ServiceManager    *Manager`. So every owner file needs `service_manager.go` to compile.
- The `Manager` names the component manager (four type checks) and the metrics service by concrete type:
  `service_manager.go:593` — `componentManager, _ := m.services["component-manager"].(*ComponentManager)`; `:594` —
  `metricsService, _ := m.services["metrics"].(*Metrics)`; `:1142` — `if cm, ok := service.(*ComponentManager); ok {`;
  `:1543` — `cm, ok := cmService.(*ComponentManager)`; `:1812` — `componentManager, concreteComponentManager :=
  componentManagerService.(*ComponentManager)`; `:82` — `startupMetricsService   *Metrics`; and `:1693` — `case
  StorageObservabilityServiceName:`. The component manager uses `startupUnitCounts` (`service_manager.go:109`) and
  `attributeShutdownError` from the Manager's file. Scratch build of the Manager's files plus the shared files without
  the owners: 8 errors, all `undefined: ComponentManager`, `undefined: Metrics` or `undefined:
  StorageObservabilityServiceName`.
- So at the pin the `Manager`, the `ComponentManager` and `Metrics` compile only together: 5,447 non-test lines with
  the files they need (§ 2.3, option A).
- At run time the component manager is added by default (`service_configs.go:83`) and the Manager refuses a
  composition that disables it: `service_manager.go:164-166` —
  `for _, name := range []string{"service-manager", "component-manager"} {` / `if !resolved[name].Enabled {`; and
  `HeartbeatService.Start` refuses without one: `heartbeat.go:147` — `componentService, exists :=
  hb.serviceManager.GetService("component-manager")`.
- Tests cross owners through two helpers. Five files that test the Manager or the component manager
  (`startup_observability_test.go` 849, `startup_observability_amendment_test.go` 515, `stream_override_expiry_test.go`
  435, `service_manager_health_listener_test.go` 336, `pprof_test.go` 73; 2,208 lines) reach `requireGauge` and
  `gaugeValue` in `storage_observability_test.go`, so as written they land with the storage service. The two census
  tests (`message_logger_census_test.go` 556, `stream_planning_census_test.go` 278) import
  `cmd/e2e-semstreams/mission`, `componentregistry`, `examples/processors/*` and `frameworkcapabilities/graphresearch`,
  none in the port set; they cannot be carried as written.

### 2.3 Proposed split (option B) and merge order

One pull request per owner, with two stated groups. Test lines: each test file goes to the first pull request in which
every production file it references exists (file edges; the four external `service_test` files placed by hand by the
`service.` names they call). Not in tree = imported and absent from this tree and from `docs/admission-ledger.yaml`.

| PR | Owners | Files | Non-test | Test | New imports not in the tree | Waits for |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `BaseService` | `base.go`; shared, read by two or more owners: `service_configs.go`, `configurable.go`, `openapi.go`, `openapi_registry.go`, `openapi_types.go`, `schema.go`; composition-root helpers with no owner: `pprof.go`, `graph_runtime.go`; `doc.go`, `README.md` (only what has landed) | 1,475 | 1,157 | none (renames § 2.4) | — |
| 2 | `Manager` + `Metrics` | `service_manager.go`, `service_manager_config.go`, `startup_metrics.go`, `graph_triples_http.go`, `middleware.go`, `dependencies.go`, `registry.go`, `register.go` (only landed constructors), `metrics.go` | 2,960 | 2,855 | none | PR 1 |
| 3 | `ComponentManager` | `component_manager.go`, `component_manager_config.go`, `component_manager_http.go`, `stream_override_expiry.go`, `rule_pack_bind.go` | 2,487 | 4,563 | `composition` (#342); `projection.PredicateReconciler` (Q2) | PR 2, #342 |
| 4 | `MessageLogger` | `message_logger.go`, `message_logger_http.go`, `message_logger_kv_watch.go` | 2,245 | 2,613 | none | PR 2 |
| 5 | `StorageObservabilityService` | `storage_observability.go`, `_http.go`, `_metrics.go` | 1,540 | 4,261 | none (`natsclient/storagereport`); its tests use four names that package has since unexported (`UnknownCapacity`, `UnknownGrowth`, `DefaultWarningHeadroom`, `DefaultCriticalHorizon`: 10 uses in 3 test files) | PR 2 |
| 6 | `MetricsForwarder` + `HeartbeatService` | `metrics_forwarder.go`, `heartbeat.go` | 751 | 1,702 | none | PR 2 to build; PR 3 by choice |
| 7 | `LogForwarder` | `log_forwarder.go`; ruling (b) and the `flow-service.websocket` decision | 124 | 270 | none | PR 2 |

Sums: 11,582 non-test; 17,421 test plus the 834 census lines that cannot be carried. Edges: PR 2 after PR 1 because
`Manager` embeds `*BaseService` (`service_manager.go:32`); every later PR after PR 2 because of `dependencies.go:37`;
PR 3 after #342 because `component_manager.go` imports `composition`. PR 6 builds after PR 2; waiting for PR 3 is an
ordering choice, not a need: `HeartbeatService.Start` refuses without a component-manager service (`heartbeat.go:147`)
but asserts only an interface, `heartbeat.go:151` — `componentManager, ok :=
componentService.(componentHealthGetter)`, so its tests can compose a stand-in, as the § 7 probe did. PRs 4, 5 and 7
are independent of each other and of PR 3. As measured, PR 5's tests include the five Manager and component-manager
test files above; moving `requireGauge` and `gaugeValue` (two helpers) lets them land in PR 2 and PR 3.

- Group reasons. `Metrics` rides with the `Manager` because the Manager drives its server: `service_manager.go:620` —
  `server, err := metricsService.claimManagerServer()`, `:639` — `if err := service.startServer(ctx, server); err !=
  nil {`, `:642` — `service.setManagerServerHealthy(true)`. `MetricsForwarder` and `HeartbeatService` (751 lines
  together) have one shape: `BaseService`, a ticker loop and a join goroutine, both substrate observability (ADR-098);
  splitting them adds a design and a review round for 280 lines.
- What option B needs that the pin does not have, and what it costs:
  - PR 2 cannot name `*ComponentManager` (`*Metrics` is in it), and `StorageObservabilityServiceName`
    (`service_manager.go:1693`, `:1697`; declared `storage_observability.go:72`) moves to a shared file.
  - The four `*ComponentManager` type checks above become one small unexported interface the component manager
    satisfies (`startupSnapshot`, `startupStopBegun`, `setStartupMetricWriter`, `registerStartupHTTPHandlers`,
    `withComponents`, `GetComponentHealth`). It has one implementation: sweep shape 3 (S10), introduced by the split,
    not inherited from the pin.
  - PR 2 (`Manager` + `Metrics`) and PR 6 (`MetricsForwarder` + `HeartbeatService`) each carry two owners, against
    one owner per pull request (`AGENTS.md` § 4, #159), so each needs the owner's waiver. PR 6 can split in two:
    neither file uses a name from the other (file edges).
  - Until PR 3 lands, PR 2's tests compose a stand-in `component-manager` service, as the pin's own tests do
    (`service_manager_test.go:201` — `MockService: MockService{name: "component-manager", status: StatusRunning,
    healthy: true},`).
- Option A, the pin's structure: PRs 2 and 3 as one pull request holding three owners, 5,447 non-test and 7,418 test
  lines, after #342, with the owner's waiver (#159). It still moves `StorageObservabilityServiceName` to a shared file
  (`storage_observability.go` lands in PR 5); every other row is unchanged.
- Shared files: `register.go`'s `RegisterAll` (`register.go:8`) gains one constructor per PR, and the pin's seven are
  all present after the last. `doc.go` and `README.md` grow per PR; the pin README is stale (§ 5 (c)).

### 2.4 Imports and names (scratch build)

Every `service` import is in the tree or is named above. Scratch build of the 33 ported files, with the module path
rewritten, `internal/lifecyclecleanup` → `pkg/lifecyclecleanup` (ledger `:1898`), and `composition` and
`internal/componentadmission` copied from the pin (`composition` needs four scratch edits to build against this
tree's `internal/flowgraph` and `types`): `go build -gcflags=-e ./service/` lists these differences, all from rulings
or simplification PRs already merged here:

| Pin name used by `service` | Here | Used at (pin) |
| --- | --- | --- |
| `metric.MetricsRegistrar`, `RegisterGauge`, `RegisterGaugeVec` | removed for `metric.RegisterOrGet` (ledger `:732`) | `base.go:389`, `:494`; `storage_observability_metrics.go:330`, `:334`; `stream_override_expiry.go:79` |
| `component.ToolRegistryReader`, `Dependencies.ToolRegistry`, `.LifecycleManager` | dropped (#29, #102 ruling D) | `dependencies.go:34`, `:36`; `component_manager.go:47`, `:1216`, `:1218` |
| access token on `CreateComponent`, `SealComposition`, `Snapshots` | dropped (ruling C) | `component_manager.go:290`, `:384`, `:397`, `:1107`; `message_logger.go:347` |
| `Snapshots` result, `Discoverable.Meta` | `[]Declaration`; `Meta` removed (simplify-04a-09, ledger `:2268-2271`) | `component_manager.go:400`, `:697`; `message_logger.go:347-349` (#280 item 2) |
| `component.PlatformMeta` | `types.PlatformMeta` | `component_manager.go:1210` |
| `graph.EnsureCatalogBucket`, `OpenCatalogReader`, `CatalogReader`, `OwnerOf`, `EnsureCatalogRetentionClean` | `graph/kvcatalog` | `storage_observability.go:397`, `:223`; `graph_triples_http.go:181`, `:224`; `graph_runtime.go:25` |
| `graph.UnmarshalEntityState` (2 arguments) | 3 arguments | `graph_triples_http.go:207` |
| `natsclient.Status.FailureCount` | removed with the circuit breaker (simplify-04a-16) | `service_manager.go:1752` |
| `natsclient.Storage*`, `Pressure*`, `Capacity*` and others (40 names), `Client.AccountStreamLister` | `natsclient/storagereport`; 7 of the 40 under its nine renames, `AccountStreamLister` a function (ledger `:840-846`) | `storage_observability*.go` |
| `projection.PredicateReconciler` | removed as unread (#138 item 1, closed) | `rule_pack_bind.go:16` (Q2) |

Other second spellings the port meets: metric namespace `"semstreams"` (`startup_metrics.go:43`,
`storage_observability_metrics.go:196`, `:291`, `stream_override_expiry.go:67`; here `metric/core.go:32` —
`Namespace: "semengine",`), and the OpenAPI defaults `"SemStreams API"` and `"0.7.0"` (`service_manager.go:185`, `:191`).

### 2.5 HTTP routes `service` registers at the pin

- Main listener (`http_port`, default 8080), before boot commits: `/health`, `/healthz`, `/readyz`, `/services`,
  `/services/health`, `/components/health`, `/components/list`, `/components/status/`, and `/` answering 503
  `NOT READY` (`service_manager.go:1140-1149`).
- Main listener after boot: `/health`, `/healthz`, `/readyz`, `/services`, `/services/health` (`:1707-1713`);
  `/graph/triples` (`:1720`); `/openapi.json` (`:1568`); `/docs` when `swagger_ui` (`:1572`).
- `/graph/triples` readers (measured, `grep -rn '/graph/triples'`): semteams 110 lines, 15 of them in `ui/src` across
  7 production files (21 in 10 with its `.test.ts` files; `ui/src/lib/services/runStatusApi.ts:21` —
  `const TRIPLES_ENDPOINT = "/graph/triples";`, run-level pause detection; `controlsStore.svelte.ts:9`,
  `+layout.svelte:20`), the rest its e2e specs, Caddyfile, Taskfile and docs;
  semsource one doc line; semconnect and semboids none. At the pin outside `service`: no production Go, only the e2e
  ops scenario (`test/e2e/scenarios/ops/scenario.go`), its compose and task files, two flow configs and docs. The 03B
  operator-surface table (03B `design.md:722-728`) lists what is kept, not what is dropped. So it is framework API:
  it ports as at the pin, with a contract line in PR 2.
- Services, mounted at `"/" + serviceNameToPrefix(name)` (`:1517-1520`, prefixes `:1687-1700`):
  `/components/{health,list,types/,types,status/,config/,flowgraph,validate,paths}`
  (`component_manager_http.go:68-78`); `/message-logger/{entries,stats,subjects}`,
  `GET /message-logger/trace/{traceID}`, `/message-logger/kv/`, `GET /message-logger/kv/{bucket}/watch`
  (`message_logger_http.go:34-45`); `/storage-observability/report` (`storage_observability_http.go:133`).
- Components: `/<component name>/...` for any component with `RegisterHTTPHandlers` (`service_manager.go:1550-1555`).
- Health listener (`StartHealthListener`): `/health`, `/healthz` (`:1306-1307`).
- Metrics service: `metric.Server` at `:9090/metrics` (`metrics.go:79-83`), which also serves `/health`
  (`metric/handler.go:123` here). pprof: `http.DefaultServeMux` on its port (`pprof.go:28`).

## 3. Shape sweep (pin paths)

Disposition is proposed; the design decides it under Bounded port scope.

- **S1, required (ADR-113; lifecycle suite): every run context descends from the Start context.** `base.go:254` —
  `runtimeCtx, runtimeCancel := context.WithCancel(ctx)`; `component_manager.go:464` — `runtimeCtx, cancel :=
  context.WithCancel(ctx)`; `:622` — `childCtx, cancel := context.WithCancel(ctx)`; `message_logger.go:714`;
  `metrics.go:134`; `storage_observability.go:287`; `service_manager.go:707` — `publisherCtx, cancel :=
  context.WithCancel(ctx)`; `:1181`, `:1308`; loops on the Start context itself: `heartbeat.go:168`,
  `metrics_forwarder.go:218`. The lifecycle-suite probe (§ 7): four owners fail `StartContextEndStopsNothing`.
- **S2, required (context ownership: a provider closure keeps a context).** `service_manager.go:1189-1190` —
  `BaseContext: func(_ net.Listener) context.Context {` / `return serverCtx`, and `:1314`. ADR-113 names the same shape
  in `metric.Server` (`docs/adr/113-start-context-bounds-start.md`, Consequences).
- **S3, required for two owners (ADR-108 Decision 2, #196): copied lifecycle state machines.**
  `component_manager.go:98-110` (`:102` — `lifecycleUsed     bool`) and `message_logger.go:234-239` adopt
  `internal/lifecycleguard`. The same shape, optional unless the owner schedules it: `base.go:110-114`,
  `metrics.go:29-38`, `service_manager.go:62-80` (three copies: HTTP, health listener, health publisher),
  `heartbeat.go:65-73`, `metrics_forwarder.go:139`, `storage_observability.go:167`.
- **S4, required design item (background-work spec; framework API): `MaybeStartPProf` starts work nothing stops or
  joins.** `pprof.go:28`, `:38`. Callers: pin `internal/boot/run.go:74`; semsource `cmd/semsource/run.go:109`,
  semteams `cmd/semteams/main.go:93`, semboids `cmd/semboids/main.go:100`, so it ports (#190) and is fixed, not left
  out. The spec "binds every package in the module and every later port" (`background-work/spec.md:15-22`); an HTTP
  server waits on in-flight requests, so the shape is `Run(ctx)` or `Shutdown(ctx)`, not `Close` (`:17-20`). Either
  changes the signature the three consumers call (Q3). Lands in PR 1.
- **S5, required (03B SS#1218): two `ErrAlreadyStopped`.** `base.go:28` — `var ErrAlreadyStopped = errors.New("service
  already stopped")`, beside `pkg/errs/errs.go:43`. No `service` code returns it; one reader, `service_manager.go:888`.
- **S6, shape 1, optional: two config decoders; three services accept unknown keys silently.** Strict
  `service_configs.go:130-139` (`decoder.DisallowUnknownFields()`); plain `heartbeat.go:84`, `metrics_forwarder.go:28`,
  `:37`, `storage_observability.go:178`. No binding rule makes it required: the unknown-key rule (`AGENTS.md` § 1;
  `component-registration` spec, "Unknown configuration keys are refused") names components, and the developer
  contract's fail-closed rule covers enum and grammar values, not keys. A simple fix (the strict decoder is in the
  same package), which the owner's 2026-10-19 intent favours; the design decides.
  - Letter case: the strict decoder also accepts a key in another letter case (probe, go1.26.4: `{"HTTP_Port":9}`
    under `DisallowUnknownFields` fills `http_port`, no error). #341's spec delta leaves "the contents of a
    component's or service's `config` payload" to its owner (`config-loading/spec.md:9-10` on
    `claude/config-key-case`), so letter-case refusal in `service`'s own decoders belongs to this port, if anywhere.
- **S7, shape 5: classification by error text.** `message_logger_http.go:443` — `if strings.Contains(err.Error(), "not
  found") {`; `:497` — `if strings.Contains(err.Error(), "no keys found") {`. Optional; a simple fix.
- **S8, required in part: health publishing drops failures without a word.** Two health loops,
  `component_manager.go:1476-1489` and `service_manager.go:680-693`, each a 5 s ticker. Each drops the publish
  error, `component_manager.go:1524` — `_ = cm.natsClient.PublishToStream(ctx, subject, data)`, and
  `service_manager.go:798`, and skips a marshal failure with a bare `continue` (`component_manager.go:1518-1520`,
  `service_manager.go:792-794`). `metrics_forwarder.go:361-386` logs both at Debug.
  - Required: "any skip, drop, or degrade is declared" (`AGENTS.md` § 1) is binding, so PRs 2 and 3 declare each
    drop (log or count it; the design picks). PR 6 already logs its drops at Debug; its design says whether that
    level is enough.
  - Not in scope: the `BaseMessage` rule names polymorphic payloads; these are one fixed JSON shape per subject
    (`health.component.<name>`, `health.service.<name>`, `metrics.<component>.<name>`), so the raw maps stay.
  - Optional (shape 2): two near-identical loops. Pin readers of `health.>`/`metrics.>`: none in Go outside `service`
    (`config/streams.go:126`, `:138` declare the streams).
- **S9, shape 4: test-only code in production structs.** `service_manager.go:84-88` (`testCommitPrepared` and three
  more); `metrics.go:40-44`, used at `:323-324` to swap the listener; test-only constructors `heartbeat.go:258`,
  `metrics_forwarder.go:430`; injected `listen` functions `pprof.go:32`, `service_manager.go:1284`. All unexported, so
  Bounded port scope (`AGENTS.md` § 4, #186, #190). `newHeartbeatServiceForTest` builds a service whose `Start` refuses
  ("heartbeat requires service manager", § 7).
- **S10, shape 3: one-implementation interfaces.** `dependencies.go:20` `natsPublisher`, with an always-true check
  `metrics_forwarder.go:74` — `publisher, ok := interface{}(deps.NATSClient).(natsPublisher)`; `metrics.go:47`
  `metricsServer`; `heartbeat.go:45` `componentHealthGetter`; `message_logger.go:166`, `:175`;
  `metrics_forwarder.go:19` `MetricsGatherer` (exported, no reader outside `service`). Optional. Option B adds one
  (§ 2.3). Not a finding: `message_logger_http.go:365` `kvBucketProvider`, whose comment gives a reason (read-only
  capability).
- **S11, shape 1: two lifecycle verb pairs on `Manager`.** `service_manager.go:1040` `Start`, `:1054` `Stop` (no
  production caller at the pin) beside `StartAll` `:390` and `StopAll` `:838`.
- **S12, shape 1: the component manager is found by name 17 times in 7 files** (`"component-manager"`: `service_manager.go`
  9, `component_manager.go` 3, one each in `component_manager_http.go`, `heartbeat.go`, `register.go`,
  `rule_pack_bind.go:32`, `service_configs.go:83`). Optional.
- **S13, shape 1: two registration paths on one registry.** `startup_metrics.go:50` — `if err :=
  registry.PrometheusRegistry().Register(units); err != nil {`, beside `RegisterOrGet` everywhere else after § 2.4.
  Optional; the `metric-registry` spec forbids only the global registry.
- **S14, context rules otherwise clean.** No `context.Context` field; no `context.Background`, `TODO` or
  `WithoutCancel` in non-test code (only `doc.go:192`, `:299`, comments); the three `WithTimeout` calls are
  request-scoped (`graph_triples_http.go:147`, `:225`, `:239`). No bare `select {}`.

## 4. ADR-113 premise: where the managers receive, keep and watch a context (pin)

- Receive: `Manager.StartAll(ctx)` `service_manager.go:390` hands the same context to every service, `:443` —
  `startErr := service.Start(ctx)`, and to the HTTP runtime (`:423`) and the health publisher (`:471`).
  `ComponentManager.Start` `component_manager.go:446`, `Stop` `:774`. Callers: pin `internal/boot/run.go:600` —
  `if err := manager.StartAll(runtimeCtx); err != nil {` with a process root that outlives `StopAll(shutdownCtx)`
  (`:643-645`); consumers § 6.
- Keep: no context field. Cancels: `component_manager.go:107` — `cancel            context.CancelFunc`; per component
  `:123`. Closure: `:463` — `parent := ctx`, used after `Start` by `:512` — `rollbackErr :=
  lifecyclecleanup.RollbackFailedStart(parent, cm.cleanupFailedStart)`.
- Derive: run context from the Start context `:464`; each component's `Start` context is a child of that run
  context, `:622`, `:636` — `launchCtx := childCtx`, then `:699` — `if err := lc.Start(ctx); err != nil {`.
- Watch: `BaseService.Start(runtimeCtx)` `:493` → `base.go:280` `contextMonitor`; `supervise` `:501` → `:1483` —
  `case <-ctx.Done():`; `stream_override_expiry.go:93`.
- Cancel orders ADR-113 removes: `:1033` — `if retryable && cancel != nil {`, `:1039` — `stopErr :=
  lifecycle.Stop(ctx)`, `:1040` — `if !retryable && cancel != nil {`.

## 5. Surface audit (pin-wide, non-test, plus the four consumers)

Callers are typed uses from the second tool, `grep` of method calls (interfaces), and consumer tarball greps.
Methods reached through `Service`, `HTTPHandler`, `error` or `json` interfaces count as read.

**(a) Exported with no reader** (pin outside and inside `service`, and the consumers): `base.go:166` `WithHealthCheck`,
`:173` `WithHealthInterval`, `:180` `OnHealthChange`, `:360` `BaseService.OnHealthChange`;
`component_manager.go:1139` `IsInitialized`, `:1144` `IsStarted`, `:1333` `GetHealthyComponents`, `:1345`
`GetUnhealthyComponents`; `component_manager_config.go:14` `DefaultComponentManagerConfig`; `service_manager.go:354`
`GetAllServices`, `:367` `ListConstructors`, `:379` `HasConstructor`, `:970` `GetHealthyServices`, `:988`
`GetUnhealthyServices`, `:1006` `GetServiceStatus`, `:1019` `GetAllServiceStatus`, `:1040` `Start`, `:1054` `Stop`,
`:1355` `StopHealthListener`; `registry.go:53` `Registry.Services`; `openapi_types.go:88` `NewOpenAPISpec`, `:97`
`AddPath`, `:102` `AddTag`; `openapi_registry.go:35` `GetOpenAPISpec`; `configurable.go:14` `Configurable` (no type
assertion anywhere). Unexported, no caller: `component_manager.go:1200` — `func (cm *ComponentManager)
shutdownCallback(ctx context.Context) error {`. Not settled by grep (names too common): `Metrics.Port`, `Path`, `URL`.

Kept with readers (framework API, contract line due in each design):

- pin `internal/boot`: `RegisterAll`, `NewServiceRegistry`, `NewServiceManager`, `Dependencies`,
  `ConfigureFromServices`, `StartAll`/`StopAll`/`StartHealthListener` (through `run.go:535`), `RegisterInstance`,
  `GetService`, `ComponentsImplementing`, `ConfigureRulePackMutations`, `WireGraphRuntime`, `MaybeStartPProf`;
  OpenAPI types: `cmd/openapi-generator`, `gateway/*` (consumer adapters), `processor/agentic-dispatch/http.go:36`.
- semsource `cmd/semsource/run.go`: `NewServiceRegistry` `:371`, `RegisterAll` `:372`, `NewServiceManager` `:376`,
  `Dependencies` `:377`, `ConfigureFromServices` `:391`, `StartAll` `:197`, `StopAll` `:222`, `MaybeStartPProf` `:109`.
- semteams `cmd/semteams/`: `main.go` `RegisterAll` `:715`, `UseHTTPMiddleware` `:249`, `ConfigureRulePackMutations`
  `:234`, `WireGraphRuntime` `:840`, `RegisterInstance` + `NewMilestoneService` `:858` (#30), `StartAll` `:916`;
  `rule_config_service.go` `BaseService` `:32`, `GetService` `:89`, `ComponentsImplementing` `:92`; `middleware.go:73`
  `HTTPMiddleware`.
- semboids (on `beta.160`): `cmd/semboids/main.go` `Registry.Register` `:405`, `StartAll(signalCtx)` `:457`,
  `StopAll(shutdownTimeout)` `:465`; `internal/api/service.go` `BaseService` `:72`, `WithLogger`, `WithMetrics`,
  `WithNATS` `:96-98`, `OpenAPISpec` and five spec types `:171-197`. semconnect: no `service` import.

**(b) Config read by no behaviour:** `component_manager_config.go:10` — `EnabledComponents []string` with key
`enabled_components` and schema text "List of component names to enable (empty=all)", only defaulted at
`component_manager.go:146-147`; `LogForwarderConfig.MinLevel`/`ExcludeSources` (`log_forwarder.go:47`, `:53`) reach
only a log line (`:105-107`), ruling (b) covers it. Unknown keys accepted: S6.

**(c) Documented, not implemented:** `configurable.go:13` — "The Discovery service exists to expose this information
via HTTP API." (no such service); `README.md:202-204` `RegisterConstructor` (removed, `service_manager.go:282`);
`README.md:328-345` a `discovery` service and service-manager keys `read_timeout`, `write_timeout`,
`shutdown_timeout`, which the strict decoder refuses (`service_manager.go:178`; the keys are `http_read_timeout`,
`http_write_timeout`, `service_manager_config.go:10-26`); `README.md:354-369` seven error sentinels, none in the code;
`component_manager_config.go:10` "empty=all", which nothing filters.

**`GenericJSON`:** none. `git grep`-equivalent on the pin copy for `GenericJSON` in `service`: 0 hits. Known shapes
sent as raw JSON maps instead: S8.

## 6. Adopter seam (products composing `service.Manager`)

1. **What they must know.** (i) The context given to `StartAll` must outlive `StopAll`: semsource wraps it,
   `run.go:196` — `runtimeCtx, cancelRuntime := context.WithCancel(context.WithoutCancel(signalCtx))`; semboids does
   not, `main.go:457` — `if err := manager.StartAll(signalCtx); err != nil {`. (ii) `component-manager` is added by
   default (`service_configs.go:83`) and may not be disabled (`service_manager.go:164-166`), so its constructor must
   be registered (`RegisterAll`); `metrics` is on by default at port 9090 (`service_configs.go:86`).
   (iii) `UseHTTPMiddleware` before `StartAll`; a late call is ignored with a Warn (`service_manager.go:1100`).
   (iv) The main port defaults to 8080 (`service_manager.go:181`).
2. **If they do nothing** with (i): SIGTERM cancels every service's run context before `StopAll` (S1, § 7), and
   `StopAll` then joins services already stopping.
3. **Where they find out:** a comment in their own `main` (semsource `run.go:216-218`); for (iii) a log line.
4. **What they should know:** call `StartAll` and `StopAll`. Under ADR-113 the Start context bounds start only; the
   gap is (i), and S1 is what closes it.

Intent check: no boundary moves. `service` is admitted (D2 row 3); the exclusions are ruled (#30; ruling (b)).

## 7. Pin probe

Scratch copy, go1.26.4 darwin/arm64, 2026-10-20, about 4 minutes of wall clock.

- **Unit, once:** `go test -race -count=1 -cpu 1 -p 1 ./service/` — `ok ... 7.030s` (33.6 s with build).
- **Unit, five times:** `go test -count=5 -cpu 1 -shuffle=on -v ./service/` — seed `1792511013689784000`, `ok ...
  20.797s`, 1,785 top-level runs (357 tests × 5), no failure, no skip.
- **Integration:** `go test -tags integration -race -count=1 -cpu 1 -p 1 ./service/` (testcontainers,
  `nats:2.14-alpine`) — `ok ... 25.138s`, 390 top-level tests, one skip:
  `TestConfigBootProcessRestartHelper` (the subprocess helper, `config_boot_process_restart_integration_test.go:106`).
- **Lifecycle suite.** The pin has no suite run for `service` owners (`component/lifecycle_test_suite.go` is for
  components). This repository's `internal/harness/lifecycletest` copied into the scratch pin, with an adapter whose
  `Observe` reports `status=<Status()>` while not stopped (it sees nothing else), run on `BaseService`, `LogForwarder`,
  `HeartbeatService` (Manager with a stand-in `component-manager`) and `MetricsForwarder` (stand-in publisher): each
  passes 7 of 8 checks and fails `StartContextEndStopsNothing` — "after the Start context ended the owner stopped
  holding [status=running]". Not run: `ComponentManager`, `Manager`, `Metrics`, `MessageLogger`,
  `StorageObservabilityService` (each needs NATS or a composed registry; time-boxed).
- **Test-text guards** (this tree's `internal/harness/contract` run over a scratch copy holding the pin's `service`):
  - `TestNoFixedAddressesInTests`: `message_logger_test.go:80` — `natsClient, err :=
    natsclient.NewClient("nats://localhost:4222")` (PR 4).
  - `TestNoSleepsInTests`: 22 lines. `base_test.go:36`, `:48`, `:103`, `:197`, `:284`, `suite_integration_test.go:122`,
    `kv_test_helpers_test.go:121` (PR 1); `attack_test.go:108` (PR 3); `storage_observability_integration_test.go:339`,
    `stream_override_expiry_test.go:354` (PR 5); `heartbeat_test.go:365`, `metrics_forwarder_test.go:219`, `:261`,
    `:317`, `:410`, `:460`, `:587`, `:633`, `:678`, `:685`, `:837`, `:953` (PR 6).
  - `TestNoSkippedTests`: `config_boot_process_restart_integration_test.go:106` — `t.Skip("subprocess helper")` (PR 3);
    `service_manager_prop_test.go:168` — `t.Skip("no service left to fail")` (PR 2).
  - Pass: hidden tests and build tags (only `integration`), `t.Parallel` in integration files, bare `select`,
    process-global registration, Own server, broad Docker cleanup; `scripts/lint-test-ports.sh` clean.
    `TestNoRetainedContext` could not type-check the pin's imports; S14 is the manual check.
- **#195:** 18 `natsclient.NewTestClient` calls — PR 3: 12 (`component_manager_boot_findings_integration_test.go` 4,
  `component_manager_integration_test.go` 4, `framework_owned_bucket_guards_integration_test.go` 2, two files 1 each);
  PR 4: 4; PR 5: 2. Plus `main_test.go`'s `NewSharedTestClient` (PR 1).

## 8. Pin ADRs (85 records at `8b99efe9`)

Searched for `service/`, `ComponentManager`, `BaseService`, `MessageLogger`, `message-logger`, `LogForwarder`,
`log-forwarder`, `MetricsForwarder`, `HeartbeatService`, `StorageObservability`, `service-manager`, `ServiceManager`,
`StartAll`, `RegisterAll`, `ProjectionBinder`, `WireGraphRuntime`, `/readyz`, `/healthz`, `component-manager`. Hits:
026, 029, 030, 058, 063, 081, 088, 093, 094, 096, 098, 100; titles then read for lifecycle and boot (047, 075, 095,
103); and, for `graph_runtime.go`'s comment and retention check, 068 and 091.

- **ADR-029** instance-type patterns: services are `service.Manager` + `service.Registry`, config-driven; the Manager
  is passed as `ServiceManager` (`029:31`, `:44`). Governs `dependencies.go`, `registry.go`.
- **ADR-030** HTTP middleware: the one seam is `(*service.Manager).UseHTTPMiddleware`, no default middleware
  (`030:15`, `:87-101`). Governs `middleware.go`.
- **ADR-058** boot phases (Draft; parts superseded by ADR-091): eager wiring in the composition root before
  `StartAll`; pprof is "deliberately NOT a service.Service" (`pprof.go:12`). Carried with change 3 (#9 comment
  6099868722).
- **ADR-068** graph retention (not in `docs/adr`): D1 is a "convention + boot/lint check that live graph buckets
  carry no binding TTL/MaxBytes/MaxAge" (`068:417`). `WireGraphRuntime` is that boot check:
  `graph_runtime.go:25` (`graph.EnsureCatalogRetentionClean`, here in `graph/kvcatalog`). Governs `graph_runtime.go`.
- **ADR-091** graph mutation authority (Accepted 2026-08-05, not in `docs/adr`): "supersedes the ownership-specific
  boot/service/shutdown parts of ADR-058" and removes "boot services" (`091:11`, `:20`); `graph_runtime.go:13-15`
  records it: "It creates no ownership substrate or background coordination service." Governs `graph_runtime.go`
  and `rule_pack_bind.go`.
- **ADR-063** store substrate: the component manager owns and fills `storeregistry` at Start, clears it at Stop
  (`063:69`, `:181`, `:262`). Not in `docs/adr`; #321 asks to carry it.
- **ADR-088** (in `docs/adr`): readiness stays out of `/readyz` (`088:103-105`). With this repository's ADR-109
  (`/readyz` ports as at the pin, Consequences).
- **ADR-093** typed user responses: message-logger decoding is observability evidence (`093:41-43`).
- **ADR-094** boot-only composition: boot seals services and components (`094:35-44`); its "Terminal shutdown remains
  Start-owned" is superseded here by ADR-113. **ADR-095** one-shot running lifecycle, retained failed-Start cleanup
  (`095:24-36`): every owner. **ADR-096**: the component manager reads configuration once and owns the handles
  (`096:32-35`). 094 and 096 are carried with change 3 (#9 comment 6099868722).
- **ADR-098** observability splits by subject: substrate observability is logs and metrics, out of band (`098:58-62`):
  `MetricsForwarder`, `HeartbeatService`, the health publishers.
- **ADR-100**: the component manager serves the composition projection and findings over HTTP (`100:44-51`).
- Named, not governing: 026 (superseded watch model), 081 (names `message_logger_kv_watch.go:216` as a candidate;
  not done at the pin), 047 (the `LifecycleManager` plumbing, dropped by #102). 075 and 103: no `service` decision.
- This repository: ADR-108 Decision 2, ADR-109, ADR-111 (`service` is a change-3 durable owner that cites Decisions
  1-6, `111:55-56`), ADR-113, ADR-102/104 (`service.Dependencies.Platform` is an excepted carrier,
  `internal/harness/contract/authority_test.go:231`), and ADR-110, the write seam: "Each port that writes entity
  state, and change 3's design, cites this record" (`docs/adr/110-write-seam.md:183`; also `:7`). `service` writes
  no `ENTITY_STATES` entry itself (`graph_triples_http.go:181` only reads it); it builds mutation clients
  (`graph_runtime.go`, `rule_pack_bind.go:72`), and the design states that against ADR-110.

## 9. Open issues and pull requests

`gh issue list --state open --search <q> --limit 200` for `service`, `ComponentManager`, `component_manager`,
`message_logger`, `MessageLogger`, `heartbeat`, `metrics_forwarder`, `forwarder`, `storage_observability`,
`service_manager`, `StartAll`, `readyz`, `healthz`, `pprof`, `RegisterAll`, `lifecycleUsed`, `rule_pack_bind`,
`ProjectionBinder`, `PredicateReconciler`, `WireGraphRuntime`, `openapi`, `middleware`, `log-forwarder`,
`logforwarderpolicy`, `flow-service`, `storeregistry`, `boot`.

- **#30** (do not port `milestone_service.go`): names `service/milestone_service.go:10`. Retired by the last
  `service` PR, when `go list -deps ./service` can show no `agentic` package for the whole package.
- **#196** (lifecycleguard adoption): names `service/component_manager.go:102` and `service/message_logger.go:234`.
  PRs 3 and 4 adopt; the issue stays open for changes 4-7.
- **#195** (`natsfixture.Open`): names `service` 18. PRs 3-5 adopt (§ 7); stays open for other packages.
- **#280** item 2: names `service/message_logger.go:347-349` and says the parallel port and facts lists are "that
  port's" decision. PRs 3 and 4 decide it; items 1 and 3 keep the issue open.
- **#36** (maxdelivery re-home): names `service` as a possible starter; decided by change 7's port. Stays.
- Adjacent, not naming `service`: #321 (carry ADR-063, PR 3 cites it), #29 (its `component` half is done; the
  `service` half is § 2.4), #138 (closed; removed `PredicateReconciler`, Q2), #16 (refusal home is `composition`).
- Names the word, not the package: #9, #7, #10, #11, #24, #68 (no `sync.Once` metric workaround in `service`; the
  three `sync.Once` are stop guards), #106, #109, #110, #216, #300, #301.
- **Open pull requests** (`gh pr list --state open --json number,files`): #343 (this claim); #342
  `claude/port-composition` `8bfab97`, draft, `openspec/changes/port-composition/{.openspec.yaml,proposal.md,tasks.md}`
  — no shared file; PR 3 imports `composition`, so #342 merges first; #341 `claude/config-key-case` `91e6e58`, draft,
  `openspec/changes/config-key-letter-case/` with a `config-loading` spec delta — no shared file or capability; the
  delta leaves a service's `config` payload to its owner (S6).

## 10. Owner questions

Each starts with the recommendation; pointers are in parentheses.

1. **Split: recommend landing the service manager before the component manager.** May the service manager go in
   first with the metrics service it drives (2,960 lines), accepting a new small internal interface with one
   implementation and your waiver for two pull requests that each hold two lifecycle owners, rather than the pin's
   layout of one 5,447-line pull request holding three owners, which needs your waiver too? (§ 2.3; one-owner rule,
   #159)
2. **Predicate reconciler: recommend putting it back where it was.** May the component manager's pull request
   restore the one-method `PredicateReconciler` interface in `pkg/projection`, since the rule-pack binding needs it
   now and the rule processor, ported later, must name the same type to match? (removed as unread in #138 item 1;
   pin `rule_pack_bind.go:16`, `processor/rule/processor.go:463`)
3. **Profiling server: recommend yes.** May `MaybeStartPProf` change so its caller can stop it and wait for it,
   which changes the call in semsource, semteams and semboids, because it must port and the background-work rule
   requires a stop? (S4; `openspec/specs/background-work/spec.md:15-22`)
