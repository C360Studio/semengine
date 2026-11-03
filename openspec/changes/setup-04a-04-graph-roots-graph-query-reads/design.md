# Design: `processor/graph-query` step 2, traversal, counts and forwards

Base `9d41ea6`; pin `8b99efe9`. P0-P7, prefix costs: archived `inventory.md` § 1; P8-P12, hop costs: `notes/probes/`.
Binding: PR #504 comments 6582751320, 6582751783, 6585009466, 6588079803. Adopter seam: `inventory-delta.md`. **Goal.**
A read is complete, says it is not, or refuses, at a stated cost. **Non-goals:** steps 3 to 5; the responders of
changes 5 and 7; metrics (no reader here). Pin 917 + 1,643 lines, this about 600 + 1,200 (estimate). One PR: three
handlers on one shell, each task one developer's unit.

## D1. The four forwards: enveloped by their responders, decided now

Ruling 1 names no such case: not graph-index, not authority. The pin's are bare, keys mixed (`startTime`, `id`).

| Option | Cost |
| --- | --- |
| (a) Bare, as the pin | `searchGraph` must report the smallest revision it built from (ruling 1); a semantic part with none makes that false from step 5. A bare list has no room for #111 item 4's `truncated` |
| (b) No rows until each responder lands | Saves about 90 lines. Steps 3 and 4 read these replies before changes 5 and 7, so they decide the same thing later. Callers meanwhile see no responders, class not promised (ADR-112), not a classified refusal |
| **(c) Rows now, enveloped by the responder, forwarded byte for byte** | Eight rows, four reply types; changes 5 and 7 meet them, or edit them while nothing has shipped |

**(c),** the shape `byName` has. All four read a derived index, and the pin's responders can say how far: spatial and
temporal watch `ENTITY_STATES`; graph-embedding keeps a low-water `IndexedRevision` (read:
`pin:processor/graph-embedding/readiness.go:53,:70`). Each will answer `min_revision` or refuse it.

| `graph.query.` | Forwards to | Responder | `Reply` (`data`) |
| --- | --- | --- | --- |
| `semantic` | `graph.embedding.query.search` | `graph-embedding` | `SemanticData{results, embedder_type}` |
| `similar` | `graph.embedding.query.similar` | `graph-embedding` | `SimilarData{results}`, was `similar` |
| `spatial` | `graph.spatial.query.bounds` | `graph-index-spatial` | `SpatialData{results, truncated}` |
| `temporal` | `graph.temporal.query.range` | `graph-index-temporal` | `TemporalData{results, truncated}` |

Rows hold `entity_id` (was `id`) with `similarity`, or `lat`, `lon`, `alt`. Dropped: `query`, `duration`, the constant
`type` (`pin:graph-index-temporal/query.go:144`). Request keys are the responder's: `start_time`, `end_time` from
change 7 (`MIGRATION.md`); `scope` rides through (ADR-071). Until a responder lands, a request is refused transient
`internal`, naming the subject. *Proof:* `TestForwardPassesBytesBothWays` with `scope`; `TestNoRespondersIsTransient`;
`TestGraphQueryRepliesDecodeByTheTable`, literal JSON on a stub (no real responder yet).

## D2. `pathSearch` request and limits

Decoded strictly, since a typo in `predicates` would widen the answer. Each failure below is `invalid_request` before
any downstream request, naming the field and bound. `min_revision` is optional.

- `start_entity` passes `ValidateEntityID`; each predicate `vocabulary.ParsePredicate` (`AGENTS.md` § 2).
- `direction` is required: `outgoing`, `incoming` or `both`. The pin read a typo and nothing as outgoing (P11).
- `max_depth` is required, 1 to 10: it is the question asked, and a default would hide a cut-off. `max_nodes` is the
  most entities in the reply, the start included: 0 means 100, else 1 to 500. P7 is gone.
- Dropped: `timeout` (a bad value was ignored, P10), `include_siblings` (no reader, `pin:pathrag.go:28`), `max_paths`
  (with one path per entity, it is `max_nodes`).

