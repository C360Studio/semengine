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

`component.Shell`, one exported type in `component/shell.go`. Its zero value is an unstarted shell; it must not be
copied after first use (`go vet` copylocks reports it). A component holds it as an unexported named field. It composes
`internal/lifecycleguard.Guard`; `component` gains the imports `internal/lifecycleguard` and, through it,
`pkg/lifecyclecleanup`. Neither reaches a graph or agentic package, so `TestComponentReachesNoAgenticOrGraphPackage`
holds unchanged; the other import rule forbids only harness and test libraries in production code
(`internal/harness/contract/imports_test.go:82`). No exported signature names an internal type. Services keep composing
the guard directly (ADR-108 D1). The name is ADR-108's coined term, defined in the type's doc comment; renaming it is
the owner's call at acceptance.

## D2. Surface

```go
type Shell struct{ /* unexported */ }

func (s *Shell) Initialize() error
func (s *Shell) Start(ctx context.Context, start, cleanup func(context.Context) error) error
func (s *Shell) Stop(ctx context.Context) error
func (s *Shell) Health(stuck ...string) HealthStatus
func (s *Shell) DataFlow() FlowMetrics
func (s *Shell) CountInput(bytes int)
func (s *Shell) CountError()
```

A component supplies `start` (acquires its resources, creates its run context), `cleanup` (releases them in its own
order and ends its run context), and its current stuck states at each `Health` call. That is ADR-108's "composes one
type and supplies two functions", plus the stuck states ADR-109 D2 has each component name.

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
`DataFlow`: `MessagesPerSecond` and `BytesPerSecond` are the `CountInput` totals over uptime, zero when not running;
`ErrorRate` is errors per input while running and at least one input was counted, else zero; `LastActivity` is the
time of the last `CountInput`, the zero time before the first. These are graph-ingest's rules today
(`component.go:841-926`), moved; the three words are graph-ingest's (its tests pin `degraded`,
`health_integration_test.go:71`). The vocabulary itself stays #110 item 3's.

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
- `Health` returns `c.shell.Health(stuck...)`, where `stuck` holds the poison message when the inventory is
  non-empty and the backlog fault when one is held (`:854-863`, unchanged).

Declared behaviour changes, three, each small: `Initialize` no longer re-validates (`component/lifecycle.go:76-78`
promised it); `LastActivity` before the first input is the zero time, not the construction time (a construction is
not activity; no test or reader depends on it, inventory section 1); the "component initialized" log line goes.

