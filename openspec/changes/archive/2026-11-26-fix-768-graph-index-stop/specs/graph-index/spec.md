# graph-index Delta

## MODIFIED Requirements

### Requirement: Stop writes accepted work

Index work a lane has started when `Stop` begins SHALL be written whole before `Stop` returns nil. Index work the
keyed pool accepted but no lane has started SHALL be dropped: no read, no write, no failure mark, no revision
completed, and no log line, from the moment `Stop` begins, even while it waits on query handlers. The next start's
replay re-derives what was dropped. `Stop` waits on the lanes for at most one entity's writes per lane.

#### Scenario: Accepted work at Stop

- **WHEN** `Stop` is called while one entity is being written and later entries wait in the lane's queue
- **THEN** the entity in flight has its index rows stored when `Stop` returns nil, the queued entries have none, no
  entity is counted failed, `ErrorCount` is unchanged, no revision past the one in flight is completed, and no
  warning is logged
- **AND** the tests that verify this are `TestStopDropsQueuedWork` and, with `Stop` held in a query drain,
  `TestStopDropsQueuedWorkFromItsStart`
