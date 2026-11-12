# graph-query

## ADDED Requirements

### Requirement: Embedding verbs can be turned off

When `embedding_enabled` is `false`, `semantic` and `similar` SHALL refuse every request with class `invalid` and
code `not_enabled`, the message naming the verb, without sending any request. Every other verb SHALL be unchanged by
the key.

#### Scenario: Not the same as a missing responder or a missing vector

- **WHEN** `embedding_enabled` is true and nothing serves `graph.embedding.query.search`, or one entity has no vector
- **THEN** the caller is refused transient `internal` or `embedding_unavailable` as today, never `not_enabled`
  (`TestNoRespondersIsTransient`)

#### Scenario: Turned off beside a running responder

- **WHEN** graph-embedding is serving and graph-query is configured with `embedding_enabled: false`
- **THEN** `semantic` and `similar` are refused invalid `not_enabled` naming the verb, never answered
  (`TestNotEnabledRefusesBeforeForwarding`, `TestEmbeddingVerbsRefuseWhenNotEnabled`)

#### Scenario: Left out

- **WHEN** a configuration leaves `embedding_enabled` out
- **THEN** it reads true and `semantic` and `similar` forward as before (`TestConfigEmbeddingEnabled`,
  `TestEmbeddingVerbsRefuseWhenNotEnabled`)

## MODIFIED Requirements

### Requirement: Refusals that are not errors

A responder's classified refusal, a caller's `invalid_request`, an unusable community view or empty level, global
search's `entity_not_found`, and `not_enabled` SHALL NOT count as an error in `Health`; each request still counts once
in `DataFlow` on entry.

#### Scenario: A question this build cannot answer

- **WHEN** `globalSearch` refuses a count question `invalid_request`
- **THEN** the request is one in `DataFlow` and no error in `Health` (`TestGlobalSearchRoutesByHints`)

#### Scenario: A verb turned off

- **WHEN** `semantic` is refused `not_enabled`
- **THEN** the request is one in `DataFlow` and no error in `Health` (`TestNotEnabledRefusesBeforeForwarding`)

### Requirement: Configuration is strict and read

Decoding SHALL refuse an unknown key, naming it. `query_timeout_ms` SHALL be the deadline on the wait for each
downstream reply, default 15000; like the handler's context, it does not end a wait to write to a broker that has
stopped reading (ADR-112, "Time"). `embedding_enabled` SHALL be a boolean, true when left out, and a value of any other
type SHALL be refused. A configuration with no ports SHALL take the default required `graph_queries` NATS-request port
on `graph.query.*`, and any other input port shape SHALL be refused.

#### Scenario: A key the pin ignored

- **WHEN** a configuration sets `max_traversal_depth`
- **THEN** construction fails naming that key (`TestUnknownConfigKeyIsRefused`)

#### Scenario: The timeout is read

- **WHEN** a downstream verb never replies and `query_timeout_ms` is small
- **THEN** the caller receives a transient refusal before its own request timeout (`TestQueryTimeoutIsRead`)

#### Scenario: The embedding key is read

- **WHEN** a configuration sets `embedding_enabled` to `false`, to `true`, to `null`, to `"false"`, or leaves it out
- **THEN** it reads false, true, true, is refused at construction, and true (`TestConfigEmbeddingEnabled`)

### Requirement: Derived-index replies are forwarded unchanged

`semantic` and `similar`, while `embedding_enabled` is true, and `spatial` and `temporal` SHALL forward the request
bytes to the verb their table row names and return the reply bytes unchanged. graph-embedding serves the first two;
no responder in this tree serves the last two yet.

#### Scenario: A field graph-query does not read rides through

- **WHEN** a caller of `semantic` sends a request holding `scope`
- **THEN** `graph.embedding.query.search` receives exactly those bytes and the caller receives exactly its reply
  (`TestForwardPassesBytesBothWays`)

#### Scenario: A responder not yet deployed

- **WHEN** a caller of one of the four asks, `embedding_enabled` is true, and nothing serves the verb behind it
- **THEN** the caller receives a transient `internal` refusal naming that subject (`TestNoRespondersIsTransient`)
