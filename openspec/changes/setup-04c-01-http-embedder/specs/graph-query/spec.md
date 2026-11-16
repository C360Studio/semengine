# graph-query Delta

## MODIFIED Requirements

### Requirement: searchGraph answers as global search

When `embedding_enabled` is false, `graph.query.searchGraph` SHALL decode and answer every request as `globalSearch`
does, with the same `data`, and SHALL make no semantic request. When it is true, searchGraph SHALL answer as
`globalSearch` when that answer has text matches. When it has none, searchGraph SHALL ask the embedding `search` verb
and answer the global reply with `fallback` holding reason `no_matches` and either the embedding reply's data or the
refusal's code and message. When `globalSearch` is refused `index_not_ready`, searchGraph SHALL do the same with
reason `index_not_ready` and an empty text, and if the embedding search also fails it SHALL return the
`index_not_ready` refusal. Any other refusal SHALL be returned with no semantic request.

#### Scenario: The same question

- **WHEN** `embedding_enabled` is false and one request is sent to `searchGraph` and to `globalSearch`
- **THEN** both replies' `data` are equal and no semantic request is made (`TestSearchGraphAnswersAsGlobalSearch`,
  `TestSearchGraphTierZeroAsksNoEmbedding`)

#### Scenario: No word matches at tier 1

- **WHEN** `embedding_enabled` is true and the word search matches nothing
- **THEN** the reply carries `fallback` with reason `no_matches` and the embedding search's data
  (`TestSearchGraphFallsBackWhenNoMatches`)

#### Scenario: The fallback fails

- **WHEN** the word search matches nothing and the embedding search is refused
- **THEN** the reply carries `fallback` with the refusal's code and message, never an unlabelled empty answer
  (`TestSearchGraphNamesAFailedFallback`)

## ADDED Requirements

### Requirement: Losing the embedding provider keeps word search

With graph-embedding running on an outside provider, losing the provider SHALL leave the word searches answering as
before. The embedding verbs SHALL refuse rather than answer empty, and graph-embedding's readiness SHALL read
`degraded`, naming the provider failure among `failed_reasons`.

#### Scenario: The provider goes away

- **WHEN** the provider is closed and an entity is then written
- **THEN** globalSearch and searchGraph find it by its words, `semantic` is refused, and graph-embedding reads
  `degraded` with `connection_refused` counted (`TestLosingTheProviderKeepsWordSearch`)
