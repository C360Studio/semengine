# entity-state-follower

## ADDED Requirements

### Requirement: Open waits for ENTITY_STATES within a bound

`follower.Open` SHALL wait for the `ENTITY_STATES` bucket at most `StartupAttempts` times `StartupInterval` and then
return a transient error naming the bucket, holding nothing. It SHALL refuse a nil context, a nil client, an empty
status key, a nil `Deliver` and a nil `Failed`. The gauges' Prometheus subsystem SHALL be derived from the status key.

#### Scenario: The bucket never appears

- **WHEN** `Open` is called and `ENTITY_STATES` does not exist
- **THEN** `Open` returns a transient error naming `ENTITY_STATES` after the bounded wait, and no watcher exists
- **AND** the test that verifies this is `TestFollowerOpenRefusesAbsentBucket`

### Requirement: Run stops when its context ends and leaves nothing

`Follower.Run` SHALL run on the caller's goroutine until its context ends, then return nil with no watcher, ticker or
goroutine of its own left. It SHALL refuse a nil context. It SHALL publish the envelope once at once and then every
`readiness.DefaultHeartbeat`, on a goroutine `Run` joins, so a blocked `Deliver` never delays a heartbeat.

#### Scenario: Run's context ends

- **WHEN** `Run`'s context is cancelled
- **THEN** `Run` returns nil and no goroutine it started is still running
- **AND** the test that verifies this is `TestFollowerRunReturnsAndLeavesNothing`

### Requirement: Every delivered revision is observed before it is delivered

The follower SHALL record each delivered revision, update or delete, in its watermark before it calls `Deliver` with
it, and its floor SHALL advance only through `Complete`. `Indexed` SHALL return the floor.

#### Scenario: A delivered revision not yet completed

- **WHEN** revision R of key K is delivered and `Complete(K, R)` has not been called
- **THEN** `Indexed` is below R

### Requirement: The startup replay validates each stored entity

Until the watch's end-of-snapshot marker, the follower SHALL decode each delivered non-deleted value with
`graph.UnmarshalEntityState` and, on a refusal, latch reset with the refusal's reason; it SHALL still deliver the entry.

#### Scenario: An unreadable stored entity

- **WHEN** `ENTITY_STATES` holds a value that does not decode when `Run` starts
- **THEN** reset is latched with that reason and `QueryReady` returns the fatal code `graph_state_reset_required`
- **AND** the test that verifies this is `TestFollowerLatchesResetOnUnreadableSnapshotValue`

### Requirement: The initial build completes at the snapshot's revision

At the end-of-snapshot marker the follower SHALL fix the bootstrap target at the highest revision observed. The
bootstrap latch SHALL set once the floor reaches that target, or the envelope reads ready, and SHALL never clear.

#### Scenario: Writes continue during the replay

- **WHEN** writes keep arriving after the marker and the floor reaches the fixed target
- **THEN** the latch is set although the live target is higher
- **AND** the tests that verify this are `TestFollowerSnapshotEndSetsBootstrapTarget` and `TestLatchBootstrap_ContinuousWrite`

### Requirement: A lost feed is declared

When the watch cannot start, its update channel closes while `Run`'s context is live, or a heartbeat finds
`ENTITY_STATES`'s backing stream created after the one `Open` bound, the follower SHALL report the lost feed through
`Stuck`, `degraded` in the next envelope and `index_not_ready` from `QueryReady`, and SHALL keep publishing. The state
SHALL hold until the process restarts.

#### Scenario: The update channel closes

- **WHEN** `ENTITY_STATES` is deleted while `Run` runs
- **THEN** `Stuck` names the lost feed and the next envelope's state is `degraded`
- **AND** the test that verifies this is `TestFollowerDeclaresLostFeed`

#### Scenario: The bucket is recreated

- **WHEN** `ENTITY_STATES` is deleted and created again while `Run` runs
- **THEN** within one heartbeat `Stuck` names the lost feed, although the update channel stays open

### Requirement: Reset is sticky

`Reset` SHALL latch the first reason given, by the follower or the plug-in, for the life of the process; later calls
SHALL change nothing.

#### Scenario: Two resets

- **WHEN** `Reset` is called with reason A and then with reason B
- **THEN** the envelope's reason and `Stuck` name A

### Requirement: The envelope

The follower's envelope SHALL be `ComputeIndexStatus` over the floor, its commit time and the bucket's last sequence,
with these overrides: a target of 0 after the snapshot marker reads ready; a `Failed()` count above zero reads not
ready, `degraded`, and is carried in `failed_count`; a failed target read reads `degraded`; a latched reset reads
`reset_required` with its reason.

#### Scenario: Unresolved writes

- **WHEN** `Failed()` returns 2 and the floor has reached the target
- **THEN** the envelope reads `ready` false, state `degraded`, `failed_count` 2
