# Design: setup-04a-04-graph-roots-processor-shell

Base `7615030`. Inventory: `inventory.md` (drafted in the same pass; no `INVENTORY PASS` yet). Change 4, PR 2 of 9
(#9 comment 6386856657). Retires #306 (`Closes #306`, tasks 2.2 and 4.2). Addresses #109 (the shell half; the
follower lands with graph-index). Governing records: ADR-108 D1-D3, ADR-113, ADR-095, ADR-109 D2, ADR-058.

## Context

The *processor shell* is the part of a component that is the same for every component (ADR-108): starting once,
stopping, cleaning up a start that failed, and answering `Health()` and `DataFlow()`. Half of it exists as
`internal/lifecycleguard.Guard` (`guard.go:31`). The other half is inline in graph-ingest, the tree's only production
component (`processor/graph-ingest/component.go:841-950`, inventory section 1). Graph-index, graph-query and
objectstore, the next three ports, each carry their own copy at the pin (inventory section 9). ADR-108 ruled the
shell public in `component`, composing the guard, landing in change 4 with graph-ingest moving onto it.

## Options

- **A. Do nothing.** Each port copies graph-ingest's 110 lines; ADR-108 D3's reason ("fixed and proven once") is lost.
- **B. Grow the guard.** Add running, start time and counters to `internal/lifecycleguard` and a thin `component`
  wrapper. Services, which compose the guard and answer `health.Status`, would carry state they never read.
- **C. An embedded base, as `service.BaseService`** (`service/base.go:67`, options bag `:64`). Embedding promotes the
  counting methods onto graph-ingest's exported `Component`, so any importer could bump its counters; the options bag
  has no second value to carry.
- **D. A plug-in interface the shell drives** (the component supplies `start`/`cleanup`/`stuck` methods and the shell
  implements `LifecycleComponent` around it). One implementation per interface, and the shell would also have to
  forward ports and `DebugStatus`: a layered interface (shape sweep 3).
- **E. A small struct held as a named field, with explicit one-line delegation** (chosen). Nothing is promoted, no
  constructor, no options.

## D1. Home and name

`component.Lifecycle`, one exported type in `component/lifecycle_type.go`. Its zero value is an unstarted shell; it
must not be
copied after first use (`go vet` copylocks reports it). A component holds it as an unexported named field. It composes
`internal/lifecycleguard.Guard`; `component` gains the imports `internal/lifecycleguard` and, through it,
`pkg/lifecyclecleanup`. Neither reaches a graph or agentic package, so `TestComponentReachesNoAgenticOrGraphPackage`
holds unchanged; the other import rule forbids only harness and test libraries in production code
(`internal/harness/contract/imports_test.go:82`). No exported signature names an internal type. Services keep composing
the guard directly (ADR-108 D1). The name is ADR-108's coined term, defined in the type's doc comment; renaming it is
the owner's call at acceptance.

## D2. Surface

```go
type Lifecycle struct{ /* unexported */ }

func (l *Lifecycle) Initialize() error
func (l *Lifecycle) Start(ctx context.Context, start, cleanup func(context.Context) error) error
func (l *Lifecycle) Stop(ctx context.Context) error
func (l *Lifecycle) Health(stuck ...string) HealthStatus
func (l *Lifecycle) DataFlow() FlowMetrics
func (l *Lifecycle) CountInput(bytes int)
func (l *Lifecycle) CountError()

// FlowMetrics gains two fields (component/discovery.go:83-88):
Messages int64 `json:"messages"` // input messages counted since construction
Bytes    int64 `json:"bytes"`    // their payload bytes
```

A component supplies `start` (acquires its resources, creates its run context), `cleanup` (releases them in its own
order and ends its run context), and its current stuck states at each `Health` call. That is ADR-108's "composes one
type and supplies two functions". `Initialize` returns an error, always nil, only so a component's `Initialize` is
the one line `return c.shell.Initialize()`.

