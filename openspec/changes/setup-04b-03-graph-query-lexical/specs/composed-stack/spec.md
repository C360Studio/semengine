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
- **AND** once graph-lexical's `indexed_revision` reaches that revision, `localSearch` on the lead and a global
  search on a word of the fixed names answer the same entities as graph-lexical's `search` for that word, in its
  order, the lead's group named in `groups`; once the lead's level-0 record's `entity_revision` reaches it too, that
  group's `record_state` is `read`
- **AND** `entity`, `entitiesByPrefix`, `entitiesByPredicate` and `pathSearch` posted to `/graph-gateway/graphql` on
  the process's HTTP port answer HTTP 200 with no `errors` and the fixed entities' IDs and triples
- **AND** a websocket client connected to the output's port receives a `put` for each fixed entity, whose payload is
  the `ENTITY_STATES` value at that revision, and `synced`
- **AND** after an edit and a delete the answers and records follow, a deleted entity's word answers nothing from
  graph-lexical, `localSearch` or global search, and the websocket client's folded frames match `ENTITY_STATES` read
  back after them (`TestSmoke_OneConfigAnswers`)

### Requirement: A running stack recovers from a broker restart

With file storage, a stack that keeps running while the NATS server restarts on the same URL SHALL reach ready again
without a process restart, and SHALL apply what is published after the restart. Because of the restart, no
`GRAPH_STATUS` envelope SHALL report a lost feed or `reset_required`, graph-lexical's included, and
`/components/health` SHALL list all eight components by name, each healthy (graph-embedding's `embedding worker
watch lost` included).

#### Scenario: Restart the broker under the running stack

- **WHEN** the NATS server restarts while the stack runs, and a second batch, an edit and a delete are published after
- **THEN** the graph-index, graph-embedding, graph-clustering and graph-lexical keys reach their revision and
  graph-ingest's reads not degraded, with answers, graph-lexical's `search`, `localSearch` and global search included,
  that include them, no envelope reports a lost feed or `reset_required`, and `/components/health` lists
  graph-ingest, graph-index, graph-embedding, graph-clustering, graph-lexical, graph-query, graph-gateway and
  websocket, a total of 8, each healthy (`TestSmoke_BrokerRestartUnderRunningStack`)
