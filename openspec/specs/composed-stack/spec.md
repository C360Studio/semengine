# composed-stack Specification

## Purpose
The tier-0 graph stack (graph-ingest, graph-index, graph-embedding, graph-clustering and graph-query) boots from one JSON
configuration through the real startup path, answers fixed questions through the paths a consumer uses, and keeps those answers
across four restarts: the whole stack, a process killed mid-ingest, NATS restarted between runs, and a quick NATS restart
(about 0.2 s down) under the running stack. A second, three-component stack without graph-embedding (the no-embedder
stack) answers symbol and prefix fusion questions and refuses natural-language ones, naming the embedding search
subject. A longer outage is not yet covered (#574). The tests live in `internal/smoke`.

## Requirements

### Requirement: The graph stack boots from one configuration

graph-ingest, graph-index, graph-embedding, graph-clustering and graph-query SHALL start together from one JSON
configuration through `config.Manager`, `ServiceManager` and `ComponentManager`, and SHALL answer the fixed questions
about entities a producer published on `entity.>`.

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
- **AND** after an edit and a delete the answers and records follow (`TestSmoke_OneConfigAnswers`)

### Requirement: A whole-stack restart keeps the answers

After the stack stops cleanly and starts again on the same NATS server, it SHALL reach ready with the same answers,
community answers included, and the same set of embedding records, and SHALL apply what is published after the
restart.

#### Scenario: Stop and start the stack

- **WHEN** the stack process stops on SIGTERM and a new process boots from the same configuration
- **THEN** after a new entity and an edit are published, the graph-index, graph-embedding and graph-clustering keys
  reach their revision, the lead's level-0 record's `entity_revision` reaches it, graph-ingest's reads not degraded, and
  the answers and record set equal those before the stop plus the new writes (`TestSmoke_StackRestartKeepsAnswers`)

### Requirement: A process killed mid-ingest loses no acknowledged write

When the stack process is killed while graph-ingest is applying a batch, every publish the `ENTITY` stream
acknowledged SHALL be present exactly once after a new process boots, and the index, embedding and community records
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
- **THEN** after a new entity and an edit are published, the graph-index, graph-embedding and graph-clustering keys
  reach their revision, the lead's level-0 record's `entity_revision` reaches it, graph-ingest's reads not degraded,
  with the same answers plus the new writes (`TestSmoke_BrokerRestartBetweenRuns`)

### Requirement: A running stack recovers from a broker restart

With file storage, a stack that keeps running while the NATS server restarts on the same URL SHALL reach ready again
without a process restart, and SHALL apply what is published after the restart. Because of the restart, no
`GRAPH_STATUS` envelope SHALL report a lost feed or `reset_required`, and `/components/health` SHALL list all five
components by name, each healthy (graph-embedding's `embedding worker watch lost` included).

#### Scenario: Restart the broker under the running stack

- **WHEN** the NATS server restarts while the stack runs, and a second batch, an edit and a delete are published after
- **THEN** the graph-index, graph-embedding and graph-clustering keys reach their revision and graph-ingest's reads not
  degraded, the lead's level-0 record's `entity_revision` reaches it, with answers, `localSearch` included, that include
  them, no envelope reports a lost feed or `reset_required`, and `/components/health` lists graph-ingest, graph-index,
  graph-embedding, graph-clustering and graph-query, a total of 5, each healthy
  (`TestSmoke_BrokerRestartUnderRunningStack`)

### Requirement: The graph stack answers fusion without an embedder

graph-ingest, graph-index and graph-query SHALL start from one configuration with no graph-embedding, and
`fusion.Engine` over `fusionnats` SHALL answer symbol and prefix questions from them. A natural-language question SHALL
be refused naming `graph.embedding.query.search`, never answered empty. `pkg/fusion/fusionnats` SHALL import neither
`graph/embedding`, `processor/graph-embedding` nor `graph/readiness`, directly or through a package it imports.

#### Scenario: No embedder

- **WHEN** the stack boots without graph-embedding and the producer publishes the fixed entities
- **THEN** the three components are healthy, `GRAPH_STATUS` holds no `graph-embedding` key, a symbol lens and a prefix
  lens return the expected nodes, relations and paths, an unknown name is a miss, and an NL lens's `Fuse` returns a
  refusal naming `graph.embedding.query.search`, transient `internal` as graph-query's "A refusal reaches the caller
  intact" requires (`TestSmoke_NoEmbedderAnswers`)

#### Scenario: An embedding import

- **WHEN** a package `fusionnats` imports reaches `graph/embedding`
- **THEN** `TestFusionNATSImportsNoEmbedding` fails naming the import path

