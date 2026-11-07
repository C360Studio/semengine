# Delta: entity-state-follower

## MODIFIED Requirements

### Requirement: Every delivered revision is observed before it is delivered

The follower SHALL pass each delivered revision, update or delete, to its watermark before it calls `Deliver` with it,
and its floor SHALL advance only through `Complete`. `Indexed` SHALL return the floor.

#### Scenario: A delivered revision not yet completed

- **WHEN** revision R of key K is delivered and `Complete(K, R)` has not been called
- **THEN** `Indexed` is below R

### Requirement: A lost feed is declared

When the watch cannot start, its update channel closes while `Run`'s context is live, the watermark refuses a
delivered revision, or a heartbeat finds `ENTITY_STATES`'s backing stream created after the one `Open` bound, the
follower SHALL report the lost feed through `Stuck`, `degraded` in the next envelope and `index_not_ready` from
`QueryReady`, and SHALL keep publishing. The state SHALL hold until the process restarts.

#### Scenario: The update channel closes

- **WHEN** `ENTITY_STATES` is deleted while `Run` runs
- **THEN** `Stuck` names the lost feed and the next envelope's state is `degraded`
- **AND** the test that verifies this is `TestFollowerDeclaresLostFeed`

#### Scenario: The bucket is recreated

- **WHEN** `ENTITY_STATES` is deleted and created again while `Run` runs
- **THEN** within one heartbeat `Stuck` names the lost feed, although the update channel stays open

#### Scenario: A revision delivered out of order

- **WHEN** the watch delivers revision 5 and then revision 3
- **THEN** `Stuck` names the lost feed and `QueryReady` returns `index_not_ready`
- **AND** the test that verifies this is `TestFollowerDeclaresLostFeedOnOutOfOrderDelivery`

### Requirement: The envelope

The follower's envelope SHALL be `ComputeIndexStatus` over the floor, the commit time `IndexedAt` returns with it and
the bucket's last sequence, with these overrides: a target of 0 after the snapshot marker reads ready; a `Failed()`
count above zero reads not ready, `degraded`, and is carried in `failed_count`; a failed target read reads `degraded`;
a latched reset reads `reset_required` with its reason.

#### Scenario: Unresolved writes

- **WHEN** `Failed()` returns 2 and the floor has reached the target
- **THEN** the envelope reads `ready` false, state `degraded`, `failed_count` 2
