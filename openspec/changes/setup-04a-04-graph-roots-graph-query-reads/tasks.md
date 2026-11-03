# Tasks: setup-04a-04-graph-roots-graph-query-reads

Step 2 of the archived `split.md`. Tests come first. Each code task ends at a clean-tree local `task verify` at its
commit (#171); integration tests run under `task test:integration -- processor/graph-query`. "Shown to fail" means `task
mutate:check`, or the failing-first output, recorded in `notes/<task>.md`. One task is one developer's unit. Every task
that adds a table row extends `declaredRows` (with a per-row request, since a composed verb decodes strictly and
`everyVerbRequest` no longer fits) and `TestQueryVerbsDeclareEachRespondersVerbs`, and adds its row to
`TestGraphQueryRepliesDecodeByTheTable`. Every task that moves a consumer's shape appends to
`processor/graph-query/MIGRATION.md`. Test oracles are literal JSON. Real responders wherever a real one can produce the
case. No test fails on its own timer unless marked `// Own deadline:`; no `time.Sleep`, no `Skip`; no reach check after
`rapid.Check`. Each count rule stays one `CountInput` and at most one `CountError`.

## 1. Design

- [ ] 1.1 Pre-owner design review passes; the owner answers `owner-questions.md`, accepts the new exports and fills
  `Approved-by:`. Gate: `task spec:check`, `task docs:check`.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Forwards

- [ ] 2.1 Four forwards: the eight rows, `ResponderGraphEmbedding`, `ResponderGraphIndexSpatial`,
  `ResponderGraphIndexTemporal`, and `SemanticData`, `SimilarData`, `SimilarityHit`, `SpatialData`, `SpatialHit`,
  `TemporalData`, `TemporalHit` in a new `graph/query_search_types.go`. `queryHandlers` takes each forward's downstream
  row by responder and name, since `semantic` forwards to `search`; forwards stay byte for byte. Premise:
  `processor/graph-query/query.go:24-35`, `graph/query_verbs.go:44-81` (D1). Check: `TestForwardPassesBytesBothWays` with
  `scope` on `semantic`, `TestNoRespondersIsTransient`, `TestForwardedRefusalKeepsClassCodeDetail`, reply decode of each
  from literal JSON on a stub, `TestReservedSubjectsDeclaredOnce`. MIGRATION: the four reply shapes, `similar` to
  `results`, `id` to `entity_id`, `start_time` and `end_time` at change 7.

## 3. Path search

- [ ] 3.1 `pathSearch` with its row, `PathSearchRequest`, `PathSearchData`, `PathEntity`, `MaxPathSearchDepth`,
  `MaxPathSearchNodes`, `DefaultPathSearchNodes` in `graph/query_path_types.go`. The walk is one unexported function taking
  the hop as an argument, also what production calls, with no test-only path. Premise: P7-P12,
  `pin:processor/graph-query/pathrag.go` (D2, D3, D4). Check: `TestPathSearchRefusesBadRequests`,
  `TestPathSearchOnARealIndex`, `TestPathSearchTruncatedIsExact`, `TestPathSearchMatchesTheModel`,
  `TestPathSearchRevisionIsTheSmallest`, `TestPathSearchRefusedMidWalk`, `TestPathSearchRequestsAreBounded`,
  `TestSlowPathSearchLeavesOtherVerbsAnswering`, the two new cases of `TestMinRevisionRefusalThroughGraphQuery` and
  `TestComposedVerbsCountAsForwardsDo`; the exactness and limit checks shown to fail. Carries the behaviours of
  `pathrag_test` and `TestComponent_PathSearch_*` (`inventory.md` § 9); their mock goes. MIGRATION: request keys, the reply,
  required `max_depth` and `direction`, dropped `timeout`, `include_siblings`, `max_paths`, `type`, `score`; a decoder of
  `paths` reads empty, so it goes under "The changes that fail silently".

## 4. Counts

- [ ] 4.1 `hierarchyStats` with its row, `HierarchyStatsRequest`, `HierarchyStatsData`, `HierarchyChild`, and the one page
  read both count verbs share, IDs only. Premise: `pin:query.go:470-560`, P4, `processor/graph-ingest/query.go:280-346`
  (D4, D5, D7). Check: `TestHierarchyStatsDeclaresAPartialPage`, `TestCountsAddUpAcrossPages`, `TestCountsOfAnEmptyGroup`,
  `TestCountsReadOnePage`, `TestCountsRefuseABrokenPage`, `TestAuthorityOnlyAnswerClaimsNoRevision`; `truncated` shown to
  fail. MIGRATION, under "The changes that fail silently" too: `totalEntities` to
  `entities_scanned`, `truncated`, `next_cursor`, `name` gone, enveloped.
- [ ] 4.2 `summary` with its row; `SummaryRequest` and `SummaryData` change (`exclude_predicates`, no
  `entity_sample_limit`, `cursor`, `min_revision`, `entities_scanned`, `truncated`, `next_cursor`; `predicates` without
  `omitempty`; no `predicate_total`). Needs 4.1's page read. Premise: `graph/query_summary_types.go:21-104`, P4,
  `pin:summary.go:95-109` (D5, D6, D7). Check: `TestSummaryPredicates`, `TestSummaryRefusesWhenPredicatesCannot`,
  `TestAuthorityOnlyAnswerClaimsNoRevision` (excluded case), a literal test of type buckets in ADR-102 order carried from
  `summary_test`, with its `StillReturnsEntityFacet` case reversed. A PR comment on #105 and #111 says which item landed.
  MIGRATION, under "The changes that fail silently" too: those keys; and a down index now refuses.

## 5. Records and guidance

- [ ] 5.1 Port ADR-071 with a note under its title (pin path and SHA, packages, ledger row, that `pkg/fusion` and
  `graph-embedding` arrive in change 5). Ledger: new ADR-071 row; the `processor/graph-query`
  row's contract, dependencies, risks and tests name this step; `pathrag.go`, `summary.go` leave "later steps". Check:
  `task ledger:diff -- docs/adr/071-fusion-domain-scoped-nl-retrieval.md`, `task docs:check`.
- [ ] 5.2 `.agents/skills/query-pattern/SKILL.md`, `graph/README.md`, `processor/graph-query/README.md`, `doc.go`:
  the new verbs, limits, costs, the sample being the lexically smallest IDs, `indexed_revision` 0. `graph/query_response.go`'s
  doc says 0 claims nothing. `AGENTS.md` § 2 and `.agents/rules-index.md`: the reserved-subject row names the three new
  families. `MIGRATION.md`'s table and new-names list current. Check: `task ateam:doctor`, `task docs:check`.

## 6. Land

- [ ] 6.1 Cross-agent review of record passes at the last code commit (`reviewed-by:`). PR #511's body carries
  `implemented-by:` and no closing keyword; `design.md`, "Scope", lists what stays open. Check: read
  `closingIssuesReferences` back, empty.
