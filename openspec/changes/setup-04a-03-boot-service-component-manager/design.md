# Design: setup-04a-03-boot-service-component-manager

This design rests on the whole-package inventory, `INVENTORY PASS` round 2 at `ad946b7`
(`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md`, "the inventory"), on this PR's shape
sweep and pin probe (`sweep.md`, "the sweep"), on PR 2's accepted design
(`openspec/changes/archive/2026-10-21-setup-04a-03-boot-service-manager/design.md`, "PR 2's design") and on the owner's
rulings on #9 comment 6296023694 (option B: the component manager lands after the service manager, behind PR 2's
unexported `componentStartup`; Q2: restore `projection.PredicateReconciler`). Pin paths are at `8b99efe9`; repository
lines at `d39fae3`. Size: 2,487 non-test pin lines (inventory § 2.3 row 3), one lifecycle owner, under the ~3,000 cap.
The spec deltas are named in D9 and written after the owner accepts this design (task 1.2).

**Decision records.**

- ADR-113 governs D2 and is kept; how it applies to each component's Start context, and who reacts to the root context
  ending (its Decision ¶2 leaves that to change 3, `:66-69`), is owner question Q-A. Its Consequences already state the
  target: "The ported component manager holds no per-component cancel, so the pin's two cancel orders
  (`service/component_manager.go:1033-1042`) do not come over" (`docs/adr/113-start-context-bounds-start.md:107-108`).
- Pin ADR-095 (one-shot running lifecycle, retained failed-start cleanup) is carried into `docs/adr/` in this PR, as
  PR 1's design said it would be (archived `2026-10-20-setup-04a-03-boot-service-base/design.md:19-21`). Kept: one
  Start per instance; a failed Start makes one bounded rollback and keeps what it could not release for a later Stop
  (`095:33-36`), which `internal/lifecycleguard` implements. Changed, as PR 2 recorded: "A deadline is a failed exit
  result, not authority for later running-generation rejoin" (`095:30`) is superseded by ADR-113 (D2, sweep C6). Its
  JetStream-consumer clauses govern `natsclient` and graph-ingest, not this file; the port note says so.
- Pin ADR-096 is carried in this PR with a port note. Only its fourth Decision paragraph stands (in-tree ADR-100,
  `docs/adr/100-compositions-are-validated-diagrams-are-projections.md:95-100`): the component manager reads its
  configuration once, at construction, and is the sole owner of the component handles (D3). The Flow parts name
  packages this repository does not have.
- ADR-094 (carried in PR 2) is kept; its port note's "Not ported yet: the component manager's part"
  (`docs/adr/094-boot-only-composition-and-observable-rule-activation.md:20-21`) is rewritten to "kept".
- ADR-100 (in tree) is kept: one interpreter, `composition.Analyze`, decides boot findings, and the HTTP projections
  serve the result retained at construction (D3, D5).
- Pin ADR-063 (store substrate) is cited through #321 and not carried in this change: the component manager fills and
  clears `storage/storeregistry` (D3) exactly as `component/dependencies.go:51-62` already says.
- ADR-108 Decision 2 names this owner for `internal/lifecycleguard` (`docs/adr/108-processor-shell.md:71-72`): D2.
- ADR-110 and pin ADR-091: `ConfigureRulePackMutations` builds `projection.MutationClient`s, which write through
  graph-ingest's mutation seam; `service` writes no `ENTITY_STATES` entry (D6).
- ADR-109 and ADR-088: readiness is PR 2's (`componentStartup.GetComponentHealth`); nothing here changes it.

## Inventory addendum (measurements PR 3 needs that the inventory lacks)

