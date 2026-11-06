# graph-query

## ADDED Requirements

### Requirement: Global search answers by its hints

`globalSearch` SHALL classify the question once with the keyword classifier and answer only when one hint is the
question's only hint: a path intent with a start node, `path`; a time range, `temporal`; an area, `spatial`; no hint
and a question of one token, no white space after trimming, `entity`. No answer SHALL drop a hint the question
carried. A classifier answer of no result and no error SHALL be refused transient `internal`.

#### Scenario: Routing

- **WHEN** `globalSearch` is asked "what is connected to sensor-001", "what changed in the last 2 hours", "what is
  within 5km of 40.7,-74.0" and "sensor-001"
- **THEN** the replies' `strategy` is `path`, `temporal`, `spatial` and `entity` (`TestGlobalSearchRoutesByHints`)

#### Scenario: A hint no strategy honours

- **WHEN** "how many sensors are connected to pump-42", "top 5 sensors connected to pump-42", "find sensors similar
  to pump-42 in the last 2 hours" or "how many pumps changed in the last 2 hours" is asked
- **THEN** each is refused `text_search_unsupported` and no downstream request is made
  (`TestGlobalSearchRoutesByHints`)

#### Scenario: Classification ended with the request

- **WHEN** the classifier returns no result and no error because the request's context has ended
- **THEN** the caller receives transient `internal` and `Health` counts one error
  (`TestGlobalSearchEndedClassification`)

### Requirement: Global search refuses what no strategy can honour

`globalSearch` SHALL decode `query` and `min_revision` strictly, refusing a blank `query` and any other key with
`invalid_request` naming it. Two of path, time range and area together SHALL be refused `invalid_request` naming both,
whatever other hints the question carries, before any other routing, and a question the classifier refuses for an
impossible number SHALL be refused `invalid_request` naming the phrase. Each refusal SHALL come before any downstream
request.

#### Scenario: Two filters

- **WHEN** a question carries a path start and a time range, or a time range and an area, or "how many pumps
  connected to pump-42 in the last 2 hours" adds a count to a path and a time range
- **THEN** it is refused class invalid, code `invalid_request`, naming both, and no downstream request is made
  (`TestGlobalSearchRefusesTwoFilters`)

#### Scenario: An impossible number

- **WHEN** a question asks for "the last 0 hours"
- **THEN** it is refused class invalid, code `invalid_request`, naming the phrase
  (`TestGlobalSearchRefusesImpossibleNumbers`)

### Requirement: Questions needing the text search are refused until it exists

A question with no strategy in "Global search answers by its hints", and an `entity` or `path` reference that names no
entity (no alias holder and no valid ID, or a valid ID with no record), SHALL be refused with class invalid and code
`text_search_unsupported`, whether or not `COMMUNITY_INDEX` holds records. This refusal SHALL NOT count as an error in
`Health`. It SHALL never be `index_not_ready` or an empty success.

#### Scenario: A free-text question

- **WHEN** "forklift", "how many pumps are there" or "find sensors similar to pump-42" is asked, with and without
  community records
- **THEN** each is refused class invalid, code `text_search_unsupported`, and `Health` counts no error
  (`TestGlobalSearchTextTierRefusal`)

#### Scenario: A zone that names no entity

- **WHEN** "the area of the plant" is asked and no entity holds the alias `area-of`
- **THEN** it is refused `text_search_unsupported`, never answered empty (`TestZoneQuestionNeverAnswersEmpty`)

### Requirement: One resolver reads an entity reference

`entityByAlias` and global search SHALL resolve a reference the same way: the alias verb's holder; else the reference
itself when it is a valid entity ID; else not found, with no second request. `entityByAlias` SHALL answer not found as
`entity_not_found`; global search SHALL answer it as "Questions needing the text search are refused until it exists"
says. A refusal from the alias or entity verb
SHALL reach the caller with its class, code and detail. No suffix lookup SHALL be made.

#### Scenario: An ambiguous alias

- **WHEN** a global search reference is an alias two entities carry
- **THEN** the caller receives `alias_ambiguous` as the alias verb sent it (`TestGlobalSearchPassesAliasRefusals`)

#### Scenario: An unknown reference

- **WHEN** a one-token question is no alias and no entity ID, or a valid entity ID with no record
- **THEN** it is refused `text_search_unsupported`, never `entity_not_found`
  (`TestGlobalSearchUnknownReferenceFallsThrough`)

### Requirement: A strategy answers with its own result

The reply SHALL be enveloped `graph.GlobalSearchData` with `strategy` and exactly one result: `entity`, the entity
verb's reply bytes unchanged; `path`, the `graph.PathSearchData` of step 2's walk from the resolved start with direction
`both`, `max_depth` 3 and `max_nodes` 100; `temporal` or `spatial`, the `graph.TemporalData` or `graph.SpatialData`
their verbs return to a `graph.TemporalRangeRequest` or `graph.SpatialBoundsRequest` with `limit` 100.

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

### Requirement: A strategy's revision is its read's

A global search reply's `indexed_revision` SHALL be the alias reply's for `entity`, the walk's for `path` and the
responder envelope's for `temporal` and `spatial`, and `min_revision` SHALL reach each derived-index read.

#### Scenario: A revision far ahead

- **WHEN** `globalSearch` carries a `min_revision` far above the index's revision
- **THEN** the caller receives `revision_not_reached` with its class and detail
  (`TestMinRevisionRefusalThroughGraphQuery`)

## MODIFIED Requirements

### Requirement: Requests are counted for operators

Each request SHALL count once in `DataFlow` on entry, and one error in `Health` when `graph-query` refuses: no
downstream answer; a composed verb's reply that does not decode, names no `producer` or is not JSON; a broken prefix
page; an unreadable community record; a `localSearch` that cannot read `max_payload` or fit its reply even with no
entities; or a global search whose classification ended with the request. A responder's classified refusal, a
caller's `invalid_request`, an unusable community view and global search's `text_search_unsupported` SHALL NOT count as
an error.

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

#### Scenario: A question this build cannot answer

- **WHEN** `globalSearch` refuses `text_search_unsupported`
- **THEN** the request is one in `DataFlow` and no error in `Health` (`TestGlobalSearchTextTierRefusal`)
