# Tasks: setup-04a-05-fusionnats

PR D of the change-5 split. Tests come first. Each code task ends at a clean-tree local `task verify` (#171).
"Shown to fail" is `task mutate:check` or failing-first output in `notes/<task>.md`. No `time.Sleep`, no `Skip`, no
test deadline of its own, no `context.WithValue`, no `t.Parallel` in an integration file. Review stops: after 2.2,
after 3.1, and before the archive.

## 1. Design

- [ ] 1.1 Pre-owner design review passes; the owner answers the two questions in `proposal.md` and fills
  `Approved-by:`. Hold: OPEN-Q proposal awaits owner approval. Gate: `task spec:check`, `task docs:check`.

## 2. Package

- [ ] 2.1 Client and wire (D1, D2): port `client.go` and `doc.go` without the readiness watch, `Status` and `Close`;
  `New` with its refusals and the verb-table lookup; replies decoded by row; `graph` request types; `predicate`;
  the search counts; `UnmarshalEntityState`; the pin's unit tests on the tree's wire, `TestStatus_*` and
  `slice_e_contract_test.go` not carried. Check: `TestNewRefusesNilRequester`, `TestNewRefusesNegativeTimeout`,
  `TestSubjectsComeFromVerbTable`, `TestReplyShapeDecidedByVerb`, `TestNeighborsReadsPredicate`,
  `TestSemanticCountsReachResolution`, `TestReconcileTotal` (generated, seed recorded, reference model), the pin's
  `TestReconcileHydration` and `TestEntities_EmptyShortCircuits`; the transport-boundary contract test passes.
- [ ] 2.2 Poisoned seeds (D3, as the owner answers question 1): the per-seed read after a batch refusal coded
  `graph_state_reset_required`; `UnhydratedError`'s doc. Check: `TestPoisonedSeedUnhydrated`,
  `TestAllSeedsPoisonedDefer`, `TestPoisonedDuplicateReadOnce`, `TestPoisonFallbackReturnsOtherError`,
  `TestPoisonFallbackEndedContext`, `TestOwnValidationNotRetried`, each shown to fail by a mutant. **Review stop.**
- [ ] 2.3 Real wire (D3): `TestPoisonedSeedOverRealResponders` (integration; graph-ingest and graph-query on the
  package's shared `natsfixture` server through `natsfixture.Open`, #195), and an integration test driving every
  `RetrievalClient` method against the real responders in place of the pin's fake-handler test.

## 3. Proof and close-out

- [ ] 3.1 No-embedder proof (D5): `TestSmoke_NoEmbedderAnswers` in `internal/smoke`; `TestFusionNATSImportsNoEmbedding`
  and its sensitivity case in `internal/harness/contract`. **Review stop.**
- [ ] 3.2 Readiness leftovers (D4): `Reading.Raw` removed with its tests' assertions; `graph/readiness/watcher.go`
  package doc; ledger `graph/readiness` known_risks.
- [ ] 3.3 Records (D6): ledger row `pkg/fusion/fusionnats`; `scripts/cover-check.sh` gains `pkg/fusion/fusionnats`;
  `pkg/fusion/README.md`, `pkg/fusion/MIGRATION.md` (`New` returns an error; no `Status`, no `Close`; a poisoned seed
  is listed); `pkg/fusion/retrieval.go:37`; port notes on ADR-066, ADR-071, ADR-083, ADR-084; the prefix-poison issue
  filed, `status:blocked` on this PR, linked without a closing keyword; a comment on #195 that fusionnats adopted it.
- [ ] 3.4 Spec sync and archive as the last content commit. **Final review.**
