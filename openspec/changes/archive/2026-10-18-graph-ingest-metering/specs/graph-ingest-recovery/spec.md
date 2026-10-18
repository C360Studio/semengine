# graph-ingest-recovery

## ADDED Requirements

### Requirement: graph-ingest's data flow counts input messages

graph-ingest's `DataFlow()` SHALL count each input message once: each delivery on a stream input, redeliveries
included, and each mutation request that reaches a handler. `BytesPerSecond` SHALL count those messages' payload
bytes. Query requests, in-process creates and the containers hierarchy inference creates SHALL NOT count. `ErrorRate`
SHALL be the processing errors counted since start divided by the input messages counted.

#### Scenario: A stream message that creates containers

- **WHEN** one stream message is applied and hierarchy inference creates two containers for it
- **THEN** `DataFlow()` counts one message

#### Scenario: An append to three subjects

- **WHEN** one append request writes three entities
- **THEN** `DataFlow()` counts one message