`Health` takes zero or more stuck states where ADR-108 D3(c) says "one verdict the follower or the plug-in supplies"
(`docs/adr/108-processor-shell.md:84-86`). The list generalises it because ADR-109 D2 has a component name each of
its stuck states, and graph-ingest has two (poison, backlog fault) and graph-index will have the follower's and the
plug-in's. An empty string is not a stuck state and is ignored, so a caller passes its states without branching.

The two totals are the exact counts the rates are derived from. They are read by graph-ingest's two exact-count tests
(D6) and served by the component manager's status routes as `data_flow.messages` and `data_flow.bytes`
(`service/component_manager.go:575`; `service/component_manager_http.go:341`, `:478`). Their contract line is the
`component-lifecycle` requirement on `DataFlow`. Every other `FlowMetrics` builder in the tree is a test literal
(`component/registry_integration_test.go:121`), which compiles unchanged.

### D2a. The name: `component.Lifecycle` (owner, 2026-10-25)

ADR-108 calls the concept the processor shell; the owner chose `component.Lifecycle` for the type on 2026-10-25
("shell" reads as a boundary one talks through; this is the component's own lifecycle state). Measured collision
risk: `pkg/lifecycle` is the workflow-entity substrate (its doc, `pkg/lifecycle/doc.go:1-12`) and has no non-test
importer in the tree, so the two never meet in one file today; graph-ingest already names its guard field
`lifecycle` (`processor/graph-ingest/component.go:446`, `:975`, `:1093`), so `lifecycle component.Lifecycle` keeps
the existing reads. The type sits beside `component.LifecycleComponent` (the interface the manager calls,
`component/lifecycle.go:79`): a `LifecycleComponent` holds a `Lifecycle` to answer the lifecycle half of that
interface. The type's doc comment says both things in one sentence. File: `component/lifecycle_type.go`, since
`component/lifecycle.go` already holds `State` and the interface.

## D3. What a caller observes: lifecycle

- `Initialize` returns nil, every time, and does no I/O.
- `Start` before `Initialize` returns an error and leaves the shell unused: a `Start` after `Initialize` is admitted.
- After that check, `Start` is the guard's `Start` (nil or ended context refused; second Start refused with
  `errs.ErrAlreadyStarted`; failed or panicking `start` rolled back through `cleanup`; a failed rollback kept for the
  next `Stop`). `Stop` is the guard's `Stop`. Unchanged from graph-ingest today (ADR-095 kept; ADR-113 kept).
- The shell is *running* from the moment `start` returns nil until `cleanup` begins, whether for a `Stop` or a
  rollback. A failed `start` never makes it running.
- The shell holds no context and no cancel, and creates neither. ADR-113 rejected the guard holding the run context
  (`docs/adr/113-start-context-bounds-start.md:102-107`); the same reason holds one layer up.

## D4. What a caller observes: reads

Both reads may be called at any time, during `Start` or `Stop` and after `Stop`, and never wait for either
(`component/lifecycle.go:66-74`).

| State | `Healthy` | `Status` | `LastError` | `Uptime` |
| --- | --- | --- | --- | --- |
| not running | false | `stopped` | empty (stuck ignored) | 0 |
| running, no stuck state | true | `running` | empty | since `start` returned |
| running, stuck states given | false | `degraded` | the states joined by `"; "` | since `start` returned |

`ErrorCount` is the `CountError` total and never lowers `Healthy` (ADR-109 D2). `LastCheck` is the time of the read.
`DataFlow`: `Messages` and `Bytes` are the `CountInput` totals since construction, whether running or not;
`MessagesPerSecond` and `BytesPerSecond` are those totals over uptime, zero when not running; `ErrorRate` is errors
per input while running and at least one input was counted, else zero; `LastActivity` is the time of the last
`CountInput`, the zero time before the first. These are graph-ingest's rules today (`component.go:841-926`), moved,
plus the two totals; the three words are graph-ingest's (its tests pin `degraded`, `health_integration_test.go:71`).
The shell's literal `degraded` and graph-ingest's `graph.IndexStateDegraded` (`component.go:869`) become two
spellings of one word; a note on #110 (item 3, the vocabulary) records it (task 6.3).

