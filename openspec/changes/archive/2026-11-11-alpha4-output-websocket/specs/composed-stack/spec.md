# composed-stack

## MODIFIED Requirements

### Requirement: The graph stack boots from one configuration

graph-ingest, graph-index, graph-embedding, graph-clustering, graph-query, graph-gateway and an applied-state
`websocket` output SHALL start together from one JSON configuration through `config.Manager`, `ServiceManager` and
`ComponentManager`, and SHALL answer the fixed questions about entities a producer published on `entity.>`, over NATS
and over graph-gateway's `/graph-gateway/graphql` route, and SHALL stream those entities to a websocket client.

#### Scenario: Answers from one configuration

- **WHEN** the stack boots from the configuration and the producer publishes the fixed entities
- **THEN** once graph-ingest's consumer has acknowledged the last publish, graph-ingest's readiness key reads not
  degraded, the graph-index, graph-embedding and graph-clustering keys report an indexed revision at or above the
  `ENTITY_STATES` revision read after it, `entity`, `relationships`, `pathSearch`, `summary` and `entityByAlias`
  return the expected answers, and each entity has one generated embedding record
- **AND** `semantic` on a word only one entity's name holds answers that entity alone, and `similar` answers the other
  entities whose names share a word with the entity's, as a set, since BM25's scores depend on arrival order
- **AND** once the lead's level-0 record's `entity_revision` reaches that revision, `localSearch` on the lead answers
  from that group, and a text question on a word of the fixed names answers that group
- **AND** `entity`, `entitiesByPrefix`, `entitiesByPredicate` and `pathSearch` posted to `/graph-gateway/graphql` on
  the process's HTTP port answer HTTP 200 with no `errors` and the fixed entities' IDs and triples
- **AND** a websocket client connected to the output's port receives a `put` for each fixed entity, whose payload is
  the `ENTITY_STATES` value at that revision, and `synced`
- **AND** after an edit and a delete the answers and records follow, and the websocket client's folded frames match
  `ENTITY_STATES` read back after them (`TestSmoke_OneConfigAnswers`)

### Requirement: A running stack recovers from a broker restart

With file storage, a stack that keeps running while the NATS server restarts on the same URL SHALL reach ready again
without a process restart, and SHALL apply what is published after the restart. Because of the restart, no
`GRAPH_STATUS` envelope SHALL report a lost feed or `reset_required`, and `/components/health` SHALL list all seven
components by name, each healthy (graph-embedding's `embedding worker watch lost` included).

#### Scenario: Restart the broker under the running stack

- **WHEN** the NATS server restarts while the stack runs, and a second batch, an edit and a delete are published after
- **THEN** the graph-index, graph-embedding and graph-clustering keys reach their revision and graph-ingest's reads not
  degraded, the lead's level-0 record's `entity_revision` reaches it, with answers, `localSearch` included, that include
  them, no envelope reports a lost feed or `reset_required`, and `/components/health` lists graph-ingest, graph-index,
  graph-embedding, graph-clustering, graph-query, graph-gateway and websocket, a total of 7, each healthy
  (`TestSmoke_BrokerRestartUnderRunningStack`)
