# Tasks: setup-04a-04-graph-roots-graph-query-search

Step 4 of the archived `split.md`. Tests come first. Each code task ends at a clean-tree local `task verify` at its
commit (#171); integration tests run under `task test:integration -- ./processor/graph-query/ ./processor/graph-index/`.
"Shown to fail" means `task mutate:check`, or the failing-first output, recorded in `notes/<task>.md`. One task is one
developer's unit. The task that adds the table row extends `declaredRows` with its request,
`TestQueryVerbsDeclareEachRespondersVerbs` and `TestGraphQueryRepliesDecodeByTheTable`. Every task that moves a
consumer's shape appends to `processor/graph-query/MIGRATION.md`. Test oracles are literal JSON; questions come from
`notes/probes/step4-pin-probe.log`. No `time.Sleep`, no `Skip`, no test deadline of its own. Each request stays one
`CountInput` and at most one `CountError`. No `context.WithValue`; no new run context (ADR-113: nothing outlives a
handler).

## 1. Design

- [x] 1.1 Pre-owner design review passes; the owner accepts the new exports and fills `Approved-by:`. Gate:
  `task spec:check`, `task docs:check`.

## 2. graph-index

- [x] 2.1 A refused index plan marks the entity failed (D6): `processor/graph-index/index.go:429-432`. Premise:
  `index.go:207-213`, `:243-250`. Check: `TestContractValidStateAlwaysPlans`, a generated check at the byte bounds
  (six-part IDs of 256 bytes, predicates of 194, aliases and names), seed and completed checks recorded; the mark
  recorded as uncovered in the PR, with the reachability reading of D6 and its departures from #428's test sentence
  and failing-first. The generator reaches the 902-byte incoming key and the test asserts it did. Shown to fail by
  D6's mutant: `buildEntityIndexPlan` refusing a predicate of exactly `vocabulary.MaxPredicateBytes`. Ledger: the
  `processor/graph-index` row's risks and tests. Closes #428.

## 3. Global search

- [x] 3.1 Hints and routing, no NATS: the typed read of `query.ClassificationResult.Options`, the D1 table, the
  two-filter refusal, the classifier error as `invalid_request`, the temporary refusal's code and class (D1, D2, D5);
  `graph.GlobalSearchRequest` and `graph.GlobalSearchData` in a new `graph/query_global_search_types.go`. Premise:
  `graph/query/classifier_chain.go:90-146`, `:182-222`, `pin:graphrag.go:633-679`. Check:
  `TestGlobalSearchRoutesByHints` (with D1's five mixed-hint rows), `TestGlobalSearchRefusesTwoFilters`,
  `TestGlobalSearchRefusesImpossibleNumbers`, `TestGlobalSearchEndedClassification`, strict decoding (`level` refused
  by name); the path-plus-count and path-plus-time rows shown to fail. Carries the behaviours of
  `slice_e_contract_test`'s strategy reports, renamed.
- [x] 3.2 The resolver and the `entity` and `path` strategies, the `globalSearch` row and handler (D3, D4):
  `entityByAlias` calls the shared resolver; `path` calls step 2's walk in process. Premise:
  `processor/graph-query/query.go:161-197`, `pathsearch.go:29-178`, `pin:entity_resolver.go`,
  `pin:graphrag.go:402-490`, `:1001-1066`, `:2562-2609`. Check: `TestGlobalSearchEntity`, `TestGlobalSearchPath`,
  `TestGlobalSearchPassesAliasRefusals`, `TestGlobalSearchUnknownReferenceFallsThrough`,
  `TestZoneQuestionNeverAnswersEmpty`, `TestGlobalSearchTextTierRefusal` (with and without `COMMUNITY_INDEX` records),
  `TestEntityByAlias` unchanged, `globalSearch` in `TestMinRevisionRefusalThroughGraphQuery` and
  `TestComposedVerbsCountAsForwardsDo`, the lifecycle suite. A resolver that swallows `alias_ambiguous` shown to fail.
  Carries `query_error_class_test`'s global-search cases (`:152-220`). MIGRATION: request keys, reply, strategy names,
  partial IDs only as aliases, the temporary code. Closes #405, #479.
- [x] 3.3 The `temporal` and `spatial` strategies (D4): `graph.TemporalRangeRequest`, `graph.SpatialBoundsRequest`.
  Premise: `pin:graphrag.go:1196-1358`, `graph/query_search_types.go:37-52`. Check: `TestGlobalSearchTemporal`,
  `TestGlobalSearchSpatial` (a test responder on the real broker; request JSON literal; `truncated` passed),
  `TestGlobalSearchNoResponders`. Carries `query_error_class_test.go:570-621`.

## 4. Records and guidance

- [x] 4.1 ADR-114 Consequences: D3's one sentence on global search. Ledger: `graph` row names the four exports;
  `graph/query` row names its first production reader; `processor/graph-query` row's contract, dependencies
  (`graph/query`), risks and tests name this step and what is not carried (D7). Check: `task docs:check`.
- [x] 4.2 `.agents/skills/query-pattern/SKILL.md` (`globalSearch` arrives; which questions it answers),
  `processor/graph-query/README.md`, `doc.go`, `graph/README.md`, `MIGRATION.md`'s table and step row. Check:
  `task ateam:doctor`, `task docs:check`.

## 5. Land

- [ ] 5.1 Cross-agent review of record passes at the last code commit (`reviewed-by:`). PR #530's body carries
  `implemented-by:` and `Closes #405`, `Closes #428`, `Closes #479`, and no other closing keyword;
  `inventory-delta.md` § 4 lists what stays open. Comments, each read back: #111 (step 4 removed the semantic
  site; the `graphrag` site is step 5's), #487 (note 1 applied: explicit chains; no depth hint at tier 0), #486 (the
  routing table is where an LLM tier's strategies join). Check: read `closingIssuesReferences` back: #405, #428 and
  #479 only.
