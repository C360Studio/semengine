# output-websocket

## ADDED Requirements

### Requirement: One input per instance

The `websocket` component SHALL take exactly one input: a kv-watch of every `ENTITY_STATES` key with no history, or a
core NATS subject. It SHALL refuse any other input at construction, naming the port, and SHALL name a kv-watch of
`ENTITY_STATES` as the replacement for a `jetstream` input. It SHALL refuse the keys `delivery_mode`, `ack_timeout` and
`passthrough`, naming each.

#### Scenario: Removed surface refused

- **WHEN** a configuration carries `delivery_mode`, or a `jetstream` input
- **THEN** construction fails with an error naming that key or port (`TestConfigRefusals`)

### Requirement: Applied state is a snapshot, a marker, then every change

For a kv-watch input, a client SHALL receive one `put` per current entity and one `poisoned` per poisoned key, then
one `synced` frame carrying the revision, then each later change in revision order, with no change missing or
repeated. Each `put` payload SHALL be the stored bytes of that revision.

#### Scenario: Late client

- **WHEN** an entity is written, then a client connects, then a second entity is written and the first deleted
- **THEN** the client receives `put` of the first, `synced`, `put` of the second, `delete` of the first
  (`TestAppliedStateSnapshotThenChanges`)

### Requirement: No stream before the view is caught up

Until the instance's view has caught up, and while its watch is lost, an upgrade SHALL be refused with 503 and
`Health` SHALL be degraded naming why. A lost watch SHALL close every client with code 1013.

#### Scenario: Not caught up

- **WHEN** `ENTITY_STATES` does not exist
- **THEN** an upgrade gets 503 and `Health` is degraded; once the bucket exists and the view catches up, an upgrade
  succeeds (`TestNotReadyRefusesUpgrade`)

### Requirement: A subject is delivered at most once, unchanged

For a subject input, each message that is valid JSON SHALL be sent to every connected client as a `data` frame whose
`payload` is the message's bytes unchanged. A message that is not JSON SHALL NOT be sent and SHALL be counted. Nothing
SHALL be acknowledged or redelivered.

#### Scenario: Fan-out

- **WHEN** two clients are connected and a JSON message and a non-JSON message are published
- **THEN** both receive one `data` frame with the JSON bytes unchanged, and the drop counter rises by one
  (`TestSubjectFanOut`)

### Requirement: A slow client never slows another

A client that stops reading SHALL NOT delay frames to other clients. On a subject input its queue SHALL discard its
oldest frame when full and count the discard; on applied state its pending changes SHALL merge per key.

#### Scenario: Stalled client

- **WHEN** one client stops reading while frames continue
- **THEN** another client receives the last frame (`TestStalledClientDoesNotSlowOthers`)

### Requirement: Keep-alive

The component SHALL ping each client every `ping_interval`, and a pong SHALL extend the read deadline to twice that. A
client that answers pings and sends nothing SHALL stay connected, and a client that answers none SHALL be closed.

#### Scenario: Quiet client

- **WHEN** a client answers pings and sends nothing for several read deadlines
- **THEN** it still receives the next frame (`TestQuietClientStaysConnected`)

### Requirement: Start bounds start; Stop is the only stop

A failed `Start` SHALL release the listener. Ending `Start`'s context after it returned SHALL stop nothing. `Stop` SHALL
close every client with code 1001, and SHALL release the port and leave no client counted once it returns nil.

#### Scenario: Stop

- **WHEN** a running instance with connected clients is stopped
- **THEN** each client sees close 1001, a dial of the port is refused, and `clients_connected` is 0 (lifecycle suite,
  `TestStartContextEndingStopsNothing`)
