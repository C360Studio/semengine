# graph-embedding Delta

## ADDED Requirements

### Requirement: Provider failures are retried

On each repair pass graph-embedding SHALL re-drive Worker failures whose reason `embedding.RetryableReason` accepts,
converging the entity on its current `ENTITY_STATES` value as a delivery does; no other failure SHALL be re-driven.
An entity queued after its failure, or seeded failed at Start, SHALL wait for the Worker's outcome before another
re-drive, and SHALL stay counted failed until it,
unless hop 1's own read or write of it fails meanwhile (a known limit, #734).

#### Scenario: A failure the lane leaves alone

- **WHEN** an entity fails `dimension_mismatch` and repair passes run
- **THEN** the embedder is not called for it and it stays failed (`TestRepairRetriesProviderFailures`)

#### Scenario: A retry still waiting

- **WHEN** passes run while a re-driven entity's embedding is still in progress
- **THEN** no second call is made for that entity (`TestRepairWaitsForRedrivenOutcome`)

#### Scenario: A failure seeded at Start

- **WHEN** a pass runs while the Worker's first snapshot is still retrying a failed record found at Start
- **THEN** the lane does not re-drive it and it stays failed with its reason (`TestStartSeedsFailedMap`)

### Requirement: Retries are paced by the provider's answer

A pass SHALL re-drive every retryable failure that is not waiting when, since the previous pass, the Worker stored a
vector for an entity that was failed. Otherwise it SHALL re-drive one: the next after the last re-driven, in entity
ID order. A skipped or deleted outcome SHALL NOT count as a stored vector.

#### Scenario: The provider is still down

- **WHEN** four entities hold retryable failures and no vector was stored since the previous pass
- **THEN** each pass picks one, a different one each time, wrapping in entity ID order (`TestRetryTargets`)

#### Scenario: The provider answers again

- **WHEN** a vector was stored for a failed entity since the previous pass
- **THEN** the pass picks every retryable failure that is not waiting, and the next pass picks one again unless
  another vector was stored (`TestRetryTargets`)

#### Scenario: An outcome without a vector

- **WHEN** the only outcome since the previous pass for a failed entity is skipped or deleted
- **THEN** the pass picks one (`TestRetryTargets`)
