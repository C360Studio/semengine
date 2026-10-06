# Tasks: setup-04a-02-ingest-kernel

Each task names the outcome that proves it and the gate that checks it. An unticked task that says "Hold:" is one
`task spec:queue` reports as blocked until the named review or ruling exists. "This pull request" is PR #93. Where
there is code, the failing test is written and shown failing before the code that makes it pass. Owner of each task:
**D** developer (`semengine-developer`), **R** reviewer (`semengine-reviewer`), **W** technical writer
(`semengine-technical-writer`), **A** architect. Evidence is recorded on this pull request as a comment unless a task
says otherwise. Ported tests that are new or repaired carry `// Requirement: <capability>/<Requirement heading>` (#81's
form). No task asserts a fact that exists only after merge.

## 1. Inventory, design and acceptance

- [ ] 1.1 (R) Independent inventory review of `inventory.md` (`base: 1334ce6`), including its three open evidence
      questions. Hold: `INVENTORY PASS` recorded on this pull request.
- [ ] 1.2 (A) `design.md` and the seven deltas under `specs/` restated on the passed inventory; every requirement has a
      scenario; `task spec:check` passes. Hold: task 1.1.
- [ ] 1.3 (R) Independent pre-owner design review; PASS recorded with the reviewed files' checksums. Hold: task 1.2.
- [ ] 1.4 Owner acceptance of the design on #91, and PR #92 merged into `main` and merged into this branch. Hold: owner
      acceptance on #91; PR #92 merged.
- [ ] 1.5 Owner ruling on question B (`graph/llm.EntityParts`); the `harness-boundaries` delta matches it. Hold: owner
      ruling on #91.
