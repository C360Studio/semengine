# graph-embedding

## ADDED Requirements

### Requirement: Readiness waits for the second hop

graph-embedding SHALL report an `ENTITY_STATES` revision applied on its `graph-embedding` readiness key only when the
entity's embedding for that revision has reached a terminal outcome (generated, failed, or removed), or hop 1 has
ended it.

#### Scenario: A pending record is not caught up

- **WHEN** an entity revision has a pending record and the Worker has not reported it
- **THEN** the envelope's indexed revision is below that revision (`TestReadinessWaitsForHopTwo`)

### Requirement: A lost Worker watch is a stuck state

When `embedding.Worker.Run` returns while the component is running, graph-embedding SHALL report `Healthy` false with
the stuck state `embedding worker watch lost` until the process restarts.

#### Scenario: The watch is lost

- **WHEN** the Worker's watch ends while the component runs
- **THEN** `Health` is not healthy and names the state (`TestHealthNamesLostWorkerWatch`)

### Requirement: The failed map never moves backwards

graph-embedding SHALL count an entity as failed only for a failure whose source revision is at or above the entity's
latest terminal outcome.

#### Scenario: A late failure for an older revision

- **WHEN** revision 7 is generated and then a failure for revision 5 is reported
- **THEN** the entity is not counted failed (`TestFailedMapHistoryMatchesModel`)

### Requirement: Tier-0 config

graph-embedding SHALL refuse at construction an unknown config key, naming it, and any `embedder_type` other than
`bm25`, and SHALL import no provider module.

#### Scenario: A removed key

- **WHEN** the config holds `batch_size`
- **THEN** construction fails naming `batch_size` (`TestConfigRefusesUnknownKey`)
