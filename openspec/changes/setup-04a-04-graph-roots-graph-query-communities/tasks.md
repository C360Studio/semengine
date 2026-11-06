# Tasks: setup-04a-04-graph-roots-graph-query-communities

Step 3 of the archived `split.md`. Tests come first. Each code task ends at a clean-tree local `task verify` at its
commit (#171); integration tests run under `task test:integration -- ./processor/graph-query/ ./graph/clustering/`.
"Shown to fail" means `task mutate:check`, or the failing-first output, recorded in `notes/<task>.md`. One task is one
developer's unit. The task that adds the table row extends `declaredRows` with its request,
`TestQueryVerbsDeclareEachRespondersVerbs` and `TestGraphQueryRepliesDecodeByTheTable`. Every task that moves a
consumer's shape appends to `processor/graph-query/MIGRATION.md`. Test oracles are literal JSON. Records come from a
real `clustering.Maintainer` and `Tick` wherever one can produce the case. No `time.Sleep`, no `Skip`, no test deadline
of its own; a wait for the view retries the request, each refusal being the event,
under `probe.Await` and `t.Context()`. Each request stays one `CountInput`
and at most one `CountError`.

## 1. Design

- [x] 1.1 Pre-owner design review passes; the owner answers `owner-questions.md`, accepts the new exports and fills
  `Approved-by:`. Gate: `task spec:check`, `task docs:check`.

## 2. The record's revision

- [x] 2.1 `Community.EntityRevision` (`entity_revision`, no `omitempty`), the group's latest applied write, held or not;
  `GroupKey(entityID, level)` and `DecodeCommunity(key, value)`, which refuses bad JSON, a record whose level and ID do
  not give its key, and revision 0; `GetAllCommunities` uses `DecodeCommunity`; the maintainer and `GroupKey` share
  `groupIDs` and `communityKey`. Premise: `graph/clustering/maintainer.go:120-161`, `:240-272`, `:390`;
  `storage.go:55-68`, `:107-110` (D1). Check: `TestRecordNamesItsLatestWrite` (held and unheld deletes, a recreated
  group); `TestMaintainerMatchesModel` on its ascending histories, its model extended from the spec, not from
  `maintainer.go`; `TestHistoryOrderIrrelevant`, comparing records apart from `entity_revision`;
  `TestUnchangedRecordAddsNoRevision`, split into nothing applied (no write) and a same-content upsert (three writes,
  once); `TestGroupKey`; `TestDecodeCommunity`; `TestRecordIsBounded` with the field. A mutant that skips a delete's
  revision shown to fail. Ledger: the `graph/clustering` row's contract and tests. `graph/clustering/README.md`: the
  field, what it promises, and that the caller must feed ascending revisions across keys, which change 7 probes on the
  watch it wires (ADR-112).

## 3. Local search

- [x] 3.1 The request and the ranking, no NATS: decoding and validation, terms, the score, order, the fit and the
  answer, in `processor/graph-query/localsearch.go`; `graph.LocalSearchRequest`, `graph.LocalSearchData`,
  `graph.MaxSearchTerms` in `graph/query_local_search_types.go`. Premise: `pin:graphrag.go:2319-2397` (D4, D5). Check:
  `TestLocalSearchScoresAsThePin`, `TestLocalSearchTermsAreDistinctAndBounded`, `TestLocalSearchOrderIsDeterministic`,
  `TestLocalSearchAnswerTemplate`; the tie break and the repeat removal shown to fail. Carries the behaviours of
  `graphrag_test`'s `TestScoreEntityQuery` and `TestFilterEntitiesByQuery_*` (`inventory.md` § 9).
- [x] 3.2 The community view and the verb: the run context, the view over `COMMUNITY_INDEX` through
  `kvcatalog.OpenCatalogReader` and `DecodeCommunity`, its reopen every 5 s; the `localSearch` row; the handler with the
  record
  read, the entity read when it is missing, the page read and `min_revision`. Concurrency, readiness and lifecycle: an
  Opus developer. Premise: `processor/graph-query/component.go:137-174`, `pkg/graphview/view.go:216`, `:403`,
  `processor/graph-index/component.go:258-283`, `pin:processor/graph-query/component.go:563-576`, `:710-750` (D2, D3,
  D6). Check: `TestLocalSearchReadsOnePage` with a counting
  subscriber, `TestLocalSearchReadsTheAskedLevel`, `TestLocalSearchReadsItsWriteBack`, `TestLocalSearchRefusals`,
  `TestLocalSearchRefusesBadRequests` (no downstream request), `TestLocalSearchEntitiesPassUnchanged`,
  `TestLocalSearchFitsMaxPayload` (`// Own server:` a small `max_payload`), `TestLocalSearchBeforeCommunityIndexExists`,
  `TestLocalSearchAfterCommunityIndexAppears` (with no `GRAPH_STATUS` key and no `COMMUNITY_SUMMARIES`),
  `TestLocalSearchRefusesAfterWatchLoss`, `TestLocalSearchDuringBlockedTick`, `TestStopEndsTheCommunityView`,
  `localSearch`'s case in `TestMinRevisionRefusalThroughGraphQuery` and `TestComposedVerbsCountAsForwardsDo`, the
  lifecycle suite. `entities_truncated` and the poison refusal shown to fail. MIGRATION: the reply, the dropped keys,
  `communityId` to `community_id`, the wrapper, `min_revision`'s stuck case under "The changes that fail silently".
- [x] 3.3 The truncation counter `semengine_graph_query_search_truncated_total{verb}`, through
  `metric.RegisterOrGet` on the injected registry, using the collector it returns (D6). Check:
  `TestTruncatedSearchIsCounted`, a nil registry registering nothing. README: the metric and what it measures for #506.

## 4. Records and guidance

- [ ] 4.1 Port ADR-087 with a note under its title: pin path and SHA, Decisions 2 and 5 kept and how (D7), the store's
  decisions not ported (#485), its ledger row. ADR-099's port note: "shape-for-shape" yields to PR #504 comment
  6588079803 for graph-query's verbs. Ledger: new ADR-087 row; `graph` row names the new exports; `pkg/graphview`'s
  reader is graph-query's community view, not `summary_view.go`; the `processor/graph-query` row's contract,
  dependencies, risks and tests name this step and `community_cache.go` as not carried. Check: `task ledger:diff --
  docs/adr/087-community-summary-store-ownership.md`, `task docs:check`.
- [ ] 4.2 `.agents/skills/query-pattern/SKILL.md` (`localSearch` arrives; the sample is the lexically smallest IDs; send
  a revision of a write to the group searched), `processor/graph-query/README.md`, `doc.go`, `graph/README.md`,
  `MIGRATION.md`'s table and SemSource section, and the three corrections of #522. Check: `task ateam:doctor`,
  `task docs:check`.

## 5. Land

- [ ] 5.1 Cross-agent review of record passes at the last code commit (`reviewed-by:`). PR #524's body carries
  `implemented-by:` and `Closes #406`, `Closes #522`, and no other closing keyword; `inventory-delta.md`, "Open issues",
  lists what stays open. Comments, each read back: #477 (graph-query's site adopts the view), #32 (the owner's Q6
  removed the answer interface), #68 (graph-query's `sync.Once` site is not carried). Check: read
  `closingIssuesReferences` back: #406, #522 and #527 only.
