# Design: setup-04a-03-boot-service-base

This design depends on `INVENTORY PASS` for `inventory.md` (round 2 at `ad946b7`) and on the owner's split ruling (#9
comment 6296023694: option B; `MaybeStartPProf` gains a stop in this PR). Its spec delta is
`specs/service-lifecycle/spec.md`. Pin paths are at `8b99efe9`; repository lines are at `a2758da` (`main` merged, #341
included). Size: 1,475 non-test pin lines (inventory § 2.3 row 1), under the ~3,000 cap; no waiver. This is PR 1 of 7:
`BaseService`, the `Service` interface and the shared files. `Manager` + `Metrics` (PR 2) follow it.

**Decision records.**

- ADR-113 governs D2 and is kept: the Start context bounds Start; `Stop(ctx)` is the only stop.
- Pin ADR-058 (boot phases) is carried into `docs/adr/` in this PR with a port note and a ledger row, because this PR
  changes one of its decisions. Step 4 (`058:392-400`, `pprof.go:12`) keeps "pprof is deliberately NOT a
  service.Service, started early before NATS" and drops "fire-and-forget": the owner's ruling and the
  `background-work` spec give it a stop (D3). The port note also records: its ownership sections are superseded by
  pin ADR-091; its "Terminal shutdown remains Start-owned" by ADR-113; its `Service` listing (`058:41-42`,
  `Stop(timeout)`, `RegisterMetrics`) is not this tree's (D1, D2); `internal/boot` and the mains are not ported.
- Pin ADR-095 is not carried here. Its subject is owners of JetStream consumers; it comes with the component manager
  (PR 3), with ADR-094 and ADR-096. Its "a deadline is ... not authority for later running-generation rejoin"
  (`095:30`) is superseded here by ADR-113 (D2).
- Pin ADR-068 D1 ("boot/lint check that live graph buckets carry no binding TTL/MaxBytes/MaxAge", `068:417`) and pin
  ADR-091 ("removes ... boot services", `091:11`, `:20`) bind `graph_runtime.go` (D4). They are cited, not copied:
  `graph/kvcatalog` and `internal/graphmutation` implement them. Change 2 ported those, and the carry rule (#157)
  does not reopen its decisions; this PR only calls them.
- ADR-110 binds change 3's design ("Each port that writes entity state, and change 3's design, cites this record",
  `docs/adr/110-write-seam.md:183`). `service` writes no `ENTITY_STATES` entry; `WireGraphRuntime` builds the typed
  mutation client that writes through graph-ingest's seam (D4).
- ADR-108 Decision 2 names the component manager and message logger, not `BaseService`; D2 recommends the guard
  anyway. ADR-111 does not apply: nothing in this PR keeps durable state.

## D1. Files and surface (surface audit, #190; owner 2026-10-19, simple fixes)

| Pin file (non-test lines) | Disposition | Change |
| --- | --- | --- |
| `base.go` (495) | adapt | D2, with its one stop sentinel; the dropped surface below; "semstreams platform" → SemEngine |
| `pprof.go` (52) | adapt | D3 |
| `graph_runtime.go` (35) | adapt | `graph.EnsureCatalogRetentionClean` → `kvcatalog.EnsureCatalogRetentionClean` (inventory § 2.4) |
| `service_configs.go` (192) | carry | none (D5) |
| `schema.go` (160), `openapi.go` (25) | carry | none |
| `openapi_types.go` (119) | adapt | `NewOpenAPISpec`, `AddPath`, `AddTag` dropped |
| `openapi_registry.go` (40) | adapt | `GetOpenAPISpec` dropped; "called from init()" advice rewritten (below) |
| `configurable.go` (50) | adapt | `Configurable` dropped; the Discovery comment rewritten |
| `doc.go` (307), `README.md` (669) | rewritten | only the surface this PR lands, with the contract lines below |

