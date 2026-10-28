# entity-state-follower

## MODIFIED Requirements

### Requirement: Run returns only after the watch's delivery closes

After stopping the watch, `Run` SHALL wait until the watch's delivery closes, for at most 5 s, then return nil, or
the watch's `Stop` error when it says more than that the watch was already gone. When the 5 s run out, it SHALL return
an error saying the delivery did not close after `Stop`. Changed: the wait had no bound (#454).

#### Scenario: A backlogged watch at cancellation

- **WHEN** `Run`'s context is cancelled while nats.go's watch delivery is blocked on a full updates buffer
- **THEN** `Run` returns nil only after that delivery has ended
- **AND** the test that verifies this is `TestFollowerRunReturnsAndLeavesNothing`

#### Scenario: The watch's delivery never closes

- **WHEN** the watch is stopped and its update channel stays open, as at nats.go v1.54.0 when the watch's context
  ends, or its connection closes, after nats.go subscribes and before it installs the handler that closes the channel
  (no goroutine is then left to join; on every other path the wait ends when nats.go's delivery ends)
- **THEN** the wait returns, after 5 s of its clock, an error saying the delivery did not close after `Stop`
- **AND** the test that verifies this is `TestStopWatchGivesUpOnADeliveryThatNeverCloses`
