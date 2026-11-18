# entity-state-follower Delta

## ADDED Requirements

### Requirement: A damaged entity is recorded per key and cleared by its repair

The follower SHALL record a damaged entity when the plug-in reports one: the key, the revision the plug-in read and
the refusal `graph.UnmarshalEntityState` returned, one entry per key at the highest reported revision. It SHALL drop
the entry when the plug-in completes a higher revision of that key, or when a delete or purge of that key is delivered
above it. No report SHALL stop the follower or refuse every query.

#### Scenario: Report, then repair

- **WHEN** the plug-in reports key B damaged at revision 5, then completes B at revision 5, then at revision 7
- **THEN** B is recorded after the first two calls and not after the third
  (`TestDamagedEntityClearsOnHigherCompletion`)

#### Scenario: A delete clears it

- **WHEN** B is recorded at revision 5 and a delete of B is delivered at revision 6
- **THEN** B is no longer recorded (`TestDamagedEntityClearsOnDelete`)

#### Scenario: Any history

- **WHEN** generated sequences of reports, completions and deletes over a few keys, with rising revisions, are applied
- **THEN** the recorded keys are exactly those whose highest-revision event is a report, checked against a reference
  model with the seed and the completed runs recorded (`TestPropDamagedSetFollowsLatestEvent`)

### Requirement: A failed read can hide a damaged entity until its repair

A plug-in that completes a higher revision of a damaged key without reading a valid value for it, after a failed read,
clears the entry; it SHALL be recorded again when the plug-in's repair reads the value. This is a declared limit.

#### Scenario: A failed read, then the repair

- **WHEN** B is recorded at revision 5, the plug-in completes revision 6 after a failed read, then its repair reads
  B's damaged value at revision 6 and reports it
- **THEN** B is not recorded after the completion and is recorded after the report
  (`TestDamagedEntityClearsOnHigherCompletion`)

### Requirement: A damaged entity never withholds a query

A recorded damaged entity SHALL NOT make `QueryReady` refuse, before or after the first build, and SHALL NOT count in
`Withhold`. The plug-in completes a damaged revision, so the first build completes with damage stored.

#### Scenario: Damage stored at start

- **WHEN** `ENTITY_STATES` holds a value that does not decode when `Run` starts, and the plug-in reports and completes
  it
- **THEN** `QueryReady` returns nil once the floor reaches the snapshot's revision
  (`TestFollowerServesPastDamagedReplay`)

### Requirement: The plug-in reads what is damaged

The follower SHALL answer, at any time, the number of recorded damaged entities, all of them or those an ID filter
keeps, and, for one ID, the refusal recorded for it or nil.

#### Scenario: Count and refusal

- **WHEN** B and C are recorded and the filter keeps IDs under B's prefix only
- **THEN** the count is 2 unfiltered and 1 filtered, B's refusal is fatal `graph_state_reset_required` naming B, and
  A's is nil (`TestDamagedCountAndRefusal`)

### Requirement: Stuck names the damaged entities

While any damaged entity is recorded, `Stuck` SHALL hold one sentence with the count, the first 10 IDs in sorted order
each with its reason, the number not named, and the repair: write a valid value for the key or delete it.

#### Scenario: Eleven damaged entities

- **WHEN** 11 entities are recorded
- **THEN** `Stuck` names 11, the first 10 IDs with reasons, 1 more, and the repair (`TestStuckNamesDamagedEntities`)

## MODIFIED Requirements

### Requirement: The envelope

The follower's envelope SHALL be `ComputeIndexStatus` over the floor, the commit time `IndexedAt` returns with it and
the bucket's last sequence, with these overrides: a target of 0 after the snapshot marker reads ready; a `Failed()`
count above zero, or a recorded damaged entity, reads not ready, `degraded`, and is carried in `failed_count`, which
is `Failed()`'s count plus the damaged count; a failed target read reads `degraded`.

#### Scenario: Unresolved writes

- **WHEN** `Failed()` returns 2 and the floor has reached the target
- **THEN** the envelope reads `ready` false, state `degraded`, `failed_count` 2

#### Scenario: A damaged entity

- **WHEN** `Failed()` returns 0, one damaged entity is recorded and the floor has reached the target
- **THEN** the envelope reads `ready` false, state `degraded`, `failed_count` 1 and `failed_reasons`
  `{"damaged_entity": 1}` (`TestEnvelopeCountsDamagedEntities`)

### Requirement: Failure reasons

When the `Failed()` count is above zero, the envelope SHALL carry in `failed_reasons` the per-reason counts the same
`Failed()` call returned with the count, and, while any damaged entity is recorded, `damaged_entity` with their
number; with neither it SHALL carry no reasons. The envelope SHALL carry no entity ID.
`Failed()` SHALL return a map the follower owns (the plug-in keeps no reference to it). The envelope SHALL NOT carry
`first_failure_at`.

#### Scenario: Failures with reasons

- **WHEN** `Failed()` returns count 3 and reasons `{"timeout": 2, "delete_failed": 1}` while outcomes are reported
  concurrently
- **THEN** the published envelope has `failed_count` 3 and those `failed_reasons`, under `-race`
  (`TestEnvelopeCarriesFailedReasons`)

#### Scenario: A plug-in with no reasons

- **WHEN** `Failed()` returns a count and `nil` reasons, as graph-index does, and no damaged entity is recorded
- **THEN** the envelope has no `failed_reasons` (`TestGraphIndexEnvelopeUnchanged`)

## REMOVED Requirements

### Requirement: The startup replay validates each stored entity

**Reason**: The follower latched a process-lifetime reset on the first damaged value in the replay. The owner ruled
that the components keep serving with a damaged entity (#657 ruling 3; #658, 2026-11-18). Every plug-in decodes each
value it applies and reports a refused one, so the follower no longer decodes.

**Migration**: "A damaged entity is recorded per key and cleared by its repair" and "A damaged entity never withholds
a query".

### Requirement: Reset is sticky

**Reason**: `Reset` is replaced by the per-key damage report; nothing latches for the life of the process.

**Migration**: "A damaged entity is recorded per key and cleared by its repair".
