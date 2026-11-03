# Design: `processor/graph-query` step 2, traversal, counts and forwards

Base `9d41ea6`; pin `8b99efe9`. Evidence: archived `inventory.md` § 1, `notes/probes/`. Binding: PR #504 comments
6582751320, 6582751783, 6585009466, 6588079803. **Goal.** A read is complete, says it is not, or refuses, at a stated
cost. **Non-goals:** steps 3 to 5; changes 5 and 7's responders; metrics. Pin 917 + 1,643 lines, this about 620 + 1,250
(estimate); one PR, each task one developer's unit.

## D1. The four forwards: enveloped by their responders, decided now

Ruling 1 names no such case: not graph-index, not authority. The pin's are bare, keys mixed (`startTime`, `id`).

| Option | Cost |
| --- | --- |
| (a) Bare, as the pin | `searchGraph` must report the smallest revision it built from (ruling 1); a semantic part with none makes that false from step 5. `similar`, which no composed verb reads (`pin:query.go:701`), follows for one rule across the four. A bare list has no room for #111 item 4's `truncated` |
| (b) No rows until each responder lands | Saves about 90 lines. Steps 3 and 4 read these replies first, so they decide this later anyway. Callers see no responders, class not promised (ADR-112) |
| **(c) Rows now, enveloped by the responder, forwarded byte for byte** | Eight rows, four reply types; changes 5 and 7 meet them, or edit them while nothing has shipped |

**(c),** the shape `byName` has. All four read a derived index. Spatial and temporal watch `ENTITY_STATES` (read).
graph-embedding's low-water `IndexedRevision` (`pin:processor/graph-embedding/readiness.go:53-56,:70`) is half an
answer: it is served on `status`, not `search`, and counts failed and skipped outcomes as terminal, so in an embedder
outage a `min_revision` success could lack a searchable write, against "reflected" in `graph/query_response.go`. What
`indexed_revision` means on `semantic` is change 5's to settle; the row declares the envelope only.

| `graph.query.` | Forwards to | Responder | `Reply` (`data`) |
| --- | --- | --- | --- |
| `semantic` | `graph.embedding.query.search` | `graph-embedding` | `SemanticData{results, embedder_type}` |
| `similar` | `graph.embedding.query.similar` | `graph-embedding` | `SimilarData{results}`, was `similar` |
| `spatial` | `graph.spatial.query.bounds` | `graph-index-spatial` | `SpatialData{results, truncated}` |
| `temporal` | `graph.temporal.query.range` | `graph-index-temporal` | `TemporalData{results, truncated}` |

Rows hold `entity_id` (was `id`) with `similarity`, or `lat`, `lon`, `alt`. Dropped: `query`, `duration`, the constant
`type`. Request keys are the responder's: `start_time`, `end_time` at change 7; `scope` rides through (ADR-071). Until
a responder lands, a request is refused transient `internal`, naming the subject. *Proof:*
`TestForwardPassesBytesBothWays` with `scope`; `TestNoRespondersIsTransient`; `TestGraphQueryRepliesDecodeByTheTable`,
literal JSON on a stub (no real responder yet).

## D2. `pathSearch` request and limits

Decoded strictly, since a typo in `predicates` would widen the answer. Each failure below is `invalid_request` before
any downstream request, naming the field and bound. `min_revision` is optional.

- `start_entity` passes `ValidateEntityID`; each predicate `vocabulary.ParsePredicate` (`AGENTS.md` § 2).
- `direction` is required: `outgoing`, `incoming` or `both`. The pin read a typo and nothing as outgoing (P11).
- `max_depth` is required, 1 to 10: it is the question asked, and a default would hide a cut-off. `max_nodes` is the
  most entities in the reply, the start included: 0 means 100, else 1 to 500. P7 is gone.
- Dropped: `timeout`, since `max_depth`, `max_nodes` and the handler budget bound the walk and a valid value worked at
  the pin (`pin:pathrag.go:100-105`, P10); a walk does outlive a caller who gave up, on a verb that runs serially;
  `include_siblings` (no reader, `pin:pathrag.go:28`), `max_paths` (with one path per entity, it is `max_nodes`).

*Where limits live.* (a) Constants beside `MaxPrefixQueryLimit`. (b) Config `max_traversal_*`: its one setter on record
is SemConnect's file (5, 100), and SemConnect does not call the verb (`inventory.md` § 3). **(a);** a caller wanting
less sends less. *Over a limit.* (a) Refuse. (b) Clamp as `prefix` does: a clamped depth is silent unless every reply
echoes it, and `truncated` stops being exact. **Refuse;** the refusal states the bound.

