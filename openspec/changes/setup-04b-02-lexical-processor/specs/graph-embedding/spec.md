# graph-embedding Delta

## ADDED Requirements

### Requirement: A body store that hangs frees the Worker's lane

graph-embedding SHALL read an offloaded body through `graph/entitytext`, so a read that has not returned 10 seconds
after it began ends with a deadline error. The entity's record SHALL then be failed with reason `content_error`, as
any body read failure is today, without withholding the verbs, and the lane SHALL go on to its next record.

#### Scenario: One hung read on a lane

- **WHEN** a lane holds two pending records whose bodies sit on a store that blocks the first read until its context
  ends, then returns an error that wraps no context error, and serves the second
- **THEN** the first record is failed `content_error` and the second is generated, 10 seconds later
  (`TestHungStoreFreesWorkerLane`, under `testing/synctest`)
