# graph-query

Written for design D1 option (a), recommended; if the owner rules (b), the "unhonoured" clauses become refusals.

## ADDED Requirements

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

`label` SHALL be the first non-empty string value, in triple order, of the entity's most salient label predicate: the
lowest priority number in `vocabulary.DiscoverLabelPredicates()`, read at `Start`, ties by predicate name. It SHALL
be omitted when the entity has none.

#### Scenario: The label

- **WHEN** an entity holds `dc.terms.title` and an unregistered display-name predicate
- **THEN** its label is the title, and with neither it has no label (`TestMatchLabel`)

### Requirement: The text reply declares what it could not read

`graph.TextSearchData` SHALL carry `level`, `groups` (each record's `community_id`, `member_count`,
`community_summary` and `keywords`), `matches`, `entities_scanned`, `entities_truncated`, `unreadable_groups` (the
poisoned records at the level), `unhonoured_hints` and `answer`, a plain text built from these fields that says it is
a sample when `entities_truncated` is true. An answer with `unreadable_groups` above 0 SHALL count one error in
`Health`.

#### Scenario: A corrupt record

- **WHEN** one record at the level is unreadable and another matches
- **THEN** the reply answers with `unreadable_groups` 1 and `Health` counts one error; after a valid write to that key
  `unreadable_groups` is 0 (`TestTextTierPoisonIsDeclared`)

#### Scenario: A cut answer says so

- **WHEN** a selected group has more than one page
- **THEN** `entities_truncated` is true and the answer says it is a sample (`TestTextTierAnswer`)

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

## MODIFIED Requirements

### Requirement: Refusals that are not errors

A responder's classified refusal, a caller's `invalid_request`, an unusable community view or empty level, and global
search's `entity_not_found` SHALL NOT count as an error in `Health`; each request still counts once in `DataFlow` on
entry.

#### Scenario: A question this build cannot answer

- **WHEN** `globalSearch` refuses a count question `invalid_request`
- **THEN** the request is one in `DataFlow` and no error in `Health` (`TestGlobalSearchRoutesByHints`)

### Requirement: Global search answers by its hints

`globalSearch` SHALL classify a question of more than one token once with the keyword classifier and answer: a path
intent with a start node and no other hint, `path`; a time range alone, `temporal`; an area alone, `spatial`; no
hint, or only path words with no start, similarity or ranking, `text`, each such hint named in `unhonoured_hints`. A
count or other aggregation hint, with or without others, SHALL be refused `invalid_request` naming it, and so SHALL a
text-routed hint beside a filter (a start, a time range or an area) or a start carrying path predicates, naming
both. No answer
SHALL drop a hint the classifier reported without naming it. No result and no error, hints that cannot be read, and
a classifier error not of class invalid SHALL be refused transient `internal`.

#### Scenario: Routing

- **WHEN** `globalSearch` is asked "what is connected to sensor-001", "what changed in the last 2 hours", "what is
  within 5km of 40.7,-74.0" and "forklift hydraulic leak"
- **THEN** the replies' `strategy` is `path`, `temporal`, `spatial` and `text` (`TestGlobalSearchRoutesByHints`)

#### Scenario: A hint no strategy honours

- **WHEN** "find sensors similar to pump-42" or "top 5 sensors" is asked
- **THEN** the reply's `strategy` is `text` and `unhonoured_hints` names `similarity` or `ranking`
  (`TestGlobalSearchRoutesByHints`)

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
their verbs return to a `graph.TemporalRangeRequest` or `graph.SpatialBoundsRequest` with `limit` 100; `text`, the
`graph.TextSearchData` of the text tier.

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

### Requirement: Truncated searches are counted

An answer with `entities_truncated` true SHALL add one to a counter labelled by the verb asked (`localSearch`,
`globalSearch` or `searchGraph`), registered on the metrics registry graph-query is given; given none, it SHALL
register nothing.

#### Scenario: A truncated answer

- **WHEN** a `localSearch`, `globalSearch` or `searchGraph` answer is truncated
- **THEN** the counter for that verb rises by one (`TestTruncatedSearchIsCounted`)

## REMOVED Requirements

### Requirement: Questions needing the text search are refused until it exists

**Reason:** the text tier exists; its routing is "Global search answers by its hints" and "The text tier answers a
question no strategy routes". **Migration:** `text_search_unsupported` is never sent; `processor/graph-query/MIGRATION.md`
names what each question gets now, and #479's "the area of the plant" is `entity_not_found`.
