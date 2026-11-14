# composed-stack Delta

## MODIFIED Requirements

### Requirement: The graph stack boots from one configuration

graph-ingest, graph-index, graph-clustering, graph-lexical, graph-query, graph-gateway and an applied-state
`websocket` output SHALL start together from one JSON configuration through `config.Manager`, `ServiceManager` and
`ComponentManager`, with no graph-embedding, and SHALL answer the fixed questions about entities a producer
published on `entity.>`, over NATS and over graph-gateway's `/graph-gateway/graphql` route, and SHALL stream those
entities to a websocket client.

#### Scenario: Answers from one configuration

- **WHEN** the stack boots from the configuration and the producer publishes the fixed entities
- **THEN** once graph-ingest's consumer has acknowledged the last publish, graph-ingest's readiness key reads not
  degraded, the graph-index, graph-clustering and graph-lexical keys report an indexed revision at or above the
  `ENTITY_STATES` revision read after it, `GRAPH_STATUS` holds no `graph-embedding` key, and `entity`,
  `relationships`, `pathSearch`, `summary` and `entityByAlias` return the expected answers
- **AND** graph-lexical's `search`, once its `indexed_revision` reaches that revision, answers a word only one
  entity's name holds with that entity alone, and a word every base's name holds with exactly the live bases
- **AND** once graph-lexical's `indexed_revision` reaches that revision, `localSearch` on the lead and a global
  search on a word of the fixed names answer the same entities as graph-lexical's `search` for that word, in its
  order, the lead's group named in `groups`; once the lead's level-0 record's `entity_revision` reaches it too, that
  group's `record_state` is `read`
- **AND** `fusion.Engine` over `fusionnats` answers a word question with the live entities whose name holds the
  word, with provenance `lexical`
- **AND** `entity`, `entitiesByPrefix`, `entitiesByPredicate` and `pathSearch` posted to `/graph-gateway/graphql` on
  the process's HTTP port answer HTTP 200 with no `errors` and the fixed entities' IDs and triples
- **AND** a websocket client connected to the output's port receives a `put` for each fixed entity, whose payload is
  the `ENTITY_STATES` value at that revision, and `synced`
- **AND** after an edit and a delete the answers follow, a deleted entity's word answers nothing from graph-lexical,
  `localSearch`, global search or fusion, and the websocket client's folded frames match `ENTITY_STATES` read back
  after them (`TestSmoke_OneConfigAnswers`)

### Requirement: A whole-stack restart keeps the answers

After the stack stops cleanly and starts again on the same NATS server, it SHALL reach ready with the same answers,
community answers included, and SHALL apply what is published after the restart. graph-lexical's `search` replies
SHALL be equal before the stop and after the restart, hits, order and scores, for the same entities.

#### Scenario: Stop and start the stack

- **WHEN** the stack process stops on SIGTERM and a new process boots from the same configuration
- **THEN** before any new write, graph-lexical's replies to the fixed words equal those before the stop
  (`TestSmoke_StackRestartKeepsAnswers`)
- **AND** after a new entity and an edit are published, the graph-index, graph-clustering and graph-lexical keys
  reach their revision, the lead's level-0 record's `entity_revision` reaches it, graph-ingest's reads not degraded,
  and the answers equal those before the stop plus the new writes (`TestSmoke_StackRestartKeepsAnswers`)

### Requirement: A process killed mid-ingest loses no acknowledged write

When the stack process is killed while graph-ingest is applying a batch, every publish the `ENTITY` stream
acknowledged SHALL be present exactly once after a new process boots, and the index, word-index and community records
SHALL catch up.

#### Scenario: Kill mid-ingest

- **WHEN** the process is killed with at least one input applied and at least one not acknowledged, and a new process
  boots
- **THEN** each acknowledged entity answers with each statement once, and readiness and records catch up, the lead's
  level-0 community record included once its `entity_revision` reaches the last revision, with a new entity and an
  edit published after the new process boots (`TestSmoke_KillMidIngestKeepsAcknowledgedWrites`)

### Requirement: A broker restart between runs keeps the answers

With file storage, a stack started after the NATS server restarted SHALL reach ready with the same answers, community
answers included.

#### Scenario: Restart the broker while the stack is stopped

- **WHEN** the stack stops, the NATS server restarts on the same URL, and the stack boots again
- **THEN** after a new entity and an edit are published, the graph-index, graph-clustering and graph-lexical keys
  reach their revision, the lead's level-0 record's `entity_revision` reaches it, graph-ingest's reads not degraded,
  with the same answers plus the new writes (`TestSmoke_BrokerRestartBetweenRuns`)

### Requirement: A running stack recovers from a broker restart

With file storage, a stack that keeps running while the NATS server restarts on the same URL SHALL reach ready again
without a process restart, and SHALL apply what is published after the restart. Because of the restart, no
`GRAPH_STATUS` envelope SHALL report a lost feed or `reset_required`, graph-lexical's included, and
`/components/health` SHALL list all seven components by name, each healthy.

#### Scenario: Restart the broker under the running stack

- **WHEN** the NATS server restarts while the stack runs, and a second batch, an edit and a delete are published after
- **THEN** the graph-index, graph-clustering and graph-lexical keys reach their revision and graph-ingest's reads not
  degraded, with answers, graph-lexical's `search`, `localSearch` and global search included, that include them, no
  envelope reports a lost feed or `reset_required`, and `/components/health` lists graph-ingest, graph-index,
  graph-clustering, graph-lexical, graph-query, graph-gateway and websocket, a total of 7, each healthy
  (`TestSmoke_BrokerRestartUnderRunningStack`)

## ADDED Requirements

### Requirement: fusionnats imports no embedding code

`pkg/fusion/fusionnats` SHALL import neither `graph/embedding`, `processor/graph-embedding` nor `graph/readiness`,
directly or through a package it imports.

#### Scenario: An embedding import

- **WHEN** a package `fusionnats` imports reaches `graph/embedding`
- **THEN** `TestFusionNATSImportsNoEmbedding` fails naming the import path

## REMOVED Requirements

### Requirement: The graph stack answers fusion without an embedder

**Reason**: The main stack composes no graph-embedding (design D7), and a word question answers from graph-lexical
(design D2), so the separate three-component stack checks nothing the main one does not. Its import rule moves to
"fusionnats imports no embedding code"; the word question moves to "The graph stack boots from one configuration".
