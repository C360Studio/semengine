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

`entity`, `batch`, `prefix`, `prefixIDs`, `byName` and `predicate` SHALL forward the request bytes to the matching
`graph.ingest.query.*`, `graph.index.query.byName` or `graph.index.query.predicate` verb and return the reply bytes
unchanged, without decoding, re-encoding or re-checking either.

#### Scenario: Bytes pass through

- **WHEN** a caller of `entity`, `batch`, `prefix`, `prefixIDs`, `byName` or `predicate` sends bytes Q, with an unknown
  field and non-canonical spacing, and the downstream verb replies with such bytes B
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
page; a search that cannot read `max_payload` or fit its reply even with no matches; a global search whose
classification ended with the request, whose hints cannot be read, or whose classifier failed other than for invalid
input.

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

A responder's classified refusal, a caller's `invalid_request`, a hint not honoured because a responder refused it or
global search does not apply it, a search answered without community records, and `not_enabled` SHALL NOT count as an
error in `Health`; each request still counts once in `DataFlow` on entry.

#### Scenario: A question this build cannot answer

- **WHEN** `globalSearch` answers a count question with its aggregation hint not honoured
- **THEN** the request is one in `DataFlow` and no error in `Health` (`TestEveryQuestionIsSearched`)

#### Scenario: A verb turned off

- **WHEN** `semantic` is refused `not_enabled`
- **THEN** the request is one in `DataFlow` and no error in `Health` (`TestNotEnabledRefusesBeforeForwarding`)

### Requirement: Configuration is strict and read

Decoding SHALL refuse an unknown key, naming it. `query_timeout_ms` SHALL be the deadline on the wait for each
downstream reply, default 15000; like the handler's context, it does not end a wait to write to a broker that has
stopped reading (ADR-112, "Time"). `embedding_enabled` SHALL be false when left out. A configuration with no ports SHALL
take the default required `graph_queries` NATS-request port on `graph.query.*`, and any other input port shape SHALL be
refused.

#### Scenario: A key the pin ignored

- **WHEN** a configuration sets `max_traversal_depth`
- **THEN** construction fails naming that key (`TestUnknownConfigKeyIsRefused`)

#### Scenario: The timeout is read

- **WHEN** a downstream verb never replies and `query_timeout_ms` is small
- **THEN** the caller receives a transient refusal before its own request timeout (`TestQueryTimeoutIsRead`)

#### Scenario: The embedding key is read

- **WHEN** a configuration sets `embedding_enabled` to `false`, to `true`, to `null`, to `"false"`, or leaves it out
- **THEN** it reads false, true, false, is refused at construction, and false (`TestConfigEmbeddingEnabled`)

### Requirement: Derived-index replies are forwarded unchanged

`semantic` and `similar`, while `embedding_enabled` is true, and `spatial` and `temporal` SHALL forward the request
bytes to the verb their table row names and return the reply bytes unchanged. graph-embedding serves the first two;
no responder in this tree serves the last two yet.

#### Scenario: A field graph-query does not read rides through

- **WHEN** a caller of `semantic` sends a request holding `scope`
- **THEN** `graph.embedding.query.search` receives exactly those bytes and the caller receives exactly its reply
  (`TestForwardPassesBytesBothWays`)

#### Scenario: A responder not yet deployed

- **WHEN** a caller of one of the four asks, `embedding_enabled` is true, and nothing serves the verb behind it
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

`pathSearch` SHALL make at most 1 + `max_nodes` requests (1 + 2 × `max_nodes` for `both`). `hierarchyStats` SHALL
make exactly one prefix request; `summary` one prefix request and, unless predicates are excluded, one `predicateList` first.
`localSearch` SHALL make one word search and at most one batch request; `globalSearch` the same, plus at most one
alias and one entity request and one walk within `pathSearch`'s bound. A long request on one verb SHALL NOT stop
another verb answering.

#### Scenario: Counting the requests

- **WHEN** counting subscribers sit beside the real responders
- **THEN** the counts are within those bounds (`TestPathSearchRequestsAreBounded`, `TestCountsReadOnePage`,
  `TestSearchRequestsAreBounded`)

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

