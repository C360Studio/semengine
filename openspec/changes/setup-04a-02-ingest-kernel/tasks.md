# Tasks: setup-04a-02-ingest-kernel

Each task names the outcome that proves it and the gate that checks it. An unticked task that says "Hold:" is one
`task spec:queue` reports as blocked until the named review, ruling or task exists. "This pull request" is PR #93.
Where there is code, the failing test is written and shown failing before the code that makes it pass. Owner of each
task: **D** developer (`semengine-developer`), **R** reviewer (`semengine-reviewer`), **W** technical writer
(`semengine-technical-writer`), **A** architect, **O** the owner. Evidence is recorded on this pull request as a
comment unless a task says otherwise. New or repaired tests carry `// Requirement: <capability>/<Requirement
heading>` (#81's form). No task asserts a fact that exists only after merge.

## 1. Inventory, design and acceptance

- [ ] 1.1 (R) Independent review of `inventory.md` (round 2, and round 3's §8), including its open evidence
      questions. Hold: `INVENTORY PASS` recorded on this pull request.
- [ ] 1.2 (A) `design.md` and the deltas under `specs/` restated on the passed inventory; every requirement has a
      scenario; `task spec:check` passes. Hold: task 1.1.
- [ ] 1.3 (R) Independent pre-owner design review; PASS recorded with the reviewed files' checksums. Hold: tasks 1.2,
      1.9.
- [ ] 1.4 (O) Owner acceptance of the design on #91; PR #92 merged into `main`, `origin/main` merged into this branch,
      and #92's final file list re-read against design "Order with #92". Hold: task 1.3; owner acceptance on #91;
      PR #92 merged.
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
- [ ] 1.9 (A) Apply #77's ruling (#77 comment 6035317931) to design D7 and D13 and the deltas: a public home and name
      for the failed-start rollback helper, whether it keeps accepting a nil callback (#77 inventory, section 13, Q3
      and Q4), the `natsfixture` rollback copy reconciled (foundation D9), and a `lifecycle-suite` delta for the #38
      exception granted to ported components; PR #93's body names `Closes #77` (#77 comment 6035358884).

## 2. Probes and harness

- [ ] 2.1 (D) Re-run inventory P-1, P-4 and P-7 on the branch's copy at the first port commit and post the seeds, the
      hit list and the coverage figures; any difference from `inventory.md` is a finding on this pull request. Hold:
      task 1.4.
- [ ] 2.2 (D) `TestOneImagePin` in Go files matches `nats:` followed by a digit or a variable, or `nats@sha256:` (design
      D10): sensitivity cases, written first, plant `"nats:in"` in a Go file (passes), `"nats:2.10"` and
      `"nats:${TAG}"` in a Go file (fail), and keep "digest literal in Go" failing. Gate: `task test:unit`. Hold:
      task 1.4.
- [ ] 2.3 (D) `TestNoProcessGlobalRegistration` and its sensitivity test (`metric-registry`, "No process-global
      registration"). Gate: `task test:unit`. Hold: task 1.4.
- [ ] 2.4 (D) `TestReservedSubjectsDeclaredOnce` and its sensitivity test (`graph-transport-boundary`). It is red until
      task 3.7 declares the subjects; written first. Hold: task 1.4.
- [ ] 2.5 (D) `TestPublicSignatures` and `TestNoSecondAuthorityField` pass over the ported tree with no new exception
      and no change to either check (rulings B and C; design D9, D14); the pass is posted with each check's package
      count. Gate: `task test:unit`. Hold: task 1.4.
- [ ] 2.6 (D) The `natsfixture` helper of design D3 (`nats-fixture`, "Connected value for a package's tests"), its three
      scenarios as tests, written first. Gate: `task test:integration`. Hold: task 1.4.
- [ ] 2.7 (D) `internal/lifecycleguard` (design D13): `lifecycletest.Run` over a test owner built only from the guard,
      with a failing factory, written first. Gate: `task test:unit`. Hold: task 1.4.

## 3. Port, per package in design D1's order

Every port task: files land at the row's destination with import paths rewritten; tests travel with the package and
are repaired (sleeps, skips, fixed addresses, unbounded cleanups, the fixture client of design D3, the out-of-set test
imports of design D2); the package's dead surface (design D6, appendix of `inventory.md`) is removed after a `gopls
references` check, with the tests that read only it; READMEs are read claim by claim; `task verify` passes on the push.

- [ ] 3.1 (D) `internal/lifecyclecleanup`, `types`, `storage`, `model` (carry). Hold: task 1.4.
- [ ] 3.2 (D) `graph/llm`: `client.go` only (design D1a), its package comment rewritten to describe the interface;
      no other `graph/llm` file, no `model/wire`, no `pkg/worker` and no `internal/componentadmission` in the tree,
      and no `go-openai` in `go.mod`. Hold: task 1.4.
- [ ] 3.3 (D) `graph`: 11 fixture-client sites; dead sentinels, `IncomingEdges` and `Event.Payload` removed; the names
      of design D9 renamed under #69 (`semengine_config`, its description, `semengine.graph.alert.v1`). Hold: task 1.4.
- [ ] 3.4 (D) `graph/readiness`: `Watcher` takes `Run(ctx)` (design D5), with `synctest` tests that `Run` returns
      `ctx.Err()` and leaves nothing running; `Set`, `NewSet`, `Dump`, `Verdict` removed; gauges through
      `RegisterOrGet` under `semengine` (design D4); 2 sleeps repaired. Hold: task 1.4.
- [ ] 3.5 (D) `graph/structural`, `internal/graphmutation` (`InterfaceType` becomes
      `semengine.graph.mutation`, design D9), `storage/storeregistry` (the `pkg/fusion` assertion removed, noted for
      change 5). Hold: task 1.4.
- [ ] 3.6 (D) `pkg/dispatch` → `internal/dispatch`: `BoundedDispatcher`, `New`, `Config`, `Deps`, `ErrQueueFull`,
      `KeyedPool.Submit` and `Stats` removed; `ErrStopped` declared in `internal/dispatch`, with no `pkg/worker` import
      (ruling E); `KeyedPool.Shutdown(ctx)` with no fixed default wait
      (`TestKeyedPoolShutdownWithoutDeadlineWaitsForJoin`, `synctest`); metrics through `RegisterOrGet`; unbounded
      cleanups, sleeps and the fixture-client site repaired. Hold: task 1.4.
- [ ] 3.7 (D) Reserved subjects declared in `graph` (#16); `graph/exact_entity.go:15` and the graph-ingest literal
      sites use it in task 3.12. Task 2.4 turns green with 3.12. Hold: task 1.4.
- [ ] 3.8 (D) `graph/inference`: `HierarchyConfig` without `Org`/`Platform`, the carrier passed to the constructor
      (design D9); `ReviewWorker.Shutdown(ctx)` with its `synctest` test (design D5; stays here under ruling F);
      `ReviewMetrics`, `NATSAnomalyStorage.Watch`/`Cleanup` and the other dead rows removed; the six unread `Config`
      fields, `ReviewConfig.LLM` among them, dropped with their defaults and validation, and a test that
      `RejectUnknownKeys` refuses each (design D6, D1a); `doc.go`'s `cfg.Review.LLM` example removed. Hold: task 1.4.
- [ ] 3.9 (D) `pkg/projection`: carried code; its repair is tasks 4.1–4.2. Hold: task 1.4.
- [ ] 3.10 (D) `pkg/lifecycle`: 7 fixture-client sites; `harness_gate_integration_test.go` on per-package payload
      registration; `Manager.Watch` and `WatchEvents` in the watch shape of design D5. Hold: task 1.4.
- [ ] 3.11 (D) `component`: `ToolRegistry` and `ToolRegistryReader` removed (#29); `lifecycle_test_suite.go` and its
      self-test not ported; `ProcessorMetrics`, `config_validator.go`, `Registry.Snapshot` and the other dead rows
      removed; `CreateComponent`, `SealComposition` and `Snapshots` without the access-token parameter, their doc
      comments directing callers to the component manager, and the token's 19 test uses dropped (design D14); 3
      fixture-client sites, 1 sleep repaired. Hold: task 1.4.
- [ ] 3.12 (D) `processor/graph-ingest`: metrics per design D4 (`TestGraphIngestMetricsRegisterOnItsRegistry`,
      `TestGraphIngestNilRegistryRegistersNothing`, written first); the authority as one `types.PlatformMeta`; unknown
      configuration keys refused by strict decoding (`TestCreateGraphIngestRefusesUnknownKey`); the owner-lifecycle
      state replaced by `internal/lifecycleguard` (design D13), the tests that read its fields reading the guard;
      `MergeEntity` removed and its 27 test call sites calling `mergeEntityOnLane(ctx, entity, false)` (design D6);
      `TEST_DISPUTE.md` not ported; 21 fixture-client sites, 10 sleeps, 6 skips (rewritten as integration tests), 11
      fixed addresses, 5 unbounded cleanups repaired; the eight out-of-set test imports adapted (design D2). Hold:
      task 1.4.
- [ ] 3.13 (D) graph-ingest under the lifecycle suite: an in-package adapter whose `Observe` lists consumers, request
      subscriptions, ingest lanes and the status loop; failing factory = a refused broker; `lifecycletest.Run` green;
      `TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop` carried on the adapter, green (design D7).
      Hold: task 1.4.

## 4. Repair evidence (design D8)

- [ ] 4.1 (D) #20: `FaultKV.FailAfter(Update, …)` in graph-ingest's entity bucket → the typed client reports
      commit-unknown; conflict and not-found stay not-committed. Fails first on the pin's classification. Hold: task
      1.4.
- [ ] 4.2 (D) #19: `ReconcileMutation` gains the expected revision; the two `projection-mutation` scenarios as tests.
      Hold: task 1.4.
- [ ] 4.3 (D) #15: memory stream + `Fixture.Restart` + retained guard bucket: re-ingestion at lower sequences is
      applied and queried back; a same-generation redelivery is a no-op; the two degrades of design D8 are logged and
      counted. Fails first on the pin's guard key. Hold: task 1.4.
- [ ] 4.4 (D) Settlement (Q18): the process-kill test of design D8 (a `prochost.Helper` in graph-ingest's `TestHelperProcess`;
      the guard-record write held open by a `_test.go` wrapper in the first run; the entity write and the pending
      acknowledgement confirmed from the buckets and the consumer's info; the helper confirmed alive, then killed;
      redelivery acknowledged in the second run and the state equal to one application); the long-apply test; the
      durable-record failure test. Hold: task 1.4.
- [ ] 4.5 (D) Q13: SemStreams PR #1437's graph-ingest half (head `0ea823a6`) with its two test files on a
      test-registered payload type; the "Payload panics while giving its entity ID" scenario green. Hold: task 1.4.
- [ ] 4.6 (D) #33: the hierarchy refusal names `inference.RegisterPayloads` (`component-registration`). Hold: task 1.4.
- [ ] 4.7 (D) #29: the contract test that `go list -deps ./component` lists no agentic path. Hold: task 1.4.

## 5. Ledger, gates and boundaries

- [ ] 5.1 (W) Rows at the full pin SHA, written on top of #92's ledger: 16 package rows (15 whole, and `graph/llm`
      `adapt` to `client.go`, naming the files left for change 7) and three `defer-exclude` rows (`pkg/worker`, ruling
      E; `internal/componentadmission`, ruling C; `model/wire`, change 7, design D1a). Each package row has its verdict
      from design D1, its adapt items (including #69's renames of design D9, with the consumers who edit them), its
      `proving_tests` and its `known_risks` (graph-ingest: #75's shared gauges, and the failed-rollback branch of design
      D7 as task 1.9 states it; `graph/inference`: unit coverage 47.9% (674/1406) at the pin after the drop, the gate
      and the tests owed by change 7, the tracking issue of task 6.5; `component`: a consumer can call the three
      registry methods directly, review only). The `internal/lifecyclecleanup` file row reconciled with the package
      row; Q13's row cites PR #1437 head `0ea823a6`; #29 and #33 rows name `class:port-refactor`. Gate: `task
      ledger:check`. Hold: task 1.4.
- [ ] 5.2 (D) `scripts/cover-check.sh` gains the nine targets of design D11 (`graph/inference` is not one, ruling F);
      the tests design D11 names for `internal/graphmutation`, `pkg/projection`, `component` and `pkg/lifecycle` are
      added; each target's figure is posted. Ticked only on a green `task cover:check`. Hold: task 1.4.
- [ ] 5.3 (D) `go.mod`: `golang.org/x/net` at a version that downgrades nothing, and no `go-openai`; `task vuln` and
      `task tidy:check` green. Hold: task 1.4.

## 6. Docs and guidance

- [ ] 6.1 (W) The developer and reviewer contract sections and the four skills of design D12, adapted (`new-payload`
      with SS PR #1437's two lines); AGENTS.md rows: "One NATS image pin" names its new review-only part (a Go
      literal whose tag starts with a letter); new rows for `TestNoProcessGlobalRegistration`,
      `TestReservedSubjectsDeclaredOnce`, the `component` no-agentic test, a ported component refusing unknown
      configuration keys (each component's own test; that a new component has one is review only), and a ported
      component composing `internal/lifecycleguard` (review only), and a consumer creating a component, sealing the
      composition or reading the admission snapshots only through the component manager, never by calling
      `Registry.CreateComponent`, `SealComposition` or `Snapshots` itself (the three methods' doc comments; review
      only; ruling C). Gate: `task docs:check`. Hold: task 1.4.
- [ ] 6.2 (W) `docs/repository-map.md` lists the ported packages and the two new ones; READMEs carried with their
      claims checked (tasks 3.x). Hold: task 1.4.
- [ ] 6.3 (W) Tracking issues filed and linked here: the adoption sweep of design D3 (the `natsfixture` helper), the
      adoption sweep of design D13 (the guard), the `component.Discoverable.ConfigSchema` question (design D6, K4).
      Hold: task 1.4.
- [ ] 6.4 (W) #77 gets a comment naming the public helper, the test and the ledger line that carry its ruling, as
      task 1.9 designs them. Hold: tasks 1.4, 1.9.
- [ ] 6.5 (W) A tracking issue filed and linked here for `graph/inference`'s tests owed by change 7 (ruling F): the
      80% target in `scripts/cover-check.sh`, the 451 statements short at the pin after the drop (design D11), and, for
      each of the 37 configuration fields still read, a test that fails when the field is ignored. Hold: task 1.4.

## 7. Review and archive

- [ ] 7.1 (R) Cross-agent review of record by the other agent on the last implementation commit, the head before task
      7.2's archive commit, named in the PR body's `reviewed-by:` line; the CI run and commit named in the verdict.
      Hold: tasks 2.1–6.5.
- [ ] 7.2 (W) Specs synced from the deltas, the change archived; the archive commit is the last content commit. Hold:
      task 7.1.
