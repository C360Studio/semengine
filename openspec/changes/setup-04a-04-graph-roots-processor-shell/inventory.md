# Inventory: setup-04a-04-graph-roots-processor-shell

base: `761503062f2405ffc4983d4a107b1b3b49380646` (origin/main after PR #407). Pin paths are SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, written `pin:<path>:<line>`, read from a local pin checkout (local only).

Status: not yet reviewed. No `INVENTORY PASS`; the design was drafted in the same pass at the caller's direction, as
on `2026-10-24-setup-04a-04-graph-roots-revlag`.

Kind of change: tree code, not a port. No pin package is carried, so the pin probe, the shape sweep of a carried
package and the per-package pin ADR listing (architect contract, Extraction slices, category 6) do not apply; the
governing ADRs are listed in section 6 because the brief asks for them. The pin is read only to check that the shell
fits its next three adopters (section 9).

Briefing hypotheses that did not hold:

- "The suite in `component/`": the lifecycle suite is `internal/harness/lifecycletest/lifecycletest.go:83` —
  `func Run(t *testing.T, factory Factory, mustFail Factory, promise Promise) {`; `component/` has no lifecycle test.
- "The guard already has property tests": `internal/lifecycleguard/guard_test.go` has nine example tests
  (`:109`-`:409`) and no generated check (`grep -n "rapid\|quick\." internal/lifecycleguard/guard_test.go`: empty).
- `processor/graph-ingest/component.go` is 2,643 lines (`wc -l`), not 2,659 or 2,665.

## 1. What exists that the shell replaces or composes

### The state machine

- `internal/lifecycleguard/guard.go:31` — `type Guard struct {`: one admitted Start, Stop safe before, during and
  after it, a failed cleanup kept for the next Stop. Fields `:33-44`; no `running` flag, no start time, no counters.
- `:55` — `func (g *Guard) Start(ctx context.Context, start, cleanup func(context.Context) error) (startErr error) {`
  (nil and ended contexts refused `:56-61`; second Start `:63-66`; rollback through
  `lifecyclecleanup.RollbackFailedStart` `:75`).
- `:105` — `func (g *Guard) Stop(ctx context.Context) error {` (doc `:101-104` states the open panic defect #309).
- Imports: `pkg/errs`, `pkg/lifecyclecleanup` only (`go list -f '{{join .Imports "\n"}}' ./internal/lifecycleguard`).

### The contract text

- `component/lifecycle.go:43-64`: the `LifecycleComponent` rules in plain words (ADR-108 D3(e), ADR-113).
- `component/lifecycle.go:66-74`: reads (Health, DataFlow, DebugStatus) answer at any time; tested only for
  graph-ingest.
- `component/lifecycle.go:76-78` — `// graph-ingest breaks the last rule today: it validates its configuration in` …
  "the second check leaves with simplification wave PR 10 or the processor shell (#109)".
- `component/discovery.go:65-72` — `type HealthStatus struct {`; `:83-88` — `type FlowMetrics struct {`.

### The suite

- `internal/harness/lifecycletest/lifecycletest.go:39-43` — `type Owner interface {` (Start, Stop, Observe); eight
  checks `:98-112`. Owners that run it today (`git grep -n "lifecycletest.Run" -- '*_test.go'`): the guard's test owner
  (`internal/lifecycleguard/guard_test.go:110`), graph-ingest
  (`processor/graph-ingest/lifecycle_integration_test.go:348`), `config.Manager`, `metric.Server`,
  `natsclient.Client`, and seven `service` owners.

### graph-ingest's inline lifecycle (all `processor/graph-ingest/component.go`)

| Range | What | Shell-owned under ADR-108 D3? |
| --- | --- | --- |
| `:441-450` | `mu`, `running`, `initialized`, `startTime`, `lifecycle lifecycleguard.Guard`, `startCompleted` | all but `startCompleted` and `mu` (ports) |
| `:516-520` | `messagesProcessed`, `bytesProcessed`, `errors`, `lastActivity` | yes (D3(c) counters) |
| `:696` | `comp.lastActivity.Store(time.Now())` at construction | yes |
| `:841-881` | `Health()`: running + stuck states (poison, backlog fault) → Healthy/Status/LastError | yes, less the stuck list |
| `:889-926` | `DataFlow()`: counts over uptime | yes |
| `:932-950` | `Initialize()`: idempotent, re-runs `config.Validate()` | yes; the re-validation goes (D3(d)) |
| `:965-976` | `Start`: initialized check, nil-client check, `c.lifecycle.Start(ctx, c.start, c.release)` | yes, less the client check |
| `:1039-1043` | `start` sets `running`, `startTime` | yes |
| `:1048` | `c.startCompleted.Store(true)`: request handlers refuse until set (`:1070`) | no: graph-ingest's request gate |
| `:1092-1094` | `Stop`: `return c.lifecycle.Stop(ctx)` | yes |
| `:1102-1114` | `release`: clears `running`, runs `cleanup`, clears handles, logs | the `running` clear only |
| `:1116-1160`, `:1162-1173` | `cleanup`, `clearLifecycleHandles` | no: graph-ingest's own release order (ADR-113) |

Counter writers: `component.go:1544-1546` and `canonical_mutations.go:40-42` (one input message, its bytes, last
activity); eight `atomic.AddInt64(&c.errors, 1)` sites at `component.go:1555`, `:1586`, `:2184`, `:2284`, `:2295`,
`:2331`, `:2534`, `:2540`. Readers of `c.running` outside Health/DataFlow/start/release: none (`git grep -n
"c.running" processor/graph-ingest ':!*_test.go'`).

### The service adopters

They compose the guard directly; ADR-108 D1 keeps them off the shell.

Production composers of `lifecycleguard.Guard` (`git grep -n lifecycleguard -- '*.go' ':!*_test.go'`):
`config/manager.go:57`, `service/base.go:84`, `service/component_manager.go:82`, `service/heartbeat.go:51`,
`service/message_logger.go:200`, `service/metrics.go:30`, `service/metrics_forwarder.go:54`,
`service/service_manager.go:43`, `service/storage_observability.go:150`, and graph-ingest. Each spells
`x.lifecycle.Start(ctx, x.start, x.cleanup)` and `x.lifecycle.Stop(ctx)` (`service/component_manager_lifecycle.go:33`,
`:148`; `service/message_logger.go:264`, `:520`). None answers `component.HealthStatus`; `service.BaseService.Health`
returns `health.Status` (`service/base.go:155`). The shell changes none of these files.

### Duplication, rule by rule (ADR-108 D3)

| Rule | graph-ingest today | Services today | What checks it |
| --- | --- | --- | --- |
| nil/ended ctx refused; second Start refused; Stop before Start safe; repeated Stop no-op | the guard, `:975`, `:1093` | the guard | suite on each owner; guard tests |
| failed Start rolled back; failed cleanup kept for next Stop | the guard + `release` | the guard | `TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop`; guard tests |
| ending Start's ctx stops nothing (ADR-113) | own run context `:983` | own run context (`service/base.go:200`) | suite `StartContextEndStopsNothing` |
| Start before Initialize refused, instance not used up | inline `:966-971` | n/a (services have no Initialize) | `TestComponent_Start_BeforeInitialize` (`component_test.go:668`) asserts only an error |
| Initialize idempotent, no I/O; config validated once | inline `:932-950`, validates twice (factory `:614`, Initialize `:941`) | n/a | `TestComponent_Initialize_InvalidConfig` (`component_test.go:657`) tests the duplicate check |
| Health from running + named stuck states; ErrorCount never lowers Healthy (ADR-109 D2) | inline `:841-881` | `BaseService` has its own, other type | `poison_scoping_test.go:88`, `health_integration_test.go:57-114`, `discoverable_reads_test.go:18` |
| reads answer during/after Stop; Stop's drain reads "stopped" | `release` clears `running` first `:1103-1105` | per service | `TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop` |
| DataFlow: inputs over uptime, error rate per input | inline `:889-926` | n/a | `component_test.go:614`, `TestIntegration_DataFlowCountsInputMessages` |

Only graph-ingest answers `Health()`/`DataFlow()` in production today (`git grep -n "DataFlow() component.FlowMetrics"
-- '*.go' ':!*_test.go'`: one hit, `component.go:889`). The duplication the shell removes is therefore future copies
(section 9), plus graph-ingest's private copy.

## 2. The claimed gap

"No shell exists": `git grep -n -E "type Shell|ProcessorShell" -- '*.go'` finds only YAML structs in
`internal/harness/contract/mergegate_test.go:330`, `:339`; `git grep -n "lifecycleguard" component/`: empty. No
open change under `openspec/changes/` (only `archive/`). The gap is real.

## 3. Adjacent claims

### Open pull requests

`gh pr list --state open --json number,title,isDraft,changedFiles,headRefName` returned `[]` on 2026-10-25. Issues #362
and #363 are unclaimed; their files are `service/stream_override_expiry.go`, `service/component_manager*.go` and
`composition` (#362 body; #363 body). The shell edits none of them: no overlap, no merge order to state.

### Open issues

Queries: `graph-ingest`, `lifecycleguard`, `component`, `processor shell`, `DataFlow`, `Health`, `lifecycle`,
`LifecycleComponent`, each as `gh issue list --state open --search <q> --limit 200`.

| Issue | What it says | Touches this change? |
| --- | --- | --- |
| #109 | shell + `ENTITY_STATES` follower, adopted by four followers; spatial/temporal plug-ins; `workers` removed | shell part only; follower is graph-index's PR (ADR-108 D1, D2; #9 comment 6386856657) |
| #306 | `clearLifecycleHandles` sets `c.entityCache = nil` (`component.go:1170`) while `invalidateEntityCacheEntry` (`:2370`, `:2376`), `repopulateEntityCacheEntry` (`:2396`, `:2402`) and `query.go:487-488` read it unlocked; double read at `query.go:487-488` can nil-deref | yes: the release path this change edits |
| #300 | sections G (split `component.go`) and D's file seams routed to #109 (owner, 2026-10-18, #300 comment); the rest parked | G/D only; optional maintenance (PR #299 D8) |
| #196 | guard adoption tracker; component rows adopt through the shell | graph-ingest row: comment, not close |
| #309 | a cleanup that panics leaves later Stops refused or waiting (`guard.go:101-104`) | the shell inherits it; no tree trigger (#309 body) |
| #301 | four test-only NATS fault fields on `Component` (`component.go:414-431`, comment and four fields) | no: their reasons are unchanged by the shell (#301 comment from PR #310) |
| #216 | graph-ingest stops receiving when its input stream is recreated | no: consumer rebinding in natsclient + graph-ingest; #291, its pair, is closed |
| #361 | semsource gives `StartAll` a context that cannot end | no: a consumer composition root; nothing here |
| #399 | ambiguous mutation-port refusal names no port (`canonical_mutations.go:121-122`) | no: config refusal text, another function |
| #110 item 3 | `HealthStatus.Status` as free text; vocabulary decided at temporal's port (#132 closed) | the shell holds graph-ingest's three words; does not settle the vocabulary |
| #280 | `pkg/lifecycle` one-field structs; storage doc | no |
| #270, #141, #195, #106, #107, #103, #108, #111, #15, #134, #20, #33, #198, #405 | name graph-ingest's word or other code | names the word or another package, not graph-ingest's lifecycle |
| others from the `component`/`Health` queries (#29, #281, #36, #340, #321, #105, #155, #35, #284, #32, #34, #27, #96, #68, #161, #162, #114, #350, #95, #94) | — | names the word, not the shell, guard or graph-ingest lifecycle |

### Specs and ledger rows

- `openspec/specs/lifecycle-suite/spec.md:13` (Portable floor), `:176` (Observe adapter: no `Observe` in production
  code), `:195`, `:214` (failed-cleanup branch proven per component).
- `openspec/specs/component-registration/spec.md:41` — `### Requirement: The component model reaches no agentic or
  graph package`.
- `openspec/specs/service-lifecycle/spec.md:600` — `### Requirement: A component's Health, DataFlow and DebugStatus
  answer at any time`.
- `openspec/specs/graph-ingest-recovery/spec.md:242` — `### Requirement: graph-ingest's data flow counts input
  messages`; its proving tests read exact totals: `dataflow_integration_test.go:109`, `:111` and
  `metrics_test.go:48-49`, `:55-56`, `:64` (`git grep -n -E "&[a-z]+\.(messagesProcessed|bytesProcessed)"
  processor/graph-ingest/*_test.go`; `discoverable_reads_test.go:28` is a write).
- Rule text that sends a ported component to the guard: `AGENTS.md:83`, `.agents/rules-index.md:28`,
  `.agents/contracts/semengine-developer.md:326`, `.agents/contracts/semengine-reviewer.md:344`. ADR-108 D1 sends
  components to the shell.
- `FlowMetrics` readers: `service/component_manager.go:575` builds the status record; `component_manager_http.go:341`
  and `:478` serve it, so `data_flow.last_activity` and any new field reach operators.
- `internal/cache/cache.go:49-50`: `Close`'s doc promises nothing about use after `Close`; no `internal/cache` test
  calls `Get`, `Set` or `Delete` after an explicit `Close` (`background_test.go:209-210`, `:278`, `:359`,
  `benchmark_test.go:344`; the rest are deferred or cleanup calls).
- `docs/admission-ledger.yaml:2267` (`component`, `adapt`, adaptations (1)-(14)), `:2325` (`processor/graph-ingest`,
  item (6) names `internal/lifecycleguard`, `:2341`), `:2381` (`processor/graph-ingest/component.go` row, destination
  "decodeEntity and consume closure").

## 4. Who it is for (new surface; the pin has no shell)

- One exported type in `component` and its methods. Production caller after this change: graph-ingest (plumbing),
  the only production `Discoverable` in the tree (section 1).
- Framework API by owner ruling: ADR-108 D1 (`docs/adr/108-processor-shell.md:64-65`: "public, as the one way a
  SemEngine component implements `LifecycleComponent`"), owner question 1 ruled yes (#9 comment 6097814764). Contract
  line to name: `component/README.md` (the `LifecycleComponent` paragraph, `:123-125`) and the new spec capability;
  a test calling it as a product would is owed (an example in package `component_test`).
- Two `FlowMetrics` fields for the exact totals (added in the design's revision): read by graph-ingest's exact-count
  tests and served by the status routes (section 3); contract line, the `component-shell` `DataFlow` requirement.
- External adopters (ADR-108 Consequences, measured there, not re-read): semboids' sim component writes its own
  lifecycle (`internal/sim/component.go:146`, `:546` at `8c03cc53`); semteams unmeasured.

## 5. The problem shape

Shape: one owner composes a state machine and derives status reads from it (a lifecycle base). Closest instance:
`service.BaseService` (`service/base.go:67-93`): composes `lifecycleguard.Guard` (`:84`), keeps status, start time and
counters, answers `Health()` (`:155`), is embedded by services (`service/message_logger.go:190`) and configured by an
options bag (`:64`, `:117-138`). Its `Health` returns `health.Status`, so components cannot use it (ADR-108 D1,
`:61-63`). Second instance: graph-ingest's inline copy (section 1).

## 6. Governing ADRs

| ADR | Decides | Bearing here |
| --- | --- | --- |
| ADR-108 D1-D3 | shell public in `component`, composes the guard, adds Health/DataFlow/counters; lands with change 4; owns D3's list | this change implements D1 and D3 for the shell half |
| ADR-108 D4-D7 | follower, plug-ins, spatial/temporal, one projection | not this change |
| ADR-113 | Start's ctx bounds Start; Stop the only stop; the owner makes and ends its run context; the guard holding the run context rejected (`:102-107`) | binds: the shell holds no context and no cancel |
| ADR-095 (pin, ported) | one Start per instance; failed Start's cleanup kept for the next Stop | kept, through the guard |
| ADR-109 D2 | Health = running and not stuck; ErrorCount never lowers Healthy | the shell becomes its one implementation |
| ADR-058 | construction-time (phase A) registration, runtime work in Start/Stop | kept: collectors stay registered at construction by the component; the shell registers none |
| ADR-096 | manager owns handles, not a component's state machine | kept: `component.ManagedComponent.State` (`component/lifecycle.go:93`) stays the manager's record |

## Same-class collision table

Triggered: a runtime-coordination primitive, a component's lifecycle state and its reads.

| Dimension | Evidence |
| --- | --- |
| Semantic class | whether one component instance is unstarted, running, stopping or done, and what its Health/DataFlow say |
| Owners | `internal/lifecycleguard.Guard` (`guard.go:31`); graph-ingest inline (`component.go:441-450`, `:516-520`); `service.BaseService` (`service/base.go:67`) for services; `component.ManagedComponent.State` (`component/lifecycle.go:88-105`) the manager's view |
| Catalogs | none: `git grep -n -i "lifecycle catalog"` finds only ADR text; ADR-095 forbids one |
| Status | `component.HealthStatus` (`discovery.go:65`), `FlowMetrics` (`:83`), `component.State` strings (`lifecycle.go:26-40`), `service.Status` (`service/base.go`), graph-ingest readiness envelope (`readiness.go`, not lifecycle) |
| Lifecycle | guard Start/Stop; graph-ingest `start`/`release`/`cleanup` (`:981`, `:1102`, `:1116`) |
| Ownership | one instance, one Start (ADR-095); no claims or leases |
| Readers | `service/component_manager.go:575` (`DataFlow`), `health.FromComponentHealth` (`health/status.go:95`), `/readyz` via component health |
| Writers | graph-ingest's start/release and counter sites (section 1) |
| Recovery | failed-Start rollback and retained cleanup (guard `:72-85`); no restart (ADR-095) |

## Adopter seam inventory

The shell is reached by component authors outside this repository.

1. **What must they know?** (a) Hold one shell per component and route `Initialize`, `Start`, `Stop`, `Health`,
   `DataFlow` through it. (b) Their start creates the run context and their cleanup ends it (ADR-113); the shell
   makes none. (c) Count inputs and errors on the shell, or DataFlow reads zero. (d) Their cleanup must tolerate a
   partial start and must not panic (#309). Four facts, over the two-fact line; (b) and (d) are settled below.
2. **If they do nothing?** Not composing the shell: as today (ADR-108 D3(e)). Composing it but skipping (c): zero
   rates, Health still right. Skipping (b): continuing work dies with the Start context; nothing outside this module
   catches it (the suite is internal, ADR-113 Consequences). A panicking cleanup: later Stops refused or waiting.
3. **Where do they find out?** (a) compile error for a missing method, else nowhere if they hand-write Health; (b)
   doc only; (c) visible as zero rates; (d) doc only.
4. **What should they have to know?** Supply start, cleanup and stuck states; count inputs. The gap is (b) and (d):
   correctness facts at "doc". (b) is ADR-113's recorded gap, caught inside this module by the suite. (d) is #309,
   optional under #185 (no real-world trigger); the shell's `Start` doc states it.

Prefer observation: counting inputs (c) could be observed only by wrapping the author's handlers, a second channel
beside natsclient's callbacks; not proposed.

## Context-retention inventory (lifecycle work)

`git grep -n -E "context\.(Context|CancelFunc)" processor/graph-ingest/component.go internal/lifecycleguard/guard.go
component/lifecycle.go` on struct fields: graph-ingest holds `cancel` (`:456`), `ingestPoolCancel`, `ingestSubmitCancel`
(`:550-551`), each the private cancel ADR-113 allows; the guard holds none (`guard.go:31-45`). Roots:
`component.go:983` — `runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))`, the one run context
ADR-113 permits. The shell must add none.

## 9. Fit with the next adopters (pin)

| Adopter | Lifecycle state | Health | DataFlow | Initialize |
| --- | --- | --- | --- | --- |
| `pin:processor/graph-index/component.go` | `:245-259` running, startTime, cleanupPending, counters; second Start on running returns nil `:620-622` | `:506-534` running && errorCount==0 | `:537-578` same as graph-ingest's | `:580-597` re-validates |
| `pin:processor/graph-query/component.go` | `:177-194` lifecycleUsed, cleanupPending, stopping, counters | `:348-370` started && NATS connected, lastError | `:382-397` counts over time since reset | `:399` gate on `initialized` (`:490-496`) |
| `pin:storage/objectstore/component.go` | `:49-52` lifecycleUsed, terminal, stopping, cleanupPending | `:1209-1226` started only | `:1229-1242` zeros | `:135-138` no-op |

Each Start/Stop is "start under ctx, cleanup under ctx" (graph-query `:480-595`, `:597-641`); each Health is running
plus at most one verdict; each DataFlow is counters over uptime or nothing. One shape fits all three. Their
differences (graph-index's nil second Start, its error-count term, graph-query's per-second rate and NATS term,
objectstore's missing Initialize gate) are behaviour changes their own ports carry as `adapt` items.

## 10. Size

- Shell: about 150-180 non-test lines with doc comments (D3's rules; the guard stays where it is).
- graph-ingest: about 110 lines leave or shrink (`:441-450`, `:516-520`, `:696`, `:841-950`, `:965-976`,
  `:1039-1043`, `:1103-1105`); ten counter sites change spelling.
- Tests: 14 graph-ingest test files set or read the moving fields (`c.running = true` 11 sites, `startTime` 4,
  `messagesProcessed` 1, `atomic.LoadInt64(&c.errors)` 14 reads in 5 files; searches `git grep -n -E
  "\.(running|startTime|messagesProcessed|errors)( |\)|,)" processor/graph-ingest/*_test.go` and `git grep -c
  "atomic.LoadInt64(&[a-z]*\.errors)"`), and 8 files reach the guard through `c.lifecycle` or its three helpers
  (`test_owner_support_test.go:80-125`). Most are setup lines. Assertions read the moving counters in two places:
  the 14 `errors` reads, and the exact message and byte totals at `dataflow_integration_test.go:109`, `:111` and
  `metrics_test.go:48-49`, `:55-56`, `:64`.
- #306: about 15 lines and one test. #300 G/D: a move of roughly 800-1,000 lines out of `component.go`.
