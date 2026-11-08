# graph-query Specification

## Purpose
The `processor/graph-query` component answers graph reads on `graph.query.*`: it forwards authority and name reads unchanged, and composes the alias and relationship lookups into one envelope, so a caller either gets a correct answer or a refusal it can act on. The package lands in steps; this spec holds what is in the tree (`processor/graph-query/MIGRATION.md`).

## Requirements

### Requirement: The component serves exactly its declared verbs

The graph-query component SHALL subscribe to the subject of every `graph.QueryVerbs()` entry whose responder is
`graph-query`, and to no other request subject. `Start` SHALL fail naming a declared verb with no handler. A verb
another component serves under `graph.query.*` that the table does not declare SHALL be left to that component.

#### Scenario: A consumer verb beside the component

- **WHEN** graph-query has started and another subscriber serves `graph.query.sources`
- **THEN** a request on `graph.query.sources` is answered by that subscriber alone
- **AND** a request on each declared verb is answered by graph-query (`TestGraphQueryServesExactlyTheDeclaredVerbs`)

### Requirement: Authority and name replies are forwarded unchanged

`entity`, `batch`, `prefix` and `byName` SHALL forward the request bytes to the matching `graph.ingest.query.*` or
`graph.index.query.byName` verb and return the reply bytes unchanged, without decoding, re-encoding or re-checking
either.

#### Scenario: Bytes pass through

- **WHEN** a caller of `entity`, `batch`, `prefix` or `byName` sends bytes Q, with an unknown field and non-canonical
  spacing, and the downstream verb replies with such bytes B
- **THEN** the downstream verb receives exactly Q and the caller receives exactly B (`TestForwardPassesBytesBothWays`)

### Requirement: A refusal reaches the caller intact

Every verb SHALL return a downstream classified refusal with its class, code and detail unchanged. When a downstream
request reports no responders, the component SHALL refuse with class transient and code `internal`, naming the
subject. A downstream request that times out SHALL stay transient.

#### Scenario: A relayed refusal

- **WHEN** a downstream verb refuses with class C, code X and detail D
- **THEN** the caller's classified error has class C, code X and detail D (`TestForwardedRefusalKeepsClassCodeDetail`)

#### Scenario: No responders

- **WHEN** a downstream request reports no responders
- **THEN** the caller receives a transient `internal` refusal naming that subject (`TestNoRespondersIsTransient`)

### Requirement: Alias lookup refuses what the alias verb refuses

`entityByAlias` SHALL ask the alias verb first and SHALL return any refusal it gives, `alias_ambiguous`,
`index_not_ready`, `internal` and `revision_not_reached` among them, without reading an entity (ADR-114).

#### Scenario: A shared alias

- **WHEN** a query names an alias two entities share
- **THEN** the caller receives the `alias_ambiguous` refusal, not "not found" (`TestEntityByAlias`)

### Requirement: Alias lookup answers in one envelope

The request SHALL be `alias` with optional `min_revision`. A held alias SHALL read its holder. An alias with no holder
SHALL read the input as an entity ID when it is a valid one. A success SHALL be a `graph.QueryResponse` from producer
`graph-query` whose `data` is the entity verb's JSON as it came, a `graph.ExactEntity` not re-decoded into a struct,
and whose `indexed_revision` is the alias reply's.

#### Scenario: An ID given to the alias verb

- **WHEN** a query names a valid entity ID that no entity holds as an alias, and that entity exists
- **THEN** the reply's `data` is that entity and `indexed_revision` is the alias reply's (`TestEntityByAlias`)

### Requirement: A broken alias or entity reply is refused

An alias reply that does not decode or names no `producer`, and an entity reply that is not JSON, SHALL be refused
transient `internal`, naming the subject.

#### Scenario: An entity reply that is not JSON

- **WHEN** the alias verb names a holder and the entity verb replies with bytes that are not JSON
- **THEN** the caller receives a transient `internal` refusal naming the entity verb's subject (`TestEntityByAlias`)

### Requirement: An unknown alias that is no entity ID is not found