*Numbers.* 10: the pin's default (`pin:component.go:97`). 100: the pin's node default and step 4's value. 500 is a
judgement: `both` on sparse nodes takes 4.5 to 6.2 s across two runs, hubs take longer and the true bound is the 30 s
budget. No consumer calls the verb, so no timeout decides it. A reply at 500 is at most about 456 KB. Raising later
breaks nobody; lowering does. *Proof:* `TestPathSearchRefusesBadRequests`, a table, no downstream request.

## D3. `pathSearch` reply and walk

(a) The pin's full path per entity: 1,000 paths of 10 steps is 6 MB (P12), with `type` a constant and `score` a
function of depth, and step 4 reads only IDs (`pin:graphrag.go:2590-2593`). (b) **Each entity and edge once.** `data`
is `graph.PathSearchData`: `entities` `[{entity_id, depth}]` in visit order, the start first; `edges`
`[graph.RelationshipEdge]`, where `edges[i]` first reached `entities[i+1]`; `truncated`. A path is the edges followed
back. Enveloped (ruling 1). An edge says what the graph holds; the pin reports an incoming edge backwards (P8).

Breadth first, so `depth` is the shortest distance. It reads `entity` for the start, then at each node below
`max_depth` asks `outgoing`, `incoming` or both, each carrying `min_revision`. Neighbours are filtered, sorted by
entity, predicate, then outgoing before incoming, and deduplicated before the limit (`AGENTS.md` § 2), so a truncated
answer repeats against an unchanged graph. **Any refusal or broken reply ends the walk with that refusal, its message
naming the hop's entity and its class, code and detail kept, never a partial answer.** `truncated` is true exactly when
an unseen entity was found and left out, as read at the hops; the pin says true when nothing was (P9). An edge may name
an entity with no record. `indexed_revision` is the smallest among the hops. *Hubs, declared:* a hop over `max_payload`
refuses the walk whole, as at the pin (`pin:pathrag.go:290`), naming the entity so a caller can find the hub:
`incoming` at 12,000 edges fits, at 13,000 is `response_too_large`, class invalid, and longest IDs reach it near 3,500
(arithmetic). `predicates` filters after the hop. Real trigger, by reading: `enable_hierarchy` (default false) with a
container over about 12,000 members.

*Invariants:* spec, "Path search answers within its limits". Inputs interact, so one generated check. *Proof:*
`TestPathSearchOnARealIndex`, literal JSON, each direction, cycle, filter; `TestPathSearchTruncatedIsExact`;
`TestPathSearchMatchesTheModel` (rapid, the walk against an independent relaxation over a generated graph); stubs only
for `TestPathSearchRevisionIsTheSmallest`, `TestPathSearchRefusedMidWalk`.

## D4. What a request costs

| Verb | Requests | Measured: one client, idle laptop, not a promise |
| --- | --- | --- |
| `pathSearch` | at most 1 + `max_nodes`, doubled for `both` | sparse nodes, two runs: 100: 0.4 s, `both` 0.9 to 1.2 s; 500: 1.8 s, `both` 4.5 to 6.2 s |
| `hierarchyStats` | one page: one key scan of the group, up to 1,000 bodies | 20 ms at 10,000; 157 ms at 100,000; 1.8 to 2.7 s at 1,000,000 |
| `summary` | `predicateList`, then one page of everything; or, with `prefix`, one page of that group alone | a scan of `PREDICATE_INDEX`, 2.8 s at 1M keys, 8 s at 3M to 5M, one run, against graph-index's 10 s `listTimeout`; plus the page. Roughly 4 to 10 s at a million entities, near refusal; `exclude_predicates` is the way out |

A request ends with the handler's 30 s budget, refused transient (`natsclient/request.go:32-40`). A reply over
`max_payload` is `response_too_large` (`natsclient/request_response_bounds_integration_test.go:141`). natsclient reads
nats.go as one handler at a time per subscription (`owned_delivery.go:21-22`), so a slow walk would delay the next
`pathSearch`: not promised here. What is: another verb still answers. *Proof:* counting subscribers beside real
responders, `TestPathSearchRequestsAreBounded`, `TestCountsReadOnePage`; `TestSlowPathSearchLeavesOtherVerbsAnswering`,
a held stub the test releases.

## D5. Counts: one page, declared (Q7(a), Fable ruling 2)

