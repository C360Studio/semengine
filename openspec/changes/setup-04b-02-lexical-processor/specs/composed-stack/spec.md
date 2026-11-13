# composed-stack Delta

## MODIFIED Requirements

### Requirement: The graph stack boots from one configuration

graph-ingest, graph-index, graph-embedding, graph-clustering, graph-lexical, graph-query, graph-gateway and an
applied-state `websocket` output SHALL start together from one JSON configuration through `config.Manager`,
`ServiceManager` and `ComponentManager`, and SHALL answer the fixed questions about entities a producer published on
`entity.>`, over NATS and over graph-gateway's `/graph-gateway/graphql` route, and SHALL stream those entities to a
websocket client.

#### Scenario: Answers from one configuration

- **WHEN** the stack boots from the configuration and the producer publishes the fixed entities
- **THEN** once graph-ingest's consumer has acknowledged the last publish, graph-ingest's readiness key reads not
  degraded, the graph-index, graph-embedding, graph-clustering and graph-lexical keys report an indexed revision at or
  above the `ENTITY_STATES` revision read after it, `entity`, `relationships`, `pathSearch`, `summary` and
  `entityByAlias` return the expected answers, and each entity has one generated embedding record
- **AND** `semantic` on a word only one entity's name holds answers that entity alone, and `similar` answers the other
  entities whose names share a word with the entity's, as a set, since BM25's scores depend on arrival order
- **AND** graph-lexical's `search`, once its `indexed_revision` reaches that revision, answers a word only one
  entity's name holds with that entity alone, and a word every base's name holds with exactly the live bases
- **AND** once the lead's level-0 record's `entity_revision` reaches that revision, `localSearch` on the lead answers
  from that group, and a text question on a word of the fixed names answers that group
- **AND** `entity`, `entitiesByPrefix`, `entitiesByPredicate` and `pathSearch` posted to `/graph-gateway/graphql` on
  the process's HTTP port answer HTTP 200 with no `errors` and the fixed entities' IDs and triples
- **AND** a websocket client connected to the output's port receives a `put` for each fixed entity, whose payload is
  the `ENTITY_STATES` value at that revision, and `synced`
- **AND** after an edit and a delete the answers and records follow, a deleted entity's word answers nothing from
  graph-lexical, and the websocket client's folded frames match `ENTITY_STATES` read back after them
  (`TestSmoke_OneConfigAnswers`)

### Requirement: A whole-stack restart keeps the answers

After the stack stops cleanly and starts again on the same NATS server, it SHALL reach ready with the same answers,
community answers included, and the same set of embedding records, and SHALL apply what is published after the
restart. graph-lexical's `search` replies SHALL be equal before the stop and after the restart, hits, order and
scores, for the same entities.

#### Scenario: Stop and start the stack

- **WHEN** the stack process stops on SIGTERM and a new process boots from the same configuration
- **THEN** before any new write, graph-lexical's replies to the fixed words equal those before the stop
  (`TestSmoke_StackRestartKeepsAnswers`)
- **AND** after a new entity and an edit are published, the graph-index, graph-embedding, graph-clustering and
  graph-lexical keys reach their revision, the lead's level-0 record's `entity_revision` reaches it, graph-ingest's
  reads not degraded, and the answers and record set equal those before the stop plus the new writes
  (`TestSmoke_StackRestartKeepsAnswers`)
