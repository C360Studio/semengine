# Tasks: fix-706-fusion-nl-merged-search

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" is failing-first output
in `notes/<task>.md` or `task mutate:check`. One pull request (design D5), `Closes #706`.

## 1. Design

- [ ] 1.1 Architect inventory and design (`inventory.md`, `design.md`): INVENTORY PASS at 48c2e10d, then pre-owner
  design review passes; the owner fills `Approved-by:` and rules on design questions 1 to 3.
  Hold: OPEN-Q proposal awaits owner approval

## 2. graph-query: mergedSearch

- [ ] 2.1 Tests first, shown failing on main (`notes/2.1.md`):
  - graph-query (D4, D6): `TestMergedSearchMatchesSearchGraphOrder` (tier 0 and tier 1, searchGraph as the oracle),
    `TestMergedSearchScopesBothSearches`, `TestMergedSearchDecodesStrictly`, `TestMergedSearchReplyShape`,
    `TestMergedSearchTierZeroAsksOnlyWords`, `TestMergedSearchFailedMergeKeepsWords`,
    `TestMergedSearchEmbeddingWaitBound`, `TestMergedSearchBrokenEmbeddingIDKeepsWords`,
    `TestMergedSearchBrokenWordIDRefused`, `TestMergedSearchOtherRefusalIsReturned`,
    `TestMergedSearchWordIndexNotReady`;
  - `TestSearchWithoutWordIndex` extended to `mergedSearch`; the embedding-verbs-off tests pass unchanged;
  - `TestQueryVerbsDeclareEachRespondersVerbs` and `TestGraphQueryServesExactlyTheDeclaredVerbs` with the new row.
  Premises: `processor/graph-query/textsearch.go:76-125,144-169`, `merge.go:32,124-131`, `globalsearch.go:187-201`,
  `metrics.go:16`, `config.go:16`, `graph/query_verbs.go:133`.
- [ ] 2.2 `graph.MergedSearchData` and its result type with doc comments; the `mergedSearch` row; the handler,
  sharing searchGraph's ask-and-merge step (D4) with the embedding wait bound (D6); the merge-failed counter's help
  text; `processor/graph-query/query.go:59-74` wiring. Accept: 2.1 passes, `task verify`.

## 3. fusion: ask the merged search, say where seeds came from

- [ ] 3.1 Tests first, shown failing (`notes/3.1.md`):
  - fusionnats unit tests: `TestNLResolveAsksTheMergedSearch` (replaces `TestNLResolveAsksTheWordIndex`),
    `TestMergedResolveProvenance`, `TestNLMeaningAloneWhileWordIndexBuilds`, plus `TestSubjectsComeFromVerbTable`
    and `TestVerbLookupNamesMissingRow` for the `mergedSearch` row;
  - engine: `TestResolutionProvenanceWins`;
  - integration-tagged: `TestNLMergedOverRealResponders` and the updated `TestNLResolveWithoutWordIndex`.
  `TestProvenanceForMode`, `TestDeferralProvenanceFollowsMode` and the smoke's lexical check stay as they are.
  Premises: `pkg/fusion/fusionnats/client.go:23-58,100-101,148-186`, `pkg/fusion/engine_lens.go:131`,
  `pkg/fusion/retrieval.go:75-80`.
- [ ] 3.2 Implement D1 (B), D2 (a) and D3: `fusion.Resolution.Provenance`, the engine's choice at
  `engine_lens.go:131`, fusionnats' `mergedSearch` row and decode; `Seed.Similarity` and `Node.Similarity` doc
  comments. Accept: 3.1 passes, `task verify`.

## 4. Docs, ledger, follow-up, review

- [ ] 4.1 Update the docs and ledger rows, and file the follow-up:
  - `pkg/fusion/README.md` (`:60,108-151`), `pkg/fusion/MIGRATION.md` (`:77-85,151`, plus the design's Migration
    Plan), `pkg/fusion/fusionnats/doc.go`;
  - `processor/graph-query/README.md`, `doc.go:50-80`, `MIGRATION.md`;
  - `.agents/skills/query-pattern/SKILL.md` (`:29,85,122`);
  - the ADR-062 (`:28-33`) and ADR-071 (`:16-19`) port notes;
  - `docs/admission-ledger.yaml` rows for `pkg/fusion/fusionnats` (`:4817-4893`), `pkg/fusion` (`:4695`), ADR-062
    (`:4794`), ADR-071 (`:4512-4524`) and `processor/graph-query` (`:4419-4421,:4436-4437`);
  - file an issue (`status:blocked`, linked from the PR without a closing keyword): searchGraph waits up to
    `query_timeout_ms` for the embedding search, longer than a caller's default 5 s timeout.
  Accept: `task docs:check`; the issue number is in the PR body.
- [ ] 4.2 Independent code review records an approving head at a commit with a clean local `task verify`; reviewer
  findings resolved.

## 5. Close-out

- [ ] 5.1 Archive the change and sync `openspec/specs/{fusion,graph-query,graph-transport-boundary}` as the last
  content commit; `task spec:check`.
