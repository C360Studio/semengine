# Tasks: setup-04a-05-fusion

PR C of the change-5 split. Tests come first. Each code task ends at a clean-tree local `task verify` (#171).
"Shown to fail" is `task mutate:check` or failing-first output in `notes/<task>.md`. No `time.Sleep`, no `Skip`, no
test deadline of its own, no `context.WithValue`. Review stops: after 2.2, after 2.4, and before the archive.

## 1. Design

- [x] 1.1 Pre-owner design review passes; the owner answers the three questions in `proposal.md` and fills
  `Approved-by:`. Gate: `task spec:check`, `task docs:check`.

## 2. Package

- [x] 2.1 Contract and readiness (D1, D2, D8): port `contract.go`, `lens.go`, `retrieval.go`, `rank_signals.go`,
  `engine_lens.go`, `engine_facets.go`, `engine_graph.go`, `doc.go` without the status read, `IndexStatus`,
  `IndexState`, `ErrReadinessUnknown`, `Response.Index`, `ViewRevision` and the `Fuse` path; D1's error table;
  `ContractVersion` "2"; `UnhydratedReason`'s doc stops claiming it mirrors `graph.MissingReason` (F19); the pin's lens,
  facet, graph, rank, score and unhydrated tests ported. Check:
  `TestDeferredOnIndexNotReady`, `TestFatalReplyReturnsError`, `TestFatalWalkReadSkipped`,
  `TestEndedContextReturnsError`, `TestAllSeedsUnhydratedDefersWithoutMiss`, `TestContractVersionIsTwo`,
  `TestGraphProjectionRoundTrip`, `TestRankIsStablePermutation` (generated, seed recorded: the output is the input's
  multiset; equal-score groups are built by construction, entities sharing one name that matches no query and no
  signals, so the expected order is resolve order; the pin's signal tests stay as examples).
  Done in `bb3556d3809b2deac0cbd2e5b2d1693fe272c79c`; choices, seed and mutation checks in `notes/2.1.md`.
- [x] 2.2 Labels and counts (D3 under R1, D4): `Resolution`, `Response.UnreadableRecords` and `FailedRecords`, edges
  sorted before caps, `Node.RelationsTruncated`, `Response.PathsTruncated`, the path stop, impact and graph
  truncation. Check: `TestNLAnswerCarriesSearchCounts`, `TestNLMissCarriesSearchCounts`, `TestSymbolAnswerOmitsCounts`,
  `TestEdgesSortedBeforeCap`, `TestRelationsCapLabelled`, `TestRelationsFaultLabelled`,
  `TestRelationsUnreadTargetLabelled`, `TestPathsCapLabelled`, `TestPathStopsAtUnreadNode`,
  `TestImpactUnreadFileTruncates`, `TestGraphCounterpartFaultTruncates`; a mutant per label. **Review stop.**
  Done in `e6a45550e19aad272875aa3e7953adc396db9c41`; choices and mutation checks in `notes/2.2.md`.
- [ ] 2.3 Construction, metrics, logging, hydration (D4, D5, D7): `NewEngine` with a logger, options and an error; both
  counters through `metric.RegisterOrGet`; the skipped-read counter and one Warn line per facet per call; nil-context
  refusals; `hydrate.go` and its tests; the `storeregistry.Registry` assertion. Check: `TestNewEngineRefusesNilClient`,
  `TestNewEngineRefusesNilLogger`, `TestCounterOnlyOnInjectedRegistry`, `TestEnginesShareOneSeries`,
  `TestRegistrationRefusalReturned`, `TestSkippedReadLoggedAndCounted` (mutants: log removed, counter removed),
  `TestFuseRefusesNilContext`, `TestResolveBodyRefusesNilContext`, the pin's `hydrate_test.go` and `body_reason_test.go`
  tests, `-race`.
- [ ] 2.4 `fusionvocab` (D6, under V1): `New(parents)`. Check: `TestClassSpecificityCountsParents`,
  `TestClassSpecificityNilParentsIsZero`, `TestPredicateSalience`; `go list -deps ./pkg/fusion/...` names no
  semvocab and no `vocabulary/cco`. **Review stop.**

## 3. Close-out

- [ ] 3.1 `pkg/fusion/README.md` and `pkg/fusion/MIGRATION.md` (`NewEngine` logger, options and error; `RetrievalClient`
  without `Status` and with `Resolution`; `index` and `view_revision` gone; `contract_version` "2"; `defer_reason`
  `hard_stop`, `status_unknown`, `bootstrap_incomplete`, `unrecognized_state` gone; a reset that the pin deferred is now
  an error on a seed read and a truncated facet on a walk read; a node's refs list by predicate, then target; the new
  labels, counts and skipped-read counter;
  `fusionvocab.New(cco.Parents)` from semvocab); `Response.DeferReason`'s doc rewritten (pin `contract.go:288-289`);
  `scripts/cover-check.sh` gains `pkg/fusion unit`; ADR-062 ported with its port note and ledger row; ledger rows
  `pkg/fusion`, `pkg/fusion/fusionvocab`; ledger edits: `vocabulary/cco` known_risks
  (`docs/admission-ledger.yaml:2723-2725`), `storage/storeregistry` (`:2271-2272`), `graph` (`:2133-2137`),
  `graph/readiness` (`:2190`, `Reading.Raw` is PR D's question), `graph/embedding` consumer_purpose (`:3767`, pin
  `pkg/fusion` does not import it); stale comments `graph/index_status.go:5` and `:143-144`,
  `graph/readiness/watcher.go:23` and `:251`, `storage/storeregistry/storeregistry.go:22-25` and `:94-95`; port notes on
  ADR-066, ADR-084 and ADR-085 where they name fusion's gate; comment on #110 (item 2 done, stays for item 3); spec sync
  and archive as the last content commit. **Final review.**