An alias with no holder whose input is not a valid entity ID SHALL be refused with class invalid and code
`entity_not_found`, without a second request.

#### Scenario: An unknown alias

- **WHEN** a query names `sn-1`, which no entity holds as an alias
- **THEN** the caller receives `entity_not_found` and the entity verb receives no request (`TestEntityByAlias`)

### Requirement: Relationships are enveloped and refuse an unknown direction

`relationships` SHALL take `entity_id` and `direction`, forwarding the request bytes unchanged, and SHALL read
`direction` as `outgoing` or `incoming`, and SHALL refuse any other value, or none, with `invalid_request`. A success
SHALL be a `graph.QueryResponse` from producer `graph-query`, `indexed_revision` being the index reply's and `data` a
non-null list of `graph.RelationshipEdge`, each with `predicate`, `from_entity_id` and `to_entity_id`.

#### Scenario: Row keys on the wire

- **WHEN** a real index holds one relationship and a caller asks for it in each direction
- **THEN** `data` compares equal to the literal JSON row, with exactly the keys `predicate`, `from_entity_id` and
  `to_entity_id`, in both directions (`TestRelationshipsRowKeys`)

#### Scenario: A direction the component does not read

- **WHEN** a request sends `direction` `both`, or none
- **THEN** the caller receives `invalid_request` and no index verb receives a request
  (`TestRelationshipsRefusesUnknownDirection`)

### Requirement: A broken index reply is refused

An index reply to `relationships` that does not decode or names no `producer` SHALL be refused transient `internal`.

#### Scenario: A broken index reply

- **WHEN** the index replies `{}`
- **THEN** the caller receives a transient `internal` refusal, not an empty list
  (`TestRelationshipsRefusesBrokenIndexReply`)

### Requirement: Revision requests answer or refuse, never wrongly succeed

Every verb SHALL accept `min_revision`. On an enveloped verb it SHALL reach every derived-index read unchanged, and on
`localSearch` it SHALL be compared with the group record's `entity_revision`, which is the reply's `indexed_revision`;
on a bare verb, and on an answer built from no derived-index read, it SHALL be ignored, an authority read having no
index to catch up. A success built from a derived-index read to `min_revision` M SHALL carry `indexed_revision` at or
above M.

#### Scenario: Reading a write back

- **WHEN** a caller writes entity E, receives `kv_revision` W, and requests `relationships` for E with `min_revision` W
- **THEN** it receives a reply with `indexed_revision` at or above W reflecting the write, or a `revision_not_reached`
  refusal
- **AND** never a success with `indexed_revision` below W (`TestReadYourWrites`)

#### Scenario: Reading a write back through a group

- **WHEN** a caller writes entity E at W and asks `localSearch` for E with `min_revision` W, before and after the
  group's next tick
- **THEN** before, it is refused `revision_not_reached` with the record's revision in the detail; after, it answers
  with `indexed_revision` at or above W (`TestLocalSearchReadsItsWriteBack`)

#### Scenario: A revision far ahead

- **WHEN** `byName`, `relationships` in either direction, `entityByAlias`, `pathSearch`, `summary` or `localSearch`
  carries a `min_revision` far above the index's revision
- **THEN** the caller receives `revision_not_reached` with its class and detail
  (`TestMinRevisionRefusalThroughGraphQuery`)

#### Scenario: An answer that read only the authority

- **WHEN** `hierarchyStats` carries a `min_revision` far above any revision
- **THEN** it answers bare (`TestHierarchyStatsIgnoresMinRevision`)

#### Scenario: A summary that read only the authority

- **WHEN** `summary` with `exclude_predicates` carries a `min_revision` far above any revision
- **THEN** it answers, with `indexed_revision` 0 (`TestAuthorityOnlyAnswerClaimsNoRevision`)

### Requirement: Requests are counted for operators

