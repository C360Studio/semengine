# graph-ingest-recovery

## ADDED Requirements

### Requirement: The boot sweep waits at most 5 s for its watch to close

After stopping its `ENTITY_STATES` snapshot watch, graph-ingest's boot sweep SHALL wait until the watch's delivery
closes for at most 5 s. When the 5 s run out, the sweep SHALL fail, classified transient, with an error saying the
delivery did not close after `Stop`, so `Start` fails (#456).

#### Scenario: The snapshot watch's delivery never closes

- **WHEN** the sweep stops its watch and the update channel stays open, as at nats.go v1.54.0 when the watch's
  context ends, or its connection closes, after nats.go subscribes and before it installs the handler that closes the
  channel (no goroutine is then left to join; on every other path the wait ends when nats.go's delivery ends)
- **THEN** the sweep returns, after 5 s of its clock, a transient error saying the delivery did not close after
  `Stop`, and records no poison
- **AND** the test that verifies this is `TestEntityStateGuardGivesUpOnADeliveryThatNeverCloses`
