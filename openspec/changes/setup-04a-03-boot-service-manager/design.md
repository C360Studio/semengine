# Design: setup-04a-03-boot-service-manager

This design rests on the whole-package inventory, `INVENTORY PASS` round 2 at `ad946b7`
(`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md`, "the inventory"), on this PR's
shape sweep and pin probe (`sweep.md`, "the sweep"), and on the owner's rulings on #9: comment 6296023694 (option
B: the service manager lands before the component manager behind one small unexported interface; this PR holds two
lifecycle owners under waiver) and comment 6297390898 (PR 1 accepted; Q2, the SS#1218 proof is this PR's `StopAll`
test). Pin paths are at `8b99efe9`; repository lines at `db7ef2c`. Size: 2,960 non-test pin lines (inventory § 2.3
row 2), under the ~3,000 cap; nothing assigned to another PR has to come now. The spec deltas are
`specs/service-lifecycle/spec.md` and a new capability, `specs/operator-surface/spec.md` (no current spec covers
the routes: `health-status` explicitly leaves readiness to `/readyz`, `openspec/specs/health-status/spec.md:5-8`).

**Decision records.**

- ADR-113 governs D2 and D4 and is kept: `StartAll`'s context bounds `StartAll`; `StopAll(ctx)` is the only stop.
- Pin ADR-030 (HTTP middleware) is carried into `docs/adr/` in this PR: `middleware.go` and `UseHTTPMiddleware`
  implement its Phase 1 (`030:85-118`), kept unchanged. Port note: Phase 0's identity helpers live in
  `processor/agentic-dispatch`, not ported here, and Phases 2-3 were never built.
- Pin ADR-094 (boot-only composition) is carried in this PR, not PR 3 as PR 1's design expected (its Decision
  records): the `Manager`'s seal (`service_manager.go:502-542`, `CompositionSealedError`) and the `/services`
  `restart_required` answer (`:1909-1917`) implement "Boot seals service and component composition" (`094:35-44`),
  and this PR is the first to carry that code. Port note: its "Terminal shutdown remains Start-owned" (`:44`) is
  superseded by ADR-113; its component-manager and rule-activation parts arrive with PR 3 and change 6.
- Not carried here: pin ADR-029, a catalogue of registry patterns across instance types mostly not ported (tools,
  flows, personas); `service.Registry` keeps its Pattern A shape unchanged, and the `component` port did not carry
  it either. ADR-096 and ADR-100 (in tree) belong to PR 3; pin ADR-098 is carried with PR 6 (`MetricsForwarder`,
  `HeartbeatService`) and cited here for D2's declared drops ("logs and metrics ... tight", `098:58-62`).
- Pin ADR-095 (one-shot running lifecycle, retained failed-start cleanup) governs both owners here (inventory § 8)
  and is cited, not carried: its file comes with PR 3, whose JetStream consumers are its subject (PR 1 design,
  Decision records). Kept: a `StartAll` runs once and a second is refused; a failed `StartAll` makes one bounded
  rollback, and if that fails every handle stays retained for `StopAll` to retry (`095:33-36`, D2). Changed: "a
  deadline is a failed exit result, not authority for later running-generation rejoin" (`095:30`) is superseded by
  ADR-113, as PR 1 recorded (archived design `2026-10-20-setup-04a-03-boot-service-base/design.md:19-21`): a
  `StopAll` whose bound ends returns that error, and a later `StopAll` joins what is still running (D2, Q-B).
- ADR-088 and ADR-109: `/readyz` ports unchanged (`109:107-109`); ADR-109 names its proof tests (D6).
- ADR-110: `graph_triples_http.go` only reads `ENTITY_STATES`, through `kvcatalog.OpenCatalogReader`'s read-only
  capability; this PR writes no entity state.

## D1. Files and surface (surface audit, #190)

