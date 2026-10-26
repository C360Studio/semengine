# Tasks: setup-04a-04-graph-roots-processor-shell

Each task names its outcome and the gate that proves it. Tests come first. Each stop's task ends only when
`task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass pre-owner review and are accepted by the
  owner, with owner question 1 answered. Gate: `task spec:check` and `task docs:check`.

## 2. Tests

- [ ] 2.1 `component/lifecycle_type_test.go` and `component/shell_example_test.go` (design D10):
  `TestLifecycleOwnerPassesTheLifecycleSuite` on the real clock through `lifecycletest.Run`; inside `synctest.Test`,
  `TestLifecycleReadsInEveryPhase`, `TestLifecycleReadsDoNotWaitForStartOrStop`,
  `TestLifecycleStartBeforeInitializeIsRefusedAndLeavesItUnused`, `TestLifecycleFailedStartNeverRuns` and
  `TestLifecycleSuiteCatchesWorkUnderTheStartContext` (the check called directly); `ExampleLifecycle`. Written before
  `component/lifecycle_type.go`. Gate: in 3.1.
- [ ] 2.2 `TestEntityCacheUseDuringStop` in `processor/graph-ingest` (design D7), shown failing at the base under
  `-race` with the output in `notes/2.2.md`. Gate: in 4.2.
- [ ] 2.3 `TestCacheUsableAfterClose` in `internal/cache`'s suite over every implementation (design D7). Gate:
  `go test -race -count=1 ./internal/cache`.

## 3. The Lifecycle type

- [ ] 3.1 `component/lifecycle_type.go` and the two `FlowMetrics` fields as design D1-D5, and the `LifecycleComponent`
  doc comment (D13). Gate: `go test -race -count=1 ./component`, `go test -count=5 -cpu 1 -shuffle=on ./component`,
  `TestComponentReachesNoAgenticOrGraphPackage`, then `task test:integration` (whose profile `task cover:check` reads)
  followed by `task cover:check`.

## 4. graph-ingest

- [ ] 4.1 graph-ingest composes the shell as design D6; test reads and setup lines changed as D6 lists, every
  assertion's oracle unchanged, `TestComponent_Initialize_InvalidConfig` as D6 states, and the
  `TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop` comment naming M7. Gate: `go test -race -count=1
  ./processor/graph-ingest`, `task test:integration -- ./processor/graph-ingest`, `task test:repeat`.
- [ ] 4.2 The #306 fix as design D7; 2.2 passes. The `Cache.Close` doc sentence of D7. Gate: as 4.1, and 2.3's.
- [ ] 4.3 #300 sections G and D as the owner answered question 1: the move-only commit of design D8's file map, or a
  comment on #300 and #109 naming the next pull request as their home. Gate: as 4.1 when code moves; the comment read
  back otherwise.

## 5. Ledger, rules and docs

- [ ] 5.1 The ledger edits of design D11; the docs, rule text and records of D13 (README, the four rule lines,
  repository map, ADR-108 note). Gate: `task ledger:check`, `task docs:check`, and the `AGENTS.md` and
  `rules-index.md` guard in `task verify`.

## 6. Evidence and review

- [ ] 6.1 `task mutate:check` for M1-M8 (design D10) and coverage of `component`, in `notes/6.1.md`; `task verify`
  passes on a clean tree at the last commit before the archive commit.
- [ ] 6.2 Cross-agent review stop; `reviewed-by:` in the PR body.
- [ ] 6.3 Comments on #196 (graph-ingest's row, through the shell), #109 (follower stays open) and #110 (the two
  spellings of `degraded`, design D4), each saying "implemented in PR #409 at `<SHA>`, pending merge" and read back.
  The update after merge goes in the PR body or a note on those issues, not in this checklist.

## 7. Close

- [ ] 7.1 Archive (`openspec archive setup-04a-04-graph-roots-processor-shell`) as the last content commit, syncing
  `component-lifecycle` and `graph-ingest-recovery` into `openspec/specs/` with the Purpose design D12 gives. Gate:
  `task spec:check` and `task docs:check` on the archive commit.
