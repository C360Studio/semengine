# Tasks: setup-04a-05-first-consumer-graph-embedding

PR A of `split.md`. Tests come first. Each code task ends at a clean-tree local `task verify` (#171); integration
tests run under `task test:integration -- ./processor/graph-embedding/`. "Shown to fail" is `task mutate:check` or
failing-first output in `notes/<task>.md`. No `time.Sleep`, no `Skip`, no test deadline of its own, no
`context.WithValue`.

## 1. Design

- [x] 1.1 Pre-owner design review passes; the owner fills `Approved-by:` (rulings already given: PR #550 comment
  6716972354). Gate: `task spec:check`, `task docs:check`.

## 2. Component

- [x] 2.1 Config and factory (D4), with `Register` and the ledger row. Check: `TestConfigRefusesUnknownKey`,
  `TestConfigRefusesHTTPEmbedder`, one test per kept key.
- [x] 2.2 Shell, follower plug-in and hop 1 (D1, D5). Check: `TestReadinessWaitsForHopTwo`,
  `TestCoalescedRevisionsComplete`, `TestStorageRefLaneEmbedsIdentityThenBody`, `TestHeldLowRevisionWithProgress`,
  `TestNoCompletionsReadsDegraded` (follower), `TestGraphIndexHeldRevisionNotDegraded` (held low revision, later
  completions: not ready, not degraded), lifecycle suite, `-race`.
- [x] 2.3 Worker under the run context and its lost watch (D2). Check: `TestHealthNamesLostWorkerWatch`,
  `TestStopJoinsWorker`.
- [ ] 2.4 Failed map, follower `Failed` with reasons, repair lane (D3). Check: `TestFailedMapHistoryMatchesModel`,
  `TestEnvelopeCarriesFailedReasons` (`-race`), `TestGraphIndexEnvelopeUnchanged`,
  `TestStrandedWithoutRevisionClearsOnlyOnRepair`, `TestRepairConvergesStranded` (including read success with write
  failure); remove `FirstFailureAt`
  (`graph/index_status.go:85`, `index_status_test.go:59`).
- [ ] 2.5 Health and tier 0 (D6, D7). Check: `TestHealthIgnoresErrorCount`, `TestNoStatusVerb`,
  `TestGraphEmbeddingImportsNoProviderModule`.

## 3. Close-out

- [ ] 3.1 `processor/graph-embedding/MIGRATION.md` (batch_size, http, status verb, first_failure_at); `README.md`;
  ledger row;
  ADR-083 port note for graph-embedding (advances #482); spec sync and archive as the last content commit.
