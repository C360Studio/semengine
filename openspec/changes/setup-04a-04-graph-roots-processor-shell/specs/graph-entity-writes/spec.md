# graph-entity-writes

## ADDED Requirements

### Requirement: The entity cache stays usable through Stop

A graph-ingest query, `CreateEntity` call or write that reads or updates the entity query cache while `Stop` runs, or
after it, SHALL neither panic nor race with `Stop` clearing the cache: it uses the cache it found, which stays safe to
use after it is closed.

#### Scenario: Cache use during Stop

- **WHEN** cache reads, invalidations and repopulations run on goroutines while `Stop` runs, under the race detector
- **THEN** none panics and the race detector reports nothing
- **AND** the test that verifies this is `TestEntityCacheUseDuringStop`
