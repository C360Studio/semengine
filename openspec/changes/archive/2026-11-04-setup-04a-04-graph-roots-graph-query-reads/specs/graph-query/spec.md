# graph-query

## ADDED Requirements

### Requirement: Derived-index replies are forwarded unchanged

`semantic`, `similar`, `spatial` and `temporal` SHALL forward the request bytes to the verb their table row names and
return the reply bytes unchanged. The table row declares what changes 5 and 7 must send; no responder serves these yet.

#### Scenario: A field graph-query does not read rides through

- **WHEN** a caller of `semantic` sends a request holding `scope`
- **THEN** `graph.embedding.query.search` receives exactly those bytes and the caller receives exactly its reply
  (`TestForwardPassesBytesBothWays`)

#### Scenario: A responder not yet deployed

- **WHEN** a caller of one of the four asks and nothing serves the verb behind it
- **THEN** the caller receives a transient `internal` refusal naming that subject (`TestNoRespondersIsTransient`)

### Requirement: Path search refuses a bad request before any read

`pathSearch` SHALL decode its request strictly. It SHALL refuse with `invalid_request`, before any downstream request
and naming the field: an unknown key; a `start_entity` that is not a valid entity ID; a `predicates` entry that is not
a canonical predicate; a `direction` other than `outgoing`, `incoming` or `both`, or none; a `max_depth` outside 1 to
10, or none; a `max_nodes` below 0 or above 500. A `max_nodes` of 0 SHALL mean 100.

#### Scenario: Numbers the pin accepted

- **WHEN** a request sends `max_depth` -5, or `max_nodes` 2000000000
- **THEN** the caller receives `invalid_request` naming the field and its bound, and no downstream verb receives a
  request (`TestPathSearchRefusesBadRequests`)

### Requirement: Path search answers within its limits

A success SHALL be a `graph.QueryResponse` from producer `graph-query` whose `data` is a `graph.PathSearchData`.
`entities` SHALL hold at most `max_nodes` rows, each entity once, the start first at depth 0, each `depth` the shortest
distance and at most `max_depth`. `edges[i]` SHALL be the edge that first reached `entities[i+1]`, joining it to an
earlier entity. Neighbours SHALL be ordered by entity, predicate, then outgoing before incoming, and deduplicated
before the limit.

#### Scenario: Generated graphs

- **WHEN** a generated graph and request are walked
- **THEN** the reply holds the rules above and matches an independent model (`TestPathSearchMatchesTheModel`)

### Requirement: Path search edges say what the graph holds

Each row of `edges` SHALL name the edge in the direction the graph holds it, whichever way the walk crossed it.

#### Scenario: An incoming edge

- **WHEN** a real index holds B -p-> A and a caller asks from A with `direction` `incoming`
- **THEN** `data` compares equal to the literal JSON with entities A at 0 and B at 1, and the edge `from_entity_id` B,
  `to_entity_id` A (`TestPathSearchOnARealIndex`)

### Requirement: Path search truncation is exact

`truncated` SHALL be true exactly when the walk, as read at the hops, found an unseen entity and left it out. When it
is false, `entities` SHALL be every entity within `max_depth` under `direction` and `predicates`.

#### Scenario: The limit meets the graph exactly

- **WHEN** a graph holds exactly `max_nodes` entities reachable from the start
- **THEN** every one is in `entities` and `truncated` is false
- **AND** with `max_nodes` one lower, `truncated` is true (`TestPathSearchTruncatedIsExact`)

### Requirement: A failed read ends a path search

`pathSearch` SHALL read the start through the entity verb first. Any refusal or broken reply, on any hop, SHALL end the
walk: a refusal reaches the caller with class, code and detail, its message naming the hop's entity, a broken reply is
transient `internal`, and no partial answer is returned. Every hop SHALL carry the request's `min_revision`.

#### Scenario: A start that does not exist

- **WHEN** a request names an entity with no record
- **THEN** the caller receives the entity verb's `entity_not_found` (`TestPathSearchOnARealIndex`)

#### Scenario: A hub over the reply limit

- **WHEN** a hop's reply is too large for the connection
- **THEN** the caller receives that `response_too_large` refusal, whole, naming the entity
  (`TestPathSearchRefusedMidWalk`)