Each request SHALL count once in `DataFlow` on entry, and one error in `Health` when `graph-query` refuses: no
downstream answer; a composed verb's reply that does not decode, names no `producer` or is not JSON; a broken prefix
page; an unreadable community record; a `localSearch` that cannot read `max_payload` or fit its reply even with no
entities; a global search whose classification ended with the request, whose hints cannot be read, or whose
classifier failed other than for invalid input.

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

- **WHEN** an alias or index reply does not decode or names no `producer`, an entity reply is not JSON, a prefix page
  is broken, or a community record is unreadable
- **THEN** the request is one error in `Health`, while a responder's refusal, a caller's `invalid_request` and a view
  not yet caught up add none (`TestComposedVerbsCountAsForwardsDo`)

### Requirement: Refusals that are not errors

A responder's classified refusal, a caller's `invalid_request`, an unusable community view or empty level, and global
search's `entity_not_found` SHALL NOT count as an error in `Health`; each request still counts once in `DataFlow` on
entry.

#### Scenario: A question this build cannot answer

- **WHEN** `globalSearch` refuses a count question `invalid_request`
- **THEN** the request is one in `DataFlow` and no error in `Health` (`TestGlobalSearchRoutesByHints`)

### Requirement: Configuration is strict and read

Decoding SHALL refuse an unknown key, naming it. `query_timeout_ms` SHALL be the deadline on the wait for each
downstream reply, default 15000; like the handler's context, it does not end a wait to write to a broker that has
stopped reading (ADR-112, "Time"). A configuration with no ports SHALL take the default required `graph_queries`
NATS-request port on `graph.query.*`, and any other input port shape SHALL be refused.

#### Scenario: A key the pin ignored

- **WHEN** a configuration sets `max_traversal_depth`
- **THEN** construction fails naming that key (`TestUnknownConfigKeyIsRefused`)

#### Scenario: The timeout is read

- **WHEN** a downstream verb never replies and `query_timeout_ms` is small
- **THEN** the caller receives a transient refusal before its own request timeout (`TestQueryTimeoutIsRead`)

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

### Requirement: Local search answers from one group's record and one page

`localSearch` SHALL read the record of `entity_id`'s group at `level` (0 to 2, default 0) from graph-query's view of
`COMMUNITY_INDEX`, then one `graph.ingest.query.prefix` page of that group at `graph.MaxPrefixQueryLimit`, and SHALL
make no other request when the record is there. It SHALL answer enveloped with a `graph.LocalSearchData`, whose
`entities_truncated` SHALL be true exactly when the page had a continuation or matches were left out to fit
`max_payload`.

#### Scenario: The reply's fields

- **WHEN** a group's record and page are read
- **THEN** `data` holds the record's ID, level, member count, summary and keywords; the page's matching entities, each
  as the page sent it; the number of distinct entities read; and an answer text (`TestLocalSearchReadsOnePage`)

#### Scenario: A group larger than a page

- **WHEN** a group holds more entities than one page and its only match sorts after the first page
- **THEN** `entities` is empty, `entities_truncated` is true, and the group received exactly one prefix request
  (`TestLocalSearchReadsOnePage`)

#### Scenario: A group that fits a page

- **WHEN** a whole group fits one page
- **THEN** `entities` equals an independent filter of the group and `entities_truncated` is false
  (`TestLocalSearchReadsOnePage`)

#### Scenario: The asked level

- **WHEN** an entity's groups have records at levels 0 and 1 and a request asks level 1
- **THEN** the answer is the level-1 record's (`TestLocalSearchReadsTheAskedLevel`)

#### Scenario: Entities pass unchanged

- **WHEN** a matching entity holds a value graph-query does not read, such as an integer above 2^53
- **THEN** that entity reaches the caller as the page held it (`TestLocalSearchEntitiesPassUnchanged`)

#### Scenario: A reply over the payload limit

- **WHEN** so many matches that the reply would exceed `max_payload`
- **THEN** the lowest-ranked are left out until it fits, and `entities_truncated` is true
  (`TestLocalSearchFitsMaxPayload`)

### Requirement: Local search ranks by the pin's text score

