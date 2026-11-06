# graph-query

## ADDED Requirements

### Requirement: Global search answers by its hints

`globalSearch` SHALL classify the question once with the keyword classifier and pick from its hints: a path intent
with a start node and neither a time range nor an area, `path`; a time range alone, `temporal`; an area alone,
`spatial`; no hint and a question of one token, `entity`. No answer SHALL drop a hint the question carried.

#### Scenario: Routing

- **WHEN** `globalSearch` is asked "what is connected to sensor-001", "what changed in the last 2 hours", "what is
  within 5km of 40.7,-74.0" and "sensor-001"
- **THEN** the replies' `strategy` is `path`, `temporal`, `spatial` and `entity` (`TestGlobalSearchRoutesByHints`)

### Requirement: Global search refuses what no strategy can honour

`globalSearch` SHALL decode `query` and `min_revision` strictly, refusing a blank `query` and any other key with
`invalid_request` naming it. Two of path, time range and area together SHALL be refused `invalid_request` naming both,
and a question the classifier refuses for an impossible number SHALL be refused `invalid_request` naming the phrase.
Each refusal SHALL come before any downstream request.

#### Scenario: Two filters

- **WHEN** a question carries a path start and a time range, or a time range and an area
- **THEN** it is refused class invalid, code `invalid_request`, naming both, and no downstream request is made
  (`TestGlobalSearchRefusesTwoFilters`)

#### Scenario: An impossible number

- **WHEN** a question asks for "the last 0 hours"
- **THEN** it is refused class invalid, code `invalid_request`, naming the phrase
  (`TestGlobalSearchRefusesImpossibleNumbers`)

### Requirement: Questions needing the text search are refused until it exists

A question with no strategy in "Global search answers by its hints", and an `entity` or `path` reference that resolves
to no entity, SHALL be refused with class invalid and code `text_search_unsupported`, whether or not `COMMUNITY_INDEX`
holds records. This refusal SHALL NOT count as an error in `Health`. It SHALL never be `index_not_ready` or an empty
success.

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
itself when it is a valid entity ID; else not found, with no second request. A refusal from the alias or entity verb
SHALL reach the caller with its class, code and detail. No suffix lookup SHALL be made.

#### Scenario: An ambiguous alias

- **WHEN** a global search reference is an alias two entities carry
- **THEN** the caller receives `alias_ambiguous` as the alias verb sent it (`TestGlobalSearchPassesAliasRefusals`)

#### Scenario: An unknown reference

- **WHEN** a one-token question is no alias and no entity ID
- **THEN** it is refused `text_search_unsupported` after one alias request
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
