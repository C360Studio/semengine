# Design: setup-04a-03-boot-service-heartbeat-metrics-forwarder

base: `ed174de3adbbdd46ac51e83ef9fd4e14e74e646a` (draft PR #383). Inputs: `sweep.md` (this change; H-ids and
S-ids), the whole-package inventory (`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md`)
and PR 5's archived design (`openspec/changes/archive/2026-10-23-setup-04a-03-boot-service-storage-observability/
design.md`), whose shape this copies. Pin lines are SemStreams `8b99efe9`, `service/`; `hb:n` is `heartbeat.go`, `mf:n`
`metrics_forwarder.go`. Two lifecycle owners, 751 non-test pin lines, in one pull request as the accepted split says
(inventory § 2.3 row 6; owner, #9 comment 6296023694, option B).

Owner intent applied, not re-asked: ADR-113 (context ownership and lifecycle control); no context-value bags; no back
doors on production types; natsclient passes nats.go through; no SemStreams state migration; simple idiomatic Go;
port whole, drop only dead surface; every real defect found and not fixed is filed; S6 takes the simple fix, the strict
decoder already in the package (owner 2026-10-19).

## Inventory addendum

- **Callers outside `service`.** Pin, Go: none for any exported name of these files (`sweep.md` H4). Consumers reach
  both only through configuration: semsource enables `heartbeat` with `{}` (`cmd/semsource/run.go:1117-1120`);
  semteams enables `metrics-forwarder` with `push_interval`, `include_go_metrics`, `include_proc_metrics`
  (`configs/flow-bootstrap.json:198-205`). The surface reached from outside is the two configurations, the heartbeat
  log record and the samples on `metrics.<component>.<name>` (the `METRICS` stream, `config/streams.go:123`).
- **How each is reached today.** `RegisterAll` registers neither (`service/register.go:19-24`). `config`'s defaults
  enable `heartbeat` with `{"interval": "30s"}` (`config/config.go:495-498`), so a composition built from a loaded
  document with `RegisterAll` is refused today: "create configured service heartbeat: no constructor registered for
  service heartbeat" (`sweep.md` § 3, second tree probe). `metrics-forwarder` is not materialized by default
  (`service/service_configs.go:85-89`).
- **Open pull requests and issues:** `sweep.md` § 4. This PR retires #344.

## D1. Files and surface (surface audit, #190)

- **Carried, adapted:** `heartbeat.go` and `metrics_forwarder.go`, module path rewritten.
- **Dropped, no reader** (pin-wide and consumer search, `sweep.md` H4):
  - `MetricsGatherer` (S10): the forwarder gathers from `deps.MetricsRegistry.PrometheusRegistry()`, the one
    production gatherer (`mf:172-175`)
  - `HeartbeatConfig.Validate`, `MetricsForwarderConfig.Validate` (H4): folded into unexported `applied()` methods
  - the four test-only constructors (S9) and the `natsPublisher` assertion (`mf:73-77`; S10): the forwarder holds the
    `*natsclient.Client` it is given
  - `componentHealthGetter` (S10): `Start` asserts `*ComponentManager` (D2), the one implementation
  - the `config` and `startTime` fields (H4, H5) and the two file-level package comments (H9).
- **Kept with contract lines** (README, ledger `contract`, D9): `NewHeartbeatService`, `NewMetricsForwarderService`
  (through `RegisterAll`); `HeartbeatConfig` (`interval`) and `MetricsForwarderConfig` (`push_interval`,
  `include_go_metrics`, `include_proc_metrics`); `HeartbeatService` and `MetricsForwarder` with the `Service` methods
  `Start`, `Stop`, `Health`.
- **Logger (H1).** Each service logs through `deps.Logger` with `service=<name>`, or `slog.Default()` when none is
  given, as PR 5 does (`service/storage_observability.go:205-208`).
- **Text (H9):** the publish loop's comment says why the first round runs at once (a reader that joins between
  `Start` and the first tick finds the stream empty), without the SemStreams note; "application metrics" are
  `semengine_*`.
- **`RegisterAll`** registers `heartbeat` and `metrics-forwarder`. Neither serves a route, so no OpenAPI spec.
- **Options for the gatherer and publisher:** (a) concrete dependencies, chosen: production has one of each, and the
  tests reach both through the registry and client a composition passes; (b) keep the interfaces: the tests would set
  them through test-only constructors, which the back-door rule removes.

## D2. Lifecycle: Start bounds Start; Stop is the only stop (both owners)

Governed by ADR-113. Each service composes `internal/lifecycleguard` (S3; ADR-108 Decision 2; every owner ported so far
composes it); the pin's `ticker`, `stopChan`, `stopOnce`, `wg` and `loopDone` go. Observable behaviour, each with its
proving test (D6 units in brackets):

- **Refusals.** `Start` refuses a nil or ended context, a second `Start`, and a `Start` after any `Stop`
  (`errs.ErrAlreadyStarted`); a refused `Start` holds nothing. `Stop` refuses nil; given an ended context it returns
  that context's error and stops nothing. Proof: `TestHeartbeatPassesTheLifecycleSuite`,
  `TestMetricsForwarderPassesTheLifecycleSuite` [U-a]; must-fail owner = a service stopped before `Start`.
- **The heartbeat's Start needs the component manager.** `Start` looks up `componentManagerName` through the
  `ServiceManager` it was constructed with and refuses, naming `component-manager`, when no service has that name or
  the service is not the framework's `*ComponentManager` (S12; `hb:147-155`). The lookup stays at `Start`: `StartAll`
  may create the component manager after the heartbeat was constructed (`service/service_manager.go:411-425`). The
  manager does not need to be started; its health read waits on nothing
  (`service/component_manager.go:558-567`). Proof: `TestHeartbeatStartRefusesWithoutTheComponentManager` [U-b]
  (no service named `component-manager`; `newStandInComponentManager()`, `service/service_manager_test.go:63-65`),
  each refusal leaving the status stopped.
- **Startup work.** `Start` starts the base service, then one loop, and returns nil; the forwarder's `Start` does no
  I/O. The loop runs under the one run context `Start` creates, kept only as a cancel (ADR-113;
  `service/storage_observability.go:281` precedent). Its first round runs at once, then one per interval.
- **Ending the Start context after `Start` returned stops nothing.** The status stays running and the loop keeps its
  cadence (probe, `sweep.md` § 3: the pin fails, H6). Proof: suite `StartContextEndStopsNothing` [U-a];
  `TestMetricsForwarderKeepsPublishingAfterStartContextEnds` [U-e].
- **Stop.** `Stop` ends the run context, waits for the loop to return and stops the base service, and returns nil only
  once both are done. If its context ends first it returns that error, and the next `Stop` waits again (the pin's
  behaviour, `hb:201-206`, `mf:251-256`, kept). A `Stop` after one that returned nil returns nil and calls nothing; a
  `Stop` while another runs is refused as transient (the guard). How soon a publish in progress sees the run context
  end depends on nats.go, which does not promise it; `Stop` stays bounded by its own context either way. Proofs:
  `TestHeartbeatFailedStopIsRetried` [U-b], `TestMetricsForwarderFailedStopIsRetried` [U-c]: the composition's
  `slog` handler (PR 4's `holdingHandler`, `service/message_logger_helpers_test.go:49-124`, a production dependency)
  holds the loop's first record ("System heartbeat"; "failed to publish metrics to NATS" over a client that never
  connects), so the loop is inside its log call:
  - the first `Stop`, under a 200 ms deadline, returns `context.DeadlineExceeded`
  - a second `Stop`, on a goroutine, has not returned within a 200 ms window while the record is held
  - after release, the second `Stop` returns nil, a third returns nil, and the suite's observation shows no loop.
- **Context rules:** neither service keeps a `context.Context`; each holds one private cancel. Locks, the join and its
  order are the developer's (failing-first under `-race`).

## D3. Configuration and construction (#344)

Both constructors decode with `decodeStrictServiceJSON` (`service/service_configs.go:136-152`, S6): an unknown key and
a key in another letter case are refused, naming the key. That retires #344: the loader's merge turns
`{"Interval":"5s"}` into `{"Interval":"5s","interval":"30s"}`, which the strict decoder refuses with "config key
Interval must be written interval" (`sweep.md` § 3, tree probe), so the operator's value is never silently replaced.
No change to `config`: the merge leaves service payloads to their owner (`openspec/specs/config-loading/spec.md:77-79`).

One duration parse for every service (H3): PR 5's `storageDuration` (`service/storage_observability.go:137-149`)
becomes `serviceDuration(service, field, value string, fallback time.Duration)` in `service/service_configs.go`, with
the same messages, and the three services call it. An empty or absent duration takes the default; one that does not
parse or is not positive is refused. The forwarder's second decode (`mf:33-44`) goes, so `"push_interval": ""` now
means the default, as `interval` already does for the heartbeat and storage observability.

Refusals, each as invalid data (`errs.WrapInvalid`, H10), naming the input:

- heartbeat: nil `deps` or nil `deps.ServiceManager` (H12; new); `interval` that does not parse, is not positive, or
  is under 1 s (`hb:30-39`)
- forwarder: nil `deps` (H2; new), nil `deps.NATSClient`, nil `deps.MetricsRegistry` (`mf:57-62`); `push_interval`
  that does not parse or is not positive.

Defaults: `interval` 30s, `push_interval` 5s, `include_go_metrics` and `include_proc_metrics` false. Each service keeps
only the applied values (H4). Configuration is applied by restart.

Proofs [U-b, U-c]: `TestHeartbeatConstructionRefusals`, `TestMetricsForwarderConstructionRefusals` (one subtest per
refusal, each asserting `errs.IsInvalid` and the named key or field; the pin's tables `heartbeat_test.go:59-157`,
`metrics_forwarder_test.go:27-186`, `:971-1022`, with the empty-string row now taking the default);
`TestHeartbeatRefusesAMiscasedKeyBesideTheDefault` [U-b]: the document `{"platform":{"org":"acme","id":"p1"},
"services":{"heartbeat":{"enabled":true,"config":{"Interval":"5s"}}}}` loaded with `config.NewLoader().LoadFromBytes`,
then `ConfigureFromServices` with `RegisterAll`, refused with an error containing `config key Interval must be written
interval`; `TestHeartbeatAndMetricsForwarderAreRegistered` [U-b].

## D4. What each service emits

- **Heartbeat.** At once and then every `interval`, one record "System heartbeat" with `uptime` (the base service's,
  rounded to the second, H5), `goroutines`, `components_healthy` and `components_total` (from
  `GetComponentHealth`; healthy means `HealthStatus.Healthy`), at Info (owner, Q1, 2026-10-23; the pin's is Debug,
  H11). Proofs [U-b]: `TestHeartbeatRecordsComponentHealth` (pin `heartbeat_test.go:379-459` on the production
  path: a manager holding `newTestComponentManager(t, ...)` with one healthy and one unhealthy component, the record
  read from `deps.Logger`, not a swapped field; asserts the level); `TestHeartbeatCadence` (inside `synctest`,
  `interval` 1s: one record at `Start` and one per second).
- **Forwarder.** At once and then every `push_interval`, it gathers the registry and publishes, for each sample of each
  family, one JSON object `{timestamp, name, component, type, value, labels}` on `metrics.<component>.<name>` with
  `PublishToStream` under the run context. `component` is the sample's `component` label, else `system`; `value` is the
  counter, gauge or untyped value, or the sample count of a histogram or summary; `type` is the family's type in lower
  case, `untyped` when absent; families named `go_*` and `process_*` are left out unless `include_go_metrics` or
  `include_proc_metrics` (`mf:305-426`, unchanged in what they produce). The mapping is one plain function of the
  gathered families, the two flags and the time, `metricSamples` (unexported), which the publish loop calls.
- **Drops are declared (S8), as the health publishers do** (`service-lifecycle`, "The service manager publishes health
  and counts its drops"): a sample that does not encode (a NaN or infinite value) or does not publish increments
  `semengine_errors_total{service="metrics-forwarder",type="metrics_publish"}`, a failed gather increments
  `type="metrics_gather"` and skips that round (H7, retained), each logged at Debug naming the subject or the error;
  the loop goes on. H8 (a label value spliced into the subject) is retained: such a publish is counted like any other.
- **Proofs:** [U-d] the pin's mapping tests (`metrics_forwarder_test.go:273-607`, `:849-969`) as tables over
  `metricSamples`; [U-c] `TestMetricsForwarderCadence` (inside `synctest`, `push_interval` 100ms over a client that
  never connects: the failed publishes of the test counter's subject number one at `Start` plus one per tick),
  `TestMetricsForwarderIncludeFlagsReachThePublisher` (a `go_` and a `process_` counter registered on the injected
  registry; their subjects are attempted only with the flag set), `TestMetricsForwarderPublishFailureIsCounted`,
  `TestMetricsForwarderGatherFailureIsCounted` (a collector registered on the injected registry that reports an
  invalid metric); [U-e] `TestMetricsForwarderPublishesEachSample` (a counter labelled `component=probe`, the payload
  read from the `METRICS` stream), `TestMetricsForwarderCountsAnUnencodableSample` (a NaN gauge counted, the other
  samples delivered).

## D5. Composition

`RegisterAll` gains the two constructors. With it, a composition built from a loaded document's default services
constructs the heartbeat (30 s) and no forwarder. Proof: `TestHeartbeatComposedFromTheConfigDefaults` [U-b] (the
document `{"platform":{"org":"acme","id":"p1"}}`, `ConfigureFromServices` with `testComponentDependencies`,
`service/component_manager_helpers_test.go:77-99`; returns nil and `GetService("heartbeat")` finds the service). Start
order does not matter: the heartbeat needs the component manager composed, not started (D2).

## D6. Tests

| Pin test file (lines) | Here |
| --- | --- |
| `heartbeat_test.go` (470) | the mock getter `:17-26` dropped (S10); config and constructor `:59-157` → U-b; lifecycle `:159-257` → U-a (the suite) and U-b, each on the production constructor; `:258-345` → U-b; `:347-377` (the pin's self-stop) replaced by the suite's `StartContextEndStopsNothing`; `:379-470` → U-b through `deps.Logger` |
| `metrics_forwarder_test.go` (1,091) | config `:27-186`, `:971-1022` → U-c; lifecycle `:188-217`, `:697-809` → U-a and U-c (`:761-781`, the pin's self-stop, replaced by the suite); cadence `:218-271` → U-c; mapping `:273-607`, `:849-969` → U-d; errors `:608-640`, `:810-848` → U-c; concurrent gather `:641-696` → U-e (`-race` over the real wire); mocks `:1024-1091` dropped |

**Units** (one or two files each; a brief cites only the D-sections and line ranges named). Order: U-a first, because
the others build the services through it; then U-b to U-e in any order. Every unit builds a service the production way,
through U-a's helpers, except U-d (a pure function).

- **U-a** `service/heartbeat_metrics_forwarder_helpers_test.go`, `service/heartbeat_metrics_forwarder_suite_test.go`
  (both new, no build tag). D2.
  - `newTestHeartbeat(t, config string, logger *slog.Logger, components ...testComponent) *HeartbeatService`: a
    `Manager` (`NewServiceManager(NewServiceRegistry())`) holding `newTestComponentManager(t, components...)` as
    `component-manager` (`RegisterInstance`), `heartbeat` registered with `NewHeartbeatService`, then `CreateService`.
  - `newTestMetricsForwarder(t, client *natsclient.Client, config string, logger *slog.Logger)
    (*MetricsForwarder, *metric.MetricsRegistry)`: `NewMetricsForwarderService` with a fresh registry, then the type
    assertion.
  - Suite adapters as PR 5's (`service/storage_observability_suite_test.go`): `Observe` reports `base-status=<status>`
    while neither stopped nor starting, and `loop` while the loop has not returned, read from the unexported field the
    join waits on (no method added). The forwarder over `newUnconnectedClient(t)`.
  - Calls: `NewHeartbeatService`, `NewMetricsForwarderService`, `Start`, `Stop`, `Status`, `NewServiceManager`,
    `RegisterInstance`, `CreateService`, `newTestComponentManager`, `newUnconnectedClient`, `stopContext`,
    `lifecycletest.Run`.
- **U-b** `service/heartbeat_test.go`; the type list of `internal/keycase/guard_test.go:23-27` gains
  `service.HeartbeatConfig` and `service.MetricsForwarderConfig`. D2, D3, D4 (heartbeat), D5.
  - `TestHeartbeatConstructionRefusals`, `TestHeartbeatRefusesAMiscasedKeyBesideTheDefault`,
    `TestHeartbeatAndMetricsForwarderAreRegistered`, `TestHeartbeatStartRefusesWithoutTheComponentManager`,
    `TestHeartbeatFailedStopIsRetried`, `TestHeartbeatRecordsComponentHealth`, `TestHeartbeatCadence`,
    `TestHeartbeatComposedFromTheConfigDefaults`.
  - Calls: the U-a helper, `config.NewLoader`, `LoadFromBytes`, `ConfigureFromServices`, `RegisterAll`,
    `GetService`, `testComponentDependencies`, `newStandInComponentManager`, `newHoldingHandler`, `heldRecord`,
    `releaseHeld`, `errs.IsInvalid`, `synctest.Test`.
- **U-c** `service/metrics_forwarder_test.go`. D2, D3, D4 (forwarder).
  - `TestMetricsForwarderConstructionRefusals`, `TestMetricsForwarderFailedStopIsRetried`,
    `TestMetricsForwarderCadence`, `TestMetricsForwarderIncludeFlagsReachThePublisher`,
    `TestMetricsForwarderPublishFailureIsCounted`, `TestMetricsForwarderGatherFailureIsCounted`.
  - Calls: the U-a helper over `newUnconnectedClient(t)`, `PrometheusRegistry().MustRegister`,
    `prometheus.NewCounter`, `prometheus.NewInvalidMetric`, `requireGauge` (`service/gauge_helpers_test.go:50`; it
    needs a clean gather, so the gather test unregisters its failing collector after `Stop`, before reading),
    `newHoldingHandler`, `synctest.Test`.
- **U-d** `service/metrics_forwarder_samples_test.go` (new). D4.
  - The pin's mapping tests as tables over `metricSamples(families []*dto.MetricFamily, includeGo, includeProc bool,
    now time.Time) []metricSample`, `metricSample` holding the subject and the six payload fields.
- **U-e** `service/metrics_forwarder_integration_test.go` (new, `integration` tag). D2, D4.
  - `TestMetricsForwarderPublishesEachSample`, `TestMetricsForwarderCountsAnUnencodableSample`,
    `TestMetricsForwarderKeepsPublishingAfterStartContextEnds` (`push_interval` 1s; `Start` under a context the test
    cancels; then the counter is incremented and the test waits, bounded at 30 s, for a sample carrying the new value).
  - Calls: `fixtureClient`, `natsclient.Client.JetStream`, `jetstream.CreateStream` (`METRICS`, `metrics.>`), a
    consumer reading the stream, the U-a helper.

**Port units** (task 2.2), each ending with `go test -race ./service/` green on the tests it turns green and `task
verify` at the end of P-2:

- **P-1** `service/heartbeat.go`, `serviceDuration` in `service/service_configs.go` (storage observability's two
  calls switched), the `heartbeat` line of `RegisterAll`. D1-D5. U-a's heartbeat half and U-b turn green.
- **P-2** `service/metrics_forwarder.go`, the `metrics-forwarder` line of `RegisterAll` (its doc comment names both).
  D1-D4. U-a's forwarder half, U-c, U-d and U-e turn green.

Harness rules as PR 5's D7: no `time.Sleep`, `Skip`, fixed port, or `t.Parallel` in an integration file; cleanup stops
under `context.WithTimeout(context.WithoutCancel(t.Context()), ...)`; every NATS connection through `fixtureClient`
(#195). Real-clock windows, listed as `docs/testing.md` requires: the two 200 ms windows of each `FailedStopIsRetried`
test; the 1 s interval of U-e, whose waits are bounded at 30 s. The cadence tests run on `synctest`'s clock; if a
dependency composed inside the bubble starts work the bubble cannot settle, the developer records it in
`notes/2.1.md` and moves that test to a real-clock bound. Generated checks: none. The mapping is a table of independent
cases with no interacting inputs or history, and Start/Stop histories are the guard's (`internal/lifecycleguard`) plus
the suite; examples suffice.

**Sensitivity** (task 2.3). Unit rows go through `task mutate:check`; integration rows (`I`) as PR 5 ran them. A row
counts as a detection only when the test fails under the mutant; a hang or timeout is inconclusive.

| Mutant | Caught by |
| --- | --- |
| a loop on the Start context (`hb:168`, `mf:218`) | each suite's `StartContextEndStopsNothing`; `TestMetricsForwarderKeepsPublishingAfterStartContextEnds` (I) |
| `Stop` not waiting for the loop | each `FailedStopIsRetried`: the first `Stop` returns nil |
| a failed `Stop` forgets the loop | each `FailedStopIsRetried`: the second `Stop` returns inside the window |
| `json.Unmarshal` for `decodeStrictServiceJSON` in `NewHeartbeatService` (#344) | `TestHeartbeatRefusesAMiscasedKeyBesideTheDefault` |
| `json.Unmarshal` for `decodeStrictServiceJSON` in `NewMetricsForwarderService` | `TestMetricsForwarderConstructionRefusals` |
| nil `deps` not refused (forwarder) | `TestMetricsForwarderConstructionRefusals` |
| the 1 s minimum removed | `TestHeartbeatConstructionRefusals` |
| `interval` / `push_interval` ignored (default used) | `TestHeartbeatCadence` / `TestMetricsForwarderCadence` |
| no first round at `Start` | both cadence tests |
| `include_go_metrics` or `include_proc_metrics` ignored | `TestMetricsForwarderIncludeFlagsReachThePublisher` |
| `component` fallback other than `system`; histogram value as the sum | U-d tables (pin `metrics_forwarder_test.go:500`, `:273`) |
| a failed publish or gather not counted | `TestMetricsForwarderPublishFailureIsCounted`, `...GatherFailureIsCounted` |
| an unhealthy component counted healthy | `TestHeartbeatRecordsComponentHealth` |
| the heartbeat logging to `slog.Default()` | `TestHeartbeatRecordsComponentHealth` (reads `deps.Logger`) |
| `Start` accepting a stand-in component manager | `TestHeartbeatStartRefusesWithoutTheComponentManager` |
| `RegisterAll` without either constructor | `TestHeartbeatAndMetricsForwarderAreRegistered`; `TestHeartbeatComposedFromTheConfigDefaults` |

Uncovered, recorded in `notes/2.3.md`: a publish in progress when `Stop` begins whose call ignores its context (D2;
nats.go, not promised); H7 and H8 (retained).

**Brief line ranges** (this file; a brief cites only these):

| Unit | Lines |
| --- | --- |
| U-a | 54-92, 164-188, 224-232 |
| U-b | 54-133, 156-174, 189-197, 224-232, 325-end (Q1) |
| U-c | 54-124, 134-155, 164-174, 198-205, 224-232 |
| U-d | 134-140, 146-155, 164-174, 206-208 |
| U-e | 54-92, 134-155, 164-174, 209-215, 224-232 |
| P-1 | 29-133, 156-163, 216-223, 325-end (Q1) |
| P-2 | 29-124, 134-155, 216-223 |
| task 2.3 | 233-257 |

## D7. Adopter seam (a composition root using `RegisterAll` and `config`)

1. **They must know:** (i) `heartbeat` is on by default from `config`'s defaults, `metrics-forwarder` is opt-in under
   `services`; (ii) the forwarded names are `semengine_*`, not `semstreams_*`.
2. **If they do nothing:** the heartbeat logs every 30 s, the forwarder is off. Today the same composition is refused
   at boot (D5). A misspelt or miscased key is refused at boot, naming it, where the pin applied a default the operator
   never chose (#344).
3. **Where they find out:** a boot error for a key; (ii) is a doc fact (no state migration); a reader of the `METRICS`
   stream sees the new names.
4. **What they should have to know:** (i) only.

Intent check: no boundary moves.

## D8. Ledger, text, ADRs, issues, merge order

- **Ledger:** the `service` row edited in place.
  - `consumer_purpose`: PR 6, framework API through configuration, the heartbeat record and the `metrics.>` samples;
    semsource enables the heartbeat, semteams the forwarder.
  - `contract`: the D9 requirements; ADR-098 Decision 3; ADR-113; ADR-108 Decision 2; ADR-095 (single use); ADR-110
    (neither writes an `ENTITY_STATES` entry); `RegisterAll` registers both.
  - `dependencies_and_side_effects`: one goroutine each under the run context, ended and joined by `Stop`; the
    heartbeat reads the component manager's health; the forwarder publishes to the `METRICS` stream and counts drops on
    `semengine_errors_total`.
  - `known_risks`: dropped `MetricsGatherer`, the `Validate` methods and the test constructors; changed: strict keys,
    nil dependencies refused, an empty `push_interval` takes the default, the injected logger, the heartbeat's level
    (Q1), Start-context end stops nothing; retained H7, H8.
- **Text:** `service/README.md` and `service/doc.go` gain both services (enabling, keys, defaults, refusals, the record,
  the subjects and payload, the drop counts). ADR-058's port note (`docs/adr/058-boot-lifecycle-phases.md:31-32`) no
  longer lists `HeartbeatService` as not yet ported. `docs/repository-map.md` row `service/`.
- **ADRs:** cited as `sweep.md` § 5; none carried (ADR-098 came with PR 5). ADR-058's exemplar mechanism is replaced by
  ADR-113, settled; no ADR decision is changed or dropped beyond that.
- **Guidance:** none to carry (`sweep.md` § 5). **Coverage:** the `service` floor holds; no new target.
- **Issues:** retires #344 (`Closes #344`, task 2.2; proof `TestHeartbeatRefusesAMiscasedKeyBesideTheDefault`).
  Comments on #196 (guard composed by two more owners) and #195 (`fixtureClient` for U-e). Filed as #384 (not
  this PR's file): `config`'s defaults name a `discovery` service that exists at neither the pin nor here
  (`config/config.go:491-494`; inventory § 5 (c)), a phantom default.
