# composed-stack

## ADDED Requirements

### Requirement: The graph stack boots from one configuration

graph-ingest, graph-index, graph-embedding and graph-query SHALL start together from one JSON configuration through
`config.Manager`, `ServiceManager` and `ComponentManager`, and SHALL answer the fixed questions about entities a
producer published on `entity.>`.

#### Scenario: Answers from one configuration

- **WHEN** the stack boots from the configuration and the producer publishes the fixed entities
- **THEN** the three readiness keys in `GRAPH_STATUS` read ready, `entity`, `relationships`, `pathSearch`, `summary`,
  `localSearch` and `entityByAlias` return the expected answers, and each entity has one generated embedding record
- **AND** after an edit and a delete the answers and records follow (`TestSmoke_OneConfigAnswers`)

### Requirement: A whole-stack restart keeps the answers

After the stack stops cleanly and starts again on the same NATS server, it SHALL reach ready with the same answers
and the same set of embedding records.

#### Scenario: Stop and start the stack

- **WHEN** the stack process stops on SIGTERM and a new process boots from the same configuration
- **THEN** every readiness key reads ready and the answers and record set equal those before the stop
  (`TestSmoke_StackRestartKeepsAnswers`)

### Requirement: A process killed mid-ingest loses no acknowledged write

When the stack process is killed while graph-ingest is applying a batch, every publish the `ENTITY` stream
acknowledged SHALL be present exactly once after a new process boots, and the index and embedding records SHALL catch
up.

#### Scenario: Kill mid-ingest

- **WHEN** the process is killed with at least one input applied and at least one not acknowledged, and a new process
  boots
- **THEN** each acknowledged entity answers with each statement once, and readiness and records catch up
  (`TestSmoke_KillMidIngestKeepsAcknowledgedWrites`)

### Requirement: A broker restart between runs keeps the answers

With file storage, a stack started after the NATS server restarted SHALL reach ready with the same answers.

#### Scenario: Restart the broker while the stack is stopped

- **WHEN** the stack stops, the NATS server restarts on the same URL, and the stack boots again
- **THEN** every readiness key reads ready with the same answers (`TestSmoke_BrokerRestartBetweenRuns`)

### Requirement: A running stack recovers from a broker restart

With file storage, a stack that keeps running while the NATS server restarts on the same URL SHALL reach ready again
without a process restart, and SHALL apply what is published after the restart. No component SHALL declare a lost
feed or a lost watch because of the restart.

#### Scenario: Restart the broker under the running stack

- **WHEN** the NATS server restarts while the stack runs, and a second batch, an edit and a delete are published after
- **THEN** every readiness key reads ready again with answers that include them, and no latched state is reported
  (`TestSmoke_BrokerRestartUnderRunningStack`)