**One page per request, as accepted, plus (f) a true total and (g) `prefix` on `summary` limited to entities.** Each
verb reads one page at the maximum limit, IDs only, passing a request `cursor` unchanged. `total_entities` is the
page's `total`: the length of the key listing, the whole group as listed, before cursor and limit. A listing under
concurrent overwrites can repeat some keys and omit as many (`natsclient/kv.go:648-693`, the reviewer's probe at
nats.go v1.55.0; #512), so `total` keeps its length and a page can repeat one entity and miss another.
`entities_scanned` is the rows read, and type and child counts are a sample of the lexically smallest IDs; `truncated`
is true exactly when `next_cursor` is set. A caller wanting the rest sends the cursor, paying one scan per page.
`entity_sample_limit` goes: its default 2000 exceeds the page, and the scan is the same. One law on literal seeds, no
interacting inputs: examples suffice.

*(g).* `predicateList` takes no entity prefix (`graph/query_predicate_types.go:22-25`), so a prefix with predicates
would set one group's types beside the whole graph's predicates. As asked that is a poor shape, and it costs the full
`PREDICATE_INDEX` scan too. **So `prefix` without `exclude_predicates: true` is `invalid_request`, naming both.** One
scope, one group's scan, and its `indexed_revision` is 0 (D7). *Others rejected:* a bounded walk or a walk to the end
(36 to 53 s at 100,000); `COMMUNITY_INDEX`, Q7(b), after change 7; an ID-count verb on graph-ingest,
not yet, the owner's answer in `owner-questions.md`, as for (f) and (g).

`hierarchyStats` takes `prefix` (validated unless empty), `cursor`, `min_revision`; children are `{prefix, count}`
(`name` repeats the prefix's last segment). An entity whose ID is the prefix counts in `entities_scanned` and no child.
`summary` takes `prefix`, `exclude_predicates`, `examples_per_type` (0 means 2; negative refused), `cursor`,
`min_revision`; type keys keep ADR-102's order. Both decode strictly. A page with no `entities` key (real ingest sends
`null` for an empty group), or an ID that does not parse, is transient `internal`; the pin skipped such IDs. *Proof:*
`TestPrefixPageNamesItsTotal` in graph-ingest, each path, a byte-trimmed page and a later page;
`TestHierarchyStatsDeclaresAPartialPage`, real ingest, literal JSON; `TestCountsAddUpAcrossPages`, against the seeded
literals; `TestCountsOfAnEmptyGroup`; `TestCountsRefuseABrokenPage` (stub); `TestSummaryPrefixReadsOneGroup`, two
groups, literal JSON; `TestSummaryRefusesPrefixWithPredicates`.

## D6. Predicates: refuse, or leave out when asked (#111 item 2, #105)

(a) A `predicates_unavailable` flag: a caller who does not read it sees no predicates, the defect again. (b)
**Refuse:** the index's refusal reaches the caller unchanged; `exclude_predicates: true` gets entities alone. It fails
closed (`AGENTS.md` § 1). `predicates` is `null` when excluded and `[]` when there are none; `omitempty` and
`predicate_total` go. #105 offers inverting: the zero value is the default, with no pointer. `predicateList` runs
first, so a refusal costs no scan and the page, read after, reflects at least as much (ruling 1). A reply that does not
decode or names no `producer` is transient `internal`, the case #111 item 2 names. *Proof:* `TestSummaryPredicates`,
literal JSON on a real index: some, none, excluded; `TestSummaryRefusesWhenPredicatesCannot`, class, code and detail
intact, no prefix request; `TestSummaryRefusesABrokenPredicateReply` (stub).

## D7. An answer that read no index

Ruling 1's list envelopes `hierarchyStats`, while its headline says bare where the authority answers. Both it and
`summary` with predicates excluded read graph-ingest's live listing after the request, which holds every committed
write, and the page cut is declared by `truncated`, so neither is a wrong success, even with `min_revision` far ahead.
But callers are told to compare `indexed_revision` (`graph/query_response.go:10-13`, `query-pattern`), and against 0
the alarm never clears: a retry loop spins for ever. **`hierarchyStats` is bare**, so it has no such field to misread
(owner, 2026-11-03, narrowing ruling 1's list). **`summary` keeps the envelope**, since it reads the predicate index
unless asked not to, and with predicates excluded its `indexed_revision` is 0, claiming nothing. Only the doc warns
there: it must say not to compare on that answer. *Proof:* `TestHierarchyStatsIgnoresMinRevision`;
`TestAuthorityOnlyAnswerClaimsNoRevision`.

## Scope

Retires no issue. Shape findings, issues, ADRs, overlap and guidance: `inventory-delta.md`.
