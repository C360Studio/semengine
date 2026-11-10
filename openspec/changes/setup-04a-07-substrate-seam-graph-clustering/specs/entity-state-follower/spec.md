# entity-state-follower

## ADDED Requirements

### Requirement: An entry carries the delivered value

`Entry.Value` SHALL be the value the watch delivered for `Entry.Revision`, and nil for a delete or purge marker. A
plug-in that applies `Value` instead of re-reading current truth SHALL ignore a delivery at or below the revision it
last applied for that key.

#### Scenario: Two writes to one key

- **WHEN** a key is written twice and both revisions are delivered
- **THEN** each entry's `Value` is the bytes written at its revision (`TestEntryCarriesDeliveredValue`)

#### Scenario: The watch's order across keys

- **WHEN** writes to several keys are interleaved at the pinned nats.go and server image
- **THEN** the watch delivers them in ascending revision order across keys, and a delivery that is not is declared a
  lost feed (`TestWatchDeliversAscendingAcrossKeys`, `TestFollowerDeclaresLostFeedOnOutOfOrderDelivery`)
