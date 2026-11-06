# Design: `processor/graph-query` step 4, the global search coordinator

Base `f5ffe3b`; pin `8b99efe9`. Evidence: `inventory-delta.md` (P1-P5), archived `inventory.md` §§ 3, 8, 9, 13 (Q5),
`notes/probes/`. Binding: ADR-114 Consequences, ADR-109 D1/D4, ADR-113, owner on #479 (comment 6527967116) and #428
(comment 6688101995), PR #504 comment 6588079803. **Goal.** `globalSearch` answers the questions tier 0 can answer
exactly, and refuses the rest with a code that says "not in this build", never an empty or filter-less answer.
**Non-goals:** the text tier and `searchGraph` (step 5); LLM classifier and strategies only it reaches (#486). Pin
about 730 + 390 lines; this about 450 + 1,000 (estimate), one PR.

## D1. Which strategy answers, read from typed hints

The keyword chain (`query.NewClassifierChain(query.NewKeywordClassifier(), nil)`) classifies once. Its `Options` are
read once into a small typed value; dispatch is a switch over it, not bare strings (Q5(b)). **A strategy answers
only when its hint is the question's only hint**: a strategy that answered past a count, a ranking limit or a
similarity hint would answer a different question.

**A one-word question is a reference, read before any hint.** A question of one token (no white space after trimming)
goes to the resolver and the `entity` strategy without being classified: the keyword patterns find trigger words
inside IDs and aliases (`acme.plant.today.sensor.temp.001` reads as a time range, `acme.plant.zone-a.sensor.temp.001`
and `connector-7` as paths, `max-pump` and `total-flow` as counts, `....most.001` as a ranking). Nothing resolves:
D2's refusal. Every other question is classified, and the table applies.

| Hints | Strategy | At the pin (P1, P2) |
| --- | --- | --- |
| Path intent with a start node, nothing else | `path` | `pathrag`; a time range or count was ignored |
| Time range, nothing else | `temporal` | same; a count or similarity was ignored |
| Area, nothing else | `spatial` | same |
| Two of path, time, area, with or without other hints (checked first: the caller can fix it); an entity and a zone or area phrase, refused by the classifier | refused `invalid_request`, naming both | `graphrag`, both filters dropped |
| Any aggregation, ranking or similarity hint, with or without others; no hint and several tokens; path words with no start | text-tier refusal (D2) | `pathrag`, `temporal` or `graphrag` |

The classifier's impossible-number error reaches the caller as `invalid_request` naming the phrase (owner, #479). A
nil result with a nil error (an ended context, `graph/query/classifier_chain.go:91-93`) is transient `internal`, one
error in `Health`, never "no hints"; so are hints that cannot be read and a classifier error that is not class
invalid, each one error. **The classifier's limits stay its own:** "no answer drops a hint" holds for the hints the
classifier reports, because the classifier refuses, class invalid, a question naming an entity and a zone or area
phrase ("the question names two places, sensor-001 and zone-b; ask about one"; `graph/query/classifier.go:164` used to
skip the zone), and global search passes that refusal as `invalid_request` with its text. "From A to B" walking from A
stays the classifier's documented limit, #535. A start in a question is
whatever the keyword classifier extracts: a hyphenated segment of a full ID ("connected to
acme.plant.robotics.gcs.drone-x.001" walks from alias `drone-x`), never the whole ID. *Proof:*
`TestGlobalSearchOneWordIsAReference` (the IDs and aliases above); `TestGlobalSearchRefusesADroppedPlace` ("what is
connected to sensor-001 in zone-b", "... near zone-b"); `TestGlobalSearchRoutesByHints`, one row per table
line, plus "how many sensors are connected to pump-42", "top 5 sensors connected to pump-42", "find sensors similar to
pump-42 in the last 2 hours" and "how many pumps changed in the last 2 hours", each D2's refusal, and "how many pumps
connected to pump-42 in the last 2 hours", `invalid_request` (two filters win); `TestGlobalSearchRefusesTwoFilters`;
`TestGlobalSearchRefusesImpossibleNumbers`; `TestGlobalSearchEndedClassification`.

## D2. The temporary refusal

| Option | Cost |
| --- | --- |
| (a) transient `index_not_ready` | Breaks ADR-109 D4's reservation; a caller retries a question no index can answer |
| (b) a new code, class transient | Retry-on-transient callers loop until step 5 merges, a code change, not a wait |
| **(c) `text_search_unsupported`, class invalid** | Says "retrying unchanged will not help"; one code to delete in step 5 |
| (d) do nothing: no `globalSearch` until step 5 | Entity and path questions wait a step for nothing |
| (e) answer empty (the pin's branch for a nil community cache, `pin:graphrag.go:475-489`, unreachable after a successful `Start`, which builds the cache first, `pin:component.go:556`) | A silent wrong answer |

**(c).** Message: "this question needs the text search, which this build of graph-query does not have". It reads no
`COMMUNITY_INDEX`, so it is the same with or without records, and `index_not_ready` keeps its meaning. Unexported in
`processor/graph-query` (no consumer should build on it), spelled in the spec and `MIGRATION.md`; step 5 removes it.
Not an error in `Health` (a declared limit, like a caller's `invalid_request`). *Proof:* `TestGlobalSearchTextTierRefusal`
(with and without community records; class, code; `Health` unchanged).

## D3. One resolver (#405)

`entityByAlias`'s rule becomes one function both use: ask the alias verb with the reference; a holder wins; else a
valid six-part ID is itself; else not found, with no second request. `alias_ambiguous`, `index_not_ready`, `internal`,
`revision_not_reached` reach the caller as they arrived (ADR-114). The suffix step goes (#405 option 1): a partial ID
such as `sensor-001` resolves only as an alias, and a six-part ID is now asked of the alias verb first (the pin
returned it as itself, `pin:entity_resolver.go:22-24`); both migration notes.

**No entity, one answer.** `entityByAlias` is a lookup and keeps `entity_not_found`. In global search a reference that
names no entity, an unknown alias that is no ID or a valid ID with no record (the entity verb's `entity_not_found`, as
`readPathStart` relays it, `pathsearch.go:78-96`), is not an answer to the question: it falls through to the text
tier, D2's refusal now, step 5's search later, as the pin's unresolved `entity_lookup` and empty `pathrag` fell
through to `graphrag` (`pin:graphrag.go:765-778`). ADR-114's Consequences speak of the lookup; task 4.1 adds one
sentence there, as step 1 amended it: "In global search a reference naming no entity falls through to the text tier
(change `setup-04a-04-graph-roots-graph-query-search`, design D3)." *Proof:* `TestGlobalSearchPassesAliasRefusals`
(each code), `TestGlobalSearchUnknownReferenceFallsThrough`, step 1's `TestEntityByAlias` unchanged; #479 item 3:
`TestZoneQuestionNeverAnswersEmpty` ("the area of the plant", P3, gets D2's refusal).

## D4. Strategies and the reply

Enveloped `graph.GlobalSearchData`: `strategy` and exactly one of `entity` (the entity verb's reply bytes, unchanged),
`path` (`graph.PathSearchData`), `temporal` (`graph.TemporalData`), `spatial` (`graph.SpatialData`). Each is the
explicit result, one hop per edge (#487 note 1); nothing is re-derived or summarised, and no entity is loaded beside
it (a caller wanting states calls `batch`). `path` runs step 2's walk in process from the resolved start, direction
`both`, depth 3 and 100 nodes (the pin's), all predicates (tier 0 never produces `path_predicates`,
`graph/query/README.md:74`); no tier-0 hint names a depth, so none is refused
for depth (a later tier's depth hint over `MaxPathSearchDepth` is `invalid_request`). `temporal` and `spatial` request
`graph.temporal.query.range` and `graph.spatial.query.bounds` with new `graph.TemporalRangeRequest` (`start_time`,
`end_time` as `time.Time`, RFC 3339 on the wire; `limit` 100) and `graph.SpatialBoundsRequest` (`north`, `south`,
`east`, `west`, `limit` 100); their `truncated` passes through. These declare what change 7's responders decode, as
`graph/query_search_types.go:3-6` does for their replies. **Rename for change 7:** the pin sent and its responder
decoded `startTime`, `endTime` (`pin:graphrag.go:1210-1214`, `pin:processor/graph-index-temporal/query.go:49-51`,
`time.Parse` RFC 3339); change 7's port reads `start_time`, `end_time`. Until change 7 both strategies refuse transient
`internal` naming the subject and count one error in `Health`, as step 2's forwards. That cost is accepted rather than
moving them: the responder is another component, possibly another process, so `ask` cannot tell "not in this build"
from "responder down", and a "not in build" code would lie once change 7 deploys; moving them would leave time and
area questions with no honest code at all. `indexed_revision`: the alias reply's (entity), the walk's (path),
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
drives the branch and no test can without a hook (#186). **Departures, recorded:** #428's body asks for a test that
drives a refused plan without a hook, and the repository asks for failing-first tests; neither is possible for the
mark. The mark is one line mirroring the write-failure path (`index.go:442`), shown by reading and named uncovered in
the PR. *Proof:* `TestContractValidStateAlwaysPlans`, a generated check at the bounds (seed recorded) that what the
contract accepts plans. The generator reaches the longest planned key, the 902-byte incoming key (two 256-byte IDs, a
194-byte predicate), and the test asserts it did. Shown able to fail by a mutant only the plan sees: inside
`buildEntityIndexPlan`, refuse a predicate of exactly `vocabulary.MaxPredicateBytes`, which `UnmarshalEntityState`
still accepts. (Tightening the KV limit to 1,023 survives, since 902 fits; `MaxPredicateBytes-1` would make the
contract refuse the state, detecting the generator, not the law.)

## D7. Not carried, ADRs, issues

Not carried: `extractSearchRefinements`, `filterClassifierTemplates` and the semantic strategy with its type-filter
fallback (`pin:graphrag.go:523-571`, `:1068-1191`), reached only from an explicit `strategy` (P1), which tier 0 never
sets; it would refuse until change 5 anyway. Step 5 decides it with the `graphrag` default, a departure from
`split.md:13`'s step 4 list; the
`graphrag` default (step 5); pin `Count`, `DurationMs`, `Degraded*`; strategy names `pathrag`, `entity_lookup` become
`path`, `entity`. ADR-060, -109, -112, -113, -114 applied; ADR-066 for D6; ADR-071's site leaves with the semantic
strategy. Retires #405, #428, #479; #111 stays (step 5's site, item 4). New exports for the owner: `graph.
GlobalSearchRequest`, `GlobalSearchData`, `TemporalRangeRequest`, `SpatialBoundsRequest`; row `globalSearch`.