| Pin file (non-test lines) | Disposition | Change |
| --- | --- | --- |
| `service_manager.go` (1,958) | adapt | D2, D3, D6; drops below; `attributeShutdownError` moves to PR 3 (sweep P16) |
| `metrics.go` (382) | adapt | D4 |
| `graph_triples_http.go` (250) | adapt | D6; `kvcatalog`, 3-argument `graph.UnmarshalEntityState` |
| `startup_metrics.go` (104) | adapt | namespace `semengine`; `publishComponents` moves to PR 3 (P16) |
| `service_manager_config.go` (82) | carry | none |
| `registry.go` (72) | adapt | `Services`, `Constructors` dropped |
| `dependencies.go` (43) | adapt | `ToolRegistry`, `LifecycleManager` dropped (#29, #102 ruling D); `natsPublisher` to PR 6 |
| `middleware.go` (42) | adapt | doc names no absent package (P13) |
| `register.go` (27) | adapt | registers only `metrics` until each owner lands |

- **Dropped, no reader** (inventory § 5 (a), the sweep's P1-P3, P5, P7, P8): `Manager.GetAllServices`,
  `ListConstructors`, `HasConstructor`, `GetHealthyServices`, `GetUnhealthyServices`, `GetServiceStatus`,
  `GetAllServiceStatus`, `StopHealthListener`; the embedded `*BaseService` with every method it promotes, `Start` and
  `Stop` among them (S11); `Registry.Services`, `Registry.Constructors`; `Metrics.Port`, `Path`, `URL`,
  `ConfigSchema`; the dead `isHTTPManager`, `hasNATSAccess`, `completeHTTPSetup`, `stopHTTPRuntime`, and
  `Metrics.natsClient`, `Metrics.running`. `ConfigSchema`, `PropertySchema`, `NewConfigSchema` stay for
  `MessageLogger.ConfigSchema`, PR 4's to re-audit (PR 1 D1).
- **Kept with contract lines** (README, ledger `contract`, the two spec deltas): `NewServiceRegistry`,
  `Registry.Register`, `RegisterAll`, `NewServiceManager`, `Dependencies`, `Constructor`, `ConfigureFromServices`,
  `StartAll`, `StopAll`, `StartHealthListener`, `RegisterInstance`, `GetService`, `UseHTTPMiddleware`,
  `HTTPMiddleware` (inventory § 5: pin `internal/boot`, semsource, semteams, semboids). Read inside the package and
  kept as at the pin: `CreateService`, `Registry.Constructor`, `ManagerConfig` and its methods, the two
  `DefaultHTTP*Timeout` constants, `Metrics`, `MetricsConfig`, `NewMetrics`.
- `Dependencies` keeps `NATSClient`, `MetricsRegistry`, `Logger`, `Platform`, `Manager`, `ComponentRegistry`,
  `PayloadRegistry`, `ServiceManager`. `Platform`, `ComponentRegistry` and `PayloadRegistry` have no reader in this PR;
  their reader is `component_manager.go` (an admitted package's caller counts, architect contract § Surface audit).
- **Text** (P13): the `agenticdispatch` sentences, the `cmd/semstreams -health-port` comment and "SemStreams" in the
  `/docs` title are rewritten. The metric namespace follows the owner's ruling (#69 comment 6024010936: "One rule
  for every outward-facing name: `semengine`"; its listed items, as far as read, name the Prometheus namespace and the
  environment-variable prefix, and the ledger records it so, `docs/admission-ledger.yaml:1230-1235`), so the gauge is
  `semengine_startup_units`. The ruling's words would cover the OpenAPI title strings, but no item read names them and
  the whole comment could not be re-read on 2026-10-21 (GitHub unreachable), so the defaults `"SemEngine API"` and
  `"SemEngine framework API"` (pin `"SemStreams API"` and "Flow-based programming framework API",
  `service_manager.go:185-189`) ride in Q-A for the owner to confirm, with the default version `"0.0.0"` (pin
  `"0.7.0"`, `:191`), what the tree reads when no version is stored (`config/manager.go:1443-1450`), since this tree
  has no release number.

## D2. Manager: StartAll bounds StartAll, StopAll is the only stop

Probe (sweep § 3): the pin `Manager` passes 7 of 8 suite checks and fails `StartContextEndStopsNothing` (the health
publisher and a child service stop), and after the `StartAll` context ends every request runs on a cancelled
context. The port's observable behaviour, proven by `TestManagerPassesTheLifecycleSuite` unless another test is
named:

- `StartAll(ctx)` refuses a nil or ended context, a second `StartAll`, and a `StartAll` after `StopAll`
  (`errs.ErrAlreadyStarted`); a refused `StartAll` starts nothing. Otherwise, in the pin's order: it creates
  `component-manager` if no service has that name (`:404-407`), seals the composition (later `RegisterInstance`,
  `CreateService` and `ConfigureFromServices` calls get `CompositionSealedError`,
  `TestCreateServiceRefusedAfterStartAll`), binds `:http_port` and serves
  the startup routes (D6), binds the metrics server it claimed from the `metrics` service (D4), starts every service
  in registration order with `ctx`, builds the full route set, starts the health publisher and commits. A bind
  failure is returned before any service starts (`TestStartAllMainBindFailureStartsNoService`,
  `TestMetricsBindFailureClosesSharedAndStartsNoLaterService`). Any later failure returns the error joined with the
  rollback's, and the rollback, under `pkg/lifecyclecleanup.RollbackFailedStart`, stops what started; a rollback that
  fails is retried by `StopAll` (`TestStartAllRollsBackFailedChildStartWithDetachedBoundedContext`,
  `TestStartAllRollsBackMainAndPublisherAfterPublisherStartFailure`, the suite's `FailedStartHoldsNothing`).
- Ending `StartAll`'s context after it returned stops nothing: services (each owns its run, ADR-113), the publisher,
  both listeners and the metrics server keep running, and a request's context is not the `StartAll` context
  (`TestRequestContextOutlivesStartContext`: cancel it, then a handler sees `r.Context().Err() == nil`). The
  servers get no `BaseContext` (S2), the shape `metric.Server` (`metric/handler.go:142-145`) and `PProfServer`
  (`service/pprof.go:59-68`) already have here; the publisher runs under a run context created in `StartAll` and
  ended in `StopAll`.
- `StopAll(ctx)` refuses a nil context. An ended context stops nothing and returns its error
  (`TestStopAllWithAnEndedContextStopsNothing`, the suite's `StopReturnsItsEndedContext`). Before any `StartAll` it
  returns nil, stops a health listener if one was started, and a later `StartAll` is refused. Otherwise it makes
  `/readyz` answer `NOT READY` at once, stops every service in reverse registration order with `ctx`, then the health
  publisher, the main and health listeners, and the metrics server last (`:880-917`). A listener stops accepting and
  `StopAll` waits for its in-flight requests (probes N1-N2, H2-H4 on #343). It returns nil only when all are
  released. A service whose `Stop` returns nil or `errs.ErrAlreadyStopped` counts as stopped
  (`TestStopAllCountsAlreadyStoppedAsStopped`: the SS#1218 substitute, owner Q2). Any other failure is returned
  naming the service or listener, and the next `StopAll` runs the whole stop again, calling every service's `Stop`
  once more (`TestStopAllRetainsServiceAuthorityAfterFailure`). A `StopAll` after one that returned nil returns nil
  and calls nothing; one while another runs is refused as transient. After a nil `StopAll`, `GetService` finds
  nothing (`:928-931`).
- **At `StopAll`'s deadline (Q-B, recommended answer).** If `ctx` ends while a listener waits for its requests,
  `StopAll` closes every connection still open on that listener, as the pin does (`:1462-1466`), which ends each
  open request's context (probe C2), and returns `ctx`'s error. Unlike the pin, which called that stop final, a later
  `StopAll` joins what is still running (ADR-113): `net/http` stops tracking a handler once its connection is closed
  (probe C3: a later `Shutdown` returns nil at once while the handler runs), so the manager itself observes its
  handlers, and a later `StopAll` returns nil only once every one has returned, or its own `ctx`'s error first. The
  rollback of a failed `StartAll` stops the listeners the same way, under its own bound. The tree already solves this
  for `metric.Server`: `admittedRequests` (`metric/handler.go:320-390`) counts admitted requests, refuses new ones
  once stopping, and lets `Stop` wait for them after a forced close. It is shared, not copied: it moves, with its
  tests (`metric/admission_test.go`), to a new package `internal/httpadmission`, imported by `metric` and `service`.
  An internal home is allowed because both hold it only in unexported fields, so no exported signature names it
  (`harness-boundaries` spec); the refusal text, "metrics server is stopping" today, becomes an argument so each
  caller names itself, and `metric`'s behaviour is otherwise unchanged (its tests are the proof). The manager wraps
  each listener's handler, main and health, and closes admission when that listener's stop begins, not when
  `StopAll` begins, so `/readyz` still answers `NOT READY` while services stop.
  Proof: `TestStopAllCutsOpenConnectionsAtItsDeadline` (a handler waits for its request context to end and then for
  a release; a 100 ms `StopAll` returns `context.DeadlineExceeded`, the handler saw its context end, a new dial is
  refused, a second `StopAll` does not return before the release and returns nil after it). The pin's
  `TestRuntimeServerCanceledStopIsTerminalWithoutResultReplay` is flipped to it. A stream that watches its request
  context, such as semboids' graph stream (`internal/api/graphstream.go:176-237`), ends at the cut, so a redeploy
  with a browser open is held at most to `StopAll`'s bound. If the owner answers no to Q-B, nothing is closed at the
  deadline: `StopAll` returns `ctx`'s error, a later one waits again for the request, and the proof is round 1's
  `TestStopAllWaitsAgainForABlockedRequest` (probe N3). Either way services stop first, so a service that serves a
  long-lived response ends it in its own `Stop`: PR 4's message-logger watch stream is the case
  (`message_logger_kv_watch.go:207-247`), and the README says so.
- **Health publisher (S8).** With a NATS client, it publishes `{"timestamp","name","status","health"}` for every
  composed service on `health.service.<name>` once at `StartAll` and every 5 s; with none it publishes nothing
  (`TestHealthPublisherPublishesEachServiceAtStart`, integration). A publish or encode that fails is declared, not
  dropped: it increments `semengine_errors_total{service="service-manager",type="health_publish"}` (the existing
  `RecordError`, `metric/core.go:139-141`, on the counter declared at `:80-88`; skipped when no registry was injected)
  and logs at Debug naming the subject, and the loop goes on (`TestHealthPublishFailureIsCounted`, an unconnected
  client). An encode failure has no deployment trigger, so its path shares the count untested (#185; recorded as
  uncovered).
- **Dedicated health listener.** `StartHealthListener(ctx, port)` does nothing for port 0; otherwise it binds `:port`
  before returning (a bind error is returned) and serves `/health` and `/healthz`. `ctx` bounds the bind only. A
  second call, or one after `StopAll` began, is refused; `StopAll` stops it whether or not `StartAll` ran
  (`TestStartHealthListener_BindsHealthAndHealthz`, `_ZeroIsNoOp`, `TestHealthListenerCannotRebindAfterCompletedStop`,
  `TestStartHealthListenerReportsBindFailureSynchronously`). Ending `ctx` after it returned stops nothing and ends no
  request's context: its server gets no `BaseContext` either (S2, pin `:1314`). Proof:
  `TestHealthListenerRequestContextOutlivesStartContext`: cancel the context given to `StartHealthListener`, then
  `/healthz` answers 200 and the context the server gives a new connection has not ended. The health routes take no
  request-scoped input and no middleware, so the test reads that context where the server makes it, as the pin's
  `TestListenerBaseContextsPreserveExactStartValues` did for both listeners.
- **Test-only code** (S9, pin `:84-88`, `:1284`), settled as P2 is:
  - `testCommitPrepared` and `testCommitRelease` (`:85-86`, read at `:1256-1258`) are dropped: unexported code whose
    only reader is a test (`TestStartupMuxCommitIsCausallyAtomic`, not carried). Commit invisibility is proven by
    `TestPreparedFullMuxStaysInvisibleUntilCommit`, which reaches the boundary through a service whose
    `RegisterHTTPHandlers` blocks, a call production makes.
  - `testSharedHTTPBound` and `testMetricsBindRelease` (`:87-88`, read at `:427-429`) are dropped for the same reason.
    `TestMetricsBindFailureClosesSharedAndStartsNoLaterService` keeps its outcome (`StartAll` fails naming the
    metrics bind, no service started, the main listener refuses a dial) without the pause. Its drained-request leg
    moves to `TestStartAllRollbackWaitsForAnInFlightRequest`: a service whose `Start` blocks until released and then
    fails holds `StartAll` open while a `/services` request blocks in a middleware; `StartAll` does not return before
    that request's handler returns, and the request answers `"status":"stopping"`.
  - The `listen` parameter of the unexported `startHealthListener` (`:1284`) is retained (Bounded port scope): it has
    a production caller, `StartHealthListener`, which passes `net.Listen`, the shape PR 1's `startPProf`
    (`service/pprof.go:59`) has, and tests need it because port 0 means disabled, so no call asks for a kernel-picked
    port. Its tests: `TestStartHealthListener_BindsHealthAndHealthz`, `_ZeroIsNoOp`,
    `TestHealthListenerCannotRebindAfterCompletedStop`, `TestHealthListenerRequestContextOutlivesStartContext`.
- **The `Manager` is not a `Service`** (S11, P3): no embedded `BaseService`; a `logger` field, set from
  `deps.Logger` by `ConfigureFromServices` (P4, `TestManagerLogsThroughTheInjectedLogger`).

Findings settled here: S1, S2, S5, S8, S9 (the `Manager`'s part), S11 and P2-P4 (ADR-113).

**Mechanism** (developer's; recommended): compose `internal/lifecycleguard` for `StartAll`/`StopAll` (start: the
sequence above; cleanup: the stop sequence, safe to repeat), as `config.Manager` and `BaseService` do. The pin's
three runtime state machines (`:62-80`, 15 fields) become handles the cleanup releases; the health listener keeps a
one-shot flag outside the guard. If the developer keeps a hand-written machine, the PR adds a generated history check
(Generated checks). #196 lists neither owner, so it stays open.

## D3. The component-manager seam (owner option B)

- **Interface.** The four `*ComponentManager` checks (`:593`, `:1142`, `:1543`, `:1812`) read one unexported
  interface, `componentStartup`: `startupSnapshot() startupUnitCounts`, `startupStopBegun() bool`,
  `setStartupMetricWriter(*startupMetricWriter)`, `registerStartupHTTPHandlers(prefix string, mux *http.ServeMux)`,
  `withComponents(func(map[string]*component.ManagedComponent) error) error` and `GetComponentHealth()
  map[string]component.HealthStatus` (signatures `component_manager.go:724`, `:763`, `:769`, `:830`, `:1319`,
  `component_manager_http.go:83`). A `component-manager` service that does not implement it gets the pin's
  not-concrete path (`:1543-1547`, `:1821`): no component routes, zero component counts, readiness from services.
  PR 3 adds `var _ componentStartup = (*ComponentManager)(nil)`. One production implementation: sweep shape 3,
  accepted by the ruling.
- **Tests.** Two stand-ins: a plain `Service` named `component-manager` (pin `service_manager_test.go:201`), and a
  test double that implements `componentStartup` for readiness and metric tests with components.
- **`StorageObservabilityServiceName`** (`storage_observability.go:70-72`) moves, with its comment, beside
  `serviceNameToPrefix` in `service_manager.go`; PR 5 reads it there.
- **Default composition until PR 3.** `RegisterAll` registers `metrics` only. `ResolveServiceConfigs` adds an enabled
  `component-manager` (`service/service_configs.go:83`), which may not be disabled (pin `service_manager.go:165-169`),
  so `ConfigureFromServices` refuses with "create configured service component-manager: no constructor registered for
  service component-manager", and a `StartAll` without it refuses likewise (probe, sweep § 3;
  `TestConfigureRefusesACompositionWithoutAComponentManager`). A consumer composing only this PR's services gets that
  boot error until PR 3 registers the constructor. No temporary relaxation: it would be a fifth spelling of the
  mandatory rule (P14), removed one PR later.

## D4. Metrics (S1, S3, S9, S10, S13; P5-P7)

- `NewMetrics` decodes `{port, path}` strictly (D5), defaults `9090` and `/metrics`, validates, reads the platform
  TLS settings from `deps.Manager`, and logs through the injected logger (P6).
- **Standalone** (`Start` before any claim): `Start(ctx)` binds `:port` and serves `metric.Server`, then starts its
  `BaseService`; a bind failure is returned and nothing is held
  (`TestMetricsStartReportsServerBindFailureBeforeReturning`). `ctx` bounds `Start` only: `metric.Server`
  (`metric/handler.go:53-56`) and `BaseService` already follow ADR-113, so no run context is derived (S1). `Stop` stops
  the server, then the `BaseService`. `TestMetricsPassesTheLifecycleSuite` (pin: 7 of 8).
- **Driven by the manager:** `claimManagerServer` hands the server to the `Manager` before any service starts, and is
  refused once `Metrics.Start` or `Stop` has been called or a server was already claimed
  (`TestMetricsRefusesAClaimAfterStart`). The `Manager` binds it after the main listener (`:423-434`) and stops it
  last; `Metrics` is healthy while that server serves (`:293-298`).
- S3: recommended, the guard holds `Metrics`' Start/Stop state (`:29-38`); S9: `testListener` is retained (Bounded
  port scope) because `metric.NewServer` maps port 0 to 9090 (`metric/handler.go:41-43`), so no configuration asks
  for a kernel-picked port; the other three test fields go with the copied machine. S10: `metricsServer` goes; the
  field is `*metric.Server`. S13: `PrometheusRegistry().Register` is retained: the pin refuses to adopt another
  writer's gauge (`TestStartupMetricWriterRejectsForeignCollectorInsteadOfAdoptingIt`), which `RegisterOrGet` would
  adopt, and the `metric-registry` spec permits direct registration on the injected registry.

## D5. Service configuration: one letter-case check (S6; PR 1 D5; #344)

`config`'s checker, `checkKeyCase` (`config/config.go:760-805`), moves into a new `internal/keycase` package as
`keycase.Check(value any, t reflect.Type, path string) error`. `config`'s `rejectCaseVariantKeys` (`:756-758`) calls
it, with no change in behaviour (the `config-loading` spec's scenarios are its tests). `decodeStrictServiceJSON`
(`service/service_configs.go:130-139`) runs it on the decoded document against the target's type before the strict
decode, so every strictly decoded service payload refuses a key in another letter case, naming both spellings:
`parse metrics config: config key Port must be written port` (`TestServiceConfigRefusesKeyInAnotherCase`, for the
manager's and the metrics service's payloads). The check walks struct fields, pointers and map values; it does not
walk slices or arrays of structs, embedded structs, or a type that decodes itself (archived design
`2026-10-20-config-key-letter-case/design.md:40-52`). Its guard test and that test's sensitivity test
(`config/load_test.go:544-560`, with their helper) move with it into `internal/keycase`'s external test package. The
guard, renamed `TestPayloadTypesReachOnlyShapesTheLetterCaseCheckWalks`, covers `config.Config`,
`service.ManagerConfig` and `service.MetricsConfig`, so a payload type that reaches a shape the walk misses fails a
test; `TestLetterCaseWalkerGapsDetectsEachShape` keeps its name. Neither service type reaches one today
(`ManagerConfig` holds ints, strings, a bool and `InfoSpec`'s three strings; `MetricsConfig` an int and a string).
Exporting the function from `config` also passes the public-signature rule (it names no `internal` type); the
internal package is chosen because it adds no public surface, a promise nothing outside this repository reads;
`internal/logforwarderpolicy` is the precedent for configuration semantics kept inside the repository for `service`
to read.

Issue #344 stays open: the heartbeat service decodes with a plain decoder (`heartbeat.go:84`, S6). PR 6 routes it
through `decodeStrictServiceJSON`, which then refuses the merged `{"Interval":"5s","interval":"30s"}` naming `Interval`.

## D6. HTTP surface (`operator-surface`)

Governed by the 03B operator-surface rows (`design.md:722-724`), ADR-030, ADR-088 and ADR-109.

- **Boot routes.** Before commit the main listener serves `/health`, `/healthz`, `/readyz`, `/services`,
  `/services/health` (and PR 3's read-only component routes) and answers anything else `503 NOT READY`
  (`:1140-1149`); after commit the full set (`:1707-1720`, `:1517-1520`, `:1568-1572`). Proof: ADR-109's two tests in
  this PR, `TestStartupReadinessAndAtomicPromotion` and `TestReadinessRequiresCommitAndClearsBeforeChildStop`, with
  `TestPreparedFullMuxStaysInvisibleUntilCommit` and `TestHealthPublisherFailureNeverCommitsPreparedRoutes`.
- **Served as at the pin:** `/readyz` (`READY` only after commit with every service running and healthy and every
  component healthy; `:1782-1866`), `/healthz` (`200 OK`), `/services`, `/services/health`
  (`TestServicesListsComposedServicesWithHealth`). `/health`'s NATS sub-status reads `Disconnected: <status>`
  without the pin's failure count, a consequence the owner accepted when `Status.FailureCount` was removed (archived
  design `2026-10-17-simplify-04a-16-natsclient-circuit/design.md:171`, which names `service_manager.go:1752`), not
  a new pin-behaviour change (`TestSystemHealthReportsNATSDisconnected`).
- **`/openapi.json` and `/docs`.** The document merges each `HTTPHandler`'s paths under its prefix and the schemas of
  every registered spec (`:1615-1684`). #349 is fixed here (`Closes #349`): `TypeNameFromReflect` gives a pointer
  its element's name, keeps a named type's bare name (so every pin registration names the same schema), and gives a
  slice, an array, a map and an anonymous struct names distinct from their element's and from each other, each
  matching OpenAPI's `^[A-Za-z0-9._-]+$` (`TestTypeNameFromReflectNamesEachShapeApart`;
  `TestOpenAPIDocumentListsMountedRoutes`, the 03B row's proof). `/docs` with `swagger_ui` loads its UI from
  `unpkg.com` (P18, declared). The `SchemaRef` registration rule still arrives with PR 3, the first service that
  registers a spec (`component_manager_http.go:20`); the developer and reviewer contracts' sentence stays true.
- **`/graph/triples`** (framework API, inventory § 2.5; contract line in `operator-surface`). Kept: `GET` only, `limit`
  default 100, clamped to 1000, 0 meaning the default; `object` matched on its `%v` text; `[]` never `null`. Corrected
  if the owner answers yes to Q-A (sweep P9-P11): `subject` must pass `types.ValidateEntityID` and `predicate`
  `vocabulary.ParsePredicate`, else `400` naming the parameter; keys are sorted before the scan, so the answer is the
  first `limit` triples of the sorted whole; only a key deleted between listing and reading is skipped, any other read
  error is a `500`. Tests: `TestGraphTriplesRefusesMalformedSubjectAndPredicate`,
  `TestGraphTriplesSkipsOnlyADeletedKey`, `TestScanGraphTriplesMatchesSortedOracle` (generated),
  `TestGraphTriplesReadsEntityStates` (integration). P12's `[]` without a NATS client is declared in the README. If the
  owner answers no, the route ports as at the pin, and P9-P11 are filed as issues (`status:blocked` on #353) rather than
  left as notes.
- **Middleware** (ADR-030): kept, outermost-first, before `StartAll`; a later call is ignored with a Warn
  (`middleware_test.go`, carried).

## D7. Tests

| Pin test file (lines) | Here |
| --- | --- |
| `service_manager_test.go` (985) | adapted; `:637-911` not carried (P17); runtime-stop tests flipped to D2 |
| `service_manager_prop_test.go` (277) | `rapid` kept; model rewritten (Generated checks); `:168` `t.Skip` becomes a no-op action |
| `service_manager_stopall_test.go` (141) | `TestServiceManager_StopAll_Idempotency` with `errs.ErrAlreadyStopped`; the other two not carried |
| `service_manager_health_listener_test.go` (336) | carried; `TestListenerBaseContextsPreserveExactStartValues` becomes the two S2 tests (D2) |
| `startup_observability_test.go` (849), `_amendment_test.go` (515) | split: tests with a real `ComponentManager` go to PR 3; the two pause tests change (D2, test-only code) |
| `metrics_owner_test.go` (263), `metrics_lifecycle_test.go` (29), `metrics_config_test.go` (14) | adapted to D4 |
| `middleware_test.go` (283), `register_test.go` (55), `registry_race_test.go` (380) | adapted; dropped surface goes |
| `graph_triples_http_test.go` (409) | carried, plus D6's tests |
| `metrics_runtime_integration_test.go` (160), `test_helpers_integration_test.go` (47) | carried (PR 1 D6) |
| `storage_observability_test.go:75-112` | `gaugeValue`, `requireGauge` move into a PR 2 helper file |

- Not carried with reasons: `TestServiceManager_StopAll_CancellationBeforeStopAll` asserts the pin's abort with
  PR 6's services; `TestBaseService_CompletedStopIsIdempotent` repeats PR 1's
  `TestBaseServiceCompletedStopRejectsSameInstanceRestart`. Of the five files placed with PR 5 by the inventory:
  `service_manager_health_listener_test.go` comes here; the two startup files split (PR 3:
  `TestComponentStartupSnapshotDistinguishesAdmittedAndLifecycleStarts`,
  `TestReadinessIncludesHealthyNonLifecycleDiscoverables`, `TestReadinessWaitsForInitialServiceHealthObservation`,
  `TestStartupMuxAdmitsOnlyReadOnlyComponentDiagnostics`,
  `TestComponentMetricPublicationRemainsMonotonicUnderReverseCompletion`,
  `TestManagerPrebindPublishesConcreteComponentCountsBeforeComponentManagerStart`,
  `TestPreparedComponentIsNotInvokedUntilLaunchBoundary`); `stream_override_expiry_test.go` is PR 3's; `pprof_test.go`
  was rewritten by PR 1.
- Changed: `TestStartAllRollsBackFailedChildStartWithDetachedBoundedContext` gains the assertion its spec scenario
  makes and the pin never checked (`service_manager_test.go:176-284`): after `StartAll` returns, a dial to the main
  listener is refused.
- New: the two suite adapters (`TestManagerPassesTheLifecycleSuite`: observes the main listener, the publisher, the
  claimed metrics server and a child `BaseService`, with `mustFail` a composition whose second service's `Start`
  fails; `TestMetricsPassesTheLifecycleSuite`: a test listener, `mustFail` a port a test listener holds, probe H1 on
  #343) and every test D2-D6 names.
- Harness rules: the `Manager` gets port 0 through the test helper that sets its configuration, as the pin's does;
  no `time.Sleep`, `Skip`, fixed port, or `t.Parallel` in an integration file; every stop in a cleanup under
  `context.WithTimeout(context.WithoutCancel(t.Context()), ...)`; integration tests reach the shared server through
  the reset (#195: `natsfixture.Open` via `fixtureClient`, PR 1's file).
- Sensitivity (task 2.3; unit mutants through `task mutate:check`, the integration one by hand, #129):

  | Mutant | Caught by |
  | --- | --- |
  | publisher run context derived from `StartAll`'s | the suite's `StartContextEndStopsNothing` |
  | `BaseContext` returning the `StartAll` context | `TestRequestContextOutlivesStartContext` |
  | the health listener's `BaseContext` returning its start context | `TestHealthListenerRequestContextOutlivesStartContext` |
  | `errs.ErrAlreadyStopped` counted as a failure | `TestStopAllCountsAlreadyStoppedAsStopped` |
  | connections left open when `StopAll`'s context ends | `TestStopAllCutsOpenConnectionsAtItsDeadline` |
  | a later `StopAll` returning while a cut handler still runs | `TestStopAllCutsOpenConnectionsAtItsDeadline` |
  | the rollback not waiting for an in-flight request | `TestStartAllRollbackWaitsForAnInFlightRequest` |
  | admission closed when `StopAll` begins rather than when a listener's stop begins | `TestReadinessRequiresCommitAndClearsBeforeChildStop` |
  | the publish failure not counted | `TestHealthPublishFailureIsCounted` |
  | the letter-case check removed from `decodeStrictServiceJSON` | `TestServiceConfigRefusesKeyInAnotherCase` |
  | the case check skipped for a nested `server_info` key | `TestServiceConfigRefusesKeyInAnotherCase` |
  | keys not sorted before the scan | `TestScanGraphTriplesMatchesSortedOracle` |
  | every read error skipped | `TestGraphTriplesSkipsOnlyADeletedKey` |
  | a slice named by its element | `TestTypeNameFromReflectNamesEachShapeApart` |
  | a claim allowed after `Metrics.Start` | `TestMetricsRefusesAClaimAfterStart` |

## Ruling to implementation

| Ruling | Implementation | Proving test |
| --- | --- | --- |
| Owner 2026-10-20 option B (#9 comment 6296023694); waiver for two owners | D1 files; D3 interface | `go build ./service/`; review of the file list |
| Owner Q2 (#9 comment 6297390898); 03B SS#1218 | `StopAll` counts `errs.ErrAlreadyStopped` as stopped | `TestStopAllCountsAlreadyStoppedAsStopped` |
| ADR-113; ADR-095 (pin, cited); `lifecycle-suite` floor | D2, D4 | the two suite tests; `TestRequestContextOutlivesStartContext`; `TestHealthListenerRequestContextOutlivesStartContext` |
| Q-B (owner, pending) | D2 at `StopAll`'s deadline | `TestStopAllCutsOpenConnectionsAtItsDeadline` |
| Q-A (owner, pending) | D6 `/graph/triples`; D1's OpenAPI defaults | D6's four tests |
| PR 1 D5 (accepted design); #344 comment 6301408420 | D5 | `TestServiceConfigRefusesKeyInAnotherCase`; `config-loading` tests unchanged |
| Owner 2026-10-12 surface audit (#190) | D1 drops and contract lines | `go doc ./service` lists only the kept surface |
| "skip, drop, degrade declared" (`AGENTS.md` § 1); pin ADR-098 | D2 health publisher | `TestHealthPublishFailureIsCounted` |
| Developer contract `:141-150`; fail closed | D6 `/graph/triples`, under Q-A | D6's four tests |
| 03B operator-surface rows; ADR-088, ADR-109 | D6 | ADR-109's tests; `TestServicesListsComposedServicesWithHealth`; `TestOpenAPIDocumentListsMountedRoutes` |
| simplify-04a-16 `Status.FailureCount` removed (archived design `:171`) | `/health` without failure count | `TestSystemHealthReportsNATSDisconnected` |
| Outward names read `semengine` (#69 comment 6024010936: the Prometheus namespace) | D1 metric namespace | the gauge name in the startup metric tests |
| Q-A's one line (the OpenAPI title strings and version) | D1 defaults | `TestOpenAPIDocumentListsMountedRoutes` |
| ADR-030, ADR-094 (pin, carried) | middleware; seal | `middleware_test.go`; `TestCreateServiceRefusedAfterStartAll` |
| #195; harness rules | D7 | `task verify` guards |

## Generated checks

Decided per `docs/testing.md` "Decide whether generated checks are needed":

- **`StopAll` passes** (a history). The pin's `rapid` state machine is kept and its model rewritten for D2: actions
  register (before `StartAll` only), `StartAll` once, arm a stop failure on a service, `StopAll`. After each action,
  checked against a reference model of who must be visited: a `StopAll` after `StartAll` or after a failed pass
  visits every service once in exact reverse registration order and hands each the pass's context; it returns nil
  exactly when no armed failure fired in that pass, a drawn `nil` or `errs.ErrAlreadyStopped` repeat counting as
  clean; after a nil pass a later `StopAll` visits nothing and returns nil. The arm action is a no-op when no service
  is left to fail. The run records its seed and check count.
- **`/graph/triples` scan** (a stated law). `TestScanGraphTriplesMatchesSortedOracle` generates keys, entities,
  triples, filters and a limit over a stub reader; the oracle sorts every matching triple by key and position and
  takes the first `limit`.
- **`StartAll` ordering, `Metrics`, `TypeNameFromReflect`:** examples suffice. The suite's eight checks plus D2's
  named tests enumerate the orderings once the guard is composed (otherwise the PR adds a model check, as PR 1
  recorded); reflected types cannot be generated, so one example per shape.
- **The letter-case check** (an input format): examples suffice, because each key is judged alone against the field
  names at its own level, so keys do not interact. The cases are one per shape the walk visits, each with an exact
  match beside a variant: a top-level field (`{"Port":9091}`), a nested struct (`server_info.Title`), a pointer, a
  map value whose struct fields are checked, and a map key that is left alone; plus the first error being the same
  on every run (keys visited sorted). The guard test closes the shapes the walk does not visit (D5).

## D8. Ledger, text, guidance, coverage

- **Ledger:** the `service` row edited in place, PR 1's text kept and PR 2's sentences added (YAML below);
  new rows `docs/adr/030-...` and `docs/adr/094-...` in the ADR-058 row's shape; the `config` row's `known_risks`
  gains "the letter-case check lives in internal/keycase, shared with service". `internal/keycase` is code written
  here and moved out of `config`, so it takes no row of its own, as `internal/lifecycleguard` has none; the same holds
  for `internal/httpadmission`, moved out of `metric`, whose row's `known_risks` gains "the admitted-request counter
  lives in internal/httpadmission, shared with service".

```yaml
- source_path: service
  source_sha: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128
  consumer_purpose: >-
    the service layer of the boot (change 3), ported in seven pull requests by lifecycle owner (#9 comment
    6296023694). PR 1: BaseService and the Service interface every service embeds, also embedded by semteams
    (cmd/semteams/rule_config_service.go:32) and semboids (internal/api/service.go:72); the OpenAPI types semboids
    builds (:171-197); MaybeStartPProf, called by semsource, semteams and semboids; WireGraphRuntime, called by
    semteams (cmd/semteams/main.go:840); the service-configuration, schema and OpenAPI registry helpers the later
    PRs read. PR 2: the service manager and the metrics service, composed by semsource
    (cmd/semsource/run.go:371-391), semteams (cmd/semteams/main.go:249, :714-719, :887, :916) and semboids
    (cmd/semboids/main.go:401-409, :457)
  destination: service
  contract: >-
    the service-lifecycle and operator-surface specs; ADR-113; ADR-088 and ADR-109 (/readyz); the background-work
    spec (PProfServer.Shutdown); pin ADR-030 and ADR-094 as carried; pin ADR-058 as carried (pprof is not a Service
    and now stops); pin ADR-068 D1 and ADR-091 as graph/kvcatalog and internal/graphmutation implement them;
    ADR-110 (service writes no ENTITY_STATES entry); service/README.md for the calls semsource, semteams and
    semboids make
  dependencies_and_side_effects: >-
    component, config, graph, graph/kvcatalog, health, internal/httpadmission, internal/keycase,
    internal/lifecycleguard, message, metric, natsclient, payloadregistry, pkg/errs, pkg/lifecyclecleanup,
    pkg/projection, pkg/security, pkg/types, types, vocabulary (all in tree); BaseService runs one health goroutine,
    joined in Stop; MaybeStartPProf binds :port on every interface and serves http.DefaultServeMux on a goroutine
    joined in Shutdown; the Manager binds :http_port (default 8080) and an optional health port on every interface,
    serves until StopAll, publishes each service's health on health.service.<name> every 5 s, and starts the metrics
    server it claims (default :9090); WireGraphRuntime may rewrite the retention of inactive catalog buckets; the
    OpenAPI registry is process-global
  known_risks: >-
    PR 1 of 7 (base.go, pprof.go, graph_runtime.go, service_configs.go, schema.go, openapi.go, openapi_types.go,
    openapi_registry.go, configurable.go, doc.go, README.md); the managers and services follow. Dropped as unread:
    WithHealthCheck, WithHealthInterval, OnHealthChange (option and method), RegisterMetrics (BaseService and Service),
    NewOpenAPISpec, AddPath, AddTag, GetOpenAPISpec, Configurable, service.ErrAlreadyStopped (one sentinel,
    errs.ErrAlreadyStopped), and NewBaseServiceWithOptions' unread config argument (Q1). Changed from the pin: ending
    Start's context stops nothing and a later Stop waits again (ADR-113); a Stop during another Stop is refused as
    transient (lifecycleguard), where the pin returned nil before the first Stop finished; a Stop given an ended context
    stops nothing; MaybeStartPProf takes a logger, binds before it returns, logs a bind failure at Warn instead of
    printing it, and stops through Shutdown (owner 2026-10-20); NewConfigSchema keeps a Category set on the outer
    PropertySchema field, which the pin dropped (fixed in the port). Retained: pprof listens on all interfaces;
    GetAllOpenAPISpecs copies the map but shares the registered spec pointers; service_configs.go, schema.go and
    openapi.go are byte-equal to the pin. PR 2 of 7 (service_manager.go, service_manager_config.go, startup_metrics.go,
    graph_triples_http.go, middleware.go, dependencies.go, registry.go, register.go, metrics.go). Dropped as unread:
    Manager.GetAllServices, ListConstructors, HasConstructor, GetHealthyServices, GetUnhealthyServices,
    GetServiceStatus, GetAllServiceStatus, StopHealthListener, the embedded BaseService with its Start and Stop,
    Registry.Services, Registry.Constructors, Metrics.Port, Path, URL and ConfigSchema, Dependencies.ToolRegistry and
    LifecycleManager; the Manager's four test-only pause channels. Changed from the pin: ending StartAll's or
    StartHealthListener's context stops nothing and request contexts are not derived from it (ADR-113); at its deadline
    StopAll still closes open connections, but a later StopAll waits for every handler still running instead of
    returning at once (Q-B); a StopAll with an ended context stops nothing; a StopAll before any StartAll stops only a
    started health listener and calls no service's Stop, where the pin called Stop on every configured service; a second
    StartAll is refused with ErrAlreadyStarted; StopAll counts errs.ErrAlreadyStopped as stopped; service payloads
    refuse keys in another letter case (internal/keycase), which PR 1 accepted; a failed health publish is counted on
    semengine_errors_total; the /health nats sub-status drops the failure count; /graph/triples validates subject and
    predicate, sorts before its limit and fails a read error other than a deleted key (Q-A); TypeNameFromReflect names
    slices, maps and anonymous structs apart (#349); the Manager logs through the injected logger; the metric namespace
    reads semengine (#69); the OpenAPI defaults read SemEngine API, SemEngine framework API and 0.0.0 (Q-A). Retained:
    the Manager's listeners on every interface; /docs loads Swagger UI from unpkg.com; with no NATS client
    /graph/triples answers []; the startup gauge registers directly on the injected registry; startHealthListener's
    listen parameter and Metrics' testListener, because port 0 does not ask for a kernel-picked port; component-manager
    is mandatory and RegisterAll registers only metrics until PR 3
  proving_tests: service/*_test.go, service/*_integration_test.go
  owner: setup-04a-03-boot-service-manager developer
  disposition: adapt
```

- **`docs/repository-map.md`:** the ledger line's count becomes 88 entries (`:102`, two ADR rows); the `service/` row
  adds "PR 2 (`setup-04a-03-boot-service-manager`) lands the service
  manager (`StartAll` bounds `StartAll`, `StopAll` is the only stop; the boot, health, readiness, service, OpenAPI
  and `/graph/triples` routes) and the metrics service"; the `docs/adr/` row adds ADR-030 and ADR-094; a new
  `internal/keycase/` row: "the one check that refuses a configuration key written in another letter case, used by
  `config` for its document and by `service` for each strictly decoded service payload"; a new
  `internal/httpadmission/` row: "counts the requests an HTTP server has admitted, refuses new ones once its stop
  begins, and lets the stop wait for them after a forced close; used by `metric`'s server and `service`'s
  listeners".
- **`docs/contract.md`:** the operator table (`:129-140`) gains a row, "graph triples (read-only) |
  `/graph/triples`", since `operator-surface` now gives the route a contract line.
- **Text:** `service/doc.go` and `service/README.md` gain the manager and metrics sections: composing with
  `NewServiceRegistry`, `RegisterAll`, `NewServiceManager`, `ConfigureFromServices`, `UseHTTPMiddleware`, `StartAll`
  with any context, `StopAll` with a fresh bounded one; the configuration keys (`http_port`, `swagger_ui`,
  `server_info`, `http_read_timeout`, `http_write_timeout`; `port`, `path`); the routes; that a service serving a
  long-lived response ends it in its `Stop`; and that until PR 3 a composition needs its own `component-manager`.
  The README's last paragraph (letter case accepted) is replaced.
- **Guidance:** no pin contract section or skill names this PR's surface beyond the `SchemaRef` rule, which stays
  with PR 3, and the pin reviewer's permission for a `BaseContext` capturing the Start context
  (`semstreams-reviewer.md:201-203`), which ADR-113 supersedes and this tree's contracts never carried.
- **Coverage:** `"service merged"` is already in `scripts/cover-check.sh`; task 3.2 records the number.

**Issues.** Retired: #349 (`Closes #349`, task 2.2). Stay: #344 (PR 6, D5); #196 (lists PR 3's and PR 4's owners;
this PR composes the guard without being listed); #195 (PRs 3-5; this PR opens no new client); #280 item 2 (the
component manager's and message logger's port lists; no PR 2 file reads them); #30 (the last `service` PR); #29
names `component` only. Names the word, not these files: issues #9, #10, #11, #24, #36, #68, #109, #110,
and #198, #216, #309, #314, #321, #350 (`gh issue list --state open --search` on `service`, `readyz`, `graph/triples`,
`openapi`, `metrics.go`, `Metrics`, `dependencies`, `TypeNameFromReflect`, `lifecycleguard`, `letter case`; empty for
`service_manager`, `ServiceManager`, `StartAll`, `StopAll`, `healthz`, `swagger`, `middleware`, `RegisterAll`,
`startup_metrics`, `StartHealthListener`, `UseHTTPMiddleware`, `PrometheusRegistry`). **Merge order:** the only open
pull request is this claim (#353, `gh pr list --state open --json number,files`); nothing overlaps.

**Probes** (the probe note, #353 comment 6315789814, and its round 2 addendum): the pin suite runs and S2 (sweep
§ 3); N1-N3, `http.Server` shutdown without `BaseContext`, and N4, `DisallowUnknownFields` and letter case, at
go1.26.9; C1-C4, `Close` after a bounded `Shutdown`, at go1.26.9; PR 1's H1-H4 on #343.

## Owner questions

Two changes to what callers see are the owner's to accept (a change to what a route answers is the owner's call,
PR #318). Each is written as the owner reads it.

- **Q-A. May `/graph/triples` answer differently from SemStreams?** Recommend yes: it would answer 400 for a malformed
  `subject` or `predicate`, answer 500 instead of a partial 200 when a stored entity cannot be read, and pick the first
  `limit` triples after sorting rather than in storage order (its only caller among the consumers, semteams, already
  treats any non-2xx answer as a failed read, `ui/src/lib/services/runStatusApi.ts:45-52` and
  `ui/src/lib/server/work/overlays.ts:102-110`, calls SemStreams' partial answer untrustworthy, `overlays.ts:12-16`,
  and sends only full entity IDs and three-part predicates, `overlays.ts:236`, `:327`, `:373`, `:423`; sweep P9-P11;
  developer contract `:141-150`). One line more: the API document's defaults would read `SemEngine API`,
  `SemEngine framework API` and version `0.0.0` instead of SemStreams' `SemStreams API`,
  `Flow-based programming framework API` and `0.7.0`, since SemEngine has no release number yet.
- **Q-B. When `StopAll` runs out of time, should it still cut the connections that are open?** Recommend yes: keep
  SemStreams' cut at the deadline, which ends each open request such as a browser's live graph stream, and have a
  later `StopAll` wait for any handler still finishing (SemStreams closes the server after a timed-out shutdown, pin
  `service_manager.go:1462-1466`; ADR-113 asks only that the error be returned and a later stop try again,
  `docs/adr/113-start-context-bounds-start.md:61-62`; the case is semboids' graph stream,
  `internal/api/graphstream.go:176-237`, open in a browser during a redeploy; probe C1-C3; if no, nothing is cut and a
  later `StopAll` waits again for the open requests, as round 1 of this design said).

The rest is answered by the repository: the rulings cited, ADR-113, the developer contract's graph rules, the surface
audit and Bounded port scope. Other changes from the pin that an operator can see are listed in the ledger's
`known_risks` above.
