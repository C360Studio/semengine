# graph-index

## ADDED Requirements

### Requirement: Configuration is strict

graph-index's factory SHALL refuse an unknown configuration key, naming it. The keys SHALL be `ports`, `workers`
(1 to 16, default 1), `coalesce_ms`, `startup_attempts` and `startup_interval_ms`; each SHALL change behaviour.

#### Scenario: A configuration carrying batch_size

- **WHEN** graph-index is constructed from a configuration with the key `batch_size`
- **THEN** construction fails with an error naming `batch_size`
- **AND** the test that verifies this is `TestUnknownConfigKeyIsRefused`

### Requirement: Startup-replay queries get a retryable refusal

Each of graph-index's eight verbs SHALL answer class transient, code `index_not_ready`, while its initial build is not
applied or while index writes it requires are unresolved, and SHALL answer class fatal, code
`graph_state_reset_required`, once reset is latched. Once the build is applied and no write is unresolved, the same
request SHALL succeed, whatever the lag.

#### Scenario: A request during the replay

- **WHEN** a request on any of the eight verbs arrives before the initial build is applied
- **THEN** it fails with class transient and code `index_not_ready`, and the same request succeeds after the build
- **AND** the test that verifies this is `TestQueryDuringStartupReplayIsIndexNotReady`

### Requirement: Health reads named stuck states only

graph-index's `Healthy` SHALL be false exactly while it is not running or a stuck state holds: reset latched, a
required index write unresolved, or the feed lost. `ErrorCount` SHALL never lower it.

#### Scenario: A failed write and its repair

- **WHEN** an entity's required writes fail and are later repaired, and errors were counted
- **THEN** `Healthy` is false while the failure holds and true after the repair
- **AND** the test that verifies this is `TestHealthIgnoresErrorCountAndTracksStuckStates`

### Requirement: DataFlow counts deliveries

graph-index's `DataFlow` SHALL count one input per delivered `ENTITY_STATES` entry, update or delete, with the value's
bytes, and SHALL not count index writes.

#### Scenario: One entity with three relationships

- **WHEN** one entity with three relationships is written once
- **THEN** `DataFlow().Messages` rises by one

### Requirement: One Start per instance

A second `Start` on a started graph-index SHALL return `errs.ErrAlreadyStarted` and change nothing; ending the context
given to `Start` after it returned SHALL stop nothing.

#### Scenario: Start twice

- **WHEN** `Start` is called on a running graph-index
- **THEN** it returns `errs.ErrAlreadyStarted` and the component keeps running
- **AND** the test that verifies this is `TestSecondStartIsRefused`

### Requirement: Indexes implement replacement

For outgoing, incoming, predicate (exact, value, list, stats, compound) and name reads, a change of an entity's
statements from A to B and then to none SHALL leave each read answering B and then nothing for that entity.

#### Scenario: A then B then none

- **WHEN** an entity's statements go from A to B to none
- **THEN** each read listed answers A, then B, then nothing for that entity

### Requirement: Stop processes queued work

Work already queued for indexing when `Stop` runs SHALL be processed before `Stop` returns nil.

#### Scenario: Queued work at Stop

- **WHEN** `Stop` is called with entries queued in the index lanes
- **THEN** each queued entry is processed before `Stop` returns
- **AND** the test that verifies this is `TestStopProcessesQueuedWork`
