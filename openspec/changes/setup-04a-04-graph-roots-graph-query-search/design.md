# Design: `processor/graph-query` step 4, the global search coordinator

Base `f5ffe3b`; pin `8b99efe9`. Evidence: `inventory-delta.md` (P1-P5), archived `inventory.md` §§ 3, 8, 9, 13 (Q5),
`notes/probes/`. Binding: ADR-114 Consequences, ADR-109 D1/D4, ADR-113, owner on #479 (comment 6527967116) and #428
(comment 6688101995), PR #504 comment 6588079803. **Goal.** `globalSearch` answers the questions tier 0 can answer
exactly, and refuses the rest with a code that says "not in this build", never an empty or filter-less answer.
**Non-goals:** the text tier and `searchGraph` (step 5); LLM classifier and strategies only it reaches (#486). Pin
about 730 + 390 lines; this about 450 + 1,000 (estimate), one PR.

## D1. Which strategy answers, read from typed hints

The keyword chain (`query.NewClassifierChain(query.NewKeywordClassifier(), nil)`) classifies once. Its `Options` are
read once into a small typed value; dispatch is a switch over it, not bare strings (Q5(b)).

| Hints | Strategy | At the pin (P1, P2) |
| --- | --- | --- |
| Path intent with a start node, no time, no area | `path` | `pathrag`; a time range was ignored |
| Time range only | `temporal` | same |
| Area only | `spatial` | same |
| No hint, the question one token | `entity` | `graphrag` (`entity_lookup` needed the LLM) |
| Two of path, time, area | refused `invalid_request`, naming both | `graphrag`, both filters dropped |
| Anything else: no hint, path words with no start, similarity, count, ranking | text-tier refusal (D2) | `graphrag` |

An `entity` or `path` reference that resolves to nothing falls through to the text tier, so it gets D2's refusal
(ADR-114: not found falls through). The classifier's impossible-number error reaches the caller as `invalid_request`
naming the phrase (owner, #479). *Proof:* `TestGlobalSearchRoutesByHints`, one row per table line, questions from
P1's log; `TestGlobalSearchRefusesTwoFilters`; `TestGlobalSearchRefusesImpossibleNumbers`.

## D2. The temporary refusal

| Option | Cost |
| --- | --- |
| (a) transient `index_not_ready` | Breaks ADR-109 D4's reservation; a caller retries a question no index can answer |
| (b) a new code, class transient | Retry-on-transient callers loop until step 5 merges, a code change, not a wait |
| **(c) `text_search_unsupported`, class invalid** | Says "retrying unchanged will not help"; one code to delete in step 5 |
| (d) do nothing: no `globalSearch` until step 5 | Entity and path questions wait a step for nothing |
| (e) answer empty, as the pin's zone path did | The silent wrong answer #479 item 3 reports |

**(c).** Message: "this question needs the text search, which this build of graph-query does not have". It reads no
`COMMUNITY_INDEX`, so it is the same with or without records, and `index_not_ready` keeps its meaning. Unexported in
`processor/graph-query` (no consumer should build on it), spelled in the spec and `MIGRATION.md`; step 5 removes it.
Not an error in `Health` (a declared limit, like a caller's `invalid_request`). *Proof:* `TestGlobalSearchTextTierRefusal`
(with and without community records; class, code; `Health` unchanged).

## D3. One resolver (#405)

`entityByAlias`'s rule becomes one function both use: ask the alias verb with the reference; a holder wins; else a
valid six-part ID is itself; else not found, with no second request. `alias_ambiguous`, `index_not_ready`, `internal`,
`revision_not_reached` reach the caller as they arrived (ADR-114). The suffix step goes (#405 option 1): a partial ID
such as `sensor-001` resolves only as an alias, a migration note. *Proof:* `TestGlobalSearchPassesAliasRefusals` (each
code), `TestGlobalSearchUnknownReferenceFallsThrough`, step 1's `TestEntityByAlias` unchanged; #479 item 3:
`TestZoneQuestionNeverAnswersEmpty` ("the area of the plant", P3, gets D2's refusal).

## D4. Strategies and the reply

Enveloped `graph.GlobalSearchData`: `strategy` and exactly one of `entity` (the entity verb's reply bytes, unchanged),
`path` (`graph.PathSearchData`), `temporal` (`graph.TemporalData`), `spatial` (`graph.SpatialData`). Each is the
explicit result, one hop per edge (#487 note 1); nothing is re-derived or summarised, and no entity is loaded beside
it (a caller wanting states calls `batch`). `path` runs step 2's walk in process from the resolved start, direction
`both`, depth 3 and 100 nodes (the pin's), predicates from the hints; no tier-0 hint names a depth, so none is refused
for depth (a later tier's depth hint over `MaxPathSearchDepth` is `invalid_request`). `temporal` and `spatial` request
`graph.temporal.query.range` and `graph.spatial.query.bounds` with new `graph.TemporalRangeRequest` (`start_time`,
`end_time`, `limit` 100) and `graph.SpatialBoundsRequest` (`north`, `south`, `east`, `west`, `limit` 100), the
declaration change 7's responders decode; their `truncated` passes through. Until change 7 they refuse transient
`internal` naming the subject, as step 2's forwards. `indexed_revision`: the alias reply's (entity), the walk's (path),
the responder envelope's (temporal, spatial); `min_revision` reaches each. *Proof:* `TestGlobalSearchEntity`,
`TestGlobalSearchPath` (a real index; the reply equals `pathSearch`'s for the same request), `TestGlobalSearchTemporal`
and `TestGlobalSearchSpatial` (a test responder on the real broker checks the request JSON literally),
`TestGlobalSearchNoResponders`, `globalSearch` in `TestMinRevisionRefusalThroughGraphQuery` and
`TestComposedVerbsCountAsForwardsDo`.

## D5. Request

`graph.GlobalSearchRequest`: `query` (required, not blank) and `min_revision`, decoded strictly. The pin's `level`,
`max_communities`, `summarize_threshold`, `include_*` are refused by name until step 5 brings what reads them. No
length bound beyond `max_payload`: classification is linear in the question and scans no entity.

## D6. #428: a refused index plan fails the entity

Owner's two choices: fail and hold readiness, or complete with a skip metric. **Fail and hold:** `indexEntity` marks
the entity failed before returning the plan's error, so `ErrorCount`, `semengine_graph_index_write_failures_total` and
the failed set count it and graph-index refuses `index_not_ready` until the entity is rewritten or deleted (ADR-066: an
index that drops rows and reports caught up). Completing would leave its old rows answering silently. *Reachability,
read not probed:* every check in `buildEntityIndexPlan` is implied by `UnmarshalEntityState`'s contract and the byte
budgets (incoming key at most 902 of 1,024 bytes; predicate alphabet inside the KV alphabet), so no stored value
drives the branch and no test can without a hook (#186). *Proof:* `TestContractValidStateAlwaysPlans`, a generated
check at the bounds (seed recorded) that what the contract accepts plans; the mark itself is shown by reading and
recorded as uncovered.

## D7. Not carried, ADRs, issues

Not carried: `extractSearchRefinements`, `filterClassifierTemplates` and the semantic strategy with its type-filter
fallback (`pin:graphrag.go:523-571`, `:1068-1191`), reached only from LLM output (P1; owner question 1); the
`graphrag` default (step 5); pin `Count`, `DurationMs`, `Degraded*`; strategy names `pathrag`, `entity_lookup` become
`path`, `entity`. ADR-060, -109, -112, -113, -114 applied; ADR-066 for D6; ADR-071's site leaves with the semantic
strategy. Retires #405, #428, #479; #111 stays (step 5's site, item 4). New exports for the owner: `graph.
GlobalSearchRequest`, `GlobalSearchData`, `TemporalRangeRequest`, `SpatialBoundsRequest`; row `globalSearch`.
