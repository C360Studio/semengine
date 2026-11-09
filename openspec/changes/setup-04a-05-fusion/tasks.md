# Tasks: setup-04a-05-fusion

PR C of the change-5 split. Tests come first. Each code task ends at a clean-tree local `task verify` (#171).
"Shown to fail" is `task mutate:check` or failing-first output in `notes/<task>.md`. No `time.Sleep`, no `Skip`, no
test deadline of its own, no `context.WithValue`. Review stops: after 2.2, after 2.4, and before the archive.

## 1. Design

- [ ] 1.1 Pre-owner design review passes; the owner answers the four questions in `proposal.md` and fills
  `Approved-by:`. Gate: `task spec:check`, `task docs:check`.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Package

- [ ] 2.1 Contract and readiness (D1, D2, D8): port `contract.go`, `lens.go`, `retrieval.go`, `rank_signals.go`,
  `engine_lens.go`, `engine_facets.go`, `engine_graph.go`, `doc.go` without the status read, `IndexStatus`,
  `IndexState`, `ErrReadinessUnknown`, `Response.Index`, `ViewRevision` (under G2) and the `Fuse` path;
  `ContractVersion` "2"; the pin's lens, facet, graph, rank, score and unhydrated tests ported. Check:
  `TestDeferredOnIndexNotReady`, `TestFatalReplyReturnsError`, `TestAllSeedsUnhydratedDefersWithoutMiss`,
  `TestContractVersionIsTwo`, `TestGraphProjectionRoundTrip`, `TestRankIsStablePermutation` (seed recorded).
- [ ] 2.2 Labels and counts (D3 under R1, D4): `Resolution`, `Response.UnreadableRecords` and `FailedRecords`,
  `Node.RelationsTruncated`, `Response.PathsTruncated`, the path stop, impact and graph truncation. Check:
  `TestNLAnswerCarriesSearchCounts`, `TestSymbolAnswerOmitsCounts`, `TestRelationsCapLabelled`,
  `TestRelationsFaultLabelled`, `TestRelationsUnreadTargetLabelled`, `TestPathsCapLabelled`,
  `TestPathStopsAtUnreadNode`, `TestImpactUnreadFileTruncates`, `TestGraphCounterpartFaultTruncates`; a mutant per
  label. **Review stop.**
- [ ] 2.3 Construction, metrics, hydration (D5, D7): `NewEngine` with options and an error, the counter through
  `metric.RegisterOrGet`, nil-context refusals, `hydrate.go` and its tests, the `storeregistry.Registry` assertion.
  Check: `TestNewEngineRefusesNilClient`, `TestCounterOnlyOnInjectedRegistry`, `TestEnginesShareOneSeries`,
  `TestRegistrationRefusalReturned`, `TestFuseRefusesNilContext`, `TestResolveBodyRefusesNilContext`, the pin's
  `hydrate_test.go` and `body_reason_test.go` tests, `-race`.
- [ ] 2.4 `fusionvocab` (D6, under V1): `New(parents)`. Check: `TestClassSpecificityCountsParents`,
  `TestClassSpecificityNilParentsIsZero`, `TestPredicateSalience`; `go list -deps ./pkg/fusion/...` names no
  semvocab and no `vocabulary/cco`. **Review stop.**

## 3. Close-out

- [ ] 3.1 `pkg/fusion/README.md` and `pkg/fusion/MIGRATION.md` (`NewEngine` options and error, `RetrievalClient`
  without `Status` and with `Resolution`, `index` and `view_revision` gone, `contract_version` "2", the new labels and
  counts, `fusionvocab.New(cco.Parents)` from semvocab); `scripts/cover-check.sh` gains `pkg/fusion unit`; ADR-062
  ported with its port note and ledger row; ledger rows `pkg/fusion`, `pkg/fusion/fusionvocab`; ledger edits:
  `vocabulary/cco` known_risks (`docs/admission-ledger.yaml:2723-2725`), `storage/storeregistry` (`:2271-2272`),
  `graph` (`:2133-2137`), `graph/readiness` (`:2190`, `Reading.Raw` is PR D's question), `graph/embedding`
  consumer_purpose (`:3767`, pin `pkg/fusion` does not import it); stale comments `graph/index_status.go:143-144`,
  `graph/readiness/watcher.go:23` and `:251`, `storage/storeregistry/storeregistry.go:22-25` and `:94-95`; port notes
  on ADR-066 and ADR-084 where they name fusion's gate; comment on #110 (item 2 done, stays for item 3); spec sync and
  archive as the last content commit. **Final review.**
