# Tasks: fix-216-consumer-rebind

graph-ingest records a lost input binding as a stuck state until the process restarts (#216). Owner ruling: #216
comment 6443457897, "Yes, restart is the recovery (small fix)".

- Tests first. Each new test runs on the base before the production change; its failure (or its pass, for a test
  that pins kept behaviour) is recorded in `notes/<task>.md`, linked from a PR comment.
- Developer notes on choices the design does not spell out go in `notes/<task>.md`.
- Developers do not commit; the orchestrator commits each task.
- Each code task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory (`inventory.md`) passes independent review (INVENTORY PASS; committed at `6d500e0`).
- [ ] 1.2 Codex design review of `design.md` and the `graph-ingest-recovery` delta.
  Hold: Codex design review on PR #431 (the owner starts Codex). Gate: `task spec:check` and `task docs:check`.
- [ ] 1.3 Owner acceptance of the reviewed design (the ruling chose the option, not this design).
  Hold: owner acceptance on #216 or PR #431.

## 2. Tests

- [ ] 2.1 T1 to T4 (design, Tests) in `processor/graph-ingest`: T1 and T4 in their own files with
  `// Own server: restarts the server mid-test.` above `natsfixture.New`, each asserting `f.URL()` is unchanged
  across `Restart`; T2 and T3 beside the health tests in `health_integration_test.go`, T3 replacing
  `TestIntegration_HealthClearsWhenObservationSucceeds_ConsumptionNotChecked`. Gate: on the base, T2 and T3 fail at
  their last step (consumer created again, still unhealthy) and T1 and T4 pass; the output is in `notes/2.1.md`.
  Committed with 3.1.

## 3. Code

- [ ] 3.1 The lost input binding (design D1) in `processor/graph-ingest/readiness.go` and `consume.go`: each bound
  consumer's readiness record carries its loop's `Closed()` channel; the tick records a lost binding when the loop has
  ended before `Stop` began (a fact published safely and kept off the consumer binding), never clears it, and feeds
  it to `Health()` and the envelope. No natsclient change.
  Gate: `go test -race ./processor/graph-ingest/`, `go test -tags integration ./processor/graph-ingest/`,
  `task cover:check` and `task verify`.
- [ ] 3.2 Mutation rows M1, M3, M4, M5 and M6 (design, Tests) with `task mutate:check`, each reported as detected
  (naming the test) or as a survivor, in `notes/3.2.md`, which also states the two unproven scenarios (design,
  Tests); the PR body lists them as uncovered.

## 4. Docs

- [ ] 4.1 Text (design D2): `docs/contract.md:79`, `processor/graph-ingest/README.md:188`, and the graph-ingest row of
  `docs/admission-ledger.yaml` (`known_risks`, `proving_tests`). Gate: `task docs:check` and `task ledger:check`.

## 5. Code review

- [ ] 5.1 Codex code review of record at a named commit with a local `task verify`; the PR body carries
  `implemented-by: claude (opus)` and `reviewed-by: codex`. Hold: Codex code review on PR #431 (the owner starts
  Codex). Gate: Codex's comment.

## 6. Archive

- [ ] 6.1 Sync the `graph-ingest-recovery` delta into `openspec/specs/graph-ingest-recovery/spec.md` and archive, as
  the last content commit. Gate: `task spec:check` and `task docs:check`.
