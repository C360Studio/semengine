# graph-query Delta

## MODIFIED Requirements

### Requirement: Embedding verbs can be turned off

When `embedding_enabled` is `false`, which it is when the key is left out, `semantic` and `similar` SHALL refuse every
request with class `invalid` and code `not_enabled`, the message naming the verb, without sending any request. Every
other verb SHALL be unchanged by the key.

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
