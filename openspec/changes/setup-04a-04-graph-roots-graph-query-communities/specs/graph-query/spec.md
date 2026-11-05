# graph-query

## ADDED Requirements

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

The answer SHALL be `Found {n} matching entities among {entities_scanned} read from {community_id}, which has
{member_count} members.`, followed by the record's summary when it has one, and then by `Key themes:` and its first
five keywords, separated by a comma and a space and ending in a period, when it has keywords.

#### Scenario: The answer text

- **WHEN** three of 40 read entities match in a group of 7,000 members with a summary and keywords
- **THEN** the answer is the literal template filled with those values (`TestLocalSearchAnswerTemplate`)

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
projection. `Stop` SHALL return only after the view has stopped.

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
- **THEN** the next `localSearch` is refused `index_not_ready`, and after the bucket is recreated and published it
  answers again (`TestLocalSearchRefusesAfterWatchLoss`)

#### Scenario: Stop

- **WHEN** graph-query stops with the view running
- **THEN** `Stop` returns after the view's `Run` has returned (`TestStopEndsTheCommunityView`)

### Requirement: Truncated searches are counted

An answer with `entities_truncated` true SHALL add one to a counter labelled by verb, registered on the metrics
registry graph-query is given; given none, it SHALL register nothing.

#### Scenario: A truncated answer

- **WHEN** a `localSearch` answer is truncated
- **THEN** the counter for `localSearch` rises by one (`TestTruncatedSearchIsCounted`)

## MODIFIED Requirements

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

Every verb SHALL count its request once in `DataFlow` on entry and SHALL count one error in `Health` whenever
`graph-query` itself refuses because no usable answer arrived: a downstream request got no answer, or a reply a
composed verb reads does not decode, names no `producer` or is not JSON, or a prefix page is broken, or a community
record is unreadable. A responder's classified refusal, a caller's `invalid_request` and a community view that is not
usable SHALL NOT count as an error.

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
