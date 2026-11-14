# Tasks: setup-04b-03-graph-query-lexical

Tests come first. Each code task ends at a clean-tree local `task verify`. "Shown to fail" is `task mutate:check` or
failing-first output in `notes/<task>.md`. Premises cite `inventory.md` (INVENTORY PASS at `ef4313c3`), `design.md`
and `notes/probes/`. Each code task names its pull request (design D9): A or B.

## 1. Design

- [ ] 1.1 Architect inventory and design (`inventory.md`, `design.md`, `notes/probes/`): where words are searched,
  local and global search, hydration, the router, what search accepts, the tier 1 fallback's shape, the quality bar.
  Pre-owner design review passes; the owner fills `Approved-by:` and rules on design questions 1 and 2.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Code

- [ ] 2.1 PR A. What search accepts (design D6). Premise: `ParseQuery` counts distinct tokens against
  `graph.MaxSearchTerms` (`graph/lexical/query.go:96-97`; 16 at `graph/query_local_search_types.go:7`) and reserves a
  final `*` or `~` after a word rune (`:125-138`); graph-lexical refuses a parse error `invalid_request`
  (`processor/graph-lexical/query.go:75-78`); pasted lines hold up to 21 words and SemSource's questions up to 10;
  worst case at 50,000 entities is 19.6 ms at 16 words and 24.5 ms at 32 (`notes/probes/query-length-probe.log`).
  Accept: `TestParseQuery` refuses 33 distinct words and takes 32; `TestParseQueryReservesForms` takes `*bold* text`
  and still refuses `pump*`, `retry~` and `name:retry`, and the `~` refusal's message says to quote the term;
  `TestParseQueryExcluded` lists `race` for `go test -race`; `TestSearchDeclaresExcluded` (graph-lexical integration)
  sees `excluded` in the reply; `FuzzParseQuery`, `TestScorecardHoldsItsFloor` and `TestScorecardBehaviour` pass
  unchanged; a mutant that drops the leading-star check is shown to fail. PR body: `Closes #624`.
- [ ] 2.2 PR B. Local search on the index (design D2, D4). Premise: today's local search reads one prefix page and
  ranks it by substring (`processor/graph-query/localsearch.go:252-317`, `:96`); a group ID is a valid scope
  (`graph/clustering/storage.go:101-111`, `pkg/types/entity_id.go:190`, `graph/id_prefix.go:45`); the batch reply
  names missing IDs and comes in no set order (`graph/query_batch_types.go:51-56`,
  `processor/graph-ingest/query.go:130-148`). Accept: `TestLocalSearchIsScopedToTheGroup` (a better match outside the
  group is not returned), `TestLocalSearchRecordStates` (`read`, `none`, `not_ready`, `unreadable`, none refusing),
  `TestLocalSearchPassesWordIndexRefusals` (`index_not_ready`, `graph_state_reset_required`, `revision_not_reached`,
  a parse refusal, each as it arrived), `TestSearchMatchesKeepHitOrder` and `TestSearchDropsMissingHits`; the
  substring scorer and `checkedSearchTerms` are deleted.
- [ ] 2.3 PR B. Global search and its groups (design D3, D4). Premise: today's text tier lets records gate the read
  (`processor/graph-query/textsearch.go:50-92`, `:111-153`); the answer's group is among the first ten hits' groups
  for 98.0% of questions (`notes/probes/search-quality-probe.log`). Accept: `TestGlobalSearchGroupsHits` (groups in
  order of their best match, counts summing to the matches, records attached), `TestGlobalSearchWithoutCommunityIndex`
  (answers with records `not_ready`), `TestGlobalSearchMatchesEqualWordIndexOrder` and `TestTruncatedSearchIsCounted`
  updated; today's text tier and its group score are deleted.
- [ ] 2.4 PR B. The router (design D5). Premise: `chooseRoute` refuses on aggregation, two filters and a hint beside a
  filter (`processor/graph-query/globalsearch.go:399-449`); 27 plain questions route or refuse as
  `openspec/changes/archive/2026-11-12-setup-04b-01-lexical-index/notes/probes/router-probe.log` shows. Accept:
  `TestEveryQuestionIsSearched` feeds the 27 probe questions and #535's: none is refused, each reply holds `text`, and
  each hint is listed honoured or not with a reason; `TestVerifiedStartAddsAWalk` (a start that resolves adds `path`
  beside `text`); `TestUnverifiedStartIsReported` (`pump-99` is a not-honoured hint with code `entity_not_found`);
  `TestClassifierRefusalIsAHint`; `TestGlobalSearchEndedClassification` unchanged.
- [ ] 2.5 PR B. The quality bar (design D8). Premise: the word index is within one point of a plain BM25 globally
  and locally, today's paths far below (`notes/probes/search-quality-probe.log`). Accept: `TestSearchScorecard`
  passes, logging R@1, R@10 and MRR for both verbs and the reference; a mutant that drops the local scope, and one that
  re-sorts matches by ID, are each shown to fail it.
- [ ] 2.6 PR B. Smoke (design D9; #628 second item). Premise: `checkCommunities` reaches local and text search through
  the community record (`internal/smoke/smoke_integration_test.go:788`); composed-stack `:32-33` and `:98`. Accept:
  `TestSmoke_OneConfigAnswers` and `TestSmoke_BrokerRestartUnderRunningStack` pass with local and global search
  awaited on graph-lexical's `indexed_revision`, and the bm25 assertions left for change 4.

## 3. Docs and close-out

- [ ] 3.1 PR B. `processor/graph-query/README.md` (`:31-33`, `:129-230`), `MIGRATION.md` (SemSource's `graph_search`,
  `localSearch` callers), `.agents/skills/query-pattern/SKILL.md:79-134`, the ledger row
  (`docs/admission-ledger.yaml:4346-4396`) and ADR-087's port note (design question 2) say what the verbs do now;
  `task docs:check` passes.
- [ ] 3.2 File the batch-read defect (design D4): one damaged entity refuses a whole `graph.ingest.query.batch` reply,
  beside #578, linked from PR B without a closing keyword.
- [ ] 3.3 PR B. Sync the specs and archive the change as the last content commit; `task spec:check` passes.