### Requirement: The community view follows the bucket

graph-query's `Start` SHALL neither wait for nor fail on a missing `COMMUNITY_INDEX`. While the bucket is missing, or
its view is catching up or lost, searches SHALL answer with `record_state` `not_ready` and the other verbs SHALL
answer. Once the bucket exists, searches SHALL attach records without a restart; after a lost watch they SHALL never
attach a record from the old projection; during a tick, each group's latest. `Stop` SHALL return only after the view
has stopped.

#### Scenario: During a tick

- **WHEN** the bucket holds one group's new record and another group's old one, as a tick held between two writes
  leaves it
- **THEN** a `localSearch` in each group answers at once, with the new record and the old one respectively
  (`TestLocalSearchDuringBlockedTick`)

#### Scenario: Readiness comes from the view alone

- **WHEN** `COMMUNITY_INDEX` is published while `GRAPH_STATUS` holds no key for graph-clustering and
  `COMMUNITY_SUMMARIES` does not exist
- **THEN** `localSearch` answers with `record_state` `read` (ADR-087, Decision 5;
  `TestLocalSearchAfterCommunityIndexAppears`)

#### Scenario: Before the bucket exists

- **WHEN** graph-query starts with no `COMMUNITY_INDEX`
- **THEN** `Start` succeeds, `localSearch` answers with `record_state` `not_ready`, and `entity` answers
  (`TestLocalSearchBeforeCommunityIndexExists`)

#### Scenario: The bucket appears

- **WHEN** `COMMUNITY_INDEX` is created and published after `Start`
- **THEN** `localSearch` attaches the record without a restart (`TestLocalSearchAfterCommunityIndexAppears`)

#### Scenario: A lost watch

- **WHEN** `COMMUNITY_INDEX` is deleted under a live view
- **THEN** once the view has seen the watch's loss (when nats.go reports it is not promised), `localSearch` answers
  with `record_state` `not_ready`, and after the bucket is recreated and published it attaches the record again
  (`TestLocalSearchRefusesAfterWatchLoss`, renamed `TestLocalSearchAfterWatchLoss`)

#### Scenario: Stop

- **WHEN** graph-query stops with the view running
- **THEN** `Stop` returns after the view's `Run` has returned (`TestStopEndsTheCommunityView`)

### Requirement: Truncated searches are counted

An answer with `truncated` true SHALL add one to a counter labelled by the verb asked (`localSearch`, `globalSearch`
or `searchGraph`), registered on the metrics registry graph-query is given; given none, it SHALL register nothing.

#### Scenario: A truncated answer

- **WHEN** a `localSearch`, `globalSearch` or `searchGraph` answer is truncated
- **THEN** the counter for that verb rises by one (`TestTruncatedSearchIsCounted`)

### Requirement: Global search refuses a classification it cannot read

No result and no error from the classifier, hints that cannot be read, and a classifier error not of class invalid
SHALL be refused transient `internal`.

#### Scenario: Classification ended with the request

- **WHEN** the classifier returns no result and no error because the request's context has ended
- **THEN** the caller receives transient `internal` and `Health` counts one error
  (`TestGlobalSearchEndedClassification`)

### Requirement: One resolver reads an entity reference

`entityByAlias` and global search SHALL resolve a reference the same way: the alias verb's holder; else the reference
itself when it is a valid entity ID; else not found, with no second request. `entityByAlias` SHALL answer not found as
`entity_not_found`, global search as a not-honoured hint with that code. A refusal from the alias or entity verb SHALL
reach an `entityByAlias` caller with its class, code and detail, and SHALL be a global search hint's `code`. No
suffix lookup SHALL be made.

#### Scenario: An ambiguous alias

- **WHEN** a global search reference is an alias two entities carry
- **THEN** the reply holds `text`, and the `entity` hint is not honoured with code `alias_ambiguous`
  (`TestGlobalSearchPassesAliasRefusals`)

#### Scenario: An unknown reference

- **WHEN** a one-token question is no alias and no entity ID, or a valid entity ID with no record
- **THEN** the reply holds `text` and a not-honoured `entity` hint with code `entity_not_found`
  (`TestGlobalSearchUnknownWordIsSearched`, `TestGlobalSearchUnknownIDIsSearched`)