#### Scenario: A hop refused mid-walk

- **WHEN** the index refuses `index_not_ready` on a later hop
- **THEN** the caller receives that refusal, not the entities found so far (`TestPathSearchRefusedMidWalk`)

### Requirement: A request's cost is bounded

`pathSearch` SHALL make at most 1 + `max_nodes` downstream requests, or 1 + 2 × `max_nodes` for `both`.
`hierarchyStats` SHALL make exactly one prefix request, and `summary` one prefix request and, unless predicates are
excluded, one `predicateList` request first. A long request on one verb SHALL NOT stop another verb answering.

#### Scenario: Counting the requests

- **WHEN** counting subscribers sit beside the real responders
- **THEN** the counts are within those bounds (`TestPathSearchRequestsAreBounded`, `TestCountsReadOnePage`)

#### Scenario: A walk that is held

- **WHEN** a `pathSearch` waits on a responder that does not answer
- **THEN** a request on `entity` is answered (`TestSlowPathSearchLeavesOtherVerbsAnswering`)

### Requirement: Counts declare a partial page

`hierarchyStats` and `summary` SHALL read one prefix page, IDs only, sending the request's `cursor` unchanged, and
SHALL decode their request strictly. `total_entities` SHALL be the page's `total`, the group as listed;
`entities_scanned` the rows on that page; `next_cursor` the page's; and `truncated` SHALL be true exactly when
`next_cursor` is set. A page with no `entities` or `total` key, an ID that does not parse, or a row outside the prefix,
SHALL be refused transient `internal`.

#### Scenario: A group larger than a page

- **WHEN** a group holds more entities than one page
- **THEN** `truncated` is true and `next_cursor` is set (`TestHierarchyStatsDeclaresAPartialPage`)
- **AND** `total_entities` is the number seeded, and a caller following `next_cursor` to the end sums to it
  (`TestCountsAddUpAcrossPages`)

#### Scenario: An empty group

- **WHEN** a prefix holds no entity
- **THEN** `total_entities` and `entities_scanned` are 0, `truncated` false and the lists empty, not an error
  (`TestCountsOfAnEmptyGroup`)

#### Scenario: A broken page

- **WHEN** the prefix verb replies `{}`, or a row whose ID does not parse
- **THEN** the caller receives transient `internal` (`TestCountsRefuseABrokenPage`)

### Requirement: Hierarchy counts name each child

`hierarchyStats` SHALL take `prefix`, `cursor` and `min_revision`, refusing an invalid non-empty prefix with the
validator's refusal. The reply SHALL be a bare `graph.HierarchyStatsData`: `prefix`, `total_entities`,
`entities_scanned`, `truncated`, `next_cursor` when set, and `children`, each `{prefix, count}`, by count descending
then prefix. An entity whose ID is the prefix SHALL count in `entities_scanned` and in no child.

#### Scenario: Keys on the wire

- **WHEN** a real graph-ingest holds entities under two children of a prefix shorter than an entity ID
- **THEN** the reply compares equal to the literal JSON (`TestHierarchyStatsDeclaresAPartialPage`)

#### Scenario: A prefix that is an entity's ID

- **WHEN** the prefix is the six-part ID of an entity a real graph-ingest holds
- **THEN** `total_entities` and `entities_scanned` are 1 and `children` is empty (`TestHierarchyStatsOfAnExactID`)

### Requirement: A summary has its predicates or says why not

`summary` SHALL take `prefix`, `exclude_predicates`, `examples_per_type`, `cursor` and `min_revision`; a negative
`examples_per_type` SHALL be `invalid_request`, and 0 SHALL mean 2. `data` SHALL be `graph.SummaryData`. Unless
excluded, `summary` SHALL ask `predicateList` first and return its refusal unchanged, without reading a page. A
`prefix` without `exclude_predicates` SHALL be `invalid_request` naming both. `predicates` SHALL be `null` when
excluded and `[]` when there are none.

#### Scenario: One group

- **WHEN** two groups hold entities and a caller sends the first's `prefix` with `exclude_predicates`
- **THEN** `data` compares equal to the literal JSON of that group alone (`TestSummaryPrefixReadsOneGroup`)

