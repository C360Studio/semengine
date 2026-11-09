# Tasks: setup-04a-05-fusionnats

PR D of the change-5 split. Tests come first. Each code task ends at a clean-tree local `task verify` (#171).
"Shown to fail" is `task mutate:check` or failing-first output in `notes/<task>.md`. No `time.Sleep`, no `Skip`, no
test deadline of its own, no `context.WithValue`, no `t.Parallel` in an integration file. Review stops: after 2.2,
after 3.1, and before the archive.

## 1. Design

- [x] 1.1 Pre-owner design review passes; the owner answers the two questions in `proposal.md` and fills
  `Approved-by:`. Done: PR #577 comment 6749007774. Gate: `task spec:check`, `task docs:check`.

## 2. Package

- [x] 2.1 Client and wire (D1, D2): port `client.go` and `doc.go` without the readiness watch, `Status` and `Close`;
  `New` with its refusals and the verb-table lookup; replies decoded by row; `graph` request types; `predicate`;
  the search counts; `UnmarshalEntityState`; the pin's unit tests on the tree's wire, its exact-entity tests
  (`slice_e_contract_test.go:16-53`) carried, `TestStatus_*` and `slice_e_contract_test.go:55-105` not. Check:
  `TestNewRefusesNilRequester`, `TestNewRefusesNegativeTimeout`, `TestSubjectsComeFromVerbTable`,
  `TestVerbLookupNamesMissingRow`, `TestNewZeroTimeoutUsesDefault`, `TestEnvelopedRowNeedsProducer`,
  `TestNeighborsReadsPredicate`,
  `TestSemanticCountsReachResolution`, `TestReconcileTotal` (generated, seed recorded, reference model), the pin's
  `TestReconcileHydration` and `TestEntities_EmptyShortCircuits`; the transport-boundary contract test passes.
  Done: commit 5bc7fe9e; run, seed and mutation checks in `notes/2.1.md`.
- [x] 2.2 Poisoned seeds (D3, as the owner answers question 1): the per-seed read after a batch refusal coded
  `graph_state_reset_required`; `UnhydratedError`'s doc. Check: `TestPoisonedSeedUnhydrated`,
  `TestAllSeedsPoisonedDefer`, `TestPoisonedDuplicateReadOnce`, `TestPoisonFallbackReturnsOtherError`,
  `TestPoisonFallbackEndedContext`, `TestPoisonFallbackEmptyID`, `TestOwnValidationNotRetried` (both paths), each
  shown to fail by a mutant. **Review stop.** Done: mutation checks and the not-found test in `notes/2.2.md`.
- [x] 2.3 Real wire (D3): graph-ingest, graph-index, graph-query and graph-embedding (bm25) on the package's shared
  `natsfixture` server through `natsfixture.Open` (#195). Check: `TestRealRespondersAnswer` (every `RetrievalClient`
  method, NL with a scope; replaces the pin's fake-handler test), `TestPoisonedSeedOverRealResponders` (bad bytes Put
  after graph-ingest's `Start` returned).
  Done: commit edbbe9ba; waits and mutants in `notes/2.3.md`.

## 3. Proof and close-out

- [x] 3.1 No-embedder proof (D5; the owner chose Q-B, so no other PR comes first):
  `TestSmoke_NoEmbedderAnswers`
  in `internal/smoke`;
  `TestFusionNATSImportsNoEmbedding`
  and its sensitivity case in `internal/harness/contract`. **Review stop.**
  Done: commit 2b19ff8a; second composition, waits and mutants in `notes/3.1.md`.
- [x] 3.2 Readiness leftovers (D4): `Reading.Raw` removed with its tests' assertions; `graph/readiness/watcher.go`
  package doc (the fusionnats sentence goes, graph-clustering stays); ledger `graph/readiness` known_risks.
  Done: d90c885.
- [x] 3.3 Records (D6): ledger row `pkg/fusion/fusionnats`; `scripts/cover-check.sh` gains `pkg/fusion/fusionnats`;
  `pkg/fusion/README.md`, `pkg/fusion/MIGRATION.md` (`New` returns an error; no `Status`, no `Close`; a poisoned seed
  is listed); `pkg/fusion/retrieval.go:37`; port notes on ADR-062 (`:17`), ADR-066, ADR-071, ADR-083, ADR-084; ledger
  `pkg/fusion` row and comment (`:4592`, `:4600`, `:4627`, `:4631`) and the ADR-062 row (`:4694`); the prefix-poison
  issue (#578) and the graph-query issue (#579, shape C), both filed `status:blocked`, linked from this PR without a
  closing keyword; comments on #195 (fusionnats adopted it) and #110 (item 2 done in PR C, `Reading.Raw` removed in
  PR D; stays for item 3).
  Done: 8ca4cae, 57bddd4; ledger, coverage, docs and port notes in this task's commit; coverage and corrections
  in `notes/3.3.md`.
- [x] 3.4 Spec sync and archive as the last content commit; the `composed-stack` Purpose names the no-embedder
  stack beside the four-component one. **Final review.**
  Done: 5a1d7cc plus the review-fix commit that follows it; see `notes/3.4.md`.
