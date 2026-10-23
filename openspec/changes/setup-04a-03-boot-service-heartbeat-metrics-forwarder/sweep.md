# Shape sweep: setup-04a-03-boot-service-heartbeat-metrics-forwarder (PR 6 of 7)

base: `ed174de3adbbdd46ac51e83ef9fd4e14e74e646a` (`main` `a1c5f21` plus the claim commit; draft PR #383)

The per-PR read the architect contract asks for (§ The shape sweep), of the two files this pull request carries, on a
scratch copy of the SemStreams pin `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` (`gh api .../tarball/8b99efe9`, local
only). It re-uses the whole-package inventory, `INVENTORY PASS` round 2 at `ad946b7`
(`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md`, "the inventory" below), wherever
that already pins a line, and adds what it did not pin. Disposition is proposed here and settled in `design.md`. Pin
lines are `service/<file>:<line>`; `hb:n` is `heartbeat.go`, `mf:n` is `metrics_forwarder.go`.

Files (non-test pin lines, 751 in all): `metrics_forwarder.go` 471, `heartbeat.go` 280. Test files 1,561 lines:
`metrics_forwarder_test.go` 1,091, `heartbeat_test.go` 470 (§ 6).

## 1. Findings the inventory already pins (cited, not repeated)

| Id | Shape | Pin lines in these files | Disposition |
| --- | --- | --- | --- |
| S1 | loop on the Start context itself | `hb:168` — `go hb.heartbeatLoop(ctx)`; `mf:218` — `go mf.publishLoop(ctx)` | required (ADR-113; probe § 3: both fail `StartContextEndStopsNothing`); design D2 |
| S3 | copied lifecycle state machine | `hb:62-73`, `mf:131-143` (`ticker`, `stopChan`, `stopOnce`, `wg`, `loopDone`), hand checks `hb:135-143`, `mf:197-206` | optional for these owners (the inventory's S3); recommended in D2: every owner ported so far composes `internal/lifecycleguard`, and the S1 fix rewrites this state anyway |
| S6 | plain config decoder; unknown keys accepted | `hb:84` — `if err := json.Unmarshal(rawConfig, &cfg); err != nil {`; `mf:28`, `mf:37` | required here: #344 (bug, `status:blocked` on this PR; its 2026-10-21 comment names this PR as the one that closes it) and probe § 3; D3 |
| S8 | drops logged at Debug only | `mf:376` — `mf.logger.Debug("failed to marshal metrics entry", ...)`, `mf:382`, and the gather failure `mf:294` | required: "any skip, drop, or degrade is declared" (`AGENTS.md` § 1); the inventory left the level to this design; D4 |
| S9 | test-only code in production | `hb:257-280` `newHeartbeatServiceForTest`; `mf:149-188` `newMetricsForwarderWithPublisher` ("for testing"), `mf:428-436`, `mf:438-471` | required: dropped (back-door rule, #186, #190); D1 |
| S10 | one-implementation interfaces | `mf:17-21` `MetricsGatherer` (exported, no reader outside `service`); `mf:73-77` — `publisher, ok := interface{}(deps.NATSClient).(natsPublisher)` (always true; `natsPublisher` is not in this tree, `service/dependencies.go:1-31`); `hb:44-47` `componentHealthGetter` | required for `MetricsGatherer` (surface audit (a)); optional for the other two, recommended in D1 |
| S12 | the component manager found by its literal name | `hb:147` — `componentService, exists := hb.serviceManager.GetService("component-manager")` | optional; D2 uses `componentManagerName` (`service/service_manager.go:28`) |
| § 7 | `time.Sleep` in a test | `heartbeat_test.go:365`; `metrics_forwarder_test.go:261`, `:317`, `:410`, `:460`, `:587`, `:633`, `:678`, `:685`, `:837`, `:953` | required (harness rules); D6 |

**What the scratch build needs beyond the module path.** `natsPublisher` is gone from the tree, so `mf:74` and the
test-only constructors do not build; `NewBaseServiceWithOptions` lost its config argument (PR 1's Q1), so `hb:111`,
`hb:270`, `mf:169`, `mf:458` change; `validateLifecycleContext` (`hb:132`, `hb:186`, `mf:194`, `mf:236`) does not exist
(the guard refuses nil and ended contexts). Each is a row above or S3.

## 2. Findings added by this sweep

Each is `path:line` — the line's text at the pin, then the shape and the proposed disposition.

- **H1, the injected logger is ignored (optional; recommended).** `hb:118` — `logger: slog.Default().With("source",
  "heartbeat"),` and `mf:184` — `logger: slog.Default().With("component", "metrics-forwarder"),`. `deps.Logger` reaches
  only the embedded `BaseService` (`hb:107-109`, `mf:66-68`); every record either service writes goes to the process
  default. Probe § 3: with `Dependencies{Logger: ...}`, the "System heartbeat" and "MetricsForwarder started" records
  reach the default logger and never the injected one. The pin's tests reach the output by assigning the private field
  (`heartbeat_test.go:396`, `:425`), a test swapping a dependency under the object. Shape 1 (two loggers for one
  service). D1 uses `deps.Logger`, as PR 5 does (`service/storage_observability.go:205-208`).
- **H2, nil dependencies panic the forwarder's constructor (optional, #185 MEDIUM at most).** `mf:57` — `if
  deps.NATSClient == nil {`. Probe § 3: `NewMetricsForwarderService(nil, nil)` panics with a nil dereference. The
  service manager always passes non-nil dependencies (`service/service_manager.go:150-157`), so no composition reaches
  it. Recommended refused as invalid (D3; PR 5 precedent `service/storage_observability.go:189-192`).
- **H3, "empty push interval" spelled twice, with a second decode (optional).** `mf:33-44` decodes the payload a
  second time into a map only to refuse `"push_interval": ""`, which `mf:100-103` also refuses, while `mf:47-49`
  turns an absent key into the default. `hb:90-92` and PR 5's `storageDuration`
  (`service/storage_observability.go:137-149`) treat an empty duration as the default. Recommended: one parse for all
  three services, empty means the default (D3).
- **H4, configuration kept twice (optional).** `hb:53` `config HeartbeatConfig` beside `hb:54` `interval`, read only for
  a log line (`hb:163`); `mf:120` `config` beside `mf:129` `pushInterval`. The exported `Validate` methods (`hb:25`,
  `mf:99`) have no reader outside the constructors (pin-wide `grep -rn --include='*.go'` for `HeartbeatConfig`,
  `MetricsForwarderConfig`: no hit outside `service/`; none in the four consumer tarballs). Recommended: keep only the
  applied values, `Validate` folded into an unexported `applied()` (PR 5 O11 precedent).
- **H5, a second start time (optional).** `hb:55` `startTime time.Time`, set `hb:161`, read for the uptime `hb:232`,
  beside `BaseService`'s own (`service/base.go:74`, read by `GetStatus().Uptime`, `service/base.go:259-268`). D4 reads
  the base's.
- **H6, the forwarder stops reporting running while it keeps publishing (part of S1).** `mf:274-281` watches only
  `stopChan`, not the context, while `BaseService` stopped its status with the Start context at the pin. Probe § 3:
  after the Start context ended the forwarder held `[status=running]` no longer, its loop still running. D2.
- **H7, a gather failure drops the whole round (optional; declared by S8's count).** `mf:292-296` — `metricFamilies,
  err := mf.gatherer.Gather()` / `if err != nil {` ... `return`. Retained: what the gathered slice holds beside an
  error is the Prometheus client's to say, and no design statement here depends on it (D4).
- **H8, a label value spliced into the subject (optional, retained).** `mf:351-361` — `component := "system"` /
  `if comp, ok := labels["component"]; ok {` / `subject := fmt.Sprintf("metrics.%s.%s", component, name)`. A
  `component` label holding `.`, `*`, `>`, a space or nothing makes a deeper or malformed subject. No named trigger:
  the framework's own `component` labels are instance names. Whether nats.go refuses a malformed subject on publish is
  not promised; either way the failure is counted (D4). Recorded in D4, not fixed.
- **H9, text with no home here (required: text).** `mf:263-268` cites "e2e dataflow flake — see
  project_websocket_flake_diagnosis", a SemStreams note; `mf:327`, `mf:337` say application metrics are
  `semstreams_*` (this tree's namespace is `semengine`, ledger `service` row, PR 2); `hb:1`, `mf:1` each carry a second
  `// Package service` comment (`service/doc.go` is the package's). Rewritten (D1).
- **H10, construction errors are not classified (optional).** `hb:85`, `hb:96`, `mf:29`, `mf:53`, `mf:58`, `mf:61` —
  `fmt.Errorf(...)`. Recommended `errs.WrapInvalid` (D3; PR 5 O10 precedent).
- **H11, the heartbeat's only output is invisible by default (owner question Q1).** `hb:249` —
  `hb.logger.Debug("System heartbeat",`. The service is enabled by `config`'s defaults (`config/config.go:495-498`)
  and by semsource (`cmd/semsource/run.go:1117-1120`), and at the default Info level it writes one line at Start and
  nothing after.
- **H12, a Start-time check production never reaches (optional).** `hb:144-146` refuses a nil service manager at
  `Start`; the manager binds itself into every constructor's dependencies (`service/service_manager.go:150-157`).
  Recommended: refuse at construction (D3); the component-manager lookup stays at `Start`, because `StartAll` may
  create the component manager after the heartbeat is constructed (`service/service_manager.go:411-425`).

Not findings: `HeartbeatService`, `MetricsForwarder` (the constructors' concrete types); `HeartbeatConfig`,
`MetricsForwarderConfig` (the configuration shapes); `NewHeartbeatService`, `NewMetricsForwarderService` (registered by
`RegisterAll`); `Start`, `Stop`, `Health` (the `Service` methods the manager calls). The raw JSON map `mf:364-371` is one
fixed shape on one subject family, so neither the `BaseMessage` rule nor the `GenericJSON` rule applies (the
inventory's S8, "Not in scope").

Searched and empty in these two files: new roots or context values (`grep -n -E 'context\.Background|context\.TODO|
WithoutCancel|WithValue|ctx\.Value|select \{\}'`: 0 hits); a `context.Context` field: none; shape 5
(`strings\.Contains\(err|err\.Error\(\)`: 0); `GenericJSON`: 0; `init()`: 0; OpenAPI or HTTP routes: none.

## 3. Pin probes

The inventory's § 7 ran the lifecycle suite on both owners and found each failing `StartContextEndStopsNothing`
only. Re-run 2026-10-23 with an `Observe` that also reports the loop's join channel, plus the configuration and logger
checks above. Scratch: a copy of the pin with this tree's `internal/harness/lifecycletest/lifecycletest.go` at
`internal/lifecycletest` (as PR 5's probe), and `/tmp/pr6probe-src/zz_pr6_probe_test.go` (local only; the
orchestrating session posts it with this output on #383). go1.26.10 darwin/arm64. The heartbeat is built through a
`Manager` holding the pin's `mockComponentHealthGetter` as `component-manager`; the forwarder through
`newMetricsForwarderForTest` with the pin's mock publisher. Command: `GOFLAGS=-mod=mod GOPROXY=off go test -race
-count=1 -run 'TestPR6Probe' -v ./service/`. Output (log lines removed):

```text
--- FAIL: TestPR6ProbeSuites/heartbeat/StartContextEndStopsNothing (0.10s)
    lifecycletest.go:124: after the Start context ended the owner stopped holding [status=running loop]
--- FAIL: TestPR6ProbeSuites/forwarder/StartContextEndStopsNothing (0.10s)
    lifecycletest.go:124: after the Start context ended the owner stopped holding [status=running]
    (the other seven checks PASS for each owner)
zz_pr6_probe_test.go:94: forwarder {"push_intervall":"1s"}: accepted push_interval=5s
zz_pr6_probe_test.go:94: forwarder {"Push_Interval":"1s"}: accepted push_interval=1s
zz_pr6_probe_test.go:94: forwarder {"push_interval":"1s","Push_Interval":"9s"}: accepted push_interval=9s
zz_pr6_probe_test.go:97: forwarder nil deps: panic=runtime error: invalid memory address or nil pointer dereference
zz_pr6_probe_test.go:107: heartbeat {"intervall":"5s"}: accepted interval=30s
zz_pr6_probe_test.go:107: heartbeat {"Interval":"5s"}: accepted interval=5s
zz_pr6_probe_test.go:107: heartbeat {"Interval":"5s","interval":"30s"}: accepted interval=30s
zz_pr6_probe_test.go:151: heartbeat: default logger got System heartbeat=true; injected logger got it=false
zz_pr6_probe_test.go:161: forwarder: default got start=true injected got start=false
```

Read: S1 for both (the heartbeat's loop and status end with the Start context; the forwarder's status ends while its
loop keeps publishing, H6); S6 (a misspelt key is dropped for the default; a miscased key is applied, and beside the
correct spelling the later one wins, the #344 shape); H2; H1.

**Tree probe for #344.** `git archive` of the base with `/tmp/pr6probe-src/zz_pr6_tree_probe_test.go` (local only)
added to `service`: each document is loaded with `config.NewLoader().LoadFromBytes`, and the merged `heartbeat` payload
decoded both ways into a struct whose one field, `Interval`, is tagged `json:"interval"`:

```text
merged payload={"Interval":"5s","interval":"30s"}; json.Unmarshal interval="30s" err=<nil>; decodeStrictServiceJSON err=config key Interval must be written interval
merged payload={"interval":"30s","intervall":"5s"}; json.Unmarshal interval="30s" err=<nil>; decodeStrictServiceJSON err=json: unknown field "intervall"
merged payload={"interval":"30s"}; json.Unmarshal interval="30s" err=<nil>; decodeStrictServiceJSON err=<nil>
```

Read: the loader's merge creates the second spelling (#344's body), and the decoder PR 2 already gave every strictly
decoded service payload (`service/service_configs.go:136-152`, `internal/keycase`) refuses it, naming both spellings.
A second tree probe, `zz_pr6_tree_probe2_test.go`: a composition built from a loaded document's default services with
`RegisterAll` fails today with `create configured service heartbeat: no constructor registered for service heartbeat`
(D7).

Not re-run: the unit runs (once under `-race`, five times shuffled), the integration run and the test-text guards are
the inventory's § 7, run on the whole package; their hits in this PR's tests are § 1's last row.

## 4. Open issues and pull requests

Queries, 2026-10-23, `gh issue list --state open --search <q> --limit 200`: `heartbeat` (#344, #216, #9);
`metrics forwarder`, `MetricsForwarder`, `metrics-forwarder`, `forwarder`, `HeartbeatService`, `ADR-098` (#9);
`metrics_forwarder`, `push_interval`, `natsPublisher`, `MetricsGatherer` (none); `metrics.>` (issues #196, #68,
issue #344, #109, #362, #9, #198); `service` (issues #30, #363, #196, #344, #364, #362, #195, #361, #280, #378, #9,
issue #10, #11, #29, #36, #109, #68, #7); `lifecycleguard` (issues #309, #314, #196, #361).

- **#344** (a miscased key in the heartbeat's settings is overridden by the default) names `heartbeat` and this port.
  **Retired by this PR** (`Closes #344`; the PR body already declares it): task 2.2 decodes the heartbeat payload with
  `decodeStrictServiceJSON`; task 2.1's `TestHeartbeatRefusesAMiscasedKeyBesideTheDefault` is the proof (D3).
- **#196** (lifecycleguard adoption) names the component manager and the logger; the inventory's S3 lists `hb:65-73`
  and `mf:139` as the same shape. This PR composes the guard for both owners (D2) and comments. Stays open for
  `LogForwarder` (PR 7) and changes 4-7. Not `Closes`.
- **#195** (`natsfixture.Open` for each package's test client) names `service`; the pin's tests here use no
  `NewTestClient`. The forwarder's integration test uses `fixtureClient` (D6); a comment, not `Closes`.
- **#364** (collapse the `componentStartup` seam once all service PRs land) is adjacent: D1 has the heartbeat assert
  `*ComponentManager`, adding no reader of that seam. Stays, for after PR 7.
- **#309** (lifecycleguard: a cleanup that panics) is a defect in the guard composed here; no cleanup here panics.
  Stays.
- Name the word, not the package: #216 (natsclient delivery heartbeat); #68 (no `sync.Once` metric registration here;
  the two `stopOnce` go with S3); #109, #362, #198, #363, #30, #29, #36, #280, #361, #378, #314 (other packages or other
  `service` owners); #9, #7, #10, #11 (epics).

Open pull requests (`gh pr list --state open --json number,title,isDraft,files`, 2026-10-23): **#383**, this claim (3
files); **#382**, draft `claude/subscribe-handler-root-378` (natsclient `Subscribe`, #378), no file pushed yet; its
target is `natsclient`, which this PR only calls (`PublishToStream`); **#381**, draft
`claude/spec-queue-stale-days-262`, `openspec/changes/fix-262-stale-days-operand/` with a `spec-queue` delta. No shared
file or capability with either. Merge order in design D8.

## 5. Pin decision records for the two files (inventory § 8)

Searched the 85 records at the pin (`grep -l -i -E 'MetricsForwarder|HeartbeatService|metrics-forwarder|
metrics_forwarder|heartbeat\.go|metrics\.>'`): 058 only. Read by title for lifecycle and substrate observability:
095, 098, 029.

- **ADR-058** (boot lifecycle phases; Draft; in this tree, `docs/adr/058-boot-lifecycle-phases.md`)
  names `HeartbeatService` the exemplar boot citizen: "`Start` launches its loop goroutine under its own `wg`; `Stop`
  signals + joins" (pin `058:53-56`, tree `:85-88`). Cited. The exemplar's mechanism (its own wait group, the loop on
  the Start context) is what ADR-113 replaces, already settled. Its port note's "Not yet ported ... `HeartbeatService`"
  (tree `:31-32`) goes stale with this PR; task 3.1 edits it.
- **ADR-098** (in this tree since PR 5, `docs/adr/098-observability-splits-by-subject.md`): Decision 3, "Substrate
  observability is logs and metrics, out of band" (`:80-81`); its port note names this pull request as citing it
  (`:7-8`). Governs both services. Cited, kept.
- **ADR-095** (one-shot running lifecycle; not in this tree's `docs/adr`): instances are single-use (`hb:128-130`,
  `mf:190-192`). Kept through the guard (D2).
- **ADR-029** (instance-type patterns): the Manager is passed to services as `ServiceManager` (inventory § 8, `029:31`,
  `:44`); the heartbeat's lookup reads it. Governs `dependencies.go` (PR 2); cited, not carried.
- This repository: ADR-113, ADR-108 Decision 2 (the guard), ADR-112 (the natsclient call the forwarder makes), ADR-110
  (neither service writes an `ENTITY_STATES` entry).

Pin guidance: no contract section or skill names either service (`grep -rn -i -E 'heartbeat|metrics.forwarder'
.agents/ docs/agents* 2>/dev/null` on the pin copy: no hit outside the ADRs above). Nothing to carry.

## 6. Pin tests (1,561 lines)

| File (lines) | What it reaches |
| --- | --- |
| `heartbeat_test.go` (470) | a mock health getter `:17-26` (S10); a channel log writer `:28-36`; a managed builder `:38-57`; `Validate` `:59-110` (H4); constructor `:112-157`; lifecycle `:159-257` (`StopNotRunning` on `newHeartbeatServiceForTest` `:207`, S9); with a component manager `:258-276`; Start refusals `:277-345`; Start-context end `:347-377` (`time.Sleep` `:365`; asserts the pin's self-stop, which ADR-113 reverses); the emitted record `:379-417` and `:419-459` (private `logger` swapped `:396`, `:425`, H1); `Name` `:461-470` |
| `metrics_forwarder_test.go` (1,091) | config `:27-127`, `:971-1022`; dependency refusals `:129-186`; lifecycle `:188-217`, `:697-809` (`StopAfterContextCancellation` `:761-781` asserts the pin's self-stop); cadence under `synctest` `:218-271` (`time.Sleep` `:261`); gather, subject, format, component, filtering `:273-607`, `:849-969` (all on the mock publisher and gatherer, S9, S10; the sleeps of § 1); publish and gather errors `:608-640`, `:810-848`; concurrent gather `:641-696`; helpers and mocks `:1024-1091` |