- **Merge order:** #381 and #382 share no file or capability with this PR; either order. If #382 edits
  `docs/repository-map.md` or the ledger, whichever merges later merges `origin/main` and keeps every edit.

## D9. Spec deltas (written after acceptance; `skip_specs` removed then)

- **`service-lifecycle`, ADDED:**
  - "The heartbeat service's construction refuses an unusable configuration" (D3; scenarios: a refusal; the miscased
    key beside the default, #344; the constructor registered and built from `config`'s defaults, D5)
  - "The heartbeat service's Start bounds Start and Stop is the only stop" (D2, with the component-manager refusal)
  - "The heartbeat logs system health at Start and every interval" (D4)
  - "The metrics forwarder's construction refuses an unusable configuration" (D3)
  - "The metrics forwarder's Start bounds Start and Stop is the only stop" (D2)
  - "The metrics forwarder publishes every gathered sample and counts its drops" (D4: subject, payload, filters, the
    two counts).
- **`service-lifecycle`, MODIFIED:** "A service configuration key in another letter case is refused" names the
  heartbeat's and the forwarder's payloads and adds their types to the guard's list.
- **`operator-surface`:** none; neither service serves a route.

## Owner questions

- **Q1. Heartbeat level: Info (owner, 2026-10-23, Enjoy).** The heartbeat service is on by default and its only output
  is one line per interval, which the pin writes at Debug, so at the default log level it shows nothing after the line
  that says it started. The "System heartbeat" line is written at Info.