#### Scenario: A prefix with predicates

- **WHEN** a caller sends `prefix` without `exclude_predicates`
- **THEN** the caller receives `invalid_request` and no downstream verb a request
  (`TestSummaryRefusesPrefixWithPredicates`)

#### Scenario: The three states of predicates

- **WHEN** a real index holds predicates, or none, or the caller excludes them
- **THEN** `data` compares equal to the literal JSON for each (`TestSummaryPredicates`)

#### Scenario: An index that cannot answer

- **WHEN** `predicateList` refuses
- **THEN** the caller receives that refusal with class, code and detail, and the prefix verb no request
  (`TestSummaryRefusesWhenPredicatesCannot`)

#### Scenario: A broken predicate reply

- **WHEN** `predicateList` replies with bytes that do not decode or name no `producer`
- **THEN** the caller receives transient `internal`, not an empty list (`TestSummaryRefusesABrokenPredicateReply`)

## MODIFIED Requirements

### Requirement: Revision requests answer or refuse, never wrongly succeed

Every verb SHALL accept `min_revision`. On an enveloped verb it SHALL reach every derived-index read unchanged; on a
bare verb, and on an answer built from no derived-index read, it SHALL be ignored, an authority read having no index to
catch up. A success built from a derived-index read to `min_revision` M SHALL carry `indexed_revision` at or
above M.

#### Scenario: Reading a write back

- **WHEN** a caller writes entity E, receives `kv_revision` W, and requests `relationships` for E with `min_revision` W
- **THEN** it receives a reply with `indexed_revision` at or above W reflecting the write, or a `revision_not_reached`
  refusal
- **AND** never a success with `indexed_revision` below W (`TestReadYourWrites`)

#### Scenario: A revision far ahead

- **WHEN** `byName`, `relationships` in either direction, `entityByAlias`, `pathSearch` or `summary` carries a
  `min_revision` far above the index's revision
- **THEN** the caller receives `revision_not_reached` with its class and detail
  (`TestMinRevisionRefusalThroughGraphQuery`)

#### Scenario: An answer that read only the authority

- **WHEN** `hierarchyStats` carries a `min_revision` far above any revision
- **THEN** it answers bare (`TestHierarchyStatsIgnoresMinRevision`)

#### Scenario: A summary that read only the authority

- **WHEN** `summary` with `exclude_predicates` carries a `min_revision` far above any revision
- **THEN** it answers, with `indexed_revision` 0 (`TestAuthorityOnlyAnswerClaimsNoRevision`)

### Requirement: Requests are counted for operators

Every verb SHALL count its request once in `DataFlow` on entry and SHALL count one error in `Health` whenever
`graph-query` itself refuses because no usable answer arrived: a downstream request got no answer, or a reply a
composed verb reads does not decode, names no `producer` or is not JSON, or a prefix page is broken. A responder's
classified refusal and a caller's `invalid_request` SHALL NOT count as an error.

#### Scenario: An unanswered forward and a refusal

- **WHEN** one forward is refused by its responder and another finds no responder
- **THEN** `DataFlow` shows both requests and `Health` shows one error (`TestForwardCountsRequestsAndUnansweredOnes`)

#### Scenario: A verb that makes two requests

- **WHEN** `entityByAlias` reads a holder through the alias and entity verbs, and later finds no responder on the
  entity verb
- **THEN** each request to `entityByAlias` is one in `DataFlow`, and only the second is an error in `Health`
  (`TestComposedVerbsCountAsForwardsDo`)

#### Scenario: A walk that loses its responder

- **WHEN** `pathSearch` walks several hops and a later hop finds no responder
- **THEN** the request is one in `DataFlow` and one error in `Health` (`TestComposedVerbsCountAsForwardsDo`)

#### Scenario: A broken reply

- **WHEN** an alias or index reply does not decode or names no `producer`, an entity reply is not JSON, or a prefix
  page is broken
- **THEN** the request is one error in `Health`, while a responder's refusal and a caller's `invalid_request` add none
  (`TestComposedVerbsCountAsForwardsDo`)