#### Scenario: An unknown named start

- **WHEN** "what is connected to pump-99" or "the area of the plant" is asked and nothing is named `pump-99` or
  `area-of`
- **THEN** the reply holds `text`, no walk is made, and the hint is not honoured with code `entity_not_found`
  (`TestGlobalSearchUnknownPathStart`, `TestZoneQuestionIsSearched`)

### Requirement: A match's label is its most salient label predicate

`label` SHALL be the first non-empty string value, in triple order, of the entity's most salient label predicate holding
one: the lowest priority number in `vocabulary.DiscoverLabelPredicates()`, read at `Start`, ties by predicate name. It SHALL
be omitted when the entity has none.

#### Scenario: The label

- **WHEN** an entity holds `dc.terms.title` and an unregistered display-name predicate
- **THEN** its label is the title, and with neither it has no label (`TestMatchLabel`)

### Requirement: An answer past unreadable records is signalled

An answer with `unreadable_groups` above 0, or a local search with `record_state` `unreadable`, SHALL count one error
in `Health` and log one Warn naming the verb, the level, the count and why answering is safe.

#### Scenario: A corrupt record

- **WHEN** one group record among the matches' groups is unreadable
- **THEN** the reply answers with `unreadable_groups` 1, `Health` counts one error, and one Warn names the verb, the
  level and the count; after a valid write to that key `unreadable_groups` is 0 and nothing is logged
  (`TestTextTierPoisonIsDeclared`, renamed `TestUnreadableRecordIsDeclared`)

### Requirement: searchGraph answers as global search

When `embedding_enabled` is false, `graph.query.searchGraph` SHALL decode and answer every request as `globalSearch`
does, with the same `data`, and SHALL make no semantic request. When it is true, a word-search refusal other than
`index_not_ready` SHALL be returned as it arrived.

#### Scenario: The same question

- **WHEN** `embedding_enabled` is false and one request is sent to `searchGraph` and to `globalSearch`
- **THEN** both replies' `data` are equal and no semantic request is made (`TestSearchGraphAnswersAsGlobalSearch`,
  `TestSearchGraphTierZeroAsksNoEmbedding`)

### Requirement: A failed semantic fallback is named

When the embedding search is refused, broken, or does not answer within `query_timeout_ms`, searchGraph SHALL answer
the word list in its own order, each match with `word_rank` only, with `merge` `honoured` false, the refusal's `code`
and its message as `reason`. It SHALL never refuse the request for it.

#### Scenario: The fallback fails

- **WHEN** the embedding search is refused
- **THEN** the reply holds the word answer and `merge` not honoured with the refusal's code and message
  (`TestSearchGraphNamesAFailedMerge`)

#### Scenario: The fallback runs out of time

- **WHEN** the embedding search does not answer within `query_timeout_ms`
- **THEN** the reply holds the word answer and `merge` not honoured, with code `internal`
  (`TestSearchGraphMergeTimeoutKeepsWordAnswer`)

### Requirement: searchGraph falls back when the word index is not ready

When `embedding_enabled` is true and the word search is refused `index_not_ready`, searchGraph SHALL answer with
`merge` honoured, `detail` `index_not_ready`, `semantic` holding the embedding search's data, and `text` left out
(null), since the word index was not read. If the embedding search also fails it SHALL return the `index_not_ready`
refusal.

#### Scenario: The word index is not ready

- **WHEN** `embedding_enabled` is true, the word search is refused `index_not_ready`, and the embedding search answers
- **THEN** the reply leaves `text` out (null), holds `semantic`, and `merge` honoured with detail `index_not_ready`;
  had the embedding search failed, the `index_not_ready` refusal is returned
  (`TestSearchGraphIndexNotReadyLeavesTextOut`)

### Requirement: Embedding verbs can be turned off

When `embedding_enabled` is `false`, which it is when the key is left out, `semantic` and `similar` SHALL refuse every
request with class `invalid` and code `not_enabled`, the message naming the verb, without sending any request. Every
other verb SHALL be unchanged by the key, except `searchGraph`, whose merge runs only when the key is true.

