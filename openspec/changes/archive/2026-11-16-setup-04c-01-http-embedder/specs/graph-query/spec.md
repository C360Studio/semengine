# graph-query Delta

## MODIFIED Requirements

### Requirement: searchGraph answers as global search

When `embedding_enabled` is false, `graph.query.searchGraph` SHALL decode and answer every request as `globalSearch`
does, with the same `data`, and SHALL make no semantic request. When it is true, searchGraph SHALL answer as
`globalSearch` when that answer's `text` has matches. When it has none, searchGraph SHALL ask the embedding `search`
verb once and answer the global reply with `fallback`, a hint entry with `hint` `semantic` and `detail` `no_matches`:
on success `honoured` true and `semantic` holding the embedding reply's data as it came; on refusal or timeout
`honoured` false with the refusal's `code` and its message as `reason`. When `globalSearch` is refused
`index_not_ready`, searchGraph SHALL do the same with `detail` `index_not_ready` and SHALL leave `text` out (null),
since the word index was not read; if the embedding search also fails it SHALL return the `index_not_ready`
refusal. An empty `text` SHALL mean the word index was read and matched nothing. The fallback SHALL wait no longer
than `query_timeout_ms`. Any other refusal SHALL be returned with no semantic request.

#### Scenario: The same question

- **WHEN** `embedding_enabled` is false and one request is sent to `searchGraph` and to `globalSearch`
- **THEN** both replies' `data` are equal and no semantic request is made (`TestSearchGraphAnswersAsGlobalSearch`,
  `TestSearchGraphTierZeroAsksNoEmbedding`)

#### Scenario: No word matches at tier 1

- **WHEN** `embedding_enabled` is true and the word search matches nothing
- **THEN** the reply holds the empty `text`, `semantic` with the embedding search's data, and `fallback` honoured
  with detail `no_matches` (`TestSearchGraphFallsBackWhenNoMatches`)

#### Scenario: The fallback fails

- **WHEN** the word search matches nothing and the embedding search is refused
- **THEN** the reply holds the empty `text` and `fallback` not honoured, with the refusal's code and message, never an
  unlabelled empty answer (`TestSearchGraphNamesAFailedFallback`)

#### Scenario: The fallback runs out of time

- **WHEN** the word search matches nothing and the embedding search does not answer within `query_timeout_ms`
- **THEN** the reply holds the word answer and `fallback` not honoured, with code `internal`
  (`TestSearchGraphFallbackTimeoutKeepsWordAnswer`)

#### Scenario: The word index is not ready

- **WHEN** `globalSearch` is refused `index_not_ready` and the embedding search answers
- **THEN** the reply leaves `text` out (null), holds `semantic`, and `fallback` honoured with detail `index_not_ready`;
  had the embedding search failed, the `index_not_ready` refusal is returned
  (`TestSearchGraphIndexNotReadyLeavesTextOut`)

## ADDED Requirements

### Requirement: Losing the embedding provider keeps word search

With graph-embedding running on an outside provider, losing the provider SHALL leave the word searches answering as
before. The embedding verbs SHALL refuse rather than answer empty, and graph-embedding's readiness SHALL read
`degraded`, naming the provider failure among `failed_reasons`.

#### Scenario: The provider goes away

- **WHEN** the provider is closed and an entity is then written
- **THEN** globalSearch and searchGraph find it by its words, `semantic` is refused, and graph-embedding reads
  `degraded` with `connection_refused` counted (`TestLosingTheProviderKeepsWordSearch`)
