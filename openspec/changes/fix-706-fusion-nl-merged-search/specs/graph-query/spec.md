# graph-query Delta

## MODIFIED Requirements

### Requirement: Embedding verbs can be turned off

When `embedding_enabled` is `false`, which it is when the key is left out, `semantic` and `similar` SHALL refuse every
request with class `invalid` and code `not_enabled`, the message naming the verb, without sending any request. Every
other verb SHALL be unchanged by the key, except `searchGraph` and `mergedSearch`, whose merge runs only when the key
is true.

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

## ADDED Requirements

### Requirement: mergedSearch scopes both searches

`graph.query.mergedSearch` SHALL decode a `graph.LexicalSearchRequest` strictly, refusing `invalid_request` before
any request. It SHALL pass `scope` to the word search and the embedding search, and `min_revision` to the word search
alone.

#### Scenario: Scope reaches both searches

- **WHEN** a request holds scope `["acme.ops.docs"]` and `embedding_enabled` is true
- **THEN** the word search and the embedding search each received that scope (`TestMergedSearchScopesBothSearches`)

#### Scenario: A bad request

- **WHEN** a request holds an unknown key, a blank query, a `limit` other than 0 (meaning 10) or 1 to 100, such as
  101, or a scope member that is not an entity-ID prefix, such as `a..b`
- **THEN** it is refused `invalid_request` and no request is sent (`TestMergedSearchDecodesStrictly`)

### Requirement: mergedSearch answers the ranked list without reading it back

`mergedSearch` SHALL answer enveloped `graph.MergedSearchData` and read no entity: `results`, each `entity_id`,
`score`, and `vector_rank` when the entity is in the embedding list; the word search's `excluded` and `bodies_not_indexed`; and `merge` when `embedding_enabled` is true. Its
revision SHALL be the word search's, and its `damaged_entities` the larger of the replies it read.

#### Scenario: Results and counts in the reply

- **WHEN** a tier 1 question matches one entity by words and meaning and another by meaning only
- **THEN** both are in `results` with their merged scores and their `vector_rank`s, the reply carries the word
  search's counts and revision, and no entity read was sent (`TestMergedSearchReplyShape`)

### Requirement: mergedSearch ranks as searchGraph does

For the same query and limit, `mergedSearch`'s result IDs SHALL be searchGraph's match IDs in the same order whenever
searchGraph left no hit missing or damaged and its reply fit `max_payload`. With `embedding_enabled` false it SHALL
answer the word list alone, with no `merge`; with it true, the merged list with `merge` honoured. It SHALL fall back
as the scenarios below state, and return any other word refusal as it arrived.

#### Scenario: Same order as searchGraph

- **WHEN** one question is asked of `mergedSearch` and `searchGraph` at tier 1 and at tier 0
- **THEN** the result IDs equal searchGraph's match IDs in order (`TestMergedSearchMatchesSearchGraphOrder`)

#### Scenario: Tier 0

- **WHEN** `embedding_enabled` is false
- **THEN** only the word search is asked and the reply has no `merge` (`TestMergedSearchTierZeroAsksOnlyWords`)

#### Scenario: A failed merge

- **WHEN** the embedding search is refused or broken
- **THEN** the reply is the word list with `merge` not honoured, carrying the refusal's code, with one Warn naming the
  verb and one count on the merge-failed counter it shares with searchGraph (`TestMergedSearchFailedMergeKeepsWords`)

#### Scenario: A broken embedding hit ID

- **WHEN** an embedding hit's ID is not a literal entity ID
- **THEN** the reply is the word list with `merge` not honoured, counted as a failed merge
  (`TestMergedSearchBrokenEmbeddingIDKeepsWords`)

#### Scenario: A broken word hit ID

- **WHEN** a word hit's ID is not a literal entity ID
- **THEN** `mergedSearch` is refused transient `internal` naming the word search's subject, as searchGraph refuses
  that reply (`TestMergedSearchBrokenWordIDRefused`)

#### Scenario: Another word refusal

- **WHEN** at tier 1 the word search refuses `invalid_request` while the embedding search is in flight
- **THEN** that refusal is returned as it arrived, and the embedding request has ended before the handler returns
  (`TestMergedSearchOtherRefusalIsReturned`)

#### Scenario: The word index is not ready

- **WHEN** the word search is refused `index_not_ready` and the embedding search answers
- **THEN** the results are the embedding list, `merge` is honoured with detail `index_not_ready` and the revision is
  0; if the embedding search fails too, the `index_not_ready` refusal is returned
  (`TestMergedSearchWordIndexNotReady`)

#### Scenario: No word index

- **WHEN** nothing serves `graph.lexical.query.search`
- **THEN** `mergedSearch` is refused transient `internal` naming that subject (`TestSearchWithoutWordIndex`)

### Requirement: mergedSearch bounds its embedding wait

`mergedSearch` SHALL wait for the embedding search at most `query_timeout_ms` or 4 seconds, whichever is less, so
a caller on natsclient's default timeout gets the word answer from a slow embedder rather than a timeout.

#### Scenario: A slow embedding search

- **WHEN** `query_timeout_ms` is 15000 and the embedding search has not answered 4 seconds after it was sent
- **THEN** the reply is the word list with `merge` not honoured (`TestMergedSearchEmbeddingWaitBound`)