Proof the rest is unchanged: every assertion in the package's tests stays as written, except
`TestComponent_Initialize_InvalidConfig` (`component_test.go:657`), which tests the removed duplicate check and
becomes a test that the factory refuses that configuration and `Initialize` succeeds after a valid construction. Test
setup lines change spelling only: `c.running = true` (11 sites) and `startTime`/`messagesProcessed` writes become a
shell `Initialize` and `Start` with a start that acquires nothing (and, where the test never stops the component,
a cleanup that releases nothing) and `CountInput` calls; the 14 `atomic.LoadInt64(&c.errors)` reads
become `c.Health().ErrorCount`; the three guard helpers (`test_owner_support_test.go:80-125`) call `c.shell.Start`.
The tests that carry the lifecycle: `TestGraphIngestLifecycleSuite`,
`TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop`,
`TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop`, `TestComponent_Start_BeforeInitialize`,
`TestComponent_Stop_BeforeStart`, `TestPoisonRepairRecoveryWithoutRestart`, `TestIntegration_HealthReadsLostConsumer`,
`TestIntegration_HealthIgnoresErrorCount`, `TestIntegration_DataFlowCountsInputMessages`. No new test seam on
`Component` (#301 unchanged).

## D7. #306: the entity cache handle stays valid through Stop

Smallest fix, mirroring what the code already does for `running`: the cache handle is read once per operation, under
the same synchronization as its clear, and used through that local. `memCache.Close` only stops the expiry sweep
(`internal/cache/cache.go:346-353`); `Get`, `Set` and `Delete` take the cache's own lock (`:133`, `:207`, `:261`), so a
read that loaded the handle before the clear and runs after `Close` is safe. The handle is still cleared only by a
cleanup that succeeded (design D22 of the ingest kernel), so the suite adapter (`lifecycle_adapter_test.go:98`) is
unchanged. The other fix #306 lists (never clear the handle) would break that adapter. Observable: a query or
`CreateEntity` running during or after `Stop` neither panics nor races. Proving test: `TestEntityCacheUseDuringStop`
(unit, under `-race`), which runs the three readers on goroutines while `Stop` runs and is shown to fail at the base.

## D8. #300 sections G and D

Optional maintenance under Bounded port scope: PR #299 D8 found no binding requirement, and the owner routed them
to #109 without making them a correctness fix (#300, ruling of 2026-10-18). The file map, for whichever PR does it:
the boot sweep (`:1232-1383`) to `poison_inventory.go`; cache coherence (`:2342-2406`) to `query.go`; the metric
constructors (`:47-276`, `:2617-2643`) and `registerMetrics` to `metrics.go`; `Config` (`:278-389`) to `config.go`;
`ingestDeliveryTiming` and `setupJetStreamConsumer` with its consume closure (`:1436-1634`) to `consume.go`; the
decode, ingest and write paths (`:1689-2615`, less cache coherence) to `ingest.go`. Moves only, no edits.
Recommended: its own pull request right after this one (owner question 1), so the behaviour diff here is not hidden
in a 1,000-line move.

## D9. #309 stays

The shell inherits the guard's panic behaviour. No cleanup in the tree panics (#309 body), and no outside component
composes the shell yet, so there is no real-world trigger (#185). The shell's `Start` doc says, in plain words, that
`cleanup` must tolerate a partial start and must not panic, citing #309. Owner question 3.

## D10. Proving tests

In `component/shell_test.go` (unit; `component` stays on the 80% coverage list):

- `TestShellOwnerPassesTheLifecycleSuite`: `lifecycletest.Run` over an owner built only from a `Shell` and one worker
  goroutine under its own run context, with a failing factory (ADR-108 D3, "tested in the shell's package by the suite
  on a shell-only owner").
- `TestShellReads`: a table over every state D4 lists × {no stuck state, stuck states} × {no errors, errors},
  asserting every field of both reads. Examples suffice, no generated check: the reads depend only on the current
  phase, which the guard's tests already cover over every ordering they name, and on two inputs the table enumerates
  whole.
- `TestShellReadsDuringStopReportStopped`: `cleanup` blocks on a channel; `Health` during it reads `stopped`.
- `TestShellReadsDoNotWaitForStartOrStop`: reads return while `start` and then `cleanup` are held open.
- `TestShellStartBeforeInitializeIsRefusedAndLeavesItUnused`, `TestShellFailedStartNeverRuns`.
- `TestShellSuiteCatchesWorkUnderTheStartContext`: a shell owner whose `start` runs its worker under `ctx` fails
  `lifecycletest.CheckStartContextEndStopsNothing`. This is how a test shows a rule the shell cannot enforce (ADR-113's
  run context) is still caught: by the suite on the component's own run.
- `ExampleShell` in package `component_test`: a component composing the shell as an outside author would.

graph-ingest: D6's list, plus D7's test. Mutation rows (`task mutate:check`, recorded in `notes/6.1.md`):

| Row | Mutant | Expected to fail |
| --- | --- | --- |
| M1 | `Healthy` drops the running term | `TestShellReads` |
| M2 | `Healthy` also requires zero errors | `TestShellReads`, `TestIntegration_HealthIgnoresErrorCount` |
| M3 | running cleared after `cleanup` returns, not before it runs | `TestShellReadsDuringStopReportStopped`, `TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop` |
| M4 | `Start` skips the Initialize check | `TestShellStartBeforeInitializeIsRefusedAndLeavesItUnused`, `TestComponent_Start_BeforeInitialize` |
| M5 | running set before `start` returns | `TestShellFailedStartNeverRuns` |
| M6 | the #306 fix reverted (handle read twice, cleared unsynchronized) | `TestEntityCacheUseDuringStop` |

## D11. Ledger

Tree code: no new row. `component` (`docs/admission-ledger.yaml:2267`): adaptation (15), "`Shell` composes
`internal/lifecycleguard` and answers Health and DataFlow (ADR-108 D1, D3)"; dependencies add
`internal/lifecycleguard`, `pkg/lifecyclecleanup`; `proving_tests` add `shell_test.go`. `processor/graph-ingest`
(`:2325`): item (6) becomes "owner lifecycle through `component.Shell`"; `dependencies_and_side_effects` drops
`internal/lifecycleguard`. Disposition unchanged on both. The `component.go` row (`:2381`) changes only if D8 moves
the consume closure.

## D12. Spec delta

New capability `component-shell` (requirements in `specs/component-shell/spec.md`): Initialize and Start-before-
Initialize; one Start and Stop the only stop through the shell; running from `start`'s return to `cleanup`'s start;
the Health table; ErrorCount never lowers Healthy; DataFlow; reads never wait; the shell holds no context. Purpose for
the archive: "The processor shell, `component.Shell`: the lifecycle and the two reads every component shares
(ADR-108)." One requirement added to `graph-entity-writes` for #306. Each requirement is under 500 characters.

## D13. Docs and records

`component/lifecycle.go:43-78`: drop `:76-78` and add "`Shell` implements every rule above". `component/README.md`
`:123-125`: the shell paragraph and the example, the framework-API contract line. `docs/repository-map.md`: the
`component/` row (`:81`) names the shell; the `internal/lifecycleguard/` row (`:83`) says components reach it through
`component.Shell`. ADR-108 Status: a one-line note naming `component.Shell` and this change, as ADR-113's 2026-10-21
note does; no decision changes. Comments on #196 (graph-ingest's row: adopted through the shell) and #109 (shell
landed; follower open).

## D14. Issues and merge order

Retires #306. Addresses #109 and #196 (comments only). Stays: #309 (D9), #300 (D8; sections C, D-rest, F, H stay
parked), #301 (its reasons are untouched), #216 (consumer rebinding, not lifecycle), #361 (a consumer's composition
root), #399 (refusal text, another function), #110 item 3 (vocabulary at temporal's port). No open pull request
overlaps; #362 and #363 touch no file here. Merge before graph-index (PR 3), which composes the shell.

