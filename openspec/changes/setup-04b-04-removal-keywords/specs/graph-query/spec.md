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

### Requirement: Configuration is strict and read

Decoding SHALL refuse an unknown key, naming it. `query_timeout_ms` SHALL be the deadline on the wait for each
downstream reply, default 15000; like the handler's context, it does not end a wait to write to a broker that has
stopped reading (ADR-112, "Time"). `embedding_enabled` SHALL be false when left out. A configuration with no ports SHALL
take the default required `graph_queries` NATS-request port on `graph.query.*`, and any other input port shape SHALL be
refused.

#### Scenario: A key the pin ignored

- **WHEN** a configuration sets `max_traversal_depth`
- **THEN** construction fails naming that key (`TestUnknownConfigKeyIsRefused`)

#### Scenario: The timeout is read

- **WHEN** a downstream verb never replies and `query_timeout_ms` is small
- **THEN** the caller receives a transient refusal before its own request timeout (`TestQueryTimeoutIsRead`)

#### Scenario: The embedding key is read

- **WHEN** a configuration sets `embedding_enabled` to `false`, to `true`, to `null`, to `"false"`, or leaves it out
- **THEN** it reads false, true, false, is refused at construction, and false (`TestConfigEmbeddingEnabled`)