- [ ] 1.6 Owner ruling on question C (the registry's access token); the `harness-boundaries` delta matches it. Hold:
      owner ruling on #91.
- [ ] 1.7 Owner ruling on question D (wire and storage names); design D9 and the ledger rows match it. Hold: owner
      ruling on #91.

## 2. Probes and harness

- [ ] 2.1 (D) Re-run inventory P-1 and P-4 on the branch's copy at the first port commit and post the seeds and the
      hit list; any difference from `inventory.md` is a finding on this pull request.
- [ ] 2.2 (D) `TestOneImagePin` reads only Go files that import a container package or `os/exec` (design D10): the
      sensitivity test plants `"nats:2.10"` in an `os/exec` file (fails) and `"nats:in"` in a file importing neither
      (passes), written first. Gate: `task test:unit -- ./internal/harness/contract`.
- [ ] 2.3 (D) `TestNoProcessGlobalRegistration` and its sensitivity test (`metric-registry`, "No process-global
      registration"). Gate: `task test:unit`.
- [ ] 2.4 (D) `TestReservedSubjectsDeclaredOnce` and its sensitivity test (`graph-transport-boundary`). It is red
      until task 3.7 declares the subjects; written first.
- [ ] 2.5 (D) `TestPublicSignatures` and `TestNoSecondAuthorityField` take the exceptions tasks 1.5 and 1.6 ruled, by
      exact name, with a sensitivity case each (`harness-boundaries`).

## 3. Port, per package in design D1's order

Every port task: files land at the row's destination with import paths rewritten; tests travel with the package and
are repaired (sleeps, skips, fixed addresses, unbounded cleanups, the fixture client of design D3, the out-of-set test
imports of design D2); the package's dead surface (design D6) is removed after a `gopls references` check, with the
tests that read only it; READMEs are read claim by claim; `task verify` passes on the push; the package row is written
(task 5.1).

- [ ] 3.1 (D) `internal/lifecyclecleanup`, `types`, `storage`, `model`, `model/wire` (carry; `model/wire` dormant).
- [ ] 3.2 (D) `pkg/worker` → `internal/worker`: `Shutdown(ctx)` (design D5); written first:
      `TestPoolShutdownTwiceAfterTimeoutDoesNotPanic` (fails on the pin with `close of closed channel`, P-6),
      `TestPoolShutdownLeavesNothingRunning` (`synctest`), `TestPoolShutdownNilContextRefused`; `WithMetricsRegistry`
      and the pool metrics removed (D6); 9 sleeps repaired.
- [ ] 3.3 (D) `graph`: 11 fixture-client sites; dead sentinels and `IncomingEdges` removed; question D's names applied.
- [ ] 3.4 (D) `graph/readiness`: `Watcher` and `Set` take `Run(ctx)` (D5), with `synctest` tests that `Run` returns
      `ctx.Err()` and leaves nothing running; gauges through `RegisterOrGet` under `semengine` (D4); 2 sleeps repaired.
- [ ] 3.5 (D) `graph/structural`, `internal/componentadmission`, `internal/graphmutation`, `storage/storeregistry`
      (the `pkg/fusion` assertion removed, noted for change 5).
- [ ] 3.6 (D) `pkg/dispatch` → `internal/dispatch`: `BoundedDispatcher` and `KeyedPool` take `Shutdown(ctx)` with no
      fixed default wait (`TestDispatcherShutdownWithoutDeadlineWaitsForJoin`, `synctest`); metrics through
      `RegisterOrGet`; 14 unbounded cleanups, 5 sleeps, 1 fixture-client site repaired.
- [ ] 3.7 (D) Reserved subjects declared in `graph` (#16); `graph/exact_entity.go:15` and the graph-ingest literal
      sites use it in task 3.11. Task 2.4 turns green with 3.11.
- [ ] 3.8 (D) `graph/inference`: `HierarchyConfig` without `Org`/`Platform`, the carrier passed to the constructor
      (D9); `ReviewWorker.Shutdown(ctx)`; `NATSAnomalyStorage.Watch` in the `Run` shape (D5); `ReviewMetrics` removed.
- [ ] 3.9 (D) `pkg/projection`: carried code; its repair is task 4.1–4.2.
- [ ] 3.10 (D) `pkg/lifecycle`: 7 fixture-client sites; `harness_gate_integration_test.go` on per-package payload
      registration; `Manager.Watch` and `WatchEvents` in the `Run` shape (D5).
- [ ] 3.11 (D) `component`: `ToolRegistry` and `ToolRegistryReader` removed (#29); `lifecycle_test_suite.go` and its
      self-test not ported; `ProcessorMetrics` and the dead helpers removed; the access token per task 1.6; 3
      fixture-client sites, 1 sleep repaired.
- [ ] 3.12 (D) `processor/graph-ingest`: metrics per D4 (`TestGraphIngestMetricsRegisterOnItsRegistry`,
      `TestGraphIngestNilRegistryRegistersNothing`, written first); the authority as one `types.PlatformMeta`;
      unknown configuration keys refused (`TestCreateGraphIngestRefusesUnknownKey`); `TEST_DISPUTE.md` not ported; 21
      fixture-client sites, 10 sleeps, 6 skips (rewritten as integration tests), 11 fixed addresses, 5 unbounded
      cleanups repaired; the eight out-of-set test imports adapted (D2).
- [ ] 3.13 (D) graph-ingest under the lifecycle suite: an in-package adapter whose `Observe` lists consumers, request
      subscriptions, ingest lanes and the status loop; failing factory = a refused broker; `lifecycletest.Run` green
      (`lifecycle-suite`, "graph-ingest under the suite"); `TestStartRollbackFailureLeavesStopToFinish` (D7).

## 4. Repair evidence (design D8)

- [ ] 4.1 (D) #20: `FaultKV.FailAfter(Update, …)` in graph-ingest's entity bucket → the typed client reports
      commit-unknown; conflict and not-found stay not-committed. Fails first on the pin's classification.
- [ ] 4.2 (D) #19: `ReconcileMutation` gains the expected revision; the two `projection-mutation` scenarios as tests.
- [ ] 4.3 (D) #15: memory stream + `Fixture.Restart` + retained guard: re-ingestion at lower sequences is applied and
      queried back; a same-generation redelivery is a no-op; the two degrades of D8 are logged and counted. Fails
      first on the pin's guard key.
- [ ] 4.4 (D) Settlement (Q18): `prochost` kill between apply and acknowledgement on a file stream; after restart the
      input is redelivered and the state equals one application; long-apply in-progress test; durable-record failure
      test.
- [ ] 4.5 (D) Q13: SemStreams PR #1437's graph-ingest half (head `0ea823a6`) with its two test files on a
      test-registered payload type; the "Payload panics while giving its entity ID" scenario green.
- [ ] 4.6 (D) #33: the hierarchy refusal names `inference.RegisterPayloads` (`component-registration`).
- [ ] 4.7 (D) #29: the contract test that `go list -deps ./component` lists no agentic path.

## 5. Ledger, gates and boundaries

- [ ] 5.1 (W) 19 package rows at the full pin SHA, each with its verdict from design D1, its adapt items, its
      `proving_tests`, its `known_risks` (graph-ingest: #75's shared gauges; dormant rows: "dormant until #32"); the
      `internal/lifecyclecleanup` file row reconciled with the package row; Q13's row cites PR #1437 head `0ea823a6`;
      #29 and #33 rows name `class:port-refactor`. Gate: `task ledger:check`.
- [ ] 5.2 (D) `scripts/cover-check.sh`: the ten targets of design D11. Each package's merged figure is posted; a
      package under 80% is a finding here, with the tests added or the reason.
- [ ] 5.3 (D) `go.mod`: `go-openai` and `golang.org/x/net` at versions that downgrade nothing; `task vuln` and `task
      tidy:check` green.

## 6. Docs and guidance

- [ ] 6.1 (W) The developer and reviewer contract sections and the four skills of design D12, adapted; each new
      repository-wide rule adds its `AGENTS.md` row with what enforces it.
- [ ] 6.2 (W) `docs/repository-map.md` lists the 19 packages; READMEs carried with their claims checked (task 3.x).
- [ ] 6.3 (W) #77 gets a comment linking design D7 and owner question A.

## 7. Review and archive

- [ ] 7.1 (R) Cross-agent review of record by the other agent on the final content commit, named in the PR body's
      `reviewed-by:` line; CI run and commit named in the verdict.
- [ ] 7.2 (W) Specs synced from the deltas, the change archived; the archive commit is the last content commit.