- **Callers here.** None of the five files' exported names has a caller in the tree today: `git grep -n -w <name> --
  '*.go'` outside tests finds 0 lines for `NewComponentManager`, `ComponentManagerConfig`,
  `DefaultComponentManagerConfig`, `ComponentStatus`, `GetComponentStatus`, `GetFlowPaths`,
  `BuildComponentTypeCatalog`, `ProjectionBinder`, `ProjectionBinders`, `ComponentsImplementing`,
  `ConfigureRulePackMutations`, `PredicateReconciler`. `ComponentManager` appears only in doc comments
  (`component/dependencies.go:18`, `:52`, `component/doc.go:12`, `component/lifecycle.go:78`, `:87`,
  `component/port_provide.go:8`, `component/registry.go:100`). `GetComponentHealth` is read through `componentStartup`
  (`service/service_manager.go:46`, `service/service_manager_http.go:513`). `config.ExpiredMigrationOverrides`
  (`config/stream_bounds.go:671`) has no production reader yet; `stream_override_expiry.go` is its first.
- **Callers in the named consumers** (tarballs of the inventory's header): semteams calls
  `service.ComponentsImplementing` (`cmd/semteams/rule_config_service.go:92`) and `service.ConfigureRulePackMutations`
  (`cmd/semteams/main.go:234`); semsource, semconnect and semboids call none of these names. Every
  `component-manager` payload in their configurations is `"config": {}` (semboids `configs/flock.json:43`, semteams
  `configs/flow-bootstrap.json:169` and three more). semteams' UI reads `/components/types`, `/components/flowgraph` and
  `/components/validate` (`ui/src/lib/services/componentTypeApi.ts`, `compositionApi.ts`).
- **Pin callers outside `service`** (the inventory's typed-use tool): `ConfigureRulePackMutations`
  `internal/boot/run.go:330`; `ComponentsImplementing` `internal/boot/rule_config_service.go:92`;
  `BuildComponentTypeCatalog` `processor/agentic-tools/component_catalog_executor.go:70` (not ported, agentic domain,
  `docs/tier1-cross-check.md:97`, so a consumer adapter: the name is framework API, D1). Every other name: 0.
- **What `composition` exports today** (`go doc -short ./composition`): `Analyze(declarations []component.Declaration,
  streams config.StreamConfigs) *Result`, `Catalog(*component.Registry) []CatalogEntry`, `Mermaid(Graph) string`,
  `Validate`, and the types `CatalogEntry`, `Edge`, `Finding`, `Graph`, `Node`, `PortView`, `Ports`, `Result`. Each name
  the pin files use is there with the pin's shape (scratch build, sweep § 3).
- **What `pkg/projection` exports today:** `Contract` and `PredicateGroup` (aliases of `pkg/projection/contract`),
  `ValidateContracts`, `NewMutationClient(MutationClientConfig{NATS, Contracts, Timeout})`, `MutationClient` with
  `Reconcile(ctx, ReconcileMutation) (MutationReceipt, error)` (`pkg/projection/mutation_client.go:168`), and the
  mutation and receipt types. No `PredicateReconciler`: the ledger row records its removal
  (`docs/admission-ledger.yaml:2211-2212`). The pin's one method is `Reconcile(context.Context, ReconcileMutation)
  (MutationReceipt, error)` (pin `pkg/projection/mutation_types.go:152-155`), which `MutationClient` has as is.
- **`componentStartup`.** The pin's `ComponentManager` satisfies it unchanged: `startupSnapshot`
  `component_manager.go:724`, `startupStopBegun` `:769`, `setStartupMetricWriter` `:763`, `withComponents` `:830`,
  `GetComponentHealth` `:1319`, `registerStartupHTTPHandlers` `component_manager_http.go:83` (sweep § 3: the assertion
  compiles against the tree).
- **Open pull requests** (`gh pr list --state open --json number,files`, 2026-10-21): #359, this claim; #358,
  Dependabot `prometheus/client_golang` 1.25.0 (`go.mod`, `go.sum`), no shared file. This PR changes neither, so either
  may merge first; if #358 lands first this PR's tests run on 1.25.0.
- **Open issues** (`gh issue list --state open --search <q> --limit 200` for `service`, `ComponentManager`,
  `component_manager`, `component-manager`, `stream_override_expiry`, `migration override`, `rule_pack_bind`,
  `ProjectionBinder`, `PredicateReconciler`, `ComponentsImplementing`, `ConfigureRulePackMutations`,
  `BuildComponentTypeCatalog`, `storeregistry`, `lifecycleguard`, `natsfixture`, `flowgraph`, `composition`,
  `letter case`, `GetFlowPaths`, `componentStartup`): naming a carried file or type: #196, #195, #280, #321, #30, #344,
  #309; the dispositions are in D8. Names the word, not these files: #7, #9, #10, #11, #15, #16, #24, #29, #36, #68,
  #86, #96, #109, #129, #203, #314, #331, #340. Empty: `component_manager`, `stream_override_expiry`, `rule_pack_bind`,
  `ProjectionBinder`, `ComponentsImplementing`, `ConfigureRulePackMutations`, `BuildComponentTypeCatalog`,
  `GetFlowPaths`.

## D1. Files and surface (surface audit, #190)

| Pin file (non-test lines) | Disposition | Change |
| --- | --- | --- |
| `component_manager.go` (1,526) | adapt | split in two (D7's units): construction, composition, stores and projections stay in `component_manager.go`; Start, Stop, phases and the supervisor move to `component_manager_lifecycle.go` (the shape of PR 2's `service_manager_http.go`); D2-D4 |
| `component_manager_http.go` (673) | adapt | D5 |
| `rule_pack_bind.go` (132) | adapt | D6 |
| `stream_override_expiry.go` (130) | adapt | `metric.RegisterOrGet`, namespace `semengine` (#69); D4 |
| `component_manager_config.go` (26) | adapt | `EnabledComponents`, `DefaultComponentManagerConfig` and the no-op `Validate` dropped; the type stays empty |

- **Dropped, no reader** (inventory § 5 (a)-(c)): `IsInitialized`, `IsStarted`, `GetHealthyComponents`,
  `GetUnhealthyComponents`, `DefaultComponentManagerConfig`, `ComponentManagerConfig.EnabledComponents` (no consumer
  payload sets it, addendum) and its "empty=all" text, `ComponentManagerConfig.Validate`, unexported `shutdownCallback`;
  `toolRegistry`, `lifecycleManager` (#29, #102 ruling D). `Initialize` becomes unexported `initialize` and its
  re-entry flag and the two "not initialized" refusals go (sweep C1, recommended). An empty `ComponentManagerConfig`
  decoded strictly refuses every key, naming it: `parse component-manager config: ... unknown field
  "enabled_components"` (`TestComponentManagerConfigRefusesEveryKey`).
- **Kept with contract lines** (README, ledger `contract`, D9's deltas): `ComponentManager`, `NewComponentManager`
  (registered by `RegisterAll`), `ComponentsImplementing`, `ConfigureRulePackMutations` (semteams, pin `internal/boot`),
  `ProjectionBinder` (the seam the rule processor implements, change 6), `BuildComponentTypeCatalog` (framework API: a
  consumer adapter's caller, addendum; `TestBuildComponentTypeCatalog_Shape` calls it as a product would), the
  `HTTPHandler` methods `RegisterHTTPHandlers` and `OpenAPISpec`, and `projection.PredicateReconciler` (D6). Read inside
  the package and kept as at the pin: `GetComponentHealth`, `GetComponentStatus`, `ComponentStatus`, `GetFlowPaths`,
  `Manager.ProjectionBinders`.
- **Text** (sweep C12): the second package comment, the type comment's lifecycle, the `ComponentManagerOption` and
  `specs/008-...` comments, and `component.ManagedComponent.Config`'s gh#520 sentence are rewritten to what the code
  does. The five `"component-manager"` literals read `componentManagerName` (S12).

## D2. Start bounds Start; Stop is the only stop (ADR-113; #196)

Probe (sweep § 3): the pin manager passes 7 of 8 suite checks and fails `StartContextEndStopsNothing` ("stopped holding
[base-status=running component-work]"); a component's Start context ends when the manager's Start context ends, and also
when `Stop` returns. Behaviour that holds whatever Q-A's answer, proven by `TestComponentManagerPassesTheLifecycleSuite`
(an adapter observing the base service, the supervisor's two loops and each component's work, for a component that
follows ADR-113; `mustFail`, a composition whose one component's `Start` fails) unless another test is named:

- `Start(ctx)` refuses a nil or ended context, a second `Start` and a `Start` after `Stop` (`errs.ErrAlreadyStarted`); a
  refused `Start` starts nothing. It starts the admitted lifecycle components in two phases, `component.StoreProvider`
  components first, then the rest; each phase starts its components concurrently, and the next phase begins only after
  every `Start` in the current one returned (`TestComponentManagerStart_ProvidersRegisterBeforeConcurrentConsumers`,
  `_WaitsForAllComponentStarts`). Within a phase the start order is recorded by name (sweep C4), so the stop order is
  the reverse of a fixed order. A failure in the first phase starts nothing of the second
  (`_ProviderFailurePreventsConsumerPhase`); `Start` returns every failure joined, each naming its component (`component
  "x": ...`, `_JoinsMultipleFailures`). A component whose `Start` returns nil after `Start`'s context ended counts as
  failed (pin `:705-708`). Then it starts its base service and its supervisor (D4) and returns.
- A failed `Start` rolls back once, under `pkg/lifecyclecleanup.RollbackFailedStart`: every component that was launched,
  the one whose `Start` failed included (`lifecycle-suite`, "Failed start whose own cleanup fails"), is stopped in
  reverse launch order; if that rollback fails, the next `Stop` runs it again
  (`TestComponentManagerFailedStartCleanupFailureRetainsAuthorityForLaterStop`, outcomes only).
- Ending `Start`'s context after `Start` returned stops nothing the manager owns: base-service health checks, the health
  publisher and the override-expiry reporter run until `Stop`. They run under one run context created in `Start` and
  ended in `Stop` (ADR-113's one exception), the shape PR 2's health publisher has (`service/service_manager.go:595`).
- `Stop(ctx)` refuses a nil context; given an ended context it returns that error and stops nothing; before any `Start`
  it returns nil and a later `Start` is refused. Otherwise it marks the stop begun (so `startupStopBegun` reports true
  and `/readyz` answers `NOT READY`), refuses new component borrows and waits for admitted ones (a health or status
  request in flight; `TestComponentManagerStopFencesAndWaitsGetComponentHealthBorrow`,
  `_StopWaitsForHealthPublisherBorrowBeforeChildStop`), stops each lifecycle component with `ctx` in reverse start
  order, removing a provider's stores from the store registry before its `Stop`
  (`TestComponentManagerRunningStopDeregistersStoreBeforeLiveChildStop`), then ends the supervisor and stops the base
  service. It returns nil only when all are released. A component whose `Stop` failed, or was not reached because `ctx`
  ended, is stopped again by the next `Stop`; one whose `Stop` returned nil is not called again (sweep C6;
  `TestStopRetriesOnlyComponentsThatDidNotStop`). A `Stop` while another runs is refused as transient; a `Stop` after
  one that returned nil returns nil and calls nothing.
- **Guard (#196, ADR-108 Decision 2).** The manager composes `internal/lifecycleguard` for `Start`/`Stop` (start: the
  phases; cleanup: the stop sequence, safe to repeat), as `Manager`, `Metrics` and `config.Manager` do. The pin's nine
  lifecycle fields (`:101-110`) and the per-component `cleanupPending`, `terminal`, `stopping`, `cancel` and `startDone`
  (`:121-131`) go; a component's startup counts (`startsInvoked`, `startsCompleted`, `startsFailed`) and whether its
  `Stop` returned nil stay. Mechanism (locks, join order) is the developer's. #309 (a cleanup that panics leaves the
  guard stuck) now also covers a component whose `Stop` panics; the pin crashes the process the same way, and no caller
  recovers it, so it stays open (D8).

**The component's own Start context (owner question Q-A).** What the pin does (`component_manager.go`):

- `:464` — `runtimeCtx, cancel := context.WithCancel(ctx)`: the manager's run context descends from its Start context.
- `:622` — `childCtx, cancel := context.WithCancel(ctx)` with `ctx` = that run context, kept as
  `componentRuntime.cancel` (`:625`), and `:636`, `:699` — the component's `Start` receives it.
- `:785-791` — a `Stop` during `Start` calls the run context's cancel, so every `Start` in flight sees its context end.
- `:1033-1042` — two orders: cancel before the component's `Stop` while a failed start's cleanup is pending, after it
  otherwise. ADR-113 records this cost (`:39-41`) and removes it: "The ported component manager holds no per-component
  cancel" (`:107-108`).
- PR 2's precedent (its design D2, "Ending `StartAll`'s context after it returned stops nothing: services (each owns its
  run, ADR-113)"): `Manager` hands its own `StartAll` context to every service's `Start`
  (`service/service_manager.go:386` — `startErr := admitted.service.Start(ctx)`) and keeps no per-service cancel.
- What the three composition roots pass (inventory § 6): semsource gives `StartAll` a context that the signal cannot
  end (`cmd/semsource/run.go:196` — `context.WithCancel(context.WithoutCancel(signalCtx))`) and calls `StopAll(stopCtx)`
  on the signal (`:222`); semteams gives `StartAll` its parent context, not the signal's (`cmd/semteams/main.go:916`),
  and calls `StopAll(shutdownCtx)` on the signal (`:927-937`); semboids gives `StartAll` the signal context itself
  (`cmd/semboids/main.go:457` — `manager.StartAll(signalCtx)`) and then calls `StopAll` (`:465`).

Only option A is a shape ADR-113 already allows. B and C each keep a cancel per component, which `:107-108` rules out,
so either needs ADR-113 amended; they are listed for their costs, and Q-A is the owner confirming A.

| Option | What a component observes | Proving test | Cost |
| --- | --- | --- | --- |
| **A (recommended). Pass the manager's Start context through.** No per-component context or cancel. | Its `Start` gets the context the manager's `Start` got. That context ends only when the manager's caller ends it; the manager never ends it, `Stop` included. Its `Stop` gets the manager's `Stop` context. A `Stop` during `Start` waits for every `Start` to return (or returns its own context's error first); the caller cuts a slow boot short only by ending `Start`'s context (`113:59-60`). Once the starts return: if `Start` failed, its rollback runs first (inside `Start`), and the waiting `Stop` then stops only what the rollback left (nothing if it succeeded); if `Start` succeeded, the waiting `Stop` runs the stop sequence above. | `TestComponentStartContextIsTheManagersStartContext`: the context a component recorded is live after `Start` and after `Stop` while the caller's is live, and ends when the caller's is cancelled; ending it stops neither the component nor the manager. `TestStopDuringStartWaitsForComponentStarts`: one `Start` held open, `Stop` called; `Stop` does not return before the release, then stops each component once in reverse order and returns nil (replaces the pin's `TestComponentManagerStopDuringStartCancelsAndPreventsLateStoreCommit`). `TestStopDuringAFailingStartStopsWhatTheRollbackLeft`: a held `Start` that then fails while `Stop` waits, with one rollback `Stop` armed to fail once; `Start` returns both errors, the waiting `Stop` calls that component's `Stop` again and returns nil. | None in code (it deletes the mechanism). semsource and semteams: none, their `StartAll` context does not end on the signal. semboids: a component of its composition written to the carried contract (work hung off its Start context) stops when SIGTERM ends `signalCtx`, before `StopAll` reaches it: the race ADR-113 describes, found at shutdown by the suite inside this module and only by the doc comment outside it (`113:115-118`). Three pin tests flip (D7). |
| **B. A child context per component, ended when that component's `Start` returns.** Held only for the `Start` call. | Its `Start` context ends as soon as its own `Start` returned, success or failure. A `Stop` during `Start` may also end the children in flight. | `TestComponentStartContextEndsWhenItsStartReturns`; `TestStopDuringStartEndsStartsInFlight`. | A per-component cancel, so ADR-113 amended (`:107-108`); one context and cancel per launch, unlike PR 2's `Manager`. For all three consumers, a component that hangs work off its Start context stops at boot instead of at shutdown, silently unless its own health reports it; semboids' SIGTERM race goes, because the context it would end is already ended. |
| **C. A child context per component that outlives `Start`, ended after the component's `Stop`.** | Its `Start` context stays live until after its `Stop` returned, as at the pin's running stop. | `TestComponentStartContextEndsAfterItsStop`. | A per-component cancel, so ADR-113 amended; it hands components a context that bounds more than `Start`, the second stop ADR-113 removed. It keeps carried-contract components working in semsource and semteams; in semboids the parent is still `signalCtx`, so SIGTERM still ends them before `StopAll`. |
| **D. Do nothing (the pin as is).** | As the probe shows. | — | Fails `StartContextEndStopsNothing`; rejected by ADR-113 (option 1 on #282). |

**Who reacts to the root context ending (ADR-113 Decision ¶2, `:66-69`).** ADR-113 says "the component manager, the one
owner that watches that context, calls `Stop` on every component in reverse start order under a bounded stop context",
and "How the manager receives and watches the root context is change 3's to name". Named here: nothing in `service`
watches it. Under A the component manager receives the root's context only as `Start`'s bound, as PR 2's `Manager`
does with `StartAll`'s. The composition root reacts to SIGTERM by calling `StopAll` with a bounded context (semsource
`run.go:222`, semteams `main.go:937`, semboids `main.go:465`); `StopAll` stops the component manager, whose `Stop` stops
every component in reverse start order under that bound. So the decision is kept, the parent's cancel reaches a
component through the manager's `Stop` and never through an inherited context, and only the word "watches" moves from
the component manager to the composition root. Accepting A accepts a dated note under ADR-113's Decision recording
that (task 3.1). ADR-113 is this repository's record, so it is an amendment note, not a port note.

Under A, the store guard for a `Stop` that races a component's `Start` (pin `:1238-1247`) is unreachable and goes with
its only test (sweep C5). The waiting `Stop` rests on the guard: "Stop during a Start waits for that Start to finish", a
failed rollback "stays pending and the next Stop runs it again", and "Once ... a failed Start's rollback has succeeded,
Stop returns nil and runs nothing" (`go doc ./internal/lifecycleguard Guard.Stop`, `Guard.Start`;
`TestGuardStopWaitsForAnInFlightStart`, `TestGuardFailedRollbackLeavesCleanupForStop`).

## D3. Boot composition: built once, sealed, providers first

Governed by ADR-094, ADR-096 ¶4 and ADR-100, and pin ADR-063 through #321.

- `NewComponentManager(raw, deps)` decodes `raw` strictly (`decodeStrictServiceJSON`, so a key in another letter case is
  refused too, PR 2 D5), refuses a nil `deps`, a nil `deps.Manager` (pin `:155-157`), a config manager with no
  configuration (`:166-168`) and a nil `deps.ComponentRegistry` (sweep C2: "component-manager requires a component
  registry"; the pin's private fallback registry goes). It reads the configuration once: components, security, model
  registry, streams (`:165-175`; ADR-096 ¶4;
  `TestComponentManagerPostBootComponentAndModelWritesDoNotChangeRuntimeComposition`).
- For each enabled component it builds the instance through `Registry.CreateComponent`, running the component's
  `Initialize` before the registry publishes its declaration (`:1107-1118`), with `component.Dependencies` holding the
  NATS client, metrics registry, logger, `deps.Platform` (one spelling of the deployment's identity, ADR-102/104),
  security, model registry, payload registry and store registry. Disabled components are skipped. A component that fails
  to construct is decided by owner question Q-B: refused with every failed instance named
  (`TestComponentManagerRefusesBootWhenAComponentCannotBeBuilt`), or kept out with an Error line and a count on
  `semengine_errors_total{service="component-manager",type="component_construct"}`
  (`TestComponentConstructionFailureIsCountedAndLogged`). Enabled `rule-processor` instances that fail are collected,
  sorted by instance and refused together (`:339-372`,
  `TestComponentManagerInitializeAggregatesEnabledRulePackFailuresDeterministically`).
- It runs `composition.Analyze` over the registry's admitted declarations and the configuration's explicit streams,
  logs every finding, keeps the result for the HTTP projections, and refuses construction when any finding has error
  severity, naming each (`composition validation refused boot with N error finding(s): ...`,
  `TestComponentManagerRefusesBootOnErrorFinding`). Then it seals the registry (`Registry.SealComposition`).
- **Stores** (ADR-063 via #321). After a provider's `Start` returns nil, its `ProvidedStores` register in instance
  order; an empty instance name or a nil store is refused, a duplicate instance is refused without touching the holder,
  and the provider's own registrations from that call are removed (`TestDuplicateOwnership_RefusedNotClobbered`,
  `_RollsBackOnlyRivalRegistrations`, `TestRegisterProvidedStores_InvalidClaimFailsAtomically`, `_NilClaimFails`). A
  refusal fails that component's start.

## D4. The supervisor: component health and the override reporter

Sweep S8; pin ADR-098 (substrate observability is logs and metrics, `098:58-62`).

- **Health publisher.** With a NATS client it publishes `{"timestamp","name","health"}` for each admitted component on
  `health.component.<name>` once when `Start` returns and every 5 s; with none it publishes nothing. A publish or encode
  that fails is declared, not dropped (`AGENTS.md` § 1): it increments
  `semengine_errors_total{service="component-manager",type="health_publish"}` (`metric/core.go` `RecordError`, as PR 2's
  publisher does) and logs at Debug naming the subject, and the loop goes on
  (`TestComponentHealthPublishFailureIsCounted`, an unconnected client). An encode failure has no deployment trigger;
  its path shares the count untested (#185).
- **Override-expiry reporter.** The gauge `semengine_streams_migration_override_expired{stream,owner}` is registered
  through `metric.RegisterOrGet` at construction, and the manager uses the collector it returns (`AGENTS.md` § 1). The
  reporter evaluates the live configuration (`config.Manager.GetConfig().Get`, report-only, never composition authority)
  at construction and then every minute from `Start` to `Stop`: 0 for each declared bridge, 1 and a Warn line for each
  lapsed one; readiness is unaffected (`TestOverrideExpiry_*`, `TestComponentManagerStartRunsOverrideExpiryReporter`).
  Two managers built on one registry share the one gauge; the second's evaluation shows on the gathered family
  (`TestOverrideGaugeUsesTheRegisteredCollector`, reading `semengine_streams_migration_override_expired`).
- Both loops run under D2's run context; `Stop` returns only after both returned
  (`TestComponentManagerStopWaitsForTheOverrideExpiryLoop`, `TestSuperviseHoldsDoneUntilTheOverrideExpiryLoopReturns`);
  a failed `Start` launches neither (`TestComponentManagerFailedStartDoesNotLaunchOverrideExpiryLoop`).

## D5. HTTP surface (`operator-surface`; 03B operator rows; `docs/contract.md:131-145`)

- **Served as at the pin** under `/components/`: `health`, `list`, `types`, `types/{id}`, `status/{name}` (with
  `?debug=true` adding a component's `DebugStatus`), `config/{name}`, `flowgraph` (JSON, or Mermaid with
  `format=mermaid`), `validate`, `paths`; `GET` only, otherwise 405; a name that is empty, `.`, `..` or decodes to a
  path separator is 400, an unknown one 404 (`component_manager_http_test.go`, `attack_test.go`). Before boot commits
  only `health`, `list` and `status/` are served (PR 2 D6). `flowgraph`, `validate` and `paths` project the result kept
  at construction and compute nothing. The flowgraph answer is a tested contract (03B row; `docs/contract.md:143`):
  `TestFlowGraphMatchesGoldenForFixtureComposition` compares the JSON and Mermaid answers for a fixture composition with
  checked-in files under `service/testdata/`.
- **OpenAPI without an import-time registration (sweep C9).** `/openapi.json` takes its paths from each composed
  `HTTPHandler`'s `OpenAPISpec()` and its schemas only from the process-global registry
  (`service/service_manager_http.go:339-365`), so the pin registers the component manager's spec in `init` to make its
  two `SchemaRef`s (`#/components/schemas/Graph`, `Result`) resolve. Options: **(a, recommended: the required
  correction)** `RegisterAll`, the composition root's call, registers it: one line, it meets the binding requirement (no
  `init`; `service/openapi_registry.go:11-13`), and all three consumers call `RegisterAll` (semsource
  `cmd/semsource/run.go:372`, semteams `cmd/semteams/main.go:715`, semboids `cmd/semboids/main.go:402`); a composition
  that registers `NewComponentManager` by hand instead gets dangling references, which the README names. **(b,
  optional)** the manager's document also takes each composed handler's `ResponseTypes` and `RequestBodyTypes`, so a
  service's spec has one home; it changes PR 2's accepted `operator-surface` requirement and adds a contract rule, so it
  is not taken here.
  **(c)** keep the `init` (breaks the tree's contract line). Proof: `TestOpenAPIDocumentResolvesEverySchemaRef`, through
  `RegisterAll` and `StartAll` (every `$ref` in `/openapi.json` names a schema in it).
- **Config route (sweep C10, recommended):** the boot configuration is copied under the lock and encoded after it,
  through `writeJSON`, so a slow client holds no lock a `Stop` needs. Retained as at the pin and filed (D8, "To file"):
  C11, the manager's own health reporting healthy when it cannot tell (lock contention, `:1173-1177`, or not started,
  `:1156-1159`). Retained as at the pin and declared in the README: C13 (the 404 body for an unknown type), which breaks
  no promise: no spec or contract line states that route's error body.