*Where limits live.* (a) Constants beside `MaxPrefixQueryLimit`. (b) Config `max_traversal_*`: its one setter on record
is SemConnect's file (5, 100), and SemConnect does not call the verb (`inventory.md` § 3). **(a);** a caller wanting
less sends less. *Over a limit.* (a) Refuse. (b) Clamp as `prefix` does: a clamped depth is silent unless every reply
echoes it, and `truncated` stops being exact. **Refuse;** the refusal states the bound.

*Numbers.* 10: the pin's default (`pin:component.go:97`). 100: the pin's node default and step 4's value. 500: worst
case `both` is 1,001 requests, about 6.2 s; 1,000 would be 12 s, past both consumers' default timeouts (5 s, 10 s). A
reply at 500 is at most about 456 KB. Raising later breaks nobody; lowering does. *Proof:*
`TestPathSearchRefusesBadRequests`, a table, no downstream request.

## D3. `pathSearch` reply and walk

(a) The pin's full path per entity: 1,000 paths of 10 steps is 6 MB (P12), and `type` is a constant, `score` a function
of depth. (b) **Each entity and edge once.** `data` is `graph.PathSearchData`: `entities` `[{entity_id, depth}]` in
visit order, the start first; `edges` `[graph.RelationshipEdge]`, the edge that first reached each other entity;
`truncated`. A path is the edges followed back. Enveloped (ruling 1). An edge says what the graph holds; the pin
reports an incoming edge backwards (P8). Step 4 reads only IDs (`pin:graphrag.go:2590-2593`).

Breadth first, so `depth` is the shortest distance. It reads `entity` for the start, then at each node below
`max_depth` asks `outgoing`, `incoming` or both, each carrying `min_revision`. Neighbours are filtered, sorted by
entity and predicate, and deduplicated before the limit (`AGENTS.md` § 2), so a truncated answer repeats. **Any refusal
or broken reply ends the walk with that refusal, never a partial answer.** `truncated` is true exactly when an unseen
entity was found and left out; the pin says true when nothing was (P9). An edge may name an entity with no record.
`indexed_revision` is the smallest among the hops.

*Invariants* (spec, "Path search answers within its limits"): at most `max_nodes` entities, each once, none past
`max_depth`; one edge per entity past the start, joining it to an earlier one; when `truncated` is false, `entities` is
every entity within `max_depth`. Inputs interact, so one generated check. *Proof:* `TestPathSearchOnARealIndex`,
literal JSON, each direction, cycle, filter; `TestPathSearchTruncatedIsExact`; `TestPathSearchMatchesTheModel` (rapid,
the walk against an independent relaxation over a generated graph); stubs only for
`TestPathSearchRevisionIsTheSmallest`, `TestPathSearchRefusedMidWalk`.

## D4. What a request costs

| Verb | Requests | Measured: one client, idle laptop, not a promise |
| --- | --- | --- |
| `pathSearch` | at most 1 + `max_nodes`, doubled for `both` | 100: 0.4 s, `both` 1.2 s; 500: 1.9 s, `both` 6.2 s |
| `hierarchyStats` | one page: one key scan of the group, up to 1,000 bodies | 20 ms at 10,000; 157 ms at 100,000; 1.8 to 2.7 s at 1,000,000 |
| `summary` | `predicateList`, then one page of everything | a scan of `PREDICATE_INDEX` (45 ms at 3,000 entities); 1.43 s at 1,000,000 entities |

A request ends with the handler's 30 s budget, refused transient (`natsclient/request.go:32-40`). A reply over
`max_payload` is `response_too_large` (`natsclient/request_response_bounds_integration_test.go:141`). natsclient reads
nats.go as one handler at a time per subscription (`owned_delivery.go:21-22`), so a slow walk would delay the next
`pathSearch`: not promised here. What is: another verb still answers. *Proof:* counting subscribers beside real
responders, `TestPathSearchRequestsAreBounded`, `TestCountsReadOnePage`; `TestSlowPathSearchLeavesOtherVerbsAnswering`,
a held stub the test releases.

