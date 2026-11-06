# lifecycle-query

## ADDED Requirements

### Requirement: List returns every match or an error and leaves no watch behind

`pkg/lifecycle`'s `Manager.List` SHALL return every participant matching its options from a complete key listing, or
no participants and an error. When the listing ends early (its context ended, or the watch closed first), the error
SHALL match the context's error if it ended. On return, no nats.go watch delivery or key lister started by the call
SHALL still be running.

#### Scenario: A read error with many keys left

- **WHEN** `List` meets an entity it cannot read while more than 256 keys remain unread
- **THEN** it returns an error and no participants, and no watch delivery or key lister it started is still running

#### Scenario: The listing is cut short

- **WHEN** the context ends, or the watch closes, before the listing completes
- **THEN** `List` returns no participants and an error, matching `context.Canceled` when the context was cancelled
