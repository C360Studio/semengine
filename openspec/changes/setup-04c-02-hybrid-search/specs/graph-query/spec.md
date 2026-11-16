# graph-query Delta

## ADDED Requirements

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

- **WHEN** the same question is asked with limit 10 and with limit 100
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
`semantic` verb and `searchGraph`. It SHALL print R@1, R@10 and MRR for each beside the model the provider names,
dimensions, query prefix, commits and timings. It SHALL NOT run in `task verify`.

#### Scenario: A run names its model

- **WHEN** the scorecard runs against a provider
- **THEN** its output names the model the provider's reply reports beside the three rows
  (`notes/probes/scorecard-*.md`, task 3.4)

### Requirement: The tier 1 scorecard holds a floor

`task scorecard:tier1` SHALL exit non-zero when the merged row is below the floor recorded for the run's model or
below either single row.

#### Scenario: Below the floor

- **WHEN** the merged row's R@10 or MRR is below the floor recorded for the run's model
- **THEN** the program prints the three rows and exits 1 (`notes/probes/scorecard-*.md`, task 3.4)

## MODIFIED Requirements

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

### Requirement: Matches are the hits read back as stored

Both verbs SHALL read their hits with one `graph.ingest.query.batch` request and answer `matches` in the word index's
order, each `id`, `label`, `score` (the word index's) and `entity`, the bytes the batch sent for it, which graph-query
does not re-encode. In a searchGraph merged list the order is the merged order and `score` is the merged score. A hit
the batch names missing SHALL be left out and counted in `missing`. When the reply would exceed `max_payload`, the
lowest-ranked matches SHALL be left out until it fits. A batch refusal SHALL reach the caller as it arrived.

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

### Requirement: Search answers in a fixed template

`answer` SHALL be `Found {matched} matching entities in {groups} groups.` for global search and `Found {matched}
matching entities in {community_id}.` for local search. In a searchGraph merged list whose embedding list added
entities the word hits did not hold, `, and {extra} more by meaning` comes before the full stop. Then a space and
`Showing the first {n}.` when `truncated` is true, and, for local search with a record read, a space and the record's
summary.

#### Scenario: A cut answer

- **WHEN** 40 entities match and the limit is 10
- **THEN** the answer is `Found 40 matching entities in 3 groups. Showing the first 10.` (`TestSearchAnswer`)

#### Scenario: A merged answer

- **WHEN** 3 entities match by words and the embedding list adds 2 more
- **THEN** the answer is `Found 3 matching entities in 2 groups, and 2 more by meaning.` (`TestSearchAnswer`)

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

## REMOVED Requirements

### Requirement: searchGraph labels its semantic fallback

**Reason**: at tier 1 the embedding search is merged with every word answer, not asked only when words match nothing
(owner, #668, 2026-11-16).
**Migration**: read `merge` and each match's `word_rank` / `vector_rank`; `fallback` was never released.
