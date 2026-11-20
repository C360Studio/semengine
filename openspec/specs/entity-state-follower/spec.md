# entity-state-follower Specification

## Purpose
The entity-state follower (`graph/follower`) watches `ENTITY_STATES` on behalf of one index, tracks how far that index
has applied what it saw, publishes the index's readiness, and declares a lost feed. graph-index is its first adopter
(ADR-108 Decision 2; archived change `2026-10-27-setup-04a-04-graph-roots-graph-index`, design D3).

## Requirements

### Requirement: Open waits for ENTITY_STATES within a bound

`follower.Open` SHALL wait for the `ENTITY_STATES` bucket at most `StartupAttempts` times `StartupInterval` and then
return a transient error naming the bucket, holding nothing. It SHALL refuse a nil context, a nil client, an empty
status key, a nil `Deliver` and a nil `Failed`. The gauges' Prometheus subsystem SHALL be derived from the status key.

#### Scenario: The bucket never appears

- **WHEN** `Open` is called and `ENTITY_STATES` does not exist
- **THEN** `Open` returns a transient error naming `ENTITY_STATES` after the bounded wait, and no watcher exists
- **AND** the test that verifies this is `TestFollowerOpenRefusesAbsentBucket`

### Requirement: Run stops when its context ends and leaves nothing

`Follower.Run` SHALL run on the caller's goroutine until its context ends, then return with no watcher, ticker or
goroutine of its own left. It SHALL refuse a nil context. It SHALL publish the envelope once at once and then every
`readiness.DefaultHeartbeat`, on a goroutine `Run` joins, so a blocked `Deliver` never delays a heartbeat.

#### Scenario: Run's context ends

- **WHEN** `Run`'s context is cancelled
- **THEN** `Run` returns nil and no goroutine it started is still running
- **AND** the test that verifies this is `TestFollowerRunReturnsAndLeavesNothing`

### Requirement: Run returns only after the watch's delivery closes

