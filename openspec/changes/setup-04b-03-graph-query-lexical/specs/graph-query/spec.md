# graph-query Delta

## ADDED Requirements

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
order of its best match, each with `community_id`, `matches` (its match count), `record_state` and, when read, its
record's `member_count`, `community_summary` and `keywords`. `unreadable_groups` SHALL count the groups whose record
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
does not re-encode. A hit the batch names missing SHALL be left out and counted in `missing`. When the reply would
exceed `max_payload`, the lowest-ranked matches SHALL be left out until it fits. A batch refusal SHALL reach the
caller as it arrived.

#### Scenario: Order and missing hits

- **WHEN** the batch returns the entities in another order and names one hit missing
- **THEN** the matches follow the hits' order without it, `missing` is 1 and `truncated` is true
  (`TestSearchMatchesKeepHitOrder`, `TestSearchDropsMissingHits`)

#### Scenario: Entities pass unchanged

- **WHEN** the batch sends a matching entity holding a value graph-query does not read, such as an integer above 2^53
- **THEN** that entity reaches the caller as the batch sent it (`TestLocalSearchEntitiesPassUnchanged`)

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
  (`TestWordIndexRefusalComesFirst`)

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
matching entities in {community_id}.` for local search, followed by a space and `Showing the first {n}.` when
`truncated` is true, and, for local search with a record read, a space and the record's summary.

#### Scenario: A cut answer

- **WHEN** 40 entities match and the limit is 10
- **THEN** the answer is `Found 40 matching entities in 3 groups. Showing the first 10.` (`TestSearchAnswer`)

### Requirement: Search quality holds against a plain reference

On the word index's frozen scorecard corpus and its 500 questions, `globalSearch` and `localSearch` (each question's
answer's level-0 group) SHALL each have the answer in the first ten at most one point below a plain BM25 computed in
the test, and at or above 55% and 58% respectively.

#### Scenario: The scorecard

- **WHEN** the corpus is ingested and the questions asked through both verbs
- **THEN** both bars hold, and the test logs R@1, R@10 and MRR for both verbs and the reference
  (`TestSearchScorecard`)

## MODIFIED Requirements

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

A responder's classified refusal, a caller's `invalid_request`, a not-honoured hint, a search answered without
community records, and `not_enabled` SHALL NOT count as an error in `Health`; each request still counts once in
`DataFlow` on entry.

#### Scenario: A question this build cannot answer

- **WHEN** `globalSearch` answers a count question with its aggregation hint not honoured
- **THEN** the request is one in `DataFlow` and no error in `Health` (`TestEveryQuestionIsSearched`)

#### Scenario: A verb turned off

- **WHEN** `semantic` is refused `not_enabled`
- **THEN** the request is one in `DataFlow` and no error in `Health` (`TestNotEnabledRefusesBeforeForwarding`)

### Requirement: A request's cost is bounded

`pathSearch` SHALL make at most 1 + `max_nodes` downstream requests, or 1 + 2 × `max_nodes` for `both`.
`hierarchyStats` SHALL make exactly one prefix request, and `summary` one prefix request and, unless predicates are
excluded, one `predicateList` request first. `localSearch` SHALL make one word search and at most one batch request;
`globalSearch` the same, plus at most one alias and one entity request for its reference and one path search. A long
request on one verb SHALL NOT stop another verb answering.

#### Scenario: Counting the requests

- **WHEN** counting subscribers sit beside the real responders
- **THEN** the counts are within those bounds (`TestPathSearchRequestsAreBounded`, `TestCountsReadOnePage`,
  `TestSearchRequestsAreBounded`)

#### Scenario: A walk that is held

- **WHEN** a `pathSearch` waits on a responder that does not answer
- **THEN** a request on `entity` is answered (`TestSlowPathSearchLeavesOtherVerbsAnswering`)

### Requirement: The community view follows the bucket

graph-query's `Start` SHALL neither wait for nor fail on a missing `COMMUNITY_INDEX`. While the bucket is missing, or
its view is catching up or lost, searches SHALL answer with `record_state` `not_ready` and the other verbs SHALL
answer. Once the bucket exists, searches SHALL attach records without a restart; after a lost watch they SHALL never
attach a record from the old projection. During a tick they SHALL attach each group's latest record. `Stop` SHALL
return only after the view has stopped.

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

### Requirement: One resolver reads an entity reference

`entityByAlias` and global search SHALL resolve a reference the same way: the alias verb's holder; else the reference
itself when it is a valid entity ID; else not found, with no second request. `entityByAlias` SHALL answer not found as
`entity_not_found`; global search SHALL report it as a not-honoured hint with code `entity_not_found`. A refusal from
the alias or entity verb SHALL reach an `entityByAlias` caller with its class, code and detail, and SHALL be a global
search hint's `code`. No suffix lookup SHALL be made.

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

### Requirement: An answer past unreadable records is signalled

An answer with `unreadable_groups` above 0, or a local search with `record_state` `unreadable`, SHALL count one error
in `Health` and log one Warn naming the verb, the level, the count and why answering is safe.

#### Scenario: A corrupt record

- **WHEN** one group record among the matches' groups is unreadable
- **THEN** the reply answers with `unreadable_groups` 1, `Health` counts one error, and one Warn names the verb, the
  level and the count; after a valid write to that key `unreadable_groups` is 0 and nothing is logged
  (`TestTextTierPoisonIsDeclared`, renamed `TestUnreadableRecordIsDeclared`)

## REMOVED Requirements

### Requirement: Local search answers from one group's record and one page

**Reason**: Local search is a word search scoped to the group (design D2); the record decorates and no prefix page is
read. The reply's fields move to "Local search is scoped to the group"; "Entities pass unchanged" and the payload fit
move to "Matches are the hits read back as stored"; the one-page scenarios are gone with the page.

### Requirement: Local search ranks by the pin's text score

**Reason**: The word index ranks (design D1); the substring score is deleted. Replaced by "Local and global search
are word searches" and "Search quality holds against a plain reference".

### Requirement: Local search answers in a fixed template

**Reason**: `entities_scanned` no longer exists. Replaced by "Search answers in a fixed template".

### Requirement: Local search refuses what it cannot answer

**Reason**: A missing or unreadable record no longer refuses (design D2); it is a `record_state`. The bad-request
refusals (unknown key, invalid `entity_id`, `level` 3, empty query) move to "Search requests decode strictly"; the
word index's refusals are in "Local and global search are word searches".

### Requirement: Global search answers by its hints

**Reason**: Every question gets a word search and hints are reported (design D5). Replaced by "Every question gets a
word search".

### Requirement: Global search refuses hints it cannot honour together

**Reason**: No question is refused for its words (plan item 3, #616). Replaced by "Every question gets a word search".

### Requirement: A one-word question is an entity reference

**Reason**: A one-token question is an `entity` hint beside the word search. Replaced by "Every question gets a word
search".

### Requirement: Global search refuses what no strategy can honour

**Reason**: A blank query, a `level` outside 0 to 2 and the pin's keys move to "Search requests decode strictly". Two
filters, an impossible number and an entity beside a place no longer refuse: they are not-honoured hints under "Every
question gets a word search" (design D5).

### Requirement: A strategy answers with its own result

**Reason**: The reply always holds `text`, with `entity` or `path` added for a verified hint; `strategy`, `temporal`
and `spatial` are removed. Replaced by "Every question gets a word search".

### Requirement: A strategy's revision is its read's

**Reason**: Replaced by "A search reply's revision is the word index's".

### Requirement: The text tier answers a question no strategy routes

**Reason**: There is no routing to the text tier; every question is searched.

### Requirement: The text tier reads at most five matching groups, one page each

**Reason**: Communities stop being the gate (design D3). Replaced by "Global search groups its matches by community".

### Requirement: Text matches are the pages' entities as stored, best first

**Reason**: Replaced by "Matches are the hits read back as stored".

### Requirement: The text reply declares what it could not read

**Reason**: `entities_scanned` and `unhonoured_hints` are removed; the fields are in "Global search groups its matches
by community", "Matches are the hits read back as stored" and "Every question gets a word search".

### Requirement: The text tier refuses an index it cannot read

**Reason**: A missing community view no longer refuses (design D3); the word index's refusals pass through.

### Requirement: The text tier's revision is its level's highest readable mark

**Reason**: Replaced by "A search reply's revision is the word index's".