#### Scenario: Not the same as a missing responder

- **WHEN** `embedding_enabled` is true and nothing serves `graph.embedding.query.search`
- **THEN** the caller is refused transient `internal` naming that subject, never `not_enabled`
  (`TestNoRespondersIsTransient`)

#### Scenario: Turned off beside a running responder

- **WHEN** graph-embedding is serving and graph-query is configured with `embedding_enabled: false`
- **THEN** `semantic` and `similar` are refused invalid `not_enabled` naming the verb, never answered
  (`TestNotEnabledRefusesBeforeForwarding`, `TestEmbeddingVerbsRefuseWhenNotEnabled`)

#### Scenario: Left out

- **WHEN** a configuration leaves `embedding_enabled` out
- **THEN** it reads false and `semantic` and `similar` are refused `not_enabled`; set true, they forward
  (`TestConfigEmbeddingEnabled`, `TestEmbeddingVerbsRefuseWhenNotEnabled`)

### Requirement: Local and global search are word searches

`localSearch` and `globalSearch` SHALL find their matches with one `graph.lexical.query.search` request each, passing
`query`, `limit` and `min_revision` unchanged, and SHALL score no text of their own. A refusal from the word search, a
query it cannot parse among them, SHALL reach the caller with its class, code and detail.

#### Scenario: A refusal from the word index

- **WHEN** graph-lexical refuses `index_not_ready`, `graph_state_reset_required`, `revision_not_reached` or
  `invalid_request` for `pump*`
- **THEN** both verbs return that refusal as it arrived, and make no other request
  (`TestLocalSearchPassesWordIndexRefusals`, `TestGlobalSearchPassesWordIndexRefusals`)

#### Scenario: No word index

- **WHEN** nothing serves `graph.lexical.query.search`
- **THEN** both verbs are refused transient `internal` naming that subject (`TestSearchWithoutWordIndex`)

### Requirement: Search requests decode strictly

Both verbs' requests SHALL decode strictly, refusing an unknown key with `invalid_request` naming it, and SHALL refuse
`invalid_request`, before any request, a blank `query`, a `level` outside 0 to 2, and a `limit` other than 0 (meaning
10) or 1 to 100. `localSearch` SHALL also refuse an `entity_id` that is not a literal entity ID.

#### Scenario: A bad local request

- **WHEN** a `localSearch` request holds an unknown key, a five-part `entity_id`, `level` 3, a blank query or
  `limit` 101
- **THEN** it is refused `invalid_request` and no request is sent (`TestSearchRequestsDecodeStrictly`)

#### Scenario: A bad global request

- **WHEN** a `globalSearch` request carries `summarize_threshold`, `max_communities` or an `include_*` key, a blank
  query, or `level` 3
- **THEN** it is refused `invalid_request` naming the key or field, and no request is sent
  (`TestSearchRequestsDecodeStrictly`)

### Requirement: Local search is scoped to the group

`localSearch` SHALL scope its word search to the group ID of `entity_id` at `level` (0 to 2, omitted 0), as
`clustering.GroupKey` names it, and SHALL answer enveloped `graph.LocalSearchData`: `community_id`, `level`, `record`,
the group's `COMMUNITY_INDEX` record (`member_count`, `community_summary`, `keywords`) when one was read, and
`record_state`, one of `read`, `none`, `not_ready` or `unreadable`. The record SHALL never gate the answer: no
`record_state` refuses.

#### Scenario: A better match outside the group

- **WHEN** an entity outside the group matches the query better than one inside it
- **THEN** only the entity inside the group is a match (`TestLocalSearchIsScopedToTheGroup`)

#### Scenario: No community record

- **WHEN** the community view is missing, holds no record for the group, or holds an unreadable one
- **THEN** local search answers its matches with `record_state` `not_ready`, `none` or `unreadable`
  (`TestLocalSearchRecordStates`)

### Requirement: Global search groups its matches by community