## D5. Counts: one page, declared (Q7(a), Fable ruling 2)

(a) One page per request, as accepted. (b) A bounded walk: more scans, same bias. (c) To the end: 36 to 53 s at
100,000. (d) `COMMUNITY_INDEX`: Q7(b), after change 7. (e) A new ID-count verb on graph-ingest: `owner-questions.md`.
**(a).** Each verb reads one page at the maximum limit, IDs only, passing a request `cursor` unchanged. The sample is
the group's lexically smallest IDs; the doc says so. `entities_scanned` replaces `total_entities`/`totalEntities`,
which was no total; `truncated` is true exactly when `next_cursor` is set. A caller wanting the rest sends the cursor,
paying one scan per page. `entity_sample_limit` goes: its default 2000 exceeds the page, and the scan is the same.

`hierarchyStats` takes `prefix` (validated unless empty), `cursor`, `min_revision`; children are `{prefix, count}`
(`name` repeats the prefix's last segment). An entity whose ID is the prefix counts in `entities_scanned` and no child.
`summary` takes `exclude_predicates`, `examples_per_type` (0 means 2; negative refused), `cursor`, `min_revision`; type
keys keep ADR-102's order. Both decode strictly. A page with no `entities` key (real ingest sends `null` for an empty
group), or an ID that does not parse, is transient `internal`; the pin skipped such IDs. *Proof:*
`TestHierarchyStatsDeclaresAPartialPage`, real ingest, literal JSON; `TestCountsAddUpAcrossPages`, against the seeded
literals; `TestCountsOfAnEmptyGroup`; `TestCountsRefuseABrokenPage` (stub).

## D6. Predicates: refuse, or leave out when asked (#111 item 2, #105)

(a) A `predicates_unavailable` flag: a caller who does not read it sees no predicates, the defect again. (b)
**Refuse:** the index's refusal reaches the caller unchanged; `exclude_predicates: true` gets entities alone. It fails
closed (`AGENTS.md` § 1). `predicates` is `null` when excluded and `[]` when there are none; `omitempty` and
`predicate_total` go. #105 offers inverting: the zero value is the default, with no pointer. `predicateList` runs
first, so a refusal costs no scan and the page, read after, reflects at least as much (ruling 1). *Proof:*
`TestSummaryPredicates`, literal JSON on a real index: some, none, excluded; `TestSummaryRefusesWhenPredicatesCannot`,
class, code and detail intact, no prefix request.

## D7. An answer that read no index

Ruling 1 envelopes `hierarchyStats`, and `summary` with predicates excluded, yet they read no graph-index reply, so
"the smallest" has no value. **`indexed_revision` is 0, claiming nothing: what was read is the authority,** so
`min_revision` is met by construction, as on a bare verb. This narrows "a success has `indexed_revision` at or above M"
(spec delta). A caller comparing revisions here gets a false alarm, never a false success. *Proof:*
`TestAuthorityOnlyAnswerClaimsNoRevision`.

## Scope

**Shape findings:** required, done: S9 (D1), S11, S12 (D5, D6), S18 (D2). Optional, gone at no cost: S26. **Issues:**
retires none. Stay: #105 (its item done; folded into #32), #111 (item 2 here; 3 steps 4, 5; 4 change 7, its key in
D1), #106 (last rows), #103 (step 5), #16 (widened), #109 (change 7's followers), #68 (no collector here). The rest:
`inventory-delta.md`. **ADRs:** ADR-071 ported with a note; ADR-060, -099, -102, -109 applied. **Overlap:** #510 edits
graph-index drains, this no graph-index code: either merges first. Step 3 also edits `graph/query_verbs.go`; the later
merges `origin/main`. **Guidance:** `query-pattern` grows. Consumers: `MIGRATION.md`.
