# storage-report

## ADDED Requirements

### Requirement: Reclaim deletes only after a complete listing

`Publisher.Publish` SHALL delete report rows only after a complete listing of the bucket's keys. When the listing
ends early, it SHALL delete nothing, return an error (matching the context's when it ended), and leave no nats.go
watch delivery or key lister running.

#### Scenario: Stopped mid-listing

- **WHEN** `Publish`'s context ends while more than 256 keys are still unlisted
- **THEN** it returns an error matching the context's, deletes no row, and no watch delivery it started is running

#### Scenario: The watch closes before the listing completes

- **WHEN** the listing's watch closes before its end marker with the context live
- **THEN** `Publish` deletes no row and returns an error
