# entity-state-follower Delta

## MODIFIED Requirements

### Requirement: The envelope

The follower's envelope SHALL be `ComputeIndexStatus` over the floor, the commit time `IndexedAt` returns with it and
the bucket's last sequence, with these overrides: a target of 0 after the snapshot marker reads ready; a `Failed()`
count above zero, or a recorded damaged entity, reads not ready, `degraded`, and is carried in `failed_count`, which
is `Failed()`'s count plus the damaged count; a failed target read reads `degraded`. While the latest target read has
failed, `Stuck` SHALL name the failed status read; the next successful read SHALL clear it, and a failed read SHALL
NOT declare the feed lost.

#### Scenario: Unresolved writes

- **WHEN** `Failed()` returns 2 and the floor has reached the target
- **THEN** the envelope reads `ready` false, state `degraded`, `failed_count` 2

#### Scenario: A damaged entity

- **WHEN** `Failed()` returns 0, one damaged entity is recorded and the floor has reached the target
- **THEN** the envelope reads `ready` false, state `degraded`, `failed_count` 1 and `failed_reasons`
  `{"damaged_entity": 1}` (`TestEnvelopeCountsDamagedEntities`)

#### Scenario: A failed target read

- **WHEN** the follower's read of `ENTITY_STATES`' last sequence fails, as it does while NATS is down
- **THEN** the envelope reads `degraded`, `Stuck` names the failed status read and the feed is not lost, and after
  the next successful read `Stuck` no longer names it
