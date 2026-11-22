# graph-query Delta

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

`mergedSearch` SHALL answer enveloped `graph.MergedSearchData` and read no entity: `results` (`entity_id`, `score`,
and `word_rank`/`vector_rank` when in that list); the word search's `matched`, `excluded` and `bodies_not_indexed`;
and `merge` when `embedding_enabled` is true. Its revision SHALL be the word search's, and its `damaged_entities` the
larger of the replies it read.

#### Scenario: Ranks and counts in the reply

- **WHEN** a tier 1 question matches one entity by words and meaning and another by meaning only
- **THEN** the first result carries both ranks, the second only `vector_rank`, the reply carries the word search's
  counts and revision, and no entity read was sent (`TestMergedSearchReplyShape`)

### Requirement: mergedSearch ranks as searchGraph does

For the same query and limit, `mergedSearch`'s result IDs SHALL be searchGraph's match IDs in the same order whenever
searchGraph left no hit missing or damaged and its reply fit `max_payload`. With `embedding_enabled` false it SHALL
answer the word list alone, with no `merge`; with it true, the merged list with `merge` honoured. It SHALL fall back
as the scenarios below state, and return any other word refusal as it arrived, cancelling the embedding search.

#### Scenario: Same order as searchGraph

- **WHEN** one question is asked of `mergedSearch` and `searchGraph` at tier 1 and at tier 0
- **THEN** the result IDs equal searchGraph's match IDs in order (`TestMergedSearchMatchesSearchGraphOrder`)

#### Scenario: Tier 0

- **WHEN** `embedding_enabled` is false
- **THEN** only the word search is asked and the reply has no `merge` (`TestMergedSearchTierZeroAsksOnlyWords`)

#### Scenario: A failed merge

- **WHEN** the embedding search is refused, broken or late past `query_timeout_ms`
- **THEN** the reply is the word list with `merge` not honoured, carrying the refusal's code, with one Warn naming the
  verb and one count on the merge-failed counter (`TestMergedSearchFailedMergeKeepsWords`)

#### Scenario: The word index is not ready

- **WHEN** the word search is refused `index_not_ready` and the embedding search answers
- **THEN** the results are the embedding list, `merge` has detail `index_not_ready` and the revision is 0; if the
  embedding search fails too, the `index_not_ready` refusal is returned (`TestMergedSearchWordIndexNotReady`)

#### Scenario: No word index

- **WHEN** nothing serves `graph.lexical.query.search`
- **THEN** `mergedSearch` is refused transient `internal` naming that subject (`TestSearchWithoutWordIndex`)