After stopping the watch, `Run` SHALL wait until the watch's delivery closes, for at most 5 s, then return nil, or
the watch's `Stop` error when it says more than that the watch was already gone. When the 5 s run out, it SHALL return
an error saying the delivery did not close after `Stop`. Changed: the wait had no bound (#454).

#### Scenario: A backlogged watch at cancellation

- **WHEN** `Run`'s context is cancelled while nats.go's watch delivery is blocked on a full updates buffer
- **THEN** `Run` returns nil only after that delivery has ended
- **AND** the test that verifies this is `TestFollowerRunReturnsAndLeavesNothing`

#### Scenario: The watch's delivery never closes

- **WHEN** the watch is stopped and its update channel stays open, as at nats.go v1.55.0 when the watch's context
  ends, or its connection closes, after nats.go subscribes and before it installs the handler that closes the channel
  (no goroutine is then left to join; on every other path the wait ends when nats.go's delivery ends)
- **THEN** the wait returns, after 5 s of its clock, an error saying the delivery did not close after `Stop`
- **AND** the test that verifies this is `TestStopWatchGivesUpOnADeliveryThatNeverCloses`

### Requirement: Every delivered revision reaches the watermark first

The follower SHALL pass each delivered revision, update or delete, to its watermark before it calls `Deliver` with it.
`Indexed` SHALL return the watermark's floor, which follows the watermark's rules for accepted revisions.

#### Scenario: A delivered revision not yet completed

- **WHEN** revision R of key K is delivered and accepted, and no `Complete(K, R2)` with R2 at or above R has been called
- **THEN** `Indexed` is below R

### Requirement: The initial build completes at the snapshot's revision

At the end-of-snapshot marker the follower SHALL fix the bootstrap target at the highest revision observed. The
bootstrap latch SHALL set once the floor reaches that target, or the envelope reads ready, and SHALL never clear.

#### Scenario: Writes continue during the replay

- **WHEN** writes keep arriving after the marker and the floor reaches the fixed target
- **THEN** the latch is set although the live target is higher
- **AND** the tests that verify this are `TestFollowerSnapshotEndSetsBootstrapTarget` and `TestLatchBootstrap_ContinuousWrite`

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

### Requirement: A restored stream is a declared limit

The follower's documentation SHALL declare that a backing stream restored with the creation time `Open` read may go
undetected as a lost feed: a declared limit, not a promise, as in `graph-ingest-recovery`.

#### Scenario: Delete and restore inside one stall window

- **WHEN** `ENTITY_STATES` is deleted and restored with its original creation time before the watch resumes
- **THEN** a lost feed is not promised to be reported

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

### Requirement: Degraded when nothing completes

The envelope SHALL read `degraded` when the floor has been behind the target for 30 s with no `Complete` call; time
spent caught up (an empty graph after the snapshot included) SHALL NOT count, and a held low revision with later
completions within 30 s SHALL NOT read `degraded`.

#### Scenario: A held low revision while later revisions complete

- **WHEN** one revision is not completed for more than 30 s while later revisions keep completing
- **THEN** the envelope reads `ready` false and is not `degraded` (`TestHeldLowRevisionWithProgress`)

#### Scenario: Nothing completes

- **WHEN** the floor is behind and no revision completes for more than 30 s
- **THEN** the envelope reads `degraded` (`TestNoCompletionsReadsDegraded`)

#### Scenario: Falling behind after a long time caught up

- **WHEN** the floor has been at the target for more than 30 s with no completion and a new write arrives
- **THEN** the first reading behind is `building`, not `degraded` (`TestNoCompletionsReadsDegraded`)

### Requirement: A plug-in may narrow which failures withhold queries

`follower.Config` SHALL have an optional `Withhold func() int64`. When it is set, after the first build `QueryReady`
SHALL refuse transient `index_not_ready` for failures only while `Withhold` returns above 0; before the first build
the gate SHALL keep reading the envelope, which reads `Failed`. When it is nil, `QueryReady` SHALL use the count
`Failed` returns, as before. The envelope and `Stuck` SHALL keep reading `Failed`.

#### Scenario: A failure the plug-in does not withhold on

- **WHEN** `Failed` returns count 1 and `Withhold` returns 0 after the first build
- **THEN** `QueryReady` returns nil, while the envelope reads `failed_count` 1 and `Stuck` names it
  (`TestWithholdNarrowsQueryReady`)

#### Scenario: No Withhold

- **WHEN** `Withhold` is nil and `Failed` returns count 1
- **THEN** `QueryReady` refuses transient `index_not_ready`, as for graph-index (`TestGraphIndexWithholdUnchanged`)

### Requirement: An entry carries the delivered value

`Entry.Value` SHALL be the value the watch delivered for `Entry.Revision`, and nil for a delete or purge marker. It
SHALL be valid only for the duration of `Deliver`; a plug-in that keeps it copies it. A plug-in that re-reads current
truth at apply time SHALL NOT apply `Value` (ADR-108 D4); a plug-in that applies `Value` SHALL ignore a delivery at or
below the revision it last applied for that key.

#### Scenario: Two writes to one key

- **WHEN** a key is written twice and both revisions are delivered
- **THEN** each entry's `Value` is the bytes written at its revision (`TestEntryCarriesDeliveredValue`)

### Requirement: The follower checks delivery order

Delivery order is what the watch delivers: nats.go does not promise ascending revisions across keys. The follower
SHALL check each revision against the highest observed and, on a revision at or below it, declare the feed lost and
still deliver the entry with its value.

#### Scenario: A revision out of order

- **WHEN** the watch delivers revision 5 and then revision 3
- **THEN** `Stuck` names the lost feed, and the second entry is delivered with revision 3's value
  (`TestFollowerDeclaresLostFeedOnOutOfOrderDelivery`, extended)

### Requirement: Stuck names unresolved failures without assuming their kind

When `Failed` returns a count above zero, `Stuck` SHALL name the count as unresolved failures and, when `Failed`
returns reasons, each reason with its count.

#### Scenario: A plug-in with reasons

- **WHEN** `Failed` returns 2 with reasons `{"refused_entity": 2}`
- **THEN** `Stuck` holds one sentence naming 2 unresolved failures and `refused_entity` 2 (`TestStuckNamesReasons`)

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

- **WHEN** generated sequences over a few keys are applied: reports, completions at the reported revision, below it
  and above it, and deletes
- **THEN** a key is recorded exactly when some report of it is at or above every later completion and delete of it,
  checked against a reference model with the seed and the completed runs recorded (`TestPropDamagedSetFollowsLatestEvent`)

#### Scenario: A completion below the report

- **WHEN** the plug-in reports B at revision 7, the revision its read found, and then completes revision 6, the one
  delivered
- **THEN** B stays recorded (`TestDamagedEntityClearsOnHigherCompletion`)

### Requirement: A failed read can hide a damaged entity until its repair

A plug-in that completes a higher revision of a damaged key without reading a valid value for it, after a failed read,
clears the entry; it SHALL be recorded again when the plug-in's repair reads the value. This is a declared limit.

#### Scenario: A failed read, then the repair

- **WHEN** B is recorded at revision 5, the plug-in completes revision 6 after a failed read, then its repair reads
  B's damaged value at revision 6 and reports it
- **THEN** B is not recorded after the completion and is recorded after the report
  (`TestDamagedEntityClearsOnHigherCompletion`)

### Requirement: A report says whether it is new

A report SHALL return whether it newly recorded the key. The plug-in SHALL count one error in `ErrorCount` for each
report that newly records a key, and none for a report of a key already recorded.

#### Scenario: The same damage read twice

- **WHEN** the plug-in reports B, then reports B again at the same revision after a repair pass reads it
- **THEN** the first report returns true and the second false, and `ErrorCount` rose by 1
  (`TestDamagedEntityCountsOneError`)

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
