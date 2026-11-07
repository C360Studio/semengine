# Tasks: setup-04a-04-graph-roots-graph-query-text

Step 5 of the archived `split.md`. Tests come first. Each code task ends at a clean-tree local `task verify` at its
commit (#171); integration tests run under `task test:integration -- ./processor/graph-query/ ./pkg/graphview/`.
"Shown to fail" means `task mutate:check`, or the failing-first output, recorded in `notes/<task>.md`. One task is one
developer's unit. Test oracles are literal JSON. No `time.Sleep`, no `Skip`, no test deadline of its own. Each request
stays one `CountInput` and at most one `CountError`. No `context.WithValue`; no new goroutine or run context (ADR-113).
Every task that moves a consumer's shape appends to `processor/graph-query/MIGRATION.md`. Implementation note, not a
design prerequisite: before writing a task's tests, record the generated-check decision (`docs/testing.md:436-439`)
in `notes/<task>.md`.

## 1. Design

- [x] 1.1 Pre-owner design review passes; the owner rules on D1's option, accepts the new exports and the ADR-114
  sentence, and fills
  `Approved-by:` (PR #544 comment 6711659719: option (a)). Gate: `task spec:check`, `task docs:check`.

## 2. The counting enumeration

- [ ] 2.1 `View.ListCounted` (D8): `pkg/graphview/view.go`; `clustering.LevelPrefix` (D2) beside `GroupKey`.
  Premise: `view.go:403-450`, `:611`; `graph/clustering/storage.go:101-110`, `:137`, `maintainer.go:20`. Adopters,
  each onto `LevelPrefix` and one range check: `storage.go:102` (`GroupKey`), `:124` (`DecodeCommunity`), `:138`
  (`communityKey`), `maintainer.go:329` (`exists`); clustering's tests unchanged. Check:
  `TestLevelPrefix` (0 to 2; -1 and 3 refused invalid),
  `TestListCountedCountsPoison` (poison, heal by a valid write, delete, `ErrNotReady`, lost, stopped), `-race`; a
  count taken outside the lock shown to fail under `-race`, or the mutant that skips the count. Ledger: `pkg/graphview`
  row names the call and its one reader.

## 3. The text tier

- [ ] 3.1 Routing (D1, the option the owner rules): `chooseRoute` and the resolver's miss; `text_search_unsupported`
  removed; `level` on `graph.GlobalSearchRequest` (0 to 2) and its doc comment (`graph/query_global_search_types.go:5-7`).
  Premise: `processor/graph-query/globalsearch.go:27`, `:97`, `:219`, `:323-356`; `graph/query/classifier.go:48-94`.
  Check: `TestGlobalSearchRoutesByHints` (each row's code and text, with "http connection pool", "file path
  handling", "max retries"), `TestGlobalSearchUnknownPathStart`, `TestGlobalSearchUnknownWordIsSearched`,
  `TestZoneQuestionNeverAnswersEmpty` (expects `entity_not_found`, was `text_search_unsupported`),
  `TestGlobalSearchHintBesideAFilter`, `TestGlobalSearchUnknownIDIsSearched`, `TestGlobalSearchRequestKeys`;
  `TestGlobalSearchTextTierRefusal` removed. Step 4 tests that expect `text_search_unsupported` and change:
  `TestGlobalSearchUnknownReferenceFallsThrough` (`globalsearch_integration_test.go:348`, a valid ID with no record:
  now text), the assertion at `globalsearch_integration_test.go:48`, and the ten rows of
  `TestGlobalSearchRoutesByHints` at `globalsearch_test.go:97-111`, each to its D1 outcome; the count refusal shown
  to fail. ADR-114 sentence (D7).
- [ ] 3.2 Groups and matches (D2, D3, D5): enumeration, group score with ties by key, five above zero, one page each,
  `rankEntities`, the revision mark. Premise: `pin:graphrag.go:1374-1499`, `:2416-2470`; `localsearch.go:50`, `:119`;
  `counts.go:32`; `graph/clustering/maintainer.go:173-184`. Check: `TestTextTierReadsAtMostFiveGroups` (counting
  subscriber), `TestTextTierSkipsUnmatchedGroups`, `TestTextTierRefusesAnEmptyLevel`, `TestTextTierReadsTheAskedLevel`,
  `TestTextTierMatchesEqualAFilter`, `TestTextTierMarksACutPage`, `TestTextTierScore`, `TestTextTierRevision`,
  `TestTextTierRevisionIgnoresUnreadable`, `TestTextTierAllUnreadable`; `localSearch` tests unchanged after
  `rankEntities` returns ID and score. Carries `graphrag_test`'s
  scoring cases with the pin's order corrected (P1, P2).
- [ ] 3.3 Reply (D4, D6): `graph.TextSearchData`, `TextSearchGroup`, `TextSearchMatch` in
  `graph/query_global_search_types.go`, and the `GlobalSearchData` comment (`:13-18`) naming five strategies; label
  set read at `Start`, answer, fit, poison count with its Warn (D6), truncation counter. Premise:
  `vocabulary/registry.go:477`, `localsearch.go:172`, `metrics.go:15`. Check: `TestTextTierReply`, `TestMatchLabel`,
  `TestTextTierAnswer`, `TestTextTierFitsMaxPayload`, `TestTextTierPoisonIsDeclared`, `TestTruncatedSearchIsCounted`,
  `TestComposedVerbsCountAsForwardsDo`, the lifecycle suite. Carries `entity_digest_labels_test`'s label cases that
  survive D4. Closes #31.

## 4. `searchGraph`

- [ ] 4.1 The row and handler (D7). Premise: `graph/query_verbs.go:115`, `pin:searchgraph.go`. Check:
  `TestSearchGraphAnswersAsGlobalSearch`, `TestQueryVerbsDeclareEachRespondersVerbs`,
  `TestGraphQueryRepliesDecodeByTheTable`, `TestGraphQueryServesExactlyTheDeclaredVerbs`, `searchGraph` in
  `TestMinRevisionRefusalThroughGraphQuery`. Carries `searchgraph_test`'s non-fallback cases. MIGRATION: SemSource's
  decode and `summarize_threshold`. Closes #106, #103.

## 5. Records and guidance

- [ ] 5.1 Ledger: `graph` row (the new exports), `processor/graph-query` row's contract, dependencies (`vocabulary`
  label set), risks (D2's keyword miss, D5's empty-group exception, #525) and tests; what is not carried (D4, D7).
  `.agents/skills/query-pattern/SKILL.md:89`, `:125-126`, `docs/repository-map.md:90`,
  `processor/graph-query/README.md`, `doc.go:49`, `graph/README.md`, `MIGRATION.md`'s table, step row and "silent"
  section (SemSource's decode, `summarize_threshold`, the lost tier-1 fallback over a missing community index, R4;
  under option (b) the trigger-word list). Check: `task ateam:doctor`, `task docs:check`.

## 6. Land

- [ ] 6.1 Cross-agent review of record passes at the last code commit (`reviewed-by:`). PR #544's body carries
  `implemented-by:` and `Closes #31`, `Closes #103`, `Closes #106`, and no other closing keyword. Comments, each read
  back: #15 (#103 closed; the generation marker is this issue's), #111 (item 3's last site not carried), #506 (the
  counter gains `globalSearch` and `searchGraph`), #487 (note 2 applied). Check: `closingIssuesReferences` reads #31,
  #103, #106 only.
