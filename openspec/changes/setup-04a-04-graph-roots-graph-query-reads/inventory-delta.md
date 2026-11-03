# Inventory delta: setup-04a-04-graph-roots-graph-query-reads

base: `9d41ea6`. Adds to the archived `inventory.md`, reviewed on PR #504; repeats nothing it settles. Pins are at base
unless marked `pin:` (`8b99efe9`).

## Surface: what landed, and what step 2 meets

- `processor/graph-query/query.go:24` — `func (c *Component) queryHandlers() map[string]queryHandler {`:
  `forwardedFrom` finds a row by the verb's own name, so `semantic` to `search` needs the downstream name too.
- `graph/query_summary_types.go:21` — `type SummaryRequest struct {`: at base `IncludePredicates bool`, a default an
  omitted field cannot express (#105 comment).
- `graph/query_summary_types.go:76` — `type SummaryData struct {`: `Predicates` has `omitempty`, so `[]` and absent
  match.
- `graph/query_predicate_types.go:22` — `type PredicateListQuery struct {`: embeds `MinRevisionField`, strict decoder,
  so `summary` sends `{"min_revision":M}`.
- `natsclient/request.go:40` — `const DefaultRequestHandlerTimeout = 30 * time.Second`.
- `natsclient/owned_delivery.go:21` — `// The native library delivers on one goroutine per subscription, so at most one
  invocation runs`: natsclient's reading, not probed here; the design promises only that another verb still answers.
- `natsclient/kv_key_contract.go:43` — `ErrorCodeResponseTooLarge = "response_too_large"`, sent by
  `natsclient/request.go`'s `errors.Is(respErr, nats.ErrMaxPayload)` branch, driven by
  `natsclient/request_response_bounds_integration_test.go:141`.
- `processor/graph-ingest/query.go:280` — `keys, err = c.entityBucket.KeysByPrefix(ctx, prefixDot)`: every page lists
  every key under the prefix.
- `processor/graph-ingest/query.go:468` — `if len(ids) == 0 {`, returning nil: an empty group's page marshals
  `"entities":null` (read, not run; task 4.1's `TestCountsOfAnEmptyGroup` runs it), so `{}` differs only by the key.
- `processor/graph-query/config_test.go:42` — `{"max_traversal_depth", 5}`: stays refused, as do the spec scenario and
  `MIGRATION.md`'s instruction to delete both keys.
- `vocabulary/predicates.go:365` — `func IsValidPredicate(predicate string) bool {`; `graph` already imports
  `vocabulary` (`go list -deps ./graph`: 1 match).
- Searches closed empty: `git grep -n 'graph\.embedding\.query\|graph\.spatial\.query\|graph\.temporal\.query' --
  ':!openspec' ':!docs'`: no line, so the new rows add no reserved-subject violation at base.

## Pin facts added

P8-P12 and the hop costs: `notes/probes/`. Reads, not runs: spatial and temporal watch `ENTITY_STATES`
(`pin:processor/graph-index-spatial/component.go:123`, `pin:processor/graph-index-temporal/component.go:125`);
graph-embedding's watermark (`pin:processor/graph-embedding/readiness.go:53,:70`); `TemporalResult{ID: event.Entity,
Type: "entity"}` (`pin:processor/graph-index-temporal/query.go:144`); fusion reads `results[].entity_id`, `similarity`
(`pin:pkg/fusion/fusionnats/client.go:328-331`); `include_siblings` has no reader (`git grep` on the pin copy: only
`pathrag.go:28`); `.Score` hits in `graphrag.go` are semantic hits, none a `PathEntity`. SemConnect sets
`max_traversal_depth` 5, `max_traversal_nodes` 100 (`deploy/semstreams.json:114-115`,
`conformance/compose.semstreams.config.json:114-115` at `d0d06e00`).

Measured beside this work: the index applied about 33 to 37 entities a second. It refuses correctly until caught up, so
no defect and nothing to file. Cause, per the reviewer's re-probe: the one-lane default, `workers` 1 as at the pin
(`pin:processor/graph-index/component.go:134,:201`), four listings per apply by reading; 8 workers caught 3,000 about 4
s after a 4.8 s seed. The rate goes on the rebuild-cost issue (coordinator).

## Open pull requests

`gh pr list --state open`, 2026-11-03: #511 (this claim, no files yet); #510 draft, `fix(graph-index): Stop begins
every query drain`, six files, none this change edits. No overlap: this edits no graph-index file and reads
`predicateList`, `outgoing`, `incoming` as they are. Either merges first. No PR has over 100 files.

## Open issues

Searches, each `gh issue list --state open --search <term>`: `pathSearch`, `hierarchyStats`, `summary`, `graph-query`,
`traversal`, `max_traversal`, `spatial`, `temporal`, `semantic`, `similar`, `embedding`, `IncludePredicates`.

| Issue | Disposition, as Bounded port scope classes it |
| --- | --- |
| #105 | Stays, folded into #32 (owner, 2026-10-24). Its one live item, required by the accepted comment, done by task 4.2 |
| #111 | Stays. Item 2, required (`AGENTS.md` § 1), done by task 4.2; item 3 steps 4, 5; item 4 change 7, its `truncated` key fixed by D1 |
| #106 | Stays; retires when step 5 adds the last rows. This adds rows |
| #103 | Stays; step 5, marker with #15 |
| #16 | Stays, no code here; this widens what the reserved list covers |
| #109 | Stays; names the followers change 7 ports, which must meet D1 |
| #68 | Stays; no collector here, so no site |
| #405, #406, #31, #479, #477 | Stay; steps 3, 4 (`split.md`) |
| #485, #486, #487, #484, #32, #155 | Stay; tier 2 and provider paths |
| #10, #11, #489 | Stay; BM25 and neural retrieval, which change 5 onward serves behind `semantic` |
| #506 | Stays; keyword index, not this port |
| #508 | Stays; graph-index Stop, PR #510 |
| #9, #500 | Addressed, govern |

Names the word, not the package: #7, #24, #108, #110, #134, #141, #195, #196, #198, #300, #350, #408, #482.

## Shape findings

| # | Disposition |
| --- | --- |
| S9 | Required (`graph-transport-boundary`, verify): the four subjects enter the table, D1 |
| S11 | Required (`AGENTS.md` § 1, #111 item 2, P4): D6; malformed request refused, `pin:summary.go:140` |
| S12 | Required (`AGENTS.md` § 1, P4): D5 |
| S18 | Required for validators (`AGENTS.md` § 2) and limits (owner, D, PR #504 comment 6585009466): D2 |
| S26 | Optional; the `_ := json.Marshal` sites are not carried |

## Who each new export is for

| Export | Reader |
| --- | --- |
| `Responder*`, `SemanticData`, `SimilarData`, `SpatialData`, `TemporalData` and their rows | The table: graph-query's handlers now; step 3 and 4 strategies; responders of changes 5, 7; contract line `graph-transport-boundary` |
| `PathSearchRequest`, `PathSearchData`, `PathEntity`, limits | graph-query's handler; step 4's path strategy; products, contract line `graph-query`, "Path search" with a test calling it as a product |
| `HierarchyStats*`, `HierarchyChild`, changed `Summary*` | graph-query's handlers; products, contract line `graph-query`, "Counts" |
| `PrefixQueryResponse.Total` | graph-query's two count verbs (task 4.2); contract line `graph-transport-boundary`, "A prefix page names its total" |
| `SummaryRequest.Prefix` | graph-query's handler; no consumer today; products, contract line `graph-query`, "A summary has its predicates", with `TestSummaryPrefixReadsOneGroup` calling it as a product |
| New `graph.query.*` subjects | Whoever keeps the GraphQL gateway (`MIGRATION.md`), fusion in change 5 (`semantic`) |

Dropped from the pin's exports, each with no reader outside the package but a test (`inventory.md` § 3):
`DirectionOutgoing`, `DirectionIncoming`, `DirectionBoth`, `PathSearcher`, `NewPathSearcher`, `PathSearchResponse`,
`PathStep`, `RelationshipEntry`. Problem shape: a bounded read with a declared continuation; closest instance
`graph/query_prefix_types.go:5` (`MaxPrefixQueryLimit`), adopted for counts. No new primitive for reuse, so no adoption
list.

## Adopter seam

**1. What must they know?** Decode by the table. Read `truncated` on `pathSearch` and the two count verbs. Two facts;
the limits and required fields are not on the list, since a refusal teaches them. `indexed_revision` 0 is a third for
anyone comparing revisions there, a finding: the alarm never clears and a retry loop spins for ever. Owner question 2.

**2. If they do nothing?** Sending nothing but a start: refused, naming `direction`, then `max_depth`. Calling
`semantic` before change 5: transient refusal naming the subject. Ignoring `truncated` on a count: reads a sample as
the whole, though the key says `entities_scanned`. `total_entities` beside it shows the gap, and owner question 1
weighs closing it. A walk through a hub over about 12,000 edges (near 3,500 at longest IDs): `response_too_large`,
whole, naming the entity, as at the pin (`pin:pathrag.go:290`); `predicates` cannot help. A pin caller sending
`include_predicates` or `timeout`: refused, naming the key. A pin decoder of `paths`: empty, silently; tasks 3.1 and
4.x list these under `MIGRATION.md`'s "The changes that fail silently".

**3. Where do they find out?** Bad request, limit: typed runtime error. Partial answer: a field in every reply. Down
index: typed runtime error.

**4. What should they have to know?** Which verb to call. **Gaps:** `truncated` on counts, which only exact counts
remove; the hub limit, which only a paged `incoming` removes. **Prefer observation:** the caller predicts no limit, no
readiness and no index position; it asks and reads the refusal.

## ADRs, guidance and overlap

ADR-071 ported with a note. ADR-060, ADR-099, ADR-102 (type key order), ADR-109 applied. `query-pattern` grows. Step 3
also edits `graph/query_verbs.go`; the later merges `origin/main`. Consumers: `processor/graph-query/MIGRATION.md`.

## Intent check

No boundary moves: path search and counts are admitted (#9 comment 6386856657); the four forwards are pin surface in an
admitted package, with responders in changes 5 and 7 (#9 D2). Inventory § 11's table stands.
