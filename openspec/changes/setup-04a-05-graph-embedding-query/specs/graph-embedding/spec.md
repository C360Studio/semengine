# graph-embedding

## ADDED Requirements

### Requirement: The embedding verbs are served from Start until Stop

graph-embedding SHALL serve `search` and `similar` on the subjects their `graph.QueryVerbs()` rows name, and on no
other subject, from `Start` until `Stop` returns.

#### Scenario: After Stop

- **WHEN** `Stop` has returned
- **THEN** a request on either subject finds no responder (`TestNoResponderAfterStop`)

### Requirement: The embedding verbs answer in the envelope

Each answer SHALL be a `graph.QueryResponse` with producer `graph-embedding` and `indexed_revision` 0, whose `data` is
a `graph.SemanticData` (with `embedder_type` `bm25`) or a `graph.SimilarData`. Its `results` SHALL be a JSON array,
its `unreadable_records` SHALL be the number of `EMBEDDING_INDEX` records the cache could not decode, and its
`failed_records` the number of entities whose latest embedding failed in the Worker.

#### Scenario: An empty index

- **WHEN** a `search` request arrives after the first build with no generated record
- **THEN** the reply's `data.results` is `[]` and `indexed_revision` is 0 (`TestSearchAnswersEnveloped`)

### Requirement: Requests are decoded strictly

A `search` or `similar` request holding a key its type does not define, or data after the JSON object, SHALL be
refused `invalid_request` naming what was refused.

#### Scenario: An unknown key

- **WHEN** a `search` request holds a key `SemanticRequest` does not define
- **THEN** it is refused `invalid_request` naming the key (`TestRequestsDecodeStrictly`)

### Requirement: The embedding verbs claim no revision

Both request types SHALL accept `min_revision`. A `min_revision` of 0 SHALL be answered with `indexed_revision` 0; a
`min_revision` above 0 SHALL be refused class invalid, `invalid_request`, never answered.

#### Scenario: A caller asks for its write

- **WHEN** a `search` request, sent directly or through graph-query's `semantic` forward, holds `min_revision` 7
- **THEN** it is refused class invalid, `invalid_request` (`TestMinRevisionAboveZeroRefused`,
  `TestSemanticForwardsMinRevision`)

### Requirement: The limit is bounded

On both verbs a `limit` of 0 SHALL mean 10, a `limit` from 1 to 100 SHALL be used as sent, and any other value
SHALL be refused `invalid_request` naming `limit` and the bound.

#### Scenario: Limit edges

- **WHEN** `limit` is 0, 1, 100, 101 or -1
- **THEN** 0 returns at most 10 hits, 1 and 100 at most that many, and 101 and -1 are refused (`TestSearchLimitEdges`)

### Requirement: Hits are ranked before the limit

Every hit SHALL score above 0. The full match set SHALL be sorted by similarity, then by entity ID, before the limit
applies. The entity `similar` names SHALL never be one of its own hits.

#### Scenario: A query of stopwords only

- **WHEN** `query` holds only stopwords
- **THEN** the reply's `results` is `[]` (`TestSearchQueryEdges`)

#### Scenario: Ties across the limit

- **WHEN** several entities tie on similarity across the limit
- **THEN** the hits kept are those with the smallest entity IDs among the tied, the same on every run
  (`TestTopKAtTie`)

### Requirement: Search checks its query and scope first

`search` SHALL refuse a blank `query`, and any `scope` element that is not empty and fails
`types.ValidateEntityIDPrefix`, with `invalid_request` before the query is embedded. An empty `scope` element SHALL
match every entity, and `scope` SHALL filter candidates before they are ranked.

#### Scenario: A malformed scope

- **WHEN** `scope` holds a prefix that fails `types.ValidateEntityIDPrefix`
- **THEN** the request is refused `invalid_request` and the embedder's statistics are unchanged
  (`TestSearchScopeEdges`)

#### Scenario: A scope that matches nothing

- **WHEN** `scope` names a prefix no generated record falls under
- **THEN** the reply's `results` is `[]` (`TestSearchScopeEdges`)

### Requirement: Similar needs a generated source vector

`similar` SHALL refuse a malformed `entity_id` with `invalid_request`. It SHALL refuse class invalid, code
`embedding_unavailable`, when the entity has no record, its record is not generated, or its record has no vector. A
source record that cannot be decoded SHALL be refused class invalid with no code.

#### Scenario: Similar to an unknown entity

- **WHEN** `similar` names a well-formed entity ID with no record, or one whose record is pending
- **THEN** it is refused class invalid, code `embedding_unavailable` (`TestSimilarEdges`)

### Requirement: The embedding verbs are withheld until they can answer

Before answering, each verb SHALL ask the follower's `QueryReady`: a latched reset SHALL be refused fatal
`graph_state_reset_required`, and a lost feed, an incomplete first build or a held hop-1 failure (`delete_failed`,
`pending_write_failed`, `entity_read_failed`) SHALL be refused transient `index_not_ready`. A Worker failure SHALL NOT
withhold an answer. A vector cache that has not applied its initial snapshot, or whose watch is lost, SHALL
be refused transient `index_not_ready`.

#### Scenario: Before the first build

- **WHEN** a request arrives before the follower's first build is complete
- **THEN** it is refused transient `index_not_ready`, and the same request answers once the build completes
  (`TestQueriesWithheldUntilReady`)

#### Scenario: A hop-1 failure

- **WHEN** the failed map holds one `delete_failed` entry after the first build
- **THEN** both verbs are refused transient `index_not_ready` until that entity converges
  (`TestQueriesWithheldUntilReady`)

#### Scenario: A document whose text cannot be extracted

- **WHEN** the failed map holds one `content_error` entry and no hop-1 failure
- **THEN** `search` answers from the other vectors with `failed_records` 1 (`TestWorkerFailureAnswersAndCounts`)

### Requirement: A lost cache watch is a stuck state

A vector cache watch lost while the component runs SHALL be named in `Health` as
`vector cache watch lost; restart the process`, and `Healthy` SHALL be false until the process restarts.

#### Scenario: The cache's watch is lost

- **WHEN** `EMBEDDING_INDEX` is deleted while the component runs
- **THEN** `Health` names the lost cache watch, `Healthy` is false, and both verbs are refused `index_not_ready`
  (`TestHealthNamesLostCacheWatch`)