## D6. Rule-pack binding and the predicate reconciler (owner Q2; ADR-091, ADR-110)

- `pkg/projection` regains `type PredicateReconciler interface { Reconcile(context.Context, ReconcileMutation)
  (MutationReceipt, error) }` (pin `mutation_types.go:152-155`) with `var _ PredicateReconciler =
  (*MutationClient)(nil)`. For: `service.ProjectionBinder.SetPredicateReconciler` (`rule_pack_bind.go:16`) now, and the
  rule processor in change 6 (pin `processor/rule/processor.go:463`), so both name one type (owner Q2). The ledger row's
  "five unread interfaces ... are removed" becomes four.
- `ComponentsImplementing[T](m)` returns the admitted components that implement `T`, read through
  `GetService(componentManagerName)` and `componentStartup.withComponents` (sweep C8); nil before a component manager
  exists. `ConfigureRulePackMutations(m)` refuses a nil manager; runs every binder's `PreflightProjectionMutations`
  before anything else, so one failure injects nothing; validates each pack ID (`rulepack.ValidateID`) and refuses a
  duplicate; copies each pack's contracts (`Contract.Clone`, sweep C7) and validates them; refuses contracts without a
  NATS client; then builds one `MutationClient` per pack that has contracts and injects it. Tests (new
  `rule_pack_bind_test.go`, through a binder double): `TestConfigureRulePackMutationsInjectsOneClientPerPack`,
  `_RefusesBeforeAnyInjection`, `_RefusesADuplicatePackID`, `_RequiresNATSForContracts`, `_SkipsAPackWithoutContracts`;
  and `TestComponentsImplementingReturnsTheBuiltComponentsThatImplementTheSeam` (carried).
