# graph-query Delta

## MODIFIED Requirements

### Requirement: Losing the embedding provider keeps word search

With graph-embedding running on an outside provider, losing the provider SHALL leave the word searches answering as
before. searchGraph SHALL answer the word list with `merge` not honoured. The embedding verbs SHALL refuse rather
than answer empty, and graph-embedding's readiness SHALL read `degraded`, naming the provider failure among
`failed_reasons`. Once the provider answers again, graph-embedding SHALL embed the failed entity and read ready
without a restart.

#### Scenario: The provider goes away

- **WHEN** the provider answers 503 and an entity is then written
- **THEN** globalSearch and searchGraph find it by its words, searchGraph's `merge` is not honoured with its code set,
  `semantic` is refused, and graph-embedding reads `degraded` with `provider_unavailable` counted
  (`TestLosingTheProviderKeepsWordSearch`)

#### Scenario: The provider comes back

- **WHEN** the provider answers again after that entity failed
- **THEN** `semantic` finds the entity and graph-embedding reads ready with no failures, with no restart
  (`TestLosingTheProviderKeepsWordSearch`)
