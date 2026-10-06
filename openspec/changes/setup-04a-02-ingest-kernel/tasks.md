# Tasks: setup-04a-02-ingest-kernel

Each task names the outcome that proves it and the gate that checks it. An unticked task that says "Hold:" is one
`task spec:queue` reports as blocked until the named review, ruling or task exists. "This pull request" is PR #93.
Where there is code, the failing test is written and shown failing before the code that makes it pass. Owner of each
task: **D** developer (`semengine-developer`), **R** reviewer (`semengine-reviewer`), **W** technical writer
(`semengine-technical-writer`), **A** architect, **O** the owner. Evidence is recorded on this pull request as a
comment unless a task says otherwise. New or repaired tests carry `// Requirement: <capability>/<Requirement
heading>` (#81's form). No task asserts a fact that exists only after merge.

## 1. Inventory, design and acceptance

- [ ] 1.1 (R) Independent review of the round-2 `inventory.md`, including its open evidence questions. Hold:
      `INVENTORY PASS` recorded on this pull request.
- [ ] 1.2 (A) `design.md` and the deltas under `specs/` restated on the passed inventory; every requirement has a
      scenario; `task spec:check` passes. Hold: task 1.1.
- [ ] 1.3 (R) Independent pre-owner design review; PASS recorded with the reviewed files' checksums. Hold: task 1.2.
- [ ] 1.4 (O) Owner acceptance of the design on #91; PR #92 merged into `main`, `origin/main` merged into this branch,
      and #92's final file list re-read against design "Order with #92". Hold: task 1.3; owner acceptance on #91;
      PR #92 merged.
- [ ] 1.5 (O) Owner ruling on question B (`graph/llm.EntityParts`); the `harness-boundaries` delta matches it. Hold:
      owner ruling on #91.
- [ ] 1.6 (O) Owner ruling on question C (the registry's access token, three methods); the `harness-boundaries` delta
      matches it. Hold: owner ruling on #91.
- [ ] 1.7 (O) Owner ruling on question E (`pkg/worker` left out); design D1, D5 and the ledger rows match it. Hold:
      owner ruling on #91.
- [ ] 1.8 (O) Owner ruling on question F (`graph/inference`'s coverage target in change 7); design D11 and
      `scripts/cover-check.sh`'s target list match it. Hold: owner ruling on #91.

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
- [ ] 2.5 (D) `TestPublicSignatures` and `TestNoSecondAuthorityField` take the exceptions tasks 1.5 and 1.6 ruled, by
      exact name, with a sensitivity case each (`harness-boundaries`). Hold: tasks 1.4, 1.5, 1.6.
- [ ] 2.6 (D) The `natsfixture` helper of design D3 (`nats-fixture`, "Connected value for a package's tests"), its three
      scenarios as tests, written first. Gate: `task test:integration`. Hold: task 1.4.
- [ ] 2.7 (D) `internal/lifecycleguard` (design D13): `lifecycletest.Run` over a test owner built only from the guard,
      with a failing factory, written first. Gate: `task test:unit`. Hold: task 1.4.

## 3. Port, per package in design D1's order

Every port task: files land at the row's destination with import paths rewritten; tests travel with the package and
are repaired (sleeps, skips, fixed addresses, unbounded cleanups, the fixture client of design D3, the out-of-set test
imports of design D2); the package's dead surface (design D6, appendix of `inventory.md`) is removed after a `gopls
references` check, with the tests that read only it; READMEs are read claim by claim; `task verify` passes on the push.

- [ ] 3.1 (D) `internal/lifecyclecleanup`, `types`, `storage`, `model`, `model/wire` (carry; `model/wire` dormant).
      Hold: task 1.4.
- [ ] 3.2 (D) `pkg/worker` as task 1.7 ruled: left out, or ported to `internal/worker` with `Shutdown(ctx)` (design D5;
      written first: `TestPoolShutdownTwiceAfterTimeoutDoesNotPanic`, failing on the pin with `close of closed
      channel`, P-6), `WithMetricsRegistry` and `SubmitBlocking` removed, 9 sleeps repaired. Hold: tasks 1.4, 1.7.
- [ ] 3.3 (D) `graph`: 11 fixture-client sites; dead sentinels, `IncomingEdges` and `Event.Payload` removed; the names
      of design D9 renamed under #69 (`semengine_config`, its description, `semengine.graph.alert.v1`). Hold: task 1.4.
- [ ] 3.4 (D) `graph/readiness`: `Watcher` takes `Run(ctx)` (design D5), with `synctest` tests that `Run` returns
      `ctx.Err()` and leaves nothing running; `Set`, `NewSet`, `Dump`, `Verdict` removed; gauges through
      `RegisterOrGet` under `semengine` (design D4); 2 sleeps repaired. Hold: task 1.4.
- [ ] 3.5 (D) `graph/structural`, `internal/componentadmission`, `internal/graphmutation` (`InterfaceType` becomes
      `semengine.graph.mutation`, design D9), `storage/storeregistry` (the `pkg/fusion` assertion removed, noted for
      change 5). Hold: task 1.4.
- [ ] 3.6 (D) `pkg/dispatch` → `internal/dispatch`: `BoundedDispatcher`, `New`, `Config`, `Deps`, `ErrQueueFull`,
      `KeyedPool.Submit` and `Stats` removed; `ErrStopped` as task 1.7 ruled; `KeyedPool.Shutdown(ctx)` with no fixed
      default wait (`TestKeyedPoolShutdownWithoutDeadlineWaitsForJoin`, `synctest`); metrics through `RegisterOrGet`;
      unbounded cleanups, sleeps and the fixture-client site repaired. Hold: tasks 1.4, 1.7.
- [ ] 3.7 (D) Reserved subjects declared in `graph` (#16); `graph/exact_entity.go:15` and the graph-ingest literal
      sites use it in task 3.12. Task 2.4 turns green with 3.12. Hold: task 1.4.
- [ ] 3.8 (D) `graph/inference`: `HierarchyConfig` without `Org`/`Platform`, the carrier passed to the constructor
      (design D9); `ReviewWorker.Shutdown(ctx)` (design D5); `ReviewMetrics`, `NATSAnomalyStorage.Watch`/`Cleanup` and
      the other dead rows removed; the five unread `Config` fields dropped with their defaults and validation, and a
      test that `RejectUnknownKeys` refuses each (design D6). Hold: task 1.4.
- [ ] 3.9 (D) `pkg/projection`: carried code; its repair is tasks 4.1–4.2. Hold: task 1.4.
- [ ] 3.10 (D) `pkg/lifecycle`: 7 fixture-client sites; `harness_gate_integration_test.go` on per-package payload
      registration; `Manager.Watch` and `WatchEvents` in the watch shape of design D5. Hold: task 1.4.
- [ ] 3.11 (D) `component`: `ToolRegistry` and `ToolRegistryReader` removed (#29); `lifecycle_test_suite.go` and its
      self-test not ported; `ProcessorMetrics`, `config_validator.go`, `Registry.Snapshot` and the other dead rows
      removed; the access token per task 1.6; 3 fixture-client sites, 1 sleep repaired. Hold: tasks 1.4, 1.6.
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

- [ ] 5.1 (W) Package rows at the full pin SHA (19, or 18 under task 1.7), written on top of #92's ledger, each with
      its verdict from design D1, its adapt items (including #69's renames of design D9, with the consumers who edit
      them), its `proving_tests`, its `known_risks` (graph-ingest: #75's shared gauges, and the failed-rollback branch
      of design D7 pending #77; dormant rows: "dormant until #32"; `graph/inference`: its coverage as task 1.8
      ruled); the `internal/lifecyclecleanup` file row reconciled with the package row; Q13's row cites PR #1437 head
      `0ea823a6`; #29 and #33 rows name `class:port-refactor`. Gate: `task ledger:check`. Hold: tasks 1.4, 1.7, 1.8.
- [ ] 5.2 (D) `scripts/cover-check.sh` gains the targets of design D11 as task 1.8 ruled; the tests design D11 names
      for `internal/graphmutation`, `pkg/projection`, `component` and `pkg/lifecycle` are added; each target's figure
      is posted. Ticked only on a green `task cover:check`. Hold: tasks 1.4, 1.8.
- [ ] 5.3 (D) `go.mod`: `go-openai` and `golang.org/x/net` at versions that downgrade nothing; `task vuln` and `task
      tidy:check` green. Hold: task 1.4.

## 6. Docs and guidance

- [ ] 6.1 (W) The developer and reviewer contract sections and the four skills of design D12, adapted (`new-payload`
      with SS PR #1437's two lines); AGENTS.md rows: "One NATS image pin" names its new review-only part (a Go
      literal whose tag starts with a letter); new rows for `TestNoProcessGlobalRegistration`,
      `TestReservedSubjectsDeclaredOnce`, the `component` no-agentic test, a ported component refusing unknown
      configuration keys (each component's own test; that a new component has one is review only), and a ported
      component composing `internal/lifecycleguard` (review only). Gate: `task docs:check`. Hold: task 1.4.
- [ ] 6.2 (W) `docs/repository-map.md` lists the ported packages and the two new ones; READMEs carried with their
      claims checked (tasks 3.x). Hold: task 1.4.
- [ ] 6.3 (W) Tracking issues filed and linked here: the adoption sweep of design D3 (the `natsfixture` helper), the
      adoption sweep of design D13 (the guard), the `component.Discoverable.ConfigSchema` question (design D6, K4),
      and, if task 1.8 moved it, `graph/inference`'s tests for change 7. Hold: task 1.4.
- [ ] 6.4 (W) #77 gets a comment naming the test and the ledger line that pin the failed-rollback branch until its
      ruling (design D7). Hold: task 1.4.

## 7. Review and archive

- [ ] 7.1 (R) Cross-agent review of record by the other agent on the last implementation commit, the head before task
      7.2's archive commit, named in the PR body's `reviewed-by:` line; the CI run and commit named in the verdict.
      Hold: tasks 2.1–6.4.
- [ ] 7.2 (W) Specs synced from the deltas, the change archived; the archive commit is the last content commit. Hold:
      task 7.1.