- **Adopter seam.** A composition root must call `ConfigureRulePackMutations` after `ConfigureFromServices` and before
  `StartAll` (pin `internal/boot/run.go:330`; semteams `cmd/semteams/main.go:234`). If it does not, no binder gets a
  reconciler; with no rule processor in this tree until change 6 that has no effect today, and what the processor then
  does is change 6's to state. The README names the call; that is the one fact this surface asks an adopter to hold.

## D7. Tests

| Pin test file (lines) | Here |
| --- | --- |
| `component_manager_test_helpers_test.go` (71), `component_manager_lifecycle_test.go` (20) | one helper file; every manager is built through `NewComponentManager` with a test registry and configuration, not a struct literal (C1, C2) |
| `lifecycle_context_contract_test.go` (532) | `:177-532` adapted to D2 (outcomes, not private fields); `:1-175` not carried: their BaseService, Metrics and Manager halves are PR 1's and PR 2's suite adapters and `TestPropStopAllShutdownContract` |
| `component_manager_owner_test.go` (348), `component_manager_start_barrier_test.go` (425), `component_manager_storeregistry_test.go` (271) | adapted to D2-D3; `TestRegisterProvidedStores_SkipsWhenNotLive` goes with C5 |
| `component_manager_rulepack_init_test.go` (146), `component_manager_boot_only_test.go` (38), `component_manager_port_facts_test.go` (150), `component_manager_paths_test.go` (132), `component_manager_gaps_removed_test.go` (141) | adapted; `TestComponentManagerInitializeOrdinaryFailureRemainsBestEffort` follows Q-B |
| `component_manager_http_test.go` (395), `attack_test.go` (306), `component_catalog_test.go` (159) | carried; `attack_test.go:108`'s sleep becomes a wait on an observed outcome |
| `components_implementing_test.go` (87) | carried, plus D6's new file |
| `stream_override_expiry_test.go` (435) | `:146-170`'s embedded server not carried (C14); the composed tests use an unconnected client or the shared fixture |
| `component_manager_integration_test.go` (268), `component_manager_boot_findings_integration_test.go` (319), `component_manager_boot_only_integration_test.go` (128), `framework_owned_bucket_guards_integration_test.go` (362) | carried through `fixtureClient` (#195); `graph` catalog names from `graph/kvcatalog` |
| `config_boot_process_restart_integration_test.go` (265) | carried; the helper's `t.Skip` (`:106`) becomes a role dispatch in the package's `TestMain` (`service/fixture_client_integration_test.go:21`); `SEMSTREAMS_` variables read `SEMENGINE_` |
| PR 2's cut ranges of `startup_observability_test.go` and `_amendment_test.go` | the seven tests PR 2's `notes/2.1.md` part 5 names, appended to the two files in the tree |

- New: `TestComponentManagerPassesTheLifecycleSuite`, Q-A's three tests (option A),
  `TestStopRetriesOnlyComponentsThatDidNotStop`, `TestComponentManagerConfigRefusesEveryKey`,
  `TestComponentHealthPublishFailureIsCounted`, `TestOpenAPIDocumentResolvesEverySchemaRef`,
  `TestFlowGraphMatchesGoldenForFixtureComposition`, D6's five, Q-B's test, and the model check (Generated checks).
- Flipped by Q-A option A: `TestComponentManagerCallsComponentStopBeforeRuntimeCancellation` (the Start context is not
  ended by `Stop`), `TestComponentManagerStopDuringStartCancelsAndPreventsLateStoreCommit` (now
  `TestStopDuringStartWaitsForComponentStarts`), `TestComponentManagerFailedStartRollsBackSynchronouslyAndDoesNotReplay`
  (the rollback's `Stop` no longer sees a cancelled Start context).
- Harness rules as PR 2's D7: no `time.Sleep`, `Skip`, fixed port or `t.Parallel` in an integration file; every stop in
  a cleanup under `context.WithTimeout(context.WithoutCancel(t.Context()), ...)`. The developer units (one or two files,
  pin line ranges, names the tests call) are in `tasks.md` and `notes/2.1.md`, `notes/2.2.md`.
- Sensitivity (task 2.3; unit mutants through `task mutate:check`, the integration ones by hand, #129):

  | Mutant | Caught by |
  | --- | --- |
  | the supervisor's run context derived from `Start`'s | the suite's `StartContextEndStopsNothing` |
  | (A) a per-component child of `Start`'s context, cancelled in `Stop` | `TestComponentStartContextIsTheManagersStartContext` |
  | a `Stop` during `Start` that cancels the Starts | `TestStopDuringStartWaitsForComponentStarts` |
  | a component whose `Stop` returned nil called again by a retry | `TestStopRetriesOnlyComponentsThatDidNotStop` |
  | a failed running `Stop` made final | `TestStopRetriesOnlyComponentsThatDidNotStop` |
  | consumers launched before providers return | `TestComponentManagerStart_ProvidersRegisterBeforeConcurrentConsumers` |
  | stores deregistered after the provider's `Stop` | `TestComponentManagerRunningStopDeregistersStoreBeforeLiveChildStop` |
  | the publish failure not counted | `TestComponentHealthPublishFailureIsCounted` |
  | the override gauge's candidate used instead of `RegisterOrGet`'s result | `TestOverrideGaugeUsesTheRegisteredCollector` |
  | `RegisterAll` not registering the component manager's spec | `TestOpenAPIDocumentResolvesEverySchemaRef` |
  | preflight run per binder just before its injection | `TestConfigureRulePackMutationsRefusesBeforeAnyInjection` |
  | an error finding logged but not refused | `TestComponentManagerRefusesBootOnErrorFinding` (integration) |

## D8. Ledger, text, guidance, ADRs, coverage, issues

- **Ledger:** the `service` row edited in place: PR 3's files in `known_risks`; `contract` adds ADR-095, ADR-096 and
  ADR-100 and pin ADR-063 via #321; `dependencies_and_side_effects` adds `composition`, `model`, `pkg/projection`,
  `pkg/rulepack`, `storage/storeregistry`, and "the component manager publishes each component's health on
  `health.component.<name>` every 5 s and re-reads the configuration's stream overrides every minute". Changed from the
  pin, listed there: D1's drops; Q-A's answer; a failed `Stop` retried (C6); Stop during Start waits (C5); a nil
  component registry refused (C2); Q-B's answer; the stop order fixed by name (C4); the component manager's OpenAPI spec
  registered by `RegisterAll`, not on import (C9); the publish failure counted; namespace `semengine`. New rows for
  `docs/adr/095-...` and `docs/adr/096-...` in the ADR-094 row's shape. The `pkg/projection` row's `known_risks` reads
  "four unread interfaces ... removed; PredicateReconciler restored for service.ProjectionBinder (owner, #9 comment
  6296023694, Q2)".
- **Text:** `service/README.md` and `service/doc.go` gain the component manager: composing it (`RegisterAll` now
  registers it; PR 2's "until PR 3 a composition needs its own component-manager" goes), its configuration (none), the
  routes, `ComponentsImplementing` and `ConfigureRulePackMutations` with the call order D6 names, what a component's
  Start context is (Q-A), the declared degrade C13, and C11 with its issue. `docs/repository-map.md`: the ledger count
  (two ADR rows), the `service/` and `docs/adr/` rows.
- **Guidance** (pin `semstreams-developer.md:251`, `semstreams-reviewer.md:252`): the `SchemaRef` sentence in
  `.agents/contracts/semengine-developer.md:302-305` and `semengine-reviewer.md:327-329` becomes the rule "every
  `SchemaRef` type in a spec is listed in that spec's `ResponseTypes` or `RequestBodyTypes`, and the spec is registered
  by the composition root (`RegisterAll` for the framework's services), never from `init`", enforced by
  `TestOpenAPIDocumentResolvesEverySchemaRef`, with its row in `AGENTS.md` § 1 and `.agents/rules-index.md`
  (`verify+review`). No pin skill names this surface.
- **Coverage:** `"service merged"` is already in `scripts/cover-check.sh:46`; `"pkg/projection unit"` (`:36`) holds.
- **Issues.** Retired: none. Stay: **#196** (this PR composes the guard; adoption note on the issue, task 3.3; the
  message logger, PR 4, and changes 4-7 keep it open). **#195** (this PR's 12 test clients move to `fixtureClient`; PRs
  4 and 5 keep it open). **#321** (cited, not carried: owner's instruction for this change). **#280** item 2: the
  component manager reads neither port list by index (`grep -n 'InputFacts\|OutputFacts\|InputPorts\|OutputPorts'` over
  the five files: 0 hits; `composition` reads them), so the decision is the message logger's (PR 4). **#344**: the
  component manager's payload already goes through `decodeStrictServiceJSON`, so nothing here; the heartbeat half is PR
  6's. **#30**: the last `service` PR. **#309**: optional (no caller recovers a panic, so no trigger), noted in D2.
  Merge order: no open pull request shares a file (addendum).
- **Filed as #360** (protocol "File", #212; `status:blocked` on #359, linked without a closing keyword). Title: "service: the component manager's health says healthy when it cannot tell". Body: "The
  component manager's own health check, which `BaseService` runs and `Manager`'s readiness reads through
  `svc.IsHealthy()` (`service/service_manager_http.go:507`), returns healthy in two cases where it has not looked.
  (1) Not started: `if !cm.started.Load() { return nil // Still starting up, assume healthy }` (pin
  `service/component_manager.go:1156-1159`), reached in the window between the base service's start and the end of
  `Start`, and again once `Stop` has cleared `started`. (2) Lock contention: `if !cm.mu.TryRLock() { // Under write
  contention - assume healthy to avoid blocking. return nil }` (pin `:1173-1177`), reached whenever a component's state
  is being written (`updateComponentState` during start and stop). In both a positive signal stands in for 'unknown',
  so a component left in `StateFailed` can read healthy. The `TryRLock` is there for a real reason: a lock-acquiring
  goroutine abandoned on a timeout leaked a reader and deadlocked `Stop` at the pin (gh#508). Both are carried unchanged
  by service PR 3 (OpenSpec change `setup-04a-03-boot-service-component-manager`, design D5, sweep C11). Severity
  MEDIUM: the trigger is Go lock timing, a condition the code does not choose (#185). Expected when fixed: report the
  last completed result, or unhealthy with a reason, instead of healthy; a test holds the write lock and observes the
  answer." One issue for both sites, since they are one shape in one function pair.

## D9. Spec deltas (written in task 1.2, after acceptance)

- `service-lifecycle`, ADDED: "The component manager's Start bounds Start" (D2, with Q-A's answer; scenarios: the Start
  context ends after Start; Stop during Start; a failed Start's cleanup fails; a failed Stop is retried);
  "The component manager composes the boot set once" (D3 and Q-B; scenarios: an error finding refuses construction; a
  rule-pack failure; a nil registry; a key in the payload); "Providers start before consumers and own their stores"
  (D2-D3); "The component manager publishes component health and counts its drops" (D4); "A lapsed stream migration
  override is reported while running" (D4); "Rule-pack projection contracts are checked before any reconciler is
  injected" (D6). The existing "Every composition has a component manager" stays as written.
- `operator-surface`, ADDED: "The component manager's routes are served under /components" and "The composition
  projections serve the result kept at construction" (D5, the golden test). "/openapi.json documents the mounted
  routes" stays as written: under D5 option (a) the component manager's spec is a registered spec.

## Ruling to implementation

| Ruling | Implementation | Proving test |
| --- | --- | --- |
| Owner 2026-10-20 option B (#9 comment 6296023694); PR 2 D3 | `var _ componentStartup = (*ComponentManager)(nil)`; `RegisterAll` registers it | `go build ./service/`; the cut startup tests |
| Owner Q2 (same comment) | D6 `projection.PredicateReconciler` | D6's tests |
| ADR-113; pin ADR-095 (carried, changed); `lifecycle-suite` floor | D2 | `TestComponentManagerPassesTheLifecycleSuite`; `TestStopRetriesOnlyComponentsThatDidNotStop` |
| Q-A (owner, pending); ADR-113 Decision ¶2 (`:66-69`) | D2's component Start context; the dated note under ADR-113 | Q-A option A's three tests |
| Q-B (owner, pending) | D3's construction failure | Q-B's test |
| #196; ADR-108 Decision 2 | D2 guard | the suite; the adoption note |
| ADR-094, ADR-096 ¶4, ADR-100 | D3 | `TestComponentManagerRefusesBootOnErrorFinding`; the boot-only tests |
| Pin ADR-063 via #321 | D3 stores | the store-registry tests |
| Owner 2026-10-12 surface audit (#190) | D1 | `go doc ./service` lists only the kept surface |
| "skip, drop, degrade declared" (`AGENTS.md` § 1); pin ADR-098 | D4 | `TestComponentHealthPublishFailureIsCounted` |
| "imports register nothing" (`service/openapi_registry.go:11-13`) | D5 option (a) | `TestOpenAPIDocumentResolvesEverySchemaRef` |
| 03B operator rows; `docs/contract.md:131-145` | D5 | the HTTP tests; the golden test |
| Outward names read `semengine` (#69) | D4 gauge | `TestOverrideGaugeUsesTheRegisteredCollector` |
| ADR-110, pin ADR-091 | D6 (writes through graph-ingest) | review: no `ENTITY_STATES` write in `service` |
| #195; harness rules | D7 | `task verify` guards |

## Generated checks

Decided per `docs/testing.md` "Decide whether generated checks are needed":

- **Start and Stop histories** (order-dependent history). A `rapid` state machine over a manager composing up to four
  components (providers and consumers), each scripted to succeed or fail in `Start` and `Stop`. Actions: `Start`, `Stop`
  with a live context, `Stop` with an ended context, clear the armed failures. Checked after each action against a
  reference model: a `Stop` visits the not-yet-stopped components in exact reverse launch order (consumers by name
  descending, then providers by name descending); a component whose `Stop` returned nil is never visited again; `Stop`
  returns nil exactly when no armed failure fired and none was left; a `Start` after any `Stop` is refused and starts
  nothing; an ended context visits nothing. The run records its seed and check count
  (`TestPropComponentManagerStopHistory`).
- **Store registration** (a stated law, all or none per provider): examples suffice. Instances are judged one at a time
  in sorted order against the registry, so the pin's six examples (empty, nil, duplicate, rival rollback, reuse after
  stop, deregister before stop) cover each branch.
- **Rule-pack preflight, the override reporter, the HTTP routes:** examples suffice; each refusal or route is one
  example, the reporter's dates are fixed, and the flowgraph answer is a golden file.

## Owner questions

Two changes to what a component, or a deployment, sees are the owner's to accept. Each is written as the owner reads
it.

- **Q-A. Do you confirm that a shutdown arriving while components are still starting waits for them to finish
  starting?** Recommend yes. It is the one visible consequence of option A, the only shape ADR-113 already allows (it
  says the ported manager "holds no per-component cancel", `docs/adr/113-start-context-bounds-start.md:107-108`; B and C
  would need that record changed). Each component's `Start` gets the same context the component manager's `Start` got,
  which the manager never ends; a slow boot is cut short only by the caller ending that context (`:59-60`), as PR 2's
  service manager already does for services (`service/service_manager.go:386`). SemStreams instead cancels each
  component's context, in two orders, and cancels starts in flight when a stop arrives (pin
  `service/component_manager.go:622`, `:785-791`, `:1033-1042`; probe in `sweep.md` § 3). Once the starts return, a
  failing boot cleans up after itself first and the waiting stop releases whatever that left; a successful boot is
  stopped normally (D2, three tests). Accepting also accepts a dated note under ADR-113's Decision saying that the
  program's own shutdown code, not the component manager, reacts to SIGTERM by calling `StopAll` with a time limit
  (`:66-69`; all three consumers already do: semsource `cmd/semsource/run.go:222`, semteams `cmd/semteams/main.go:937`,
  semboids `cmd/semboids/main.go:465`). Cost by consumer: none for semsource and semteams; in semboids, which hands
  `StartAll` its signal context (`main.go:457`), a component written the old way stops at SIGTERM before `StopAll`
  reaches it.
- **Q-B. Should boot fail when a configured component cannot be built?** Recommend yes: today SemStreams logs the error
  and boots without that component, and `/readyz` then says ready (pin `service/component_manager.go:328-346`), while
  `AGENTS.md` asks every failure path to fail closed; the refusal would name every component that failed, as SemStreams
  already does for rule processors (`:339-372`). Measured cost of yes: semteams ships two example configurations that
  enable `graph-processor` (`configs/examples/bm25-semantic-search.json`,
  `configs/examples/pathrag-graph-traversal.json`), a factory no package at the pin registers (only a display name, pin
  `composition/catalog.go:42`), so both would stop booting until they are fixed; semteams' `configs/flow-bootstrap.json`
  names only factories the pin registers (its agentic ones and `graph-gateway` are not ported here, so they are the
  consumer's to register), and semboids' `configs/flock.json` names only registered ones (`sim` is its own,
  `internal/sim/factory.go:11`); semsource builds its components in code (`cmd/semsource/run.go:580` onward), not
  measured. Cost of no: the degrade must still be declared as a log line and a metric with a test observing both
  (`AGENTS.md` § 1), so the port adds `semengine_errors_total{service="component-manager",type="component_construct"}`
  beside the Error line and `TestComponentConstructionFailureIsCountedAndLogged`; `/readyz` keeps saying ready without
  the component.

The rest is answered by the repository: the rulings cited, ADR-113, the surface audit and Bounded port scope. Other
changes from the pin that an operator can see are listed in the ledger's `known_risks` (D8).
