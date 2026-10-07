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
      this pull request. Hold: task 3.1 (the first port commit).
- [x] 2.2 (D) `TestOneImagePin` in Go files matches `nats:` followed by a digit or a variable, or `nats@sha256:` (design
      D10): sensitivity cases, written first, plant `"nats:in"` in a Go file (passes), `"nats:2.10"` and
      `"nats:${TAG}"` in a Go file (fail), and keep "digest literal in Go" failing. Gate: `task test:unit`.
- [x] 2.3 (D) `TestNoProcessGlobalRegistration` and its sensitivity test (`metric-registry`, "No process-global
      registration"). Gate: `task test:unit`.
- [ ] 2.4 (D) `TestReservedSubjectsDeclaredOnce` and its sensitivity test (`graph-transport-boundary`, "Reserved
      request subjects have one declaration"): the two declaring files are `graph`'s verb table and
      `internal/graphmutation/protocol.go` (design D20). It is red until task 3.7 declares the subjects; written first.
      Hold: task 3.7; it lands in 3.7's commit, after being shown red, because a red test fails `task verify` on every
      push before it.
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

- [x] 3.1 (D) `types`, `storage`, `model` (carry). Done: the pin's files at their destinations with the module path
      rewritten, `types/README.md` corrected (name, import path, `config` and `service` arriving in change 3);
      `golang.org/x/net` enters at v0.59.0, the base's version (task 5.3).
- [x] 3.1a (D) `internal/lifecyclecleanup` → `pkg/lifecyclecleanup` (design D7): `RollbackFailedStart` keeps its name
      and its five-second budget; a nil rollback is refused (`TestRollbackFailedStartNilRollback` inverted to expect
      the error, written first and failing on the pin's code); the four scenarios of `lifecycle-suite` "Failed-start
      rollback helper" as tests; a package doc comment stating what a component that calls it must do (#77 inventory
      §10, the five facts); `internal/lifecycleguard` (task 2.7) calls it. Gate: `task test:unit`.
- [ ] 3.2 (D) What is not ported, checked on the tree (design D1, D1a): no `graph/llm`, `graph/structural`,
      `model/wire`, `pkg/worker` or `internal/componentadmission` directory; `graph/inference` holds only the slice's
      files; no `go-openai` in `go.mod`. The `go list ./...` listing is posted. Hold: task 3.8.
- [ ] 3.3 (D) `graph`, the root (design D16, D15, D9): the data model and wire types only; `EntityState` without
      `Version` (`TestStoredEntityHasNoVersion`, written first); the write-mode rules as pure functions, with
      `TestReplaceOrderedByTimestamp`, `TestConfidenceAndContextNeverOrder` and
      `TestSingleValueReadPicksLatestAcrossSources` at rule level, written first (design D15);
      `ReconcilePredicatesRequest` with a required `source`; the reserved sources `graph-ingest-indexing-profile`,
      `graph-ingest-hierarchy` and `semengine-lifecycle` as three constants and `IsReservedSource` (ruling 5, design
      D15), with a rule-level `TestIsReservedSourceNamesAll` listing the three literal names; `IndexStatusResponse`
      without `Phase`, `Revision` and `LastSynced` and with `published_at` (`TestIndexStatusResponseHasNoLegacyFields`,
      written first, design D16); `ExactEntityReader` without the `natsclient` import, a zero timeout passed through;
      `events.go` not ported; dead sentinels, `IncomingEdges` and the other dead `graph` rows removed, test-only exports
      unexported or moved into `_test.go` files; README rewritten; the #69 rename of the configuration bucket constant
      (`semengine_config`); 11 fixture-client sites. `TestGraphImportsNoTransport` and its sensitivity test
      (`graph-transport-boundary`, "The graph root imports no transport"), written first and red on the pin-shaped root,
      land in this task's commit. Gate: `task test:unit`.
- [ ] 3.3a (D) `graph/kvcatalog` (design D16): `kvcatalog.go`, `owned_bucket_retention.go` and `IsKVTombstone` from
      the pin's `graph`, with their tests; no `TOOL_CALL_OUTCOMES` or `ENTITY_SUFFIX_INDEX` row or constant; the
      configuration bucket's description names SemEngine (design D9); `TestEntityStatesKeepsOneRevision`
      (`graph-entity-writes`, "One stored revision per entity"). Gate: `task test:unit`, `task test:integration`.
- [ ] 3.3b (D) The reply envelope and the verb table in `graph` (design D18, D20): `QueryResponse` with
      `indexed_revision` and `producer`, built only with both (`TestQueryResponseCarriesIndexedRevisionAndProducer`,
      written first); the `min_revision` request field; no `UnwrapQueryResponse`; the verb table with `entity`,
      `batch` and `prefix`, responder `graph-ingest`. Gate: `task test:unit`.
- [ ] 3.4 (D) `graph/readiness`: `Watcher` takes `Run(ctx)` (design D5), with `synctest` tests that `Run` returns
      `ctx.Err()` and leaves nothing running; `Set`, `NewSet`, `Dump`, `Verdict` removed, the row's `known_risks`
      naming their return with #110; `Publisher.Publish` sets `published_at` (`TestPublishStampsPublishedAt`, written
      first) and the inputs lose `LastSynced` (design D16); gauges through
      `RegisterOrGet` under `semengine` (design D4); 2 sleeps repaired; receives `readiness_gate.go` and the
      computation in `index_status.go` from `graph`, with their tests (design D16).
- [ ] 3.5 (D) `internal/graphmutation` (`InterfaceType` becomes `semengine.graph.mutation`, design D9) and
      `storage/storeregistry` (the `pkg/fusion` assertion removed, noted for change 5).
- [ ] 3.6 (D) `pkg/dispatch` → `internal/dispatch`: `BoundedDispatcher`, `New`, `Config`, `Deps`, `ErrQueueFull`,
      `KeyedPool.Submit` and `Stats` removed; `ErrStopped` declared in `internal/dispatch`, with no `pkg/worker` import
      (ruling E); `KeyedPool.Shutdown(ctx)` with no fixed default wait
      (`TestKeyedPoolShutdownWithoutDeadlineWaitsForJoin`, `synctest`); metrics through `RegisterOrGet`; unbounded
      cleanups, sleeps and the fixture-client site repaired.
- [ ] 3.7 (D) `graph`'s `ExactEntityReader` takes its subject from the verb table, and graph-ingest's literal sites
      use the table in task 3.12 (#16, design D20). Task 2.4 turns green with 3.12. Hold: task 3.3b.
- [ ] 3.8 (D) `graph/inference`, the hierarchy slice only (#97, design D1a): `hierarchy.go`, `container_entity.go` and
      the `TripleAdder` interface, with a package comment for what the slice holds; `HierarchyConfig` without
      `Org`/`Platform`, the carrier passed to the constructor (design D9); `DefaultHierarchyConfig`, `OnEntityCreated`,
      `ClearCache`, `GetMetrics` and `GetCacheStats` removed, the tests that called `OnEntityCreated` calling
      `GetHierarchyTriples` and the adder; `GetHierarchyTriples` returns every failure, an inverse edge's and the
      sibling pass's included (`TestGetHierarchyTriplesReportsEveryFailure`, written first, design D21); hierarchy
      statements carry graph-ingest's producer as `Source` and the triggering time, which `GetHierarchyTriples` takes
      as an argument and passes to the containers it creates (design D15); 1 sleep repaired.
- [ ] 3.9 (D) `pkg/projection`: carried code without `Version`; the typed client refuses a statement with no
      timestamp before sending, on `Create`, `Append` and `Reconcile` (`TestMutationClientRefusesMissingTimestamp`,
      written first; `projection-mutation`, "The typed client never reads the clock"); `Reconcile` requires
      `Metadata.Source` and sends it as the request's source (`TestMutationClientReconcileRequiresSource`, written
      first; design D15); its repair is tasks 4.1–4.2.
- [ ] 3.10 (D) `pkg/lifecycle`: 7 fixture-client sites; `harness_gate_integration_test.go` on per-package payload
      registration; `Manager.Watch` and `WatchEvents` in the watch shape of design D5; no `Version: 1`
      (design D15); every statement the manager writes carries the constant source `semengine-lifecycle`, which its
      reconciles send as the request's source (`TestTransitionReplacesItsPhaseStatement`, written first: two
      transitions with different `TransitionSource`s leave one phase statement carrying `semengine-lifecycle`; design
      D15); the emitter's error no
      longer discarded (`TestManagerWithoutClientRefusesEmit`, written first, design D17).
- [ ] 3.11 (D) `component`: `ToolRegistry` and `ToolRegistryReader` removed (#29) and `LifecycleManager` removed
      (#102, design D17); `lifecycle_test_suite.go` and its self-test not ported; `ProcessorMetrics`,
      `config_validator.go`, `Registry.Snapshot` and the other dead rows removed; `CreateComponent`,
      `SealComposition` and `Snapshots` without the access-token parameter, their doc comments directing callers to
      the component manager, and the token's 19 test uses dropped (design D14); 3 fixture-client sites, 1 sleep
      repaired. The contract test that `go list -deps ./component` lists no agentic or graph-family path, with its
      sensitivity test (`component-registration`, "The component model reaches no agentic or graph package"),
      written first and red on the pin's `dependencies.go`, lands in this task's commit.
- [ ] 3.12 (D) `processor/graph-ingest`: metrics per design D4 (`TestGraphIngestMetricsRegisterOnItsRegistry`,
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
- [ ] 3.12a (D) graph-ingest's write seam (#100, #98; design D15): one write path for the seven write sites
      (`TestEntityWritesHaveOneSeam` and its sensitivity case, written first); births on every lane stamp the
      profile, and with hierarchy enabled the stream and in-process births add hierarchy statements while a mutation
      create adds none (`TestMutationCreateBirthGetsNoHierarchy`, which holds the pin's behavior, ruling B; its
      `task mutate:check` record, with a mutant that runs the inference on that lane, posted); the tests of task 4.8.
      Hold: task 3.12.
- [ ] 3.12b (D) Fail closed at birth and on the guard record (#111 item 1; design D21): the tests of task 4.9. Hold:
      task 3.12a.
- [ ] 3.13 (D) graph-ingest under the lifecycle suite: an in-package adapter whose `Observe` lists consumers, request
      subscriptions, ingest lanes and the status loop; failing factory = a refused broker; `lifecycletest.Run` green;
      `TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop` carried on the adapter, green, driving the
      guard through its methods (design D22), and citing `lifecycle-suite`/"Failed start whose own cleanup fails"
      (design D7.3); the "Cleanup succeeds after a failed start" scenario is the suite's failed-start check on the
      refused-broker factory. Hold: task 3.12.

## 4. Repair and port-refactor evidence (design D8, D15–D21)

- [ ] 4.1 (D) #20: `FaultKV.FailAfter(Update, …)` in graph-ingest's entity bucket → the typed client reports
      commit-unknown; conflict and not-found stay not-committed. Fails first on the pin's classification. Hold: task
      3.12a.
- [ ] 4.2 (D) #19: `ReconcileMutation` gains the expected revision; the two `projection-mutation` scenarios as tests.
      Hold: task 3.9.
- [ ] 4.3 (D) #15: memory stream + `Fixture.Restart` + retained guard bucket: re-ingestion at lower sequences is
      applied and queried back; a same-generation redelivery is a no-op; the no-bucket degrade of design D8 is logged
      and counted. Fails first on the pin's guard key. Hold: task 3.12.
- [ ] 4.4 (D) Settlement (Q18): the process-kill test of design D8 (a `prochost.Helper` in graph-ingest's
      `TestHelperProcess`; the guard-record write held open by a `_test.go` wrapper in the first run; the entity write
      and the pending acknowledgement confirmed from the buckets and the consumer's info; the helper confirmed alive,
      then killed; redelivery acknowledged in the second run and the state equal to one application); the long-apply
      test; the durable-record failure test. Hold: task 3.12a.
- [ ] 4.5 (D) Q13: SemStreams PR #1437's graph-ingest half (head `0ea823a6`) with its two test files on a
      test-registered payload type; the "Payload panics while giving its entity ID" scenario green. Hold: task 3.12.
- [ ] 4.6 (D) #33: the hierarchy refusal names `inference.RegisterPayloads` (`component-registration`). Hold: task
      3.12.
- [ ] 4.7 (D) #29 and #102: the `component` contract test of task 3.11 green on the ported tree, with its package
      count posted. Hold: task 3.11.
- [ ] 4.8 (D) #100 and #98, the `graph-entity-writes` scenarios through graph-ingest, each written first and failing on
      the pin's code: `TestWriteModesAgreeAcrossLanes`, `TestReplaceKeepsOtherSourcesStatements`,
      `TestStreamLaneGroupsByStampedSource`, `TestReconcileReplacesOnlyItsSource`,
      `TestReconcileRefusesForeignSourceStatement` (a reconcile with no `source` on the wire included, ruling 1),
      `TestStreamLaneRefusesReservedSource` (ruling 5), `TestReplaceOrderedByTimestamp` on the stream lane (the
      stale-set counter rises by one for the same source and not for another), `TestConfidenceAndContextNeverOrder`,
      `TestWriteRefusesStatementWithoutSourceOrTimestamp` (an empty create included),
      `TestGraphableLaneStampsFromEnvelope`, `TestGraphableLaneRefusesWithoutEnvelopeMetadata`,
      `TestDerivedStatementsCarryTriggeringTime` (rulings A and C and the round-4 review's derived-statement finding,
      #91 comment 6037287957; rulings 1 and 5, #91 comment 6037604840; design D15). Hold: task 3.12a.
- [ ] 4.9 (D) #111 item 1: `TestHierarchyFailureFailsTheBirth` (`graph-entity-writes`, "Birth with hierarchy fails
      closed") and `TestCorruptGuardRecordIsRefused` (`graph-ingest-recovery`, "A record that cannot be decoded"),
      each written first and failing on the pin's code (design D21). Hold: task 3.12b.

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
      dropped, read by `gateway/graph-gateway` and needed back by #110, design D6). Adapt items that name adopters:
      `component`'s `LifecycleManager` drop names semboids `internal/sim/component.go:255-256` and
      `cmd/semboids/main.go:190` and semteams `cmd/semteams/main.go:209` (ruling D, design D17); `graph`'s readiness
      envelope names semsource's reads of `revision` and `last_synced`, a silent change (ruling F, design D16); the
      write rules name raw-wire reconcile callers, who send `source` (design D15). The `graph/structural` row cites
      ruling E and the change to foundation D2 and 03B D10 it makes. `class:port-refactor`
      notes for later changes: rule (change 6: `events.go` emission, the `version` read, the lifecycle manager at
      registration), `service` (change 3: no lifecycle manager copied into component dependencies), graph-query
      (change 4: partial-ID resolution without the suffix verb, envelope producers, the verb table), `fusionnats`
      (change 5: no `UnwrapQueryResponse`), `pkg/fusion` (change 5: its envelope copy without the legacy fields,
      #110). The `internal/lifecyclecleanup` package row is `adapt` to
      `pkg/lifecyclecleanup` (public home, nil rollback refused), naming the harness copy; the file row's
      `known_risks` names the production home and why the copy stays (design D7.4); Q13's row cites PR #1437 head
      `0ea823a6`; #29 and #33 rows name `class:port-refactor`. Gate: `task ledger:check`.
- [ ] 5.2 (D) `scripts/cover-check.sh` gains the ten targets of design D11, `graph/inference` and `graph/kvcatalog`
      among them; the tests design D11 names for `internal/graphmutation`, `pkg/projection`, `component` and
      `pkg/lifecycle` are added; each target's figure is posted. Ticked only on a green `task cover:check`. Hold: task
      1.12.
- [ ] 5.3 (D) `go.mod`: `golang.org/x/net` at a version that downgrades nothing, and no `go-openai`; `task vuln` and
      `task tidy:check` green.

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
