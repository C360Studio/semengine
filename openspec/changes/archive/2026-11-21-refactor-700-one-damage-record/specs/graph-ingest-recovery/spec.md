# graph-ingest-recovery Delta

## ADDED Requirements

### Requirement: graph-ingest records a poisoned entity until its repair

graph-ingest SHALL record a poisoned entity (an `ENTITY_STATES` value its stored-value rule refuses) when a read, a
mutation's read or the boot sweep finds one: the entity ID, the revision read and the refusal, one entry per ID, kept
at the highest revision found. No read or write SHALL consult the record for a decision.

#### Scenario: The record decides nothing

- **WHEN** a stale entry is held for an entity whose value is valid
- **THEN** reads serve the entity and writes to it commit (`TestPoisonInventoryIsObservabilityOnly`)

### Requirement: A proof of repair at or above the recorded revision clears it

graph-ingest SHALL drop a recorded entry on proof of repair at or above its revision: a delete that removed that
revision, a write that committed above it, a read that validated the key's bytes, or a re-check that finds the key
absent or valid. A proof below the recorded revision SHALL leave it.

#### Scenario: A delete repairs it

- **WHEN** an entity's stored value is refused on read, and the entity is then deleted at its stored revision
- **THEN** it is recorded and the gauge reads 1 after the read, and after the delete it is not recorded, the gauge
  reads 0 and Health recovers (`TestPoisonRepairRecoveryWithoutRestart`)

#### Scenario: A repair outside the mutation API

- **WHEN** a recorded entity's value is replaced with a valid one outside graph-ingest and then read
- **THEN** the read clears the entry (`TestPoisonOutOfBandRepairClearsOnNextRead`)

#### Scenario: A newer poison outlives an older proof

- **WHEN** a delete or an absence proves repair up to one revision and the key is poisoned again at a higher one
- **THEN** the higher entry stays (`TestStaleAbsenceKeepsNewerPoison`, `TestDeleteCleanupKeepsNewerPoison`,
  `TestPoisonRevisionGuardConcurrentRepair`)

#### Scenario: A repair that finished before the record

- **WHEN** an entity is recorded after its repair by a commit or a delete already completed
- **THEN** the re-check drops the entry (`TestVerifyAfterRecordDropsStaleRecordAfterCompletedRepair`)

### Requirement: graph-ingest reports its poisoned entities

While any entry is recorded, `Health()` SHALL name the count and the first 10 IDs in sorted order with their reasons,
the gauge `semengine_graph_ingest_poisoned_entities` SHALL equal the count, and the debug status SHALL list every
entry in sorted order. A new entry SHALL invalidate that entity's query-cache entry.

#### Scenario: Eleven poisoned entities

- **WHEN** 11 entities are recorded
- **THEN** Health names 11, the first 10 IDs in sorted order with reasons and 1 more, and the debug status lists all
  11 in sorted order (`TestHealthNamesTenPoisonedEntities`)

#### Scenario: Detection invalidates the cached entry

- **WHEN** an entity is cached and its stored value is then poisoned outside graph-ingest and detected
- **THEN** the new entry deletes the entity's query-cache entry (`TestDetectionInvalidatesCachedEntry`)

#### Scenario: Concurrent records and clears

- **WHEN** reports and clears of distinct keys run concurrently and all have returned
- **THEN** the gauge equals the number of keys left recorded (`TestGaugeFollowsConcurrentRecordAndClear`)