## D15. ADRs kept, changed, dropped

Kept: ADR-108 D1-D3 (implemented), D3(a) no restart; ADR-113 (the shell makes no run context); ADR-095 (one Start,
retained cleanup); ADR-109 D2 (one implementation); ADR-058 (collectors at construction); ADR-096 (the manager's
`ManagedComponent.State` stays its own record). Changed or dropped: none.

## Unit of work

One pull request. The shell's only production caller is graph-ingest; a shell landed alone would have tests as its
only callers, which the surface rule reads as a finding until graph-index arrives. The change is tree code of about
180 added and 110 removed non-test lines across one new type and one adopter, well under the one-owner bound (#159);
no waiver is needed.

## Owner questions

1. **Split `component.go` (#300 G/D) in its own pull request right after this one?** Recommended yes: the move is
   about 1,000 lines with no edits, and inside this PR it would bury the lifecycle change a reviewer has to read
   closely. The other answer saves one review round and makes this PR several times larger.
2. **Keep #109 open until the last follower lands (change 7)?** Recommended yes: this PR says "Addresses #109" and
   posts what landed; the other answer splits the shell into its own issue, closed here, and leaves #109 as the
   follower and plug-in work.
3. **Leave the panic-in-cleanup defect (#309) open?** Recommended yes: nothing in the tree triggers it and no outside
   component uses the shell yet; the shell's doc says a cleanup must not panic. The other answer fixes it here, about
   fifteen lines and two tests in `internal/lifecycleguard`, before any outside author can hit it.
