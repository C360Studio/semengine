# Tasks: setup-04a-05-graph-embedding-query

PR B of the change-5 split. Tests come first. Each code task ends at a clean-tree local `task verify` (#171);
integration tests run under `task test:integration -- ./processor/graph-embedding/ ./graph/embedding/`. "Shown to
fail" is `task mutate:check` or failing-first output in `notes/<task>.md`. No `time.Sleep`, no `Skip`, no test
deadline of its own, no `context.WithValue`. Review stops: after 2.2, after 2.4, and before the archive.

## 1. Design

- [ ] 1.1 Pre-owner design review passes; the owner answers the four questions in `proposal.md` and fills
  `Approved-by:`. Gate: `task spec:check`, `task docs:check`.
  Hold: OPEN-Q proposal awaits owner approval.

## 2. Component

- [ ] 2.1 Wire types (D1, D3, D4): `graph.SemanticRequest`, `graph.SimilarRequest`, `DefaultSimilarityLimit`,
  `MaxSimilarityLimit`, `UnreadableRecords` on `SemanticData` and `SimilarData`; the `QueryResponse` doc names the
  embedding verbs among replies reporting 0. Check: `TestRequestsDecodeStrictly`.
- [ ] 2.2 The vector cache on `graphview.View` (D2); remove `Storage.ListGeneratedEntityIDs` and its tests' uses.
  Check: `TestVectorCacheNotAuthoritative`, `TestVectorCachePoisonCountedAndHeals`, `TestFindSimilarOrderAndLimit`,
  `TestVectorCacheRunReportsLostWatch`, the bubble and backlog stop tests, `-race`; benchmark output at 50,000
  records posted on the PR. **Review stop.**
- [ ] 2.3 Handlers, entry checks and ranking (D1, D3, D6), subscribed from the verb table, drained first on Stop.
  Check: `TestVerbsServeTableSubjects`, `TestSearchAnswersEnveloped`, `TestSearchLimitEdges`, `TestSearchQueryEdges`,
  `TestSearchScopeEdges`, `TestSimilarEdges`, `TestTopKAtTie`, `TestScopedSearch`; mutants for the score-above-0
  rule, scope before limit, and the limit refusal.
- [ ] 2.4 Withheld calls and the lost cache watch (D5). Check: `TestQueriesWithheldUntilReady`,
  `TestHealthNamesLostCacheWatch`, `TestNoResponderAfterStop`, lifecycle suite, `-race`. **Review stop.**

## 3. Close-out

- [ ] 3.1 `processor/graph-embedding/MIGRATION.md` (enveloped `data.results`, limit refused not clamped, unknown keys
  and `min_revision` refused, score-0 hits dropped, `similar` field renamed `results`); package `README.md`s
  (graph-embedding, graph/embedding, graph-query's "waits for its responder" lines); ADR-071 port note (the
  responder and `Scope` are present; compatibility note changed); ledger rows `graph/embedding` (item 7) and
  `processor/graph-embedding` (not-carried list); comment on #477 (vector cache site done, issue stays) and on #111
  if the owner picks S0; spec sync and archive as the last content commit. **Final review.**
