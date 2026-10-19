# Tasks: start-context

Each task names its outcome and the gate that proves it. Tests come first; each new test's failure on the base, or
the mutant that shows it able to fail, is recorded in `notes/<task>.md`. Codex review stops end sections 2, 3 and 4.
Each stop's task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass pre-owner review and are accepted by the
  owner, with owner question 1 answered. Gate: `task spec:check` and `task docs:check`.

## 2. Suite (tests first)

- [ ] 2.1 `refowner_test.go` (D4): failpoint `workDiesWithStartContext` and its table row; the clean double's run
  context survives the Start context; `TestAbortStopThenFinishJoinsWorker` reordered; `finalize` comments. The
  matrix is red until 2.2. Gate: `go test -race -count=1 ./internal/harness/lifecycletest/`, failures named in the
  note.
- [ ] 2.2 `lifecycletest.go` (D1-D3): `CheckStartContextEndStopsNothing`, `CheckStopReturnsItsEndedContext`, the
  settle window; the spec delta matches. Mutants M1-M4. Gate: `task verify`.
- [ ] 2.3 Review stop A (Codex) on 2.1-2.2.

## 3. Adopters

- [ ] 3.1 metric (D5): `TestServerServesAfterStartContextEnds`, then the change, `handler.go:54-55`,
  `README.md:209`. M5. Gate: `task verify`.
- [ ] 3.2 graph-ingest (D6): the adapter reports the status loop while it runs; T-G2 replaces
  `TestComponent_RespectsContext_Cancellation`; T-G3; then the `start` split and comments. M6-M8. Gate: `task verify`
  (integration included).
- [ ] 3.3 The guard's test owner and natsclient pass unchanged (D7); the run recorded in `notes/3.3.md`. Gate:
  `task verify`.
- [ ] 3.4 Review stop B (Codex) on 3.1-3.3.

## 4. Docs and rules

- [ ] 4.1 `component/lifecycle.go` (D8), `pkg/errs/shutdown.go:17`, the `keyed_ingest.go` and `component.go`
  comments (D6). Gate: `task verify`.
- [ ] 4.2 Contracts, `AGENTS.md:71`, `.agents/rules-index.md:17`, `docs/setup-plan.md`, the ledger row, the ADR-108
  note (D9). Gate: `task verify` (`TestAgentSetupParity`, the `AGENTS.md` size and row counts) and `task docs:check`.
- [ ] 4.3 Review stop C (Codex) on 4.1-4.2.

## 5. Archive

- [ ] 5.1 Sync `lifecycle-suite`, archive the change (the last content commit). Gate: `task spec:check` and
  `task docs:check`.
