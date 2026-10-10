# Tasks: setup-04a-02-ingest-kernel

Each task names the outcome that proves it and the gate that checks it. An unticked task that says "Hold:" is one
`task spec:queue` reports as blocked until the named review, ruling or task exists. "This pull request" is PR #93.
Where there is code, the failing test is written and shown failing before the code that makes it pass. Owner of each
task: **D** developer (`semengine-developer`), **R** reviewer (`semengine-reviewer`), **W** technical writer
(`semengine-technical-writer`), **A** architect, **O** the owner. Evidence is recorded on this pull request as a
comment unless a task says otherwise. New or repaired tests carry `// Requirement: <capability>/<Requirement
heading>` (#81's form). No task asserts a fact that exists only after merge.

## 1. Inventory, design and acceptance

- [x] 1.1 (R) Independent review of `inventory.md` (round 2, and round 3's §8), including its open evidence
      questions. Done: `INVENTORY PASS` at `0f781ac` and `d94dbad`, recorded in PR #93 comment 6035876773.
- [x] 1.2 (A) `design.md` and the deltas under `specs/` restated on the passed inventory; every requirement has a
      scenario; `task spec:check` passes. Done at `c5dd296`.
- [x] 1.3 (R) Independent pre-owner design review; PASS recorded with the reviewed files' checksums. Done: PR #93
      comment 6035876773.
- [x] 1.4 (O) Owner acceptance of the design on #91; PR #92 merged into `main`, `origin/main` merged into this branch,
      and #92's final file list re-read against design "Order with #92". Done: accepted on #91 (2026-10-07); #92
      merged as `1e383fe`, merged here in `8d75cd3`; its final 25 files match "Order with #92".
- [x] 1.5 (O) Owner ruling on question B: "port" only `graph/llm/client.go`, drop `ReviewConfig.LLM`, no
      authority-rule exception (#91 comment 6035477895). Applied in design D1, D1a, D6 and D9; the
      `harness-boundaries` delta no longer modifies "No second spelling of deployment authority"; `model/wire` follows
      the same reasoning (D1a, P-9).
- [x] 1.6 (O) Owner ruling on question C: "drop the token" (#91 comment 6035477895). Applied in design D1, D9 and
      D14; the `harness-boundaries` delta no longer modifies "Public signatures name no internal type".
- [x] 1.7 (O) Owner ruling on question E: `pkg/worker` is not ported, `internal/dispatch` declares its own "stopped"
      error (#91 comment 6035429806). Applied in design D1, D4, D5 and D6.
- [x] 1.8 (O) Owner ruling on question F: `graph/inference` joins the coverage gate in change 7; its `ReviewWorker`
      `synctest` test stays here (#91 comment 6035429806). Applied in design D6 and D11.
- [x] 1.9 (A) Question A, ruled on #77 (comment 6035317931), applied: design D7 settles the helper's public home and
      name (`pkg/lifecyclecleanup.RollbackFailedStart`), refuses a nil rollback, reconciles the `natsfixture` copy
      (foundation D9), and the `lifecycle-suite` delta writes the #38 exception for ported components; PR #93's body
      names `Closes #77` (#77 comment 6035358884).
- [x] 1.10 (A) Round 5: the pre-port audit (PR #93 comment 6036316289) folded into the change. Rulings #97, #98 and
      #99 applied (design D1, D1a, D11, D15); port-refactors #100–#104, #106's pattern and #111 item 1 designed with
      a failing-first test each (design D15–D21); the guard-state question of tasks 3.12 and 3.13 answered (design
      D22); `inventory.md` §9 with its probes; `task spec:check` and `task docs:check` pass on the change. Round 6:
      the owner's rulings A–F on the round-4 review (#91 comment 6037287957) and that review's four findings applied
      (design D1a, D6, D11, D15, D16, D17, D21, "Owner questions"; `inventory.md` §9.15). Round 7: rulings 1–5 on the
      round-6 review (#91 comment 6037604840) and that review's findings applied (design D15, "Left open", "Ruled").
- [x] 1.11 (R) Independent review of rounds 5 to 7: `INVENTORY PASS` on `inventory.md` §9 and a design PASS,
      recorded on this pull request with the reviewed files' checksums. Done: PR #93 comment 6037662484.
- [x] 1.12 (O) Owner acceptance of rounds 5 to 7 on #91. Done: accepted 2026-10-07 (#91 comment 6037996648).

## 2. Probes and harness

- [ ] 2.1 (D) Re-run inventory P-1, P-4 and P-7, and §9.1's slice runs, on the branch's copy at the first port commit
      and post the seeds, the hit list and the coverage figures; any difference from `inventory.md` is a finding on
      this pull request.
- [x] 2.2 (D) `TestOneImagePin` in Go files matches `nats:` followed by a digit or a variable, or `nats@sha256:` (design
      D10): sensitivity cases, written first, plant `"nats:in"` in a Go file (passes), `"nats:2.10"` and
      `"nats:${TAG}"` in a Go file (fail), and keep "digest literal in Go" failing. Gate: `task test:unit`.
- [x] 2.3 (D) `TestNoProcessGlobalRegistration` and its sensitivity test (`metric-registry`, "No process-global
      registration"). Gate: `task test:unit`.
- [x] 2.4 (D) `TestReservedSubjectsDeclaredOnce` and its sensitivity test (`graph-transport-boundary`, "Reserved
      request subjects have one declaration"): the two declaring files are `graph`'s verb table and
      `internal/graphmutation/protocol.go` (design D20). It is red until task 3.7 declares the subjects; written first.
      Hold: task 3.7; it lands in 3.7's commit, after being shown red, because a red test fails `task verify` on every
      push before it.
      Done: `internal/harness/contract/reservedsubjects_test.go`, red on `986953c` naming `graph/exact_entity.go:14`
      and green in task 3.7's commit. The reserved subjects are read from `graph.QueryVerbs` and
      `graphmutation.SubjectFamily`, so a verb added to the table is held from the commit that adds it.
- [ ] 2.5 (D) `TestPublicSignatures` and `TestNoSecondAuthorityField` pass over the ported tree with no new exception
      and no change to either check (rulings B and C; design D9, D14); the pass is posted with each check's package
      count. Gate: `task test:unit`. Hold: tasks 3.1–3.13 (the ported tree).
- [x] 2.6 (D) The `natsfixture` helper of design D3 (`nats-fixture`, "Connected value for a package's tests"), its three
      scenarios as tests, written first. Gate: `task test:integration`.
- [x] 2.7 (D) `internal/lifecycleguard` (design D13): `lifecycletest.Run` over a test owner built only from the guard,
      with a failing factory, written first. Gate: `task test:unit`.

## 3. Port, per package in design D1's order

Every port task: files land at the row's destination with import paths rewritten; tests travel with the package and
are repaired (sleeps, skips, fixed addresses, unbounded cleanups, the fixture client of design D3, the out-of-set test
imports of design D2); the package's dead surface (design D6, appendix of `inventory.md`) is removed after a `gopls
references` check, with the tests that read only it; READMEs are read claim by claim; `task verify` passes on the push.
Every test a task names as "written first" is shown failing before the code that makes it pass, and the failure is
posted on this pull request.

- [x] 3.1 (D) `types` (adapt: `ComponentConfig.Equal` refuses trailing input after the first JSON value, design D1),
      `storage` and `model` (carry). Done: the pin's files at their destinations with the module path rewritten,
      `types/README.md` corrected (name, import path, `config` and `service` arriving in change 3);
      `golang.org/x/net` enters at v0.59.0, the base's version (task 5.3); the `Equal` repair landed in `9f40bd4`.
- [x] 3.1a (D) `internal/lifecyclecleanup` → `pkg/lifecyclecleanup` (design D7): `RollbackFailedStart` keeps its name
      and its five-second budget; a nil rollback is refused (`TestRollbackFailedStartNilRollback` inverted to expect
      the error, written first and failing on the pin's code); the four scenarios of `lifecycle-suite` "Failed-start
      rollback helper" as tests; a package doc comment stating what a component that calls it must do (#77 inventory
      §10, the five facts); `internal/lifecycleguard` (task 2.7) calls it. Gate: `task test:unit`.
- [x] 3.2 (D) What is not ported, checked on the tree (design D1, D1a): no `graph/llm`, `graph/structural`,
      `model/wire`, `pkg/worker` or `internal/componentadmission` directory; `graph/inference` holds only the slice's
      files; no `go-openai` in `go.mod`. The `go list ./...` listing is posted.
      Done: checked at `6cbb88e`, after task 3.8. The five directories are absent; `graph/inference` holds the slice
      and its own `doc.go`; neither `go.mod` nor `go.sum` names `go-openai`. The listing (37 packages) is PR #93
      comment 6045139429.
- [ ] 3.3 (D) `graph`, the root (design D16, D15, D9): the data model and wire types only; `EntityState` without
      `Version` (`TestStoredEntityHasNoVersion`, written first); the write-mode rules as pure functions, with
      `TestReplaceOrderedByTimestamp`, `TestConfidenceAndContextNeverOrder` and
      `TestSingleValueReadPicksLatestAcrossSources` at rule level, written first (design D15);
      `ReconcilePredicatesRequest` with a required `source`; the reserved sources `graph-ingest-indexing-profile`,
      `graph-ingest-hierarchy` and `semengine-lifecycle` as three constants and `IsReservedSource` (ruling 5, design
      D15), with a rule-level `TestIsReservedSourceNamesAll` listing the three literal names; `IndexStatusResponse`
      without `Phase`, `Revision` and `LastSynced` and with `published_at` (`TestIndexStatusResponseHasNoLegacyFields`,
      written first, design D16); `ExactEntityReader` without the `natsclient` import, a zero timeout passed through;
      `events.go` not ported; `QueryResponse`, `NewQueryResponse`, `MinRevisionField` and the three aliases of
      `query_response_types.go` not in the root (ruled P, task 3.12k; #132 item (a) settled by removal); dead sentinels,
      `IncomingEdges` and the other dead `graph` rows removed, test-only exports
      unexported or moved into `_test.go` files; README rewritten, saying the query envelope arrives with change 4; the
      #69 rename of the configuration bucket constant (`semengine_config`); 11 fixture-client sites.
      `TestGraphImportsNoTransport` and its sensitivity test
      (`graph-transport-boundary`, "The graph root imports no transport"), written first and red on the pin-shaped root,
      land in this task's commit. Gate: `task test:unit`. Hold: the sweep of test-only exports waits for their readers
      (tasks 3.4–3.12k), since `gopls references` on the tree sees none before they are ported. The rest landed in the
      root's port commit, and the 11 fixture-client sites, in the catalog's test files, with task 3.3a.
- [x] 3.3a (D) `graph/kvcatalog` (design D16): `kvcatalog.go`, `owned_bucket_retention.go` and `IsKVTombstone` from
      the pin's `graph`, with their tests; no `TOOL_CALL_OUTCOMES` or `ENTITY_SUFFIX_INDEX` row or constant; the
      configuration bucket's description names SemEngine (design D9); `TestEntityStatesKeepsOneRevision`
      (`graph-entity-writes`, "One stored revision per entity"). Gate: `task test:unit`, `task test:integration`.
      Done: the 11 fixture-client sites call one `natsfixture.Open` helper; `FrameworkOwnedBuckets`, dead at the pin
      (appendix), is dropped with the test of its unexported derivation; `TestEntityStatesKeepsOneRevision` holds the
      pin's behavior and its `task mutate:check` record (a mutant declaring `History` 2) is posted on this pull request.
- [x] 3.3b (D) The reply envelope and the verb table in `graph` (design D18, D20): `QueryResponse` with
      `indexed_revision` and `producer`, built only with both (`TestQueryResponseCarriesIndexedRevisionAndProducer`,
      written first); the `min_revision` request field, declared and embedded in no type yet (design D18); a constructor
      that refuses an empty producer; no `UnwrapQueryResponse`; the verb table with `entity`, `batch` and `prefix`,
      responder `graph-ingest`. Gate: `task test:unit`.
      Done: `MinRevisionField` in `graph/query_contracts.go`; `QueryVerbs` in `graph/query_verbs.go`, its request and
      reply types written as strings, as the pin's graph-query table writes them; `UnwrapQueryResponse`, its key-set
      constants and its four tests removed; the field-set test now holds the four keys. Three `task mutate:check`
      detections (constructor drops the revision, accepts an empty producer; table regains a suffix verb) are quoted in
      the commit body. Later: ruled P (#91 comment 6080973822), the type, its constructor, `MinRevisionField` and their
      five tests leave with task 3.12k and return with change 4; the verb table stays. Of the three mutation records,
      the two on the constructor are of code no longer in the tree; the one on the verb table (`query_verbs_test.go:34`)
      stands.
- [x] 3.4 (D) `graph/readiness`: `Watcher` takes `Run(ctx)` (design D5), with `synctest` tests that `Run` returns
      `ctx.Err()` and leaves nothing running; `Set`, `NewSet`, `Dump`, `Verdict` removed, the row's `known_risks`
      naming their return with #110; `Publisher.Publish` sets `published_at` (`TestPublishStampsPublishedAt`, written
      first) and the inputs lose `LastSynced` (design D16); gauges through
      `RegisterOrGet` under `semengine` (design D4); 2 sleeps repaired; receives `readiness_gate.go` and the
      computation in `index_status.go` from `graph`, with their tests (design D16).
      Done: `Run` refuses a nil context, a nil source and an empty key before any I/O; `Start` and `Stop` are gone.
      `set.go` and `set_test.go` are not ported, and the two sleeps were both in `set_test.go` (`:85`, `:302`), so
      they leave with it; the `known_risks` text naming `Set`'s return with #110 is written with the row in task 5.1.
      `Gauges.Register` returns the registry's refusal, and with a nil registry registers nothing (the pin fell back
      to Prometheus' global registry). `TestComputeIndexStatus_PreExistingFieldsUnchanged` asserts `IndexedRevision`
      where it asserted the removed `Revision` string. Six `task mutate:check` detections are quoted in the commit body.
      Later: ruled P (#91 comment 6080973822), narrowed (#91 comment 6085720598): task 3.12k removes
      `ComputeIndexStatus` and `WithRevisionGauges` with their tests; the `Watcher` and the gate stay. Of the six
      mutation records, the three on `Run` stand, and 3.12k re-runs the three on `Register`. What stays from this task:
      the `Watcher` in its `Run(ctx)` shape, the gate, the publisher with `published_at`, the gauges through
      `RegisterOrGet`, `ComputeBacklogStatus`, and the drop of `Set`.
- [x] 3.5 (D) `internal/graphmutation` (`InterfaceType` becomes `semengine.graph.mutation`, design D9) and
      `storage/storeregistry` (the `pkg/fusion` assertion removed, noted for change 5).
      Done: `TestInterfaceTypeNamesSemEngine`, written first, holds the new name; `SubjectFamily` stays at
      `protocol.go:16` (D20). `IsCommitUnknown` and `Registry.Instances` removed (D6). The three tests that called
      `IsCommitUnknown` assert `errors.As` to `*TransportError` with `TransportCommitUnknown`, the function's own
      test. `TestInstancesSnapshot` becomes `TestDistinctInstancesResolveTheirOwnStores` (both names resolve, each
      to its own store); its copy half left with the method. The no-op `Deregister` test reads `Streamable` for the
      name. Not in the design: the client refuses a nil context before any request (developer contract, "Context
      ownership"; `TestClientRefusesNilContext`, written first). For task 5.1: the `storeregistry_test.go:30`
      `fusion.StoreResolver` assertion is removed and returns with `pkg/fusion` in change 5, a `port-refactor` note
      on both rows (D2); the `graphmutation` row's adapt items are the rename, the drop and the nil-context refusal.
      Seven `task mutate:check` detections are quoted in the commit body.
- [x] 3.6 (D) `pkg/dispatch` → `internal/dispatch`: `BoundedDispatcher`, `New`, `Config`, `Deps`, `ErrQueueFull`,
      `KeyedPool.Submit` and `Stats` removed; `ErrStopped` declared in `internal/dispatch`, with no `pkg/worker` import
      (ruling E); `KeyedPool.Shutdown(ctx)` with no fixed default wait
      (`TestKeyedPoolShutdownWithoutDeadlineWaitsForJoin`, `synctest`); metrics through `RegisterOrGet`; unbounded
      cleanups, sleeps and the fixture-client site repaired.
      Done: ported `keyed_pool.go`, `keyed_pool_test.go` and `errors.go`; `doc.go` rewritten for `KeyedPool` alone.
      `completion_watcher.go` leaves too, a transitive drop: its only reader was `dispatcher.go:138`. With them go
      `dispatcher_test.go`, `completion_watcher_test.go` and `integration_test.go`, which read no `KeyedPool`; the
      pin's five sleeps (`integration_test.go:131`, `:138`, `:208`; `dispatcher_test.go:151`, `:243`) and its one
      fixture-client site (`integration_test.go:37`) were all in them, so they leave with them. `ErrLaneFull` and
      `dispatch_dropped_total` leave with `Submit`, their only producer and writer; `ErrNATSClientRequired` with
      `New`. The unbounded second `Stop` of `TestKeyedPool_StopDeadlineCanResumeConstructorOwnedDrain` becomes
      `TestKeyedPoolShutdownUnderABlockedProcess`, in a `synctest` bubble with an hour-bounded later `Shutdown`.
      `Shutdown` refuses a nil context; the drain starts at the first `Shutdown`, so cancelling the run context
      leaves nothing running (`TestKeyedPoolRunContextCancelLeavesNothingRunning`, failing first); `NewKeyedPool`
      returns a registration refusal (`TestKeyedPoolReturnsARegistrationRefusal`, failing first). Tests that read
      `Stats` read the pool's counters or their own. Nine `task mutate:check` detections are quoted in the commit
      body.
- [x] 3.7 (D) `graph`'s `ExactEntityReader` takes its subject from the verb table, and graph-ingest's literal sites
      use the table in task 3.12 (#16, design D20). Task 2.4's test turns green here; in task 3.12 graph-ingest's
      literal sites must take their subjects from the table to keep it green.
      Done: `ReadExactEntity` requests on `entityVerb.Subject`, the table's entity entry, now named in
      `query_verbs.go`; `exactEntityQuerySubject` is gone. Two `task mutate:check` detections and two wrong changes
      made by hand (the check reads files from disk; the checker is a test file) are quoted in the commit body.
- [x] 3.8 (D) `graph/inference`, the hierarchy slice only (#97, design D1a): `hierarchy.go`, `container_entity.go` and
      the `TripleAdder` interface, with a package comment for what the slice holds; `HierarchyConfig` without
      `Org`/`Platform`, the carrier passed to the constructor (design D9); `DefaultHierarchyConfig`, `OnEntityCreated`,
      `ClearCache`, `GetMetrics` and `GetCacheStats` removed, the tests that called `OnEntityCreated` calling
      `GetHierarchyTriples` and the adder; `GetHierarchyTriples` returns every failure, an inverse edge's and the
      sibling pass's included (`TestGetHierarchyTriplesReportsEveryFailure`, written first, design D21); hierarchy
      statements carry graph-ingest's producer as `Source` and the triggering time, which `GetHierarchyTriples` takes
      as an argument and passes to the containers it creates (design D15); 1 sleep repaired.
      Done: ported `hierarchy.go`, `container_entity.go` (`RegisterPayloads` kept for task 4.6) and the three test
      files; `TripleAdder` alone in `applier.go`; a new `doc.go` describes the slice. The constructor is
      `NewHierarchyInference(entityManager, tripleAdder, config, platform types.PlatformMeta, logger)` and the call is
      `GetHierarchyTriples(ctx, entityID, at time.Time)`. With the five dropped methods go the three counters only
      `GetMetrics` read, and the tests of that surface (`TestDefaultHierarchyConfig`,
      `TestHierarchyInference_ClearCache` and `_GetMetrics`, `TestAttack_ClearCacheDuringOperations`,
      `TestAttack_MetricsConcurrency`).
      `TestHierarchyInference_InverseEdgeWriteFailureIsNonFatal`, which held the pin's warning, gives way to
      `TestGetHierarchyTriplesReportsEveryFailure` (failing first): on any failure the call returns the failures joined
      and no statements. One helper builds every hierarchy statement with `graph.SourceHierarchy` and the time passed
      in (`TestGetHierarchyTriplesStampsSourceAndTime`, failing first); a zero time is refused
      (`TestGetHierarchyTriplesRefusesZeroTime`). The sleep in `TestAttack_GoroutineCount` gives way to a `synctest`
      bubble. Task 3.12 must pass `deps.Platform` to the constructor and the triggering time to the call, and fail
      the birth on its error (design D21, task 4.9). Fourteen `task mutate:check` detections and one wrong change made
      by hand (`TestNoSecondAuthorityField` reads files from disk) are quoted in the commit body. Later: ruled M (#91
      comment 6080973822; #145), task 3.12j removes the inverse-edge write, and with it `TripleAdder`, `AddTriple` and
      `NewHierarchyInference`'s adder argument; a failure is a container's or a forward edge's.
- [x] 3.9 (D) `pkg/projection`: carried code without `Version`; the typed client refuses a statement with no
      timestamp before sending, on `Create`, `Append` and `Reconcile` (`TestMutationClientRefusesMissingTimestamp`,
      written first; `projection-mutation`, "The typed client never reads the clock"); `Reconcile` requires
      `Metadata.Source` and sends it as the request's source (`TestMutationClientReconcileRequiresSource`, written
      first; design D15); its repair is tasks 4.1–4.2.
      Done: ported `contract.go`, `doc.go`, `mutation_client.go`, `mutation_types.go`, `contract_test.go` and
      `mutation_client_test.go`; the tests build `graph.EntityState` without `Version`, and the whole surface stays
      (design D6, K1). `canonicalizeTriples`, which `Create`, `Append` and `Reconcile` all call before any request
      (the exact read included), refuses a statement with no `Timestamp` under metadata with none, and metadata
      with no `Source` on all three, as `MutationInvalid`, class invalid, not-committed; the clock read is gone.
      `Reconcile` sends `Metadata.Source` as the request's `source`. The two tests, written first in
      `mutation_client_refusals_test.go`, fail on the pin's code. The carried tests now give their statements a
      time and their reconciles a source. Seven `task mutate:check` detections are quoted in the commit body.
- [x] 3.10 (D) `pkg/lifecycle`: 7 fixture-client sites; `harness_gate_integration_test.go` on per-package payload
      registration; `Manager.Watch` and `WatchEvents` in the watch shape of design D5; no `Version: 1`
      (design D15); every statement the manager writes carries the constant source `semengine-lifecycle`, which its
      reconciles send as the request's source (`TestTransitionReplacesItsPhaseStatement`, written first: two
      transitions with different `TransitionSource`s leave one phase statement carrying `semengine-lifecycle`; design
      D15); the emitter's error no longer discarded (`TestManagerWithoutClientRefusesEmit`, written first, design
      D17).
      Done: ported every file but `harness_gate_integration_test.go`, the whole surface kept (design D6, K1); no
      `Version: 1`; the 6 fixture-client sites of `manager_integration_test.go` open through `natsfixture.Open`.
      `Watch` and `WatchEvents` take a callback that runs on the caller's goroutine and return when the watch ends,
      with nothing left running; a nil context is refused before a subscription opens (`watch_test.go`, in `synctest`
      bubbles). Every statement the manager builds carries `semengine-lifecycle`, and its three reconcile builders
      send it as the request's `source` (`TestManagerWritesUnderTheLifecycleSource`, request level, written first).
      With no NATS client every write returns `ErrEmitFailed` naming the missing client
      (`TestManagerWithoutClientRefusesEmit`; on the pin's code it panics). The commit body quotes the failing runs
      and the mutation records. With task 3.12: `harness_gate_integration_test.go` ported into package `lifecycle`,
      its registry `payloadfixture.NewWithSubset(t, RegisterPayloads)` (design D2), its fixture-client site (the
      seventh) opened through `natsfixture.Open` with the package's `openClient`, and graph-ingest built on its own
      metrics registry, from which the rejection counter is read (design D4). `TestTransitionReplacesItsPhaseStatement`
      (`transition_phase_integration_test.go`) drives a create and two transitions, `rule` then `operator`, through a
      real graph-ingest: the entity holds one phase statement, and it carries `semengine-lifecycle`. It passes on this
      tree without task 3.12a's conditional replace by source. It was written after the manager's change, which
      `TestManagerWritesUnderTheLifecycleSource` drove first, so its red runs are wrong changes made by hand (records
      in the bodies of `b95e790` and `97d9ac6`): lifecycle's registration skipped, the birth is refused at
      graph-ingest's registered-type gate; the phase statement's source taken from the `TransitionSource`, the source
      assertion fails; graph-ingest's reconcile keeping the earlier phase statements, the count assertion fails.
- [x] 3.11 (D) `component`: `ToolRegistry` and `ToolRegistryReader` removed (#29) and `LifecycleManager` removed
      (#102, design D17); `lifecycle_test_suite.go` and its self-test not ported; `ProcessorMetrics`,
      `config_validator.go`, `Registry.Snapshot` and the other dead rows removed; `CreateComponent`,
      `SealComposition` and `Snapshots` without the access-token parameter, their doc comments directing callers to
      the component manager, and the token's 19 test uses dropped (design D14); 3 fixture-client sites, 1 sleep
      repaired. The contract test that `go list -deps ./component` lists no agentic or graph-family path, with its
      sensitivity test (`component-registration`, "The component model reaches no agentic or graph package"),
      written first and red on the pin's `dependencies.go`, lands in this task's commit.
      Done: ported 42 files; `Dependencies` has no `ToolRegistry` or `LifecycleManager`, and `ToolRegistryReader` is
      gone. Not ported: `lifecycle_test_suite.go` and `lifecycle_test_support_test.go`; `config_validator.go` and its
      test; `metrics.go` (`ProcessorMetrics`); `registerable.go`; `logging.go` and its test; `main_test.go`, whose
      `TestMain` opens a shared NATS client no test reads; `component/flowgraph/`, a separate package only
      `composition/` imports. Dropped with the D6 rows (`GetString`, `GetInt`, `GetBool`, `GetFloat64`,
      `ValidateJSONSize`, `Registry.Snapshot`, `IsLifecycleComponent`, `MergePortConfig` with `mergePortDirection`):
      `MaxInt` and `MinInt`, which only `GetInt` and `GetFloat64` read. `CreateComponent(instanceName, config, deps,
      prepare)`, `SealComposition()` and `Snapshots()` take no token, the private `createComponent` folded into
      `CreateComponent`; their doc comments name `service.ComponentManager` in plain text; the token's 19 test uses are
      gone. The 3 fixture-client sites open through `natsfixture.Open`; the pin's sleep (`attack_test.go:62`) is
      replaced by a `synctest` bubble. `TestComponentReachesNoAgenticOrGraphPackage` and its sensitivity test
      (`internal/harness/contract/componentdeps_test.go`) failed first on the pin's `dependencies.go`, with only its
      agentic import cut, naming `graph`, `graph/kvcatalog`, `internal/graphmutation`, `pkg/projection` and
      `pkg/lifecycle`. `component/README.md` read claim by claim and rewritten to what the code does. Shown able to
      fail (records in the commit body): `LifecycleManager` restored, the contract test names the same five paths;
      the `agentic-` rule removed, the sensitivity test's `processor/agentic-tools` case fails; a goroutine left
      blocked in the mock's `DebugStatus`, the leak test panics; `task mutate:check` on `CreateComponent`'s seal
      check, detection at `registry_boot_admission_test.go:160`.
      Removed (found 2026-10-09, #153 item 1): the rows `inventory.md:958-978` marks `drop`, each shown by `gopls
      references` to have no reader outside its own tests: `Registry.ListComponentTypes`, `GetFactory`,
      `ListAvailable` with `Info`, `GetComponentSchema`, `declarationSnapshot.Factory`, `PortFacts.KVReadBucket`,
      `StreamFacts.ConsumerName`, `GetPropertyValue`, `GetProperties`, `IsComplexType`, `SortedPropertyNames`,
      `PortFieldInfo.ZeroIsOmitted`, `ValidateNetworkConfig` and `ValidateConfigKey`. With them went what only they
      read: `ValidatePortNumber`, `MinPort` and `MaxPort` (read by `ValidateNetworkConfig` alone), the private fields
      `PortFacts.kvReadBucket` and `StreamFacts.consumerName`, and the copy of `zeroIsOmitted` into discovery
      metadata. `PortFieldInfo.zeroIsOmitted` stays: the port resolver reads it on a port kind's field constraints
      (`component/port_resolver.go:113`). `Registry.ListFactories`, whose one production reader was `ListAvailable`,
      stays: tests read it (`component/registry_test.go`, `processor/graph-ingest/component_test.go:757`), and it has
      no D6 row. Tests whose only subject was a removed name went with it (`TestGetPropertyValue`,
      `TestGetPropertiesByCategory`, `TestGetPropertiesDefaultCategory`, `TestComplexTypeDetection`,
      `TestSchemaOrdering`, three `TestSchemaFallback` subtests); the `ConsumerName`, `ZeroIsOmitted` and `Factory`
      checks were cut from tests that check other facts, the `Factory` one now reading `Declaration().FactoryIdentity`.
      `component/README.md`'s Registry list names only `ListFactories`.
      Done in `2799632` (the rows above): `task verify` green on `c6515e9` (this branch after `main` at `b58d5fb` was
      merged in, `370b384`). Notes: PR #93 comment 6087157769.
- [x] 3.12 (D) `processor/graph-ingest`: metrics per design D4 (`TestGraphIngestMetricsRegisterOnItsRegistry`,
      `TestGraphIngestNilRegistryRegistersNothing`, written first); the authority as one `types.PlatformMeta`; unknown
      configuration keys refused by strict decoding (`TestCreateGraphIngestRefusesUnknownKey`); the owner-lifecycle
      state replaced by `internal/lifecycleguard` (design D13), the tests that read or set its fields driving the
      guard through its methods (design D22); `MergeEntity` removed and its 27 test call sites calling
      `mergeEntityOnLane(ctx, entity, false)` (design D6); the suffix index, its cache and the suffix verb not ported
      (`TestGraphIngestProvisionsNoSuffixIndex`, written first, design D19); subscriptions taken from the verb table
      (`TestGraphIngestServesExactlyTheDeclaredVerbs`, written first, design D20); `lastSyncedRFC3339` and its test
      not ported (design D16); `TEST_DISPUTE.md` not ported; 21
      fixture-client sites, 10 sleeps, 6 skips (rewritten as integration tests), 11 fixed addresses, 5 unbounded
      cleanups repaired; the eight out-of-set test imports adapted (design D2).
      Done: production in `18e6a77`; unit tests in `e8b3c31`, `d88d688`, `be10223`, `d09ebd1` and `88d3692`;
      integration tests in `ab69042`, `4ed6266`, `4c2a3a7` and `67d3bda`. Counted at the SemStreams pin and on the
      tree at `67d3bda`: all 22 of the pin's integration test files and 33 of its 34 unit test files are on the tree.
      `indexing_profile_registry_test.go` is not ported (design D2); three of its tests moved into
      `indexing_profile_test.go`. `TEST_DISPUTE.md` is absent. Repaired, with none of each left on the tree: 21
      fixture-client sites (the `TestMain`'s `NewSharedTestClient` is not ported), 10 sleeps, 6 skips (rewritten in
      `component_integration_test.go`), 11 fixed addresses, 5 unbounded cleanups, and 27 `MergeEntity` test call sites
      across 10 files. None of the eight out-of-set test imports is left (design D2); `pkg/lifecycle`'s
      `harness_gate_integration_test.go` is adapted under task 3.10. Each commit body quotes its failing runs and
      mutation records. The choices the design does not spell out are in PR #93's "Notes for checkpoint 3" comments
      (6050325985, 6050697147, 6051342812, 6052103765 and 6053166232).
- [x] 3.12a (D) graph-ingest's write seam (#100, #98; design D15): one write path for the seven write sites
      (`TestEntityWritesHaveOneSeam` and its sensitivity case, written first); births on every lane stamp the profile,
      and with hierarchy enabled the stream and in-process births add hierarchy statements while a mutation create adds
      none (`TestMutationCreateBirthGetsNoHierarchy`, which holds the pin's behavior, ruling B; its `task mutate:check`
      record, with a mutant that runs the inference on that lane, posted); derived statements take the latest
      `Timestamp` among the write's own statements, read before graph-ingest adds its own, reserved sources included,
      and a stream arrival with no statement is poison (design D15; `TestStreamLaneRefusesEmptyArrival`);
      `TestReplaceEntityRetryAfterLostBirthMergesOnlyTheArrival` drops its clock-dependent profile check; the tests of
      task 4.8. The seam reads its revision from `natsclient.KVStore.UpdateWithRetryRead` (design D23). One commit adds
      D23's natsclient tests, the README entry, the `kv.go:323` doc, the `natsclient` row's three adapt items, and
      `UpdateWithRetryRead` and `UpdateJSON` in `entity_writes_seam_test.go`'s map with a planted case for the first.
      Then, each failing first on `930bf49`: a no-op append reports `unchanged` at the revision it read, a key deleted
      after the read included (`TestCanonicalAppendNoOpReportsTheRevisionItRead`); an all-older arrival writes nothing
      over a profiled entity and still counts each set (`TestStreamArrivalAllOlderWritesNothing`, with an unprofiled
      case that writes once and stamps the profile, its `task mutate:check` mutant "skip regardless of profile"); on the
      stream and append lanes, a value under another key and an empty value are refused and recorded at the revision
      read (`TestWriteSeamRefusesValueUnderAnotherKey`, `TestWriteSeamRefusesEmptyStoredValue`); `errNoOpAddDuplicate`
      and `inventoryEntityPoisonAtCurrentRevision` go; the adoption issue (#141) is filed. Each commit body quotes its
      failing runs and mutation records. The choices the design does not spell out are in PR #93's "Notes for checkpoint
      3" comments (6062508678, 6063769189, 6064772182, 6065164632, 6065823926, 6067057540, 6067512048, 6068062023,
      6069037847, 6069561642 and 6069916935).
- [x] 3.12b (D) Fail closed at birth and on the guard record (#111 item 1; design D21): the tests of task 4.9; and #130,
      the hierarchy container cache that is never invalidated, fixed here, since failing the birth closed turns it from
      a warning into a redelivery that never ends (PR #93 comment 6061824292). With it, `graph/inference` as #134
      routes here: a constructor that takes the store, the authority and the logger, refuses an empty authority, and
      whose nil result means hierarchy is off (item 1); one interface over graph-ingest with one adapter, and the verb
      named for what it writes (item 2); sibling edges, `ListWithPrefix` and `enable_type_siblings` dropped (item 3,
      ruled G, #91 comment 6062681355; `hierarchy.type.sibling` stays registered in `vocabulary`); test doubles that
      refuse what graph-ingest refuses (item 4); and graph-ingest's two hierarchy fetches made one (#139 section D).
      Done in seven commits, each failing first and with its mutation records in its body: `999f3f5` (#130: each birth
      asks storage, the doubles refuse as graph-ingest does), `c12dbbf` and `f58d311` (#134 items 1–3; the verb is
      `AddToContainers`), `92c19f6` (item 4's second half: no watchdogs, exact counts), `c4a885f` (a failed inference
      fails the birth on both lanes, one fetch; `TestHierarchyFailureFailsTheBirth`), `ecf00a7` (a key deleted after
      the stream lane's birth read stores nothing and is redelivered, D21 as `dd8d2a7` wrote it) and `36e370d` (a
      guard record of any length but eight is refused, counted and logged once per key;
      `TestCorruptGuardRecordIsRefused`). Choices the design does not spell out: PR #93 comments 6072002735,
      6072192627, 6072528806, 6072740123, 6073028310 and 6073367616. CI green at `074c800` (run 37926039654), once
      `main`'s go1.26.9 (PR #143, #142) was merged in and `golang.org/x/net` raised under task 5.3.
- [x] 3.12c (D) A boot sweep that cannot run fails `Start` (ruled H, #91 comment 6062681355; #139 section C, first
      item): `entityWatchLost`, `entityBootstrapStarted`, `entityBootstrapComplete`, `markEntityWatchLost` and their
      reader branches go; a test written first, failing on the pin's code, makes the sweep fail and shows `Start`
      returning the error with nothing left running.
      Done in `8e0ecd1` (CI 37929231613 green): `TestGraphIngest_FailedBootSweepFailsStart`
      (`lifecycle_integration_test.go`) for each failure (a refused watch, updates closed before the end-of-snapshot
      marker, the context ended mid-sweep), failing first on `52611ea`; `TestEntityStateGuardFailureIsReturned`
      replaces the test of the old behaviour; `ensureEntityQueriesReady` and Health's "sweep unavailable" branch go;
      one unexported seam, `watchEntityStates`. Mutation records and choices: PR #93 comment 6080843431.
- [x] 3.12d (D) #147: the boot sweep (`sweepEntityStateGuardEntry`) and `classifyStoredStateRMWError` decode through
      `decodeStoredEntity`, so a value stored under one key that names another entity is inventoried at boot, not at
      its first read; a test written first, failing on the current code.
      Done in `6102b75` (CI 37931629741 green): `TestBootSweepInventoriesMisKeyedValue`, failing first on `707205a`;
      no caller can hand the classifier a mis-keyed value, so it decodes through the one rule with no test of its own.
      Notes and the mutation record: PR #93 comment 6081213285.
- [x] 3.12e (D) #150: on an entity with an indexing profile, an arrival whose only statement is the indexing profile
      writes nothing, older or newer, and counts no set as not applied (D23; `graph-entity-writes`, "A profile-only
      arrival"; `7f8d9fc` is the sibling fix); a test written first, failing on the current code, shows the revision and
      the stored value unchanged. Done in `1edc247` (CI 37934155773 green): `TestStreamArrivalOnlyProfileWritesNothing`,
      older and newer cases, failing first on `8f8f8c8`. A profile-only arrival on a profiled entity declines whatever
      its age, since the immutable profile is dropped before the replace and nothing of the arrival can apply; a newer
      one no longer updates `MessageType` or `StorageRef`. Notes and the mutation record: PR #93 comment 6081549343.
- [x] 3.12f (D) #149: the revision fences on canonical reconcile and delete get tests that can fail: a write injected
      between the handler's read and its write, and the unit bucket's `Delete` checking `LastRevision`; a
      `task mutate:check` record for each fence (the fenced update as a `Put`, `DeleteAtRevision` as `Delete`).
      Done in `8fb4408`: `TestCanonicalReconcileFenceHoldsAgainstAWriteAfterItsRead` and
      `TestCanonicalDeleteFenceHoldsAgainstAWriteAfterItsRead`; each mutant a detection by the new test and a survivor
      of the old one; no production change, both fences hold. CI on `8fb4408` (run 37936839601) failed on the known
      flake #144 alone; CI on `f4364bc`, which carries it, is green (run 37939020158). Notes: PR #93 comment
      6081968133.
- [x] 3.12g (D) #151 and #152: `graph.NewExactEntityReader` refuses a negative timeout at construction (zero keeps
      meaning the default, D16); `lifecycle.Manager.CreateFromOperator` refuses a second JSON value after the first,
      as `types/component.go` does since task 3.1; a test for each, written first.
      Done in `8504a73` (CI 37942461597 green): `NewExactEntityReader` returns `(ExactEntityReader, error)`;
      `NewManager` panics on the refusal its 5 s constant cannot cause; mutation records and choices in PR #93 comment
      6082801917.
- [x] 3.12h (D) #146, ruled N (#91 comment 6080973822): `natsclient.IsKVConflictError` and `IsKVNotFoundError`
      classify by type only (`errors.Is` on natsclient's sentinels and on jetstream's `ErrKeyNotFound`, `ErrKeyDeleted`
      and `ErrKeyExists`, and `errors.As` on `*jetstream.APIError` with its error code), and `errs.IsTransient` and
      `errs.IsFatal` (N extended, #91 comment 6083704900) by class and sentinel, never by text; a test per function,
      written first, with an error whose text holds each string the old match read and whose type does not match;
      natsclient's ledger row gains the `adapt` items, and `pkg/errs`'s row (`docs/admission-ledger.yaml:489-507`, now
      `carry`) becomes `adapt` with the `IsTransient` and `IsFatal` item (design D23, "Classification").
      Done in `08988f1`: `TestIsKVNotFoundErrorClassifiesByType` and `TestIsKVConflictErrorClassifiesByType`
      (`natsclient/kv_classify_test.go`), `TestIsTransientClassifiesByClassAndSentinel` and
      `TestIsFatalClassifiesByClassAndSentinel` (`pkg/errs/classify_by_type_test.go`), each failing first at `5e4c009`
      on every string the pin matched; `task mutate:check` with the pin's text match put back as a fallback: a detection
      for each of the four, 3 of 3 runs. `IsKVConflictError` also matches API code 10164 (a replicated stream's wrong
      last sequence). No production caller changes its verdict for an error the code, nats.go or the server produces.
      Ledger: natsclient gains `natsclient-kv-not-found-by-type` and `natsclient-kv-conflict-by-type`; `pkg/errs`
      becomes `adapt` with `errs-classify-by-class-and-sentinel`. `task verify` green on `c6515e9` (this branch after
      `main` at `b58d5fb` was merged in, `370b384`). Notes: PR #93 comment 6085641996.
- [x] 3.12i (D) #148, ruled O (#91 comment 6080973822): when `handleCanonicalDelete`'s pre-read refuses with a
      graph-state error, the handler proceeds to `DeleteAtRevision` at the caller's expected revision, the KV's
      revision check being the fence; the repair test drives the wire handler. A test written first, failing on the
      current code: a poisoned entity is deleted over the wire at its current revision and a later create births it;
      a stale expected revision is still refused.
      Done in `3d66c91`: `TestPoisonRepairRecoveryWithoutRestart`'s repair drives the wire handlers, and
      `TestCanonicalDeleteRemovesAValueTheReadRefuses` (`processor/graph-ingest/poison_scoping_test.go`), both failing
      first (`:94`, `:473`); the refusal is told apart by type, `errors.As` on `*graph.StateContractError`;
      `deleteEntityAtRevision` no longer counts the fence's `ErrKVRevisionMismatch` in the counter `Health` reads; four
      `task mutate:check` mutants (the old refusal put back, the fence removed, every read error letting the delete
      proceed, the mismatch counted again), each a detection, 3 of 3 runs. `task verify` green on `c6515e9` (this branch
      after `main` at `b58d5fb` was merged in, `370b384`). Notes: PR #93 comment 6086661797.
- [x] 3.12j (D) #145, ruled M (#91 comment 6080973822): a birth writes no inverse `contains` edge into its containers;
      `AddTriple` leaves the inference's `EntityStore`, and `TripleAdder` and graph-ingest's hierarchy dedup lane
      (`addTripleLane`) go; the duplicate-suppression metric's Help text (`processor/graph-ingest/component.go:248`,
      "(append|hierarchy)") drops `hierarchy`; #134 items 5 (the run-time inverse lookup), 7 (`ExistsEntity`) and 11 are
      settled by removal, as the ruling says. A test written first, failing on the current code: each container keeps
      its revision across a member's birth.
      Done in `ccdbcdb`: `TestMemberBirthKeepsContainerRevisions` (`processor/graph-ingest/hierarchy_birth_test.go`), on
      the stream and in-process lanes, failing first at `08988f1` (`:357`); `AddToContainers` creates only absent
      containers and returns the forward edges; `ExistsEntity`, `TripleAdder` (`graph/inference/applier.go`),
      `addTripleLane` and the hierarchy dedup lane are gone; the metric's Help names one lane, `append`; `task
      mutate:check` with the inverse write put back at graph-ingest's seam: a detection, 3 of 3 runs. `task verify`
      green on `c6515e9` (this branch after `main` at `b58d5fb` was merged in, `370b384`). Notes: PR #93 comment
      6086376487.
- [x] 3.12k (D) Ruled P (#91 comment 6080973822), narrowed (#91 comment 6085720598; design D6, D16, D18): remove
      `ComputeIndexStatus`, the revision gauges and the query envelope, which return with their first readers in change
      4. The readiness gate and the `Watcher` stay, with their tests.
      **Removed:**
      - `ComputeIndexStatus`, `IndexStatusInputs` and the three `TestComputeIndexStatus_*` tests;
      - `GaugeOption`, `WithRevisionGauges`, the revision-gauge fields, and the revision branches in `NewGauges`,
        `MetricNames`, `Register` (`:159-166`) and `Set`;
      - in `graph`: `query_contracts.go`, `query_contracts_test.go` and `query_response_types.go` (the data types it
        aliases stay).
      **Kept:** `watcher.go`, `watcher_test.go`, `readiness_gate.go`, `readiness_gate_test.go` and
      `TestBootstrapScope_GateVerdictInvariant` (ADR-088:73).
      **Rewritten:**
      - `TestIndexStatusResponse_StalenessWireRoundTrip` and
        `TestIndexStatusResponse_BootstrapCompleteWireRoundTrip` build their envelope as a literal; the second's
        first assertion, that `ComputeIndexStatus` invents no bootstrap verdict, leaves with `ComputeIndexStatus`
        (#135 (b), fixed when it returns);
      - `registeredGauges` and the five gauge tests run without options, and their name lists drop `indexed_revision`
        and `target_revision`;
      - `graph/README.md`'s envelope paragraph keeps only "Replies on `graph.ingest.query.*` are not enveloped.";
      - every comment that names a removed symbol or test, `graph/index_status.go:76` and the package comment at
        `graph/readiness/watcher.go:23` included; `graph/index_status.go:123-124` names the kept gate and its test and
        stays.
      **Added:** an under-1 ms row in `TestComputeBacklogStatus_Staleness` (`StalenessMs` 1), and a `Publisher.Key`
      test, nil and set (graph-ingest reads it at `readiness.go:341`).
      **Proof:**
      - `git grep -n -w -E
        'ComputeIndexStatus|IndexStatusInputs|WithRevisionGauges|GaugeOption|QueryResponse|NewQueryResponse|MinRevisionField'
        -- graph processor/graph-ingest` lists nothing;
      - `TestPublishStampsPublishedAt`, `TestIndexStatusResponseHasNoLegacyFields`,
        `TestExactEntityReaderReturnsValidatedEntityAndRevision` and the `TestComputeBacklogStatus_*` tests are green;
      - `watcher_test.go` and `readiness_gate_test.go` are unchanged and green, and so are
        `TestBootstrapScope_GateVerdictInvariant`, `TestComputeBacklogStatus_ObservationFailureCannotReportReady` and
        `TestPublisher_ValueIsPlainEnvelopeJSON`, which still call the gate and the `Watcher`;
      - 3.4's three `Register` mutation records, re-run on the rewritten gauge tests, are each a detection;
      - the five tests in `processor/graph-ingest/readiness_integration_test.go` and
        `TestIntegration_ReadinessGaugesAreEmitted` are green and unchanged;
      - `go test -cover ./graph/readiness/` is at or above 80% (91.2% expected, 165 of 181 statements), quoted.
      Gate: `task verify`.
      Done in `64e705d`: the removals as listed; `watcher_test.go`, `readiness_gate.go` and `readiness_gate_test.go`
      unchanged (`watcher.go:23`'s comment edited, as the task says); the proof grep lists nothing; the under-1 ms row
      (a mutant dropping the floor at `index_status.go:143` survives without it and is detected with it, 3 of 3) and
      `TestPublisherKey` (two detections); 3.4's three `Register` records re-run, each a detection; `go test -race
      -cpu=1 -cover ./graph/readiness/` 91.1% (163 of 179; D11's 165 of 181 counted one more statement each in
      `NewGauges` and `MetricNames`, both covered); the six graph-ingest readiness integration tests green at package
      level. `task verify` on `64e705d` ended `verify: ok`. Notes: PR #93 comment 6099387563.
- [ ] 3.13 (D) graph-ingest under the lifecycle suite: an in-package adapter whose `Observe` lists consumers, request
      subscriptions, ingest lanes and the status loop; failing factory = a refused broker; `lifecycletest.Run` green;
      `TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop` carried on the adapter, green, driving the
      guard through its methods (design D22), and citing `lifecycle-suite`/"Failed start whose own cleanup fails"
      (design D7.3); the "Cleanup succeeds after a failed start" scenario is the suite's failed-start check on the
      refused-broker factory.

## 4. Repair and port-refactor evidence (design D8, D15–D21)

- [ ] 4.1 (D) #20: `FaultKV.FailAfter(Update, …)` in graph-ingest's entity bucket → the typed client reports
      commit-unknown; conflict and not-found stay not-committed. Fails first on the pin's classification.
- [ ] 4.2 (D) #19: `ReconcileMutation` gains the expected revision; the two `projection-mutation` scenarios as tests.
- [ ] 4.3 (D) #15: memory stream + `Fixture.Restart` + retained guard bucket: re-ingestion at lower sequences is
      applied and queried back; a same-generation redelivery is a no-op; the no-bucket degrade of design D8 is logged
      and counted. Fails first on the pin's guard key.
- [ ] 4.4 (D) Settlement (Q18): the process-kill test of design D8 (a `prochost.Helper` in graph-ingest's
      `TestHelperProcess`; the guard-record write held open by a `_test.go` wrapper in the first run; the entity write
      and the pending acknowledgement confirmed from the buckets and the consumer's info; the helper confirmed alive,
      then killed; redelivery acknowledged in the second run and the state equal to one application); the long-apply
      test; the durable-record failure test.
- [ ] 4.5 (D) Q13: SemStreams PR #1437's graph-ingest half (head `0ea823a6`) with its two test files on a
      test-registered payload type; the "Payload panics while giving its entity ID" scenario green.
- [ ] 4.6 (D) #33: the hierarchy refusal names `inference.RegisterPayloads` (`component-registration`).
- [ ] 4.7 (D) #29 and #102: the `component` contract test of task 3.11 green on the ported tree, with its package
      count posted.
- [x] 4.8 (D) #100 and #98, the `graph-entity-writes` scenarios through graph-ingest, each written first and failing on
      the pin's code: `TestWriteModesAgreeAcrossLanes`, `TestReplaceKeepsOtherSourcesStatements`,
      `TestStreamLaneGroupsByStampedSource`, `TestReconcileReplacesOnlyItsSource`,
      `TestReconcileRefusesForeignSourceStatement` (a reconcile with no `source` on the wire included, ruling 1),
      `TestStreamLaneRefusesReservedSource` (ruling 5), `TestReplaceOrderedByTimestamp` on the stream lane (the
      stale-set counter rises by one for the same source and not for another), `TestConfidenceAndContextNeverOrder`,
      `TestWriteRefusesStatementWithoutSourceOrTimestamp` (an empty create included),
      `TestGraphableLaneStampsFromEnvelope`, `TestGraphableLaneRefusesWithoutEnvelopeMetadata`,
      `TestDerivedStatementsCarryTriggeringTime` (rulings A and C and the round-4 review's derived-statement finding,
      #91 comment 6037287957; rulings 1 and 5, #91 comment 6037604840; design D15). Later: ruled M, task 3.12j removes
      the in-process append; `TestWriteModesAgreeAcrossLanes` keeps its birth, replace and conditional-replace cases and
      loses the in-process append case (`write_modes_test.go:175`).
- [x] 4.9 (D) #111 item 1: `TestHierarchyFailureFailsTheBirth` (`graph-entity-writes`, "Birth with hierarchy fails
      closed") and `TestCorruptGuardRecordIsRefused` (`graph-ingest-recovery`, "A record that cannot be decoded"),
      each written first and failing on the pin's code (design D21). Done with task 3.12b, in `c4a885f`, `ecf00a7` and
      `36e370d`.

## 5. Ledger, gates and boundaries

- [ ] 5.1 (W) Rows at the full pin SHA, written on top of #92's ledger: 14 package rows, the `graph/inference` row
      `adapt` to the hierarchy slice, naming the files left for change 7 and #97; the `graph` row's destination note
      naming `graph/kvcatalog` and `graph/readiness` as the homes of the moved files (design D16). Five
      `defer-exclude` rows: `pkg/worker` (ruling E), `internal/componentadmission` (ruling C), `model/wire` (change
      7), `graph/llm` (changes 4 and 7, design D1a) and `graph/structural` (change 7, design D1a). Each package row
      has its verdict from design D1, its adapt items (including #69's renames of design D9, with the consumers who
      edit them, and the audit's port-refactors, each naming its issue), its `proving_tests` (graph-ingest's includes
      `TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop`, the `lifecycle-suite` exception's proof) and
      its `known_risks` (graph-ingest: #75's shared gauges, and `ENTITY_STATES` kept at one revision per key (#99);
      `component`: a consumer can call the three registry methods directly, review only; `graph/readiness`: `Set`
      dropped, read by `gateway/graph-gateway` and needed back by #110, design D6; `ComputeIndexStatus` and the
      revision gauges not carried (ruled P, design D16), each named with the change that brings its first reader and
      the last commit holding its adapted code, #135 (b) to be fixed on return; `graph`: the query envelope and its
      three aliases not carried (ruled P, design D18), returning with change 4). Adapt items that name adopters:
      `component`'s `LifecycleManager` drop names semboids `internal/sim/component.go:255-256` and
      `cmd/semboids/main.go:190` and semteams `cmd/semteams/main.go:209` (ruling D, design D17); `graph`'s readiness
      envelope names semsource's reads of `revision` and `last_synced`, a silent change (ruling F, design D16); the
      write rules name raw-wire reconcile callers, who send `source` (design D15). The `graph/structural` row cites
      ruling E and the change to foundation D2 and 03B D10 it makes. `class:port-refactor`
      notes for later changes: rule (change 6: `events.go` emission, the `version` read, the lifecycle manager at
      registration), `service` (change 3: no lifecycle manager copied into component dependencies), graph-query
      (change 4: partial-ID resolution without the suffix verb, the envelope type, its constructor, `min_revision` and
      their producers, the verb table), `fusionnats` (change 5: no `UnwrapQueryResponse`; the readiness `Watcher`
      through `Run(ctx)`, design D5), graph-index (change 4: `ComputeIndexStatus`, `WithRevisionGauges`),
      graph-embedding (change 5: `ComputeIndexStatus`, `WithRevisionGauges`), graph-clustering (change 7: the readiness
      `Watcher` through `Run(ctx)`, design D5), `pkg/fusion` (change 5: its envelope copy without the legacy fields,
      #110). The `internal/lifecyclecleanup` package row is `adapt` to
      `pkg/lifecyclecleanup` (public home, nil rollback refused), naming the harness copy; the file row's
      `known_risks` names the production home and why the copy stays (design D7.4); Q13's row cites PR #1437 head
      `0ea823a6`; #29 and #33 rows name `class:port-refactor`. The `graph/inference` row also records the inverse-edge
      write's removal (ruled M); the `pkg/errs` row is `adapt` (ruled N, task 3.12h). Gate: `task ledger:check`.
- [ ] 5.2 (D) `scripts/cover-check.sh` gains the ten targets of design D11, `graph/inference` and `graph/kvcatalog`
      among them; the tests design D11 names for `internal/graphmutation`, `pkg/projection`, `component` and
      `pkg/lifecycle` are added; each target's figure is posted. `graph/readiness` at or above 80% after task 3.12k
      (91.1%, 163 of 179 statements, measured at `64e705d`; design D11). Ticked only on a green `task cover:check`.
      Hold: task 1.12.
- [x] 5.3 (D) `go.mod`: `golang.org/x/net` at a version that downgrades nothing, and no `go-openai`; `task vuln` and
      `task tidy:check` green. Done in `074c800`: v0.59.0 → v0.60.0 for five HTTP/2 advisories, nothing else moved,
      no `go-openai` in `go.mod` or `go.sum`; both gates green there, locally and in CI run 37926039654.

## 6. Docs and guidance

- [ ] 6.1 (W) The developer and reviewer contract sections and the four skills of design D12, adapted (`new-payload`
      with SS PR #1437's two lines); AGENTS.md rows: "One NATS image pin" names its new review-only part (a Go
      literal whose tag starts with a letter); new rows for `TestNoProcessGlobalRegistration`,
      `TestReservedSubjectsDeclaredOnce`, the `component` contract test (no agentic or graph-family package),
      `TestGraphImportsNoTransport`, a ported component refusing unknown configuration keys (each component's own
      test; that a new component has one is review only), and a ported component composing
      `internal/lifecycleguard` (review only), and a consumer creating a component, sealing the composition or
      reading the admission snapshots only through the component manager, never by calling
      `Registry.CreateComponent`, `SealComposition` or `Snapshots` itself (the three methods' doc comments; review
      only; ruling C), and a ported component whose failed `Start` cleanup fails reporting both errors and keeping
      what is left for a later `Stop` (`lifecycle-suite`, "Failed start whose own cleanup fails"; each component's own
      named test; that a newly ported component has one is review only). Gate: `task docs:check`.
- [ ] 6.2 (W) `docs/repository-map.md` lists the ported packages and the three new ones (`internal/lifecycleguard`,
      `pkg/lifecyclecleanup`, `graph/kvcatalog`), and `docs/tier1-cross-check.md:68` records the change-2
      dispositions of `internal/componentadmission` (dropped, ruling C), `internal/lifecyclecleanup` (public as
      `pkg/lifecyclecleanup`, #77 ruling), `graph/structural` (change 7) and `graph/inference` (its hierarchy slice,
      #97); READMEs carried with their claims checked (tasks 3.x).
- [ ] 6.3 (W) Tracking issues filed and linked here: the adoption sweeps of design D3 (the `natsfixture` helper), D13
      (the guard) and D20 (the responder-owned verb table), and the `component.Discoverable.ConfigSchema` question
      (design D6, K4).
- [ ] 6.4 (W) #77 gets a comment naming the public helper (`pkg/lifecyclecleanup.RollbackFailedStart`), the
      `lifecycle-suite` requirement and graph-ingest's test that carry its ruling (design D7), for SemTeams' proving
      case. The comment names the one change to a pin behavior, a nil rollback now refused with an error (design D7.2),
      and the inverted test `TestRollbackFailedStartNilRollback`, so SemTeams can review it as #77's body asks.
- [ ] 6.5 (W) A tracking issue filed and linked here for the rest of `graph/inference`, which ports with
      graph-clustering in change 7 into a package already under the 80% gate (ruling F re-read, design D11): the
      tests that rest of the package owes (451 statements short at the pin for the whole package, design D11's
      figure), a test per configuration field it reads (37), the drop of its six unread fields with `review.llm`, and
      the review worker's `Shutdown(ctx)` shape with its `synctest` test.
- [ ] 6.6 (W) PR #93's body names `Closes` for #97, #98 and #99 (their rulings close with the implementing change) and
      for #100–#104, and `Addresses` #106, #110 and #111 (this change lands a part of each); each issue gets a comment
      naming the design decision and the tests that carry it.

## 7. Review and archive

- [ ] 7.1 (R) Cross-agent review of record by the other agent on the last implementation commit, the head before task
      7.2's archive commit, named in the PR body's `reviewed-by:` line; the CI run and commit named in the verdict.
      Hold: tasks 2.1–6.6.
- [ ] 7.2 (W) Specs synced from the deltas, the change archived; the archive commit is the last content commit. Hold:
      task 7.1.
