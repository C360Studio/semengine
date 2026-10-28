# graph-ingest-recovery

## ADDED Requirements

### Requirement: Stop returns only after every input callback has returned

graph-ingest's `Stop` SHALL return nil only after every JetStream input's delivery has ended and every consume
callback has returned, through `natsclient`'s `WaitConsumerHandlers`, never on the consume loop's `Closed()` alone. A
`Stop` whose context ends first SHALL return that context's error and keep its inputs for a later `Stop` (#441).

#### Scenario: A callback held after the connection closed for good

- **WHEN** a consume callback is held, the client's connection closes for good and the consume loop's `Closed()` has
  fired, and `Stop` runs under a context that ends before the callback is released
- **THEN** `Stop` returns that context's error and keeps its inputs, and a `Stop` after the callback returns
  returns nil and holds nothing
- **AND** the test that verifies this is `TestStopJoinsCallbackAfterConnectionLoss`
