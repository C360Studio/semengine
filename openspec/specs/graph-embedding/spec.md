# graph-embedding Specification

## Purpose
The graph-embedding component follows `ENTITY_STATES` and gives every entity a BM25 embedding at tier 0, with no
external provider. This capability states when its readiness reports an entity done, how it counts and clears
failures, which configuration it accepts, and what its health reports.

## Requirements

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
latest terminal outcome. A failure with no source revision (a failed entity read, or a failed delete of an
absent entity) SHALL be cleared only by a later successful hop-1 convergence of that entity: a pending record
written, a guarded result that finds it already generated, or the record removed.

#### Scenario: A late failure for an older revision

- **WHEN** revision 7 is generated and then a failure for revision 5 is reported
- **THEN** the entity is not counted failed (`TestFailedMapHistoryMatchesModel`)

#### Scenario: A failure with no source revision

- **WHEN** an entity read fails during repair, and the Worker then reports a generated outcome for that entity
- **THEN** the entity is still counted failed, with reason `entity_read_failed`, until a repair converges the entity
  (`TestStrandedWithoutRevisionClearsOnlyOnConvergence`)

#### Scenario: A repair whose read succeeds but whose write fails

- **WHEN** a stranded entity's repair reads it successfully and the pending write then fails
- **THEN** the entity is still counted failed (`TestRepairConvergesStranded`)

### Requirement: Tier-0 config

graph-embedding SHALL refuse at construction an unknown config key, naming it, and any `embedder_type` other than
`bm25`, and SHALL import no provider module.

#### Scenario: A removed key

- **WHEN** the config holds `batch_size`
- **THEN** construction fails naming `batch_size` (`TestConfigRefusesUnknownKey`)

#### Scenario: The HTTP embedder at tier 0

- **WHEN** the config holds `embedder_type: http`
- **THEN** construction fails naming the value and tier 0 (`TestConfigRefusesHTTPEmbedder`)