## D5. What the shell does not own

The run context and the release order (each owner's, ADR-113); graph-ingest's request gate `startCompleted`
(`component.go:450`, `:1070`), which differs from running on purpose (handlers serve through Stop's drain); which
states are stuck; the readiness envelope (`graph/readiness`; `component` may not import it); Prometheus collectors
(registered at construction on the injected `metric.MetricsRegistry`, ADR-058 phase A; the shell registers none and
its counters are plain integers); ports; `DebugStatus`; configuration validation (the factory's).

## D6. graph-ingest moves onto the shell

Leaves `processor/graph-ingest/component.go`: the fields `running`, `initialized`, `startTime`, `lifecycle`,
`messagesProcessed`, `bytesProcessed`, `errors`, `lastActivity` (`:441-446`, `:516-520`), the construction-time
`lastActivity` store (`:696`), `DataFlow`'s and `Health`'s bodies, `Initialize`'s body including its second
`Validate`, and the `running` writes in `start` and `release` (`:1039-1043`, `:1103-1105`). The ten counter sites call
`CountInput` / `CountError`. What remains:

- `Initialize` returns `c.shell.Initialize()`. `Start` keeps its nil-client refusal (`:972-974`), then returns
  `c.shell.Start(ctx, c.start, c.release)`. `Stop` returns `c.shell.Stop(ctx)`. `DataFlow` returns
  `c.shell.DataFlow()`.
- `Health` returns `c.shell.Health(poison, fault)`, the poison message when the inventory is non-empty and the
  backlog fault when one is held, each empty otherwise (`:854-863`).

Declared behaviour changes, three, each small:

- `Initialize` no longer re-validates (`component/lifecycle.go:76-78` promised it).
- `LastActivity` before the first input is the zero time, not the construction time: the status routes serve
  `data_flow.last_activity` as `0001-01-01T00:00:00Z` until graph-ingest counts an input (`component_manager.go:575`,
  `component_manager_http.go:341`, `:478`). The old value claimed activity at construction; no test reads it.
- The "component initialized" log line goes.

Plus one addition: the routes serve `data_flow.messages` and `data_flow.bytes` (D2).

Proof the rest is unchanged: every assertion in the package's tests stays as written, except
`TestComponent_Initialize_InvalidConfig` (`component_test.go:657`), which tests the removed duplicate check and becomes
a test that the factory refuses that configuration and `Initialize` succeeds after a valid construction. Reads change
spelling, oracles do not: `TestIntegration_DataFlowCountsInputMessages` (`dataflow_integration_test.go:109-111`) and
`TestComponent_MetricsAtomic_MessageProcessing` (`metrics_test.go:48-49`, `:55-56`, `:64`) keep comparing exact counts
with their own tallies, reading `c.DataFlow().Messages` and `.Bytes` for `messagesProcessed` and `bytesProcessed`; the
14 `atomic.LoadInt64(&c.errors)` reads become `c.Health().ErrorCount`. Setup lines: `c.running = true` (11 sites) and
the `startTime` and `messagesProcessed` writes become a shell `Initialize` and `Start` with a start that acquires
nothing (and, where the test never stops the component, a cleanup that releases nothing) and `CountInput` calls; the
three guard helpers (`test_owner_support_test.go:80-125`) call `c.shell.Initialize` and `c.shell.Start`. The comment on
`TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop` (`discoverable_reads_test.go:16-17`) names a mutant site that
leaves `release`; it names M7 instead. The tests that carry the lifecycle: `TestGraphIngestLifecycleSuite`,
`TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop`,
`TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop`, `TestComponent_Start_BeforeInitialize`,
`TestComponent_Stop_BeforeStart`, `TestPoisonRepairRecoveryWithoutRestart`, `TestIntegration_HealthReadsLostConsumer`,
`TestIntegration_HealthIgnoresErrorCount`, `TestIntegration_DataFlowCountsInputMessages`. No new test seam on
`Component` (#301 unchanged).

## D7. #306: the entity cache handle stays valid through Stop

Smallest fix: each operation reads the cache handle once, under `c.mu`, the lock `clearLifecycleHandles` then also
takes for the clear, and uses that local copy. `memCache.Close` only stops the expiry sweep
(`internal/cache/cache.go:346-353`); `Get`, `Set` and `Delete` take the cache's own lock (`:133`, `:207`, `:261`), so a
use that loaded the handle before the clear and runs after `Close` is safe. That is read from the code; `Cache.Close`'s
doc (`cache.go:49-50`) promises nothing and no test uses a cache after `Close`. So this change adds one sentence to
that doc ("After Close, Get, Set and Delete keep working; only background expiry stops") and one test,
`TestCacheUsableAfterClose`, run in `internal/cache`'s suite over every implementation; the `pkg/cache` ledger row
(`docs/admission-ledger.yaml:803`, `adapt`) names it under `proving_tests`.

The handle is still cleared only by a cleanup that succeeded (design D22 of the ingest kernel), so the suite adapter
(`lifecycle_adapter_test.go:98`) is unchanged. The other fix #306 lists (never clear the handle) would break that
adapter. Observable: a query or `CreateEntity` running during or after `Stop` neither panics nor races. Proving test:
`TestEntityCacheUseDuringStop` (unit, under `-race`), which runs the three readers on goroutines while `Stop` runs and
is shown to fail at the base.

## D8. #300 sections G and D

Optional maintenance under Bounded port scope: PR #299 D8 found no binding requirement, and the owner routed them
to #109 without making them a correctness fix (#300, ruling of 2026-10-18). The file map, for whichever PR does it:
the boot sweep (`:1232-1383`) to `poison_inventory.go`; cache coherence (`:2342-2406`) to `query.go`; the metric
constructors (`:47-276`, `:2617-2643`) and `registerMetrics` to `metrics.go`; `Config` (`:278-389`) to `config.go`;
`ingestDeliveryTiming` and `setupJetStreamConsumer` with its consume closure (`:1436-1634`) to `consume.go`; the
decode, ingest and write paths (`:1689-2615`, less cache coherence) to `ingest.go`. Moves only, no edits.
Recommended: its own pull request right after this one (owner question 1), so the behaviour diff here is not hidden
in a 1,000-line move.

## D9. #309 stays, and #109 stays open

Issue #309: the shell inherits the guard's panic behaviour. No cleanup in the tree panics (#309 body) and no outside
component composes the shell yet, so there is no real-world trigger, and under #185 the fix is not required work.
The shell's `Start` doc says, in plain words, that `cleanup` must tolerate a partial start and must not panic, and
cites that issue.

Issue #109: ADR-108's Status says the record does not close #109, which stays the refactor
(`docs/adr/108-processor-shell.md:9-11`); its follower, plug-in and adoption work is in later pull requests. This one
addresses it and posts what landed (task 6.3).

## D10. Proving tests

In `component/lifecycle_type_test.go` (unit; `component` stays on the 80% coverage list). Every shell test runs inside
`synctest.Test`, as `internal/harness/lifecycletest/refowner_test.go:429-434` and `internal/cache/cache_test.go:132`
do: `Uptime`, the rates and `LastCheck` read the bubble's clock, which a test moves with `<-time.After(d)` followed by
`synctest.Wait()` (`internal/cache/background_test.go:93-94`), and the suite's 100 ms settle window
(`lifecycletest.go:27-29`, `:200-206`) is exact inside the bubble.

- `TestLifecycleOwnerPassesTheLifecycleSuite`: `lifecycletest.Run` over an owner built only from a `Lifecycle` and one worker
  goroutine under its own run context, with a failing factory (ADR-108 D3, "tested in the shell's package by the suite
  on a shell-only owner").
- `TestLifecycleReadsInEveryPhase`: one example per history the shell can be in, each read with and without stuck states
  and with and without counted errors, asserting every field of both reads: new; `start` held open; running; a
  `start` that failed, rolled back; a rollback's `cleanup` held open; a `Stop`'s `cleanup` held open; after a `Stop`
  whose `cleanup` failed; after a `Stop` that succeeded; `Stop` before `Start`. A held phase is entered on an explicit
  signal: the held function closes an `entered` channel and waits on a `release` channel, and the test reads only
  after `<-entered`. Examples suffice, with no generated check: the shell's reads depend only on which of these nine
  histories it is in and on the two inputs, and the table enumerates all of them.
- `TestLifecycleReadsDoNotWaitForStartOrStop`: both reads return while `start`, then `cleanup`, is held open (inside the
  bubble a read that blocks deadlocks the bubble and fails the test).
- `TestLifecycleStartBeforeInitializeIsRefusedAndLeavesItUnused`, `TestLifecycleFailedStartNeverRuns`.
- `TestLifecycleSuiteCatchesWorkUnderTheStartContext`: a shell owner whose `start` runs its worker under `ctx` fails
  `lifecycletest.CheckStartContextEndStopsNothing`. This is how a test shows a rule the shell cannot enforce (ADR-113's
  run context) is still caught: by the suite on the component's own run.
- `ExampleLifecycle` in package `component_test`: a component composing the shell as an outside author would.

In `internal/cache`: `TestCacheUsableAfterClose` (D7). In graph-ingest: D6's list, plus `TestEntityCacheUseDuringStop`.

Mutation rows (`task mutate:check`, recorded in `notes/6.1.md`). Each names unit tests only: `task mutate:check`
cannot run an integration test (#129).

| Row | Mutant | Expected to fail |
| --- | --- | --- |
| M1 | `Healthy` drops the running term | `TestLifecycleReadsInEveryPhase` |
| M2 | `Healthy` also requires zero errors | `TestLifecycleReadsInEveryPhase` |
| M3 | running cleared after `cleanup` returns, not before it runs | `TestLifecycleReadsInEveryPhase`, `TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop` |
| M4 | `Start` skips the Initialize check | `TestLifecycleStartBeforeInitializeIsRefusedAndLeavesItUnused`, `TestComponent_Start_BeforeInitialize` |
| M5 | running set before `start` returns | `TestLifecycleFailedStartNeverRuns` |
| M6 | the #306 fix reverted (handle read twice, cleared without the lock) | `TestEntityCacheUseDuringStop` |
| M7 | the shell holds its lock across `start` and `cleanup` | `TestLifecycleReadsDoNotWaitForStartOrStop`, `TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop` |
| M8 | `DataFlow` reports `Messages` as zero when not running | `TestLifecycleReadsInEveryPhase`, `TestComponent_MetricsAtomic_MessageProcessing` |

## D11. Ledger

Tree code: no new row. `component` (`docs/admission-ledger.yaml:2267`): adaptation (15), "`Lifecycle` composes
`internal/lifecycleguard` and answers Health and DataFlow; `FlowMetrics` carries the `Messages` and `Bytes` totals
(ADR-108 D1, D3)"; dependencies add `internal/lifecycleguard`, `pkg/lifecyclecleanup`; `proving_tests` add
`shell_test.go`. `processor/graph-ingest` (`:2325`): item (6) becomes "owner lifecycle through `component.Lifecycle`";
`dependencies_and_side_effects` drops `internal/lifecycleguard`. `pkg/cache` (`:803`): `proving_tests` add
`TestCacheUsableAfterClose`. Dispositions unchanged. The `component.go` row (`:2381`) changes only if D8 moves the
consume closure.

## D12. Spec delta

New capability `component-lifecycle` (requirements in `specs/component-lifecycle/spec.md`): Initialize and Start-before-
Initialize; one Start and Stop the only stop through the shell; running from `start`'s return to `cleanup`'s start;
the Health table; ErrorCount never lowers Healthy; DataFlow with its totals; reads never wait; the shell holds no
context. Purpose for the archive: "The processor shell, `component.Lifecycle`: the lifecycle and the two reads every
component shares (ADR-108)." The #306 requirement goes to `graph-ingest-recovery`, whose Purpose is what graph-ingest
promises when a process stops, and which already holds graph-ingest's data-flow requirement (`spec.md:242`);
`graph-entity-writes` is about write modes. Each requirement is under 500 characters.

## D13. Docs and records

- `component/lifecycle.go:43-78`: drop `:76-78` and add "`Lifecycle` implements every rule above".
- `component/README.md` `:123-125`: the shell paragraph and the example, the framework-API contract line; the two
  `FlowMetrics` totals.
- Rule text that still sends a component to the guard, contradicting ADR-108 D1: `AGENTS.md:83`,
  `.agents/rules-index.md:28`, `.agents/contracts/semengine-developer.md:326`,
  `.agents/contracts/semengine-reviewer.md:344`. Each becomes "a component keeps its lifecycle state in
  `component.Lifecycle`, a service in `internal/lifecycleguard`, never in a copy of its own". The row count in `AGENTS.md`
  and `rules-index.md` does not change.
- `internal/cache/cache.go:49-50`: D7's sentence.
- `docs/repository-map.md`: the `component/` row (`:81`) names the shell; the `internal/lifecycleguard/` row (`:83`)
  says components reach it through `component.Lifecycle`.
- ADR-108 Status: a one-line note naming `component.Lifecycle` and this change, as ADR-113's 2026-10-21 note does.
- Comments on #196 (graph-ingest's row: adopted through the shell), #109 (shell landed; follower open) and #110
  (D4's two spellings of `degraded`).

## D14. Issues and merge order

Retires #306. Addresses #109 and #196 (comments only). Stays: #309 and #109 (D9), #300 (D8; sections C, D-rest, F,
H stay parked), #301 (its reasons are untouched), #216 (consumer rebinding, not lifecycle), #361 (a consumer's
composition root), #399 (refusal text, another function), #110 item 3 (vocabulary at temporal's port). No open pull
request overlaps; #362 and #363 touch no file here. Merge before graph-index (PR 3), which composes the shell.

## D15. ADRs kept, changed, dropped

Kept: ADR-108 D1-D3 (implemented), D3(a) no restart; ADR-113 (the shell makes no run context); ADR-095 (one Start,
retained cleanup); ADR-109 D2 (one implementation); ADR-058 (collectors at construction); ADR-096 (the manager's
`ManagedComponent.State` stays its own record). Generalised, not changed: D3(c)'s "one verdict" becomes a list of
stuck states (D2), which ADR-109 D2 already implies. Dropped: none.

## Unit of work

One pull request. The shell's only production caller is graph-ingest; a shell landed alone would have tests as its
only callers, which the surface rule reads as a finding until graph-index arrives. The change is tree code of about
190 added and 110 removed non-test lines across one new type, two new fields and one adopter, well under the
one-owner bound (#159); no waiver is needed.

## Owner question

1. **Split `component.go` (#300 G/D) in its own pull request right after this one?** Recommended yes: the move is
   about 1,000 lines with no edits, and inside this PR it would bury the lifecycle change a reviewer has to read
   closely. The other answer saves one review round and makes this PR several times larger.

Not owner questions, settled by records: #109 stays open (ADR-108 Status, `:9-11`); #309 stays open (#185: no
real-world trigger, so not required work). See D9.
