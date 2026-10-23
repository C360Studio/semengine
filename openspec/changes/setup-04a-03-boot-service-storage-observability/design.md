# Design: setup-04a-03-boot-service-storage-observability

base: `1bd55d17623b218f81c2a62d4c451e3568b27a59` (draft PR #376). Inputs: `sweep.md` (this change; O-ids), the
whole-package inventory (`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md`, S-ids),
and PR 4's archived design (`openspec/changes/archive/2026-10-22-setup-04a-03-boot-service-message-logger/design.md`),
whose shape this copies. Pin lines are SemStreams `8b99efe9`, `service/`; `:n` is `storage_observability.go`,
`_http.go` and `_metrics.go` the other two files. One lifecycle owner, 1,540 non-test pin lines (under the ~3,000
bound, #159).

Owner intent applied, not re-asked: ADR-113 (context ownership and lifecycle control); no context-value bags; no back
doors on production types; natsclient passes nats.go through; no SemStreams state migration; simple idiomatic Go;
port whole, drop only dead surface; every real defect found and not fixed is filed; S6 takes the simple fix, the
strict decoder already in the package (owner 2026-10-19).

## Inventory addendum

- **Callers outside `service`.** Pin, Go: none for any exported name of these files (`sweep.md` O3). Consumers: none
  enables the service or names its route or types; semsource lists the bucket `STORAGE_REPORT` among those its cutover
  purges (`internal/cutover/buckets.go:37`). The surface reached from outside is the configuration, the route
  `/storage-observability/report`, the `semengine_storage_*` metrics and the health message.
- **Why it ports.** 03B's operator-surface row: "storage observability report ... served as at the pin | Keep"
  (`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:728`); `docs/contract.md:140`. Pin ADR-098
  names it the model for substrate observability (`098:92-94`).
- **Who mounts it.** `serviceNameToPrefix` keeps `storage-observability` (`service/service_manager_http.go:382-385`);
  the service is not materialized by default (`service/service_configs.go:85-89` adds only component-manager,
  service-manager and metrics).
- **Open pull requests and issues:** `sweep.md` § 4. This PR retires none; it comments on #195 and #196.

## D1. Files and surface (surface audit, #190)

- **Carried, adapted:** the three files, module path rewritten, § 2.4 renames: `natsclient.<name>` becomes
  `storagereport.<name>` with the package's nine renames, `graph.OwnerOf` and `graph.EnsureCatalogBucket` become
  `kvcatalog.OwnerOf` and `kvcatalog.EnsureCatalogBucket`, `deps.NATSClient.AccountStreamLister` becomes
  `storagereport.AccountStreamLister(deps.NATSClient)` (`natsclient/storagereport/storage_inventory.go:119`). The
  scratch build then leaves the six errors `sweep.md` § 1 lists; D2-D4 resolve each.
- **Not carried:** the second `StorageObservabilityServiceName` (`:70-72`; it is `service/service_manager_http.go:26`).
- **Dropped, no reader:**
  - `Snapshot` (O3)
  - `RegisterMetrics` (O2; `Service` has no such method since PR 1)
  - the `schema` struct tags on `StorageObservabilityConfig` (O8)
  - `collectionInterval` and the raw `config` field (O11)
  - the reader-function fields of the two surfaces and their nil branches (O4; D5).
- **Kept with contract lines** (README, ledger `contract`, D9): `NewStorageObservabilityService` (through
  `RegisterAll`), `StorageObservabilityConfig`, `StorageObservabilityService` with `Start`, `Stop`, `Health` and the
  `HTTPHandler` methods `RegisterHTTPHandlers` and `OpenAPISpec`; `StorageReportResponse` and `StorageReportSummary`
  (the route's wire shape).
- **`RegisterAll`** registers the constructor under `storage-observability`. It registers no OpenAPI spec: the route's
  spec has no `SchemaRef` (O7).
- **Options:** (a) chosen; (b) keep `Snapshot` as framework API: it would need a contract line and a product test for
  a read the route already serves; (c) keep `RegisterMetrics`: a method on no interface, with a registrar type the tree
  does not have.

## D2. Lifecycle: Start bounds Start; Stop is the only stop

Governed by ADR-113. The service composes `internal/lifecycleguard` for `Start` and `Stop` (S3: optional for this
owner in the inventory, recommended because every owner ported so far composes it, `service/base.go:191-193`,
`service/message_logger.go:257-258`; ADR-108 Decision 2). The pin's `stopOnce`, `stopChan` and `loopDone` go. Observable
behaviour, each with its proving test (D7 units in brackets):

- **Refusals.** `Start` refuses a nil or ended context, a second `Start`, and a `Start` after any `Stop`
  (`errs.ErrAlreadyStarted`); a refused `Start` holds nothing. `Stop` refuses nil; given an ended context it returns
  that context's error and stops nothing. Proof: `TestStorageObservabilityPassesTheLifecycleSuite` [U-a]; must-fail
  owner = a service stopped before `Start` (`service/base_lifecycle_test.go:249-259` precedent).
- **Startup work.** `Start` starts the base service, then the collector's and the consumer's loops, and returns nil.
  It does no I/O and never fails on account or bucket state: an unreadable account or an unreachable bucket is retried
  by the loops, each on its own schedule (the collector every `interval`, the consumer's watch after 5 s,
  `natsclient/storagereport/storage_report_consumer.go:40`). The loops run under the one run context `Start` creates,
  kept only as a cancel (ADR-113; `service/message_logger.go:276` precedent). Proof:
  `TestStorageObservabilityBootsWithoutAReachableAccount` [U-b] (pin `..._integration_test.go:318-346`,
  moved to a never-connected client and a log signal instead of the pin's `time.Sleep(3 * time.Second)`).
- **Ending the Start context after `Start` returned stops nothing.** The loops keep running, so a collection after it
  still publishes. Proof: suite `StartContextEndStopsNothing` (probe § 3: the pin fails it, holding neither its base
  status nor its loops); `TestStorageObservabilityKeepsCollectingAfterStartContextEnds` [U-f].
- **Stop.** `Stop` ends the run context, waits for both loops to return and stops the base service, and returns nil
  only once all three are done. If its context ends first it returns that error, and the next `Stop` waits again
  (the pin's behaviour, probe § 3, kept). A `Stop` after one that returned nil returns nil and calls nothing; a `Stop`
  while another runs is refused as transient (the guard). How long a collection in progress takes to see the run
  context end depends on whether the JetStream listing honours its context, which nats.go does not promise; `Stop`
  stays bounded by its own context either way. Proofs:
  - `TestStorageObservabilityFailedStopIsRetried` [U-b]. The composition's `slog` handler (PR 4's `holdingHandler`,
    `service/message_logger_helpers_test.go:49-124`, a production dependency, not a seam) holds the collector's first
    "storage inventory collection failed; serving last known result" record
    (`natsclient/storagereport/storage_inventory.go:329`), so the collector's goroutine is inside its log call:
    - the first `Stop`, under a 200 ms deadline, returns `context.DeadlineExceeded` (the held loop keeps it from
      finishing, so the deadline cannot fail a correct implementation);
    - a second `Stop`, on a goroutine, has not returned within a 200 ms observation window while the record is held;
    - after release, the second `Stop` returns nil, a third returns nil, and the suite's observation shows no loop.
  - Suite `RepeatedStopIsNoOp`.
- **Reads during and after Stop.** `Health` and `GET /report` read the consumer's snapshot and the collector's last
  inventory, which do no I/O (`storagereport`: `storage_report_consumer.go:174-186`, `storage_inventory.go:247-254`),
  so they answer at any time; after `Stop` the report is the last one read and the verdict is the base service's own
  after its stop (unhealthy, "Service is unhealthy (failed checks: 0)": `service/base.go:160-165`, `:246`).
  Proof: `TestStorageObservabilityRoutesAfterStop` [U-e].
- **Context rules:** the service keeps no `context.Context`; it holds one private cancel. Locks, the join and its order
  are the developer's (failing-first under `-race`).

## D3. Configuration and construction

`NewStorageObservabilityService` (the one constructor) decodes with `decodeStrictServiceJSON`
(`service/service_configs.go:136`): an unknown key, nested ones inside `pressure_thresholds` included, and a key in
another letter case are refused, naming the key (S6; probe § 3 measured all three accepted at the pin). It refuses,
as invalid data (`errs.WrapInvalid`, O10) and naming the input:

- a nil `deps` or NATS client (as `:187-189`)
- a nil metrics registry (O5; new)
- an `interval` or `timeout` that does not parse or is not positive (`:140-152`), and a `timeout` above the `interval`
  (`:122-126`)
- pressure thresholds that `StoragePressureThresholds.Resolve` refuses (`:129-132`;
  `natsclient/storagereport/storage_pressure.go:136-192`).

Defaults have one home, the applied form (`:110-138`): `interval` 1m, `timeout` 15s
(`storagereport.DefaultStorageInventoryInterval`, `DefaultStorageInventoryTimeout`), thresholds as
`storagereport.DefaultStoragePressureThresholds()`. The service keeps only the applied values (O11) and logs the
applied interval at start. Configuration is applied by restart, as the pin's file comment says (`:40-47`).

Proofs [U-b]: `TestStorageObservabilityConstructionRefusals` (one subtest per refusal, each asserting the invalid
class and the named key or field); the pin's `:128-224` config tests, with `:189-190` compared against
`storagereport.DefaultStoragePressureThresholds().Resolve()` instead of the two unexported names;
`TestStorageObservabilityService_IsRegistered` (pin `:226-235`).

Options for the registry: (a) refuse nil, chosen: a service whose metrics reach no scrape is the phantom signal the
pin's comment names (`:245-249`); (b) accept and register nothing (the pin): 19 gauges updated that no one reads.

## D4. Metrics (`_metrics.go`)

- **Names.** Every collector keeps its pin name under the namespace `semengine` (#69): `semengine_storage_resource_*`
  (7), `semengine_storage_resources`, `semengine_storage_pressure_resources`, `semengine_storage_account_*` (9) and
  `semengine_storage_report_collected_timestamp_seconds`; labels as at the pin (`:70`, `:241-288`). No migration of
  the `semstreams_storage_*` names (owner: no SemStreams state migration); the README states the rename.
- **Registration (O2).** The constructor registers the 19 collectors once, each through `metric.RegisterOrGet` under
  `storage-observability`, and the surface writes to the collector each call returns. A second service built on the
  same registry therefore writes the series the registry serves. Proof:
  `TestStorageMetrics_SecondRegistrationWritesTheCanonicalCollectors` [U-c] (replaces `RegisterIsIdempotent`, pin
  `:777-786`).
- **`ForgetAccount` (O1).** When the consumer drops the account row it held, every per-tier series the surface
  published for it (limit, used, declared, growth, headroom, time to threshold, and the three closed-enum families)
  is retracted, and a later `ObserveAccount` publishes afresh. The freshness gauge is left as it is: it reports the
  newest collection seen, which a dropped row does not undo. Proof:
  `TestStorageAccountMetrics_ForgetAccountRetractsEveryTierSeries` [U-c].
- **Carried as at the pin:** absence published as no series, retraction on a label change and on `ForgetResource`, the
  categorical aggregates emitted in full, the monotonic freshness gauge. Proofs: the pin's `:485-839` and
  `storage_observability_tier_metrics_test.go:61-149`, `:224-244` [U-c].

## D5. Health and the report route (report-only)

- **Health (O4).** `Health` returns the base service's status with the storage picture appended to its message
  (joined with a semicolon, as `:496-504`). It never changes `Healthy`, `Status` or the sub-statuses, so pressure, an
  over-committed tier, an unread report or a failed collection make nothing unhealthy, degraded or not ready. The
  service sets no health check. The rendering is one plain function of the base status, the consumer's snapshot and
  the collector's inventory (the pin's `describe`, `message`, `worstResources`, `accountFindings` and
  `collectionState`, `:496-663`, unchanged in what they print), which `Health` calls with
  `s.consumer.Snapshot()` and `s.collector.Latest()`.
- **The route (O4, O6, O7).** `RegisterHTTPHandlers(prefix, mux)` mounts `GET <prefix>/report` (prefix with or
  without a trailing slash). It answers 200 with `StorageReportResponse` built from the consumer's snapshot by
  `storageReportResponseFrom` (`_http.go:171-202`, unchanged): `report_only` true; `synced`; `updated_at` only once a
  change was applied; `summary` with `resources`, `pressure` (never null), `not_evaluated` and `worst_pressure` only
  when something was evaluated; `account` only once an account row was read; every row in `resources` (never null,
  never filtered). It answers 200 whatever the report says. `HEAD` is answered as `GET`; any other method gets 405
  from ServeMux. `OpenAPISpec` documents `/report` as at the pin; its comment is rewritten to what this tree does:
  paths come from the mounted service, and no spec is registered because none names a schema.
- **One produced truth.** The route, the health message and the metrics read the one consumer, fed by one watch of
  the published bucket; none recomputes. Proof: `TestStorageReportHTTP_CannotDivergeFromTheMetricsSurface` [U-e] (pin
  `_http_test.go:346-445`, over `storagereport.NewConsumer` and the pin's fake `ReportWatchStore`).
- **Proofs** [U-d] the pin's health tests (`storage_observability_test.go:249-283`, `:346-483`,
  `storage_observability_tier_metrics_test.go:151-222`) through the function; `TestPropStorageHealthKeepsTheBaseVerdict`
  (generated); [U-e] the pin's route tests (`_http_test.go:128-345`), `TestStorageReportHTTP_AnswersGETAndHEAD`; [U-f]
  `TestStorageObservabilityCriticalPressureKeepsReadiness`, the pin's readiness test (`:284-344`) on the production
  path: a manager composing the service over the shared server, with thresholds configured so a bounded stream holding
  a little data is critical (`{"pressure_thresholds":{"warning_headroom":0.99,"high_headroom":0.98,
  "critical_headroom":0.97}}`; a free fraction at or below `critical_headroom` is critical,
  `natsclient/storagereport/storage_pressure.go:611`). It waits for the critical row on `/storage-observability/report`,
  then asserts `/readyz` answers `READY` and `/health` 200 with `critical=1` in the service's message.
- **Options for O4:** (a) plain functions, chosen: the tests pass values in, and production has no field a test could
  swap; (b) carry the reader fields: one indirection and three nil branches no production value reaches.
- **Alert rules (O9):** Q1.

## D6. Adopter seam (a composition root enabling `storage-observability`)

1. **They must know:** (i) enable it in `services`, optionally with `interval`, `timeout` and `pressure_thresholds`;
   (ii) `Dependencies.MetricsRegistry` is required (the service manager passes the one it holds); (iii) the metrics are
   `semengine_storage_*`, not `semstreams_storage_*`.
2. **If they do nothing:** the service is off. If they get (ii) wrong, or misspell a key, construction refuses, naming
   it, where the pin ran with defaults the operator never chose. An unreadable account never stops boot; health says
   the report has not been read.
3. **Where they find out:** a boot error for (ii) and for a misspelt key. (iii) is a doc fact; an alert or dashboard
   still on the old names reads nothing, which is a naming change and not a correctness fact the framework holds (no
   state migration). If Q1 carries the rules, the rule file shows the names.
4. **What they should have to know:** (i) only.

Intent check: no boundary moves.

## D7. Tests

| Pin test file (lines) | Here |
| --- | --- |
| `storage_observability_test.go` (839) | split by section: builders `:24-73` → U-a; gauge helpers `:75-114` already in `service/gauge_helpers_test.go`; `staticSnapshot` `:116-118` dropped (O4); config and registration `:128-241` → U-b; health `:243-483` → U-d, except the readiness test `:284-344` → U-f; metrics `:485-839` → U-c |
| `storage_observability_http_test.go` (624) | U-e; the surface helpers `:99-120` replaced by the route over a real service and by `storageReportResponseFrom` (O4); alert rules `:448-624` per Q1 |
| `storage_observability_integration_test.go` (346) | `:33-316` → U-f through `fixtureClient`; `:318-346` → U-b on a never-connected client (no sleep; no `Snapshot`, O3) |
| `storage_observability_tier_metrics_test.go` (244) | builders `:21-59` → U-a; `:61-149`, `:224-244` → U-c; `:151-222` → U-d |

Unexported names (§ 2.3): `UnknownGrowth(reason)` becomes `storagereport.Growth{State: storagereport.GrowthUnknown,
Unavailable: reason}`; `UnknownCapacity()` becomes `storagereport.Capacity{State: storagereport.CapacityUnknown}`
(the bodies, `natsclient/storagereport/storage_growth.go:134-136`, `storage_resource.go:138`).

**Units** (one or two files each; a brief cites only the D-sections and line ranges named). Order: U-a first, because
the others build the service and rows through it; then U-b to U-f in any order. Every unit builds the service the one
production way, `NewStorageObservabilityService`, through U-a's helper, except U-c (the metrics type alone) and the
rendering tests of U-d and U-e (pure functions).

- **U-a** `service/storage_observability_helpers_test.go`, `service/storage_observability_suite_test.go` (both new,
  no build tag). D2.
  - `newTestStorageObservability(t *testing.T, client *natsclient.Client, config string, logger *slog.Logger)
    (*StorageObservabilityService, *metric.MetricsRegistry)`: `NewStorageObservabilityService(json.RawMessage(config),
    &Dependencies{NATSClient: client, MetricsRegistry: metric.NewMetricsRegistry(), Logger: logger})`, then the type
    assertion.
  - Row builders carried from the pin (`resourceRow`, `withPressure`, `withGrowth`, `unboundedRow`, `unknownRow`,
    `fillingTier`, `inheritedPressureRow`) with the replacements above.
  - Suite adapter: `lifecycletest.Run`, `Owner`, `Observation`, `Promise{}`; the service over `newUnconnectedClient(t)`
    (`service/service_manager_test.go:816`). `Observe` reports the base status when it is neither stopped nor starting,
    and `loops` while a loop goroutine has not returned, read from the unexported field the join waits on (no method is
    added for it). Must-fail owner: stopped before `Start`.
  - Calls: `NewStorageObservabilityService`, `Start`, `Stop`, `Status`; `newHoldingHandler`, `heldRecord`,
    `releaseHeld`, `arrived` (`service/message_logger_helpers_test.go:62-94`).
- **U-b** `service/storage_observability_test.go`. D2, D3.
  - Pin `:128-241` adapted; `TestStorageObservabilityConstructionRefusals` (replaces `:237-241`);
    `TestStorageObservabilityFailedStopIsRetried`; `TestStorageObservabilityBootsWithoutAReachableAccount`: the helper
    over `newUnconnectedClient(t)` with `{"interval":"1s","timeout":"1s"}`, `Start` nil, then both the collector's and
    the consumer's warnings arrive (`arrived`, 10 s bound; the consumer's is "could not watch the storage report;
    operator surfaces are serving their last values", `storage_report_consumer.go:215`), then `Health()` is healthy
    with "storage report has not been read yet" in its message and `GET /report` answers `synced` false.
  - Calls: `NewStorageObservabilityService`, `RegisterAll`, `NewServiceRegistry`, `Registry.Constructor`, `Start`,
    `Stop`, `Health`, `RegisterHTTPHandlers`, `storagereport.DefaultStoragePressureThresholds`, `errs.IsInvalid`.
- **U-c** `service/storage_observability_metrics_test.go` (new), `service/storage_observability_tier_metrics_test.go`.
  D4.
  - Pin `storage_observability_test.go:485-839` and tier `:61-149`, `:224-244`, metric names read as
    `semengine_storage_*`; the two new tests of D4.
  - `newTestStorageMetrics(t)`: `newStorageObservabilityMetrics()` registered on a fresh `metric.NewMetricsRegistry()`.
  - Calls: `newStorageObservabilityMetrics`, `register(registry)`, `ObserveResource`, `ForgetResource`,
    `ObserveAccount`, `ForgetAccount`, `pressureSeverityValue`, `pressureStateNotEvaluated`; `gaugeValue`,
    `requireGauge`.
- **U-d** `service/storage_observability_health_test.go` (new). D5, Health.
  - Pin `storage_observability_test.go:249-283`, `:346-483` and tier `:151-222`, calling the rendering function with
    the base status, snapshot and inventory each test built; `TestPropStorageHealthKeepsTheBaseVerdict`.
  - Calls: the rendering function (the developer names it; it takes `health.Status`, `storagereport.Snapshot`,
    `storagereport.StorageInventory` and returns `health.Status`), `health.NewHealthy`, `NewDegraded`, `NewUnhealthy`,
    `health.Aggregate`.
- **U-e** `service/storage_observability_http_test.go`. D5, route; Q1.
  - Shape tests (pin `:128-310`) through `storageReportResponseFrom` and `json.Marshal`; route tests (pin `:311-345`)
    and `TestStorageReportHTTP_AnswersGETAndHEAD` and `TestStorageObservabilityRoutesAfterStop` through
    `RegisterHTTPHandlers` on a service from U-a's helper over `newUnconnectedClient(t)`, with `httptest`; the
    divergence test (pin `:346-445`) over `storagereport.NewConsumer` and the pin's fake store `:24-85`; the alert-rule
    tests (pin `:448-624`) per Q1, reading `semengine_storage_`.
  - Calls: `storageReportResponseFrom`, `RegisterHTTPHandlers`, `Start`, `Stop`, `storagereport.NewConsumer`,
    `storagereport.ConsumerConfig`, `Consumer.Run`, `Consumer.Snapshot`, the U-c metrics names.
- **U-f** `service/storage_observability_integration_test.go`. D2, D5.
  - Pin `:33-316` over `fixtureClient(t)` (streams created on the shared server, which the reset empties), every wait
    bounded (30 s) and read through `GET /storage-observability/report` and the registry, not `Snapshot`.
  - `TestStorageObservabilityKeepsCollectingAfterStartContextEnds`: `{"interval":"1s","timeout":"1s"}`; `Start`
    under a context the test then cancels; then a stream is created, and the test waits (30 s bound) for its row on
    the route.
  - `TestStorageObservabilityCriticalPressureKeepsReadiness` (D5): `createTestServiceManager`,
    `newStandInComponentManager`, `RegisterInstance`, `StartAll`, `stopManagerAtEnd`, `httpGet`
    (`service/service_manager_test.go:63-190`), the service from U-a's helper over `fixtureClient(t)`.
  - Calls: `fixtureClient`, `natsclient.Client.JetStream`, `jetstream.CreateStream`, `Publish`, plus the above.

**Port units** (task 2.2), in this order, each ending with `go test -race ./service/` green on the tests it turns
green and `task verify` at the end of P-2:

- **P-1** `service/storage_observability_metrics.go`. D4. It builds on its own: it names only
  `StorageObservabilityServiceName`, `metric`, `storagereport` and the Prometheus client. U-c turns green.
- **P-2** `service/storage_observability.go`, `service/storage_observability_http.go`, and the one `RegisterAll` line
  in `service/register.go` (its doc comment names the service). D1, D2, D3, D5. U-a, U-b, U-d, U-e and U-f
  turn green.

Harness rules as PR 4's D7: no `time.Sleep`, `Skip`, fixed port, or `t.Parallel` in an integration file; cleanup stops
under `context.WithTimeout(context.WithoutCancel(t.Context()), ...)`; every NATS connection through `fixtureClient`
(#195). Real-clock windows, listed as `docs/testing.md` requires: the two 200 ms windows of
`TestStorageObservabilityFailedStopIsRetried`; the 1 s collection interval of U-b and U-f, whose waits are bounded at
10 s (unit) and 30 s (integration). Every other wait is on a signal.

**Generated checks.** "Health never moves the base verdict" is a stated law (D5), so it gets a rapid property,
`TestPropStorageHealthKeepsTheBaseVerdict`:

- generator: a base status (healthy, degraded or unhealthy, any message, with or without sub-statuses), a snapshot
  (synced or not; rows with any pressure state, evaluated or not, own or inherited basis; an account with any
  over-commitment state, known or not) and an inventory (stale or not, collected or not)
- checks: `Healthy`, `Status` and the sub-statuses equal the base's; the message begins with the base message when
  that is not empty. Oracle: the base status itself
- the run records its seed and check count.

The metrics surface's history (observe, change labels, forget, observe again) keeps the pin's example tests, plus the
`ForgetAccount` example: the histories are short and the pin's examples name each transition the code branches on
(`_metrics.go`: label change `:351-354`, category change `:383-392`, forget `:398-410`, tier leaving `:458-466`).
Start/Stop histories are the guard's (`internal/lifecycleguard/guard_test.go`) plus the suite; examples suffice for
both.

**Sensitivity** (task 2.3). Unit rows go through `task mutate:check`. Integration rows (`I`) are run as PR 4 ran them,
with `GOFLAGS=-tags=integration` (`openspec/changes/archive/2026-10-22-setup-04a-03-boot-service-message-logger/
notes/2.3.md`), or by hand where that fails. A row counts as a detection only when the test fails under the mutant; a
hang or timeout is inconclusive, so each Stop-path test runs under a bounded context and asserts the result.

| Mutant | Caught by |
| --- | --- |
| run context derived from `Start`'s (the pin's `:287`) | suite `StartContextEndStopsNothing` (probe § 3); `TestStorageObservabilityKeepsCollectingAfterStartContextEnds` (I) |
| `Stop` not waiting for the loops | `TestStorageObservabilityFailedStopIsRetried`: the first `Stop` returns nil, not `DeadlineExceeded` |
| a failed `Stop` forgets the loops it did not join | `TestStorageObservabilityFailedStopIsRetried`: the second `Stop` returns inside the window while the record is held |
| `json.Unmarshal` for `decodeStrictServiceJSON` (the pin's `:178`) | `TestStorageObservabilityConstructionRefusals` (unknown, nested and miscased keys) |
| a nil metrics registry accepted | `TestStorageObservabilityConstructionRefusals` |
| `ForgetAccount` retracting nothing | `TestStorageAccountMetrics_ForgetAccountRetractsEveryTierSeries` |
| the surface keeps its own candidate, not what `RegisterOrGet` returned | `TestStorageMetrics_SecondRegistrationWritesTheCanonicalCollectors` |
| namespace `semstreams` (the pin's `_metrics.go:196`) | `TestStorageMetrics_PublishesPerResourceSeries` (pin `storage_observability_test.go:485`, reading `semengine_` names) |
| critical pressure marks health unhealthy | `TestStorageObservabilityHealth_CriticalPressureFailsNoGate` (pin `storage_observability_test.go:249`); `TestPropStorageHealthKeepsTheBaseVerdict` |
| the base message overwritten (the pin's `:497-503` guard removed) | `TestStorageObservabilityHealth_PreservesTheBaseVerdictsReason` (pin `storage_observability_test.go:397`) |
| own-bound before inherited dropped from the sort (`:608-610`) | `TestInheritedPressure_DoesNotEvictOwnBoundFindingsFromTheHealthMessage` (pin `storage_observability_tier_metrics_test.go:184`) |
| the freshness gauge allowed to move backwards (`_metrics.go:187`) | `TestStorageMetrics_FreshnessGaugeNeverMovesBackwards` (pin `storage_observability_test.go:805`) |
| a label change not retracting the old series (`_metrics.go:352-354`) | `TestStorageMetrics_OwnerChangeDoesNotStrandTheOldSeries` (pin `storage_observability_test.go:599`) |
| the route filtering out not-evaluated rows | `TestStorageReportHTTP_NotEvaluatedRowsStayVisible` (pin `storage_observability_http_test.go:228`) |
| the pin's method check (`_http.go:138`) for the `GET` pattern | `TestStorageReportHTTP_AnswersGETAndHEAD` |
| `RegisterAll` without the constructor | `TestStorageObservabilityService_IsRegistered` |
| critical pressure turning `/readyz` not ready | `TestStorageObservabilityCriticalPressureKeepsReadiness` (I) |

Uncovered, recorded in `notes/2.3.md`: a collection in progress when `Stop` begins whose listing ignores its context
(D2; nats.go, not promised); the bucket lock held across acquisition (O12, retained).

**Brief line ranges** (this file; a brief cites only these):

| Unit | Lines |
| --- | --- |
| U-a | 53-95, 191-222, 274-279 |
| U-b | 53-122, 191-208, 223-231, 274-279 |
| U-c | 123-142, 191-208, 232-239 |
| U-d | 143-175, 191-208, 240-245, 274-295 |
| U-e | 143-175, 191-208, 246-253, 274-279, 390-end (Q1) |
| U-f | 53-95, 143-175, 191-208, 254-264, 274-279 |
| P-1 | 123-142, 265-269 |
| P-2 | 15-122, 143-175, 265-273 |
| task 2.3 | 296-323 |

## D8. Ledger, text, ADRs, coverage, issues, merge order

- **Ledger:** the `service` row edited in place.
  - `consumer_purpose`: PR 5, framework API through configuration, the route, the metrics and the health message; no
    consumer enables it; 03B operator-surface row "Keep".
  - `contract`: the D9 requirements; ADR-098 Decisions 3 and 4; ADR-113; ADR-108 Decision 2; ADR-110 (writes no
    `ENTITY_STATES` entry; owns `STORAGE_REPORT` through `kvcatalog.EnsureCatalogBucket`); pin ADR-068 D1 as
    `graph/kvcatalog` implements it; kept names (D1); `RegisterAll` registers the constructor.
  - `dependencies_and_side_effects`: `natsclient/storagereport`; two goroutines under the run context (a collection
    every `interval`, a watch of `STORAGE_REPORT` retried every 5 s), both ended and joined by `Stop`; creates or
    reconciles `STORAGE_REPORT` on first use; registers 19 collectors under `storage-observability`.
  - `known_risks`: dropped `Snapshot`, `RegisterMetrics`, the `schema` tags; changed: strict keys, nil registry
    refused, `semengine_storage_*` names, `HEAD` answered, Start-context end stops nothing; retained O12.
  - The `natsclient` row's `ForgetAccount` sentence gains its implementer, this service's metrics.
- **ADR-098** carried to `docs/adr/098-observability-splits-by-subject.md` with a port note (pin path and SHA, the
  package it came with, its ledger row, and what it names that is not ported: ADR-027, the ops agent and
  `emit_diagnosis`, and the agent-execution conditions of Decisions 1-2), and its own ledger row, as ADR-058 came with
  PR 1. PR 6 (metrics forwarder, heartbeat) cites it.
- **Text:** `service/README.md` and `service/doc.go` gain the service: enabling it, its keys and defaults, its
  refusals, report-only and the capacity-admission deferral gate the pin's spec records (pin
  `openspec/specs/storage-observability/spec.md:169-218`, four conditions, as the reason nothing gates on pressure),
  the route and its body, the health message, the metric names and the rename. `docs/repository-map.md` rows `service/`
  (`:55`) and `internal/lifecycleguard/` (`:78`); `docs/adr/` row (`:30`) gains ADR-098.
- **Guidance:** none to carry (`sweep.md` § 5).
- **Coverage:** the `service` floor (`scripts/cover-check.sh`) holds; no new target.
- **Issues:** retires none. Comments on #195 (fixture adopted for these tests) and #196 (guard composed). Nothing to
  file: every finding is fixed here or retained with its reason (O12); Q1 keeps the rules, so no issue is filed for
  them.
- **Merge order:** #374 (Gemini), #377 (natsclient handler root) and this PR all edit `docs/repository-map.md`;
  whichever merges later merges `origin/main` and keeps every edit. #374's `scripts/port-status.sh` reads the ledger;
  if it parses the `service` row, its own tests on the merged head say so. #358 (`client_golang` 1.25.0) shares no
  file; either order.

## D9. Spec deltas (written after acceptance; `skip_specs` removed then)

- **`service-lifecycle`, ADDED:**
  - "The storage observability service's construction refuses an unusable configuration" (D3; scenarios: a refused
    construction, `TestStorageObservabilityConstructionRefusals`; omitted fields take the documented defaults; the
    constructor is registered and the service is not materialized by default)
  - "The storage observability service's Start bounds Start and Stop is the only stop" (D2's bullets as scenarios,
    each naming its test, including that `Start` succeeds with the account unreadable)
  - "Storage pressure is reported and gates nothing" (D5: the health verdict is the base's for every report;
    scenarios: the property; critical pressure leaves `/readyz` `READY` and `/health` 200 with the pressure in the
    message).
- **`operator-surface`, ADDED:**
  - "The storage report is served at /storage-observability/report" (D5: always 200, `GET` and `HEAD`, 405 otherwise,
    the body's fields and their absence rules, every row served unfiltered; scenario: the route and the metrics read
    one consumer)
  - "The storage metrics are published under semengine_storage_" (D4: names, absence as no series, retraction on
    forget for resources and the account, aggregates in full, the canonical collector written; with Q1 (a), the example
    alert rules read only published names).

  No MODIFIED requirement: "/openapi.json documents the mounted routes" already covers `/report`.

## Owner questions

- **Q1. Decided (owner, 2026-10-23, PR #376 comment 6363685574): keep the example alert rules, renamed to the
  `semengine_storage_*` names, at `configs/prometheus/rules/`, with their four tests.** At the pin the storage metrics
  ship with a file of 13 example Prometheus alerts (339 lines), and the four tests check it against the metrics: every
  alert reads a metric the service publishes, none fires on a report row disappearing, unbounded storage is named, and
  each alert says nothing is enforced. It is the first piece of the dashboard-and-drift-test item still open on the
  epic (#9, item 5). The file is an example operators copy, not something the engine loads.