- **Dropped, no reader** (pin, its admitted set and the four consumers; inventory § 5 (a)):
  - `WithHealthCheck`, `WithHealthInterval`, `OnHealthChange` (`base.go:166`, `:173`, `:180`);
  - the method `BaseService.OnHealthChange` (`:360`) and the callback field behind both;
  - `NewOpenAPISpec`, `AddPath`, `AddTag` (`openapi_types.go:88`, `:97`, `:102`); `GetOpenAPISpec`
    (`openapi_registry.go:35`);
  - `Configurable` (`configurable.go:14`; no type assertion anywhere).

  `SetHealthCheck` stays: the component manager reads it. The health interval stays 30 s (`base.go:122`), since no
  production caller set another.
- **Dropped, called by nothing** (found here; not in inventory § 5): `RegisterMetrics` on `BaseService` and on `Service`
  (`base.go:389`, `:494`). The pin's own test says so: "nothing in the framework calls Service.RegisterMetrics"
  (`stream_override_expiry_test.go:220-221`). Its parameter type, `metric.MetricsRegistrar`, is gone here, replaced by
  `metric.RegisterOrGet` (ledger `metric`, `:732`). `BaseService` registers no collector of its own. It records its
  status through `CoreMetrics().RecordServiceStatus` (`metric/core.go:114`) on the injected registry, as at the pin, so
  the `metric-registry` spec holds unchanged.
