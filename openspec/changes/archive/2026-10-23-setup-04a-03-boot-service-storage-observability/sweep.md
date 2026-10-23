# Shape sweep: setup-04a-03-boot-service-storage-observability (PR 5 of 7)

base: `1bd55d17623b218f81c2a62d4c451e3568b27a59` (`main` `05d6ec6` plus the claim commit; draft PR #376)

The per-PR read the architect contract asks for (§ The shape sweep), of the three files this pull request carries, on
a scratch copy of the SemStreams pin `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` (`gh api .../tarball/8b99efe9`, local
only). It re-uses the whole-package inventory, `INVENTORY PASS` round 2 at `ad946b7`
(`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md`, "the inventory" below), wherever
that already pins a line, and adds what it did not pin. Disposition is proposed here and settled in `design.md`. Pin
lines are `service/<file>:<line>`; `storage_observability.go` is written `:n` where no file is named, `_http.go` and
`_metrics.go` abbreviate `storage_observability_http.go` and `storage_observability_metrics.go`.

Files (non-test pin lines, 1,540 in all): `storage_observability.go` 663, `_http.go` 254, `_metrics.go` 623. Test files
2,053 lines (§ 6). The five Manager and component-manager test files the inventory counted with this PR (§ 2.3, 2,208
lines) landed in PRs 2 and 3 once `requireGauge` and `gaugeValue` moved to `service/gauge_helpers_test.go`.

## 1. Findings the inventory already pins (cited, not repeated)

| Id | Shape | Pin lines in these files | Disposition |
| --- | --- | --- | --- |
| S1 | run context from the Start context | `:287` — `runCtx, cancel := context.WithCancel(ctx)`; goroutines `:294`, `:298`; join `:304` | required (ADR-113; probe § 3); design D2 |
| S3 | copied lifecycle state machine | `:167-171` (`stopOnce`, `stopChan`, `cancel`, `wg`, `loopDone`), with the hand checks `:274-281` | optional for this owner (the inventory's S3); recommended in D2, since every owner ported so far composes `internal/lifecycleguard` and the S1 fix rewrites this state anyway |
| S6 | plain config decoder; unknown keys accepted | `:178` — `if err := json.Unmarshal(rawConfig, &cfg); err != nil {` | required here: probe § 3 shows a typo'd key and a key in another letter case each change or lose an operator's setting without a word; owner intent 2026-10-19 (simple fix, strict decoder in the package); D3 |
| S14 | context rules otherwise clean | re-checked below | none |
| § 2.3 | test names `storagereport` has since unexported | `UnknownCapacity` `storage_observability_test.go:737`, `:759`, `:760`, `:766`, `storage_observability_http_test.go:89`, `:90`; `UnknownGrowth` `storage_observability_test.go:36`, `storage_observability_tier_metrics_test.go:96`; `DefaultWarningHeadroom`, `DefaultCriticalHorizon` `storage_observability_test.go:189-190` | required (build); D7 |
| § 2.4 | renamed names | every `natsclient.Storage*`, `Pressure*`, `Capacity*`, `Tier*`, `Overcommitment*` name; `graph.OwnerOf` `:223`, `graph.EnsureCatalogBucket` `:397`; `metric.MetricsRegistrar` `:351`, `_metrics.go:309`, `:330`, `:334` | required (build); D1, D4 |
| § 2.4 | metric namespace `"semstreams"` | `_metrics.go:196`, `:291` | required (#69; ledger `service` row, PR 2: "the metric namespace reads semengine"); D4 |
| § 7 | `time.Sleep` in a test | `storage_observability_integration_test.go:339` — `time.Sleep(3 * time.Second)` | required (harness rules); D7 |
| § 7 / #195 | test client | `storage_observability_integration_test.go:35` — `tc := natsclient.NewTestClient(t, natsclient.WithKV())`, `:322` | required (harness rules): `fixtureClient`; D7 |

**What the scratch build leaves after the mechanical renames.** The three pin files with the module path rewritten,
`natsclient.<name>` requalified to `storagereport.<name>` with the nine renames (ledger `natsclient` row,
`docs/admission-ledger.yaml:844-846`), `graph.OwnerOf` and `graph.EnsureCatalogBucket` to `kvcatalog`, and the
duplicate `StorageObservabilityServiceName` (`:72`; already `service/service_manager_http.go:26`) removed, built in a
`git archive` copy of the base (`go build -gcflags=-e ./service/`, go1.26.10), report exactly six errors:

```text
storage_observability.go:204:14: cannot use metrics (variable of type *storageObservabilityMetrics) as storagereport.Observer value in struct literal: *storageObservabilityMetrics does not implement storagereport.Observer (missing method ForgetAccount)
storage_observability.go:218:19: deps.NATSClient.AccountStreamLister undefined (type *natsclient.Client has no field or method AccountStreamLister)
storage_observability.go:255:80: too many arguments in call to NewBaseServiceWithOptions
storage_observability.go:270:12: undefined: validateLifecycleContext
storage_observability.go:350:72: undefined: metric.MetricsRegistrar
storage_observability_metrics.go:311:65: undefined: metric.MetricsRegistrar
```

(plus unused imports; line numbers are the scratch copy's, one line above the pin's). Each is a finding below or a
§ 2.4 row: O1, `storagereport.AccountStreamLister(client)` (ledger D2), PR 1's Q1 (`NewBaseServiceWithOptions` lost
its config argument), S3 (the guard refuses nil contexts), O2.

## 2. Findings added by this sweep

Each is `path:line` — the line's text at the pin, then the shape and the proposed disposition.

- **O1, the observer is missing `ForgetAccount` (required: build; accepted design).** `_metrics.go:412-467`
  (`ObserveAccount`) and `:479-495` (`deleteAccountTierSeries`) are all the account half the pin has. The tree's
  `storagereport.Observer` gained `ForgetAccount(row AccountReport)`, called when the consumer drops a held account
  row (`natsclient/storagereport/storage_report_consumer.go:66-71`), and its accepted design names this type as the one
  implementer: "The one implementer is the pin's `service` metrics surface, which already deletes per-tier series"
  (`openspec/changes/archive/2026-10-17-simplify-04a-13-natsclient-storage/design.md:73-74`, D3, #85). D4.
- **O2, two registration paths and a method nothing calls (required: surface audit; the `RegisterOrGet` review rule).**
  `:351` — `func (s *StorageObservabilityService) RegisterMetrics(registrar metric.MetricsRegistrar) error {`, and the
  constructor's second registration `:243-253`, commented "nothing in the framework calls that method today". PR 1
  dropped `RegisterMetrics` from `Service` (ledger `service` row, known_risks), so the method has no reader.
  `_metrics.go:309-336` registers through the removed `Register*` methods and keeps its own candidates; `AGENTS.md`
  § 1: "Callers use the collector `metric.RegisterOrGet` returns, never their own candidate". D4.
- **O3, exported with no reader (required: surface audit (a)).** `:360` — `func (s *StorageObservabilityService)
  Snapshot() natsclient.StorageReportSnapshot {`, "for surfaces built on top of this service". Its one reader is a test
  (`storage_observability_integration_test.go:343`). Pin-wide, outside `service`, no Go file names
  `StorageObservability`, `StorageReportResponse`, `StorageReportSummary` or `storage-observability` except comments
  in `natsclient/storage_*.go` and `graph/constants.go:86` (`grep -rn --include='*.go'` on the pin copy); in the four
  consumer tarballs the only hit is the bucket name `STORAGE_REPORT` in semsource's cutover list
  (`internal/cutover/buckets.go:37`) and one skill line; no consumer enables the service. No contract line offers
  `Snapshot`. Dropped (D1); the route serves the same read.
- **O4, shape 4: reader-function seams for tests (optional, unexported).** `:477-480` — `type storageHealthSurface
  struct {` / `snapshotOf  func() natsclient.StorageReportSnapshot` / `inventoryOf func() natsclient.StorageInventory`,
  with nil branches `:483-485`, `:650-652` that no production value reaches; `_http.go:122-125` — the same for the
  route, with `:152-154` and a nil-logger fallback `:159-162` (`logger = slog.Default()`). The comments give the reason:
  rendering "is exercised directly ... without standing up an account" (`:472-476`, `_http.go:118-121`). Production
  sets each field once, to `consumer.Snapshot` and `collector.Latest` (`:211`, `:216`, `:237`). Optional (Bounded port
  scope); recommended in D5 as plain functions of the values, which the tests call directly (PR 4 M2 precedent).
- **O5, a missing metrics registry is accepted and the gauges reach no scrape (optional).** `:243` — `if
  deps.MetricsRegistry != nil {`; with no registry the observer still updates its gauges, which nothing registers
  (probe § 3: `nil MetricsRegistry: err=<nil>`). The pin's own comment calls an unregistered metric "the phantom-signal
  class this capability exists to remove" (`:245-249`). Trigger: a composition that omits `MetricsRegistry` (#185:
  MEDIUM at most); the service manager passes its own dependencies. Recommended refused at construction (D3; PR 4 M6
  precedent).
- **O6, the route refuses `HEAD` (optional).** `_http.go:138` — `if r.Method != http.MethodGet {`: probe § 3, `HEAD
  /report: 405`. Every message-logger route answers `GET` and `HEAD` through a `GET` pattern
  (`service/message_logger_http.go:39`). Recommended in D5: `mux.HandleFunc("GET "+prefix+"report", ...)`, so
  ServeMux answers 405 for other methods.
- **O7, documented, not implemented (required: text, PR 3 C12 precedent).** `_http.go:210-217` — "the generator writes
  each registered spec's paths at the document ROOT, unprefixed (cmd/openapi-generator/openapi_builder.go)" and
  `specs/openapi.v3.yaml`: neither exists in this tree, where `/openapi.json` takes paths from each mounted
  `HTTPHandler` and schemas from registered specs (`openspec/specs/operator-surface/spec.md:82-84`). The spec has no
  `SchemaRef`, so nothing needs registering. Rewritten in D5.
- **O8, schema tags with no reader (required: surface audit (c); PR 4 precedent).** `:87` — the tag
  `schema:"type:string,description:How often the JetStream account is enumerated,default:1m,category:advanced"` on
  `Interval`, and `:91`, `:96`. Nothing in
  `service` reads a `schema` tag (`service/schema.go:88`, `:102` read `json` and `description` only; `ConfigSchema`
  left with PR 4); the defaults they state are a second spelling of `:111`, `:115`. Dropped from this file (D3); the
  tags inside `storagereport.StoragePressureThresholds` are #281's, not this package's.
- **O9, an example alert rule set the tests read and the tree lacks (owner question Q1).**
  `storage_observability_http_test.go:450` — `const storageAlertRuleFile =
  "../configs/prometheus/rules/storage-pressure.yml"` (339 lines at the pin, 13 alerts, 22 lines naming
  `semstreams_storage_*`). Four tests read it (`:490-624`): every metric a rule names is published; no rule keys on a
  series disappearing; unbounded resources are named; every rule is annotated report-only. Pin ADR-098 lists
  "alerting rules" among the report's consumers (`098:92-94`); 03B ruled metric names a tested contract with a
  checked-in dashboard definition (03B `design.md:631-636`, carried to #9 item 5).
- **O10, construction errors are not classified (optional).** `:123`, `:130`, `:146`, `:149`, `:179`, `:188` —
  `fmt.Errorf(...)`; the logger and component manager refuse as invalid data (`service/message_logger.go:36-40`, `:87`).
  Recommended `errs.WrapInvalid` (D3).
- **O11, the configured interval spelled twice (optional).** `:364-369` — `collectionInterval()` re-derives the default
  from the raw string, and `config` (`:159`) is kept only for it, while `applied` already holds the duration
  (`:110-138`). Recommended: keep the applied values only (D3).
- **O12, retained, recorded (optional, no named trigger beyond a slow server).** `:390-403` — `lazyReportBucket.resolve`
  holds its mutex across `EnsureCatalogBucket`, so the consumer's watch retry waits behind a collector's acquisition
  without its own context; `Stop` is still bounded by its context, and a later `Stop` waits again (D2). The health
  line before the first collection reads "last collection did not succeed (no successful collection yet); serving last
  known result" (probe § 3), which is true but reads like a failure on a normal boot.

Not findings: `StorageObservabilityConfig`, `StorageObservabilityService`, `NewStorageObservabilityService` (the
configuration and the constructor `RegisterAll` registers); `StorageReportResponse`, `StorageReportSummary` (the
route's wire shape, `_http.go:58-114`); `Health`, `RegisterHTTPHandlers`, `OpenAPISpec` (the `Service` and
`HTTPHandler` methods the manager calls); `lazyReportBucket` and `lazyReportPublisher` (the boot-safe acquisition the
file comment promises, `:33-38`; each has one production use). `storagereport.InventoryPublisher` with one
implementation is `storagereport`'s retained finding (simplify-04a-13 inventory), not this package's.

Searched and empty in these three files: new roots or context values (`grep -n -E 'context\.Background|context\.TODO|
WithoutCancel|WithValue|ctx\.Value|select \{\}'`: 0 hits; the one derived context is `:287`); a `context.Context`
field: none (`cancel` `:169` is a `CancelFunc`); shape 5 (`strings\.Contains\(err|err\.Error\(\)`: one hit,
`_http.go:163`, which only logs the error); `GenericJSON`: 0; `init()`: 0 (the OpenAPI spec is not registered at
import, `_http.go:210-217`).

## 3. Pin probes (inventory § 7 left `StorageObservabilityService` "not run")

Two probes, 2026-10-22, outside the repository: a copy of the scratch pin with this tree's
`internal/harness/lifecycletest/lifecycletest.go` copied to `internal/lifecycletest` (as PR 4's probe), and two test
files, `/tmp/pr5probe-src/zz_so_probe_test.go` and `/tmp/pr5probe-src/zz_so_hold_probe_test.go` (local only; the
orchestrating session posts both with this output on #376). go1.26.10 darwin/arm64. The service is built with
`NewStorageObservabilityService` over a `natsclient.Client` that is never connected (`NewClient("nats://
probe.invalid:4222")`), so every collection and every watch fails; `Observe` reports the base status and whether the
loop join (`loopDone`) is still open.

**Suite and behaviour probe.** Command: `GOFLAGS=-mod=mod GOPROXY=off go test -race -count=1 -run 'TestProbeStorage'
-v ./service/`. Output (INFO and WARN log lines removed):

```text
--- FAIL: TestProbeStorageSuite (0.12s)
    --- PASS: TestProbeStorageSuite/NilContextsRefused (0.00s)
    --- PASS: TestProbeStorageSuite/PreCancelledStartRefused (0.00s)
    --- PASS: TestProbeStorageSuite/StopBeforeStartSafe (0.00s)
    --- FAIL: TestProbeStorageSuite/StartContextEndStopsNothing (0.10s)
    --- PASS: TestProbeStorageSuite/StopReturnsItsEndedContext (0.00s)
    --- PASS: TestProbeStorageSuite/RepeatedStopIsNoOp (0.00s)
    --- PASS: TestProbeStorageSuite/SecondStartRefusedOrRestartCycle (0.00s)
    --- PASS: TestProbeStorageSuite/FailedStartHoldsNothing (0.00s)
    zz_so_probe_test.go:67: loops ended after Start ctx cancel; status=stopped
    zz_so_probe_test.go:82: {"intervall":"1s"}: accepted, interval=1m0s
    zz_so_probe_test.go:79: {"Interval":"7s"}: err=invalid storage-observability config: timeout (15s) must not exceed interval (7s)
    zz_so_probe_test.go:82: {"pressure_thresholds":{"warn_headroom":0.3}}: accepted, interval=1m0s
    zz_so_probe_test.go:92: health right after Start: healthy=true status=healthy message="Service operating normally; storage report has not been read yet; last collection did not succeed (no successful collection yet); serving last known result"
    zz_so_probe_test.go:98: GET /report: 200 body="{\"report_only\":true,\"synced\":false,\"summary\":{\"resources\":0,\"pressure\":{},\"not_evaluated\":0},\"resources\":[]}\n"
    zz_so_probe_test.go:98: HEAD /report: 405 body="Method not allowed\n"
    zz_so_probe_test.go:98: POST /report: 405 body="Method not allowed\n"
    zz_so_probe_test.go:102: health after Stop: healthy=false status=unhealthy message="Service is unhealthy (failed checks: 0); storage report has not been read yet; last collection did not succeed (StorageInventoryCollector.Collect: resolve the account stream lister failed: Client.JetStream: get JetStream context failed: JetStream not initialized); serving last known result"
    zz_so_probe_test.go:103: second Start after Stop: err=storage-observability instance already stopped; create a new instance
    zz_so_probe_test.go:109: nil MetricsRegistry: err=<nil>
FAIL    github.com/c360studio/semstreams/service    0.719s
```

The failing check's message: `after the Start context ended the owner stopped holding [base-status=running loops]`.
Read: S1 holds for this service (both loops and the base service end with the Start context); S6 (a misspelt key is
dropped and the default applies; `Interval` in another letter case is applied, which here surfaced only because 7 s
is below the default timeout; a nested misspelt threshold is dropped); a `Start` that cannot read the account still
returns nil and reports healthy (the boot-safety promise, `:33-38`); O6; O5.

**Held-collection probe.** A `slog.Handler` holds the first record whose message starts "storage inventory collection
failed" until released; the first `Stop` runs under a 200 ms deadline, a second `Stop` runs on a goroutine and is
observed for 200 ms, then the hold is released. Command: `go test -race -count=3 -run
'TestProbeStorageStopWhileCollectorHeld' -v ./service/`. Output, three runs alike:

```text
    zz_so_hold_probe_test.go:59: first Stop=wait for storage observability loops: context deadline exceeded; second Stop still waiting while held; after release: <nil>; third Stop: <nil>
--- PASS: TestProbeStorageStopWhileCollectorHeld (0.41s)
```

Read: at the pin `Stop` waits for the loops, returns its context's error when the context ends first, and a later
`Stop` waits again. So pin ADR-095's "a failed Stop is final", which PR 3 and PR 4 had to change, does not apply here;
D2 keeps this behaviour. The collector logs through the injected logger, so the handler hold reaches the collector's
goroutine with no hook (D7 uses it).

Not re-run: the unit runs (once under `-race`, five times shuffled), the integration run and the test-text guards are
the inventory's § 7, run on the whole package; their hits in this PR's tests are § 1's two rows.

## 4. Open issues and pull requests

Queries, 2026-10-22, `gh issue list --state open --search <q> --limit 200`: `storage_observability`,
`StorageObservability` (no hits); `storage-observability`, `storage observability` (#195); `storagereport` (#281);
`STORAGE_REPORT` (#9); `storage report` (#321, #20, #9); `lifecycleguard` (#309, #314, #196, #361); `natsfixture.Open`
(#195); `kvcatalog` (#294, #195); `ForgetAccount`, `ADR-098` (none); `RegisterOrGet` (#68, #109, #9); `alert rules`
(#110, #9); `prometheus` (#68); `dashboard` (#9); `service` (#30, #363, #344, #196, #364, #362, #195, #361, #280, #9,
and #10, #11, #369, #29, #36, #109, #68, #7).

- **#195** (`natsfixture.Open` for each package's test client) names "storage-observability's test files" (its
  comment of PR #343). Stays open: this PR adopts `fixtureClient` for them (D7) and comments; other packages remain.
  Not `Closes`.
- **#196** (lifecycleguard adoption) names the component manager and the logger, not this file; the inventory's S3
  lists `storage_observability.go:167` as the same shape. This PR composes the guard (D2) and comments. Stays open for
  changes 4-7. Not `Closes`.
- **#281** (`component`: `GenerateConfigSchema` drops a field whose tag it refuses) names
  `natsclient/storage_pressure.go:82`, `:86`, `:91`, now in `storagereport`, a package this PR does not carry. Stays,
  in `component`; O8 drops only this file's tags.
- **#9** item 5 (a checked-in dashboard definition with a drift test) is adjacent to O9; Q1 says whether the alert
  rules land here. #9 is the epic; not retired.
- **#309** (lifecycleguard: a cleanup that panics) is a defect in the guard this PR composes; no cleanup here panics.
  Stays.
- Name the word, not the package: #321, #20 ("storage report" in other senses); #314, #361 (lifecycleguard ledger text,
  semsource's context); #294 (kvcatalog write guards); #68 (no `sync.Once` in these files), #109, #110; #10, #11, #7
  (epics); #30, #363, #344, #364, #362, #280, #369, #29, #36 (other `service` owners or other packages; none names a
  storage-observability file).

Open pull requests (`gh pr list --state open --json number,title,isDraft,files`, 2026-10-22): **#376**, this claim (3
files); **#374**, Gemini's draft `gemini/port-status-tracker`, which edits `docs/repository-map.md` (lines 10-17 and a
new `scripts/port-status.sh` row) where task 3.1 here edits the `service/` and `internal/lifecycleguard/` rows (`:55`,
`:78`), and whose `scripts/port-status.sh` prints the `service` split from the ledger; **#358**, dependabot
`prometheus/client_golang` 1.24.1 → 1.25.0 (`go.mod`, `go.sum`). Merge order in design D8.

## 5. Pin decision records for the three files (inventory § 8)

Searched the 85 records at the pin (`grep -l -i -E 'storage.observability|storage_observability|StorageObservability|
storage report|STORAGE_REPORT|storage pressure|report-only'`): 074, 098. Governing or cited:

- **ADR-098** (Accepted 2026-08-22), Decisions 3 and 4: "Substrate observability is logs and metrics, out of band",
  which must be "complete, well-labelled, and alertable" (`098:58-62`); "The framework does not decide what is
  operationally concerning ... publishes classified facts and refuses to enforce on them" (`098:64-67`); and
  "Storage-observability is unaffected and its shape is affirmed: a service that collects, derives, and publishes,
  with Prometheus metrics, alerting rules, health, and an HTTP route as consumers. It is the model for decision 3"
  (`098:92-94`). Binding on this service; not in this tree's `docs/adr`. Carried here (D8).
- **ADR-074** names "report-only" only as a runtime mode of another subject (`074:99`); not governing.
- **ADR-068 D1** (not in the pin's `docs/adr`; inventory § 8): the report bucket carries no binding TTL; it is
  `graph/kvcatalog`'s descriptor (`graph/kvcatalog/kvcatalog.go:96`), already ported. Kept, untouched.
- **ADR-095** (one-shot lifecycle): kept; its final failed Stop, which PRs 3 and 4 changed, does not arise (probe § 3).
- The pin's design record for this code is `openspec/specs/storage-observability/spec.md` (7 requirements), not ported
  (simplify-04a-13 inventory `:102`). Its service-level requirements ("Pressure state MUST be derived and reported
  without enforcement", `:139-167`; "Operators MUST get an actionable storage report", `:251-283`) are carried as this
  change's spec deltas (D9); the capacity-admission deferral gate (`:169-218`) is carried as README text (D8).
- This repository: ADR-113, ADR-108 Decision 2, ADR-109 (`/readyz` reads health, which pressure never touches),
  ADR-110 (the service writes no `ENTITY_STATES` entry; it owns its own `graph/kvcatalog` bucket), ADR-112 (the
  natsclient calls `storagereport` makes).

Pin guidance: the one hit outside docs, `.agents/skills/entity-or-bucket/SKILL.md:30` (`STORAGE_REPORT` keeps ten
revisions because they are a growth series), is already carried (`.agents/skills/entity-or-bucket/SKILL.md:28`).
Operator pages that describe the service (`docs/operations/35-storage-pressure-runbook.md`) are sources for the README,
not carried.

## 6. Pin tests (2,053 lines)

| File (lines) | What it reaches |
| --- | --- |
| `storage_observability_test.go` (839) | row builders `:24-73` (`UnknownGrowth` `:36`); gauge helpers `:75-114` (already moved); `staticSnapshot` `:116-118` (O4); config `:128-224` (`Default*` `:189-190`); registration `:226-241`; health `:243-483`, the readiness test `:284-344` building the service as a struct literal (`:296-301`) and driving manager internals (`:307-317`); metrics `:485-839` (`UnknownCapacity` `:737`, `:759-766`; `RegisterIsIdempotent` `:777-786`, O2) |
| `storage_observability_http_test.go` (624) | a fake `ReportWatchStore` `:24-85`; `unknownRow` `:87-97` (`UnknownCapacity` `:89-90`); the surface helpers `:99-120` (O4); response shape and route `:128-345`; one consumer feeding both surfaces `:346-445`; the alert rules `:448-624` (O9) |
| `storage_observability_integration_test.go` (346) | the whole wire on a real server `:33-316` (`NewTestClient` `:35`); boot without a reachable bucket `:318-346` (`NewTestClient` `:322`, `time.Sleep` `:339`, `Snapshot` `:343`, O3) |
| `storage_observability_tier_metrics_test.go` (244) | tier builders `:21-59`; tier gauges `:61-149` (`UnknownGrowth` `:96`); inherited pressure in health `:151-222`; inherited pressure in the gauge `:224-244` |
