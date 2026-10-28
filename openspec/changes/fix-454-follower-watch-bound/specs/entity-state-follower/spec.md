# entity-state-follower

## MODIFIED Requirements

### Requirement: Run returns only after the watch's delivery closes

After stopping the watch, `Run` SHALL wait until the watch's delivery closes, for at most 5 s. It SHALL then return
nil, or the watch's `Stop` error when that error says more than that the watch was already gone. When the 5 s run
out, it SHALL return an error saying the watch's delivery did not close after `Stop`. The bound exists because at
nats.go v1.54.0 a watch whose context ends, or whose connection closes, after nats.go subscribes and before it installs
the handler that closes the update channel never closes that channel. The wait runs after `Run`'s context has ended,
so it is terminal finalization with no caller context (background-work, "No fixed shutdown timeout"). Changed: the
wait had no bound and could block `Run` for good (#454).

#### Scenario: A backlogged watch at cancellation

- **WHEN** `Run`'s context is cancelled while nats.go's watch delivery is blocked on a full updates buffer
- **THEN** `Run` returns nil only after that delivery has ended
- **AND** the test that verifies this is `TestFollowerRunReturnsAndLeavesNothing`

#### Scenario: The watch's delivery never closes

- **WHEN** the watch is stopped and its update channel stays open
- **THEN** the wait returns, after 5 s and not before, an error saying the delivery did not close after `Stop`
- **AND** the test that verifies this is `TestStopWatchGivesUpOnADeliveryThatNeverCloses`