- **Dropped parameter, read by nothing** (found here, § 5 (b)'s class; Q1): `NewBaseServiceWithOptions` stores its
  `cfg` argument (`base.go:85`) and never reads it. Every caller passes `nil`: 11 in the pin's `service`, semteams
  `rule_config_service.go:41` and `rule_config_service_test.go:38`, semboids `internal/api/service.go:93`. The new
  signature is `NewBaseServiceWithOptions(name string, opts ...Option)`, and `base.go` no longer imports `config`.
- **Text:** `configurable.go:13` says "The Discovery service exists"; it does not, so the sentence goes.
  `openapi_registry.go:13` says to call `RegisterOpenAPISpec` from `init()`. That breaks the "imports register nothing"
  rule (`AGENTS.md` § 1), so the text says to call it from the composition root. The pin's `init()` callers arrive
  with PRs 3 and 4, which follow the rule.
- **Kept with contract lines** (`service/README.md`, the ledger `contract` and the spec delta):
  - `BaseService`, `NewBaseServiceWithOptions`, `WithLogger`, `WithMetrics`, `WithNATS`, `Service`, `Status`:
    embedded and called by semteams (`rule_config_service.go:32`, `:41`) and semboids (`internal/api/service.go:72`,
    `:93-98`);
  - `OpenAPISpec`, `PathSpec`, `OperationSpec`, `ParameterSpec`, `ResponseSpec`, `TagSpec`, `Schema`, `HTTPHandler`:
    semboids `:171-197` (`Schema` at `:195`);
  - `MaybeStartPProf` (D3); `WireGraphRuntime` (D4; semteams `main.go:840`).

  The rest (`service_configs.go`, `schema.go`, `RegisterOpenAPISpec`, `GetAllOpenAPISpecs`) is read by later PRs'
  files (inventory § 2.2) and kept as at the pin. `ConfigSchema`, `PropertySchema` and `NewConfigSchema` are kept
  too, but their only readers are `Metrics.ConfigSchema` (`metrics.go:356`) and `MessageLogger.ConfigSchema`
  (`message_logger.go:1196`), which exist to satisfy the dropped `Configurable`. PRs 2 and 4 re-audit those two
  methods; if both go, the three names go with the last of them.
- **Options:** (a) chosen; (b) keep the dropped surface: exported names with no reader and no contract line
  (#186, #190); (c) keep `cfg`: one unread parameter in a framework signature, to spare two consumers a one-word
  edit.

## D2. BaseService: Start bounds Start, Stop is the only stop (S1, S5; ADR-113)

At the pin, the run context derives from Start's (`base.go:254`), and a goroutine stops the service when that
context ends (`:280`, `:457-460`). The § 7 probe: `BaseService` passes 7 of 8 suite checks and fails
`StartContextEndStopsNothing`. A Stop whose context ended gives up its claim, so a later Stop returns nil without
waiting ("Later calls are nil/no-op; they neither wait again nor replay a prior result", `base.go:296-298`). The
pin's `TestBaseServiceStopIsOneShotAfterFailedJoin` asserts that and passed in § 7's unit runs. The port's
observable behaviour, each item proven by `TestBaseServicePassesTheLifecycleSuite` unless another test is named:

- `Start(ctx)` refuses a nil context, an ended context (the error wraps the context's), a second Start, and a Start
  after a Stop (`errs.ErrAlreadyStarted`). A refused Start starts nothing. Once started, `Status()` is
  `StatusRunning`. The health check runs once at once and then every 30 s.
- Ending Start's context after Start returned stops nothing. `TestStartContextEndingKeepsHealthChecksRunning`
  (`synctest`): it cancels the Start context and advances the bubble's clock 60 s. Status stays running and two
  more checks have run.
- `Stop(ctx)` refuses nil. A context that has already ended stops nothing: Stop returns its error at once, the
  status stays `StatusRunning` and checks go on. This is the guard's reading (`internal/lifecycleguard/guard.go:108-110`
  returns before cleanup), and the suite's `StopReturnsItsEndedContext` accepts it.
  `TestStopWithAnEndedContextStopsNothing`.
- A Stop with a live context begins the stop: `Status()` becomes `StatusStopping` and no new check starts. It returns
  nil once the health goroutine has exited, with `StatusStopped` and `IsHealthy()` false. If its context ends while
  it waits, it returns that error with `StatusStopping`, and a later Stop waits again.
  `TestRepeatedStopWaitsForABlockedHealthCheck` replaces the pin's test. A health check blocks; a Stop whose context
  is cancelled while it waits returns `context.Canceled`. A second Stop does not return while the check blocks, then
  returns nil once the check returns. No further check runs.
- Stop before Start returns nil, and a later Start is refused, as at the pin (`base_lifecycle_test.go:113-118`,
  kept).
- A failed health check (`SetHealthCheck` returning an error), or a NATS client that is not healthy, makes
  `IsHealthy()` false, and `Health()` reports unhealthy. Status-based `Health()` is unchanged.
  `TestFailingHealthCheckMakesTheServiceUnhealthy` (`synctest`; the pin's `TestService_FailingHealthCheck` without
  its sleep).

**One stop sentinel (S5, 03B SS#1218).** `base.go:28`'s `service.ErrAlreadyStopped` goes. No `service` code returns
it. Its one reader, `service_manager.go:888`, is PR 2's and reads `errs.ErrAlreadyStopped`. The `Service.Stop` doc
says a repeated Stop returns nil. The `lifecycle-suite` spec's floor ("repeated Stop as a no-op", checked as a nil
return) decides that. So the 03B row's test wording ("a stopped service and a stopped component both return the one
`ErrAlreadyStopped`", 03B `design.md:488`; the 04A foundation design's "SS#1218 proof", `design.md:766`) cannot be
written for `BaseService`. Changing an accepted proof commitment is the owner's call (Q2). The proposed substitute:
no second sentinel here, a repeated Stop returning nil (the suite), and PR 2's StopAll test that a service whose Stop
returns `errs.ErrAlreadyStopped` counts as stopped.

**Mechanism** (developer's): recommended, compose `internal/lifecycleguard` with a run context created in Start and
cancelled in Stop, and drop `contextMonitor`, `done` and `cancel`, as `config.Manager` did. The behaviour above is
the guard's. One more observable difference it brings: a Stop while another runs is refused as transient
(`internal/lifecycleguard/guard_test.go:358`). The pin returned nil there while the first Stop was still waiting on
a blocked check (probe H6), claiming a stop that had not finished. Recorded in `known_risks`. The test
adapter's `Observe` lists `health-loop` while the channel Stop waits on is open, and leaves `Calls` empty (config
design D2's reason). `mustFail` is a `BaseService` stopped before Start.

**Adopters.** semteams' `ruleConfigService` calls `BaseService.Stop` (`rule_config_service.go:73-80`): no change
beyond D1's argument. semboids (on `beta.160`) still calls `Stop(timeout)` (`internal/api/service.go:126-128`); its
move to `Stop(ctx)` predates this PR. A service embedding `BaseService` that relied on its Start context to stop
the health loop now calls Stop, with a fresh bounded context: a Stop handed an already-ended signal context stops
nothing (above). The README says so.

## D3. The profiling server stops (S4, S9; owner Q3; `background-work`)

The new shape (`background-work/spec.md:19-22`): an HTTP server waits on in-flight requests, so `Shutdown(ctx)`. The
owner's ruling expects the three consumers to change their call. A new `(*PProfServer, error)` result would not make
them: their bare statements would still compile, never shut the server down, and drop the bind error the pin
printed. An added logger parameter does make them, and keeps the pin's "log and continue" inside the helper:

```go
// MaybeStartPProf serves http.DefaultServeMux on :port when debug && port > 0. A bind failure is
// logged at Warn on logger and returns nil; profiling is a debug aid, never a boot gate.
func MaybeStartPProf(debug bool, port int, logger *slog.Logger) *PProfServer

// Shutdown stops the server and waits for its requests and goroutine.
func (s *PProfServer) Shutdown(ctx context.Context) error
```

- Disabled (`!debug || port <= 0`): returns nil and starts nothing. `TestMaybeStartPProfDisabledStartsNothing`.
- Enabled: binds `:port` before returning. A bind failure is logged at Warn naming the port and the error, and nil is
  returned with nothing started: the pin's posture ("a best-effort debug aid degrading, never a boot gate",
  `pprof.go:23-24`), with the pin's two `fmt.Printf` lines replaced by the logger. A nil logger means
  `slog.Default()`, as in `kvcatalog.EnsureCatalogRetentionClean`. `TestMaybeStartPProfLogsABindFailure`: the test
  holds `net.Listen("tcp", ":0")`, passes its port and a recording logger, and requires one Warn naming the port and a
  nil result. Probe H1: `bind: address already in use` on darwin and linux.
- `Shutdown(ctx)` refuses a nil context first, before anything else and on a nil server too ("at the call that
  receives it", `background-work`). A nil server's Shutdown with a context returns nil, so a caller may defer
  Shutdown on whatever it got. Otherwise Shutdown stops accepting and returns nil once every in-flight request has
  finished and the serving goroutine has exited. If `ctx` ends first it returns that error, and a later Shutdown waits
  again. A serve error other than `http.ErrServerClosed` is returned by Shutdown.
  `TestPProfShutdownWaitsForAnInFlightRequest` holds a request in a blocking handler. Shutdown with an ended context
  returns `context.Canceled`. After the handler returns, Shutdown returns nil (probes H2 to H4).
  `TestPProfShutdownRefusesANilContext` covers a started server and a nil one.
- Nothing left behind (`background-work`, "Stopped inside a bubble"):
  `TestPProfServesTheIndexAndLeavesNothingRunning` serves through `startPProf` on a `127.0.0.1:0` listener inside
  `synctest.Test` (probe H5). `GET /debug/pprof/` returns 200 (blank `net/http/pprof` import in the test, as at the
  pin), and Shutdown returns nil.
- **Seam (S9, Bounded port scope).** The pin's unexported `startPProf(debug, port, server, listen)`
  (`pprof.go:32`) becomes `startPProf(server *http.Server, listener net.Listener) *PProfServer`. The `listen`
  function goes: the test hands in a listener, as `scripts/lint-test-ports.sh` asks. `server` is the pin's own seam,
  kept so the in-flight test can give it a blocking handler. Production passes `&http.Server{}` and the bound
  listener.
- **ADR-058 kept:** not a `Service`, called early by the composition root.
- **Contract line** (README, ledger, spec). semsource `cmd/semsource/run.go:109`, semteams `cmd/semteams/main.go:93`
  and semboids `cmd/semboids/main.go:100` each change their one line to the first statement below, keeping their own
  `debug` and `port` arguments, and add the second on their exit path, after their services have stopped:

```go
pprofServer := service.MaybeStartPProf(debug, port, logger)

stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := pprofServer.Shutdown(stopCtx); err != nil {
    logger.Warn("pprof shutdown", "error", err)
}
```

## D4. Graph runtime (ADR-068 D1, ADR-091, ADR-110)

`WireGraphRuntime(ctx, client, logger, contracts...)` keeps its signature (semteams `main.go:840`). It runs the
catalog-retention backstop, now `kvcatalog.EnsureCatalogRetentionClean`, which refuses a nil context or client
(`graph/kvcatalog/owned_bucket_retention.go:39-48`). Then it builds `projection.NewMutationClient`. It starts
nothing (ADR-091); its comment (`graph_runtime.go:13-15`) stays.

- `TestWireGraphRuntimeStripsAForeignTTL` (integration): the `EMBEDDING_INDEX` bucket is created with a 7-day TTL and
  a key, as in `TestIntegration_EnsureCatalogRetentionClean_StripsForeignTTL`. `WireGraphRuntime` returns a client;
  the bucket's TTL is 0; the key survives.
- `TestWireGraphRuntimeRefusesANilContext`: an invalid-class error, and no client.

## D5. Service configuration helpers (S6)

`service_configs.go` is carried unchanged. Its strict decoder, `decodeStrictServiceJSON` (`:130-139`), has no
caller until PR 2. A strict decoder accepts a key in another letter case (inventory S6 probe). #341 has merged
(`ea60ae4`): `config` now refuses letter-case variants through `rejectCaseVariantKeys` (`config/config.go:732`,
unexported), and its spec leaves "the contents of a component's or service's `config` payload" to the owner that
decodes it (`openspec/specs/config-loading/spec.md:79`). PR 1 does nothing with the checker: no service configuration
shape exists here. PR 2, which declares the first one (`service_manager_config.go`), adopts it and decides how
`service` reaches it without a second copy (one interpreter per fact). Task 3.3 files that issue for PR 2. The plain
decoders of S6 are PR 5's and PR 6's.

## D6. Tests

- **Carried and adapted:** `base_lifecycle_test.go` (unit) and `base_test.go`. In `base_test.go`:
  - the tests that need no NATS become unit tests;
  - `TestService_HealthMonitoring` stays integration;
  - the five `time.Sleep` lines (`:36`, `:48`, `:103`, `:197`, `:284`) and the polling helpers give way to
    `synctest` or `probe.Await`;
  - the dropped options' subtests go;
  - `TestService_ContextCancellation` and `TestBaseServiceParentCancellationConvergesAndJoins` assert the pin's abort
    (Stopped after the Start context ends). They become D2's `TestStartContextEndingKeepsHealthChecksRunning`;
  - `TestBaseServiceStopIsOneShotAfterFailedJoin` becomes D2's `TestRepeatedStopWaitsForABlockedHealthCheck`.
- **Carried and rewritten:** the pin's `pprof_test.go` (inventory § 2.3 placed it in PR 5, for the `waitForListener`
  helper in a storage test file). Its two tests become D3's `TestMaybeStartPProfDisabledStartsNothing` and
  `TestPProfServesTheIndexAndLeavesNothingRunning`, which need no such helper.
- **Moved in:** `TestResolveServiceConfigsIsPureAndCanonical` and `TestPendingServiceChangesClassifications` (pin
  `service_composition_test.go:233-273`, `:351-392`) go to `service_configs_test.go`. They read nothing else.
- **Not carried:**
  - `main_test.go`'s `NewSharedTestClient` (#195) gives way to a `natsfixture.Shared` file with `TestMain`, as
    `config/fixture_client_integration_test.go:18-60`;
  - `suite_integration_test.go` repeats `TestService_Creation` and `TestService_Lifecycle` plus a sleep (`:122`);
  - `kv_test_helpers_test.go` and `kv_test_helpers_integration_test.go`: no test uses `KVTestHelper` but its own
    suite (`grep`), so its sleep (`:121`) goes with it;
  - `test_helpers_integration_test.go`'s `safeResponseRecorder` moves to PR 2 with its one user,
    `metrics_runtime_integration_test.go`.
- **New:**
  - D2's suite adapter, its two `synctest` tests and `TestStopWithAnEndedContextStopsNothing`;
  - D3's other three tests;
  - D4's two tests;
  - `TestDecodeStrictServiceJSON` (unknown key, trailing value, empty input);
  - `TestCanonicalServiceJSONIsIdempotent` (Generated checks);
  - `TestSchemaFromType` (one case per kind `schema.go` handles: primitives, struct tags, slice, map, pointer,
    `time.Time`);
  - `TestOpenAPIRegistryReturnsACopy`; `TestNewConfigSchema`.
- **Harness rules:**
  - no `time.Sleep`, `Skip`, fixed port, or `t.Parallel` in an integration file;
  - every Stop or Shutdown in a cleanup runs under `context.WithTimeout(context.WithoutCancel(t.Context()), ...)`;
  - integration tests reach the shared server only through the reset.
- **Sensitivity** (task 2.3; unit mutants through `task mutate:check`, the integration one by hand, #129):

  | Mutant | Caught by |
  | --- | --- |
  | run context derived from Start's | `TestStartContextEndingKeepsHealthChecksRunning` and the suite |
  | Stop returns nil although its context ended | the suite's `StopReturnsItsEndedContext` |
  | a later Stop returns at once | `TestRepeatedStopWaitsForABlockedHealthCheck` |
  | Shutdown returns before the in-flight request | `TestPProfShutdownWaitsForAnInFlightRequest` |
  | the bind moved back into the goroutine, or its failure not logged | `TestMaybeStartPProfLogsABindFailure` |
  | the retention call removed | `TestWireGraphRuntimeStripsAForeignTTL` |

## Ruling to implementation

| Ruling | Implementation | Proving test |
| --- | --- | --- |
| Owner 2026-10-20 Q1 (#9 comment 6296023694), option B, PR 1's files | the files of D1 | review of the file list; `go build ./service/` |
| Owner 2026-10-20 Q3; `background-work` | `pprof.go` (D3) | D3's five tests; the consumers' old call no longer compiles |
| ADR-113; `lifecycle-suite` floor | `base.go` Start/Stop (D2) | `TestBaseServicePassesTheLifecycleSuite`, `TestStartContextEndingKeepsHealthChecksRunning`, `TestStopWithAnEndedContextStopsNothing`, `TestRepeatedStopWaitsForABlockedHealthCheck` |
| 03B SS#1218, one `ErrAlreadyStopped`; Q2 | `base.go:28` removed; `Service.Stop` doc | build; the suite's `RepeatedStopIsNoOp`; PR 2's StopAll test |
| `metric-registry` spec; ledger `metric` (`RegisterOrGet`) | `RegisterMetrics` dropped (D1) | build; `TestNoProcessGlobalRegistration` (existing) |
| Surface audit (#9 comment 5968830525, #190); Q1 | D1's drops | `go doc ./service` lists only the kept surface |
| ADR-068 D1, ADR-091, ADR-110 | `graph_runtime.go` (D4) | `TestWireGraphRuntimeStripsAForeignTTL`, `TestWireGraphRuntimeRefusesANilContext` |
| #195; harness rules | test files (D6) | `task verify` guards |
| Owner 2026-10-19, simple fixes | `configurable.go`, `openapi_registry.go` text | review |

## Generated checks

Decided per `docs/testing.md` "Decide whether generated checks are needed":

- **BaseService lifecycle** (a history: Start, Stop, context endings, a blocking check). Examples suffice when the
  guard is composed. The suite's eight checks enumerate the orderings of Start, Stop and context ending, and D2's
  named tests add the ended-context, blocked-check and clock cases. The guard's state machine has its own tests
  (`internal/lifecycleguard/guard_test.go`). If the developer keeps a hand-written state machine instead, the PR adds
  a generated history check against a model of Start/Stop/cancel/block and records it in a note.
- **`canonicalServiceJSON`** (a normalization law). `TestCanonicalServiceJSONIsIdempotent` uses `rapid`, generating
  nested JSON objects with strings, `json.Number` values, booleans, nulls, arrays and objects. It checks that
  canonicalizing twice equals canonicalizing once. It also checks that the canonical text decodes, with
  `UseNumber`, to a value deep-equal to the input's decoded value: an oracle that does not call the code under test.
  The run records its seed and check count.
- **The profiling server** (Shutdown before and after a request ends): the spec's two scenarios are the two
  orderings. **`SchemaFromType`**: no stated law; one example per kind.

## D7. Ledger, text, guidance, coverage

- **Ledger:** a new `service` row (`adapt`), appended. PRs 2 to 7 edit it in place, as `config`'s row was. Plus a
  `docs/adr/058-boot-lifecycle-phases.md` row, in the ADR-102 row's shape.

```yaml
- source_path: service
  source_sha: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128
  consumer_purpose: >-
    the service layer of the boot (change 3), ported in seven pull requests by lifecycle owner (#9 comment
    6296023694). PR 1: BaseService and the Service interface every service embeds, also embedded by semteams
    (cmd/semteams/rule_config_service.go:32) and semboids (internal/api/service.go:72); the OpenAPI types semboids
    builds (:171-197); MaybeStartPProf, called by semsource, semteams and semboids; WireGraphRuntime, called by
    semteams (cmd/semteams/main.go:840); the service-configuration, schema and OpenAPI registry helpers the later
    PRs read
  destination: service
  contract: >-
    the service-lifecycle spec; ADR-113; the background-work spec (PProfServer.Shutdown); pin ADR-058 as carried
    (pprof is not a Service and now stops); pin ADR-068 D1 and ADR-091 as graph/kvcatalog and
    internal/graphmutation implement them; ADR-110 (service writes no ENTITY_STATES entry); service/README.md for
    the calls semsource, semteams and semboids make
  dependencies_and_side_effects: >-
    component, graph/kvcatalog, health, internal/lifecycleguard, metric, natsclient, pkg/errs, pkg/projection, types
    (all in tree); BaseService runs one health goroutine, joined in Stop; MaybeStartPProf binds :port on every
    interface and serves http.DefaultServeMux on a goroutine joined in Shutdown; WireGraphRuntime may rewrite the
    retention of inactive catalog buckets; the OpenAPI registry is process-global
  known_risks: >-
    PR 1 of 7 (base.go, pprof.go, graph_runtime.go, service_configs.go, schema.go, openapi.go, openapi_types.go,
    openapi_registry.go, configurable.go, doc.go, README.md); the managers and services follow. Dropped as unread:
    WithHealthCheck, WithHealthInterval, OnHealthChange (option and method), RegisterMetrics (BaseService and
    Service), NewOpenAPISpec, AddPath, AddTag, GetOpenAPISpec, Configurable, service.ErrAlreadyStopped (one sentinel,
    errs.ErrAlreadyStopped), and NewBaseServiceWithOptions' unread config argument (Q1). Changed from the pin: ending
    Start's context stops nothing and a later Stop waits again (ADR-113); a Stop during another Stop is refused as
    transient (lifecycleguard), where the pin returned nil before the first Stop finished; a Stop given an ended
    context stops nothing; MaybeStartPProf takes a logger, binds before it returns, logs a bind failure at Warn
    instead of printing it, and stops through Shutdown (owner 2026-10-20). Retained: pprof listens on all
    interfaces; letter-case variants of service config keys are accepted until PR 2 adopts config's checker (issue
    filed); service_configs.go, schema.go and openapi.go are byte-equal to the pin
  proving_tests: service/*_test.go, service/*_integration_test.go
  owner: setup-04a-03-boot-service-base developer
  disposition: adapt
```

- **`docs/repository-map.md`** gains a row: "| `service/` | Ported from the pin in seven pull requests by lifecycle
  owner; `setup-04a-03-boot-service-base` (PR 1) lands `BaseService` and the `Service` interface (Start bounds
  Start, Stop is the only stop, a health check every 30 s), `MaybeStartPProf`, which serves the profiling handlers
  until `Shutdown`, `WireGraphRuntime`, and the OpenAPI types and service-configuration helpers the managers read.
  The service and component managers and the services follow in PRs 2 to 7 |".
- **Text:** `service/doc.go` and `service/README.md` describe only this surface and compile against it. The README
  sections cover embedding `BaseService` (`NewBaseServiceWithOptions(name, opts...)`, Stop with a fresh bounded
  context), `MaybeStartPProf` then `Shutdown`, and `WireGraphRuntime`. The pin README's `RegisterConstructor`
  (`:202-204`), `discovery` service and `read_timeout` keys (`:328-345`) and seven absent sentinels (`:354-369`)
  are not carried.
- **Guidance:** no pin contract section or skill names this PR's surface (`grep` of the pin's `.agents`,
  `.claude` and `AGENTS.md`). One sentence becomes false here: "no OpenAPI registry" in
  `.agents/contracts/semengine-developer.md:302-303` and `semengine-reviewer.md:327-328`. Both say instead that
  `service` now has one, and that the pin's `SchemaRef` registration rule arrives with the first service that
  registers a spec (PR 3).
- **Coverage:** `"service merged"` in `scripts/cover-check.sh` (03B D10 lists `service`). Not measured here: the PR
  1 subset needs D2 and D3 before it builds. Task 3.2 records the number.

**Issues:** none open names a PR 1 file except #195 (`main_test.go`'s shared client), which stays open for PRs 3 to
5. #9 stays (`Addresses #9`). **Merge order:** #341 is merged and this design is written against it. #342
(`composition`) is still at design, a draft with only OpenSpec files; PR 1 needs nothing from it and is expected to
merge first. #342's implementation then merges `origin/main` and keeps PR 1's ledger, map and cover-check lines
beside its own. PR 2 follows this one; PR 3 waits for #342.

**Probes** (go1.26.9 on darwin/arm64 and in the `golang:1.26.9` image, linux/arm64; source, commands and output
in the probe comment on #343, which task 2.1 cites):

- **H1:** a second `net.Listen("tcp", ":p")` while `:p` is held fails `bind: address already in use`.
- **H2:** `http.Server.Shutdown` under a blocked handler returns `context.DeadlineExceeded` at its 100 ms bound.
- **H3:** `Serve` has already returned `http: Server closed` while the handler still blocks.
- **H4:** a second Shutdown after the handler returns gives nil.
- **H5:** a loopback HTTP server and client inside `synctest.Test`, shut down, leave the bubble: 20 runs with
  `-race` on each platform.
- **H6** (a scratch copy of the pin, darwin only): a second `BaseService.Stop` while the first waits on a blocked
  health check returns nil with the status `stopping`; the first returns only after the check is released.

## Owner questions

- **Q1. Recommended: yes.** May `NewBaseServiceWithOptions` lose the configuration argument that it never reads and
  every caller passes as `nil`, so semteams deletes `nil,` from two calls and semboids from one? (D1; `base.go:85`,
  `:118`; semteams `rule_config_service.go:41` and `rule_config_service_test.go:38`, semboids
  `internal/api/service.go:93`)
- **Q2. Recommended: yes.** May the promised check that a stopped service and a stopped component both return the
  one `ErrAlreadyStopped` be replaced by this: no second sentinel in `service`, a repeated Stop that returns nil as
  the lifecycle suite requires, and PR 2's test that a service whose Stop returns `errs.ErrAlreadyStopped` counts as
  stopped? (D2; 03B `design.md:488`; 04A foundation `design.md:766`; `lifecycle-suite`, "Portable floor")
