# graph-embedding Delta

## ADDED Requirements

### Requirement: Provider failures are retried

On each repair pass, after hop 1's own failures, graph-embedding SHALL re-drive Worker failures whose reason
`embedding.RetryableReason` accepts, by converging the entity on its current `ENTITY_STATES` value as a delivery does.
A failure with any other reason SHALL NOT be re-driven. A re-driven entity SHALL stay counted failed until the
Worker's outcome, and SHALL NOT be re-driven again until that outcome arrives; a failure seeded at Start waits the
same way.

#### Scenario: A failure the lane leaves alone

- **WHEN** an entity fails `dimension_mismatch` and repair passes run
- **THEN** the embedder is not called for it and it stays failed (`TestRepairRetriesProviderFailures`)

#### Scenario: A retry still waiting

- **WHEN** a pass runs while a re-driven entity's embedding is still in progress
- **THEN** no second call is made for that entity (`TestRepairWaitsForRedrivenOutcome`)

### Requirement: Retries are paced by the provider's answer

A pass SHALL re-drive every retryable failure when, since the previous pass, the Worker stored a vector for an entity
that was failed. Otherwise it SHALL re-drive one: the next after the last re-driven, in entity ID order.

#### Scenario: The provider is still down

- **WHEN** four entities hold retryable failures and the embedder still fails
- **THEN** each pass makes one embedder call, for a different entity each time, and all four stay failed
  (`TestRepairRetriesProviderFailures`)

#### Scenario: The provider answers again

- **WHEN** the embedder answers and a pass's one re-drive gets a vector
- **THEN** the next pass re-drives the other three and each gets a vector (`TestRepairRetriesProviderFailures`)