`globalSearch` SHALL search every entity, unscoped, and answer enveloped `graph.GlobalSearchData` whose `text` is a
`graph.TextSearchData`: `level`; `matches`; and `groups`, one for each level-`level` group holding a match, in the
order of its best match, each with `community_id`, `matches` (its match count), `record_state` and `record`, its
record as local search names it, present exactly when read. `unreadable_groups` SHALL count the groups whose record
could not be read.

#### Scenario: Grouped hits

- **WHEN** a question matches entities in three groups
- **THEN** `matches` keeps the word index's order, `groups` lists the three by their best match, and their counts sum
  to the number of matches (`TestGlobalSearchGroupsHits`)

#### Scenario: Before any community record

- **WHEN** `COMMUNITY_INDEX` does not exist
- **THEN** global search answers with every group's `record_state` `not_ready` (`TestGlobalSearchWithoutCommunityIndex`)

### Requirement: Matches are the hits read back as stored

Both verbs SHALL read their hits with one `graph.ingest.query.batch` request and answer `matches` in the word index's
order, each `id`, `label`, `score` (the word index's) and `entity`, the bytes the batch sent for it, which graph-query
does not re-encode. In a searchGraph merged list the order is the merged order and `score` is the merged score. A hit
the batch names missing SHALL be left out and counted in `missing`. A batch refusal SHALL reach the caller as it
arrived.

#### Scenario: Order and missing hits

- **WHEN** the batch returns the entities in another order and names one hit missing
- **THEN** the matches follow the hits' order without it, `missing` is 1 and `truncated` is true
  (`TestSearchMatchesKeepHitOrder`, `TestSearchDropsMissingHits`)

#### Scenario: Entities pass unchanged

- **WHEN** the batch sends a matching entity holding a value graph-query does not read, such as an integer above 2^53
- **THEN** that entity reaches the caller as the batch sent it (`TestLocalSearchEntitiesPassUnchanged`)

### Requirement: A reply over the payload limit loses its lowest matches

When a search reply would exceed `max_payload`, the lowest-ranked matches SHALL be left out until it fits.

#### Scenario: A reply over the payload limit

- **WHEN** so many matches that the reply would exceed `max_payload`
- **THEN** the lowest-ranked are left out until it fits, and `truncated` is true (`TestLocalSearchFitsMaxPayload`)

### Requirement: A search reply declares what it left out

A local or global search reply SHALL carry `matched`, the word index's count before the limit; `excluded`, its
excluded words; `bodies_not_indexed`, its count; and `truncated`, true exactly when `matched` is above the hits
returned, a hit is missing, or matches were left out to fit `max_payload`.

#### Scenario: A limited answer

- **WHEN** 40 entities match, the limit is 10 and the query is `pump -test`
- **THEN** `matched` is 40, `truncated` is true and `excluded` is `["test"]` (`TestSearchDeclaresWhatItLeftOut`)

### Requirement: A merged search reply declares what it left out

In every searchGraph reply with `embedding_enabled` true, merged or not, `truncated` SHALL be true exactly when the
lists read (both, or the word list alone when the embedding search failed) held more entities than the limit,
`matched` is above the word hits read, a hit is missing, or matches were left out to fit.

#### Scenario: A limited merged answer

- **WHEN** at tier 1 the word search returns 3 hits, the embedding list holds 12 entities, 2 of them among those 3,
  and the limit is 10
- **THEN** `matched` is 3 and `truncated` is true, since 13 entities were found and 10 sent
  (`TestSearchGraphMergedTruncation`)

### Requirement: Every question gets a word search

`globalSearch` SHALL answer every question that decodes with a word search, and no hint's trigger word SHALL refuse
it. It SHALL classify a question of more than one token once with the keyword classifier and list each hint in
`hints` with `hint`, `detail`, `honoured`, and, when not honoured, `reason` and the refusal's `code` when one caused
it. A classifier refusal of class invalid SHALL be a not-honoured `classification` hint carrying its text.

#### Scenario: Plain questions

- **WHEN** the 27 router-probe questions and "a path from pump-7 to pump-9" are asked of a graph naming none of
  their words as entities
- **THEN** none is refused, each reply holds `text`, and each hint the classifier reported is listed with
  `honoured` false and a reason, `hint` being one of `entity`, `path`, `time_range`, `area`, `aggregation`,
  `similarity`, `ranking` and `classification` (`TestEveryQuestionIsSearched`)

### Requirement: The word search comes before any hint

Global search SHALL send its word search before reading any hint. A refusal of the word search, a query the word
index's rules refuse among them, SHALL be the reply, and no hint SHALL be read: a one-token question the word index
refuses is not looked up as an entity.

#### Scenario: A question the word index refuses

- **WHEN** the question is `the of and`, or the one token `pump*`
- **THEN** the reply is the word index's `invalid_request`, and no alias, entity or path request is made
  (`TestWordIndexRefusalComesFirst`, `TestSearchGraphOtherRefusalIsReturned`)

### Requirement: A refused word search cancels the embedding search

searchGraph's embedding search is not a hint: it is sent with the word search, and on any word refusal other than
`index_not_ready` it SHALL be cancelled. The call waits for that request to end, never for its reply, and leaves
nothing running after it returns.

#### Scenario: Another word refusal

- **WHEN** the word search is refused with a code other than `index_not_ready`
- **THEN** that refusal is the reply and the embedding request is cancelled (`TestSearchGraphOtherRefusalIsReturned`)

### Requirement: A hint applies only when it is verified

A one-token question SHALL be an `entity` hint. An `entity` or `path` hint SHALL be honoured only when the shared
resolver finds its entity: then `entity` (the entity verb's bytes) or `path` (the path search from it, direction
`both`, `max_depth` 3, `max_nodes` 100) SHALL be added beside `text`. Every other hint SHALL be reported not honoured.

#### Scenario: An aggregation word

- **WHEN** "minimum viable configuration" is asked
- **THEN** the reply holds `text` and an `aggregation` hint not honoured, its reason saying global search computes no
  aggregation (`TestEveryQuestionIsSearched`)

#### Scenario: A start that resolves

- **WHEN** "what is connected to pump-7" is asked and `pump-7` resolves
- **THEN** the reply holds `text` and `path`, and the `path` hint is honoured (`TestVerifiedStartAddsAWalk`)

#### Scenario: A start that does not resolve

- **WHEN** "what is connected to pump-99" is asked and nothing is named `pump-99`
- **THEN** the reply holds `text`, and the `path` hint is not honoured with code `entity_not_found`
  (`TestUnverifiedStartIsReported`)

### Requirement: A search reply's revision is the word index's

A local or global search reply's `indexed_revision` SHALL be the word search reply's. A hint's own read SHALL carry
`min_revision`, and its refusal SHALL make the hint not honoured, never refuse the reply.

#### Scenario: The revision

- **WHEN** graph-lexical answers at revision R
- **THEN** the reply's `indexed_revision` is R (`TestSearchRevisionIsTheWordIndexs`)

### Requirement: Search answers in a fixed template

`answer` SHALL be `Found {matched} matching entities in {groups} groups.` for global search and `Found {matched}
matching entities in {community_id}.` for local search. In a searchGraph merged list, when some matches sent have no
`word_rank`, `, and {extra} more by meaning` comes before the full stop, `extra` being their number. Then a space and
`Showing the first {n}.` when `truncated` is true, and, for local search with a record read, a space and the record's
summary.

#### Scenario: A cut answer

- **WHEN** 40 entities match and the limit is 10
- **THEN** the answer is `Found 40 matching entities in 3 groups. Showing the first 10.` (`TestSearchAnswer`)

#### Scenario: A merged answer

- **WHEN** 3 entities match by words, all 3 are sent, and 2 more are sent with no `word_rank`
- **THEN** the answer begins `Found 3 matching entities in 2 groups, and 2 more by meaning.` (`TestSearchAnswer`)

### Requirement: Search quality holds against a plain reference

On the word index's frozen scorecard corpus and its 500 questions, `globalSearch` and `localSearch` (each question's
answer's level-0 group) SHALL each have the answer in the first ten at most one point below a plain BM25 computed in
the test, and at or above 55% and 58% respectively, and an MRR at most 0.01 below that reference's on the same
questions.

#### Scenario: The scorecard

- **WHEN** the corpus is ingested and the questions asked through both verbs
- **THEN** the R@10 and MRR bars hold, and the test logs R@1, R@10 and MRR for both verbs and the reference
  (`TestSearchScorecard`)

### Requirement: Losing the embedding provider keeps word search

With graph-embedding running on an outside provider, losing the provider SHALL leave the word searches answering as
before. searchGraph SHALL answer the word list with `merge` not honoured. The embedding verbs SHALL refuse rather
than answer empty, and graph-embedding's readiness SHALL read `degraded`, naming the provider failure among
`failed_reasons`.

#### Scenario: The provider goes away

- **WHEN** the provider is closed and an entity is then written
- **THEN** globalSearch and searchGraph find it by its words, searchGraph's `merge` is not honoured with its code set,
  `semantic` is refused, and graph-embedding reads `degraded` with `connection_refused` counted
  (`TestLosingTheProviderKeepsWordSearch`)

### Requirement: searchGraph merges words and meaning

When `embedding_enabled` is true and the word search answers, searchGraph SHALL ask the word search and the
embedding `search` verb, each for `graph.MaxSimilarityLimit` entities whatever the request's limit, and SHALL answer
`text.matches` as one list. Each entity in either list scores the sum, over the lists holding it, of 1 / (60 + its
1-based place). The list is ordered by score, highest first, entity ID ascending on ties, and cut to the request's
limit.

#### Scenario: Both lists hold matches

- **WHEN** the word list is [A, B] and the embedding list is [C, A]
- **THEN** the matches are A, C, B, each entity once, A with `word_rank` 1 and `vector_rank` 2
  (`TestMergeRanks`, `TestSearchGraphMergesWordsAndMeaning`)

#### Scenario: A smaller limit is a prefix

- **WHEN** the same question is asked with limit 10 and with limit 100, no hit is missing, and both replies fit
  `max_payload`
- **THEN** the first ten matches are the same, in the same order (`TestSearchGraphLimitIsAPrefix`)

### Requirement: A merged match says which lists held it

In a searchGraph merged list each match SHALL carry `word_rank` and `vector_rank`, its place in each list, omitted
when it is not in that list, and its `score` is the merged score. The reply SHALL carry `merge`, a hint entry with
`hint` `semantic`, `detail` `merged` and `honoured` true, and `semantic` with the embedding search's data as it came.

#### Scenario: Words match nothing

- **WHEN** the word search matches nothing and the embedding search answers
- **THEN** the matches are the embedding list's, each with `vector_rank` only, and `merge` is honoured
  (`TestSearchGraphMeaningAloneWhenWordsMatchNothing`)

### Requirement: searchGraph asks both searches at once

searchGraph SHALL send the word search and the embedding search before waiting on either reply, so the embedding
wait is not added to the word wait.

#### Scenario: Both searches are in flight together

- **WHEN** each search answers only after the other has been asked
- **THEN** searchGraph answers (`TestSearchGraphAsksBothAtOnce`)

### Requirement: Tier 1 search quality is measured before each tier 1 release

`task scorecard:tier1` SHALL boot graph-ingest, graph-index, graph-lexical, graph-embedding over the named provider
(section bodies only) and graph-query, and ask the frozen scorecard's 500 questions through `globalSearch`, the
`semantic` verb and `searchGraph`. It SHALL print R@1, R@10 and MRR for each, with the model the provider's reply
names, dimensions, query prefix, commits and timings. It SHALL NOT run in `task verify`.

#### Scenario: A run names its model

- **WHEN** the scorecard runs against a provider
- **THEN** its output names the model the provider's reply reports beside the three rows
  (`notes/probes/scorecard-*.md`, task 3.4)

### Requirement: The tier 1 scorecard holds a floor

Tier 1 has one floor, held by every model, kept as constants in the scorecard program, each citing the probe note it
was measured from. `task scorecard:tier1` SHALL exit 1 when the merged row's R@10 or MRR is below that floor or its R@10
is below either single row's, and 2 on any other failure.

#### Scenario: Below the floor

- **WHEN** the merged row's R@10 or MRR is below tier 1's floor
- **THEN** the program prints the three rows and exits 1 (`notes/probes/scorecard-*.md`, task 3.4)