The query SHALL be lowercased, split on white space, with repeats removed, and hold 1 to `graph.MaxSearchTerms`
terms. A term SHALL score 1.0 when it equals the entity's type position, else 0.5 when it is a substring of a predicate
or a string value; an entity scores the sum over the term count and SHALL match only at 0.3 or above. Matches SHALL be
ordered by score descending, then entity ID.

#### Scenario: The pin's scores

- **WHEN** entities are scored against the literal table carried from the pin's `graphrag_test`
- **THEN** each score and each kept or dropped entity matches it (`TestLocalSearchScoresAsThePin`)

#### Scenario: Repeated and too many terms

- **WHEN** a query repeats a term, or holds more than `graph.MaxSearchTerms` distinct terms
- **THEN** the repeat counts once, and the long query is refused `invalid_request` before any read
  (`TestLocalSearchTermsAreDistinctAndBounded`)

#### Scenario: Ties

- **WHEN** several entities score the same
- **THEN** they are ordered by entity ID (`TestLocalSearchOrderIsDeterministic`)

### Requirement: Local search answers in a fixed template

The answer SHALL be `Found {n} matching entities among {entities_scanned} read from {community_id}.`, followed by a
space and the record's summary when it has one. When it has none, it SHALL be followed by `It has {member_count}
members.` and, when it has keywords, by `Key themes:` and its first five keywords, separated by a comma and a space
and ending in a period.

#### Scenario: The answer text

- **WHEN** three of 40 read entities match in a group of 7,000 members with a summary and keywords
- **THEN** the answer is the first sentence and the summary, the summary already stating the member count and themes
  (`TestLocalSearchAnswerTemplate`)

#### Scenario: A record with no summary

- **WHEN** the group's record has no summary
- **THEN** the answer states the member count and, when the record has keywords, its first five
  (`TestLocalSearchAnswerTemplate`)

### Requirement: Local search refuses what it cannot answer

A request that does not decode strictly, or has an invalid `entity_id`, a `level` outside 0 to 2 or a bad query,
SHALL be refused `invalid_request` before any read. With no usable view, `localSearch` SHALL refuse `index_not_ready`,
transient. With no record for the group, it SHALL read the entity and refuse `index_not_ready` when it exists, else
return the entity verb's refusal as it arrived. An unreadable record SHALL be refused transient `internal`, naming its
key.

#### Scenario: No record

- **WHEN** an entity's group at the asked level has no record
- **THEN** an existing entity is refused `index_not_ready`, and an absent one gets the entity verb's refusal
  (`TestLocalSearchRefusals`)

#### Scenario: An unreadable record

- **WHEN** the group's key holds a value that is not a record
- **THEN** the request is refused `internal` naming the key, and after a valid write to that key it answers
  (`TestLocalSearchRefusals`)

#### Scenario: A bad request

- **WHEN** a request holds an unknown key, a five-part `entity_id`, `level` 3 or an empty query
- **THEN** it is refused `invalid_request` and no downstream request is made (`TestLocalSearchRefusesBadRequests`)

### Requirement: The community view follows the bucket

graph-query's `Start` SHALL neither wait for nor fail on a missing `COMMUNITY_INDEX`. While the bucket is missing, or
its view is catching up or lost, `localSearch` SHALL refuse `index_not_ready` and the other verbs SHALL answer. Once
the bucket exists, `localSearch` SHALL answer without a restart; after a lost watch it SHALL never answer from the old
projection. During a tick it SHALL answer each group from its latest record. `Stop` SHALL return only after the view
has stopped.

#### Scenario: During a tick

- **WHEN** the bucket holds one group's new record and another group's old one, as a tick held between two writes
  leaves it
- **THEN** a `localSearch` in each group answers at once, from the new record and the old one respectively, with no
  refusal (`TestLocalSearchDuringBlockedTick`)

#### Scenario: Readiness comes from the view alone

- **WHEN** `COMMUNITY_INDEX` is published while `GRAPH_STATUS` holds no key for graph-clustering and
  `COMMUNITY_SUMMARIES` does not exist
- **THEN** `localSearch` answers (ADR-087, Decision 5; `TestLocalSearchAfterCommunityIndexAppears`)

#### Scenario: Before the bucket exists

- **WHEN** graph-query starts with no `COMMUNITY_INDEX`
- **THEN** `Start` succeeds, `localSearch` is refused `index_not_ready`, and `entity` answers
  (`TestLocalSearchBeforeCommunityIndexExists`)

#### Scenario: The bucket appears

- **WHEN** `COMMUNITY_INDEX` is created and published after `Start`
- **THEN** `localSearch` answers without a restart (`TestLocalSearchAfterCommunityIndexAppears`)

#### Scenario: A lost watch

- **WHEN** `COMMUNITY_INDEX` is deleted under a live view
- **THEN** once the view has seen the watch's loss (when nats.go reports it is not promised), `localSearch` is
  refused `index_not_ready`, and after the bucket is recreated and published it answers again
  (`TestLocalSearchRefusesAfterWatchLoss`)

#### Scenario: Stop

- **WHEN** graph-query stops with the view running
- **THEN** `Stop` returns after the view's `Run` has returned (`TestStopEndsTheCommunityView`)

### Requirement: Truncated searches are counted

An answer with `entities_truncated` true SHALL add one to a counter labelled by the verb asked (`localSearch`,
`globalSearch` or `searchGraph`), registered on the metrics registry graph-query is given; given none, it SHALL
register nothing.

#### Scenario: A truncated answer

- **WHEN** a `localSearch`, `globalSearch` or `searchGraph` answer is truncated
- **THEN** the counter for that verb rises by one (`TestTruncatedSearchIsCounted`)

### Requirement: Global search answers by its hints

`globalSearch` SHALL classify a question of more than one token once with the keyword classifier and answer: a path
intent with a start node and no other hint, `path`; a time range alone, `temporal`; an area alone, `spatial`; no
hint, or only path words with no start, similarity or ranking, `text`, each such hint named in `unhonoured_hints`.
No answer SHALL drop a hint the classifier reported without naming it.

#### Scenario: Routing

- **WHEN** `globalSearch` is asked "what is connected to sensor-001", "what changed in the last 2 hours", "what is
  within 5km of 40.7,-74.0" and "forklift hydraulic leak"
- **THEN** the replies' `strategy` is `path`, `temporal`, `spatial` and `text` (`TestGlobalSearchRoutesByHints`)

#### Scenario: A hint no strategy honours

- **WHEN** "find sensors similar to pump-42" or "top 5 sensors" is asked
- **THEN** the reply's `strategy` is `text` and `unhonoured_hints` names `similarity` or `ranking`
  (`TestGlobalSearchRoutesByHints`)

### Requirement: Global search refuses hints it cannot honour together

A count or other aggregation hint, with or without others, SHALL be refused `invalid_request` naming it, and so SHALL
a text-routed hint beside a filter (a start, a time range or an area) or a start carrying path predicates, naming
both.

#### Scenario: A text-routed hint beside a filter

- **WHEN** "top 5 sensors connected to pump-42", "find sensors similar to pump-42 in the last 2 hours" or "what is
  related to it in the last 2 hours" is asked
- **THEN** each is refused `invalid_request` naming both, and no downstream request is made
  (`TestGlobalSearchHintBesideAFilter`)

#### Scenario: A count

- **WHEN** "how many sensors are connected to pump-42", "how many pumps changed in the last 2 hours" or "max retries"
  is asked
- **THEN** each is refused `invalid_request` naming the aggregation, and no downstream request is made
  (`TestGlobalSearchRoutesByHints`)

### Requirement: Global search refuses a classification it cannot read

No result and no error from the classifier, hints that cannot be read, and a classifier error not of class invalid
SHALL be refused transient `internal`.

#### Scenario: Classification ended with the request

- **WHEN** the classifier returns no result and no error because the request's context has ended
- **THEN** the caller receives transient `internal` and `Health` counts one error
  (`TestGlobalSearchEndedClassification`)

### Requirement: A one-word question is an entity reference

A question of one token, no white space after trimming, SHALL be resolved as an entity reference before any
classification and answered by `entity`, reading no hint. A reference that names no entity SHALL be answered by the
text tier.

#### Scenario: Trigger words inside a reference

- **WHEN** an entity holds the ID `acme.plant.today.sensor.temp.001`, `acme.plant.zone-a.sensor.temp.001`,
  `acme.plant.link.sensor.temp.001` or `acme.plant.most.sensor.temp.001`, or the alias `connector-7`, `max-pump` or
  `total-flow`, and that one word is asked
- **THEN** each reply's `strategy` is `entity` and names that entity (`TestGlobalSearchOneWordIsAReference`)

### Requirement: Global search refuses what no strategy can honour

`globalSearch` SHALL decode `query`, `min_revision` and `level` strictly, refusing a blank `query`, a `level` outside
0 to 2, and any other key with `invalid_request` naming it. Two of path, time range and area together SHALL be
refused `invalid_request` naming both, before any other routing, and a question the classifier refuses (an impossible
number, an entity beside a zone or area phrase) SHALL be refused `invalid_request` with its text. Each refusal SHALL
come before any downstream request.

#### Scenario: Two filters

- **WHEN** a question carries a path start and a time range, or a time range and an area, or "how many pumps
  connected to pump-42 in the last 2 hours" adds a count to a path and a time range
- **THEN** it is refused class invalid, code `invalid_request`, naming both, and no downstream request is made
  (`TestGlobalSearchRefusesTwoFilters`)

#### Scenario: An impossible number

- **WHEN** a question asks for "the last 0 hours"
- **THEN** it is refused class invalid, code `invalid_request`, naming the phrase
  (`TestGlobalSearchRefusesImpossibleNumbers`)

#### Scenario: An entity and a place

- **WHEN** "what is connected to sensor-001 in zone-b", "what is connected to sensor-001 near zone-b" or "show
  sensor-001 in zone-b" is asked
- **THEN** it is refused class invalid, code `invalid_request`, naming both places, and no downstream request is made
  (`TestGlobalSearchRefusesADroppedPlace`, `TestChainRefusesAnEntityBesideAZone`)

#### Scenario: A pin key

- **WHEN** a request carries `summarize_threshold`, `max_communities` or an `include_*` key, or `level` 3
- **THEN** it is refused `invalid_request` naming the key (`TestGlobalSearchRequestKeys`)

### Requirement: One resolver reads an entity reference

`entityByAlias` and global search SHALL resolve a reference the same way: the alias verb's holder; else the reference
itself when it is a valid entity ID; else not found, with no second request. `entityByAlias` and a global search
named start SHALL answer not found as `entity_not_found`; a one-word global search reference SHALL be answered by the
text tier. A refusal from the alias or entity verb SHALL reach the caller with its class, code and detail. No suffix
lookup SHALL be made.

#### Scenario: An ambiguous alias

- **WHEN** a global search reference is an alias two entities carry
- **THEN** the caller receives `alias_ambiguous` as the alias verb sent it (`TestGlobalSearchPassesAliasRefusals`)

#### Scenario: An unknown reference

- **WHEN** a one-token question is no alias and no entity ID, or a valid entity ID with no record
- **THEN** it is answered by the text tier, never `entity_not_found` (`TestGlobalSearchUnknownWordIsSearched`,
  `TestGlobalSearchUnknownIDIsSearched` for the valid ID with no record)

#### Scenario: An unknown named start

- **WHEN** "what is connected to pump-99" or "the area of the plant" is asked and nothing is named `pump-99` or
  `area-of`
- **THEN** the caller receives `entity_not_found`, never an empty answer, and no walk or text search is made
  (`TestGlobalSearchUnknownPathStart`, `TestZoneQuestionNeverAnswersEmpty`)

### Requirement: A strategy answers with its own result

The reply SHALL be enveloped `graph.GlobalSearchData` with `strategy` and exactly one result: `entity`, the entity
verb's reply bytes unchanged; `path`, the `graph.PathSearchData` of step 2's walk from the resolved start with direction
`both`, `max_depth` 3 and `max_nodes` 100; `temporal` or `spatial`, the `graph.TemporalData` or `graph.SpatialData`
returned for a `graph.TemporalRangeRequest` or `graph.SpatialBoundsRequest` with `limit` 100; `text`, the
text tier's `graph.TextSearchData`.

#### Scenario: A path answer is the path search's

- **WHEN** a path question resolves to E
- **THEN** `data.path` equals `pathSearch`'s `data` for E, `both`, 3, 100 (`TestGlobalSearchPath`)

#### Scenario: A time-range request

- **WHEN** "what changed in the last 2 hours" is asked and a responder serves `graph.temporal.query.range`
- **THEN** it receives `start_time`, `end_time` and `limit` 100, and its `truncated` reaches the caller
  (`TestGlobalSearchTemporal`)

#### Scenario: No responder yet

- **WHEN** a time-range or area question is asked with no responder on its verb
- **THEN** the caller receives transient `internal` naming that subject (`TestGlobalSearchNoResponders`)

#### Scenario: A text answer

- **WHEN** a free-text question is answered
- **THEN** `data.text` is set and no other result field is (`TestTextTierReply`)

### Requirement: A strategy's revision is its read's

A global search reply's `indexed_revision` SHALL be the alias reply's for `entity`, the walk's for `path`, the
responder envelope's for `temporal` and `spatial`, and the highest `entity_revision` among the level's readable
records for `text`; `min_revision` SHALL reach each derived-index read and SHALL be compared with that mark for
`text`.

#### Scenario: A revision far ahead

- **WHEN** `globalSearch` carries a `min_revision` far above the index's revision
- **THEN** the caller receives `revision_not_reached` with its class and detail
  (`TestMinRevisionRefusalThroughGraphQuery`)

### Requirement: The text tier answers a question no strategy routes

`globalSearch` SHALL answer with strategy `text`: a question of more than one token with no hint; a one-token question
that names no entity (no alias holder and no valid ID, or a valid ID with no record), searching its words; and a
question whose only hints are path words with no start, similarity or ranking, naming each in `unhonoured_hints`. The
reply SHALL be enveloped `graph.GlobalSearchData` with `strategy` `text` and `text` a `graph.TextSearchData`.

#### Scenario: A free-text question

- **WHEN** "forklift hydraulic leak" is asked and records exist at level 0
- **THEN** the reply's `strategy` is `text` and `unhonoured_hints` is empty (`TestGlobalSearchTextQuestion`)

#### Scenario: Trigger words in free text

- **WHEN** "http connection pool" or "file path handling" is asked
- **THEN** the reply's `strategy` is `text` and `unhonoured_hints` is `["path"]` (`TestGlobalSearchRoutesByHints`)

#### Scenario: A word that names no entity

- **WHEN** the one word "forklift" is asked and no alias or entity matches it
- **THEN** the reply's `strategy` is `text` and its query is that word (`TestGlobalSearchUnknownWordIsSearched`)

### Requirement: The text tier reads at most five matching groups, one page each

The text tier SHALL read the community view once per request, the records under `clustering.LevelPrefix(level)`
(`level` 0 to 2, omitted 0), and score each over the question's lowercased, de-duplicated terms: 2.0 for each term
contained in its statistical summary, 1.5 for each keyword and term where one contains the other. It SHALL read up to
five records scoring above 0, best first, ties by key, each with one `graph.ingest.query.prefix` page at
`MaxPrefixQueryLimit`, and no other entity read.

#### Scenario: Seven matching groups

- **WHEN** seven groups at level 0 match the question
- **THEN** exactly five prefix requests are made, for the five best (`TestTextTierReadsAtMostFiveGroups`)

#### Scenario: No group matches

- **WHEN** no record at the level scores above 0
- **THEN** the reply has no groups and no matches, no prefix request is made, and the answer says so
  (`TestTextTierSkipsUnmatchedGroups`)

### Requirement: Text matches are the pages' entities as stored, best first

The text tier SHALL rank each page's entities by the score `localSearch` uses, keeping those at 0.3 or above, merged
across groups by score then entity ID, each match carrying `id`, `label`, `score` and `entity`, the page's bytes
unchanged. `entities_truncated` SHALL be true exactly when a page had more to read or matches were left out to fit
`max_payload`.

#### Scenario: Groups that fit a page

- **WHEN** every selected group fits one page
- **THEN** the matches equal an independent filter of those groups and `entities_truncated` is false
  (`TestTextTierMatchesEqualAFilter`)

### Requirement: A match's label is its most salient label predicate

`label` SHALL be the first non-empty string value, in triple order, of the entity's most salient label predicate holding
one: the lowest priority number in `vocabulary.DiscoverLabelPredicates()`, read at `Start`, ties by predicate name. It SHALL
be omitted when the entity has none.

#### Scenario: The label

- **WHEN** an entity holds `dc.terms.title` and an unregistered display-name predicate
- **THEN** its label is the title, and with neither it has no label (`TestMatchLabel`)

### Requirement: The text reply declares what it could not read

`graph.TextSearchData` SHALL carry `level`, `groups` (each record's `community_id`, `member_count`,
`community_summary` and `keywords`), `matches`, `entities_scanned`, `entities_truncated`, `unreadable_groups` (the
poisoned records at the level), `unhonoured_hints` and `answer`, a plain text built from these fields that says it is
a sample when `entities_truncated` is true.

#### Scenario: A cut answer says so

- **WHEN** a selected group has more than one page
- **THEN** `entities_truncated` is true and the answer says it is a sample (`TestTextTierAnswer`)

### Requirement: An answer past unreadable records is signalled

An answer with `unreadable_groups` above 0 SHALL count one error in `Health` and log one Warn naming the verb, the
level, the count and why answering is safe.

#### Scenario: A corrupt record

- **WHEN** one record at the level is unreadable and another matches
- **THEN** the reply answers with `unreadable_groups` 1, `Health` counts one error, and one Warn names the verb, the
  level and the count; after a valid write to that key `unreadable_groups` is 0 and nothing is logged
  (`TestTextTierPoisonIsDeclared`)

### Requirement: The text tier refuses an index it cannot read

The text tier SHALL refuse transient `index_not_ready` when the community view is missing, catching up, lost or
stopped, or the level holds no record, readable or not, and transient `internal` naming the count, one error, when
every record at the level is unreadable. Each refusal SHALL come before any prefix read, and a caller's
`invalid_request` SHALL come before any of them.

#### Scenario: An empty level

- **WHEN** records exist at level 0 only and level 1 is asked
- **THEN** the caller receives `index_not_ready`; after a level-1 write the same request answers
  (`TestTextTierRefusesAnEmptyLevel`)

#### Scenario: Every record unreadable

- **WHEN** every record at the level is unreadable
- **THEN** the caller receives transient `internal` naming the count, and `Health` counts one error
  (`TestTextTierAllUnreadable`)

### Requirement: The text tier's revision is its level's highest readable mark

The reply's `indexed_revision` SHALL be the highest `entity_revision` among the level's readable records; an
unreadable record SHALL never count toward it. A `min_revision` above it SHALL be refused transient
`revision_not_reached`, with `indexed_revision` in detail, before any prefix read.

#### Scenario: A revision not reached

- **WHEN** `min_revision` is above every readable record's `entity_revision` at the level, while an unreadable record
  may hold a higher one
- **THEN** the caller receives `revision_not_reached` and no prefix request is made (`TestTextTierRevision`,
  `TestTextTierRevisionIgnoresUnreadable`)

### Requirement: searchGraph answers as global search

`graph.query.searchGraph` SHALL decode and answer every request as `globalSearch` does, with the same `data`. It SHALL
make no semantic request.

#### Scenario: The same question

- **WHEN** one request is sent to `searchGraph` and to `globalSearch`
- **THEN** both replies' `data` are equal (`TestSearchGraphAnswersAsGlobalSearch`)
